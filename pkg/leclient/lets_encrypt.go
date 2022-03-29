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
	"fmt"
	"net/url"
	"strings"

	"github.com/eggsampler/acme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/certman-operator/config"
)

// make interface for github.com/eggsampler/acme.Client to allow testing
// the full acme.Client type uses all of these functions. i've only uncommented
// the ones we use for ease of mocking
type AcmeClientInterface interface {
	//AccountKeyChange(acme.Account, crypto.Signer) (acme.Account, error)
	//DeactivateAccount(acme.Account) (acme.Account, error)
	//DeactivateAuthorization(acme.Account, string) (acme.Authorization, error)
	//Directory() acme.Directory
	FetchAuthorization(acme.Account, string) (acme.Authorization, error)
	FetchCertificates(acme.Account, string) ([]*x509.Certificate, error)
	//FetchChallenge(acme.Account, string) (acme.Challenge, error)
	//FetchOrder(acme.Account, string) (acme.Order, error)
	FinalizeOrder(acme.Account, acme.Order, *x509.CertificateRequest) (acme.Order, error)
	//NewAccount(crypto.Signer, bool, bool, ...string) (acme.Account, error)
	NewOrder(acme.Account, []acme.Identifier) (acme.Order, error)
	//NewOrderDomains(acme.Account, ...string) (acme.Order, error)
	RevokeCertificate(acme.Account, *x509.Certificate, crypto.Signer, int) error
	UpdateAccount(acme.Account, bool, ...string) (acme.Account, error)
	UpdateChallenge(acme.Account, acme.Challenge) (acme.Challenge, error)
}

// define the LetsEncryptClientInterface interface
type LetsEncryptClientInterface interface {
	UpdateAccount(string) error
	CreateOrder([]string) error
	GetOrderURL() string
	OrderAuthorization() []string
	FetchAuthorization(string) error
	GetAuthorizationURL() string
	GetAuthorizationIndentifier() (string, error)
	SetChallengeType() error
	GetChallengeURL() string
	GetDNS01KeyAuthorization() (string, error)
	UpdateChallenge() error
	FinalizeOrder(*x509.CertificateRequest) error
	GetOrderEndpoint() string
	FetchCertificates() ([]*x509.Certificate, error)
	RevokeCertificate(*x509.Certificate) error
}

type LetsEncryptClient struct {
	Client        AcmeClientInterface
	Account       acme.Account
	Order         acme.Order
	Authorization acme.Authorization
	Challenge     acme.Challenge
}

// UpdateAccount updates the ACME clients account by accepting
// email address/'s as a string. If an error occurs, it is returned.
func (c *LetsEncryptClient) UpdateAccount(email string) (err error) {
	var contacts []string

	if email != "" {
		contacts = []string{fmt.Sprintf("mailto:%s", email)}
	}

	account, err := c.Client.UpdateAccount(c.Account, true, contacts...)
	if err != nil {
		return err
	}

	c.Account = account
	return err
}

// CreateOrder accepts and appends domain names to the acme.Identifier.
// It then calls acme.Client.NewOrder and returns nil if successful
// and an error if an error occurs.
func (c *LetsEncryptClient) CreateOrder(domains []string) (err error) {
	var ids []acme.Identifier

	for _, domain := range domains {
		ids = append(ids, acme.Identifier{Type: "dns", Value: domain})
	}
	c.Order, err = c.Client.NewOrder(c.Account, ids)
	if err != nil {
		return err
	}
	return nil
}

// GetOrderURL returns the URL field from the ACME Order struct.
func (c *LetsEncryptClient) GetOrderURL() string {
	return c.Order.URL
}

// OrderAuthorization returns the Authorizations field from the ACME
// Order struct.
func (c *LetsEncryptClient) OrderAuthorization() []string {
	return c.Order.Authorizations
}

// FetchAuthorization accepts an authURL and then calls acme.FetchAuthorization
// with both the authURL and c.Account from the ACME struct. If an error
// occurs it is returned.
func (c *LetsEncryptClient) FetchAuthorization(authURL string) (err error) {
	c.Authorization, err = c.Client.FetchAuthorization(c.Account, authURL)
	return err
}

// GetAuthorizationURL returns the URL from from the ACME Authorization struct.
func (c *LetsEncryptClient) GetAuthorizationURL() string {
	return c.Authorization.URL
}

// GetAuthorizationIndentifier returns the Authorization.Identifier.Value
// field from an ACME nested struct. An error is also returned if this field
// (.Value)is empty.
func (c *LetsEncryptClient) GetAuthorizationIndentifier() (AuthID string, err error) {
	AuthID = c.Authorization.Identifier.Value
	if AuthID == "" {
		err = errors.New("Authorization indentifier not currently set")
	}
	return AuthID, err
}

// SetChallengeType sets the local ACME structs challenge
// via the acme pkgs ChallengeMap. If an error occurs, it
// is returned.
func (c *LetsEncryptClient) SetChallengeType() (err error) {
	c.Challenge = c.Authorization.ChallengeMap["dns-01"]
	return err
}

// GetDNS01KeyAuthorization passes the KeyAuthorization string from the acme
// Challenge struct to the acme EncodeDNS01KeyAuthorization func. It returns
// this var as keyAuth. If this field is not set, an error is returned.
func (c *LetsEncryptClient) GetDNS01KeyAuthorization() (keyAuth string, err error) {
	keyAuth = acme.EncodeDNS01KeyAuthorization(c.Challenge.KeyAuthorization)
	if keyAuth == "" {
		err = errors.New("Authorization key not currently set")
	}
	return keyAuth, err
}

// GetChallengeURL returns the URL from the acme Challenge struct.
func (c *LetsEncryptClient) GetChallengeURL() string {
	return c.Challenge.URL
}

// UpdateChallenge calls the acme UpdateChallenge func with the local ACME
// structs Account and Challenge. If an error occurs, it is returned.
func (c *LetsEncryptClient) UpdateChallenge() (err error) {
	c.Challenge, err = c.Client.UpdateChallenge(c.Account, c.Challenge)
	return err
}

// FinalizeOrder accepts an x509.CertificateRequest as csr and calls acme FinalizeOrder
// by passing the csr along with the local ACME structs Account and Order. If an error
// occurs, it is returned.
func (c *LetsEncryptClient) FinalizeOrder(csr *x509.CertificateRequest) (err error) {
	c.Order, err = c.Client.FinalizeOrder(c.Account, c.Order, csr)
	return err
}

// GetOrderEndpoint returns the Certificate string from the acme Order struct.
func (c *LetsEncryptClient) GetOrderEndpoint() string {
	return c.Order.Certificate
}

// FetchCertificates calls the acme FetchCertificates Client method with the Account from
// the local ACME struct and Certificate from the acme Order struct. A slice of x509.Certificate's
// is returned along with an error if one occurrs.
func (c *LetsEncryptClient) FetchCertificates() (certbundle []*x509.Certificate, err error) {
	certbundle, err = c.Client.FetchCertificates(c.Account, c.Order.Certificate)
	return certbundle, err
}

// RevokeCertificate accepts x509.Certificate as certificate and calls the acme RevokeCertificate
// Client method along with local ACME structs Account and PrivateKey from the acme Account struct.
// If an error occurs, it is returned.
func (c *LetsEncryptClient) RevokeCertificate(certificate *x509.Certificate) (err error) {
	err = c.Client.RevokeCertificate(c.Account, certificate, c.Account.PrivateKey, 0)
	return err
}

// getLetsEncryptAccountPrivateKey accepts client.Client as kubeClient and retrieves the
// letsEncrypt account secret. The PrivateKey is de
func getLetsEncryptAccountPrivateKey(kubeClient client.Client) (privateKey crypto.Signer, err error) {
	secret, err := GetSecret(kubeClient, letsEncryptAccountSecretName, config.OperatorNamespace)
	if err != nil {
		return privateKey, err
	}
	if secret.Data[letsEncryptAccountPrivateKey] == nil {
		return nil, fmt.Errorf("lets encrypt private key not found")
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

func getLetsEncryptAccountURL(kubeClient client.Client) (url string, err error) {
	secret, err := GetSecret(kubeClient, letsEncryptAccountSecretName, config.OperatorNamespace)
	if err != nil {
		return url, err
	}

	urlBytes := secret.Data[letsEncryptAccountUrl]
	url = string(urlBytes)
	url = strings.TrimRight(url, "\n")

	return url, nil
}

// NewClient accepts a client.Client as kubeClient and calls the acme NewClient func.
// A LetsEncryptClient is returned, along with any error that occurs.
func NewClient(kubeClient client.Client) (*LetsEncryptClient, error) {
	accountURL, err := getLetsEncryptAccountURL(kubeClient)
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(accountURL)
	if err != nil {
		return nil, err
	}

	acmeClient := &LetsEncryptClient{}

	directoryURL := ""
	if strings.Contains(acme.LetsEncryptStaging, u.Host) {
		directoryURL = acme.LetsEncryptStaging
	} else if strings.Contains(acme.LetsEncryptProduction, u.Host) {
		directoryURL = acme.LetsEncryptProduction
	} else {
		return nil, errors.New("cannot found let's encrypt directory url")
	}

	acmeClient.Client, err = acme.NewClient(directoryURL)
	if err != nil {
		return nil, err
	}

	privateKey, err := getLetsEncryptAccountPrivateKey(kubeClient)
	if err != nil {
		return nil, err
	}

	if privateKey == nil {
		return nil, errors.New("private key cannot be empty")
	}
	acmeClient.Account = acme.Account{PrivateKey: privateKey, URL: accountURL}

	return acmeClient, nil
}
