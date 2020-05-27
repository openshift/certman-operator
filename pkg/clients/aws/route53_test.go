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

package aws

import (
	"testing"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNewClient(t *testing.T) {
	t.Run("returns an error if the credentials aren't set", func(t *testing.T) {
		testClient := setUpEmptyTestClient(t)

		_, actual := NewClient(testClient, testHiveAWSSecretName, testHiveNamespace, testHiveAWSRegion)

		if actual == nil {
			t.Error("expected an error when attempting to get missing account secret")
		}
	})

	t.Run("returns a client if the credential is set", func(t *testing.T) {
		testClient := setUpTestClient(t)

		_, err := NewClient(testClient, testHiveAWSSecretName, testHiveNamespace, testHiveAWSRegion)

		if err != nil {
			t.Errorf("unexpected error when creating the client: %q", err)
		}
	})
}

// helpers
var testHiveNamespace = "uhc-doesntexist-123456"
var testHiveCertificateRequestName = "clustername-1313-0-primary-cert-bundle"
var testHiveCertSecretName = "primary-cert-bundle-secret"
var testHiveACMEDomain = "not.a.valid.tld"
var testHiveAWSSecretName = "aws"
var testHiveAWSRegion = "not-relevant-1"

var certRequest = &certmanv1alpha1.CertificateRequest{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: testHiveNamespace,
		Name:      testHiveCertificateRequestName,
	},
	Spec: certmanv1alpha1.CertificateRequestSpec{
		ACMEDNSDomain: testHiveACMEDomain,
		CertificateSecret: v1.ObjectReference{
			Kind:      "Secret",
			Namespace: testHiveNamespace,
			Name:      testHiveCertSecretName,
		},
		Platform: certRequestPlatform,
		DnsNames: []string{
			"api.gibberish.goes.here",
		},
		Email:             "devnull@donot.route",
		ReissueBeforeDays: 10000,
	},
}

var certRequestPlatform = certmanv1alpha1.Platform{
	AWS: &certmanv1alpha1.AWSPlatformSecrets{
		Credentials: v1.LocalObjectReference{
			Name: testHiveAWSSecretName,
		},
		Region: testHiveAWSRegion,
	},
}

var certSecret = &v1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: testHiveNamespace,
		Name:      testHiveCertSecretName,
	},
}

var awsSecret = &v1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: testHiveNamespace,
		Name:      testHiveAWSSecretName,
	},
	Data: map[string][]byte{
		"aws_access_key_id":     {},
		"aws_secret_access_key": {},
	},
}

func setUpEmptyTestClient(t *testing.T) (testClient client.Client) {
	t.Helper()

	s := scheme.Scheme
	s.AddKnownTypes(certmanv1alpha1.SchemeGroupVersion, certRequest)

	// aws is not an existing secret
	objects := []runtime.Object{certRequest}

	testClient = fake.NewFakeClientWithScheme(s, objects...)
	return
}

func setUpTestClient(t *testing.T) (testClient client.Client) {
	t.Helper()

	s := scheme.Scheme
	s.AddKnownTypes(certmanv1alpha1.SchemeGroupVersion, certRequest)

	// aws is not an existing secret
	objects := []runtime.Object{certRequest, awsSecret}

	testClient = fake.NewFakeClientWithScheme(s, objects...)
	return
}
