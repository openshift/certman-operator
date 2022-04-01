package mock

import (
	"github.com/go-logr/logr"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
)

type MockClient struct {
	AnswerDNSChallengeFQDN  string
	AnswerDNSChallengeError error

	ValidateDNSWriteAccessBool  bool
	ValidateDNSWriteAccessError error

	DeleteAcmeChallengeResourceRecordsError error
}

type MockClientOptions struct {
	AnswerDNSChallengeFQDN  string
	AnswerDNSChallengeError error

	ValidateDNSWriteAccessBool  bool
	ValidateDNSWriteAccessError error

	DeleteAcmeChallengeResourceRecordsError error
}

func NewMockClient(opts *MockClientOptions) (c *MockClient) {
	c = &MockClient{}
	c.AnswerDNSChallengeFQDN = opts.AnswerDNSChallengeFQDN
	c.AnswerDNSChallengeError = opts.AnswerDNSChallengeError
	c.ValidateDNSWriteAccessBool = opts.ValidateDNSWriteAccessBool
	c.ValidateDNSWriteAccessError = opts.ValidateDNSWriteAccessError
	c.DeleteAcmeChallengeResourceRecordsError = opts.DeleteAcmeChallengeResourceRecordsError
	return
}

func (c *MockClient) GetDNSName() string {
	return "Mock"
}

func (c *MockClient) AnswerDNSChallenge(reqLogger logr.Logger, acmeChallengeToken string, domain string, cr *certmanv1alpha1.CertificateRequest) (string, error) {
	return c.AnswerDNSChallengeFQDN, c.AnswerDNSChallengeError
}

func (c *MockClient) ValidateDNSWriteAccess(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) (bool, error) {
	return c.ValidateDNSWriteAccessBool, c.ValidateDNSWriteAccessError
}

func (c *MockClient) DeleteAcmeChallengeResourceRecords(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) error {
	return c.DeleteAcmeChallengeResourceRecordsError
}
