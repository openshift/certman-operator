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
	"os"
	"strings"
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
	corev1 "k8s.io/api/core/v1"
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

func TestGetFedrampHostedZoneIDPath(t *testing.T) {
	tests := []struct {
		testFedRampHostedZoneID     string
		ExpectError                 bool
		Name                        string
		ExpectedFedRampHostedZoneID string
		fedramp                     bool
	}{
		{
			Name:                        "returns a FedRamp Hosted Zone ID",
			testFedRampHostedZoneID:     "Z10091REDACTEDW6I",
			ExpectError:                 false,
			ExpectedFedRampHostedZoneID: "Z10091REDACTEDW6I",
			fedramp:                     true,
		},
	}
	for _, test := range tests {
		t.Run("returns the client type", func(t *testing.T) {
			// r53AWS := &awsClient{
			// 	client: &mockroute53.MockRoute53Client{},
			// }
			r53 := &mockroute53.MockRoute53Client{}
			FedrampHostedZoneIDErrorString, err := r53.GetFedrampHostedZoneIDPath(test.testFedRampHostedZoneID)

			if FedrampHostedZoneIDErrorString != test.ExpectedFedRampHostedZoneID {
				t.Errorf("FedrampHostedZoneIDErrorString(): got %s, expected %s\n", FedrampHostedZoneIDErrorString, test.ExpectedFedRampHostedZoneID)
			}
			if FedrampHostedZoneIDErrorString == "" {
				t.Errorf("FedrampHostedZoneIDErrorString is empty: %q", err)
			}
			if err != nil {
				t.Errorf("unexpected error when creating returning the FedrampHostedZoneID: %q", err)
			}

		})
	}
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
func TestNewClient_Fedramp(t *testing.T) {

	defer func() {
		_ = os.Unsetenv(fedrampEnvVariable)
		fedramp = os.Getenv(fedrampEnvVariable) == "true"
	}()
	err := os.Setenv(fedrampEnvVariable, "true")
	if err != nil {
		t.Fatalf("Error setting env var: %v", err)
	}
	fedramp = os.Getenv(fedrampEnvVariable) == "true"

	tests := []struct {
		name          string
		secret        *corev1.Secret
		expectError   bool
		errorContains string
	}{
		{
			name: "success with full secret",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "certman-operator-aws-credentials",
					Namespace: "certman-operator",
				},
				Data: map[string][]byte{
					"aws_access_key_id":     []byte("test-access-key"),
					"aws_secret_access_key": []byte("test-secret-key"),
				},
			},
			expectError: false,
		},
		{
			name:          "secret missing",
			secret:        nil,
			expectError:   true,
			errorContains: "not found",
		},
		{
			name: "secret missing access key",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "certman-operator-aws-credentials",
					Namespace: "certman-operator",
				},
				Data: map[string][]byte{
					// "aws_access_key_id" missing
					"aws_secret_access_key": []byte("test-secret-key"),
				},
			},
			expectError:   true,
			errorContains: "did not contain key aws_access_key_id",
		},
		{
			name: "secret missing secret key",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "certman-operator-aws-credentials",
					Namespace: "certman-operator",
				},
				Data: map[string][]byte{
					"aws_access_key_id": []byte("test-access-key"),
					// "aws_secret_access_key" missing
				},
			},
			expectError:   true,
			errorContains: "did not contain key aws_secret_access_key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := corev1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			var kubeClient client.Client
			if tt.secret != nil {
				kubeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.secret).Build()
			} else {
				kubeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			}

			c, err := NewClient(logr.Discard(), kubeClient, "certman-operator-aws-credentials", "", "us-gov-west-1", "")
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain %q, got %v", tt.errorContains, err)
				}
				if c != nil {
					t.Errorf("expected client to be nil on error, got non-nil client")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if c == nil {
					t.Fatalf("expected a valid client but got nil")
				}
			}
		})
	}
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
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			r53 := &awsClient{
				client: test.TestClient,
			}

			actualFQDN, err := r53.AnswerDNSChallenge(logr.Discard(), "fakechallengetoken", certRequest.Spec.ACMEDNSDomain, certRequest, "id0")
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
		isFedramp          bool
	}{
		{
			Name: "validates write access",
			TestClient: &mockroute53.MockRoute53Client{
				ZoneCount: 1,
			},
			CertificateRequest: certRequest,
			ExpectedResult:     true,
			ExpectError:        false,
			isFedramp:          false,
		},
		{
			Name: "validates write access fedramp true",
			TestClient: &mockroute53.MockRoute53Client{
				ZoneCount: 1,
			},
			CertificateRequest: certRequest,
			ExpectedResult:     true,
			ExpectError:        false,
			isFedramp:          true,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			r53 := &awsClient{
				client: test.TestClient,
			}
			if test.isFedramp {
				defer func() {
					err := os.Unsetenv(fedrampEnvVariable)
					if err != nil {
						t.Fatalf("Error unsetting environment variable %s: %s", fedrampEnvVariable, err)
					}
					err = os.Unsetenv(fedrampHostedZoneIDVariable)
					if err != nil {
						t.Fatalf("Error unsetting environment variable %s: %s", fedrampHostedZoneIDVariable, err)
					}
					//reload again after completing the test
					fedrampHostedZoneID = os.Getenv(fedrampHostedZoneIDVariable)
					fedramp = os.Getenv(fedrampEnvVariable) == "true"
				}()

				err := os.Setenv(fedrampEnvVariable, "true")
				if err != nil {
					t.Fatalf("Error unsetting environment variable %s: %s", fedrampEnvVariable, err)
				}
				err = os.Setenv(fedrampHostedZoneIDVariable, "id33")
				if err != nil {
					t.Fatalf("Error unsetting environment variable %s: %s", fedrampHostedZoneIDVariable, err)
				}
				// reload
				fedrampHostedZoneID = os.Getenv(fedrampHostedZoneIDVariable)
				fedramp = os.Getenv(fedrampEnvVariable) == "true"
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
