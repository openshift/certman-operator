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

package leclient

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"

	"net/url"
	"strings"

	"github.com/eggsampler/acme"
	"github.com/openshift/certman-operator/config"

	"k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Client interface {
	GetAccount(client.Client, bool, string) (acme.Account, error)
	UpdateAccount([]string)
	CreateOrder([]string)
	GetOrderURL()
	OrderAuthorization()
	FetchAuthorization(string)
	GetAuthorizationURL()
	GetAuthorizationIndentifier()
	SetChallengeType()
	GetChallengeURL()
	GetDNS01KeyAuthorization()
	UpdateChallenge()
	FinalizeOrder()
	GetOrderEndpoint()
	FetchCertificates()
	RevokeCertificate()
}

type ACMEClient struct {
	Client        acme.Client
	Account       acme.Account
	Order         acme.Order
	Authorization acme.Authorization
	Challenge     acme.Challenge
}

func (c *ACMEClient) UpdateAccount(email string) (err error) {
	var contacts []string

	if email != "" {
		contacts = append(contacts, "mailto:"+email)
	}

	c.Account, err = c.Client.UpdateAccount(c.Account, true, contacts...)
	return err
}

func (c *ACMEClient) CreateOrder(domains []string) (err error) {
	var certDomains []string
	var ids []acme.Identifier

	for _, domain := range domains {
		certDomains = append(certDomains, domain)
		ids = append(ids, acme.Identifier{Type: "dns", Value: domain})
	}
	c.Order, err = c.Client.NewOrder(c.Account, ids)
	if err != nil {
		return err
	}
	return nil
}

func (c *ACMEClient) GetAccount(kubeClient client.Client, namespace string) (err error) {
	accountURL, err := getLetsEncryptAccountURL(kubeClient)
	if err != nil {
		return err
	}

	privateKey, err := getLetsEncryptAccountPrivateKey(kubeClient)
	if err != nil {
		return err
	}
	c.Account = acme.Account{PrivateKey: privateKey, URL: accountURL}
	return nil
}

func (c *ACMEClient) GetOrderURL() (URL string, err error) {
	URL = c.Order.URL
	return URL, err
}

func (c *ACMEClient) OrderAuthorization() []string {
	return c.Order.Authorizations
}

func (c *ACMEClient) FetchAuthorization(authURL string) (err error) {
	c.Authorization, err = c.Client.FetchAuthorization(c.Account, authURL)
	return err
}
func (c *ACMEClient) GetAuthorizationURL() string {
	return c.Authorization.URL
}
func (c *ACMEClient) GetAuthorizationIndentifier() (AuthID string, err error) {
	AuthID = c.Authorization.Identifier.Value
	if AuthID == "" {
		err = errors.New("Authorization indentifier not currently set")
	}
	return AuthID, err
}
func (c *ACMEClient) SetChallengeType() (err error) {
	c.Challenge = c.Authorization.ChallengeMap["dns-01"]
	return err
}
func (c *ACMEClient) GetDNS01KeyAuthorization() (keyAuth string, err error) {
	keyAuth = acme.EncodeDNS01KeyAuthorization(c.Challenge.KeyAuthorization)
	if keyAuth == "" {
		err = errors.New("Authorization key not currently set")
	}
	return keyAuth, err
}
func (c *ACMEClient) GetChallengeURL() string {
	return c.Challenge.URL
}
func (c *ACMEClient) UpdateChallenge() (err error) {
	c.Challenge, err = c.Client.UpdateChallenge(c.Account, c.Challenge)
	return err
}
func (c *ACMEClient) FinalizeOrder(csr *x509.CertificateRequest) (err error) {
	c.Order, err = c.Client.FinalizeOrder(c.Account, c.Order, csr)
	return err
}
func (c *ACMEClient) GetOrderEndpoint() string {
	return c.Order.Certificate
}
func (c *ACMEClient) FetchCertificates() (certbundle []*x509.Certificate, err error) {
	certbundle, err = c.Client.FetchCertificates(c.Account, c.Order.Certificate)
	return certbundle, err
}
func (c *ACMEClient) RevokeCertificate(certificate *x509.Certificate) (err error) {
	err = c.Client.RevokeCertificate(c.Account, certificate, c.Account.PrivateKey, 0)
	return err
}
func GetLetsEncryptClient(directoryUrl string) (Client ACMEClient, err error) {
	Client.Client, err = acme.NewClient(directoryUrl)
	return Client, err
}

func getLetsEncryptAccountPrivateKey(kubeClient client.Client) (privateKey crypto.Signer, err error) {
	secret, err := getLetsEncryptAccountSecret(kubeClient)
	if err != nil {
		return privateKey, err
	}
	keyBytes := secret.Data[letsEncryptAccountPrivateKey]
	keyBlock, _ := pem.Decode(keyBytes)

	switch keyBlock.Type {
	case "RSA PRIVATE KEY":
		privateKey, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
		return privateKey, err
	case "EC PRIVATE KEY":
		privateKey, err = x509.ParseECPrivateKey(keyBlock.Bytes)
		return privateKey, err
	}

	return privateKey, nil
}

func GetLetsEncryptDirctoryURL(kubeClient client.Client) (durl string, err error) {
	accountUrl, err := getLetsEncryptAccountURL(kubeClient)
	if err != nil {
		return "", err
	}

	u, err := url.Parse(accountUrl)
	if err != nil {
		return "", err
	}

	durl = ""
	if strings.Contains(acme.LetsEncryptStaging, u.Host) {
		durl = acme.LetsEncryptStaging
	} else if strings.Contains(acme.LetsEncryptProduction, u.Host) {
		durl = acme.LetsEncryptProduction
	} else {
		return "", errors.New("cannot found let's encrypt directory url.")
	}

	return durl, nil
}

func getLetsEncryptAccountURL(kubeClient client.Client) (url string, err error) {

	secret, err := getLetsEncryptAccountSecret(kubeClient)
	if err != nil {
		return url, err
	}

	urlBytes := secret.Data[letsEncryptAccountUrl]
	url = string(urlBytes)
	url = strings.TrimRight(url, "\n")

	return url, nil
}

func getLetsEncryptAccountSecret(kubeClient client.Client) (secret *v1.Secret, err error) {
	secretName := letsEncryptAccountSecretName

	secret, err = GetSecret(kubeClient, secretName, config.OperatorNamespace)
	if err != nil {
		// If it's not found err, try to use the legacy production secret name for backward compatibility
		if kerr.IsNotFound(err) {
			secretName = letsEncryptProductionAccountSecretName
			secret, err = GetSecret(kubeClient, secretName, config.OperatorNamespace)
			// If it's not found err, try to use the legacy staging secret name for backward compatibility
			if kerr.IsNotFound(err) {
				secretName = letsEncryptStagingAccountSecretName
				secret, err = GetSecret(kubeClient, secretName, config.OperatorNamespace)
			}
		}
	}
	return
}
