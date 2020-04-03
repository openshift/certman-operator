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
  "k8s.io/api/core/v1"
  metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
  "k8s.io/client-go/kubernetes/scheme"
  "k8s.io/apimachinery/pkg/runtime"
  "sigs.k8s.io/controller-runtime/pkg/client"
  "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// helpers

var testHiveNamespace = "uhc-doesntexist-123456"
var testHiveCertificateRequestName = "clustername-1313-0-primary-cert-bundle"
var testHiveSecretName = "primary-cert-bundle-secret"
var testHiveACMEDomain = "not.a.valid.tld"

var certRequest = &certmanv1alpha1.CertificateRequest{
  ObjectMeta: metav1.ObjectMeta{
    Namespace: testHiveNamespace,
    Name: testHiveCertificateRequestName,
  },
  Spec: certmanv1alpha1.CertificateRequestSpec{
    ACMEDNSDomain: testHiveACMEDomain,
    CertificateSecret: v1.ObjectReference{
      Kind: "Secret",
      Namespace: testHiveNamespace,
      Name: testHiveSecretName,
    },
    Platform: certRequestPlatform,
    DnsNames: []string{
      "api.gibberish.goes.here",
    },
    Email: "devnull@donot.route",
    ReissueBeforeDays: 10000,
  },
}

var certRequestPlatform = certmanv1alpha1.Platform{
  AWS: &certmanv1alpha1.AWSPlatformSecrets{
    Credentials: v1.LocalObjectReference{
      Name: "aws",
    },
    Region: "not-relevant",
  },
}

var certSecret = &v1.Secret{
  ObjectMeta: metav1.ObjectMeta{
    Namespace: testHiveNamespace,
    Name: testHiveSecretName,
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

func (f FakeAWSClient) AnswerDNSChallenge(reqLogger logr.Logger, acmeChallengeToken string, domain string, cr *certmanv1alpha1.CertificateRequest) (string, error) {
  return testHiveACMEDomain, nil
}

func (f FakeAWSClient) DeleteAcmeChallengeResourceRecords(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) error {
  return nil
}

func (f FakeAWSClient) ValidateDNSWriteAccess(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) (bool, error) {
  return true, nil
}

// Return an emtpy AWS client.
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
  */
  objects := []runtime.Object{certRequest, certSecret}

  testClient = fake.NewFakeClientWithScheme(s, objects...)
  return
}

func setUpTestClient(t *testing.T, accountSecretName string) (testClient client.Client) {
  t.Helper()

  s := scheme.Scheme
  s.AddKnownTypes(certmanv1alpha1.SchemeGroupVersion, certRequest)

  secret := &v1.Secret{
    ObjectMeta: metav1.ObjectMeta{
      Namespace: config.OperatorNamespace,
      Name: accountSecretName,
    },
    Data: map[string][]byte{
      "private-key": leAccountPrivKey,
    },
  }
  objects := []runtime.Object{secret, certRequest, certSecret}

  testClient = fake.NewFakeClientWithScheme(s, objects...)
  return
}
