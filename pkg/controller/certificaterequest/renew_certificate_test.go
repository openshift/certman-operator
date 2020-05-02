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

	logrTesting "github.com/go-logr/logr/testing"
	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
)

func TestShouldReissue(t *testing.T) {
	tests := []struct {
		desc     string
		testCert *certmanv1alpha1.CertificateRequest
		want     bool
	}{
		{
			desc:     "Test cert's secret has no data",
			testCert: certRequest,
			want:     true,
		},
	}

	//set up empty test client
	testClient := setUpTestClient(t, testHiveSecretName, true)
	//create a reconcile certificate object
	rcr := ReconcileCertificateRequest{
		client:        testClient,
		clientBuilder: setUpFakeAWSClient,
	}
	//create a null logger
	nullLogger := logrTesting.NullLogger{}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {

			got, _ := rcr.ShouldReissue(nullLogger, test.testCert)

			if got != test.want {
				t.Errorf("ShouldReissue() = %v, want = %v", got, test.want)
			}

		})
	}

}
