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

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
)

func TestReconcile(t *testing.T) {
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
			name:                       "errors if AWS account secret is unset",
			clientObjects:              []runtime.Object{testLESecret, certRequest, emptyCertSecret},
			expectedCertificateRequest: certRequest,
			expectError:                true,
		},
		{
			name:          "update status of a new certificaterequest with old secret",
			clientObjects: []runtime.Object{testLESecret, certRequest, validCertSecret, clusterDeploymentComplete},
			expectedCertificateRequest: &certmanv1alpha1.CertificateRequest{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CertificateRequest",
					APIVersion: "certman.managed.openshift.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testHiveNamespace,
					Name:      testHiveCertificateRequestName,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "hive.openshift.io/v1",
							Kind:               "ClusterDeployment",
							Name:               clusterDeploymentComplete.Name,
							UID:                clusterDeploymentComplete.UID,
							Controller:         boolPointer(true),
							BlockOwnerDeletion: boolPointer(true),
						},
					},
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
		{
			name:          "don't manage certs on outgoing clusterdeployment relocation",
			clientObjects: []runtime.Object{testLESecret, clusterDeploymentOutgoing, certRequest, expiredCertSecret},
			expectedCertificateRequest: func() *certmanv1alpha1.CertificateRequest {
				cr := &certmanv1alpha1.CertificateRequest{}
				cr.TypeMeta = certRequest.TypeMeta
				cr.ObjectMeta = certRequest.ObjectMeta
				cr.Spec = certRequest.Spec
				cr.Status = certRequest.Status
				cr.Status.Status = "Not reconciling: ClusterDeployment is relocating"

				return cr
			}(),
			expectError: false,
		},
		{
			name:          "do manage certs on incoming clusterdeployment relocation",
			clientObjects: []runtime.Object{testLESecret, clusterDeploymentIncoming, certRequest, validCertSecret},
			expectedCertificateRequest: &certmanv1alpha1.CertificateRequest{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CertificateRequest",
					APIVersion: "certman.managed.openshift.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testHiveNamespace,
					Name:      testHiveCertificateRequestName,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "hive.openshift.io/v1",
							Kind:               "ClusterDeployment",
							Name:               clusterDeploymentComplete.Name,
							UID:                clusterDeploymentComplete.UID,
							Controller:         boolPointer(true),
							BlockOwnerDeletion: boolPointer(true),
						},
					},
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
		{
			name:          "do manage certs on complete clusterdeployment relocation",
			clientObjects: []runtime.Object{testLESecret, clusterDeploymentComplete, certRequest, validCertSecret},
			expectedCertificateRequest: &certmanv1alpha1.CertificateRequest{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CertificateRequest",
					APIVersion: "certman.managed.openshift.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testHiveNamespace,
					Name:      testHiveCertificateRequestName,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "hive.openshift.io/v1",
							Kind:               "ClusterDeployment",
							Name:               clusterDeploymentComplete.Name,
							UID:                clusterDeploymentComplete.UID,
							Controller:         boolPointer(true),
							BlockOwnerDeletion: boolPointer(true),
						},
					},
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
		{
			name: "reqeusts a new cert",
			clientObjects: func() []runtime.Object {
				cr := &certmanv1alpha1.CertificateRequest{}
				cr.TypeMeta = certRequest.TypeMeta
				cr.ObjectMeta = certRequest.ObjectMeta
				//cr.ObjectMeta.OwnerReferences = []metav1.OwnerReference{} // this should already be set to actual owner refs by ^
				cr.Spec = certRequest.Spec
				// cr.Status = certRequest.Status // need to test that it sets the status

				// generate an leclient using a mock acme client
				newClientSecret := testLESecret
				newClientSecret.Data["account-url"] = []byte("proto://use.mock.acme.client")

				return []runtime.Object{newClientSecret, cr, clusterDeploymentComplete}
			}(),
			expectedCertificateRequest: &certmanv1alpha1.CertificateRequest{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CertificateRequest",
					APIVersion: "certman.managed.openshift.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testHiveNamespace,
					Name:      testHiveCertificateRequestName,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "hive.openshift.io/v1",
							Kind:               "ClusterDeployment",
							Name:               clusterDeploymentComplete.Name,
							UID:                clusterDeploymentComplete.UID,
							Controller:         boolPointer(true),
							BlockOwnerDeletion: boolPointer(true),
						},
					},
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
		{
			name: "handles multiple ingresses and cert secrets",
			clientObjects: func() []runtime.Object {
				cr := &certmanv1alpha1.CertificateRequest{}
				cr.TypeMeta = certRequest.TypeMeta
				cr.ObjectMeta = certRequest.ObjectMeta
				//cr.ObjectMeta.OwnerReferences = []metav1.OwnerReference{} // this should already be set to actual owner refs by ^
				cr.Spec = certRequest.Spec
				// cr.Status = certRequest.Status // need to test that it sets the status
				cr.Spec.CertificateSecret.Name = "ocm-cert-bundle-secret"

				// generate an leclient using a mock acme client
				newClientSecret := testLESecret
				newClientSecret.Data["account-url"] = []byte("proto://use.mock.acme.client")
				clusterDeploymentComplete.Spec.CertificateBundles = append(clusterDeploymentComplete.Spec.CertificateBundles, hivev1.CertificateBundleSpec{
					Name:     "ocm-cert-bundle-secret",
					Generate: true,
					CertificateSecretRef: corev1.LocalObjectReference{
						Name: "ocm-cert-bundle-secret",
					},
				})
				clusterDeploymentComplete.Spec.Ingress = append(clusterDeploymentComplete.Spec.Ingress, hivev1.ClusterIngress{
					Name:               "ocm",
					Domain:             "ocm.whatever.hostname.tld",
					ServingCertificate: "ocm-cert-bundle-secret",
				})

				return []runtime.Object{newClientSecret, cr, clusterDeploymentComplete}
			}(),
			expectedCertificateRequest: &certmanv1alpha1.CertificateRequest{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CertificateRequest",
					APIVersion: "certman.managed.openshift.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testHiveNamespace,
					Name:      testHiveCertificateRequestName,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "hive.openshift.io/v1",
							Kind:               "ClusterDeployment",
							Name:               clusterDeploymentComplete.Name,
							UID:                clusterDeploymentComplete.UID,
							Controller:         boolPointer(true),
							BlockOwnerDeletion: boolPointer(true),
						},
					},
				},
				Spec: certmanv1alpha1.CertificateRequestSpec{
					ACMEDNSDomain: testHiveACMEDomain,
					CertificateSecret: corev1.ObjectReference{
						Kind:      "Secret",
						Namespace: testHiveNamespace,
						Name:      "ocm-cert-bundle-secret",
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
			s := runtime.NewScheme()
			s.AddKnownTypes(certmanv1alpha1.SchemeGroupVersion, certRequest)

			// run the reconcile loop
			rcr := ReconcileCertificateRequest{
				client:        testClient,
				clientBuilder: setUpFakeAWSClient,
				scheme:        s,
			}
			_, err := rcr.Reconcile(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testHiveNamespace, Name: testHiveCertificateRequestName}})
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

			if !reflect.DeepEqual(actualCertficateRequest.ObjectMeta.OwnerReferences, test.expectedCertificateRequest.ObjectMeta.OwnerReferences) {
				t.Errorf("Reconcile() certificaterequest ownerreferences = %v, want %v", actualCertficateRequest.ObjectMeta.OwnerReferences, test.expectedCertificateRequest.ObjectMeta.OwnerReferences)
			}
		})
	}
}

func TestRelocationBailOut(t *testing.T) {
	tests := []struct {
		Name           string
		NamespacedName types.NamespacedName
		KubeObjects    []runtime.Object
		Expected       bool
		ExpectErr      bool
	}{
		{
			Name:           "clusterdeployment is relocating",
			NamespacedName: types.NamespacedName{Namespace: clusterDeploymentOutgoing.Namespace, Name: clusterDeploymentOutgoing.Name},
			KubeObjects:    []runtime.Object{clusterDeploymentOutgoing},
			Expected:       true,
			ExpectErr:      false,
		},
		{
			Name:           "clusterdeployment is not relocating",
			NamespacedName: types.NamespacedName{Namespace: clusterDeploymentComplete.Namespace, Name: clusterDeploymentComplete.Name},
			KubeObjects:    []runtime.Object{clusterDeploymentComplete},
			Expected:       false,
			ExpectErr:      false,
		},
		{
			Name:           "clusterdeployment is not found",
			NamespacedName: types.NamespacedName{},
			KubeObjects:    []runtime.Object{},
			Expected:       false,
			ExpectErr:      true,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			k := setUpTestClient(t, test.KubeObjects)

			relocating, err := relocationBailOut(k, test.NamespacedName)
			if (err != nil) && !test.ExpectErr {
				t.Errorf("relocationBailOut(): test.ExpectErr: %v, got: %v", test.ExpectErr, err)
			}

			if relocating != test.Expected {
				t.Errorf("relocationBailOut(): expected: %t got: %t", test.Expected, relocating)
			}
		})
	}
}
