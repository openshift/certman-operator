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
	"fmt"
	"testing"

	"github.com/go-logr/logr"

	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	certmanv1alpha1 "github.com/openshift/certman-operator/api/v1alpha1"
	"github.com/openshift/certman-operator/pkg/clients/aws/mockroute53"
	cTypes "github.com/openshift/certman-operator/pkg/clients/types"
)

var log = logf.Log.WithName("controller_certificaterequest")

func TestGetDNSName(t *testing.T) {
	t.Run("returns the client type", func(t *testing.T) {
		r53 := &awsClient{
			client: &mockroute53.MockRoute53Client{},
		}
		actualDNS := r53.GetDNSName()

		if actualDNS != "Route53" {
			t.Errorf("GetDNSName(): got %s, expected %s\n", actualDNS, "Route53")
		}
	})
}

func TestNewClient(t *testing.T) {
	t.Run("returns an error if the credentials aren't set", func(t *testing.T) {
		testClient := setUpEmptyTestClient(t)
		reqLogger := log.WithValues("Request.Namespace", testHiveNamespace, "Request.Name", testHiveCertificateRequestName)

		_, actual := NewClient(reqLogger, testClient, testHiveAWSSecretName, testHiveNamespace, testHiveAWSRegion, testHiveClusterDeploymentName)

		if actual == nil {
			t.Error("expected an error when attempting to get missing account secret")
		}
	})

	t.Run("returns a client if the credential is set", func(t *testing.T) {
		testClient := setUpTestClient(t)
		reqLogger := log.WithValues("Request.Namespace", testHiveNamespace, "Request.Name", testHiveCertificateRequestName)

		_, err := NewClient(reqLogger, testClient, testHiveAWSSecretName, testHiveNamespace, testHiveAWSRegion, testHiveClusterDeploymentName)

		if err != nil {
			t.Errorf("unexpected error when creating the client: %q", err)
		}
	})
}

func TestListAllHostedZones(t *testing.T) {
	r53 := &mockroute53.MockRoute53Client{
		ZoneCount: 550,
	}

	hostedZones, err := listAllHostedZones(r53, &route53.ListHostedZonesInput{})
	if err != nil {
		t.Fatalf("TestListAllHostedZones(): unexpected error: %s\n", err)
	}

	replyZoneCount := len(hostedZones)
	if replyZoneCount != r53.ZoneCount {
		t.Errorf("TestListAllHostedZones(): got %d zones, expected %d\n", replyZoneCount, r53.ZoneCount)
	}
}

func TestAnswerDNSChallenge(t *testing.T) {
	tests := []struct {
		Name         string
		TestClient   route53iface.Route53API
		Namespace    string
		ExpectedFQDN string
		ExpectError  bool
	}{
		{
			Name: "returns an fdqn",
			TestClient: &mockroute53.MockRoute53Client{
				ZoneCount: 1,
			},
			Namespace:    testHiveNamespace,
			ExpectedFQDN: fmt.Sprintf("%s.%s", cTypes.AcmeChallengeSubDomain, testHiveACMEDomain),
			ExpectError:  false,
		},
		{
			Name:        "zero dnszones",
			TestClient:  &mockroute53.MockRoute53Client{ZoneCount: 1},
			Namespace:   "ns-with-no-dnszones",
			ExpectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			r53 := &awsClient{
				client: test.TestClient,
			}

			actualFQDN, err := r53.AnswerDNSChallenge(logr.Discard(), "fakechallengetoken", certRequest.Spec.ACMEDNSDomain, certRequest, testHiveACMEDomain)
			if test.ExpectError == (err == nil) {
				t.Errorf("AnswerDNSChallenge() %s: ExpectError: %t, actual error: %s\n", test.Name, test.ExpectError, err)
			}

			if actualFQDN != test.ExpectedFQDN {
				t.Errorf("AnswerDNSChallenge() %s: expected %s, got %s\n", test.Name, test.ExpectedFQDN, actualFQDN)
			}
		})
	}
}

func TestValidateDNSWriteAccess(t *testing.T) {
	tests := []struct {
		Name               string
		TestClient         *mockroute53.MockRoute53Client
		CertificateRequest *certmanv1alpha1.CertificateRequest
		ExpectedResult     bool
		ExpectError        bool
	}{
		{
			Name: "validates write access",
			TestClient: &mockroute53.MockRoute53Client{
				ZoneCount: 1,
			},
			CertificateRequest: certRequest,
			ExpectedResult:     true,
			ExpectError:        false,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			r53 := &awsClient{
				client: test.TestClient,
			}

			actualResult, err := r53.ValidateDNSWriteAccess(logr.Discard(), test.CertificateRequest)
			if test.ExpectError == (err == nil) {
				t.Errorf("ValidateDNSWriteAccess() %s: ExpectError: %t, actual error: %s\n", test.Name, test.ExpectError, err)
			}

			if actualResult != test.ExpectedResult {
				t.Errorf("ValidateDNSWriteAccess() %s: expected %t, got %t\n", test.Name, test.ExpectedResult, actualResult)
			}
		})
	}
}

func TestDeleteAcmeChallengeResourceRecords(t *testing.T) {
	tests := []struct {
		Name               string
		TestClient         *mockroute53.MockRoute53Client
		CertificateRequest *certmanv1alpha1.CertificateRequest
		ExpectError        bool
	}{
		{
			Name: "cleans dns records",
			TestClient: &mockroute53.MockRoute53Client{
				ZoneCount: 1,
			},
			CertificateRequest: certRequest,
			ExpectError:        false,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			r53 := &awsClient{
				client: test.TestClient,
			}

			err := r53.DeleteAcmeChallengeResourceRecords(logr.Discard(), test.CertificateRequest)
			if test.ExpectError == (err == nil) {
				t.Errorf("ValidateDNSWriteAccess() %s: ExpectError: %t, actual error: %s\n", test.Name, test.ExpectError, err)
			}
		})
	}
}

// helpers
var testHiveName = "doesntexist"
var testHiveNamespace = "uhc-doesntexist-123456"
var testHiveCertificateRequestName = "clustername-1313-0-primary-cert-bundle"
var testHiveCertSecretName = "primary-cert-bundle-secret" //#nosec - G101: Potential hardcoded credentials
var testHiveACMEDomain = "name0"
var testHiveAWSSecretName = "aws"
var testHiveAWSRegion = "not-relevant-1"
var testHiveClusterDeploymentName = "test-cluster"
var testDnsZoneID = "hostedzone/id0"

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

var testDnsstatus = &hivev1.AWSDNSZoneStatus{
	ZoneID: &testDnsZoneID,
}

var testDnsZone = &hivev1.DNSZone{
	ObjectMeta: metav1.ObjectMeta{
		Name:      testHiveName,
		Namespace: testHiveNamespace,
	},
	Status: hivev1.DNSZoneStatus{
		AWS: testDnsstatus,
	},
}
var testClusterDeployment = &hivev1.ClusterDeployment{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: testHiveNamespace,
		Name:      testHiveClusterDeploymentName,
	},
}

func setUpEmptyTestClient(t *testing.T) (testClient client.Client) {
	t.Helper()

	s := scheme.Scheme
	s.AddKnownTypes(certmanv1alpha1.GroupVersion, certRequest)

	// aws is not an existing secret
	objects := []runtime.Object{certRequest}
	testClient = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()
	return
}

func setUpTestClient(t *testing.T) (testClient client.Client) {
	t.Helper()

	s := scheme.Scheme
	s.AddKnownTypes(certmanv1alpha1.GroupVersion, certRequest)
	if err := hivev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}

	// aws is not an existing secret
	objects := []runtime.Object{certRequest, awsSecret, testClusterDeployment, testDnsZone}

	testClient = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()
	return
}
