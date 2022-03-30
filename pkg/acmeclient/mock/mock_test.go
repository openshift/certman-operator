package mock

import (
	"crypto/x509"
	"reflect"
	"testing"

	"github.com/eggsampler/acme"
)

func TestFakeAcmeClient(t *testing.T) {
	tests := []struct {
		Name           string
		Options        *FakeAcmeClientOptions
		ExpectedClient *FakeAcmeClient
	}{
		{
			Name: "sets options",
			Options: &FakeAcmeClientOptions{
				Available:                true,
				FetchAuthorizationCalled: true,
				FetchCertificatesCalled:  true,
				FinalizeOrderCalled:      true,
				NewOrderCalled:           true,
				RevokeCertificateCalled:  true,
				UpdateAccountCalled:      true,
				UpdateChallengeCalled:    true,
			},
			ExpectedClient: &FakeAcmeClient{
				Available:                true,
				FetchAuthorizationCalled: true,
				FetchCertificatesCalled:  true,
				FinalizeOrderCalled:      true,
				NewOrderCalled:           true,
				RevokeCertificateCalled:  true,
				UpdateAccountCalled:      true,
				UpdateChallengeCalled:    true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			actualClient := NewFakeAcmeClient(test.Options)

			if !reflect.DeepEqual(actualClient, test.ExpectedClient) {
				t.Errorf("NewFakeAcmeClient() %s: expected %v, got %v\n", test.Name, test.ExpectedClient, actualClient)
			}
		})
	}
}

func TestUpdateAccount(t *testing.T) {
	tests := []struct {
		Name                   string
		Options                *FakeAcmeClientOptions
		ExpectedFunctionCalled bool
		ExpectError            bool
		ExpectedErrorString    string
	}{
		{
			Name: "let's encrypt available",
			Options: &FakeAcmeClientOptions{
				Available:           true,
				UpdateAccountCalled: false,
			},
			ExpectedFunctionCalled: true,
			ExpectError:            false,
		},
		{
			Name: "let's encrypt unavailable",
			Options: &FakeAcmeClientOptions{
				Available:           false,
				UpdateAccountCalled: false,
			},
			ExpectedFunctionCalled: true,
			ExpectError:            true,
			ExpectedErrorString:    "acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			mockAcmeClient := NewFakeAcmeClient(test.Options)

			// there's no account to update, so that return value is dropped
			_, err := mockAcmeClient.UpdateAccount(acme.Account{}, true, "email@domain.tld")
			if err != nil {
				if !test.ExpectError {
					t.Errorf("UpdateAccount() %s: got unexpected error \"%s\"\n", test.Name, err)
				}
				if err.Error() != test.ExpectedErrorString {
					t.Errorf("UpdateAccount() %s: expected error \"%s\", got error \"%s\"\n", test.Name, test.ExpectedErrorString, err)
				}
			}
			if err == nil && test.ExpectError {
				t.Errorf("UpdateAccount() %s: expected error \"%s\" but didn't get one\n", test.Name, test.ExpectedErrorString)
			}

			if mockAcmeClient.UpdateAccountCalled != test.ExpectedFunctionCalled {
				t.Errorf("UpdateAccount() %s: ExpectedFunctionCalled: %t, got %t\n", test.Name, test.ExpectedFunctionCalled, mockAcmeClient.UpdateAccountCalled)
			}
		})
	}
}

func TestFetchAuthorization(t *testing.T) {
	tests := []struct {
		Name                   string
		Options                *FakeAcmeClientOptions
		ExpectedFunctionCalled bool
		AuthorizationURL       string
		ExpectedAuthorization  acme.Authorization
		ExpectError            bool
		ExpectedErrorString    string
	}{
		{
			Name: "let's encrypt available",
			Options: &FakeAcmeClientOptions{
				Available:                true,
				FetchAuthorizationCalled: false,
			},
			AuthorizationURL: "proto://arbitrary.url",
			ExpectedAuthorization: acme.Authorization{
				URL: "proto://arbitrary.url",
			},
			ExpectedFunctionCalled: true,
			ExpectError:            false,
		},
		{
			Name: "let's encrypt unavailable",
			Options: &FakeAcmeClientOptions{
				Available:                false,
				FetchAuthorizationCalled: false,
			},
			AuthorizationURL:       "proto://arbitrary.url",
			ExpectedFunctionCalled: true,
			ExpectError:            true,
			ExpectedErrorString:    "acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			mockAcmeClient := NewFakeAcmeClient(test.Options)

			// there's no account to update, so that return value is dropped
			authorization, err := mockAcmeClient.FetchAuthorization(acme.Account{}, test.AuthorizationURL)
			if err != nil {
				if !test.ExpectError {
					t.Errorf("FetchAuthorization() %s: got unexpected error \"%s\"\n", test.Name, err)
				}
				if err.Error() != test.ExpectedErrorString {
					t.Errorf("FetchAuthorization() %s: expected error \"%s\", got error \"%s\"\n", test.Name, test.ExpectedErrorString, err)
				}
			}
			if err == nil && test.ExpectError {
				t.Errorf("FetchAuthorization() %s: expected error \"%s\" but didn't get one\n", test.Name, test.ExpectedErrorString)
			}

			if mockAcmeClient.FetchAuthorizationCalled != test.ExpectedFunctionCalled {
				t.Errorf("FetchAuthorization() %s: ExpectedFunctionCalled: %t, got %t\n", test.Name, test.ExpectedFunctionCalled, mockAcmeClient.UpdateAccountCalled)
			}

			if !reflect.DeepEqual(authorization, test.ExpectedAuthorization) {
				t.Errorf("FetchAuthorization() %s: authorization: expected %v, got %v\n", test.Name, test.ExpectedAuthorization, authorization)
			}
		})
	}
}

func TestFetchCertificates(t *testing.T) {
	tests := []struct {
		Name                   string
		Options                *FakeAcmeClientOptions
		ExpectedFunctionCalled bool
		ExpectError            bool
		ExpectedErrorString    string
	}{
		{
			Name: "let's encrypt available",
			Options: &FakeAcmeClientOptions{
				Available:               true,
				FetchCertificatesCalled: false,
			},
			ExpectedFunctionCalled: true,
			ExpectError:            false,
		},
		{
			Name: "let's encrypt unavailable",
			Options: &FakeAcmeClientOptions{
				Available:               false,
				FetchCertificatesCalled: false,
			},
			ExpectedFunctionCalled: true,
			ExpectError:            true,
			ExpectedErrorString:    "acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			mockAcmeClient := NewFakeAcmeClient(test.Options)

			// returning certificates isn't yet implemented, so that value is dropped
			_, err := mockAcmeClient.FetchCertificates(acme.Account{}, "order.certificate")
			if err != nil {
				if !test.ExpectError {
					t.Errorf("FetchCertificates() %s: got unexpected error \"%s\"\n", test.Name, err)
				}
				if err.Error() != test.ExpectedErrorString {
					t.Errorf("FetchCertificates() %s: expected error \"%s\", got error \"%s\"\n", test.Name, test.ExpectedErrorString, err)
				}
			}
			if err == nil && test.ExpectError {
				t.Errorf("FetchCertificates() %s: expected error \"%s\" but didn't get one\n", test.Name, test.ExpectedErrorString)
			}

			if mockAcmeClient.FetchCertificatesCalled != test.ExpectedFunctionCalled {
				t.Errorf("FetchCertificates() %s: ExpectedFunctionCalled: %t, got %t\n", test.Name, test.ExpectedFunctionCalled, mockAcmeClient.FetchCertificatesCalled)
			}
		})
	}
}

func TestFinalizeOrder(t *testing.T) {
	tests := []struct {
		Name                   string
		Options                *FakeAcmeClientOptions
		ExpectedFunctionCalled bool
		ExpectError            bool
		ExpectedErrorString    string
	}{
		{
			Name: "let's encrypt available",
			Options: &FakeAcmeClientOptions{
				Available:           true,
				FinalizeOrderCalled: false,
			},
			ExpectedFunctionCalled: true,
			ExpectError:            false,
		},
		{
			Name: "let's encrypt unavailable",
			Options: &FakeAcmeClientOptions{
				Available:           false,
				FinalizeOrderCalled: false,
			},
			ExpectedFunctionCalled: true,
			ExpectError:            true,
			ExpectedErrorString:    "acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			mockAcmeClient := NewFakeAcmeClient(test.Options)

			// mocking order finalization isn't implemented, so that value is dropped
			_, err := mockAcmeClient.FinalizeOrder(acme.Account{}, acme.Order{}, &x509.CertificateRequest{})
			if err != nil {
				if !test.ExpectError {
					t.Errorf("FinalizeOrder() %s: got unexpected error \"%s\"\n", test.Name, err)
				}
				if err.Error() != test.ExpectedErrorString {
					t.Errorf("FinalizeOrder() %s: expected error \"%s\", got error \"%s\"\n", test.Name, test.ExpectedErrorString, err)
				}
			}
			if err == nil && test.ExpectError {
				t.Errorf("FinalizeOrder() %s: expected error \"%s\" but didn't get one\n", test.Name, test.ExpectedErrorString)
			}

			if mockAcmeClient.FinalizeOrderCalled != test.ExpectedFunctionCalled {
				t.Errorf("FinalizeOrder() %s: ExpectedFunctionCalled: %t, got %t\n", test.Name, test.ExpectedFunctionCalled, mockAcmeClient.FinalizeOrderCalled)
			}
		})
	}
}

func TestNewOrder(t *testing.T) {
	tests := []struct {
		Name                   string
		Options                *FakeAcmeClientOptions
		ExpectedFunctionCalled bool
		ExpectError            bool
		ExpectedErrorString    string
	}{
		{
			Name: "let's encrypt available",
			Options: &FakeAcmeClientOptions{
				Available:      true,
				NewOrderCalled: false,
			},
			ExpectedFunctionCalled: true,
			ExpectError:            false,
		},
		{
			Name: "let's encrypt unavailable",
			Options: &FakeAcmeClientOptions{
				Available:      false,
				NewOrderCalled: false,
			},
			ExpectedFunctionCalled: true,
			ExpectError:            true,
			ExpectedErrorString:    "acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			mockAcmeClient := NewFakeAcmeClient(test.Options)

			// mocking new orders isn't implemented, so that value is dropped
			_, err := mockAcmeClient.NewOrder(acme.Account{}, []acme.Identifier{})
			if err != nil {
				if !test.ExpectError {
					t.Errorf("NewOrder() %s: got unexpected error \"%s\"\n", test.Name, err)
				}
				if err.Error() != test.ExpectedErrorString {
					t.Errorf("NewOrder() %s: expected error \"%s\", got error \"%s\"\n", test.Name, test.ExpectedErrorString, err)
				}
			}
			if err == nil && test.ExpectError {
				t.Errorf("NewOrder() %s: expected error \"%s\" but didn't get one\n", test.Name, test.ExpectedErrorString)
			}

			if mockAcmeClient.NewOrderCalled != test.ExpectedFunctionCalled {
				t.Errorf("NewOrder() %s: ExpectedFunctionCalled: %t, got %t\n", test.Name, test.ExpectedFunctionCalled, mockAcmeClient.NewOrderCalled)
			}
		})
	}
}

func TestRevokeCertificate(t *testing.T) {
	tests := []struct {
		Name                   string
		Options                *FakeAcmeClientOptions
		ExpectedFunctionCalled bool
		ExpectError            bool
		ExpectedErrorString    string
	}{
		{
			Name: "let's encrypt available",
			Options: &FakeAcmeClientOptions{
				Available:               true,
				RevokeCertificateCalled: false,
			},
			ExpectedFunctionCalled: true,
			ExpectError:            false,
		},
		{
			Name: "let's encrypt unavailable",
			Options: &FakeAcmeClientOptions{
				Available:               false,
				RevokeCertificateCalled: false,
			},
			ExpectedFunctionCalled: true,
			ExpectError:            true,
			ExpectedErrorString:    "acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			mockAcmeClient := NewFakeAcmeClient(test.Options)

			// revoking certificates isn't implemented
			err := mockAcmeClient.RevokeCertificate(acme.Account{}, &x509.Certificate{}, acme.Account{}.PrivateKey, 0)
			if err != nil {
				if !test.ExpectError {
					t.Errorf("RevokeCertificate() %s: got unexpected error \"%s\"\n", test.Name, err)
				}
				if err.Error() != test.ExpectedErrorString {
					t.Errorf("RevokeCertificate() %s: expected error \"%s\", got error \"%s\"\n", test.Name, test.ExpectedErrorString, err)
				}
			}
			if err == nil && test.ExpectError {
				t.Errorf("RevokeCertificate() %s: expected error \"%s\" but didn't get one\n", test.Name, test.ExpectedErrorString)
			}

			if mockAcmeClient.RevokeCertificateCalled != test.ExpectedFunctionCalled {
				t.Errorf("RevokeCertificate() %s: ExpectedFunctionCalled: %t, got %t\n", test.Name, test.ExpectedFunctionCalled, mockAcmeClient.RevokeCertificateCalled)
			}
		})
	}
}

func TestUpdateChallenge(t *testing.T) {
	tests := []struct {
		Name                   string
		Options                *FakeAcmeClientOptions
		Challenge              acme.Challenge
		ExpectedChallenge      acme.Challenge
		ExpectedFunctionCalled bool
		ExpectError            bool
		ExpectedErrorString    string
	}{
		{
			Name: "let's encrypt available",
			Options: &FakeAcmeClientOptions{
				Available:             true,
				UpdateChallengeCalled: false,
			},
			Challenge: acme.Challenge{
				Type: "mock test",
			},
			ExpectedChallenge: acme.Challenge{
				Type: "mock test",
			},
			ExpectedFunctionCalled: true,
			ExpectError:            false,
		},
		{
			Name: "let's encrypt unavailable",
			Options: &FakeAcmeClientOptions{
				Available:             false,
				UpdateChallengeCalled: false,
			},
			Challenge: acme.Challenge{
				Type: "mock test",
			},
			ExpectedChallenge: acme.Challenge{
				Type: "mock test",
			},
			ExpectedFunctionCalled: true,
			ExpectError:            true,
			ExpectedErrorString:    "acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			mockAcmeClient := NewFakeAcmeClient(test.Options)

			actualChallenge, err := mockAcmeClient.UpdateChallenge(acme.Account{}, test.Challenge)
			if err != nil {
				if !test.ExpectError {
					t.Errorf("UpdateChallenge() %s: got unexpected error \"%s\"\n", test.Name, err)
				}
				if err.Error() != test.ExpectedErrorString {
					t.Errorf("UpdateChallenge() %s: expected error \"%s\", got error \"%s\"\n", test.Name, test.ExpectedErrorString, err)
				}
			}
			if err == nil && test.ExpectError {
				t.Errorf("UpdateChallenge() %s: expected error \"%s\" but didn't get one\n", test.Name, test.ExpectedErrorString)
			}

			if mockAcmeClient.UpdateChallengeCalled != test.ExpectedFunctionCalled {
				t.Errorf("UpdateChallenge() %s: ExpectedFunctionCalled: %t, got %t\n", test.Name, test.ExpectedFunctionCalled, mockAcmeClient.UpdateChallengeCalled)
			}

			if !reflect.DeepEqual(actualChallenge, test.ExpectedChallenge) {
				t.Errorf("UpdateChallenge() %s: Expected: %v, got %v\n", test.Name, test.ExpectedChallenge, actualChallenge)
			}
		})
	}
}
