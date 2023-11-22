package client

import (
	"testing"

	"github.com/go-logr/logr"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	certmanv1alpha1 "github.com/openshift/certman-operator/api/v1alpha1"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		Name                string
		Platform            certmanv1alpha1.Platform
		ClusterDeployment   *hivev1.ClusterDeployment
		ExpectError         bool
		ExpectedErrorString string
	}{
		{
			Name: "returns client for AWS",
			Platform: certmanv1alpha1.Platform{
				AWS: &certmanv1alpha1.AWSPlatformSecrets{
					Credentials: corev1.LocalObjectReference{},
				},
			},
			ClusterDeployment: testClusterDeployment,
			ExpectError:       false,
		},
		{
			Name: "returns client for GCP",
			Platform: certmanv1alpha1.Platform{
				GCP: &certmanv1alpha1.GCPPlatformSecrets{
					Credentials: corev1.LocalObjectReference{
						Name: "gcp",
					},
				},
			},
			ClusterDeployment: testClusterDeployment,
			ExpectError:       false,
		},
		{
			Name: "returns client for Azure",
			Platform: certmanv1alpha1.Platform{
				Azure: &certmanv1alpha1.AzurePlatformSecrets{
					Credentials: corev1.LocalObjectReference{
						Name: "azure",
					},
				},
			},
			ClusterDeployment: testClusterDeployment,
			ExpectError:       false,
		},
		{
			Name: "returns mock client",
			Platform: certmanv1alpha1.Platform{
				Mock: &certmanv1alpha1.MockPlatformSecrets{
					AnswerDNSChallengeFQDN:        "a.fully.qualified.domain.name",
					AnswerDNSChallengeErrorString: "",
				},
			},
			ClusterDeployment: testClusterDeployment,
			ExpectError:       false,
		},
		{
			Name:                "error on unsupported platform",
			ClusterDeployment:   &hivev1.ClusterDeployment{},
			ExpectError:         true,
			ExpectedErrorString: "Platform not supported",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			s := scheme.Scheme
			s.AddKnownTypes(hivev1.SchemeGroupVersion, &hivev1.ClusterDeployment{})

			actualClient, err := NewClient(logr.Discard(), fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(test.ClusterDeployment, &testGCPPlatformSecret, &testAzurePlatformSecret).Build(), test.Platform, test.ClusterDeployment.ObjectMeta.Namespace, test.ClusterDeployment.ObjectMeta.Name)
			if err != nil {
				if !test.ExpectError {
					t.Errorf("NewClient() %s: got unexpected error \"%s\"\n", test.Name, err)
				}
			}

			// this is a little redundant, but i'd rather confirm we get an actual client
			if actualClient == nil {
				if !test.ExpectError {
					t.Errorf("NewClient() %s: got nil client\n", test.Name)
				}
			}
		})
	}
}

// utils
var testClusterDeployment = &hivev1.ClusterDeployment{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "fake-clusterdeployment",
		Namespace: "fake-uhc-1234567890",
	},
}

var testGCPPlatformSecret = corev1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "gcp",
		Namespace: "fake-uhc-1234567890",
	},
	Data: map[string][]byte{
		"osServiceAccount.json": []byte(`{"type":"service_account","project_id":"fake_null","private_key_id":"donotusethisprivatekeyidbecauseimadeitup","private_key":"-----BEGIN PRIVATE KEY-----\nasdf\n-----END PRIVATE KEY-----\n","client_email":"not@real.emailaccount","client_id":"123456789012345678901","auth_uri": "https://accounts.google.com/o/oauth2/auth","token_uri": "https://oauth2.googleapis.com/token","auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs","client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/not@real.emailaccount"}`),
	},
}

var testAzurePlatformSecret = corev1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "azure",
		Namespace: "fake-uhc-1234567890",
	},
	Data: map[string][]byte{
		"osServicePrincipal.json": []byte(`{"clientId": "afakeclient","clientSecret":"afakesecret","tenantId":"","subscriptionId":""}`),
	},
}
