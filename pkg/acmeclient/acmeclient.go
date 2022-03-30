package acmeclient

import (
	"crypto"
	"crypto/x509"

	"github.com/eggsampler/acme"
)

// the real acme client is github.com/eggsampler/acme
// this package simply defines an interface for it

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
