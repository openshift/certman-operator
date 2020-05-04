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
	"testing"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	//logrTesting "github.com/go-logr/logr/testing"
)

func TestParseCertificateData(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantNil bool
	}{
		{
			name:    "private key test",
			data:    leAccountPrivKey,
			wantNil: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _ := ParseCertificateData(test.data)
			if (got == nil) != test.wantNil {
				t.Errorf("ParseCertificateData() Error: Got = %v", got)
			}
		})
	}
}

func TestGetCertificate(t *testing.T) {
	tests := []struct {
		name     string
		testCert *certmanv1alpha1.CertificateRequest
		wantNil  bool
	}{
		{
			name:     "Test cert request with no secret",
			testCert: certRequest,
			wantNil:  true,
		},
	}

	var c = setUpEmptyTestClient(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _ := GetCertificate(c, test.testCert)

			if (got == nil) != test.wantNil {
				t.Errorf("GetCertificate() Error: Got = %v", got)
			}
		})
	}
}
