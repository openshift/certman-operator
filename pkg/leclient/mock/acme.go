package mock

import (
	"crypto"
	"crypto/x509"
	"errors"

	"github.com/eggsampler/acme"
)

type FakeAcmeClient struct {
	// whether Let's Encrypt is working or not
	Available bool

	UpdateAccountCalled      bool
	Contacts                 []string
	NewOrderCalled           bool
	Identifiers              []acme.Identifier
	FetchAuthorizationCalled bool
	UpdateChallengeCalled    bool
	FinalizeOrderCalled      bool
}

func (fac *FakeAcmeClient) UpdateAccount(account acme.Account, tosAgreed bool, contacts ...string) (rAccount acme.Account, err error) {
	// track if this was called
	fac.UpdateAccountCalled = true
	fac.Contacts = contacts

	if !fac.Available {
		err = errors.New("acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details")
	}

	return
}

func (fac *FakeAcmeClient) FetchAuthorization(a acme.Account, url string) (aAuth acme.Authorization, err error) {
	fac.FetchAuthorizationCalled = true
	aAuth.URL = url

	if !fac.Available {
		err = errors.New("acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details")
	}

	return
}
func (fac *FakeAcmeClient) FetchCertificates(acme.Account, string) (cert []*x509.Certificate, err error) {
	return
}

func (fac *FakeAcmeClient) FinalizeOrder(acme.Account, acme.Order, *x509.CertificateRequest) (order acme.Order, err error) {
	fac.FinalizeOrderCalled = true

	if !fac.Available {
		err = errors.New("acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details")
	}

	return
}

func (fac *FakeAcmeClient) NewOrder(a acme.Account, ids []acme.Identifier) (order acme.Order, err error) {
	// track if this was called
	fac.NewOrderCalled = true
	fac.Identifiers = ids

	if !fac.Available {
		err = errors.New("acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details")
	}

	return
}
func (fac *FakeAcmeClient) RevokeCertificate(acme.Account, *x509.Certificate, crypto.Signer, int) error {
	return nil
}

func (fac *FakeAcmeClient) UpdateChallenge(acme.Account, acme.Challenge) (challenge acme.Challenge, err error) {
	fac.UpdateChallengeCalled = true

	if !fac.Available {
		err = errors.New("acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details")
	}

	return
}

func NewFakeAcmeClient() (fac *FakeAcmeClient) {
	fac.UpdateAccountCalled = false

	return
}
