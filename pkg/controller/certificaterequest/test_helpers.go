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

	"github.com/aws/aws-sdk-go/service/route53/route53iface"
	logr "github.com/go-logr/logr"
	"github.com/openshift/certman-operator/config"
	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	cClient "github.com/openshift/certman-operator/pkg/clients"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// helpers

var testHiveNamespace = "uhc-doesntexist-123456"
var testHiveCertificateRequestName = "clustername-1313-0-primary-cert-bundle"
var testHiveSecretName = "primary-cert-bundle-secret"
var testHiveACMEDomain = "not.a.valid.tld"

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
			Name:      testHiveSecretName,
		},
		Platform: certmanv1alpha1.Platform{},
		DnsNames: []string{
			"api.gibberish.goes.here",
		},
		Email:             "devnull@donot.route",
		ReissueBeforeDays: 10000,
	},
}

var certRequestAWSPlatformSecrets = &certmanv1alpha1.AWSPlatformSecrets{
	Credentials: v1.LocalObjectReference{
		Name: "aws",
	},
	Region: "not-relevant",
}

var certSecret = &v1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: testHiveNamespace,
		Name:      testHiveSecretName,
	},
}

/*
This is a newly-created ES256 elliptic curve key that has only been used for
these tests. It should never be used for anything else.
*/
var leAccountPrivKey = []byte(`-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIKjjz0SZwf3Mpo10i1VXPZPv/8/DCWX0iQ7mBjWhjY6OoAoGCCqGSM49
AwEHoUQDQgAEejflvU67Dt2u8Edg7wmcrG2GCKt7VKRL0Iy9LN8LILmEhCqYaM45
Yiu4AbJf3ISUdPj0QlWOcw0kGEXLC/w2dw==
-----END EC PRIVATE KEY-----
`)

/*
Mock certman-operator/pkg/client/aws
The fake AWS client implements the certman-operator/pkg/clients.Client interface
and just returns successes for everything.
*/
type FakeAWSClient struct {
	route53iface.Route53API
}

func (f FakeAWSClient) GetDNSName() string {
	return "Route53"
}

func (f FakeAWSClient) AnswerDNSChallenge(reqLogger logr.Logger, acmeChallengeToken string, domain string, cr *certmanv1alpha1.CertificateRequest) (string, error) {
	return testHiveACMEDomain, nil
}

func (f FakeAWSClient) DeleteAcmeChallengeResourceRecords(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) error {
	return nil
}

func (f FakeAWSClient) ValidateDNSWriteAccess(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) (bool, error) {
	return true, nil
}

// Return an empty AWS client.
func setUpFakeAWSClient(kubeClient client.Client, platfromSecret certmanv1alpha1.Platform, namespace string) (cClient.Client, error) {
	return FakeAWSClient{}, nil
}

func setUpEmptyTestClient(t *testing.T) (testClient client.Client) {
	t.Helper()

	s := scheme.Scheme
	s.AddKnownTypes(certmanv1alpha1.SchemeGroupVersion, certRequest)

	/*
	  lets-encrypt-account is not an existing secret
	  lets-encrypt-account-production is not an existing secret
	  lets-encrypt-account-staging is not an existing secret
	  aws platform secret is not defined in the cert request
	*/
	objects := []runtime.Object{certRequest, certSecret}

	testClient = fake.NewFakeClientWithScheme(s, objects...)
	return
}

/*
setUpTestClient sets up a test kube client loaded with a specified let's
encrypt account secret or aws platformsecret (in the certificaterequest)

Parameters:
t *testing.T - Testing framework hookup. the argument should always be `t` from
the calling function.
leAccountSecretName string - A string of the name for the let's encrypt account
secret. An empty string will not set up the secret at all.
setAWSPlatformSecret bool - If true, sets up the AWS platform secret in the
certificate request.
*/
func setUpTestClient(t *testing.T, leAccountSecretName string, setAWSPlatformSecret bool) (testClient client.Client) {
	t.Helper()

	if setAWSPlatformSecret {
		certRequest.Spec.Platform.AWS = certRequestAWSPlatformSecrets
	}

	s := scheme.Scheme
	s.AddKnownTypes(certmanv1alpha1.SchemeGroupVersion, certRequest)

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: config.OperatorNamespace,
			Name:      leAccountSecretName,
		},
		Data: map[string][]byte{
			"private-key": leAccountPrivKey,
		},
	}
	objects := []runtime.Object{secret, certRequest, certSecret}

	testClient = fake.NewFakeClientWithScheme(s, objects...)
	return
}

/*
set up a ReconcileCertificateRequest using a provided kube client and use the
rcr to run the Reconcile() loop
*/
func rcrReconcile(t *testing.T, kubeClient client.Client) (result reconcile.Result, err error) {
	rcr := ReconcileCertificateRequest{
		client:        kubeClient,
		clientBuilder: setUpFakeAWSClient,
	}
	request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testHiveNamespace, Name: testHiveCertificateRequestName}}

	result, err = rcr.Reconcile(request)
	return
}
