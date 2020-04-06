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
	"testing"

	"github.com/openshift/certman-operator/config"
	"k8s.io/api/core/v1"
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

func TestGetLetsEncryptAccountSecret(t *testing.T) {
	t.Run("returns an error if no secret is found", func(t *testing.T) {
		testClient := setUpEmptyTestClient(t)

		_, actual := getLetsEncryptAccountSecret(testClient)

		if actual == nil {
			t.Errorf("expected an error when attempting to get missing account secrets")
		}
	})

	t.Run("returns a secret", func(t *testing.T) {
		t.Run("if only deprecated staging secret is set", func(t *testing.T) {
			testClient := setUpTestClient(t, letsEncryptStagingAccountSecretName)

			verifyAccountSecret(t, testClient, letsEncryptStagingAccountSecretName)
		})

		t.Run("if only deprecated production secret is set", func(t *testing.T) {
			testClient := setUpTestClient(t, letsEncryptProductionAccountSecretName)

			verifyAccountSecret(t, testClient, letsEncryptProductionAccountSecretName)
		})

		t.Run("if only approved secret is set", func(t *testing.T) {
			testClient := setUpTestClient(t, letsEncryptAccountSecretName)

			verifyAccountSecret(t, testClient, letsEncryptAccountSecretName)
		})

	})
}

func TestNewClient(t *testing.T) {
	t.Run("returns an error if no account secret is found", func(t *testing.T) {
		testClient := setUpEmptyTestClient(t)

		_, actual := NewClient(testClient)

		if actual == nil {
			t.Errorf("expected an error when attempting to get missing account secrets")
		}
	})

	t.Run("returns an leclient", func(t *testing.T) {
		t.Run("if only deprecated staging secret is set", func(t *testing.T) {
			testClient := setUpTestClient(t, letsEncryptStagingAccountSecretName)

			verifyLEClient(t, testClient)
		})

		t.Run("if only deprecated production secret is set", func(t *testing.T) {
			testClient := setUpTestClient(t, letsEncryptProductionAccountSecretName)

			verifyLEClient(t, testClient)
		})

		t.Run("if only approved secret is set", func(t *testing.T) {
			testClient := setUpTestClient(t, letsEncryptAccountSecretName)

			verifyLEClient(t, testClient)
		})
	})
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

/*
Confirm that `leclient.getLetsEncryptAccountSecret()` returns the right secret.
Takes a kube client and the name of the secret to confirm was retrieved. The
kube client should already have the secret created in it.
*/
func verifyAccountSecret(t *testing.T, testClient client.Client, secretName string) {
	t.Helper()

	// this will return an error if the secret is missing
	secret, err := getLetsEncryptAccountSecret(testClient)
	if err != nil {
		t.Fatalf("got an unexpected error retrieving the account secret: %q", err)
	}

	actual := secret.Name
	expected := secretName

	if actual != expected {
		t.Errorf("got %q expected %q", actual, expected)
	}
}

/*
Confirm that `leclient.NewClient` returns an ACMEClient. Takes a kube client
with the secret created in it.
*/
func verifyLEClient(t *testing.T, testClient client.Client) {
	leclient, err := NewClient(testClient)
	if err != nil {
		t.Fatalf("unexpected error creating the leclient: %q", err)
	}

	if leclient == nil {
		t.Errorf("leclient failed to set up")
	}
}
