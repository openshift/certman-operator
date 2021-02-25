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
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
)

func TestReconcile(t *testing.T) {
	t.Run("errors if AWS account secret is unset", func(t *testing.T) {
		testClient := setUpTestClient(t, []runtime.Object{testStagingLESecret, certRequest, emptyCertSecret})

		_, err := rcrReconcile(t, testClient)

		if err == nil {
			t.Error("expected an error when reconciling without an AWS account secret")
		}
	})

	tt := []struct {
		name                       string
		clientObjects              []runtime.Object
		expectedCertificateRequest *certmanv1alpha1.CertificateRequest
		expectError                bool
	}{
		{
			name:                       "errors if lets-encrypt account secret is unset",
			clientObjects:              []runtime.Object{certRequest, emptyCertSecret},
			expectedCertificateRequest: certRequest,
			expectError:                true,
		},
		{
			name:          "update status of a new certificaterequest with old secret",
			clientObjects: []runtime.Object{testStagingLESecret, certRequest, validCertSecret},
			expectedCertificateRequest: &certmanv1alpha1.CertificateRequest{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CertificateRequest",
					APIVersion: "certman.managed.openshift.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testHiveNamespace,
					Name:      testHiveCertificateRequestName,
				},
				Spec: certmanv1alpha1.CertificateRequestSpec{
					ACMEDNSDomain: testHiveACMEDomain,
					CertificateSecret: corev1.ObjectReference{
						Kind:      "Secret",
						Namespace: testHiveNamespace,
						Name:      testHiveSecretName,
					},
					Platform: certmanv1alpha1.Platform{},
					DnsNames: []string{
						"api.gibberish.goes.here",
					},
					Email:             "devnull@donot.route",
					ReissueBeforeDays: 10,
				},
				Status: certmanv1alpha1.CertificateRequestStatus{
					Issued:     true,
					Status:     "Success",
					IssuerName: "api.gibberish.goes.here",
					// from validCertSecret
					NotBefore:    "2021-02-23 21:31:08 +0000 UTC",
					NotAfter:     "2121-01-30 21:31:08 +0000 UTC",
					SerialNumber: "178590107285161329516895083813532600983388099859",
				},
			},
			expectError: false,
		},
	}

	for _, test := range tt {
		t.Run(test.name, func(t *testing.T) {
			testClient := setUpTestClient(t, test.clientObjects)

			// run the reconcile loop
			_, err := rcrReconcile(t, testClient)
			if (err == nil) == test.expectError {
				t.Errorf("Reconcile() return error: %s. was one expected? %t", err, test.expectError)
			}

			// grab the certificaterequest from the test namespace
			actualCertficateRequest := &certmanv1alpha1.CertificateRequest{}
			err = testClient.Get(context.TODO(), client.ObjectKey{
				Namespace: testHiveNamespace,
				Name:      testHiveCertificateRequestName,
			},
				actualCertficateRequest,
			)
			if err != nil {
				t.Fatalf("unexpected error getting certificate request: %s", err)
			}

			// compare the certificaterequest from the fake client with what the test case expects
			if !reflect.DeepEqual(actualCertficateRequest.Spec, test.expectedCertificateRequest.Spec) {
				t.Errorf("Reconcile() certificaterequest spec = %v, want %v", actualCertficateRequest.Spec, test.expectedCertificateRequest.Spec)
			}

			if !reflect.DeepEqual(actualCertficateRequest.Status, test.expectedCertificateRequest.Status) {
				t.Errorf("Reconcile() certificaterequest status = %v, want %v", actualCertficateRequest.Status, test.expectedCertificateRequest.Status)
			}
		})
	}
}
