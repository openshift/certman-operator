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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// helpers

var testHiveNamespace = "uhc-doesntexist-123456"
var testHiveCertificateRequestName = "clustername-1313-0-primary-cert-bundle"
var testHiveSecretName = "primary-cert-bundle-secret"
var testHiveACMEDomain = "not.a.valid.tld"

var certRequest = &certmanv1alpha1.CertificateRequest{
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
	Status: certmanv1alpha1.CertificateRequestStatus{},
}

var certRequestAWSPlatformSecrets = &certmanv1alpha1.AWSPlatformSecrets{
	Credentials: corev1.LocalObjectReference{
		Name: "aws",
	},
	Region: "not-relevant",
}

var emptyCertSecret = &corev1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: testHiveNamespace,
		Name:      testHiveSecretName,
	},
}

var validCertSecret = &corev1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: testHiveNamespace,
		Name:      testHiveSecretName,
	},
	Data: map[string][]byte{
		// this is an absolutely garbage self-signed cert. it should not be used for
		// anything ever
		corev1.TLSCertKey: []byte(`-----BEGIN CERTIFICATE-----
MIIC2DCCAkGgAwIBAgIUH0hB45DuH9g3KyLn+Vaip0tTFRMwDQYJKoZIhvcNAQEL
BQAwazELMAkGA1UEBhMCVVMxFzAVBgNVBAgMDk5vcnRoIENhcm9saW5hMSEwHwYD
VQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQxIDAeBgNVBAMMF2FwaS5naWJi
ZXJpc2guZ29lcy5oZXJlMCAXDTIxMDIyMzIxMzEwOFoYDzIxMjEwMTMwMjEzMTA4
WjBrMQswCQYDVQQGEwJVUzEXMBUGA1UECAwOTm9ydGggQ2Fyb2xpbmExITAfBgNV
BAoMGEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDEgMB4GA1UEAwwXYXBpLmdpYmJl
cmlzaC5nb2VzLmhlcmUwgZ8wDQYJKoZIhvcNAQEBBQADgY0AMIGJAoGBALoL1zJb
vIyORwmGXQnViUQU8ZfJIEP0yp/V7wh/iS6l8VTZkTWfhMdNJcFxhZ7ZCg16e1gy
InuOGFJzoAZt9iydQ56CmNjCZ4W3F5vbyS28wxDeOf3ReCBpePN2JaXmyeoMTtrC
pe5X9WDGM058bJjZj+eRIwvRFwd5vOE7DX/hAgMBAAGjdzB1MB0GA1UdDgQWBBSQ
nk9x0PpBkPvIJPofngFlDmUQfjAfBgNVHSMEGDAWgBSQnk9x0PpBkPvIJPofngFl
DmUQfjAPBgNVHRMBAf8EBTADAQH/MCIGA1UdEQQbMBmCF2FwaS5naWJiZXJpc2gu
Z29lcy5oZXJlMA0GCSqGSIb3DQEBCwUAA4GBAI9pcwgyuy7bWn6E7GXALwvA/ba5
8Rjjs000wrPpSHJpaIwxp8BNVkCwADewF3RUZR4qh0hicOduOIbDpsRQbuIHBR9o
BNfwM5mTnLOijduGlf52SqIW8l35OjtiBvzSVXoroXdvKxC35xTuwJ+Q5GGynVDs
VoZplnP9BdVECzSa
-----END CERTIFICATE-----`),
		corev1.TLSPrivateKeyKey: []byte(""), // this should be irrelevant for testing at least for now
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

// mock secrets for letsencrypt accounts. these use the above ES236 key and
// should not be used for anything else
var testStagingLESecret = &corev1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: config.OperatorNamespace,
		Name:      "lets-encrypt-account-staging",
	},
	Data: map[string][]byte{
		"private-key": leAccountPrivKey,
	},
}

var testProductionLESecret = &corev1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: config.OperatorNamespace,
		Name:      "lets-encrypt-account-production",
	},
	Data: map[string][]byte{
		"private-key": leAccountPrivKey,
	},
}

var testDeprecatedLESecret = &corev1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: config.OperatorNamespace,
		Name:      "lets-encrypt-account",
	},
	Data: map[string][]byte{
		"private-key": leAccountPrivKey,
	},
}

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

// setUpTestClient sets up a test kube client loaded with the provided cloud
// account secret and runtime objects (certificaterequest, secret, etc)
func setUpTestClient(t *testing.T, objects []runtime.Object) client.Client {
	t.Helper()

	s := scheme.Scheme
	s.AddKnownTypes(certmanv1alpha1.SchemeGroupVersion, certRequest)

	return fake.NewFakeClientWithScheme(s, objects...)
}
