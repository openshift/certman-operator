package mock

import (
	"errors"
	"reflect"
	"testing"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
)

func TestNewMockClient(t *testing.T) {
	tests := []struct {
		Name           string
		Options        *MockClientOptions
		ExpectedClient *MockClient
	}{
		{
			Name: "returns a mock client",
			Options: &MockClientOptions{
				AnswerDNSChallengeFQDN:                  "an.arbitrary.url",
				AnswerDNSChallengeError:                 errors.New("testing challenge error"),
				ValidateDNSWriteAccessBool:              true,
				ValidateDNSWriteAccessError:             errors.New("testing validation error"),
				DeleteAcmeChallengeResourceRecordsError: errors.New("testing deletion error"),
			},
			ExpectedClient: &MockClient{
				AnswerDNSChallengeFQDN:                  "an.arbitrary.url",
				AnswerDNSChallengeError:                 errors.New("testing challenge error"),
				ValidateDNSWriteAccessBool:              true,
				ValidateDNSWriteAccessError:             errors.New("testing validation error"),
				DeleteAcmeChallengeResourceRecordsError: errors.New("testing deletion error"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			actualClient := NewMockClient(test.Options)

			if !reflect.DeepEqual(actualClient, test.ExpectedClient) {
				t.Errorf("NewMockClient() %s: expected %v, got %v\n", test.Name, test.ExpectedClient, actualClient)
			}
		})
	}
}

func TestGetDNSName(t *testing.T) {
	testClient := NewMockClient(&MockClientOptions{})
	actualDNSName := testClient.GetDNSName()

	if actualDNSName != "Mock" {
		t.Errorf("GetDNSName(): expected \"Mock\", got %s\n", actualDNSName)
	}
}

func TestAnswerDNSChallenge(t *testing.T) {
	tests := []struct {
		Name                            string
		TestClient                      *MockClient
		ExpectedAnswerDNSChallengeFQDN  string
		ExpectedAnswerDNSChallengeError error
	}{
		{
			Name: "mocks success",
			TestClient: NewMockClient(&MockClientOptions{
				AnswerDNSChallengeFQDN:  "extremely.real.website",
				AnswerDNSChallengeError: nil,
			}),
			ExpectedAnswerDNSChallengeFQDN:  "extremely.real.website",
			ExpectedAnswerDNSChallengeError: nil,
		},
		{
			Name: "mocks error",
			TestClient: NewMockClient(&MockClientOptions{
				AnswerDNSChallengeFQDN:  "",
				AnswerDNSChallengeError: errors.New("mock challenge error"),
			}),
			ExpectedAnswerDNSChallengeFQDN:  "",
			ExpectedAnswerDNSChallengeError: errors.New("mock challenge error"),
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			actualFQDN, err := test.TestClient.AnswerDNSChallenge(logf.NullLogger{}, "ignored", "irrelevant", &certmanv1alpha1.CertificateRequest{})
			if err != nil && err.Error() != test.ExpectedAnswerDNSChallengeError.Error() {
				t.Errorf("AnswerDNSChallenge() %s: expected error \"%s\", got error \"%s\"\n", test.Name, test.ExpectedAnswerDNSChallengeError.Error(), err.Error())
			}

			if actualFQDN != test.ExpectedAnswerDNSChallengeFQDN {
				t.Errorf("AnswerDNSChallenge() %s: expected %s, got %s\n", test.Name, test.ExpectedAnswerDNSChallengeFQDN, actualFQDN)
			}
		})
	}
}

func TestDeleteAcmeChallengeResourceRecords(t *testing.T) {
	tests := []struct {
		Name                                            string
		TestClient                                      *MockClient
		ExpectedDeleteAcmeChallengeResourceRecordsError error
	}{
		{
			Name: "mocks success",
			TestClient: NewMockClient(&MockClientOptions{
				DeleteAcmeChallengeResourceRecordsError: nil,
			}),
			ExpectedDeleteAcmeChallengeResourceRecordsError: nil,
		},
		{
			Name: "mocks error",
			TestClient: NewMockClient(&MockClientOptions{
				DeleteAcmeChallengeResourceRecordsError: errors.New("mock deletion error"),
			}),
			ExpectedDeleteAcmeChallengeResourceRecordsError: errors.New("mock deletion error"),
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			err := test.TestClient.DeleteAcmeChallengeResourceRecords(logf.NullLogger{}, &certmanv1alpha1.CertificateRequest{})
			if err != nil && err.Error() != test.ExpectedDeleteAcmeChallengeResourceRecordsError.Error() {
				t.Errorf("DeleteAcmeChallengeResourceRecords() %s: expected error \"%s\", got error \"%s\"\n", test.Name, test.ExpectedDeleteAcmeChallengeResourceRecordsError.Error(), err.Error())
			}
		})
	}
}

func TestValidateDNSWriteAccess(t *testing.T) {
	tests := []struct {
		Name                                string
		TestClient                          *MockClient
		ExpectedValidateDNSWriteAccessBool  bool
		ExpectedValidateDNSWriteAccessError error
	}{
		{
			Name: "mocks success",
			TestClient: NewMockClient(&MockClientOptions{
				ValidateDNSWriteAccessBool:  true,
				ValidateDNSWriteAccessError: nil,
			}),
			ExpectedValidateDNSWriteAccessBool:  true,
			ExpectedValidateDNSWriteAccessError: nil,
		},
		{
			Name: "mocks error",
			TestClient: NewMockClient(&MockClientOptions{
				ValidateDNSWriteAccessBool:  false,
				ValidateDNSWriteAccessError: errors.New("mock validation error"),
			}),
			ExpectedValidateDNSWriteAccessBool:  false,
			ExpectedValidateDNSWriteAccessError: errors.New("mock validation error"),
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			actualBool, err := test.TestClient.ValidateDNSWriteAccess(logf.NullLogger{}, &certmanv1alpha1.CertificateRequest{})
			if err != nil && err.Error() != test.ExpectedValidateDNSWriteAccessError.Error() {
				t.Errorf("ValidateDNSWriteAccess() %s: expected error \"%s\", got error \"%s\"\n", test.Name, test.ExpectedValidateDNSWriteAccessError.Error(), err.Error())
			}

			if actualBool != test.ExpectedValidateDNSWriteAccessBool {
				t.Errorf("ValidateDNSWriteAccess() %s: expected %t, got %t\n", test.Name, test.ExpectedValidateDNSWriteAccessBool, actualBool)
			}
		})
	}
}
