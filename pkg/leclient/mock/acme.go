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

	UpdateAccountCalled bool
	Contacts            []string
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

func (fac *FakeAcmeClient) FetchAuthorization(acme.Account, string) (aAuth acme.Authorization, err error) {
	return
}
func (fac *FakeAcmeClient) FetchCertificates(acme.Account, string) (cert []*x509.Certificate, err error) {
	return
}
func (fac *FakeAcmeClient) FinalizeOrder(acme.Account, acme.Order, *x509.CertificateRequest) (order acme.Order, err error) {
	return
}
func (fac *FakeAcmeClient) NewOrder(acme.Account, []acme.Identifier) (order acme.Order, err error) {
	return
}
func (fac *FakeAcmeClient) RevokeCertificate(acme.Account, *x509.Certificate, crypto.Signer, int) error {
	return nil
}
func (fac *FakeAcmeClient) UpdateChallenge(acme.Account, acme.Challenge) (challenge acme.Challenge, err error) {
	return
}

func NewFakeAcmeClient() (fac *FakeAcmeClient) {
	fac.UpdateAccountCalled = false

	return
}
