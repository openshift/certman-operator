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

	cTypes "github.com/openshift/certman-operator/pkg/clients/types"
	hiveapis "github.com/openshift/hive/apis"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	certmanv1alpha1 "github.com/openshift/certman-operator/api/v1alpha1"
)

// Define const's for testing.
const (
	testClusterName              = "foo"
	testUID                      = types.UID("1234")
	testNamespace                = "foonamespace"
	testBaseDomain               = "testing.example.com"
	testCertBundleName           = "testbundle"
	testAWSCredentialsSecret     = "aws-iam-secret"
	testExtraControlPlaneDNSName = "anotherapi.testing.example.com"
	testIngressDefaultDomain     = "apps.testing.example.com"
)

// CertificateRequestEntry generates a test Certificate that logic is validated
// with or against.
type CertificateRequestEntry struct {
	name     string
	dnsNames []string
}

// TestClusterDeploymentReconciler use table driven tests to assess cases
// that are associated with the type.
func TestClusterDeploymentReconciler(t *testing.T) {
	err := certmanv1alpha1.AddToScheme(scheme.Scheme)
	assert.Nil(t, err, "Error returned while attempting to AddToScheme: %q", err)

	err = hiveapis.AddToScheme(scheme.Scheme)
	assert.Nil(t, err, "Error returned while attempting to AddToScheme: %q", err)

	testObjects := func(obj runtime.Object) []runtime.Object {
		objList := testObjects()
		objList = append(objList, obj)
		return objList
	}

	tests := []struct {
		name                        string
		localObjects                []runtime.Object
		expectedCertificateRequests []CertificateRequestEntry
		expectFinalizerPresent      bool
	}{
		{
			name:                   "Test no cert bundles to generate",
			localObjects:           testObjects(testClusterDeploymentAws()),
			expectFinalizerPresent: true,
		},
		{
			name:                   "Test un-managed certificate request",
			localObjects:           testObjects(testUnmanagedClusterDeployment()),
			expectFinalizerPresent: false,
		},
		{
			name:                   "Test not installed cluster deployment",
			localObjects:           testObjects(testNotInstalledClusterDeployment()),
			expectFinalizerPresent: false,
		},
		{
			name:                   "Test deletion of certificate request",
			localObjects:           testObjects(testhandleDeleteClusterDeployment()),
			expectFinalizerPresent: false,
		},
		{
			name:         "Test generate control plane cert",
			localObjects: testObjects(testClusterDeploymentWithGenerateAPI()),
			expectedCertificateRequests: []CertificateRequestEntry{
				{
					name:     fmt.Sprintf("%s-%s", testClusterName, testCertBundleName),
					dnsNames: []string{fmt.Sprintf("api.%s.%s", testClusterName, testBaseDomain)},
				},
			},
			expectFinalizerPresent: true,
		},
		{
			name:         "Test generate cert with multi control plane",
			localObjects: testObjects(testClusterDeploymentWithAdditionalControlPlaneCert()),
			expectedCertificateRequests: []CertificateRequestEntry{
				{
					name: fmt.Sprintf("%s-%s", testClusterName, testCertBundleName),
					dnsNames: []string{
						fmt.Sprintf("api.%s.%s", testClusterName, testBaseDomain),
						testExtraControlPlaneDNSName,
					},
				},
			},
			expectFinalizerPresent: true,
		},
		{
			name:         "Test generate multi-control plane with ingress",
			localObjects: testObjects(testClusterDeploymentWithMultiControlPlaneAndIngress()),
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
			expectFinalizerPresent: true,
		},
		{
			name: "Test removing existing CertificateRequest",
			localObjects: func() []runtime.Object {
				cd := testClusterDeploymentAws()
				objects := testObjects(cd)

				cr := testCertificateRequest(cd)
				objects = append(objects, cr)
				return objects
			}(),
			expectFinalizerPresent: true,
		},
		{
			name: "Test claiming ownerless certificaterequest",
			localObjects: func() []runtime.Object {
				cd := testClusterDeploymentAws()
				objects := testObjects(cd)

				cr := testCertificateRequest(cd)
				// make a certificaterequest without the ownerRef field
				unownedCr := &certmanv1alpha1.CertificateRequest{}
				unownedCr.TypeMeta = cr.TypeMeta
				unownedCr.Spec = cr.Spec
				unownedCr.ObjectMeta.Name = cr.ObjectMeta.Name
				unownedCr.ObjectMeta.Namespace = cr.ObjectMeta.Namespace
				unownedCr.Status = cr.Status
				objects = append(objects, unownedCr)
				return objects
			}(),
			expectFinalizerPresent: true,
		},
		{
			name: "Test cluster relocation",
			localObjects: func() []runtime.Object {
				cd := testClusterDeploymentAws()
				cd.SetAnnotations(map[string]string{"hive.openshift.io/relocate": "fakehive/outgoing"})
				return []runtime.Object{cd}
			}(),
			// if the finalizer isn't present and no errors bubble up, the reconcile loop didn't run
			expectFinalizerPresent: false,
		},
		{
			name: "Test fake ClusterDeployment",
			localObjects: func() []runtime.Object {
				cd := testClusterDeploymentAws()
				cd.SetAnnotations(map[string]string{fakeClusterDeploymentAnnotation: "true"})
				return []runtime.Object{cd}
			}(),
			// if the finalizer isn't present and no errors bubble up, the reconcile loop didn't run
			expectFinalizerPresent: false,
		},
	}

	// Iterate over test array.
	for _, test := range tests {

		// Run each test within the table
		t.Run(test.name, func(t *testing.T) {

			// Create a NewFakeClient to interact with Reconcile functionality.
			// localObjects are defined within each test
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(test.localObjects...).Build()

			// Instantiate a ClusterDeploymentReconciler type to act as a reconcile client
			rcd := &ClusterDeploymentReconciler{
				Client: fakeClient,
				Scheme: scheme.Scheme,
			}

			// Call the ClusterDeploymentReconciler types Reconcile method with a test name and namespace object
			// to Reconcile. Validate no error is returned.
			_, err := rcd.Reconcile(context.TODO(), reconcile.Request{
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
			assert.NoError(t, fakeClient.List(context.TODO(), &crList, client.InNamespace(testNamespace)), "Error listing CertificateRequests")

			// make sure we have the right number of CertificateRequests generated
			assert.Equal(t, len(test.expectedCertificateRequests), len(crList.Items), "expectedCertificateRequests=%d should match crList.Items=%d", len(test.expectedCertificateRequests), len(crList.Items))

			// Validate whether the finalizer should be present in the resulting clusterdeployment
			cd := &hivev1.ClusterDeployment{}
			err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: testNamespace, Name: testClusterName}, cd)

			// Recent version of controller-runtime handles the Patch request in Reconcile differently which fails Get request above.
			// Since the only test having deletiontimestamp set in beginning of the test is the "Test deletion of certificate request",
			// the Reconcile loop deletes the ClusterDeployment object from the object tracker of fake client which will always fail the Get request.
			// Thus, the following if-else condition is used for now to handle the one test separately for now.
			if test.name != "Test deletion of certificate request" {
				assert.Nil(t, err, "unable to find ClusterDeployment: %q", err)
				foundFinalizer := false
				for _, finalizer := range cd.Finalizers {
					if finalizer == certmanv1alpha1.CertmanOperatorFinalizerLabel {
						foundFinalizer = true
					}
				}
				assert.Equal(t, test.expectFinalizerPresent, foundFinalizer, "expectFinalizerPresent=%v should match foundFinalizer=%v", test.expectFinalizerPresent, foundFinalizer)
			} else {
				assert.NotNil(t, err, "couldn't successfully delete ClusterDeployment: %q", err)
			}

			// validate each CertificateRequest
			for _, expectedCertReq := range test.expectedCertificateRequests {
				found := false
				for _, existingCR := range crList.Items {
					if expectedCertReq.name == existingCR.Name {
						found = true
						validateCertificateRequest(t, expectedCertReq, existingCR, cd)
						break
					}
				}

				assert.True(t, found, "didn't find expected CertificateRequest %s", expectedCertReq.name)
			}
		})
	}
}

func validateCertificateRequest(t *testing.T, expectedCertReq CertificateRequestEntry, actualCR certmanv1alpha1.CertificateRequest, cd *hivev1.ClusterDeployment) {
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

	expectedOwnerReference := &metav1.OwnerReference{
		APIVersion: hivev1.HiveAPIVersion,
		Kind:       "ClusterDeployment",
		Name:       cd.Name,
	}

	assert.Equal(t, expectedOwnerReference.Kind, actualCR.ObjectMeta.OwnerReferences[0].Kind, "owner reference kind is incorrect")
	assert.Equal(t, expectedOwnerReference.Name, actualCR.ObjectMeta.OwnerReferences[0].Name, "owner reference is incorrect")

	assert.Equal(t, testAWSCredentialsSecret, actualCR.Spec.Platform.AWS.Credentials.Name, "didn't find expected AWS creds secret name")
}

func testClusterDeploymentWithMultiControlPlaneAndIngress() *hivev1.ClusterDeployment {
	cd := testClusterDeploymentWithAdditionalControlPlaneCert()

	cd.Spec.Ingress = []hivev1.ClusterIngress{
		{
			Name:               "default",
			Domain:             testIngressDefaultDomain,
			ServingCertificate: testCertBundleName,
		},
	}

	return cd
}
func testClusterDeploymentWithAdditionalControlPlaneCert() *hivev1.ClusterDeployment {

	cd := testClusterDeploymentWithGenerateAPI()

	cd.Spec.ControlPlaneConfig.ServingCertificates.Additional = []hivev1.ControlPlaneAdditionalCertificate{
		{
			Name:   testCertBundleName,
			Domain: testExtraControlPlaneDNSName,
		},
	}

	return cd
}

func testClusterDeploymentWithGenerateAPI() *hivev1.ClusterDeployment {
	cd := testClusterDeploymentAws()

	cd.Spec.ControlPlaneConfig = hivev1.ControlPlaneConfigSpec{
		ServingCertificates: hivev1.ControlPlaneServingCertificateSpec{
			Default: testCertBundleName,
		},
	}

	cd.Spec.CertificateBundles = []hivev1.CertificateBundleSpec{
		{
			Name:     testCertBundleName,
			Generate: true,
			CertificateSecretRef: corev1.LocalObjectReference{
				Name: "testBundleSecret",
			},
		},
	}

	return cd
}

// testClusterDeployment returns a test clusterdeployment from hive
// populated with testing defined variables
func testClusterDeploymentAws() *hivev1.ClusterDeployment {
	cd := hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testClusterName,
			Namespace: testNamespace,
			UID:       testUID,
			Labels: map[string]string{
				ClusterDeploymentManagedLabel: "true",
			},
		},
		Spec: hivev1.ClusterDeploymentSpec{
			BaseDomain:  testBaseDomain,
			ClusterName: testClusterName,
			Installed:   true,
			Platform: hivev1.Platform{
				AWS: &hivev1aws.Platform{
					Region: "dreamland",
					CredentialsSecretRef: corev1.LocalObjectReference{
						Name: testAWSCredentialsSecret},
				},
			},
		},
	}

	return &cd
}

// testUnmanagedClusterDeployment returns testClusterDeployment with
// the ClusterDeploymentManagedLabel equal to false.
func testUnmanagedClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeploymentAws()
	cd.Labels[ClusterDeploymentManagedLabel] = "false"
	return cd
}

// testNotInstalledClusterDeployment returns testClusterDeployment with
// the Spec.Installed equalt to false.
func testNotInstalledClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeploymentAws()
	cd.Spec.Installed = false
	return cd
}

// testhandleDeleteClusterDeployment returns testClusterDeployment with
// SetDeletionTimestamp and Finalizer to test certificate deletion.
func testhandleDeleteClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeploymentAws()
	now := metav1.Now()
	cd.ObjectMeta.SetDeletionTimestamp(&now)
	cd.ObjectMeta.Finalizers = append(cd.ObjectMeta.Finalizers, certmanv1alpha1.CertmanOperatorFinalizerLabel)
	return cd
}

// testCertificateRequest will create a dummy certificaterequest with an owner reference set to the
// passed-in ClusterDeployment
func testCertificateRequest(cd *hivev1.ClusterDeployment) *certmanv1alpha1.CertificateRequest {
	isController := true
	cr := certmanv1alpha1.CertificateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cert-request",
			Namespace: testNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: hivev1.HiveAPIVersion,
					Kind:       "ClusterDeployment",
					Name:       cd.Name,
					UID:        cd.UID,
					Controller: &isController,
				},
			},
		},
	}

	return &cr
}

// testObjects returns a testing objects
func testObjects() []runtime.Object {
	objects := []runtime.Object{}

	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "certman-operator",
			Namespace: "certman-operator",
		},
		Data: map[string]string{
			cTypes.DefaultNotificationEmailAddress: "email@example.com",
		},
	}
	objects = append(objects, cm.DeepCopyObject())

	sAws := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testAWSCredentialsSecret,
			Namespace: "certman-operator",
		},

		StringData: map[string]string{
			"aws_access_key_id":     "aws-iam-key",
			"aws_secret_access_key": "aws-access-key",
		},
	}
	objects = append(objects, sAws.DeepCopyObject())

	sGcp := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gcp-secret",
			Namespace: "certman-operator",
		},

		StringData: map[string]string{
			"osServiceAccount.json": "random-data",
		},
	}
	objects = append(objects, sGcp.DeepCopyObject())

	return objects

}
