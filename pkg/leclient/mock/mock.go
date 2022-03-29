package mock

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
)

const (
	// this cert has never been used for anything and never should be
	selfSignedCert = `-----BEGIN CERTIFICATE-----
MIIDozCCAougAwIBAgIUP5hPTIPF1+p4pnEH5DSz8FtJ5YwwDQYJKoZIhvcNAQEL
BQAwYTELMAkGA1UEBhMCVVMxFzAVBgNVBAgMDk5vcnRoIENhcm9saW5hMRkwFwYD
VQQKDBBjZXJ0bWFuLW9wZXJhdG9yMR4wHAYDVQQDDBV0aGlzLndpbGwubm90LmJl
LnVzZWQwHhcNMjExMDE0MTkyMjE3WhcNMzExMDEyMTkyMjE3WjBhMQswCQYDVQQG
EwJVUzEXMBUGA1UECAwOTm9ydGggQ2Fyb2xpbmExGTAXBgNVBAoMEGNlcnRtYW4t
b3BlcmF0b3IxHjAcBgNVBAMMFXRoaXMud2lsbC5ub3QuYmUudXNlZDCCASIwDQYJ
KoZIhvcNAQEBBQADggEPADCCAQoCggEBAOQUdoHGS0Zw5YmYtOteBp7yKMDIo7H8
InGzfmjcy01H6Y3SFYf/CrVzmXFs/ln9OtTqzgfjyq6Ledww3ugFWvYg5Ae80OW2
THgSZRkX087SE1cZmfmUiU/Jw1ErkV/sK1f6D8hIMChowOE0S8BCX+lmWrjdVDFX
p1n1UHkRs6yW8UwUfHKxD5ImCgfQ8rnqFGRk0ghU99Qf4NiEkgkPp36Ry3fykl+Y
V4z7LWtiYftqXAWfAmGfPjO6r1SkFDjJ0oJ9/dy5z3GrEmO3XTvalZzUcFqaBqp/
5PczmnB7uylMNOyI2ua20Q+QCgiwtCeS7lQT5kI2A92mSpcTaH/MvMcCAwEAAaNT
MFEwHQYDVR0OBBYEFGFoRJ6jjwiAb4RgBQcIxQY5/Fr+MB8GA1UdIwQYMBaAFGFo
RJ6jjwiAb4RgBQcIxQY5/Fr+MA8GA1UdEwEB/wQFMAMBAf8wDQYJKoZIhvcNAQEL
BQADggEBABxeZ3DGf2WT6qPoetsX2HDLe1MmVIUQKm+KyhVzHUG2w3AlBGSIseYn
fAo22Q0jIVKFLmwaqZ+zsFyQBYvfPbGXxJlZZgWdbEFLsTV2vNdPqxd4uNFrIpcm
j98F4jOhs0fnsZPRxsxqbaxP2NYHGdIDtA7wFjePwrXsWeYyy5SI7G8QA1NyZyAz
sDEfIo7D++lLQ3khYVroF1UvT//yOBOAz3RSWwoBn7sMZXMtMauVq0yG//P5u0bV
7nhj+6L8QiC3WRRVihg8nRQc8jYrKzAxJBSBaS6Z3/dk5tc0ebp2VRB1ViNbpJVf
gdjgb7+vHIRkKtM46b1t/XNU8885PIo=
-----END CERTIFICATE-----`
)

/*
Mock certman-operator/pkg/leclient/LetsEncryptClient
this lets us unit test issuing certs, etc
*/
// this implements leclient.LetsEncryptClient
type FakeLetsEncryptClient struct {
	Available bool
}

type FakeLetsEncryptClientOptions struct {
	Available bool
}

func NewFakeLetsEncryptClient(opts *FakeLetsEncryptClientOptions) (mc *FakeLetsEncryptClient) {
	mc = &FakeLetsEncryptClient{
		Available: opts.Available,
	}

	return
}

func (c *FakeLetsEncryptClient) UpdateAccount(email string) (err error) {
	if !c.Available {
		err = errors.New("acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details")
	}

	return
}

func (c *FakeLetsEncryptClient) CreateOrder(domains []string) (err error) {
	return
}

func (c *FakeLetsEncryptClient) GetOrderURL() string {
	return ""
}

func (c *FakeLetsEncryptClient) OrderAuthorization() []string {
	return []string{}
}

func (c *FakeLetsEncryptClient) FetchAuthorization(authURL string) (err error) {
	return
}

func (c *FakeLetsEncryptClient) GetAuthorizationURL() string {
	return "https://a.fake.url"
}

func (c *FakeLetsEncryptClient) GetAuthorizationIndentifier() (AuthID string, err error) {
	return
}

func (c *FakeLetsEncryptClient) SetChallengeType() (err error) {
	return
}

func (c *FakeLetsEncryptClient) GetChallengeURL() string {
	return ""
}

func (c *FakeLetsEncryptClient) GetDNS01KeyAuthorization() (keyAuth string, err error) {
	return "", nil
}

func (c *FakeLetsEncryptClient) UpdateChallenge() (err error) {
	return
}

func (c *FakeLetsEncryptClient) FinalizeOrder(csr *x509.CertificateRequest) (err error) {
	return
}

func (c *FakeLetsEncryptClient) GetOrderEndpoint() string {
	return ""
}

func (c *FakeLetsEncryptClient) FetchCertificates() (certbundle []*x509.Certificate, err error) {
	certPem, _ := pem.Decode([]byte(selfSignedCert))
	cert, _ := x509.ParseCertificate(certPem.Bytes)
	certbundle = []*x509.Certificate{cert, cert}
	return
}

func (c *FakeLetsEncryptClient) RevokeCertificate(certificate *x509.Certificate) (err error) {
	return
}
