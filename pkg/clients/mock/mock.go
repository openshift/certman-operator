package mock

import (
	"errors"

	"github.com/go-logr/logr"

	certmanv1alpha1 "github.com/openshift/certman-operator/api/v1alpha1"
)

type MockClient struct {
	AnswerDNSChallengeFQDN        string
	AnswerDNSChallengeErrorString string

	ValidateDNSWriteAccessBool        bool
	ValidateDNSWriteAccessErrorString string

	DeleteAcmeChallengeResourceRecordsErrorString string
}

type MockClientOptions struct {
	AnswerDNSChallengeFQDN        string
	AnswerDNSChallengeErrorString string

	ValidateDNSWriteAccessBool        bool
	ValidateDNSWriteAccessErrorString string

	DeleteAcmeChallengeResourceRecordsErrorString string
}

func NewMockClient(opts *MockClientOptions) (c *MockClient) {
	c = &MockClient{}
	c.AnswerDNSChallengeFQDN = opts.AnswerDNSChallengeFQDN
	c.AnswerDNSChallengeErrorString = opts.AnswerDNSChallengeErrorString
	c.ValidateDNSWriteAccessBool = opts.ValidateDNSWriteAccessBool
	c.ValidateDNSWriteAccessErrorString = opts.ValidateDNSWriteAccessErrorString
	c.DeleteAcmeChallengeResourceRecordsErrorString = opts.DeleteAcmeChallengeResourceRecordsErrorString
	return
}

func (c *MockClient) GetDNSName() string {
	return "Mock"
}

func (c *MockClient) AnswerDNSChallenge(reqLogger logr.Logger, acmeChallengeToken string, domain string, cr *certmanv1alpha1.CertificateRequest, dnsZone string) (fqdn string, err error) {
	fqdn = c.AnswerDNSChallengeFQDN

	if c.AnswerDNSChallengeErrorString != "" {
		err = errors.New(c.AnswerDNSChallengeErrorString)
	}

	return
}

func (c *MockClient) ValidateDNSWriteAccess(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) (b bool, err error) {
	b = c.ValidateDNSWriteAccessBool

	if c.ValidateDNSWriteAccessErrorString != "" {
		err = errors.New(c.ValidateDNSWriteAccessErrorString)
	}

	return
}

func (c *MockClient) DeleteAcmeChallengeResourceRecords(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) (err error) {
	if c.DeleteAcmeChallengeResourceRecordsErrorString != "" {
		err = errors.New(c.DeleteAcmeChallengeResourceRecordsErrorString)
	}

	return
}
