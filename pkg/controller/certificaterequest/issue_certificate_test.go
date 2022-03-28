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
	"strings"
	"testing"

	logrTesting "github.com/go-logr/logr/testing"
	dto "github.com/prometheus/client_model/go"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	lemock "github.com/openshift/certman-operator/pkg/leclient/mock"
	"github.com/openshift/certman-operator/pkg/localmetrics"
)

func TestIssueCertificate(t *testing.T) {
	testCases := []struct {
		Name                 string
		KubeObjects          []runtime.Object
		LEClientOptions      *lemock.FakeLetsEncryptClientOptions
		ExpectError          bool
		ExpectedErrorMessage string
		ExpectedMetricValue  interface{}
	}{
		{
			Name:                 "handles letsencrypt maintenance",
			KubeObjects:          []runtime.Object{certRequest, validCertSecret},
			LEClientOptions:      &lemock.FakeLetsEncryptClientOptions{Available: false},
			ExpectError:          true,
			ExpectedErrorMessage: leMaintMessage,
			ExpectedMetricValue:  float64(1),
		},
	}

	for _, test := range testCases {
		t.Run(test.Name, func(t *testing.T) {
			t.Helper()

			nullLogger := logrTesting.NullLogger{}

			testClient := setUpTestClient(t, test.KubeObjects)

			// get the certificaterequest and cert secret from the kube client objects
			cr := &certmanv1alpha1.CertificateRequest{}
			err := testClient.Get(context.TODO(), types.NamespacedName{Namespace: testHiveNamespace, Name: testHiveCertificateRequestName}, cr)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			s := &v1.Secret{}
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			err = testClient.Get(context.TODO(), types.NamespacedName{Namespace: testHiveNamespace, Name: testHiveSecretName}, s)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			leMockClient := lemock.NewFakeLetsEncryptClient(test.LEClientOptions)

			rcr := ReconcileCertificateRequest{
				client:        testClient,
				clientBuilder: setUpFakeAWSClient,
			}
			testErr := rcr.IssueCertificate(nullLogger, cr, s, leMockClient)
			if err != nil && !test.ExpectError {
				t.Errorf("got unexpected error: %s", err)
			}

			if test.ExpectedMetricValue != nil {
				metricDest := &dto.Metric{Counter: &dto.Counter{}}
				err = localmetrics.MetricLetsEncryptMaintenanceErrorCount.Write(metricDest)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				metricValue := metricDest.Counter.GetValue()
				if !reflect.DeepEqual(test.ExpectedMetricValue, metricValue) {
					t.Errorf("expected: %v, got %v", test.ExpectedMetricValue, metricValue)
				}
			}

			if test.ExpectError {
				if !strings.Contains(testErr.Error(), test.ExpectedErrorMessage) {
					t.Errorf("error (%s) did not contain expected message (%s)", err.Error(), test.ExpectedErrorMessage)
				}
			}
		})
	}
}
