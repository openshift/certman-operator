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

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	"github.com/openshift/certman-operator/pkg/leclient"
	"github.com/openshift/certman-operator/pkg/localmetrics"
)

// IssueCertificate validates DNS write access then assess letsencrypt endpoint (prod or stage) based on leclient url.
// It then iterates through the CertificateRequest.Spec.DnsNames, authorizes to letsencrypt and sets a challenge in the
// form of resource record. Certificates are then generated and issued to kubernetes via corev1.
func (r *ReconcileCertificateRequest) IssueCertificate(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest, certificateSecret *corev1.Secret) error {
	timer := prometheus.NewTimer(localmetrics.MetricIssueCertificateDuration)
	defer localmetrics.UpdateCertificateIssueDurationMetric(timer.ObserveDuration())

	// Get DNS client from CR.
	dnsClient, err := r.getClient(cr)
	if err != nil {
		reqLogger.Error(err, err.Error())
		return err
	}

	proceed, err := dnsClient.ValidateDNSWriteAccess(reqLogger, cr)
	if err != nil {
		return err
	}

	if proceed {
		reqLogger.Info("write permissions for DNS has been validated")
	} else {
		reqLogger.Error(err, "failed to get write access to DNS record")
		return err
	}

	leClient, err := leclient.NewClient(r.client)
	if err != nil {
		reqLogger.Error(err, "failed to get letsencrypt client")
		return err
	}

	err = leClient.UpdateAccount(cr.Spec.Email)
	if err != nil {
		reqLogger.Error(err, "failed to update letsencrypt account")
		return err
	}

	err = leClient.CreateOrder(cr.Spec.DnsNames)
	if err != nil {
		return err
	}
	URL, err := leClient.GetOrderURL()
	if err != nil {
		return err
	}
	reqLogger.Info("created a new order with Let's Encrypt.", "URL", URL)

	for _, authURL := range leClient.OrderAuthorization() {
		err := leClient.FetchAuthorization(authURL)
		if err != nil {
			reqLogger.Error(err, "could not fetch authorizations")
			return err
		}

		domain, domErr := leClient.GetAuthorizationIndentifier()
		if domErr != nil {
			return fmt.Errorf("Could not read domain for authorization")
		}
		err = leClient.SetChallengeType()
		if err != nil {
			return fmt.Errorf("Could not set Challenge type")
		}

		DNS01KeyAuthorization, keyAuthErr := leClient.GetDNS01KeyAuthorization()
		if keyAuthErr != nil {
			return fmt.Errorf("Could not get authorization key for dns challenge")
		}
		fqdn, err := dnsClient.AnswerDNSChallenge(reqLogger, DNS01KeyAuthorization, domain, cr)

		if err != nil {
			return err
		}

		dnsChangesVerified := VerifyDnsResourceRecordUpdate(reqLogger, fqdn, DNS01KeyAuthorization)
		if !dnsChangesVerified {
			return fmt.Errorf("cannot complete Let's Encrypt challenege as DNS changes could not be verified")
		}

		reqLogger.Info(fmt.Sprintf("updating challenge for authorization %v: %v", domain, leClient.GetChallengeURL()))
		err = leClient.UpdateChallenge()
		if err != nil {
			reqLogger.Error(err, fmt.Sprintf("error updating authorization %s challenge: %v", domain, err))
			return err
		}

		reqLogger.Info("challenge successfully completed")
	}

	reqLogger.Info("generating new key")

	certKey, err := rsa.GenerateKey(rand.Reader, rSAKeyBitSize)
	if err != nil {
		return err
	}

	reqLogger.Info("creating certificate signing request")

	// Use the certkey to generate a csr for the requested domains
	csr, err := generateCRS(certKey, cr.Spec.DnsNames)
	if err != nil {
		return err
	}

	reqLogger.Info("fetching certificates")

	// Use the lets encrypt client to get certs for the csr
	certs, err := getCerts(csr, leClient)
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
	}

	reqLogger.Info("certificates are now available")

	// After resolving all new challenges, and storing the cert, delete the challenge records
	// that were used from dns in this zone.
	err = dnsClient.DeleteAcmeChallengeResourceRecords(reqLogger, cr)
	if err != nil {
		reqLogger.Error(err, "error occurred deleting acme challenge resource records from Route53")
	}

	return nil
}

// generateCRS takes an RSA private key and a slice of domains. These are used to create an x509 certificate request
// which is returned. An error is also returned if present.
func generateCRS(certKey *rsa.PrivateKey, domains []string) (csr *x509.CertificateRequest, err error) {
	tpl := &x509.CertificateRequest{
		SignatureAlgorithm: x509.SHA256WithRSA,
		PublicKeyAlgorithm: x509.RSA,
		PublicKey:          certKey.Public(),
		Subject:            pkix.Name{CommonName: domains[0]},
		DNSNames:           domains,
	}
	csrDer, err := x509.CreateCertificateRequest(rand.Reader, tpl, certKey)
	if err != nil {
		return csr, err
	}

	csr, err = x509.ParseCertificateRequest(csrDer)
	if err != nil {
		return csr, err
	}
	return csr, nil
}

// getCerts take an x509 Certificate Request and a Let's Encrypt client. It signs and finalizies an order
// then retreieves the certificates as a slice of x509 certificates. An error is also returned if present.
func getCerts(csr *x509.CertificateRequest, leClient *leclient.ACMEClient) (certs []*x509.Certificate, err error) {
	err = leClient.FinalizeOrder(csr)
	if err != nil {
		return certs, err
	}

	certs, err = leClient.FetchCertificates()
	if err != nil {
		return certs, err
	}
	return certs, nil
}
