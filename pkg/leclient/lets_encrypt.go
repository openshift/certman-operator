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
	//"fmt"
	"strings"

	"github.com/eggsampler/acme"
	"github.com/go-logr/logr"
	"github.com/openshift/certman-operator/config"
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
}

type ACMEClient struct {
	Client        acme.Client
	Account       acme.Account
	Order         acme.Order
	Authorization acme.Authorization
	Challenge     acme.Challenge
}

func (c *ACMEClient) UpdateAccount(certExpiryNotificationList []string) (err error) {
	c.Account, err = c.Client.UpdateAccount(c.Account, true, certExpiryNotificationList...)
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

func (c *ACMEClient) GetAccount(kubeClient client.Client, staging bool, namespace string) (err error) {
	accountURL, err := getLetsEncryptAccountURL(kubeClient, true)
	if err != nil {
		return err
	}

	privateKey, err := getLetsEncryptAccountPrivateKey(kubeClient, true)
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
func (c *ACMEClient) GetAuthorizationIndentifier() string {
	return c.Authorization.Identifier.Value
}
func (c *ACMEClient) SetChallengeType() {
	c.Challenge = c.Authorization.ChallengeMap["dns-01"]
}
func (c *ACMEClient) GetDNS01KeyAuthorization() string {
	return acme.EncodeDNS01KeyAuthorization(c.Challenge.KeyAuthorization)
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
func (c *ACMEClient) GetOrderEndpoint() string{
	return c.Order.Certificate
}
func (c *ACMEClient) FetchCertificates() (certbundle []*x509.Certificate, err error){
	certbundle, err = c.Client.FetchCertificates(c.Account, c.Order.Certificate)
	return certbundle, err
}
func NewGetLetsEncryptClient(staging bool) (Client ACMEClient, err error) {
	if staging {
		Client.Client, err = acme.NewClient(acme.LetsEncryptStaging)
		return Client, err
	}
	Client.Client, err = acme.NewClient(acme.LetsEncryptProduction)
	return Client, err
}

func GetLetsEncryptClient(staging bool) (acme.Client, error) {

	if staging {
		return acme.NewClient(acme.LetsEncryptStaging)
	}

	return acme.NewClient(acme.LetsEncryptProduction)
}

func GetAccount(reqLogger logr.Logger, kubeClient client.Client, staging bool, namespace string) (letsEncryptAccount acme.Account, err error) {

	accountURL, err := getLetsEncryptAccountURL(kubeClient, true)
	if err != nil {
		return letsEncryptAccount, err
	}

	privateKey, err := getLetsEncryptAccountPrivateKey(kubeClient, true)
	if err != nil {
		return letsEncryptAccount, err
	}
	letsEncryptAccount = acme.Account{PrivateKey: privateKey, URL: accountURL}
	return letsEncryptAccount, nil
}

func getLetsEncryptAccountPrivateKey(kubeClient client.Client, staging bool) (privateKey crypto.Signer, err error) {

	secretName := LetsEncryptProductionAccountSecretName

	if staging {
		secretName = LetsEncryptStagingAccountSecretName
	}

	secret, err := GetSecret(kubeClient, secretName, config.OperatorNamespace)
	if err != nil {
		return privateKey, err
	}

	keyBytes := secret.Data[LetsEncryptAccountPrivateKey]
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

func getLetsEncryptAccountURL(kubeClient client.Client, staging bool) (url string, err error) {

	secretName := LetsEncryptProductionAccountSecretName

	if staging {
		secretName = LetsEncryptStagingAccountSecretName
	}

	secret, err := GetSecret(kubeClient, secretName, config.OperatorNamespace)
	if err != nil {
		return "", err
	}

	urlBytes := secret.Data[LetsEncryptAccountUrl]
	url = string(urlBytes)
	url = strings.TrimRight(url, "\n")

	return url, nil
}

func GetCertExpiryNotificationList(email string) []string {
	var contacts []string

	if email != "" {
		contacts = append(contacts, "mailto:"+email)
	}

	return contacts
}

// func EncodeDNS01KeyAuthorization(keyauth string) string {
// 	encode := acme.EncodeDNS01KeyAuthorization(keyauth)
// 	return encode
// }
// func ChallengeType(auth acme.Authorization) (acme.Challenge, error) {
// 	//challenge, ok := auth.ChallengeMap["dns-01"]
// 	challenge, ok := auth.ChallengeMap[acme.ChallengeTypeDNS01]
// 	if !ok {
// 		var err error
// 		return challenge, err
// 	}
// 	return challenge, nil
// }

// func CreateOrder(reqLogger logr.Logger, leAccount acme.Account, leClient acme.Client, domains []string) (acme.Order, error) {
// 	var certDomains []string
// 	var ids []acme.Identifier

// 	for _, domain := range domains {
// 		reqLogger.Info(fmt.Sprintf("%v domain will be added to certificate request", domain))
// 		certDomains = append(certDomains, domain)
// 		ids = append(ids, acme.Identifier{Type: "dns", Value: domain})
// 	}
// 	letsEncryptOrder, err := leClient.NewOrder(leAccount, ids)
// 	if err != nil {
// 		return letsEncryptOrder, err
// 	}
// 	return letsEncryptOrder, nil
// }
