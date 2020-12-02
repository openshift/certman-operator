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
	"crypto/x509/pkix"
	"testing"
)

func TestIsCertificateIssuerLE(t *testing.T) {
	tests := []struct {
		name     string
		issuer   pkix.Name
		response bool
	}{
		{
			// https://letsencrypt.org/certs/fakeleintermediatex1.pem
			name: "Staging LE Issuer",
			issuer: pkix.Name{
				CommonName: "Fake LE Intermediate X1",
			},
			response: true,
		},
		{
			// https://letsencrypt.org/certs/lets-encrypt-r3.pem
			name: "R3 LE Issuer",
			issuer: pkix.Name{
				Country: []string{
					"US",
				},
				Organization: []string{
					"Let's Encrypt",
				},
				CommonName: "R3",
			},
			response: true,
		},
		{
			// https://letsencrypt.org/certs/letsencryptauthorityx3.pem
			name: "X3 LE Issuer",
			issuer: pkix.Name{
				Country: []string{
					"US",
				},
				Organization: []string{
					"Let's Encrypt",
				},
				CommonName: "Let's Encrypt Authority X3",
			},
			response: true,
		},
		{
			// Made up issuer
			name: "Other Issuer",
			issuer: pkix.Name{
				Country: []string{
					"US",
				},
				Organization: []string{
					"My Cool CA",
				},
				CommonName: "Z9",
			},
			response: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := IsCertificateIssuerLE(test.issuer)

			if got != test.response {
				t.Errorf("error validating LE cert issuer: got %v, want %v", got, test.response)
			}

		})
	}
}
