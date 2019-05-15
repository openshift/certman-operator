package clusterdeployment

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	certmanapis "github.com/openshift/certman-operator/pkg/apis"
	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	hiveapis "github.com/openshift/hive/pkg/apis"
	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
)

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

type CertificateRequestEntry struct {
	name     string
	dnsNames []string
}

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
						testIngressDefaultDomain,
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

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClient(test.localObjects...)

			rcd := &ReconcileClusterDeployment{
				client: fakeClient,
				scheme: scheme.Scheme,
			}

			_, err := rcd.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testClusterName,
					Namespace: testNamespace,
				},
			})

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			crList := certmanv1alpha1.CertificateRequestList{}
			assert.NoError(t, fakeClient.List(context.TODO(), &client.ListOptions{Namespace: testNamespace}, &crList), "error listing CertificateRequests")

			// make sure we have the right number of CertificateRequests generated
			assert.Equal(t, len(test.expectedCertificateRequests), len(crList.Items), "list of CRs doesn't match expectations")

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
			PlatformSecrets: hivev1alpha1.PlatformSecrets{
				AWS: &hivev1alpha1.AWSPlatformSecrets{
					Credentials: corev1.LocalObjectReference{
						Name: testAWSCredentialsSecret,
					},
				},
			},
		},
		Status: hivev1alpha1.ClusterDeploymentStatus{
			Installed: true,
		},
	}

	return &cd
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
