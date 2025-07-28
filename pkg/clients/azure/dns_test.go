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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Azure/go-autorest/autorest"
	"github.com/go-logr/logr"
	certmanv1alpha1 "github.com/openshift/certman-operator/api/v1alpha1"
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
					t.Fatal("Expected to get instance of azureClient but got nil")
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
			err:         fmt.Errorf("secret %v doesn't have key osServicePrincipal.json", testHiveAzureSecretName),
		},
		{
			description: "returns an error if secret doesn't have clientId in secret key osServicePrincipal.json",
			secret:      getAzureSecret(secretDataWithoutClientID),
			result:      resultWhenError,
			wantError:   true,
			err:         fmt.Errorf("key: 'osServicePrincipal.json', secret: '%v', namespace: '%v' doesn't have clientId", testHiveAzureSecretName, testHiveNamespace),
		},
		{
			description: "returns an error if secret doesn't have clientSecret in secret key osServicePrincipal.json",
			secret:      getAzureSecret(secretDataWithoutClientSecret),
			result:      resultWhenError,
			wantError:   true,
			err:         fmt.Errorf("key: 'osServicePrincipal.json', secret: '%v', namespace: '%v' doesn't have clientSecret", testHiveAzureSecretName, testHiveNamespace),
		},
		{
			description: "returns an error if secret doesn't have tenantID in secret key osServicePrincipal.json",
			secret:      getAzureSecret(secretDataWithoutTenantID),
			result:      resultWhenError,
			wantError:   true,
			err:         fmt.Errorf("key: 'osServicePrincipal.json', secret: '%v', namespace: '%v' doesn't have tenantId", testHiveAzureSecretName, testHiveNamespace),
		},
		{
			description: "returns an error if secret doesn't have subscriptionID in secret key osServicePrincipal.json",
			secret:      getAzureSecret(secretDataWithoutSubscriptionID),
			result:      resultWhenError,
			wantError:   true,
			err:         fmt.Errorf("key: 'osServicePrincipal.json', secret: '%v', namespace: '%v' doesn't have subscriptionId", testHiveAzureSecretName, testHiveNamespace),
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
var testHiveCertSecretName = "primary-cert-bundle-secret" //#nosec - G101: Potential hardcoded credentials
var testHiveACMEDomain = "not.a.valid.tld"
var testHiveAzureSecretName = "azure"
var testHiveResourceGroupName = "some-resource-group"
var testClientID = "client-id"
var testClientSecret = "client-secret"
var testTenantID = "tenant-id"
var testSubscriptionID = "subscription-id"
var secretDataWithoutClientID = "{\"clientSecret\":\"\", \"tenantId\":\"\", \"subscriptionId\":\"\"}"
var secretDataWithoutClientSecret = "{\"clientId\":\"\", \"tenantId\":\"\", \"subscriptionId\":\"\"}" //#nosec - G101: Potential hardcoded credentials
var secretDataWithoutTenantID = "{\"clientId\":\"\", \"clientSecret\":\"\", \"subscriptionId\":\"\"}" //#nosec - G101: Potential hardcoded credentials
var secretDataWithoutSubscriptionID = "{\"clientId\":\"\", \"clientSecret\":\"\", \"tenantId\":\"\"}" //#nosec - G101: Potential hardcoded credentials
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
	s.AddKnownTypes(certmanv1alpha1.GroupVersion, certRequest)

	objects := []runtime.Object{certRequest}

	if azureSecret != nil {
		objects = append(objects, azureSecret)
	}

	testClient = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()
	return
}

type mockAuthorizer struct{}

func (m *mockAuthorizer) WithAuthorization() autorest.PrepareDecorator {
	return func(p autorest.Preparer) autorest.Preparer {
		return autorest.PreparerFunc(func(r *http.Request) (*http.Request, error) {
			return p.Prepare(r)
		})
	}
}

func TestAnswerDNSChallenge(t *testing.T) {
	tests := []struct {
		name               string
		acmeChallengeToken string
		domain             string
		cr                 *certmanv1alpha1.CertificateRequest
		expectedFqdn       string
		expectError        bool
	}{
		{
			name:               "successful DNS challenge for subdomain",
			acmeChallengeToken: "test-challenge-token-123",
			domain:             "api.example.com",
			cr: &certmanv1alpha1.CertificateRequest{
				ObjectMeta: certRequest.ObjectMeta,
				Spec: certmanv1alpha1.CertificateRequestSpec{
					ACMEDNSDomain: "example.com",
					DnsNames:      []string{"api.example.com"},
					Email:         "test@example.com",
				},
			},
			expectedFqdn: "_acme-challenge.api.example.com",
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")

				// Azure DNS Zone GET
				if r.Method == "GET" && strings.Contains(r.URL.Path, "example.com") {
					w.WriteHeader(http.StatusOK)
					response := `{
						"id": "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.Network/dnsZones/example.com",
						"name": "example.com",
						"type": "Microsoft.Network/dnsZones",
						"location": "global",
						"properties": {
							"maxNumberOfRecordSets": 10000,
							"numberOfRecordSets": 2,
							"nameServers": ["ns1-01.azure-dns.com."]
						}
					}`
					w.Write([]byte(response))
					return
				}

				// Azure DNS Record PUT
				if r.Method == "PUT" {
					w.WriteHeader(http.StatusOK)
					response := `{
						"id": "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.Network/dnsZones/example.com/TXT/_acme-challenge.api",
						"name": "_acme-challenge.api",
						"type": "Microsoft.Network/dnsZones/TXT",
						"etag": "00000000-0000-0000-0000-000000000000",
						"properties": {
							"TTL": 60,
							"TXTRecords": [
								{
									"value": ["` + tt.acmeChallengeToken + `"]
								}
							]
						}
					}`
					w.Write([]byte(response))
					return
				}

				// Return 404 for unmatched requests
				w.WriteHeader(http.StatusNotFound)
				errorResponse := `{
					"error": {
						"code": "NotFound",
						"message": "The requested resource was not found."
					}
				}`
				w.Write([]byte(errorResponse))
			}))
			defer server.Close()

			testClient := setUpTestClient(t, getAzureSecret(validSecretData))
			client, err := NewClient(testClient, testHiveAzureSecretName, testHiveNamespace, testHiveResourceGroupName)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			fmt.Println("Server URL:", server.URL)

			client.zonesClient.BaseURI = server.URL
			client.recordSetsClient.BaseURI = server.URL

			// Use mock authorizer to bypass authentication
			client.zonesClient.Authorizer = &mockAuthorizer{}
			client.recordSetsClient.Authorizer = &mockAuthorizer{}

			fqdn, err := client.AnswerDNSChallenge(logr.Discard(), tt.acmeChallengeToken, tt.domain, tt.cr, tt.cr.Spec.ACMEDNSDomain)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					return
				}
				if fqdn != tt.expectedFqdn {
					t.Errorf("Expected FQDN %s, got %s", tt.expectedFqdn, fqdn)
				}
			}
		})
	}
}

func TestDeleteAcmeChallengeResourceRecords(t *testing.T) {
	tests := []struct {
		name        string
		cr          *certmanv1alpha1.CertificateRequest
		expectError bool
	}{
		{
			name: "successful deletion of DNS challenge records",
			cr: &certmanv1alpha1.CertificateRequest{
				ObjectMeta: certRequest.ObjectMeta,
				Spec: certmanv1alpha1.CertificateRequestSpec{
					ACMEDNSDomain: "example.com",
					DnsNames:      []string{"api.example.com", "www.example.com"},
					Email:         "test@example.com",
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deletedRecords := []string{}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				// Azure DNS Zone GET
				if r.Method == "GET" && strings.Contains(r.URL.Path, "example.com") {
					w.WriteHeader(http.StatusOK)
					response := `{
						"id": "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.Network/dnsZones/example.com",
						"name": "example.com",
						"type": "Microsoft.Network/dnsZones",
						"location": "global",
						"properties": {
							"maxNumberOfRecordSets": 10000,
							"numberOfRecordSets": 2,
							"nameServers": ["ns1-01.azure-dns.com."]
						}
					}`
					w.Write([]byte(response))
					return
				}

				// Azure DNS Record DELETE
				if r.Method == "DELETE" && strings.Contains(r.URL.Path, "/TXT/") {
					pathParts := strings.Split(r.URL.Path, "/")
					if len(pathParts) > 0 {
						recordName := pathParts[len(pathParts)-1]
						deletedRecords = append(deletedRecords, recordName)
					}
					w.WriteHeader(http.StatusOK)
					return
				}

				// Return 404 for unmatched requests
				w.WriteHeader(http.StatusNotFound)
				errorResponse := `{
					"error": {
						"code": "NotFound",
						"message": "The requested resource was not found."
					}
				}`
				w.Write([]byte(errorResponse))
			}))
			defer server.Close()

			testClient := setUpTestClient(t, getAzureSecret(validSecretData))
			client, err := NewClient(testClient, testHiveAzureSecretName, testHiveNamespace, testHiveResourceGroupName)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			client.zonesClient.BaseURI = server.URL
			client.recordSetsClient.BaseURI = server.URL

			// Use mock authorizer to bypass authentication
			client.zonesClient.Authorizer = &mockAuthorizer{}
			client.recordSetsClient.Authorizer = &mockAuthorizer{}

			// Test DeleteAcmeChallengeResourceRecords with mock Azure API
			err = client.DeleteAcmeChallengeResourceRecords(logr.Discard(), tt.cr)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					return
				}

				expectedRecordCount := len(tt.cr.Spec.DnsNames)
				if len(deletedRecords) != expectedRecordCount {
					t.Errorf("Expected %d records to be deleted, but got %d", expectedRecordCount, len(deletedRecords))
				}

				for _, dnsName := range tt.cr.Spec.DnsNames {
					expectedRecordName := client.generateTxtRecordName(dnsName, tt.cr.Spec.ACMEDNSDomain)
					found := false
					for _, deletedRecord := range deletedRecords {
						if deletedRecord == expectedRecordName {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("Expected TXT record %s to be deleted for DNS name %s", expectedRecordName, dnsName)
					}
				}
			}
		})
	}
}

func TestValidateDNSWriteAccess(t *testing.T) {
	tests := []struct {
		name         string
		cr           *certmanv1alpha1.CertificateRequest
		zoneType     string
		expectResult bool
		expectError  bool
		errorMessage string
	}{
		{
			name: "successful validation with public zone",
			cr: &certmanv1alpha1.CertificateRequest{
				ObjectMeta: certRequest.ObjectMeta,
				Spec: certmanv1alpha1.CertificateRequestSpec{
					ACMEDNSDomain: "example.com",
					DnsNames:      []string{"api.example.com"},
					Email:         "test@example.com",
				},
			},
			zoneType:     "Public",
			expectResult: true,
			expectError:  false,
		},
		{
			name: "rejection of private zone",
			cr: &certmanv1alpha1.CertificateRequest{
				ObjectMeta: certRequest.ObjectMeta,
				Spec: certmanv1alpha1.CertificateRequestSpec{
					ACMEDNSDomain: "private.example.com",
					DnsNames:      []string{"internal.private.example.com"},
					Email:         "test@example.com",
				},
			},
			zoneType:     "Private",
			expectResult: false,
			expectError:  false, // No error, just returns false
		},
		{
			name: "zone not found error",
			cr: &certmanv1alpha1.CertificateRequest{
				ObjectMeta: certRequest.ObjectMeta,
				Spec: certmanv1alpha1.CertificateRequestSpec{
					ACMEDNSDomain: "nonexistent.example.com",
					DnsNames:      []string{"api.nonexistent.example.com"},
					Email:         "test@example.com",
				},
			},
			zoneType:     "",
			expectResult: false,
			expectError:  true,
			errorMessage: "NotFound",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var createdRecord string
			var deletedRecord string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")

				if tt.expectError && strings.Contains(tt.errorMessage, "NotFound") {
					w.WriteHeader(http.StatusNotFound)
					errorResponse := `{
						"error": {
							"code": "NotFound",
							"message": "The requested DNS zone was not found."
						}
					}`
					w.Write([]byte(errorResponse))
					return
				}

				// Azure DNS Zone GET
				if r.Method == "GET" && strings.Contains(r.URL.Path, tt.cr.Spec.ACMEDNSDomain) {
					w.WriteHeader(http.StatusOK)
					response := fmt.Sprintf(`{
						"id": "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.Network/dnsZones/%s",
						"name": "%s",
						"type": "Microsoft.Network/dnsZones",
						"location": "global",
						"properties": {
							"zoneType": "%s",
							"maxNumberOfRecordSets": 10000,
							"numberOfRecordSets": 2,
							"nameServers": ["ns1-01.azure-dns.com."]
						}
					}`, tt.cr.Spec.ACMEDNSDomain, tt.cr.Spec.ACMEDNSDomain, tt.zoneType)
					w.Write([]byte(response))
					return
				}

				// Azure DNS Record PUT (for test record creation)
				if r.Method == "PUT" && strings.Contains(r.URL.Path, "/TXT/") {
					// Extract record name from path
					pathParts := strings.Split(r.URL.Path, "/")
					if len(pathParts) > 0 {
						recordName := pathParts[len(pathParts)-1]
						createdRecord = recordName
					}

					w.WriteHeader(http.StatusOK)
					response := `{
						"id": "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.Network/dnsZones/example.com/TXT/_certman_access_test.example.com",
						"name": "_certman_access_test.example.com",
						"type": "Microsoft.Network/dnsZones/TXT",
						"etag": "00000000-0000-0000-0000-000000000000",
						"properties": {
							"TTL": 60,
							"TXTRecords": [
								{
									"value": ["txt_entry"]
								}
							]
						}
					}`
					w.Write([]byte(response))
					return
				}

				// Azure DNS Record DELETE (for test record cleanup)
				if r.Method == "DELETE" && strings.Contains(r.URL.Path, "/TXT/") {
					pathParts := strings.Split(r.URL.Path, "/")
					if len(pathParts) > 0 {
						recordName := pathParts[len(pathParts)-1]
						deletedRecord = recordName
					}

					w.WriteHeader(http.StatusOK)
					// Azure DELETE returns empty response with 200 status
					return
				}

				// Return 404 for unmatched requests
				w.WriteHeader(http.StatusNotFound)
				errorResponse := `{
					"error": {
						"code": "NotFound",
						"message": "The requested resource was not found."
					}
				}`
				w.Write([]byte(errorResponse))
			}))
			defer server.Close()

			// Create Azure client
			testClient := setUpTestClient(t, getAzureSecret(validSecretData))
			client, err := NewClient(testClient, testHiveAzureSecretName, testHiveNamespace, testHiveResourceGroupName)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			client.zonesClient.BaseURI = server.URL
			client.recordSetsClient.BaseURI = server.URL

			// Use mock authorizer to bypass authentication
			client.zonesClient.Authorizer = &mockAuthorizer{}
			client.recordSetsClient.Authorizer = &mockAuthorizer{}

			result, err := client.ValidateDNSWriteAccess(logr.Discard(), tt.cr)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errorMessage) {
					t.Errorf("Expected error containing %q, got: %v", tt.errorMessage, err)
				}
				if result {
					t.Errorf("Expected result to be false when error occurs, got true")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					return
				}
				if result != tt.expectResult {
					t.Errorf("Expected result %v, got %v", tt.expectResult, result)
				}

				if tt.expectResult {
					expectedRecordName := "_certman_access_test." + tt.cr.Spec.ACMEDNSDomain
					if createdRecord != expectedRecordName {
						t.Errorf("Expected test record %q to be created, got %q", expectedRecordName, createdRecord)
					}
					if deletedRecord != expectedRecordName {
						t.Errorf("Expected test record %q to be deleted, got %q", expectedRecordName, deletedRecord)
					}
				}

				if !tt.expectResult && tt.zoneType == "Private" {
					if createdRecord != "" {
						t.Errorf("Expected no record creation for private zone, but %q was created", createdRecord)
					}
					if deletedRecord != "" {
						t.Errorf("Expected no record deletion for private zone, but %q was deleted", deletedRecord)
					}
				}
			}
		})
	}
}
