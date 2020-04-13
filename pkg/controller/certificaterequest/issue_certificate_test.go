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

package certificaterequest

import (
	"crypto/rand"
	"crypto/rsa"
	"github.com/stretchr/testify/assert"
	"testing"

	logrTesting "github.com/go-logr/logr/testing"
)

func TestIssueCertificate(t *testing.T) {
	t.Run("errors if lets-encrypt account secret is unset", func(t *testing.T) {
		testClient := setUpEmptyTestClient(t)
		rcr := ReconcileCertificateRequest{
			client:        testClient,
			clientBuilder: setUpFakeAWSClient,
		}

		nullLogger := logrTesting.NullLogger{}

		err := rcr.IssueCertificate(nullLogger, certRequest, certSecret)

		if err == nil {
			t.Error("expected an error")
		}
	})
}

func TestGenerateCRS(t *testing.T) {
	t.Run("Tests generating CSRs", func(t *testing.T) {
		certKey, err := rsa.GenerateKey(rand.Reader, rSAKeyBitSize)
		if err != nil {
			t.Error("Found an error with test setup, creating a private key")
		}

		DNSNames := []string{
			"foo.bar",
			"test.more",
		}

		csr, err := generateCRS(certKey, DNSNames)
		if err != nil {
			t.Error("Received an error when calling generateCRS")
		}
		assert.Equal(t, DNSNames, csr.DNSNames, "Expected DNS domains were not in csr SAN.")
		assert.Equal(t, certKey.Public(), csr.PublicKey, "Expected public key to be in csr.")
	})
}
