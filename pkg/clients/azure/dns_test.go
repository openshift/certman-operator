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

package azure

import (
	"fmt"
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
	clientTests := []struct {
		description string
		secret      *v1.Secret
		wantError   bool
		err         error
	}{
		{
			description: "returns an error if the credentials aren't set",
			secret:      nil,
			wantError:   true,
			err:         fmt.Errorf("secrets \"%v\" not found", testHiveAzureSecretName),
		},
		{
			description: "returns a client if the credentials is set",
			secret:      getAzureSecret(validSecretData),
			wantError:   false,
			err:         nil,
		},
	}
	for _, tt := range clientTests {
		t.Run(tt.description, func(t *testing.T) {
			testClient := setUpTestClient(t, tt.secret)

			client, err := NewClient(testClient, testHiveAzureSecretName, testHiveNamespace, testHiveResourceGroupName)

			if tt.wantError {
				if err == nil || tt.err.Error() != err.Error() {
					t.Errorf("Expected error: %v but got: %v", tt.err, err)
				}
			} else {
				if client == nil {
					t.Error("Expected to get instance of azureClient but got nil")
				}
				if client.resourceGroupName != testHiveResourceGroupName {
					t.Errorf("Expected client resourceGroupName to be: %v but got: %v", testHiveResourceGroupName, client.resourceGroupName)
				}
				if client.recordSetsClient == nil {
					t.Error("Expected recordSetsClient to be set but got nil")
				}
				if client.zonesClient == nil {
					t.Error("Expected zonesClient to be set but got nil")
				}
			}
		})
	}
}

func TestGetAzureCredentialsFromSecret(t *testing.T) {

	resultWhenError := map[string]string{
		"clientID":       "",
		"clientSecret":   "",
		"tenantID":       "",
		"subscriptionID": "",
	}
	resultWhenSuccess := map[string]string{
		"clientID":       testClientID,
		"clientSecret":   testClientSecret,
		"tenantID":       testTenantID,
		"subscriptionID": testSubscriptionID,
	}

	credentialTests := []struct {
		description string
		secret      *v1.Secret
		result      map[string]string
		wantError   bool
		err         error
	}{
		{
			description: "returns an error if secret doesn't have correct key",
			secret:      getAzureSecret(""),
			result:      resultWhenError,
			wantError:   true,
			err:         fmt.Errorf("Secret %v doesn't have key osServicePrincipal.json", testHiveAzureSecretName),
		},
		{
			description: "returns an error if secret doesn't have clientId in secret key osServicePrincipal.json",
			secret:      getAzureSecret(secretDataWithoutClientID),
			result:      resultWhenError,
			wantError:   true,
			err:         fmt.Errorf("Key: 'osServicePrincipal.json', secret: '%v', namespace: '%v' doesn't have clientId", testHiveAzureSecretName, testHiveNamespace),
		},
		{
			description: "returns an error if secret doesn't have clientSecret in secret key osServicePrincipal.json",
			secret:      getAzureSecret(secretDataWithoutClientSecret),
			result:      resultWhenError,
			wantError:   true,
			err:         fmt.Errorf("Key: 'osServicePrincipal.json', secret: '%v', namespace: '%v' doesn't have clientSecret", testHiveAzureSecretName, testHiveNamespace),
		},
		{
			description: "returns an error if secret doesn't have tenantID in secret key osServicePrincipal.json",
			secret:      getAzureSecret(secretDataWithoutTenantID),
			result:      resultWhenError,
			wantError:   true,
			err:         fmt.Errorf("Key: 'osServicePrincipal.json', secret: '%v', namespace: '%v' doesn't have tenantId", testHiveAzureSecretName, testHiveNamespace),
		},
		{
			description: "returns an error if secret doesn't have subscriptionID in secret key osServicePrincipal.json",
			secret:      getAzureSecret(secretDataWithoutSubscriptionID),
			result:      resultWhenError,
			wantError:   true,
			err:         fmt.Errorf("Key: 'osServicePrincipal.json', secret: '%v', namespace: '%v' doesn't have subscriptionId", testHiveAzureSecretName, testHiveNamespace),
		},
		{
			description: "returns credentials when correct secret is provided",
			secret:      getAzureSecret(validSecretData),
			result:      resultWhenSuccess,
			wantError:   false,
			err:         nil,
		},
	}

	for _, tt := range credentialTests {

		t.Run(tt.description, func(t *testing.T) {

			clientID, clientSecret, tenantID, subscriptionID, err := getAzureCredentialsFromSecret(*tt.secret)

			if tt.result["clientID"] != clientID {
				t.Errorf("Unexpected clientId when parsing azure credentials:\n Expected:%v\n Got:%v", tt.result["clientID"], clientID)
			}

			if tt.result["clientSecret"] != clientSecret {
				t.Errorf("Unexpected clientSecret when parsing azure credentials:\n Expected:%v\n Got:%v", tt.result["clientSecret"], clientSecret)
			}

			if tt.result["tenantID"] != tenantID {
				t.Errorf("Unexpected tenantID when parsing azure credentials:\n Expected:%v\n Got:%v", tt.result["tenantID"], tenantID)
			}

			if tt.result["subscriptionID"] != subscriptionID {
				t.Errorf("Unexpected subscriptionID when parsing azure credentials:\n Expected:%v\n Got:%v", tt.result["subscriptionID"], subscriptionID)
			}

			if tt.wantError && (err == nil || tt.err.Error() != err.Error()) {
				t.Errorf("Unexpected error when creating the client:\n Expected:%v\n Got:%v", tt.err, err)
			}
		})
	}
}

// helpers
var testHiveNamespace = "uhc-doesntexist-123456"
var testHiveCertificateRequestName = "clustername-1313-0-primary-cert-bundle"
var testHiveCertSecretName = "primary-cert-bundle-secret"
var testHiveACMEDomain = "not.a.valid.tld"
var testHiveAzureSecretName = "azure"
var testHiveResourceGroupName = "some-resource-group"
var testClientID = "client-id"
var testClientSecret = "client-secret"
var testTenantID = "tenant-id"
var testSubscriptionID = "subscription-id"
var secretDataWithoutClientID = "{\"clientSecret\":\"\", \"tenantId\":\"\", \"subscriptionId\":\"\"}"
var secretDataWithoutClientSecret = "{\"clientId\":\"\", \"tenantId\":\"\", \"subscriptionId\":\"\"}"
var secretDataWithoutTenantID = "{\"clientId\":\"\", \"clientSecret\":\"\", \"subscriptionId\":\"\"}"
var secretDataWithoutSubscriptionID = "{\"clientId\":\"\", \"clientSecret\":\"\", \"tenantId\":\"\"}"
var validSecretData = "{" +
	"\"clientId\":\"" + testClientID + "\"," +
	"\"clientSecret\":\"" + testClientSecret + "\"," +
	"\"tenantId\":\"" + testTenantID + "\"," +
	"\"subscriptionId\":\"" + testSubscriptionID + "\"" +
	"}"

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
	Azure: &certmanv1alpha1.AzurePlatformSecrets{
		Credentials: v1.LocalObjectReference{
			Name: testHiveAzureSecretName,
		},
		ResourceGroupName: testHiveResourceGroupName,
	},
}

var certSecret = &v1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: testHiveNamespace,
		Name:      testHiveCertSecretName,
	},
}

func getAzureSecret(credsData string) *v1.Secret {
	data := map[string][]byte{}
	if len(credsData) > 0 {
		data = map[string][]byte{
			"osServicePrincipal.json": []byte(credsData),
		}
	}

	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testHiveNamespace,
			Name:      testHiveAzureSecretName,
		},
		Data: data,
	}
}

func setUpTestClient(t *testing.T, azureSecret *v1.Secret) (testClient client.Client) {
	t.Helper()

	s := scheme.Scheme
	s.AddKnownTypes(certmanv1alpha1.SchemeGroupVersion, certRequest)

	objects := []runtime.Object{certRequest}

	if azureSecret != nil {
		objects = append(objects, azureSecret)
	}

	testClient = fake.NewFakeClientWithScheme(s, objects...)
	return
}
