/*
Copyright 2020 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package leclient

import (
	"crypto/x509"
	"reflect"
	"testing"

	"github.com/eggsampler/acme"
	"github.com/openshift/certman-operator/config"
	acmemock "github.com/openshift/certman-operator/pkg/acmeclient/mock"
	v1 "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

/*
sigs.k8s.io/controller-runtime/pkg/client/fake is supposed to be deprecated but as of
1 Apr 2020, using it is still the recommended technique for unit testing, according to
https://github.com/operator-framework/operator-sdk/blob/master/doc/user/unit-testing.md
*/

func TestNewClient(t *testing.T) {
	t.Run("returns an error", func(t *testing.T) {
		t.Run("if no account secret is found", func(t *testing.T) {
			testClient := setUpEmptyTestClient(t)

			_, actual := NewClient(testClient)

			if actual == nil {
				t.Errorf("expected an error when attempting to get missing account secrets")
			}
		})

		t.Run("if only deprecated staging secret is set", func(t *testing.T) {
			testClient := setUpTestClient(t, letsEncryptStagingAccountSecretName)

			_, err := NewClient(testClient)

			if !kerr.IsNotFound(err) {
				t.Error("expected error when using deprecated secret name")
			}
		})

		t.Run("if only deprecated production secret is set", func(t *testing.T) {
			testClient := setUpTestClient(t, letsEncryptProductionAccountSecretName)

			_, err := NewClient(testClient)

			if !kerr.IsNotFound(err) {
				t.Error("expected error when using deprecated secret name")
			}
		})
	})

	t.Run("returns an leclient", func(t *testing.T) {
		t.Run("if only approved secret is set", func(t *testing.T) {
			testClient := setUpTestClient(t, letsEncryptAccountSecretName)

			leclient, err := NewClient(testClient)
			if err != nil {
				t.Fatalf("unexpected error creating the leclient: %q", err)
			}

			if leclient == nil {
				t.Errorf("leclient failed to set up")
			}
		})
	})
}

func TestUpdateAccount(t *testing.T) {
	tests := []struct {
		Name                string
		ACME                *acmemock.FakeAcmeClient
		Email               string
		ExpectedContacts    []string
		ExpectError         bool
		ExpectedErrorString string
	}{
		{
			Name: "UpdateAccount when Let's Encrypt is up",
			ACME: &acmemock.FakeAcmeClient{
				Available: true,
			},
			Email:               "doesn't@ma.tter",
			ExpectedContacts:    []string{"mailto:doesn't@ma.tter"},
			ExpectError:         false,
			ExpectedErrorString: "",
		},
		{
			Name: "update when Let's Encrypt is down",
			ACME: &acmemock.FakeAcmeClient{
				Available: false,
			},
			Email:               "doesn't@ma.tter",
			ExpectError:         true,
			ExpectedErrorString: "acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			testLEClient := &LetsEncryptClient{
				Client: test.ACME,
			}
			err := testLEClient.UpdateAccount(test.Email)

			if err != nil {
				if !test.ExpectError {
					t.Errorf("UpdateAccount() %s: got unexpected error: %s\n", test.Name, err)
				}
				if err.Error() != test.ExpectedErrorString {
					t.Errorf("UpdateAccount() %s: expected error \"%s\", got error \"%s\"\n", test.Name, test.ExpectedErrorString, err.Error())
				}
			} else {
				if test.ExpectError {
					t.Errorf("UpdateAccount() %s: expected error \"%s\" but didn't get one\n", test.Name, test.ExpectedErrorString)
				} else if !test.ACME.UpdateAccountCalled {
					t.Errorf("UpdateAccount() %s: expected the acme client UpdateAccount() to be called but it wasn't\n", test.Name)
				}

				if !reflect.DeepEqual(test.ACME.Contacts, test.ExpectedContacts) {
					t.Errorf("UpdateAccount() %s: expected contacts: %v, got %v\n", test.Name, test.ExpectedContacts, test.ACME.Contacts)
				}
			}
		})
	}
}

func TestCreateOrder(t *testing.T) {
	tests := []struct {
		Name                string
		ACME                *acmemock.FakeAcmeClient
		Domains             []string
		ExpectedIds         []acme.Identifier
		ExpectError         bool
		ExpectedErrorString string
	}{
		{
			Name: "create order when Let's Encrypt is up",
			ACME: &acmemock.FakeAcmeClient{
				Available: true,
			},
			Domains: []string{"domain.one.tld", "other.two.tld"},
			ExpectedIds: []acme.Identifier{
				{
					Type:  "dns",
					Value: "domain.one.tld",
				},
				{
					Type:  "dns",
					Value: "other.two.tld",
				},
			},
			ExpectError: false,
		},
		{
			Name: "create order when Let's Encrypt is down",
			ACME: &acmemock.FakeAcmeClient{
				Available: false,
			},
			Domains:     []string{"domain.one.tld", "other.two.tld"},
			ExpectError: true,
			ExpectedIds: []acme.Identifier{
				{
					Type:  "dns",
					Value: "domain.one.tld",
				},
				{
					Type:  "dns",
					Value: "other.two.tld",
				},
			},
			ExpectedErrorString: "acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			testLEClient := &LetsEncryptClient{
				Client: test.ACME,
			}
			err := testLEClient.CreateOrder(test.Domains)
			if err != nil {
				if !test.ExpectError {
					t.Errorf("CreateOrder() %s: got unexpected error: %s\n", test.Name, err)
				}
				if err.Error() != test.ExpectedErrorString {
					t.Errorf("CreateOrder() %s: expected error \"%s\", got error \"%s\"\n", test.Name, test.ExpectedErrorString, err.Error())
				}
			} else {
				if test.ExpectError {
					t.Errorf("CreateOrder() %s: expected error \"%s\" but didn't get one\n", test.Name, test.ExpectedErrorString)
				}
				if !test.ACME.NewOrderCalled {
					t.Errorf("CreateOrder() %s: expected the acme client NewOrder() to be called but it wasn't\n", test.Name)
				}
			}

			if !reflect.DeepEqual(test.ACME.Identifiers, test.ExpectedIds) {
				t.Errorf("CreateOrder() %s: expected identifiers: %v, got %v\n", test.Name, test.ExpectedIds, test.ACME.Identifiers)
			}
		})
	}
}

func TestGetOrderURL(t *testing.T) {
	tests := []struct {
		Name        string
		URL         string
		ExpectedURL string
	}{
		{
			Name:        "get order URL",
			URL:         "https://i.dont.even.know/whatshouldgohere",
			ExpectedURL: "https://i.dont.even.know/whatshouldgohere",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			testLEClient := &LetsEncryptClient{
				Order: acme.Order{
					URL: test.URL,
				},
			}

			actualURL := testLEClient.GetOrderURL()

			if actualURL != test.ExpectedURL {
				t.Errorf("GetOrderURL() %s: expected %s, got %s\n", test.Name, test.ExpectedURL, actualURL)
			}
		})
	}
}

func TestOrderAuthorization(t *testing.T) {
	tests := []struct {
		Name                   string
		Authorizations         []string
		ExpectedAuthorizations []string
	}{
		{
			Name:                   "get order authorizations",
			Authorizations:         []string{"something", "else"},
			ExpectedAuthorizations: []string{"something", "else"},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			testLEClient := &LetsEncryptClient{
				Order: acme.Order{
					Authorizations: test.Authorizations,
				},
			}

			actualAuths := testLEClient.OrderAuthorization()

			if !reflect.DeepEqual(actualAuths, test.ExpectedAuthorizations) {
				t.Errorf("GetOrderURL() %s: expected %s, got %s\n", test.Name, test.ExpectedAuthorizations, actualAuths)
			}
		})
	}
}

func TestFetchAuthorization(t *testing.T) {
	tests := []struct {
		Name                     string
		ACME                     *acmemock.FakeAcmeClient
		AuthURL                  string
		ExpectedAuthorizationURL string
		ExpectError              bool
		ExpectedErrorString      string
	}{
		{
			Name: "get order authorizations when Let's Encrypt is up",
			ACME: &acmemock.FakeAcmeClient{
				Available: true,
				FetchAuthorizationResult: acme.Authorization{
					URL: "https://i.dont.even.know/whatshouldgohere",
				},
			},
			ExpectedAuthorizationURL: "https://i.dont.even.know/whatshouldgohere",
			ExpectError:              false,
		},
		{
			Name: "get order authorizations when Let's Encrypt is down",
			ACME: &acmemock.FakeAcmeClient{
				Available: false,
			},
			ExpectedAuthorizationURL: "https://i.dont.even.know/whatshouldgohere",
			ExpectError:              true,
			ExpectedErrorString:      "acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			testLEClient := &LetsEncryptClient{
				Client: test.ACME,
			}

			err := testLEClient.FetchAuthorization(test.AuthURL)
			if err != nil {
				if !test.ExpectError {
					t.Errorf("FetchAuthorization() %s: got unexpected error: \"%s\"\n", test.Name, err)
				}
				if err.Error() != test.ExpectedErrorString {
					t.Errorf("FetchAuthorization() %s: got unexpected error: \"%s\" but expected error \"%s\"\n", test.Name, err, test.ExpectedErrorString)
				}
			} else {
				if test.ExpectError {
					t.Errorf("FetchAuthorization() %s: expected error \"%s\" but didn't get one\n", test.Name, test.ExpectedErrorString)
				}
				if testLEClient.Authorization.URL != test.ExpectedAuthorizationURL {
					t.Errorf("FetchAuthorization() %s: expected %v, got %v\n", test.Name, test.ExpectedAuthorizationURL, testLEClient.Authorization.URL)
				}
			}

			if !test.ACME.FetchAuthorizationCalled {
				t.Errorf("FetchAuthorization() %s: expected the acme client FetchAuthorization() to be called but it wasn't\n", test.Name)
			}
		})
	}
}

func TestGetAuthorizationURL(t *testing.T) {
	tests := []struct {
		Name                     string
		AuthURL                  string
		ExpectedAuthorizationURL string
	}{
		{
			Name:                     "get order authorization url",
			AuthURL:                  "https://i.dont.even.know/whatshouldgohere",
			ExpectedAuthorizationURL: "https://i.dont.even.know/whatshouldgohere",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			testLEClient := &LetsEncryptClient{
				Authorization: acme.Authorization{
					URL: test.AuthURL,
				},
			}

			actualURL := testLEClient.GetAuthorizationURL()

			if actualURL != test.ExpectedAuthorizationURL {
				t.Errorf("GetAuthorizationURL() %s: expected %s, got %s\n", test.Name, test.ExpectedAuthorizationURL, actualURL)
			}
		})
	}
}

func TestGetAuthorizationIdentifier(t *testing.T) {
	tests := []struct {
		Name                            string
		AuthIDValue                     string
		ExpectedAuthorizationIdentifier string
		ExpectError                     bool
		ExpectedErrorString             string
	}{
		{
			Name:                            "get order authorizations when Let's Encrypt is up",
			AuthIDValue:                     "i-dont-care-what-this-is-so-long-as-it-matches",
			ExpectedAuthorizationIdentifier: "i-dont-care-what-this-is-so-long-as-it-matches",
			ExpectError:                     false,
		},
		{
			Name:                            "get order authorizations when Let's Encrypt is down",
			AuthIDValue:                     "i-dont-care-what-this-is-so-long-as-it-matches",
			ExpectedAuthorizationIdentifier: "i-dont-care-what-this-is-so-long-as-it-matches",
			ExpectError:                     false,
		},
		{
			Name:                            "empty order authorization returns an error",
			AuthIDValue:                     "",
			ExpectedAuthorizationIdentifier: "",
			ExpectError:                     true,
			ExpectedErrorString:             "Authorization indentifier not currently set",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			testLEClient := &LetsEncryptClient{
				Authorization: acme.Authorization{
					Identifier: acme.Identifier{
						Value: test.AuthIDValue,
					},
				},
			}

			actualAuthID, err := testLEClient.GetAuthorizationIndentifier()
			if err != nil {
				if !test.ExpectError {
					t.Errorf("GetAuthorizationIndentifier() %s: got unexpected error: \"%s\"\n", test.Name, err)
				}
				if err.Error() != test.ExpectedErrorString {
					t.Errorf("GetAuthorizationIndentifier() %s: got error \"%s\", but expected error \"%s\"\n", test.Name, err, test.ExpectedErrorString)
				}
			} else {
				if test.ExpectError {
					t.Errorf("GetAuthorizationIndentifier() %s: expected error: \"%s\" but didn't get it\n", test.Name, test.ExpectedErrorString)
				}
			}

			if actualAuthID != test.ExpectedAuthorizationIdentifier {
				t.Errorf("GetAuthorizationIndentifier() %s: expected %s, got %s\n", test.Name, test.ExpectedAuthorizationIdentifier, actualAuthID)
			}
		})
	}
}

func TestSetChallengeType(t *testing.T) {
	tests := []struct {
		Name                  string
		ChallengeType         acme.Challenge
		ExpectedChallengeType acme.Challenge
	}{
		{
			Name: "get order authorizations",
			ChallengeType: acme.Challenge{
				Type: "test",
			},
			ExpectedChallengeType: acme.Challenge{
				Type: "test",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			testLEClient := &LetsEncryptClient{
				Authorization: acme.Authorization{
					ChallengeMap: map[string]acme.Challenge{"dns-01": test.ChallengeType},
				},
			}

			testLEClient.SetChallengeType()

			if !reflect.DeepEqual(testLEClient.Challenge, test.ExpectedChallengeType) {
				t.Errorf("SetChallengeType() %s: expected %v, got %v\n", test.Name, test.ExpectedChallengeType, testLEClient.Challenge)
			}
		})
	}
}

func TestGetDNS01KeyAuthorization(t *testing.T) {
	tests := []struct {
		Name                string
		KeyAuth             string
		ExpectedKeyAuth     string
		ExpectError         bool
		ExpectedErrorString string
	}{
		{
			Name:            "encode present key authorization",
			KeyAuth:         "anything",
			ExpectedKeyAuth: "7gh0Fwt_bzK4wqyVc8Qo01tXUnCma3V8LAGF0r0JcY0",
			ExpectError:     false,
		},
		// i can't figure out how to make acme.EncodeDNS01KeyAuthorization() return
		// an empty string for the keyauth . if someone can figure it out, they're
		// welcome to uncomment this test case
		//{
		//	Name:                "error encoding empty key authorization",
		//	KeyAuth:             "",
		//	ExpectError:         true,
		//	ExpectedErrorString: "Authorization key not currently set",
		//},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			testLEClient := &LetsEncryptClient{
				Challenge: acme.Challenge{
					KeyAuthorization: test.KeyAuth,
				},
			}

			actualKeyAuth, err := testLEClient.GetDNS01KeyAuthorization()
			if err != nil {
				if !test.ExpectError {
					t.Errorf("GetDNS01KeyAuthorization() %s: got unexpected error: \"%s\"\n", test.Name, err)
				}
				if err.Error() != test.ExpectedErrorString {
					t.Errorf("GetDNS01KeyAuthorization() %s: got error \"%s\", expected error \"%s\"\n", test.Name, err, test.ExpectedErrorString)
				}
			} else {
				if test.ExpectError {
					t.Errorf("GetDNS01KeyAuthorization() %s: expected error \"%s\" but didn't get one\n", test.Name, test.ExpectedErrorString)
				}
			}

			if !reflect.DeepEqual(actualKeyAuth, test.ExpectedKeyAuth) {
				t.Errorf("GetDNS01KeyAuthorization() %s: expected %v, got %v\n", test.Name, test.ExpectedKeyAuth, actualKeyAuth)
			}
		})
	}
}

func TestGetChallengeURL(t *testing.T) {
	tests := []struct {
		Name        string
		URL         string
		ExpectedURL string
	}{
		{
			Name:        "return challenge url from the LE client",
			URL:         "anything",
			ExpectedURL: "anything",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			testLEClient := &LetsEncryptClient{
				Challenge: acme.Challenge{
					URL: test.URL,
				},
			}

			actual := testLEClient.GetChallengeURL()

			if actual != test.ExpectedURL {
				t.Errorf("GetChallengeURL() %s: got %s, expected %s\n", test.Name, actual, test.ExpectedURL)
			}
		})
	}
}

func TestUpdateChallenge(t *testing.T) {
	tests := []struct {
		Name                string
		ACME                *acmemock.FakeAcmeClient
		Challenge           acme.Challenge
		ExpectError         bool
		ExpectedErrorString string
	}{
		{
			Name: "update challenge when let's encrypt is up",
			ACME: &acmemock.FakeAcmeClient{
				Available: true,
			},
			Challenge:   acme.Challenge{},
			ExpectError: false,
		},
		{
			Name: "update challenge when let's encrypt is down",
			ACME: &acmemock.FakeAcmeClient{
				Available: false,
			},
			Challenge:           acme.Challenge{},
			ExpectError:         true,
			ExpectedErrorString: "acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			testLEClient := LetsEncryptClient{
				Client:    test.ACME,
				Challenge: test.Challenge,
			}

			err := testLEClient.UpdateChallenge()
			if err != nil {
				if !test.ExpectError {
					t.Errorf("UpdateChallenge() %s: got unexpected error \"%s\"\n", test.Name, err)
				}
				if err.Error() != test.ExpectedErrorString {
					t.Errorf("UpdateChallenge() %s: got error \"%s\", expected \"%s\"\n", test.Name, err, test.ExpectedErrorString)
				}
			} else {
				if test.ExpectError {
					t.Errorf("UpdateChallenge() %s: expected error \"%s\" but didn't get it\n", test.Name, test.ExpectedErrorString)
				}
			}

			if !test.ACME.UpdateChallengeCalled {
				t.Errorf("UpdateChallenge() %s: expected the acme client UpdateChallenge() to be called but it wasn't\n", test.Name)
			}
		})
	}
}

func TestFinalizeOrder(t *testing.T) {
	tests := []struct {
		Name                string
		ACME                *acmemock.FakeAcmeClient
		CSR                 *x509.CertificateRequest
		ExpectError         bool
		ExpectedErrorString string
	}{
		{
			Name: "finalize order when let's encrypt is up",
			ACME: &acmemock.FakeAcmeClient{
				Available: true,
			},
			CSR:         &x509.CertificateRequest{},
			ExpectError: false,
		},
		{
			Name: "finalize order when let's encrypt is down",
			ACME: &acmemock.FakeAcmeClient{
				Available: false,
			},
			CSR:                 &x509.CertificateRequest{},
			ExpectError:         true,
			ExpectedErrorString: "acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			testLEClient := LetsEncryptClient{
				Client:  test.ACME,
				Account: acme.Account{},
				Order:   acme.Order{},
			}

			err := testLEClient.FinalizeOrder(test.CSR)
			if err != nil {
				if !test.ExpectError {
					t.Errorf("FinalizeOrder() %s: got unexpected error \"%s\"\n", test.Name, err)
				}
				if err.Error() != test.ExpectedErrorString {
					t.Errorf("FinalizeOrder() %s: got error \"%s\", expected \"%s\"\n", test.Name, err, test.ExpectedErrorString)
				}
			} else {
				if test.ExpectError {
					t.Errorf("FinalizeOrder() %s: expected error \"%s\" but didn't get it\n", test.Name, test.ExpectedErrorString)
				}
			}

			if !test.ACME.FinalizeOrderCalled {
				t.Errorf("FinalizeOrder() %s: expected the acme client FinalizeOrder() to be called but it wasn't\n", test.Name)
			}
		})
	}
}

func TestGetOrderEndpoint(t *testing.T) {
	tests := []struct {
		Name                string
		Order               acme.Order
		ExpectedCertificate string
	}{
		{
			Name: "return order endpoint from the LE client",
			Order: acme.Order{
				Certificate: "cert body",
			},
			ExpectedCertificate: "cert body",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			testLEClient := &LetsEncryptClient{
				Order: test.Order,
			}

			actual := testLEClient.GetOrderEndpoint()

			if actual != test.ExpectedCertificate {
				t.Errorf("GetOrderEndpoint() %s: got %s, expected %s\n", test.Name, actual, test.ExpectedCertificate)
			}
		})
	}
}

func TestFetchCertificates(t *testing.T) {
	tests := []struct {
		Name                string
		ACME                *acmemock.FakeAcmeClient
		ExpectError         bool
		ExpectedErrorString string
	}{
		{
			Name: "fetch certificates when let's encrypt is up",
			ACME: &acmemock.FakeAcmeClient{
				Available: true,
			},
			ExpectError: false,
		},
		{
			Name: "fetch certificates when let's encrypt is down",
			ACME: &acmemock.FakeAcmeClient{
				Available: false,
			},
			ExpectError:         true,
			ExpectedErrorString: "acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			testLEClient := LetsEncryptClient{
				Client:  test.ACME,
				Account: acme.Account{},
				Order:   acme.Order{},
			}

			// FetchCertificates() is a wrapper around an acme call. since it doesn't
			// do any processing, there's no point checking what it returns
			_, err := testLEClient.FetchCertificates()
			if err != nil {
				if !test.ExpectError {
					t.Errorf("FetchCertificates() %s: got unexpected error \"%s\"\n", test.Name, err)
				}
				if err.Error() != test.ExpectedErrorString {
					t.Errorf("FetchCertificates() %s: got error \"%s\", expected \"%s\"\n", test.Name, err, test.ExpectedErrorString)
				}
			} else {
				if test.ExpectError {
					t.Errorf("FetchCertificates() %s: expected error \"%s\" but didn't get it\n", test.Name, test.ExpectedErrorString)
				}
			}

			if !test.ACME.FetchCertificatesCalled {
				t.Errorf("FetchCertificates() %s: expected the acme client FetchCertificates() to be called but it wasn't\n", test.Name)
			}
		})
	}
}

func TestRevokeCertificate(t *testing.T) {
	tests := []struct {
		Name                string
		ACME                *acmemock.FakeAcmeClient
		ExpectError         bool
		ExpectedErrorString string
	}{
		{
			Name: "revoke certificate when let's encrypt is up",
			ACME: &acmemock.FakeAcmeClient{
				Available: true,
			},
			ExpectError: false,
		},
		{
			Name: "revoke certificate when let's encrypt is down",
			ACME: &acmemock.FakeAcmeClient{
				Available: false,
			},
			ExpectError:         true,
			ExpectedErrorString: "acme: error code 0 \"urn:acme:error:serverInternal\": The service is down for maintenance or had an internal error. Check https://letsencrypt.status.io/ for more details",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			testLEClient := LetsEncryptClient{
				Client:  test.ACME,
				Account: acme.Account{},
				Order:   acme.Order{},
			}

			err := testLEClient.RevokeCertificate(&x509.Certificate{})
			if err != nil {
				if !test.ExpectError {
					t.Errorf("RevokeCertificate() %s: got unexpected error \"%s\"\n", test.Name, err)
				}
				if err.Error() != test.ExpectedErrorString {
					t.Errorf("RevokeCertificate() %s: got error \"%s\", expected \"%s\"\n", test.Name, err, test.ExpectedErrorString)
				}
			} else {
				if test.ExpectError {
					t.Errorf("RevokeCertificate() %s: expected error \"%s\" but didn't get it\n", test.Name, test.ExpectedErrorString)
				}
			}

			if !test.ACME.RevokeCertificateCalled {
				t.Errorf("RevokeCertificate() %s: expected the acme client RevokeCertificate() to be called but it wasn't\n", test.Name)
			}
		})
	}
}

// helpers

/*
This is a newly-created ES256 elliptic curve key that has only been used for
these tests. It should never be used for anything else.
*/
var leAccountPrivKey = []byte(`-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIKjjz0SZwf3Mpo10i1VXPZPv/8/DCWX0iQ7mBjWhjY6OoAoGCCqGSM49
AwEHoUQDQgAEejflvU67Dt2u8Edg7wmcrG2GCKt7VKRL0Iy9LN8LILmEhCqYaM45
Yiu4AbJf3ISUdPj0QlWOcw0kGEXLC/w2dw==
-----END EC PRIVATE KEY-----
`)

func setUpEmptyTestClient(t *testing.T) (testClient client.Client) {
	t.Helper()

	/*
	  lets-encrypt-account is not an existing secret
	  lets-encrypt-account-production is not an existing secret
	  lets-encrypt-account-staging is not an existing secret
	*/
	objects := []runtime.Object{}

	testClient = fake.NewClientBuilder().WithRuntimeObjects(objects...).Build()
	return
}

func setUpTestClient(t *testing.T, accountSecretName string) (testClient client.Client) {
	t.Helper()

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: config.OperatorNamespace,
			Name:      accountSecretName,
		},
		Data: map[string][]byte{
			"private-key": leAccountPrivKey,
		},
	}
	objects := []runtime.Object{secret}

	testClient = fake.NewClientBuilder().WithRuntimeObjects(objects...).Build()
	return
}
