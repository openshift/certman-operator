/*
Copyright 2019 Red Hat, Inc.

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

package clusterdeployment

import (
	"context"
	"fmt"
	"testing"

	certmanapis "github.com/openshift/certman-operator/pkg/apis"
	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"

	hiveapis "github.com/openshift/hive/pkg/apis"
	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	hivev1aws "github.com/openshift/hive/pkg/apis/hive/v1alpha1/aws"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Define const's for testing.
const (
	testClusterName              = "foo"
	testUID                      = types.UID("1234")
	testNamespace                = "foonamespace"
	testBaseDomain               = "testing.example.com"
	testCertBundleName           = "testbundle"
	testAWSCredentialsSecret     = "aws-credentials"
	testExtraControlPlaneDNSName = "anotherapi.testing.example.com"
	testIngressDefaultDomain     = "apps.testing.example.com"
)

// CertificateRequestEntry generates a test Certificate that logic is validated
// with or against.
type CertificateRequestEntry struct {
	name     string
	dnsNames []string
}

// TestReconcileClusterDeployment use table driven tests to assess cases
// that are associated with the type.
func TestReconcileClusterDeployment(t *testing.T) {
	certmanapis.AddToScheme(scheme.Scheme)
	hiveapis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name                        string
		localObjects                []runtime.Object
		expectedCertificateRequests []CertificateRequestEntry
	}{
		{
			name: "Test no cert bundles to generate",
			localObjects: []runtime.Object{
				testConfigMap(),
				testClusterDeployment(),
			},
		},
		{
			name: "Test un-managed certificate request",
			localObjects: []runtime.Object{
				testConfigMap(),
				testUnmanagedClusterDeployment(),
			},
		},
		{
			name: "Test not installed cluster deployment",
			localObjects: []runtime.Object{
				testConfigMap(),
				testNotInstalledClusterDeployment(),
			},
		},
		{
			name: "Test deletion of certificate request",
			localObjects: []runtime.Object{
				testConfigMap(),
				testhandleDeleteClusterDeployment(),
			},
		},

		{
			name: "Test generate control plane cert",
			localObjects: []runtime.Object{
				testConfigMap(),
				testClusterDeploymentWithGenerateAPI(),
			},
			expectedCertificateRequests: []CertificateRequestEntry{
				{
					name:     fmt.Sprintf("%s-%s", testClusterName, testCertBundleName),
					dnsNames: []string{fmt.Sprintf("api.%s.%s", testClusterName, testBaseDomain)},
				},
			},
		},
		{
			name: "Test generate cert with multi control plane",
			localObjects: []runtime.Object{
				testClusterDeploymentWithAdditionalControlPlaneCert(),
				testConfigMap(),
			},
			expectedCertificateRequests: []CertificateRequestEntry{
				{
					name: fmt.Sprintf("%s-%s", testClusterName, testCertBundleName),
					dnsNames: []string{
						fmt.Sprintf("api.%s.%s", testClusterName, testBaseDomain),
						testExtraControlPlaneDNSName,
					},
				},
			},
		},
		{
			name: "Test generate multi-control plane with ingress",
			localObjects: []runtime.Object{
				testClusterDeploymentWithMultiControlPlaneAndIngress(),
				testConfigMap(),
			},
			expectedCertificateRequests: []CertificateRequestEntry{
				{
					name: fmt.Sprintf("%s-%s", testClusterName, testCertBundleName),
					dnsNames: []string{
						fmt.Sprintf("api.%s.%s", testClusterName, testBaseDomain),
						testExtraControlPlaneDNSName,
						"*." + testIngressDefaultDomain,
					},
				},
			},
		},
		{
			name: "Test removing existing CertificateRequest",
			localObjects: func() []runtime.Object {
				objects := []runtime.Object{}
				cd := testClusterDeployment()
				objects = append(objects, cd)

				cr := testCertificateRequest(cd)
				objects = append(objects, cr)

				cm := testConfigMap()
				objects = append(objects, cm)

				return objects
			}(),
		},
	}

	// Iterate over test array.
	for _, test := range tests {

		// Run each test within the table
		t.Run(test.name, func(t *testing.T) {

			// Create a NewFakeClient to interact with Reconcile functionality.
			// localObjects are defined within each test
			fakeClient := fake.NewFakeClient(test.localObjects...)

			// Instantiate a ReconcileClusterDeployment type to act as a reconcile client
			rcd := &ReconcileClusterDeployment{
				client: fakeClient,
				scheme: scheme.Scheme,
			}

			// Call the ReconcileClusterDeployment types Reconcile method with a test name and namespace object
			// to Reconcile. Validate no error is returned.
			_, err := rcd.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testClusterName,
					Namespace: testNamespace,
				},
			})

			// assert no error has been returned from calling Reconcile.
			assert.Nil(t, err, "Error returned while attempting to reconcile: %q", err)

			// Instantiate crList as a CertificateRequestList struct
			crList := certmanv1alpha1.CertificateRequestList{}

			// Assert no error is returned when listing certificates in the defined namespace
			assert.NoError(t, fakeClient.List(context.TODO(), &client.ListOptions{Namespace: testNamespace}, &crList), "Error listing CertificateRequests")

			// make sure we have the right number of CertificateRequests generated
			assert.Equal(t, len(test.expectedCertificateRequests), len(crList.Items), "expectedCertificateRequests=%d should match crList.Items=%d", len(test.expectedCertificateRequests), len(crList.Items))

			// validate each CertificateRequest
			for _, expectedCertReq := range test.expectedCertificateRequests {
				found := false
				for _, existingCR := range crList.Items {
					if expectedCertReq.name == existingCR.Name {
						found = true
						validateCertificateRequest(t, expectedCertReq, existingCR)
						break
					}
				}

				assert.True(t, found, "didn't find expected CertificateRequest %s", expectedCertReq.name)
			}

		})

	}
}

func validateCertificateRequest(t *testing.T, expectedCertReq CertificateRequestEntry, actualCR certmanv1alpha1.CertificateRequest) {
	for _, expectedDNSName := range expectedCertReq.dnsNames {
		found := false
		for _, actualDNSName := range actualCR.Spec.DnsNames {
			if expectedDNSName == actualDNSName {
				found = true
				break
			}
		}
		assert.True(t, found, "didn't find expected DNS Name in list: %s", expectedDNSName)
	}

	assert.Equal(t, testAWSCredentialsSecret, actualCR.Spec.PlatformSecrets.AWS.Credentials.Name, "didn't find expected AWS creds secret name")

	return
}

func testClusterDeploymentWithMultiControlPlaneAndIngress() *hivev1alpha1.ClusterDeployment {
	cd := testClusterDeploymentWithAdditionalControlPlaneCert()

	cd.Spec.Ingress = []hivev1alpha1.ClusterIngress{
		{
			Name:               "default",
			Domain:             testIngressDefaultDomain,
			ServingCertificate: testCertBundleName,
		},
	}

	return cd
}
func testClusterDeploymentWithAdditionalControlPlaneCert() *hivev1alpha1.ClusterDeployment {

	cd := testClusterDeploymentWithGenerateAPI()

	cd.Spec.ControlPlaneConfig.ServingCertificates.Additional = []hivev1alpha1.ControlPlaneAdditionalCertificate{
		{
			Name:   testCertBundleName,
			Domain: testExtraControlPlaneDNSName,
		},
	}

	return cd
}

func testClusterDeploymentWithGenerateAPI() *hivev1alpha1.ClusterDeployment {
	cd := testClusterDeployment()

	cd.Spec.ControlPlaneConfig = hivev1alpha1.ControlPlaneConfigSpec{
		ServingCertificates: hivev1alpha1.ControlPlaneServingCertificateSpec{
			Default: testCertBundleName,
		},
	}

	cd.Spec.CertificateBundles = []hivev1alpha1.CertificateBundleSpec{
		{
			Name:     testCertBundleName,
			Generate: true,
			SecretRef: corev1.LocalObjectReference{
				Name: "testBundleSecret",
			},
		},
	}

	return cd
}

// testClusterDeployment returns a test clusterdeployment from hive
// populated with testing defined variables
func testClusterDeployment() *hivev1alpha1.ClusterDeployment {
	cd := hivev1alpha1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testClusterName,
			Namespace: testNamespace,
			UID:       testUID,
			Labels: map[string]string{
				ClusterDeploymentManagedLabel: "true",
			},
		},
		Spec: hivev1alpha1.ClusterDeploymentSpec{
			BaseDomain:  testBaseDomain,
			ClusterName: testClusterName,
			Installed:   true,
			PlatformSecrets: hivev1alpha1.PlatformSecrets{
				AWS: &hivev1aws.PlatformSecrets{
					Credentials: corev1.LocalObjectReference{
						Name: testAWSCredentialsSecret,
					},
				},
			},
		},
	}

	return &cd
}

// testUnmanagedClusterDeployment returns testClusterDeployment with
// the ClusterDeploymentManagedLabel equal to false.
func testUnmanagedClusterDeployment() *hivev1alpha1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Labels[ClusterDeploymentManagedLabel] = "false"
	return cd
}

// testNotInstalledClusterDeployment returns testClusterDeployment with
// the Spec.Installed equalt to false.
func testNotInstalledClusterDeployment() *hivev1alpha1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Spec.Installed = false
	return cd
}

// testhandleDeleteClusterDeployment returns testClusterDeployment with
// SetDeletionTimestamp and Finalizer to test certificate deletion.
func testhandleDeleteClusterDeployment() *hivev1alpha1.ClusterDeployment {
	cd := testClusterDeployment()
	now := metav1.Now()
	cd.ObjectMeta.SetDeletionTimestamp(&now)
	cd.ObjectMeta.Finalizers = append(cd.ObjectMeta.Finalizers, certmanv1alpha1.CertmanOperatorFinalizerLabel)
	return cd
}

// testCertificateRequest will create a dummy certificaterequest with an owner reference set to the
// passed-in ClusterDeployment
func testCertificateRequest(cd *hivev1alpha1.ClusterDeployment) *certmanv1alpha1.CertificateRequest {
	isController := true
	cr := certmanv1alpha1.CertificateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cert-request",
			Namespace: testNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Name:       cd.Name,
					UID:        cd.UID,
					Controller: &isController,
				},
			},
		},
	}

	return &cr
}

// testConfigMap returns a testing ConfigMap object populated
// with testing variables.
func testConfigMap() *corev1.ConfigMap {

	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "certman-operator",
			Namespace: "certman-operator",
		},
		Data: map[string]string{
			"lets_encrypt_environment":           "staging",
			"default_notification_email_address": "email@example.com",
		},
	}

	return &cm

}
