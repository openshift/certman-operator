package mock

import (
	"reflect"
	"testing"

	"github.com/go-logr/logr"

	certmanv1alpha1 "github.com/openshift/certman-operator/api/v1alpha1"
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
				AnswerDNSChallengeFQDN:                        "an.arbitrary.url",
				AnswerDNSChallengeErrorString:                 "testing challenge error",
				ValidateDNSWriteAccessBool:                    true,
				ValidateDNSWriteAccessErrorString:             "testing validation error",
				DeleteAcmeChallengeResourceRecordsErrorString: "testing deletion error",
			},
			ExpectedClient: &MockClient{
				AnswerDNSChallengeFQDN:                        "an.arbitrary.url",
				AnswerDNSChallengeErrorString:                 "testing challenge error",
				ValidateDNSWriteAccessBool:                    true,
				ValidateDNSWriteAccessErrorString:             "testing validation error",
				DeleteAcmeChallengeResourceRecordsErrorString: "testing deletion error",
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
		Name                                  string
		TestClient                            *MockClient
		ExpectedAnswerDNSChallengeFQDN        string
		ExpectedAnswerDNSChallengeErrorString string
	}{
		{
			Name: "mocks success",
			TestClient: NewMockClient(&MockClientOptions{
				AnswerDNSChallengeFQDN:        "extremely.real.website",
				AnswerDNSChallengeErrorString: "",
			}),
			ExpectedAnswerDNSChallengeFQDN:        "extremely.real.website",
			ExpectedAnswerDNSChallengeErrorString: "",
		},
		{
			Name: "mocks error",
			TestClient: NewMockClient(&MockClientOptions{
				AnswerDNSChallengeFQDN:        "",
				AnswerDNSChallengeErrorString: "mock challenge error",
			}),
			ExpectedAnswerDNSChallengeFQDN:        "",
			ExpectedAnswerDNSChallengeErrorString: "mock challenge error",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			actualFQDN, err := test.TestClient.AnswerDNSChallenge(logr.Discard(), "ignored", "irrelevant", &certmanv1alpha1.CertificateRequest{})
			if err != nil && err.Error() != test.ExpectedAnswerDNSChallengeErrorString {
				t.Errorf("AnswerDNSChallenge() %s: expected error \"%s\", got error \"%s\"\n", test.Name, test.ExpectedAnswerDNSChallengeErrorString, err.Error())
			}

			if actualFQDN != test.ExpectedAnswerDNSChallengeFQDN {
				t.Errorf("AnswerDNSChallenge() %s: expected %s, got %s\n", test.Name, test.ExpectedAnswerDNSChallengeFQDN, actualFQDN)
			}
		})
	}
}

func TestDeleteAcmeChallengeResourceRecords(t *testing.T) {
	tests := []struct {
		Name                                                  string
		TestClient                                            *MockClient
		ExpectedDeleteAcmeChallengeResourceRecordsErrorString string
	}{
		{
			Name: "mocks success",
			TestClient: NewMockClient(&MockClientOptions{
				DeleteAcmeChallengeResourceRecordsErrorString: "",
			}),
			ExpectedDeleteAcmeChallengeResourceRecordsErrorString: "",
		},
		{
			Name: "mocks error",
			TestClient: NewMockClient(&MockClientOptions{
				DeleteAcmeChallengeResourceRecordsErrorString: "mock deletion error",
			}),
			ExpectedDeleteAcmeChallengeResourceRecordsErrorString: "mock deletion error",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			err := test.TestClient.DeleteAcmeChallengeResourceRecords(logr.Discard(), &certmanv1alpha1.CertificateRequest{})
			if err != nil && err.Error() != test.ExpectedDeleteAcmeChallengeResourceRecordsErrorString {
				t.Errorf("DeleteAcmeChallengeResourceRecords() %s: expected error \"%s\", got error \"%s\"\n", test.Name, test.ExpectedDeleteAcmeChallengeResourceRecordsErrorString, err.Error())
			}
		})
	}
}

func TestValidateDNSWriteAccess(t *testing.T) {
	tests := []struct {
		Name                                      string
		TestClient                                *MockClient
		ExpectedValidateDNSWriteAccessBool        bool
		ExpectedValidateDNSWriteAccessErrorString string
	}{
		{
			Name: "mocks success",
			TestClient: NewMockClient(&MockClientOptions{
				ValidateDNSWriteAccessBool:        true,
				ValidateDNSWriteAccessErrorString: "",
			}),
			ExpectedValidateDNSWriteAccessBool:        true,
			ExpectedValidateDNSWriteAccessErrorString: "",
		},
		{
			Name: "mocks error",
			TestClient: NewMockClient(&MockClientOptions{
				ValidateDNSWriteAccessBool:        false,
				ValidateDNSWriteAccessErrorString: "mock validation error",
			}),
			ExpectedValidateDNSWriteAccessBool:        false,
			ExpectedValidateDNSWriteAccessErrorString: "mock validation error",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			actualBool, err := test.TestClient.ValidateDNSWriteAccess(logr.Discard(), &certmanv1alpha1.CertificateRequest{})
			if err != nil && err.Error() != test.ExpectedValidateDNSWriteAccessErrorString {
				t.Errorf("ValidateDNSWriteAccess() %s: expected error \"%s\", got error \"%s\"\n", test.Name, test.ExpectedValidateDNSWriteAccessErrorString, err.Error())
			}

			if actualBool != test.ExpectedValidateDNSWriteAccessBool {
				t.Errorf("ValidateDNSWriteAccess() %s: expected %t, got %t\n", test.Name, test.ExpectedValidateDNSWriteAccessBool, actualBool)
			}
		})
	}
}
