/*
Copyright 2019 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package certificaterequest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"

	"github.com/eggsampler/acme"
	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	"github.com/openshift/certman-operator/pkg/controller/controllerutils"

	corev1 "k8s.io/api/core/v1"
)

func (r *ReconcileCertificateRequest) IssueCertificate(cr *certmanv1alpha1.CertificateRequest, certificateSecret *corev1.Secret) error {
	proceed, err := r.ValidateDnsWriteAccess(cr)
	if err != nil {
		return err
	}

	if proceed {
		log.Info("Route53 Access has been validated")
	}

	useLetsEncryptStagingEndpoint := controllerutils.UsetLetsEncryptStagingEnvironment(r.client)

	letsEncryptClient, err := GetLetsEncryptClient(useLetsEncryptStagingEndpoint)
	if err != nil {
		return err
	}

	accountUrl, err := GetLetsEncryptAccountUrl(r.client, useLetsEncryptStagingEndpoint)
	if err != nil {
		return err
	}

	privateKey, err := GetLetsEncryptAccountPrivateKey(r.client, useLetsEncryptStagingEndpoint)
	if err != nil {
		return err
	}

	letsEncryptAccount := acme.Account{PrivateKey: privateKey, URL: accountUrl}

	certExpiryNotificationList := GetCertExpiryNotificationList(cr.Spec.Email)

	letsEncryptAccount, err = letsEncryptClient.UpdateAccount(letsEncryptAccount, true, certExpiryNotificationList...)
	if err != nil {
		return err
	}

	var certDomains []string
	var ids []acme.Identifier

	for _, domain := range cr.Spec.DnsNames {
		log.Info("Domain to be added: " + domain)
		certDomains = append(certDomains, domain)
		ids = append(ids, acme.Identifier{Type: "dns", Value: domain})
	}

	letsEncryptOrder, err := letsEncryptClient.NewOrder(letsEncryptAccount, ids)
	if err != nil {
		return err
	}

	log.Info("letsEncryptOrder.URL", "OrderUrl", letsEncryptOrder.URL)

	for _, authUrl := range letsEncryptOrder.Authorizations {

		authorization, err := letsEncryptClient.FetchAuthorization(letsEncryptAccount, authUrl)
		if err != nil {
			log.Error(err, "There was problem fetching authorizations.")
		}

		domain := authorization.Identifier.Value

		challenge, ok := authorization.ChallengeMap[acme.ChallengeTypeDNS01]
		if !ok {
			return fmt.Errorf("Cloud not find DNS Challenge Authorization.")
		}

		encodeDNS01KeyAuthorization := acme.EncodeDNS01KeyAuthorization(challenge.KeyAuthorization)

		fqdn, err := r.AnswerDnsChallenge(encodeDNS01KeyAuthorization, domain, cr)
		if err != nil {
			return err
		}

		dnsChangesVerified := VerifyDnsResourceRecordUpdate(fqdn, encodeDNS01KeyAuthorization)
		if !dnsChangesVerified {
			return fmt.Errorf("Could not verify DNS changes. Cannot complete Let's Encrypt challenege.")
		}

		log.Info(fmt.Sprintf("Updating challenge for authorization %v: %v", authorization.Identifier.Value, challenge.URL))

		challenge, err = letsEncryptClient.UpdateChallenge(letsEncryptAccount, challenge)
		if err != nil {
			log.Error(err, fmt.Sprintf("Error updating authorization %s challenge: %v", authorization.Identifier.Value, err))
			return err
		}

		log.Info("Challenge successfully updated.")
	}

	certKey, err := rsa.GenerateKey(rand.Reader, RSAKeyBitSize)
	if err != nil {
		return err
	}

	log.Info("Creating certificate signing request")

	tpl := &x509.CertificateRequest{
		SignatureAlgorithm: x509.SHA256WithRSA,
		PublicKeyAlgorithm: x509.RSA,
		PublicKey:          certKey.Public(),
		Subject:            pkix.Name{CommonName: certDomains[0]},
		DNSNames:           certDomains,
	}

	csrDer, err := x509.CreateCertificateRequest(rand.Reader, tpl, certKey)
	if err != nil {
		return err
	}

	csr, err := x509.ParseCertificateRequest(csrDer)
	if err != nil {
		return err
	}

	letsEncryptOrder, err = letsEncryptClient.FinalizeOrder(letsEncryptAccount, letsEncryptOrder, csr)
	if err != nil {
		return err
	}

	certs, err := letsEncryptClient.FetchCertificates(letsEncryptAccount, letsEncryptOrder.Certificate)
	if err != nil {
		return err
	}

	var pemData []string

	for _, c := range certs {
		pemData = append(pemData, string(pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: c.Raw,
		})))
	}

	key := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certKey),
	})

	certificateSecret.Labels = map[string]string{
		"certificate_request": cr.Name,
	}

	certificateSecret.Data = map[string][]byte{
		corev1.TLSCertKey:       []byte(pemData[0] + pemData[1]), // create fullchain
		corev1.TLSPrivateKeyKey: key,
		// "letsencrypt.ca.crt":    []byte(pemData[1]),
	}

	return nil
}
