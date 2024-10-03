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
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/service/route53/route53iface"
	logr "github.com/go-logr/logr"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	certmanv1alpha1 "github.com/openshift/certman-operator/api/v1alpha1"
	"github.com/openshift/certman-operator/config"
	cClient "github.com/openshift/certman-operator/pkg/clients"
)

// helpers
var testHiveNamespace = "uhc-doesntexist-123456"
var testHiveClusterDeploymentName = "clustername-1313"
var testHiveClusterDeploymentUID = types.UID("some-fake-uid")
var testHiveCertificateRequestName = fmt.Sprintf("%s-primary-cert-bundle", testHiveClusterDeploymentName)
var testHiveSecretName = "primary-cert-bundle-secret" //#nosec - G101: Potential hardcoded credentials
var testHiveACMEDomain = "not.a.valid.tld"
var testHiveFedRampZoneID = "Z10091REDACTEDW6I"

var clusterDeploymentIncoming = &hivev1.ClusterDeployment{
	TypeMeta: metav1.TypeMeta{
		Kind:       "ClusterDeployment",
		APIVersion: "hive.openshift.io/v1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Namespace: testHiveNamespace,
		Name:      testHiveClusterDeploymentName,
		Annotations: map[string]string{
			"hive.openshift.io/relocate": "newhive/incoming",
		},
	},
}
var clusterDeploymentComplete = &hivev1.ClusterDeployment{
	TypeMeta: metav1.TypeMeta{
		Kind:       "ClusterDeployment",
		APIVersion: "hive.openshift.io/v1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Namespace: testHiveNamespace,
		Name:      testHiveClusterDeploymentName,
		Annotations: map[string]string{
			"hive.openshift.io/relocate": "newhive/complete",
		},
		UID: testHiveClusterDeploymentUID,
	},
}
var clusterDeploymentOutgoing = &hivev1.ClusterDeployment{
	TypeMeta: metav1.TypeMeta{
		Kind:       "ClusterDeployment",
		APIVersion: "hive.openshift.io/v1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Namespace: testHiveNamespace,
		Name:      testHiveClusterDeploymentName,
		Annotations: map[string]string{
			"hive.openshift.io/relocate": "newhive/outgoing",
		},
	},
}

var certRequest = &certmanv1alpha1.CertificateRequest{
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
				Name:               testHiveClusterDeploymentName,
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
	Status: certmanv1alpha1.CertificateRequestStatus{},
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

var expiredCertSecret = &corev1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: testHiveNamespace,
		Name:      testHiveSecretName,
	},
	Data: map[string][]byte{
		// this is an absolutely garbage self-signed cert. it should not be used for
		// anything ever
		corev1.TLSCertKey: []byte(`-----BEGIN CERTIFICATE-----
MIICzjCCAjegAwIBAgIUKXy4rExkr0YPDhITDqpiG8azJf4wDQYJKoZIhvcNAQEL
BQAwZzELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM
GEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDEgMB4GA1UEAwwXYXBpLmdpYmJlcmlz
aC5nb2VzLmhlcmUwHhcNMjEwMjE5MjIwMDA3WhcNMjEwMjIwMjIwMDA3WjBnMQsw
CQYDVQQGEwJBVTETMBEGA1UECAwKU29tZS1TdGF0ZTEhMB8GA1UECgwYSW50ZXJu
ZXQgV2lkZ2l0cyBQdHkgTHRkMSAwHgYDVQQDDBdhcGkuZ2liYmVyaXNoLmdvZXMu
aGVyZTCBnzANBgkqhkiG9w0BAQEFAAOBjQAwgYkCgYEA90KZ23CieKQ2p1CCkdGG
VBdfBjsnXmXOWZjbBrmv6XdED+MVXTY/Dhbt+84M+BhPAMoTG38aeeOQQnImWh2i
x0PnZi4+p2H2jbzfnu26geHz7/b1go9lyvJ2+zEl4TovMUFetfs+ufpxU6STjIdA
8W6g2H57ECMAtIFllkxWw1ECAwEAAaN3MHUwHQYDVR0OBBYEFI6Qu/Ym01edH/HM
9JGpV7FeddXVMB8GA1UdIwQYMBaAFI6Qu/Ym01edH/HM9JGpV7FeddXVMA8GA1Ud
EwEB/wQFMAMBAf8wIgYDVR0RBBswGYIXYXBpLmdpYmJlcmlzaC5nb2VzLmhlcmUw
DQYJKoZIhvcNAQELBQADgYEAbUH1aXUae9CyuqFwqmEE+OY8vKxlqEnAyQ6p67UL
7zx9n/on4lXcifHzaYDAlAYU66rYiZREcGkeH97J+QXcgv2pSqgvbI/4++AsyXDq
UdlLtE44bt04ZOkcS2n9ofSjl0iv8fsxkOl0i6NsWXpqm8aGAZDpVFKNShPsQ0rb
cxA=
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
var testLESecret = &corev1.Secret{
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

func (f FakeAWSClient) GetFedrampHostedZoneIDPath(fedrampHostedZoneID string) (string, error) {
	return testHiveFedRampZoneID, nil
}

func (f FakeAWSClient) AnswerDNSChallenge(reqLogger logr.Logger, acmeChallengeToken string, domain string, cr *certmanv1alpha1.CertificateRequest, dnsZone string) (string, error) {
	return testHiveACMEDomain, nil
}

func (f FakeAWSClient) DeleteAcmeChallengeResourceRecords(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) error {
	return nil
}

func (f FakeAWSClient) ValidateDNSWriteAccess(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) (bool, error) {
	return true, nil
}

// Return an empty AWS client.
func setUpFakeAWSClient(reqLogger logr.Logger, kubeClient client.Client, platfromSecret certmanv1alpha1.Platform, namespace string, clusterDeplymentName string) (cClient.Client, error) {
	return FakeAWSClient{}, nil
}

// setUpTestClient sets up a test kube client loaded with the provided cloud
// account secret and runtime objects (certificaterequest, secret, etc)
func setUpTestClient(t *testing.T, objects []runtime.Object) client.Client {
	t.Helper()

	s := scheme.Scheme
	s.AddKnownTypes(certmanv1alpha1.GroupVersion, certRequest)
	s.AddKnownTypes(hivev1.SchemeGroupVersion, clusterDeploymentComplete)
	s.AddKnownTypes(hivev1.SchemeGroupVersion, &hivev1.DNSZoneList{})
	s.AddKnownTypes(hivev1.SchemeGroupVersion, &hivev1.DNSZone{})
	return fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).WithStatusSubresource(certRequest).Build()
}
