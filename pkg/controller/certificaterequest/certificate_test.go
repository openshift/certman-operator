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

	"k8s.io/apimachinery/pkg/runtime"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
)

func TestParseCertificateData(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "private key test",
			data:    leAccountPrivKey,
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseCertificateData(test.data)
			if (err != nil) != test.wantErr {
				t.Errorf("ParseCertificateData() Got unexpected error: %v", err)
			}
		})
	}
}

func TestGetCertificate(t *testing.T) {
	tests := []struct {
		name     string
		testCert *certmanv1alpha1.CertificateRequest
		wantErr  bool
	}{
		{
			name:     "Test cert request with no secret",
			testCert: certRequest,
			wantErr:  true,
		},
	}

	var c = setUpTestClient(t, []runtime.Object{certRequest})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := GetCertificate(c, test.testCert)
			if (err != nil) != test.wantErr {
				t.Errorf("GetCertificate() Got unexpected error: %v", err)
			}
		})
	}
}
