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

	"github.com/go-logr/logr"
	cTypes "github.com/openshift/certman-operator/pkg/clients/types"
	hiveapis "github.com/openshift/hive/apis"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/apis/hive/v1/azure"
	"github.com/openshift/hive/apis/hive/v1/gcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				unownedCr.Name = cr.Name
				unownedCr.Namespace = cr.Namespace
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
			assert.Nil(t, err, "unable to find ClusterDeployment: %q", err)
			foundFinalizer := false
			for _, finalizer := range cd.Finalizers {
				if finalizer == certmanv1alpha1.CertmanOperatorFinalizerLabel {
					foundFinalizer = true
				}
			}
			assert.Equal(t, test.expectFinalizerPresent, foundFinalizer, "expectFinalizerPresent=%v should match foundFinalizer=%v", test.expectFinalizerPresent, foundFinalizer)

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

// TestCertificateRequestDeletion tests the deletion of the CertificateRequest.
// Recent version of controller-runtime handles the Patch request in Reconcile differently which fails Get request for ClusterDeployment as well.
// Since the only test having deletiontimestamp set is this test "Test deletion of certificate request",
// the Reconcile loop deletes the ClusterDeployment object from the object tracker of fake client which will always fail the Get request.
// Thus, the following test is split from the rest of the tests above where it's expected that the Get request will fail to confirm deletion of ClusterDeployment.
func TestCertificateRequestDeletion(t *testing.T) {
	err := hiveapis.AddToScheme(scheme.Scheme)
	assert.Nil(t, err, "Error returned while attempting to AddToScheme: %q", err)

	testObjects := func(obj runtime.Object) []runtime.Object {
		objList := testObjects()
		objList = append(objList, obj)
		return objList
	}

	t.Run("Test deletion of certificate request", func(t *testing.T) {

		// Create a NewFakeClient to interact with Reconcile functionality.
		// localObjects are defined within each test
		fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(testObjects(testhandleDeleteClusterDeployment())...).Build()

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

		// Validate whether the finalizer should be present in the resulting clusterdeployment
		cd := &hivev1.ClusterDeployment{}
		err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: testNamespace, Name: testClusterName}, cd)
		assert.NotNil(t, err, "unable to delete ClusterDeployment: %q", err)
	})
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

	assert.Equal(t, expectedOwnerReference.Kind, actualCR.OwnerReferences[0].Kind, "owner reference kind is incorrect")
	assert.Equal(t, expectedOwnerReference.Name, actualCR.OwnerReferences[0].Name, "owner reference is incorrect")

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
	cd.SetDeletionTimestamp(&now)
	cd.Finalizers = append(cd.Finalizers, certmanv1alpha1.CertmanOperatorFinalizerLabel)
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

func TestCreateCertificateRequest(t *testing.T) {
	tests := []struct {
		name          string
		certBundle    string
		secretName    string
		domains       []string
		email         string
		cd            *hivev1.ClusterDeployment
		expectAWS     bool
		expectGCP     bool
		expectAzure   bool
		expectSecrets bool
	}{
		{
			name:       "gcp_platform_setup",
			certBundle: "bundle1",
			secretName: "cert-secret",
			domains:    []string{"example.com"},
			email:      "test@example.com",
			cd: &hivev1.ClusterDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-cluster",
					Namespace: "my-namespace",
				},
				Spec: hivev1.ClusterDeploymentSpec{
					BaseDomain: "example.com",
					Platform: hivev1.Platform{
						GCP: &gcp.Platform{
							CredentialsSecretRef: corev1.LocalObjectReference{Name: "gcp-creds"},
						},
					},
				},
				Status: hivev1.ClusterDeploymentStatus{
					APIURL:        "https://api.example.com",
					WebConsoleURL: "https://console.example.com",
				},
			},
			expectGCP:     true,
			expectSecrets: true,
		},
		{
			name:       "aws_platform_setup",
			certBundle: "awsbundle",
			secretName: "aws-cert",
			domains:    []string{"aws.example.com"},
			email:      "aws@example.com",
			cd: &hivev1.ClusterDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-cluster",
					Namespace: "aws-ns",
				},
				Spec: hivev1.ClusterDeploymentSpec{
					BaseDomain: "aws.example.com",
					Platform: hivev1.Platform{
						AWS: &hivev1aws.Platform{
							Region: "us-east-1",
							CredentialsSecretRef: corev1.LocalObjectReference{
								Name: "aws-creds",
							},
						},
					},
				},
			},
			expectAWS:     true,
			expectSecrets: true,
		},
		{
			name:       "azure_platform_setup",
			certBundle: "azurebundle",
			secretName: "azure-secret",
			domains:    []string{"azure.example.com"},
			email:      "azure@example.com",
			cd: &hivev1.ClusterDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "az-cluster",
					Namespace: "az-ns",
				},
				Spec: hivev1.ClusterDeploymentSpec{
					BaseDomain: "azure.example.com",
					Platform: hivev1.Platform{
						Azure: &azure.Platform{
							CredentialsSecretRef: corev1.LocalObjectReference{
								Name: "azure-creds",
							},
							BaseDomainResourceGroupName: "az-group",
						},
					},
				},
			},
			expectAzure:   true,
			expectSecrets: true,
		},
		{
			name:       "no_platform_setup",
			certBundle: "plainbundle",
			secretName: "plain-secret",
			domains:    []string{"plain.example.com"},
			email:      "plain@example.com",
			cd: &hivev1.ClusterDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "plain-cluster",
					Namespace: "plain-ns",
				},
				Spec: hivev1.ClusterDeploymentSpec{
					BaseDomain: "plain.example.com",
					Platform:   hivev1.Platform{},
				},
			},
			expectSecrets: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := createCertificateRequest(tt.certBundle, tt.secretName, tt.domains, tt.cd, tt.email)

			assert.Equal(t, fmt.Sprintf("%s-%s", tt.cd.Name, tt.certBundle), cr.Name)
			assert.Equal(t, tt.cd.Namespace, cr.Namespace)
			assert.Equal(t, tt.domains, cr.Spec.DnsNames)
			assert.Equal(t, tt.cd.Spec.BaseDomain, cr.Spec.ACMEDNSDomain)
			assert.Equal(t, tt.email, cr.Spec.Email)
			assert.Equal(t, tt.secretName, cr.Spec.CertificateSecret.Name)
			assert.Equal(t, tt.cd.Status.APIURL, cr.Spec.APIURL)
			assert.Equal(t, tt.cd.Status.WebConsoleURL, cr.Spec.WebConsoleURL)

			if tt.expectAWS {
				require.NotNil(t, cr.Spec.Platform.AWS)
				assert.Equal(t, "aws-creds", cr.Spec.Platform.AWS.Credentials.Name)
				assert.Equal(t, tt.cd.Spec.Platform.AWS.Region, cr.Spec.Platform.AWS.Region)
			} else {
				assert.Nil(t, cr.Spec.Platform.AWS)
			}

			if tt.expectGCP {
				require.NotNil(t, cr.Spec.Platform.GCP)
				assert.Equal(t, "gcp-creds", cr.Spec.Platform.GCP.Credentials.Name)
			} else {
				assert.Nil(t, cr.Spec.Platform.GCP)
			}

			if tt.expectAzure {
				require.NotNil(t, cr.Spec.Platform.Azure)
				assert.Equal(t, "azure-creds", cr.Spec.Platform.Azure.Credentials.Name)
				assert.Equal(t, "az-group", cr.Spec.Platform.Azure.ResourceGroupName)
			} else {
				assert.Nil(t, cr.Spec.Platform.Azure)
			}
		})
	}
}

func TestGetDomainsForCertBundle(t *testing.T) {
	cases := []struct {
		name           string
		cbName         string
		cd             *hivev1.ClusterDeployment
		extraRecordEnv string
		expectDomains  []string
	}{
		{
			name:   "default_control_plane_cert_with_extra_record",
			cbName: "default-cert",
			cd: &hivev1.ClusterDeployment{
				Spec: hivev1.ClusterDeploymentSpec{
					ClusterName: "foo",
					BaseDomain:  "bar.io",
					ControlPlaneConfig: hivev1.ControlPlaneConfigSpec{
						ServingCertificates: hivev1.ControlPlaneServingCertificateSpec{
							Default: "default-cert",
						},
					},
				},
			},
			extraRecordEnv: "extra",
			expectDomains: []string{
				"api.foo.bar.io",
				"extra.foo.bar.io",
			},
		},
		{
			name:   "additional_control_plane_cert",
			cbName: "cp-additional",
			cd: &hivev1.ClusterDeployment{
				Spec: hivev1.ClusterDeploymentSpec{
					ControlPlaneConfig: hivev1.ControlPlaneConfigSpec{
						ServingCertificates: hivev1.ControlPlaneServingCertificateSpec{
							Additional: []hivev1.ControlPlaneAdditionalCertificate{{
								Name:   "cp-additional",
								Domain: "internal.cp.domain",
							}},
						},
					},
				}},
			expectDomains: []string{"internal.cp.domain"},
		},
		{
			name:   "ingress_cert_with_wildcard",
			cbName: "ingress-cert",
			cd: &hivev1.ClusterDeployment{
				Spec: hivev1.ClusterDeploymentSpec{
					Ingress: []hivev1.ClusterIngress{{
						ServingCertificate: "ingress-cert",
						Domain:             "apps.foo.io",
					}},
				},
			},
			expectDomains: []string{"*.apps.foo.io"},
		},
		{
			name:   "ingress_cert_with_existing_wildcard",
			cbName: "ingress-cert",
			cd: &hivev1.ClusterDeployment{
				Spec: hivev1.ClusterDeploymentSpec{
					Ingress: []hivev1.ClusterIngress{{
						ServingCertificate: "ingress-cert",
						Domain:             "*.prewild.foo.io",
					}},
				},
			},
			expectDomains: []string{"*.prewild.foo.io"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.extraRecordEnv != "" {
				t.Setenv("EXTRA_RECORD", tc.extraRecordEnv)
			} else {
				t.Setenv("EXTRA_RECORD", "")
			}

			cb := hivev1.CertificateBundleSpec{
				Name: tc.cbName,
			}

			logger := logr.Discard()
			domains := getDomainsForCertBundle(cb, tc.cd, logger)
			assert.ElementsMatch(t, tc.expectDomains, domains)
		})
	}
}

func TestGetCurrentCertificateRequests(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, certmanv1alpha1.AddToScheme(scheme))
	require.NoError(t, hivev1.AddToScheme(scheme))

	namespace := "test-ns"

	clusterDeployment := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: namespace,
		},
	}

	certCR1 := &certmanv1alpha1.CertificateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cr-1",
			Namespace: namespace,
		},
	}
	certCR2 := &certmanv1alpha1.CertificateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cr-2",
			Namespace: namespace,
		},
	}

	tests := []struct {
		name          string
		clientBuilder func() client.Client
		expectedNames []string
		expectError   bool
	}{
		{
			name: "should_return_all_certificateRequests_in_the_namespace",
			clientBuilder: func() client.Client {
				return fake.NewClientBuilder().
					WithScheme(scheme).
					WithRuntimeObjects(clusterDeployment, certCR1, certCR2).
					Build()
			},
			expectedNames: []string{"cr-1", "cr-2"},
			expectError:   false,
		},
		{
			name: "should_return_empty_list_if_no_certificateRequests_exist",
			clientBuilder: func() client.Client {
				return fake.NewClientBuilder().
					WithScheme(scheme).
					WithRuntimeObjects(clusterDeployment). // no CRs
					Build()
			},
			expectedNames: []string{},
			expectError:   false,
		},
		{
			name: "should_return_error_if_list_fails",
			clientBuilder: func() client.Client {
				return &failingClient{scheme: scheme}
			},
			expectedNames: nil,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reconciler := &ClusterDeploymentReconciler{
				Client: tt.clientBuilder(),
				Scheme: scheme,
			}

			crs, err := reconciler.getCurrentCertificateRequests(clusterDeployment, logr.Discard())

			if tt.expectError {
				require.Error(t, err)
				assert.Empty(t, crs)
			} else {
				require.NoError(t, err)
				require.Len(t, crs, len(tt.expectedNames))
				for i, expectedName := range tt.expectedNames {
					assert.Equal(t, expectedName, crs[i].Name)
				}
			}
		})
	}
}

func TestHandleDelete(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, certmanv1alpha1.AddToScheme(scheme))
	require.NoError(t, hivev1.AddToScheme(scheme))

	cd := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cd",
			Namespace: "default",
		},
		Spec: hivev1.ClusterDeploymentSpec{
			BaseDomain: "example.com",
		},
	}

	cr1 := &certmanv1alpha1.CertificateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cr1",
			Namespace: "default",
		},
	}
	cr2 := &certmanv1alpha1.CertificateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cr2",
			Namespace: "default",
		},
	}

	tests := []struct {
		name        string
		objects     []runtime.Object
		expectErr   bool
		expectedCRs []string
	}{
		{
			name:      "no_certificate_requests",
			objects:   []runtime.Object{cd},
			expectErr: false,
		},
		{
			name:      "delete_existing_certificate_requests",
			objects:   []runtime.Object{cd, cr1, cr2},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.objects...).Build()

			r := &ClusterDeploymentReconciler{
				Client: cl,
				Scheme: scheme,
			}

			err := r.handleDelete(cd, logr.Discard())
			if (err != nil) != tt.expectErr {
				t.Errorf("expected error = %v, got %v", tt.expectErr, err)
			}

			for _, obj := range tt.objects {
				if cr, ok := obj.(*certmanv1alpha1.CertificateRequest); ok {
					err := cl.Get(context.TODO(), client.ObjectKeyFromObject(cr), &certmanv1alpha1.CertificateRequest{})
					if err == nil {
						t.Errorf("expected CertificateRequest %s to be deleted", cr.Name)
					}
				}
			}
		})
	}
}

// helper type to simulate a failing client
type failingClient struct {
	client.Client
	scheme *runtime.Scheme
}

func (f *failingClient) Scheme() *runtime.Scheme {
	return f.scheme
}

func (f *failingClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return fmt.Errorf("simulated list error")
}
