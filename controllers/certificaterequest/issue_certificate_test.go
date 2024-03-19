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

	"github.com/eggsampler/acme"
	"github.com/go-logr/logr"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	dto "github.com/prometheus/client_model/go"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	certmanv1alpha1 "github.com/openshift/certman-operator/api/v1alpha1"
	acmemock "github.com/openshift/certman-operator/pkg/acmeclient/mock"
	"github.com/openshift/certman-operator/pkg/leclient"
	"github.com/openshift/certman-operator/pkg/localmetrics"
)

func TestIssueCertificate(t *testing.T) {
	testCases := []struct {
		Name                 string
		KubeObjects          []runtime.Object
		LEClient             *leclient.LetsEncryptClient
		ExpectError          bool
		ExpectedErrorMessage string
		ExpectedMetricValue  interface{}
	}{
		{
			Name:        "gets a certificate",
			KubeObjects: []runtime.Object{certRequest, validCertSecret},
			LEClient: &leclient.LetsEncryptClient{
				Client: acmemock.NewFakeAcmeClient(&acmemock.FakeAcmeClientOptions{
					Available: true,
					NewOrderResult: acme.Order{
						Authorizations: []string{"proto://a.fake.url"},
					},
					FetchAuthorizationResult: acme.Authorization{
						Identifier: acme.Identifier{
							Value: "issue-certificate-auth-id",
						},
					},
				}),
			},
			ExpectError: false,
		},
		{
			Name:        "handles letsencrypt maintenance",
			KubeObjects: []runtime.Object{certRequest, validCertSecret},
			LEClient: &leclient.LetsEncryptClient{
				Client: &acmemock.FakeAcmeClient{
					Available: false,
					NewOrderResult: acme.Order{
						Authorizations: []string{"proto://a.fake.url"},
					},
				},
			},
			ExpectError:          true,
			ExpectedErrorMessage: leMaintMessage,
			ExpectedMetricValue:  float64(1),
		},
	}

	for _, test := range testCases {
		t.Run(test.Name, func(t *testing.T) {
			t.Helper()

			nullLogger := logr.Discard()

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

			rcr := CertificateRequestReconciler{
				Client:        testClient,
				ClientBuilder: setUpFakeAWSClient,
			}
			testErr := rcr.IssueCertificate(nullLogger, cr, s, test.LEClient)
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

func TestFindZoneIDForChallenge(t *testing.T) {
	testZoneID := "test.openshift.io"
	tests := []struct {
		name                 string
		KubeObjects          []runtime.Object
		expectedError        bool
		expectedZone         string
		expectedErrorMessage string
	}{
		{
			name:                 "test-empty-result-list",
			KubeObjects:          []runtime.Object{},
			expectedZone:         "",
			expectedError:        true,
			expectedErrorMessage: "0 dnsZone objects in a specific namespace found, expected 1 dnsZone",
		},
		{
			name: "test more than 1 dnszone",
			KubeObjects: []runtime.Object{
				&hivev1.DNSZone{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test1",
						Namespace: "test",
					},
				},
				&hivev1.DNSZone{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test2",
						Namespace: "test",
					},
				},
			},
			expectedZone:         "",
			expectedError:        true,
			expectedErrorMessage: "2 dnsZone objects in a specific namespace found, expected 1 dnsZone",
		},
		{
			name: "AWS Zone ID doesn't Exist",
			KubeObjects: []runtime.Object{
				&hivev1.DNSZone{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test1",
						Namespace: "test",
					},
					Status: hivev1.DNSZoneStatus{
						AWS: &hivev1.AWSDNSZoneStatus{},
					},
				},
			},
			expectedZone:         "",
			expectedError:        true,
			expectedErrorMessage: "aws ZoneID doesn't exist",
		},
		{
			name: "AWS Zone ID Exist",
			KubeObjects: []runtime.Object{
				&hivev1.DNSZone{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test1",
						Namespace: "test",
					},
					Status: hivev1.DNSZoneStatus{
						AWS: &hivev1.AWSDNSZoneStatus{
							ZoneID: &testZoneID,
						},
					},
				},
			},
			expectedZone:  "test.openshift.io",
			expectedError: false,
		},
		{
			name: "GCP Zone Name doesn't Exist",
			KubeObjects: []runtime.Object{
				&hivev1.DNSZone{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test1",
						Namespace: "test",
					},
					Status: hivev1.DNSZoneStatus{
						GCP: &hivev1.GCPDNSZoneStatus{},
					},
				},
			},
			expectedZone:         "",
			expectedError:        true,
			expectedErrorMessage: "gcp ZoneName doesn't exist",
		},
		{
			name: "GCP Zone Name Exist",
			KubeObjects: []runtime.Object{
				&hivev1.DNSZone{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test1",
						Namespace: "test",
					},
					Status: hivev1.DNSZoneStatus{
						GCP: &hivev1.GCPDNSZoneStatus{
							ZoneName: &testZoneID,
						},
					},
				},
			},
			expectedZone:  "test.openshift.io",
			expectedError: false,
		},
		{
			name: "AWS/GCP DNSZoneStatus is nil",
			KubeObjects: []runtime.Object{
				&hivev1.DNSZone{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test1",
						Namespace: "test",
					},
					Status: hivev1.DNSZoneStatus{},
				},
			},
			expectedZone:         "",
			expectedError:        true,
			expectedErrorMessage: "unexpected error: not aws or gcp don't know what to do here",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			testClient := setUpTestClient(t, tc.KubeObjects)
			reconciler := &CertificateRequestReconciler{
				Client: testClient,
			}

			zoneID, err := reconciler.FindZoneIDForChallenge("test")

			if err == nil && tc.expectedError {
				t.Fatalf("got no error when expecting an error")
			}
			if err != nil && !tc.expectedError {
				t.Fatalf("unexpected error - Expected: %v - Got - %v", tc.expectedError, err)
			}
			if err != nil && err.Error() != tc.expectedErrorMessage {
				t.Fatalf("unexpected error message - Expected: %v - Got - %v", tc.expectedErrorMessage, err.Error())
			}
			if tc.expectedZone != zoneID {
				t.Fatalf("unexpected zone - Expected: %v - Got - %v", tc.expectedZone, zoneID)
			}
		})
	}
}
