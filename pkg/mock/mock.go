package mock

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"

	"github.com/eggsampler/acme"
)

type FakeAcmeClient struct {
	// whether Let's Encrypt is working or not
	Available                bool
	NewOrderResult           acme.Order
	FetchAuthorizationResult acme.Authorization

	Challenge   acme.Challenge
	Contacts    []string
	Identifiers []acme.Identifier

	FetchAuthorizationCalled bool
	FetchCertificatesCalled  bool
	FinalizeOrderCalled      bool
	NewOrderCalled           bool
	RevokeCertificateCalled  bool
	UpdateAccountCalled      bool
	UpdateChallengeCalled    bool
}

type FakeAcmeClientOptions struct {
	Available                bool
	NewOrderResult           acme.Order
	FetchAuthorizationResult acme.Authorization
	UpdateAccountCalled      bool
	NewOrderCalled           bool
	FetchAuthorizationCalled bool
	UpdateChallengeCalled    bool
	FinalizeOrderCalled      bool
	FetchCertificatesCalled  bool
	RevokeCertificateCalled  bool
}

func NewFakeAcmeClient(opts *FakeAcmeClientOptions) (fac *FakeAcmeClient) {
	fac = &FakeAcmeClient{}
	fac.NewOrderResult = opts.NewOrderResult
	fac.FetchAuthorizationResult = opts.FetchAuthorizationResult
	fac.Available = opts.Available
	fac.FetchAuthorizationCalled = opts.FetchAuthorizationCalled
	fac.FetchCertificatesCalled = opts.FetchCertificatesCalled
	fac.FinalizeOrderCalled = opts.FinalizeOrderCalled
	fac.NewOrderCalled = opts.NewOrderCalled
	fac.RevokeCertificateCalled = opts.RevokeCertificateCalled
	fac.UpdateAccountCalled = opts.UpdateAccountCalled
	fac.UpdateChallengeCalled = opts.UpdateChallengeCalled

	return
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

	if !fac.Available {
		err = errors.New("acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details")
	} else {
		aAuth = fac.FetchAuthorizationResult
	}

	return
}

// this should return at least two certificates, the cert itself and the
// intermediate cert so they can be combined into a fullchain in leclient.IssueCertificate()
func (fac *FakeAcmeClient) FetchCertificates(acme.Account, string) (cert []*x509.Certificate, err error) {
	fac.FetchCertificatesCalled = true

	if !fac.Available {
		err = errors.New("acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details")
	} else {
		// this is the garbage self-signed cert from leclient's test helpers. it
		// should never be used for anything else, ever. since it's self-signed, it
		// will be its own intermediate cert
		certBlock, _ := pem.Decode([]byte(`-----BEGIN CERTIFICATE-----
MIIC2DCCAkGgAwIBAgIUH0hB45DuH9g3KyLn+Vaip0tTFRMwDQYJKoZIhvcNAQEL
BQAwazELMAkGA1UEBhMCVVMxFzAVBgNVBAgMDk5vcnRoIENhcm9saW5hMSEwHwYD
VQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQxIDAeBgNVBAMMF2FwaS5naWJi
ZXJpc2guZ29lcy5oZXJlMCAXDTIxMDIyMzIxMzEwOFoYDzIxMjEwMTMwMjEzMTA4
WjBrMQswCQYDVQQGEwJVUzEXMBUGA1UECAwOTm9ydGggQ2Fyb2xpbmExITAfBgNV
BAoMGEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDEgMB4GA1UEAwwXYXBpLmdpYmJl
cmlzaC5nb2VzLmhlcmUwgZ8wDQYJKoZIhvcNAQEBBQADgY0AMIGJAoGBALoL1zJb
vIyORwmGXQnViUQU8ZfJIEP0yp/V7wh/iS6l8VTZkTWfhMdNJcFxhZ7ZCg16e1gy
InuOGFJzoAZt9iydQ56CmNjCZ4W3F5vbyS28wxDeOf3ReCBpePN2JaXmyeoMTtrC
pe5X9WDGM058bJjZj+eRIwvRFwd5vOE7DX/hAgMBAAGjdzB1MB0GA1UdDgQWBBSQ
nk9x0PpBkPvIJPofngFlDmUQfjAfBgNVHSMEGDAWgBSQnk9x0PpBkPvIJPofngFl
DmUQfjAPBgNVHRMBAf8EBTADAQH/MCIGA1UdEQQbMBmCF2FwaS5naWJiZXJpc2gu
Z29lcy5oZXJlMA0GCSqGSIb3DQEBCwUAA4GBAI9pcwgyuy7bWn6E7GXALwvA/ba5
8Rjjs000wrPpSHJpaIwxp8BNVkCwADewF3RUZR4qh0hicOduOIbDpsRQbuIHBR9o
BNfwM5mTnLOijduGlf52SqIW8l35OjtiBvzSVXoroXdvKxC35xTuwJ+Q5GGynVDs
VoZplnP9BdVECzSa
-----END CERTIFICATE-----`))

		cert = append(cert, &x509.Certificate{Raw: certBlock.Bytes})
		cert = append(cert, &x509.Certificate{Raw: certBlock.Bytes})
	}

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
	} else {
		order = fac.NewOrderResult
	}

	return
}

func (fac *FakeAcmeClient) RevokeCertificate(acme.Account, *x509.Certificate, crypto.Signer, int) (err error) {
	fac.RevokeCertificateCalled = true

	if !fac.Available {
		err = errors.New("acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details")
	}

	return err
}

func (fac *FakeAcmeClient) UpdateChallenge(a acme.Account, c acme.Challenge) (challenge acme.Challenge, err error) {
	fac.UpdateChallengeCalled = true
	challenge = c

	if !fac.Available {
		err = errors.New("acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details")
	}

	return
}
