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
	"reflect"
	"testing"

	"github.com/eggsampler/acme"
	"github.com/openshift/certman-operator/config"
	"github.com/openshift/certman-operator/pkg/leclient/mock"
	"k8s.io/api/core/v1"
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
		ACME                *mock.FakeAcmeClient
		Email               string
		ExpectedContacts    []string
		ExpectError         bool
		ExpectedErrorString string
	}{
		{
			Name: "UpdateAccount when Let's Encrypt is up",
			ACME: &mock.FakeAcmeClient{
				Available: true,
			},
			Email:               "doesn't@ma.tter",
			ExpectedContacts:    []string{"mailto:doesn't@ma.tter"},
			ExpectError:         false,
			ExpectedErrorString: "",
		},
		{
			Name: "update when Let's Encrypt is down",
			ACME: &mock.FakeAcmeClient{
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
		ACME                *mock.FakeAcmeClient
		Domains             []string
		ExpectedIds         []acme.Identifier
		ExpectError         bool
		ExpectedErrorString string
	}{
		{
			Name: "create order when Let's Encrypt is up",
			ACME: &mock.FakeAcmeClient{
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
			ACME: &mock.FakeAcmeClient{
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

	testClient = fake.NewFakeClient(objects...)
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

	testClient = fake.NewFakeClient(objects...)
	return
}
