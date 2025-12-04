package utils

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	syaml "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	logs "sigs.k8s.io/controller-runtime/pkg/log"
)

type CertConfig struct {
	ClusterName   string
	BaseDomain    string
	TestNamespace string
	OCMClusterID  string
}

// ExtractClusterInfoFromInfrastructure extracts cluster name and base domain from the infrastructure cluster resource
// This replicates: oc get infrastructure cluster -o jsonpath='{.status.apiServerURL}' | sed 's|https://api\.\(.*\):6443|\1|'
// Returns clusterName and baseDomain parsed from the apiServerURL
// Example: if apiServerURL is "https://api.sai-test.fvj1.s1.devshift.org:6443"
//   - fullDomain = "sai-test.fvj1.s1.devshift.org"
//   - clusterName = "sai-test"
//   - baseDomain = "fvj1.s1.devshift.org"
func ExtractClusterInfoFromInfrastructure(ctx context.Context, dynamicClient dynamic.Interface) (clusterName, baseDomain string, err error) {
	// Infrastructure is a cluster-scoped resource in config.openshift.io/v1
	infrastructureGVR := schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "infrastructures",
	}

	// Get the infrastructure cluster resource
	infra, err := dynamicClient.Resource(infrastructureGVR).Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return "", "", fmt.Errorf("failed to get infrastructure cluster: %w", err)
	}

	// Extract apiServerURL from status
	apiServerURL, found, err := unstructured.NestedString(infra.Object, "status", "apiServerURL")
	if !found || err != nil {
		return "", "", fmt.Errorf("failed to get apiServerURL from infrastructure status: %w", err)
	}

	// Parse apiServerURL: "https://api.sai-test.fvj1.s1.devshift.org:6443"
	// Extract: "sai-test.fvj1.s1.devshift.org"
	// Pattern: https://api.{clusterName}.{baseDomain}:6443
	// We need to extract the part between "https://api." and ":6443"
	if !strings.HasPrefix(apiServerURL, "https://api.") {
		return "", "", fmt.Errorf("unexpected apiServerURL format: %s", apiServerURL)
	}

	// Remove "https://api." prefix
	fullDomain := strings.TrimPrefix(apiServerURL, "https://api.")
	// Remove ":6443" suffix if present
	fullDomain = strings.TrimSuffix(fullDomain, ":6443")

	// Split by first dot to get clusterName and baseDomain
	// fullDomain = "sai-test.fvj1.s1.devshift.org"
	// parts[0] = "sai-test" (clusterName)
	// parts[1:] = ["fvj1", "s1", "devshift", "org"] -> join with "." = "fvj1.s1.devshift.org" (baseDomain)
	parts := strings.SplitN(fullDomain, ".", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unexpected domain format: %s (expected clusterName.baseDomain)", fullDomain)
	}

	clusterName = parts[0]
	baseDomain = parts[1]

	return clusterName, baseDomain, nil
}

func LoadTestConfig() *CertConfig {
	clusterName := GetEnvOrDefault("CLUSTER_NAME", "test-cluster")
	baseDomain := GetEnvOrDefault("BASE_DOMAIN", "example.com")
	ocmClusterID := GetEnvOrDefault("OCM_CLUSTER_ID", "test-cluster-id")

	return NewCertConfig(clusterName, ocmClusterID, baseDomain)
}

// LoadTestConfigFromInfrastructure loads cluster configuration by extracting it from the infrastructure cluster resource
// This is the preferred method as it uses the actual cluster information
func LoadTestConfigFromInfrastructure(ctx context.Context, dynamicClient dynamic.Interface) (*CertConfig, error) {
	clusterName, baseDomain, err := ExtractClusterInfoFromInfrastructure(ctx, dynamicClient)
	if err != nil {
		return nil, fmt.Errorf("failed to extract cluster info from infrastructure: %w", err)
	}

	ocmClusterID := GetEnvOrDefault("OCM_CLUSTER_ID", "test-cluster-id")

	return NewCertConfig(clusterName, ocmClusterID, baseDomain), nil
}

func NewCertConfig(clusterName string, ocmClusterID string, baseDomain string) *CertConfig {
	return &CertConfig{
		ClusterName:   clusterName,
		BaseDomain:    baseDomain,
		TestNamespace: "certman-operator",
		OCMClusterID:  ocmClusterID,
	}
}

func CreateAdminKubeconfigSecret(ctx context.Context, clientset *kubernetes.Clientset, config *CertConfig, secretName string) error {
	dummyKubeconfig := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: <REDACTED-TEST-CA-CERT>
    server: https://api.%s.%s:6443
  name: %s
contexts:
- context:
    cluster: %s
    user: system:admin
  name: %s-admin
current-context: %s-admin
users:
- name: system:admin
  user:
    client-certificate-data: <REDACTED-TEST-CLIENT-CERT>
    client-key-data: <REDACTED-TEST-CLIENT-KEY>`,
		config.ClusterName, config.BaseDomain, config.ClusterName,
		config.ClusterName, config.ClusterName, config.ClusterName)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: config.TestNamespace,
			Labels: map[string]string{
				"test-resource": "true",
				"cluster-name":  config.ClusterName,
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"kubeconfig": []byte(dummyKubeconfig),
		},
	}

	_, err := clientset.CoreV1().Secrets(config.TestNamespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return fmt.Errorf("failed to create admin kubeconfig secret: %w", err)
	}

	ginkgo.GinkgoLogr.Info("Created admin kubeconfig secret", "secretName", secretName)
	return nil
}

func BuildCompleteClusterDeployment(config *CertConfig, clusterDeploymentName, adminKubeconfigSecretName, ocmClusterID string) *unstructured.Unstructured {

	randomBytes := make([]byte, 3)
	if _, err := rand.Read(randomBytes); err != nil {
		// Fallback to a deterministic value if random generation fails (should never happen)
		randomBytes = []byte{0x12, 0x34, 0x56}
	}
	infraID := fmt.Sprintf("%s-%x", config.ClusterName, randomBytes)[:len(config.ClusterName)+6] // Take clusterName + "-" + 5 hex chars

	domainName := config.ClusterName

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      clusterDeploymentName,
				"namespace": config.TestNamespace,
				"labels": map[string]interface{}{
					"api.openshift.com/managed": "true",
					"api.openshift.com/id":      ocmClusterID,
					"api.openshift.com/name":    config.ClusterName,
				},
				"annotations": map[string]interface{}{
					"hive.openshift.io/protected-delete": "true",
					"hive.openshift.io/syncset-pause":    "true",
				},
			},
			"spec": map[string]interface{}{
				"installed":   true,
				"baseDomain":  config.BaseDomain,
				"clusterName": config.ClusterName,
				"clusterMetadata": map[string]interface{}{
					"clusterID": ocmClusterID,
					"infraID":   infraID,
					"adminKubeconfigSecretRef": map[string]interface{}{
						"name": adminKubeconfigSecretName,
					},
				},
				"certificateBundles": []interface{}{
					map[string]interface{}{
						"generate": true,
						"name":     "primary-cert-bundle",
						"certificateSecretRef": map[string]interface{}{
							"name": "primary-cert-bundle-secret",
						},
					},
				},
				"controlPlaneConfig": map[string]interface{}{
					"apiURLOverride": fmt.Sprintf("api.%s.%s:6443", domainName, config.BaseDomain),
					"servingCertificates": map[string]interface{}{
						"default": "primary-cert-bundle",
						"additional": []interface{}{
							map[string]interface{}{
								"domain": fmt.Sprintf("api.%s.%s", domainName, config.BaseDomain),
								"name":   "primary-cert-bundle",
							},
						},
					},
				},
				"ingress": []interface{}{
					map[string]interface{}{
						"domain":             fmt.Sprintf("apps.%s.%s", domainName, config.BaseDomain),
						"name":               "default",
						"servingCertificate": "primary-cert-bundle",
					},
				},
				"platform": map[string]interface{}{
					"aws": map[string]interface{}{
						"region": "us-east-1",
						"credentialsSecretRef": map[string]interface{}{
							"name": "aws",
						},
					},
				},
			},
			"status": map[string]interface{}{
				"apiURL":        fmt.Sprintf("https://api.%s.%s:6443", domainName, config.BaseDomain),
				"webConsoleURL": fmt.Sprintf("https://console-openshift-console.apps.%s.%s", domainName, config.BaseDomain),
			},
		},
	}
}

// Helper functions to extract values from ClusterDeployment for logging/verification
func GetClusterNameFromCD(cd *unstructured.Unstructured) string {
	clusterName, _, _ := unstructured.NestedString(cd.Object, "spec", "clusterName")
	return clusterName
}

func GetBaseDomainFromCD(cd *unstructured.Unstructured) string {
	baseDomain, _, _ := unstructured.NestedString(cd.Object, "spec", "baseDomain")
	return baseDomain
}

func GetAPIURLOverrideFromCD(cd *unstructured.Unstructured) string {
	apiURLOverride, _, _ := unstructured.NestedString(cd.Object, "spec", "controlPlaneConfig", "apiURLOverride")
	return apiURLOverride
}

func GetStatusAPIURLFromCD(cd *unstructured.Unstructured) string {
	apiURL, _, _ := unstructured.NestedString(cd.Object, "status", "apiURL")
	return apiURL
}

func GetInfraIDFromCD(cd *unstructured.Unstructured) string {
	infraID, _, _ := unstructured.NestedString(cd.Object, "spec", "clusterMetadata", "infraID")
	return infraID
}

// VerifyClusterDeploymentCriteria checks all reconciliation criteria from requirements
func VerifyClusterDeploymentCriteria(ctx context.Context, dynamicClient dynamic.Interface, gvr schema.GroupVersionResource, namespace, name, ocmClusterID string) bool {
	cd, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		ginkgo.GinkgoLogr.Error(err, "Failed to get ClusterDeployment")
		return false
	}

	// Check required label: "api.openshift.com/managed"
	labels := cd.GetLabels()
	gomega.Expect(labels).ToNot(gomega.BeNil(), "Labels should not be nil")
	gomega.Expect(labels["api.openshift.com/managed"]).To(gomega.Equal("true"), "Missing required managed label")

	// Check ClusterDeployment.Spec.Installed = True
	installed, found, _ := unstructured.NestedBool(cd.Object, "spec", "installed")
	gomega.Expect(found).To(gomega.BeTrue(), "Installed field not found")
	gomega.Expect(installed).To(gomega.BeTrue(), "Installed field not true")

	// Check NOT has annotation "hive.openshift.io/relocate" = "outgoing"
	annotations := cd.GetAnnotations()
	if annotations != nil {
		gomega.Expect(annotations["hive.openshift.io/relocate"]).ToNot(gomega.Equal("outgoing"),
			"Has relocate annotation set to outgoing - this prevents reconciliation")
	}

	// Verify OCM cluster ID matches
	gomega.Expect(labels["api.openshift.com/id"]).To(gomega.Equal(ocmClusterID),
		"OCM cluster ID mismatch", "expected", ocmClusterID, "actual", labels["api.openshift.com/id"])

	// Verify certificateBundles section exists
	certificateBundles, found, _ := unstructured.NestedSlice(cd.Object, "spec", "certificateBundles")
	gomega.Expect(found).To(gomega.BeTrue(), "certificateBundles section not found")
	gomega.Expect(certificateBundles).ToNot(gomega.BeEmpty(), "certificateBundles section is empty")

	ginkgo.GinkgoLogr.Info("All ClusterDeployment reconciliation criteria met")
	return true
}

// EnsureTestNamespace ensures the test namespace exists
func EnsureTestNamespace(ctx context.Context, clientset *kubernetes.Clientset, namespace string) error {
	_, err := clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Create namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
					Labels: map[string]string{
						"test-namespace": "true",
					},
				},
			}
			_, err = clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create namespace %s: %w", namespace, err)
			}
			ginkgo.GinkgoLogr.Info("Created test namespace", "namespace", namespace)
		} else {
			return fmt.Errorf("failed to get namespace %s: %w", namespace, err)
		}
	}
	return nil
}

// CleanupClusterDeployment removes ClusterDeployment if it exists
func CleanupClusterDeployment(ctx context.Context, dynamicClient dynamic.Interface, gvr schema.GroupVersionResource, namespace, name string) {
	err := dynamicClient.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		ginkgo.GinkgoLogr.Error(err, "Failed to cleanup ClusterDeployment", "name", name)
	} else if err == nil {
		ginkgo.GinkgoLogr.Info("Cleaned up existing ClusterDeployment", "name", name)
		// Wait for ClusterDeployment to be fully deleted
		gomega.Eventually(func() bool {
			_, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
			return apierrors.IsNotFound(err)
		}, 30*time.Second, 2*time.Second).Should(gomega.BeTrue(), "ClusterDeployment should be deleted")
	}
}

// FindCertificateRequestForClusterDeployment finds the CertificateRequest owned by a ClusterDeployment
func FindCertificateRequestForClusterDeployment(ctx context.Context, dynamicClient dynamic.Interface, crGVR schema.GroupVersionResource, namespace, clusterDeploymentName string) (*unstructured.Unstructured, error) {
	crList, err := dynamicClient.Resource(crGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list CertificateRequests: %w", err)
	}

	for i := range crList.Items {
		cr := &crList.Items[i]
		ownerRefs, found, _ := unstructured.NestedSlice(cr.Object, "metadata", "ownerReferences")
		if found && len(ownerRefs) > 0 {
			for _, ownerRef := range ownerRefs {
				ownerRefMap, ok := ownerRef.(map[string]interface{})
				if ok {
					ownerKind, _ := ownerRefMap["kind"].(string)
					ownerName, _ := ownerRefMap["name"].(string)
					if ownerKind == "ClusterDeployment" && ownerName == clusterDeploymentName {
						return cr, nil
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("no CertificateRequest found for ClusterDeployment %s", clusterDeploymentName)
}

// GetCertificateSecretNameFromCR extracts the certificate secret name from a CertificateRequest
func GetCertificateSecretNameFromCR(cr *unstructured.Unstructured) (string, error) {
	secretRef, found, _ := unstructured.NestedMap(cr.Object, "spec", "certificateSecret")
	if !found {
		return "", fmt.Errorf("certificateSecret not found in CertificateRequest spec")
	}
	name, ok := secretRef["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("certificateSecret name is empty or invalid")
	}
	return name, nil
}

// ForceDeleteCertificateRequests deletes all CertificateRequests in a namespace,
// removing finalizers if necessary to ensure deletion completes
func ForceDeleteCertificateRequests(ctx context.Context, dynamicClient dynamic.Interface, namespace string) {
	certificateRequestGVR := schema.GroupVersionResource{
		Group: "certman.managed.openshift.io", Version: "v1alpha1", Resource: "certificaterequests",
	}

	crList, err := dynamicClient.Resource(certificateRequestGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		ginkgo.GinkgoLogr.Error(err, "Failed to list CertificateRequests for cleanup")
		return
	}

	for _, cr := range crList.Items {
		crName := cr.GetName()

		// Check if CR has finalizers that might block deletion
		finalizers := cr.GetFinalizers()
		if len(finalizers) > 0 {
			ginkgo.GinkgoLogr.Info("Removing finalizers from CertificateRequest to force deletion",
				"name", crName, "finalizers", finalizers)

			// Remove all finalizers
			cr.SetFinalizers([]string{})
			_, err := dynamicClient.Resource(certificateRequestGVR).Namespace(namespace).Update(ctx, &cr, metav1.UpdateOptions{})
			if err != nil {
				ginkgo.GinkgoLogr.Error(err, "Failed to remove finalizers from CertificateRequest", "name", crName)
				// Continue trying to delete anyway
			} else {
				ginkgo.GinkgoLogr.Info("Successfully removed finalizers from CertificateRequest", "name", crName)
			}
		}

		// Delete the CertificateRequest
		err := dynamicClient.Resource(certificateRequestGVR).Namespace(namespace).Delete(ctx, crName, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			ginkgo.GinkgoLogr.Error(err, "Failed to delete CertificateRequest", "name", crName)
		} else if err == nil {
			ginkgo.GinkgoLogr.Info("Deleted CertificateRequest", "name", crName)
		}
	}

	// Verify all CRs are deleted
	time.Sleep(2 * time.Second)
	remaining, err := dynamicClient.Resource(certificateRequestGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err == nil && len(remaining.Items) > 0 {
		ginkgo.GinkgoLogr.Info("Warning: Some CertificateRequests still exist after cleanup", "count", len(remaining.Items))
	} else if err == nil {
		ginkgo.GinkgoLogr.Info("All CertificateRequests successfully deleted")
	}
}

func VerifyMetrics(ctx context.Context, clientset *kubernetes.Clientset, namespace string) (certRequestsCount, issuedCertCount int, success bool) {
	// Find the certman-operator pod
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "name=certman-operator",
	})
	if err != nil {
		ginkgo.GinkgoLogr.Error(err, "Failed to list certman-operator pods")
		return 0, 0, false
	}

	if len(pods.Items) == 0 {
		ginkgo.GinkgoLogr.Info("No certman-operator pods found")
		return 0, 0, false
	}

	// Use the first running pod
	var targetPod *corev1.Pod
	for i := range pods.Items {
		if pods.Items[i].Status.Phase == corev1.PodRunning {
			targetPod = &pods.Items[i]
			break
		}
	}

	if targetPod == nil {
		ginkgo.GinkgoLogr.Info("No running certman-operator pod found")
		return 0, 0, false
	}

	// Find the metrics port (default is 8080)
	metricsPort := int32(8080)
	for _, container := range targetPod.Spec.Containers {
		for _, port := range container.Ports {
			if port.Name == "metrics" || port.ContainerPort == 8080 {
				metricsPort = port.ContainerPort
				break
			}
		}
		if metricsPort != 8080 {
			break
		}
	}

	ginkgo.GinkgoLogr.Info("Querying metrics via Kubernetes API proxy",
		"pod", targetPod.Name,
		"port", metricsPort,
		"namespace", namespace)

	// Query metrics endpoint via API proxy using REST client
	restClient := clientset.CoreV1().RESTClient()

	// Use Raw() to get the raw response body as bytes
	result := restClient.Get().
		Namespace(namespace).
		Resource("pods").
		Name(fmt.Sprintf("%s:%d", targetPod.Name, metricsPort)).
		SubResource("proxy").
		Suffix("metrics").
		Do(ctx)

	if result.Error() != nil {
		ginkgo.GinkgoLogr.Error(result.Error(), "Failed to query metrics endpoint via API proxy")
		return 0, 0, false
	}

	metricsData, err := result.Raw()
	if err != nil {
		ginkgo.GinkgoLogr.Error(err, "Failed to read metrics response")
		return 0, 0, false
	}

	metricsText := string(metricsData)
	ginkgo.GinkgoLogr.Info("Metrics response received", "size", len(metricsText))

	// Parse metrics to find certificate_requests_count
	// Note: certRequestsCount and issuedCertCount are already declared in function signature
	lines := strings.Split(metricsText, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		if strings.HasPrefix(line, "certman_operator_certificate_requests_count") {
			lastSpace := strings.LastIndex(line, " ")
			if lastSpace > 0 {
				valueStr := strings.TrimSpace(line[lastSpace:])
				if count, err := fmt.Sscanf(valueStr, "%d", &certRequestsCount); err == nil && count == 1 {
					ginkgo.GinkgoLogr.Info("Found certificate_requests_count", "value", certRequestsCount, "line", line)
				}
			}
		}

		// Look for certman_operator_issued_certificates_count
		if strings.Contains(line, "certman_operator_issued_certificates_count") && !strings.HasPrefix(line, "#") {
			lastSpace := strings.LastIndex(line, " ")
			if lastSpace > 0 {
				valueStr := strings.TrimSpace(line[lastSpace:])
				if count, err := fmt.Sscanf(valueStr, "%d", &issuedCertCount); err == nil && count == 1 {
					ginkgo.GinkgoLogr.Info("Found issued_certificates_count", "value", issuedCertCount, "line", line)
				}
			}
		}
	}

	ginkgo.GinkgoLogr.Info("Metrics verification results",
		"certificate_requests_count", certRequestsCount,
		"issued_certificates_count", issuedCertCount)

	// Verify that we have at least 1 certificate request
	if certRequestsCount > 0 {
		ginkgo.GinkgoLogr.Info("Metrics validation successful",
			"certificate_requests_count", certRequestsCount,
			"issued_certificates_count", issuedCertCount)
		return certRequestsCount, issuedCertCount, true
	}

	ginkgo.GinkgoLogr.Info("Metrics validation: certificate_requests_count is 0 or not found",
		"certificate_requests_count", certRequestsCount,
		"issued_certificates_count", issuedCertCount)
	return certRequestsCount, issuedCertCount, false
}

// CleanupAllTestResources cleans up all resources created during testing
func CleanupAllTestResources(ctx context.Context, clientset *kubernetes.Clientset, dynamicClient dynamic.Interface, config *CertConfig, clusterDeploymentName, adminKubeconfigSecretName, ocmClusterID string) {
	// Cleanup ClusterDeployment
	clusterDeploymentGVR := schema.GroupVersionResource{
		Group: "hive.openshift.io", Version: "v1", Resource: "clusterdeployments",
	}
	CleanupClusterDeployment(ctx, dynamicClient, clusterDeploymentGVR, config.TestNamespace, clusterDeploymentName)

	// Cleanup secrets
	secrets := []string{
		adminKubeconfigSecretName,
	}

	for _, secretName := range secrets {
		err := clientset.CoreV1().Secrets(config.TestNamespace).Delete(ctx, secretName, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			ginkgo.GinkgoLogr.Error(err, "Failed to cleanup secret", "secretName", secretName)
		} else if err == nil {
			ginkgo.GinkgoLogr.Info("Cleaned up secret", "secretName", secretName)
		}
	}

	// Cleanup CertificateRequests (these are created by the operator but should be cleaned up)
	// Force delete by removing finalizers if they're stuck
	ForceDeleteCertificateRequests(ctx, dynamicClient, config.TestNamespace)

	ginkgo.GinkgoLogr.Info("Test resource cleanup completed",
		"clusterName", config.ClusterName,
		"namespace", config.TestNamespace,
		"ocmClusterID", ocmClusterID)

}

var logger = logs.Log

func DownloadAndApplyCRD(ctx context.Context, apiExtClient apiextensionsclient.Interface, crdURL, crdName string) error {
	log.Printf("CRD '%s' not found. Downloading and applying from: %s", crdName, crdURL)

	// Validate URL
	parsedURL, err := url.ParseRequestURI(crdURL)
	if err != nil {
		return fmt.Errorf("invalid CRD URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download CRD from %s: %w", crdURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download CRD. HTTP status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read CRD body: %w", err)
	}

	decoder := syaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	crd := &apiextensionsv1.CustomResourceDefinition{}
	_, _, err = decoder.Decode(data, nil, crd)
	if err != nil {
		return fmt.Errorf("failed to decode CRD YAML: %w", err)
	}

	if _, err := apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, crd, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create CRD '%s': %w", crdName, err)
	}

	log.Printf("CRD '%s' applied.", crdName)
	return nil
}

func ApplyManifestsFromURLs(ctx context.Context, cfg *rest.Config, manifestURLs []string) error {
	// Create discovery client and dynamic client from REST config
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create discovery client: %w", err)
	}

	gr, err := restmapper.GetAPIGroupResources(dc)
	if err != nil {
		return fmt.Errorf("failed to get API group resources: %w", err)
	}
	mapper := restmapper.NewDiscoveryRESTMapper(gr)

	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	for _, manifestURL := range manifestURLs {
		log.Printf("Downloading manifest from: %s", manifestURL)

		parsedURL, err := url.ParseRequestURI(manifestURL)
		if err != nil {
			return fmt.Errorf("invalid manifest URL: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
		if err != nil {
			return fmt.Errorf("failed to create HTTP request: %w", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to download manifest from %s: %w", manifestURL, err)
		}
		defer resp.Body.Close()

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read manifest from %s: %w", manifestURL, err)
		}

		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)

		for {
			var rawObj map[string]interface{}
			if err := decoder.Decode(&rawObj); err != nil {
				if err == io.EOF {
					break
				}
				return fmt.Errorf("failed to decode YAML from %s: %w", manifestURL, err)
			}
			if len(rawObj) == 0 {
				continue
			}

			obj := &unstructured.Unstructured{Object: rawObj}
			gvk := obj.GroupVersionKind()

			mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
			if err != nil {
				return fmt.Errorf("failed to get REST mapping for GVK %v: %w", gvk, err)
			}

			var dri dynamic.ResourceInterface
			if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
				ns := obj.GetNamespace()
				if ns == "" {
					ns = "certman-operator"
					obj.SetNamespace(ns)
				}
				dri = dynamicClient.Resource(mapping.Resource).Namespace(ns)
			} else {
				dri = dynamicClient.Resource(mapping.Resource)
			}

			_, err = dri.Create(ctx, obj, metav1.CreateOptions{})
			if apierrors.IsAlreadyExists(err) {
				log.Printf("Resource %s/%s already exists, skipping.", obj.GetNamespace(), obj.GetName())
				continue
			}
			if err != nil {
				return fmt.Errorf("failed to create resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
			}
			log.Printf("Successfully applied resource: %s/%s", obj.GetNamespace(), obj.GetName())
		}
	}

	return nil
}

func SetupHiveCRDs(ctx context.Context, apiExtClient apiextensionsclient.Interface) error {
	const crdURL = "https://raw.githubusercontent.com/openshift/hive/master/config/crds/hive.openshift.io_clusterdeployments.yaml"
	const crdName = "clusterdeployments.hive.openshift.io"

	_, err := apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crdName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return DownloadAndApplyCRD(ctx, apiExtClient, crdURL, crdName)
	} else if err != nil {
		return fmt.Errorf("error getting CRD '%s': %w", crdName, err)
	}

	log.Printf("CRD '%s' already exists.", crdName)
	return nil
}

// SetupCertman ensures namespace, CRD, ConfigMap and applies operator manifests
func SetupCertman(ctx context.Context, kubeClient kubernetes.Interface, apiExtClient apiextensionsclient.Interface, cfg *rest.Config) error {
	const (
		namespace     = "certman-operator"
		configMapName = "certman-operator"
		crdURL        = "https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/crds/certman.managed.openshift.io_certificaterequests.yaml"
		crdName       = "certificaterequests.certman.managed.openshift.io"
	)

	// Check namespace status and fix if terminating
	ns, err := kubeClient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err == nil && ns.Status.Phase == corev1.NamespaceTerminating {
		log.Printf("Namespace '%s' is stuck in terminating state. Removing finalizers to force deletion...", namespace)

		// Remove finalizers from the namespace to allow deletion
		if len(ns.Spec.Finalizers) > 0 {
			log.Printf("Removing %d finalizers from namespace '%s'", len(ns.Spec.Finalizers), namespace)
			ns.Spec.Finalizers = []corev1.FinalizerName{}
			_, updateErr := kubeClient.CoreV1().Namespaces().Finalize(ctx, ns, metav1.UpdateOptions{})
			if updateErr != nil {
				log.Printf("Warning: failed to remove namespace finalizers: %v", updateErr)
			} else {
				log.Printf("Successfully removed finalizers from namespace '%s'", namespace)
			}
		}

		// Wait for namespace to be fully deleted (up to 2 minutes)
		log.Printf("Waiting for namespace '%s' to be fully deleted...", namespace)
		for i := 0; i < 24; i++ {
			time.Sleep(5 * time.Second)
			_, err := kubeClient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				log.Printf("Namespace '%s' has been fully deleted.", namespace)
				break
			}
			if i == 23 {
				return fmt.Errorf("timeout waiting for namespace '%s' to finish terminating after removing finalizers", namespace)
			}
			log.Printf("Still waiting for namespace '%s' to terminate... (%d/24)", namespace, i+1)
		}
		// Reset err to NotFound so we create the namespace below
		err = apierrors.NewNotFound(corev1.Resource("namespaces"), namespace)
	}

	if apierrors.IsNotFound(err) {
		log.Printf("Namespace '%s' not found. Creating namespace", namespace)
		newNs := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}

		if _, err := kubeClient.CoreV1().Namespaces().Create(ctx, newNs, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("failed to create namespace '%s': %w", namespace, err)
		}
		log.Printf("Namespace '%s' created.", namespace)
	} else if err != nil {
		return fmt.Errorf("error getting namespace '%s': %w", namespace, err)
	} else {
		log.Printf("Namespace '%s' already exists.", namespace)
	}

	// Checking CRD exists or create it
	_, err = apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crdName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if err := DownloadAndApplyCRD(ctx, apiExtClient, crdURL, crdName); err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("error getting CRD '%s': %w", crdName, err)
	} else {
		log.Printf("CRD '%s' already exists.", crdName)
	}

	// Ensuring ConfigMap exists or create it
	_, err = kubeClient.CoreV1().ConfigMaps(namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		log.Printf("ConfigMap '%s' not found in namespace '%s'. Creating ConfigMap", configMapName, namespace)

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: namespace,
			},
			Data: map[string]string{
				"default_notification_email_address": "teste2e@redhat.com",
			},
		}

		if _, err := kubeClient.CoreV1().ConfigMaps(namespace).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("failed to create ConfigMap '%s': %w", configMapName, err)
		}
		log.Printf("ConfigMap '%s' created in namespace '%s'.", configMapName, namespace)
	} else if err != nil {
		return fmt.Errorf("error getting ConfigMap '%s': %w", configMapName, err)
	} else {
		log.Printf("ConfigMap '%s' already exists in namespace '%s'.", configMapName, namespace)
	}

	manifestURLs := []string{
		"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/service_account.yaml",
		"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/role.yaml",
		"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/role_binding.yaml",
		"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/operator.yaml",
	}

	if err := ApplyManifestsFromURLs(ctx, cfg, manifestURLs); err != nil {
		return fmt.Errorf("failed to apply certman operator manifests: %w", err)
	}

	log.Println("Certman setup completed successfully.")
	return nil
}

func SetupAWSCreds(ctx context.Context, kubeClient kubernetes.Interface) error {
	const (
		namespace  = "certman-operator"
		secretName = "aws"
	)

	awsAccessKey, awsSecretKey := getSecretAndAccessKeys()
	// Environment variables must be set - fail if not provided
	if awsAccessKey == "" {
		return fmt.Errorf("AWS_ACCESS_KEY_ID environment variable must be set")
	}
	if awsSecretKey == "" {
		return fmt.Errorf("AWS_SECRET_ACCESS_KEY environment variable must be set")
	}

	_, err := kubeClient.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})

	if err == nil {
		log.Printf("AWS Secret '%s' already exists in namespace '%s'. Skipping creation.", secretName, namespace)
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("error checking for existing AWS secret: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		StringData: map[string]string{
			"aws_access_key_id":     awsAccessKey,
			"aws_secret_access_key": awsSecretKey,
		},
		Type: corev1.SecretTypeOpaque,
	}

	_, err = kubeClient.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create AWS secret: %w", err)
	}

	log.Printf("AWS Secret '%s' created successfully in namespace '%s'.", secretName, namespace)
	return nil
}

func GetEnvOrDefault(envVar, defaultValue string) string {
	if value := os.Getenv(envVar); value != "" {
		return value
	}
	return defaultValue
}

func getSecretAndAccessKeys() (accesskey, secretkey string) {
	accesskey = os.Getenv("AWS_ACCESS_KEY_ID")
	secretkey = os.Getenv("AWS_SECRET_ACCESS_KEY")

	// Return exact values as-is - no quotes, no modification
	return accesskey, secretkey
}

// G204 lint issue for exec.command
func SanitizeInput(input string) string {
	return "\"" + strings.ReplaceAll(input, "\"", "\\\"") + "\""
}

func SetupLetsEncryptAccountSecret(ctx context.Context, kubeClient kubernetes.Interface) error {
	const (
		namespace  = "certman-operator"
		secretName = "lets-encrypt-account" //nolint:gosec // This is a secret resource name, not a credential
	)

	// Use mock ACME client URL for testing (equivalent to: echo -n "proto://use.mock.acme.client")
	mockAccountURL := "proto://use.mock.acme.client"

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate EC private key: %w", err)
	}

	// Marshal the private key to EC private key format
	keyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("failed to marshal EC private key: %w", err)
	}

	// Encode to PEM format (equivalent to the .pem file created by openssl)
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})

	// Check if secret already exists
	_, err = kubeClient.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err == nil {
		log.Printf("Let's Encrypt account secret '%s' already exists in namespace '%s'. Skipping creation.", secretName, namespace)
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("error checking for existing Let's Encrypt account secret: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"private-key": keyPEM,
			"account-url": []byte(mockAccountURL),
		},
		Type: corev1.SecretTypeOpaque,
	}

	_, err = kubeClient.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Let's Encrypt account secret: %w", err)
	}

	log.Printf("Let's Encrypt account secret '%s' created successfully in namespace '%s'.", secretName, namespace)
	return nil
}

// CleanupLetsEncryptAccountSecret removes the lets-encrypt-account secret
func CleanupLetsEncryptAccountSecret(ctx context.Context, kubeClient kubernetes.Interface) error {
	const (
		namespace  = "certman-operator"
		secretName = "lets-encrypt-account" //nolint:gosec // This is a secret resource name, not a credential
	)

	err := kubeClient.CoreV1().Secrets(namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func CleanupHive(ctx context.Context, apiExtClient apiextensionsclient.Interface) error {
	const hiveCRDName = "clusterdeployments.hive.openshift.io"

	err := apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Delete(ctx, hiveCRDName, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		log.Printf("Hive CRD not found; nothing to delete")
		return nil
	} else if err != nil {
		return fmt.Errorf("error deleting Hive CRD: %w", err)
	}

	log.Printf("Hive CRD deleted successfully")
	return nil
}

func CleanupCertman(ctx context.Context, kubeClient kubernetes.Interface, apiExtClient apiextensionsclient.Interface, dynamicClient dynamic.Interface, mapper meta.RESTMapper) error {
	const (
		certmanCRDName = "certificaterequests.certman.managed.openshift.io"
		operatorNS     = "certman-operator"
	)

	err := apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Delete(ctx, certmanCRDName, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		log.Printf("Certman CRD not found nothing to delete")
	} else if err != nil {
		return fmt.Errorf("error deleting Certman CRD: %w", err)
	} else {
		log.Printf("Certman CRD deleted successfully")
	}

	err = kubeClient.CoreV1().Namespaces().Delete(ctx, operatorNS, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		log.Printf("Namespace %s not found nothing to delete", operatorNS)
	} else if err != nil {
		return fmt.Errorf("error deleting namespace %s: %w", operatorNS, err)
	} else {
		log.Printf("Namespace %s deleted successfully", operatorNS)
	}

	manifestURLs := []string{
		"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/service_account.yaml",
		"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/role.yaml",
		"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/role_binding.yaml",
		"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/operator.yaml",
	}

	for _, manifestURL := range manifestURLs {
		log.Printf("Downloading manifest for cleanup: %s", manifestURL)

		parsedURL, err := url.ParseRequestURI(manifestURL)
		if err != nil {
			log.Printf("Invalid manifest URL %s: %v", manifestURL, err)
			continue
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
		if err != nil {
			log.Printf("Failed to create HTTP request for %s: %v", manifestURL, err)
			continue
		}

		func() {
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Printf("Failed to download manifest %s: %v", manifestURL, err)
				return
			}
			defer resp.Body.Close()

			data, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Printf("Failed to read manifest %s: %v", manifestURL, err)
				return
			}

			decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)

			for {
				var rawObj map[string]interface{}
				if err := decoder.Decode(&rawObj); err != nil {
					if err == io.EOF {
						break
					}
					log.Printf("Failed to decode YAML %s: %v", manifestURL, err)
					break
				}
				if len(rawObj) == 0 {
					continue
				}

				obj := &unstructured.Unstructured{Object: rawObj}
				mapping, err := mapper.RESTMapping(obj.GroupVersionKind().GroupKind(), obj.GroupVersionKind().Version)
				if err != nil {
					log.Printf("Failed to get REST mapping for object %v: %v", obj.GroupVersionKind(), err)
					continue
				}

				var dri dynamic.ResourceInterface
				if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
					ns := obj.GetNamespace()
					if ns == "" {
						ns = operatorNS
						obj.SetNamespace(ns)
					}
					dri = dynamicClient.Resource(mapping.Resource).Namespace(ns)
				} else {
					dri = dynamicClient.Resource(mapping.Resource)
				}

				err = dri.Delete(ctx, obj.GetName(), metav1.DeleteOptions{})
				if apierrors.IsNotFound(err) {
					log.Printf("Resource %s not found; skipping delete", obj.GetName())
				} else if err != nil {
					log.Printf("Failed to delete resource %s: %v", obj.GetName(), err)
				} else {
					log.Printf("Deleted resource %s", obj.GetName())
				}
			}
		}()
	}

	return nil
}

func CleanupAWSCreds(ctx context.Context, kubeClient kubernetes.Interface) error {
	const (
		namespace  = "certman-operator"
		secretName = "aws"
	)

	err := kubeClient.CoreV1().Secrets(namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func CreateCertmanResources(ctx context.Context, dynamicClient dynamic.Interface, namespace string) bool {
	operatorGroup := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "operators.coreos.com/v1",
			"kind":       "OperatorGroup",
			"metadata": map[string]interface{}{
				"name":      "certman-operator-og",
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"targetNamespaces": []string{namespace},
				"upgradeStrategy":  "Default",
			},
		},
	}

	catalogSource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "operators.coreos.com/v1alpha1",
			"kind":       "CatalogSource",
			"metadata": map[string]interface{}{
				"name":      "certman-operator-catalog",
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"displayName": "certman-operator Registry",
				"image":       "quay.io/app-sre/certman-operator-registry:staging-latest",
				"publisher":   "SRE",
				"sourceType":  "grpc",
			},
		},
	}

	subscription := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "operators.coreos.com/v1alpha1",
			"kind":       "Subscription",
			"metadata": map[string]interface{}{
				"name":      "certman-operator",
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"channel": "staging",
				"config": map[string]interface{}{
					"env": []interface{}{
						map[string]interface{}{"name": "FEDRAMP", "value": "false"},
						map[string]interface{}{"name": "HOSTED_ZONE_ID", "value": ""},
					},
				},
				"name":                "certman-operator",
				"source":              "certman-operator-catalog",
				"sourceNamespace":     namespace,
				"installPlanApproval": "Automatic",
			},
		},
	}

	createIfNotExist := func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured, name string) bool {
		_, err := dynamicClient.Resource(gvr).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{})
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				logger.Info(name + " already exists, skipping creation")
				return true
			}
			logger.Error(err, "Failed to create "+name)
			return false
		}
		logger.Info("Created " + name + " successfully")
		return true
	}

	logger.Info("Creating OperatorGroup")
	if !createIfNotExist(schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1",
		Resource: "operatorgroups",
	}, operatorGroup, "OperatorGroup") {
		return false
	}

	logger.Info("Creating CatalogSource")
	if !createIfNotExist(schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Resource: "catalogsources",
	}, catalogSource, "CatalogSource") {
		return false
	}

	// Verify CatalogSource is running by listing them
	catalogSourceList, err := dynamicClient.Resource(schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Resource: "catalogsources",
	}).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil || len(catalogSourceList.Items) == 0 {
		logger.Error(err, "CatalogSource not running or failed to list CatalogSources")
		return false
	}
	logger.Info("CatalogSource is running successfully")

	logger.Info("Creating Subscription")

	return createIfNotExist(schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Resource: "subscriptions",
	}, subscription, "Subscription")

}

func GetCurrentCSVVersion(ctx context.Context, dynamicClient dynamic.Interface, namespace string) (string, error) {
	csvList, err := dynamicClient.Resource(schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Resource: "clusterserviceversions",
	}).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list CSV: %w", err)
	}

	if len(csvList.Items) == 0 {
		return "", fmt.Errorf("no CSV found in namespace %s", namespace)
	}

	for i := range csvList.Items {
		csv := &csvList.Items[i]
		if !strings.HasPrefix(csv.GetName(), "certman-operator.") {
			continue
		}

		phase, found, _ := unstructured.NestedString(csv.Object, "status", "phase")
		if found && phase == "Succeeded" {
			currentVersion := strings.TrimPrefix(csv.GetName(), "certman-operator.")
			return currentVersion, nil
		}
	}

	return "", fmt.Errorf("no succeeded CSV found for certman-operator in namespace %s", namespace)
}

func GetLatestAvailableCSVVersion(ctx context.Context, dynamicClient dynamic.Interface, namespace string) (string, error) {
	packageManifestGVR := schema.GroupVersionResource{
		Group:    "packages.operators.coreos.com",
		Version:  "v1",
		Resource: "packagemanifests",
	}

	packageManifests, err := dynamicClient.Resource(packageManifestGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "catalog=certman-operator-catalog",
	})
	if err != nil {
		return "", fmt.Errorf("failed to list package manifests: %w", err)
	}

	var latestCSV string
	var latestVersion string

	for _, pm := range packageManifests.Items {
		name, _, _ := unstructured.NestedString(pm.Object, "metadata", "name")
		if name == "certman-operator" {
			channels, found, err := unstructured.NestedSlice(pm.Object, "status", "channels")
			if err != nil || !found {
				continue
			}

			for _, channel := range channels {
				channelMap, ok := channel.(map[string]interface{})
				if !ok {
					continue
				}

				channelName, _, _ := unstructured.NestedString(channelMap, "name")
				if channelName == "staging" {
					currentCSV, _, _ := unstructured.NestedString(channelMap, "currentCSV")
					if currentCSV != "" {
						version := strings.TrimPrefix(currentCSV, "certman-operator.")
						if version > latestVersion {
							latestVersion = version
							latestCSV = currentCSV
						}
					}
					break
				}
			}
			break
		}
	}

	if latestCSV == "" {
		return "", fmt.Errorf("no CSV found for certman-operator in staging channel")
	}

	logger.Info("Latest available CSV version", "csv", latestCSV, "version", latestVersion)
	return latestVersion, nil
}

func CheckForUpgrade(ctx context.Context, dynamicClient dynamic.Interface, namespace string) (bool, string, string, error) {
	currentVersion, err := GetCurrentCSVVersion(ctx, dynamicClient, namespace)
	if err != nil {
		return false, "", "", fmt.Errorf("failed to get current CSV version: %w", err)
	}

	latestVersion, err := GetLatestAvailableCSVVersion(ctx, dynamicClient, namespace)
	if err != nil {
		return false, "", "", fmt.Errorf("failed to get latest available CSV version: %w", err)
	}

	hasUpgrade := latestVersion > currentVersion

	if hasUpgrade {
		logger.Info("Upgrade available", "current", currentVersion, "latest", latestVersion)
	} else {
		logger.Info("No upgrade available", "current", currentVersion, "latest", latestVersion)
	}

	return hasUpgrade, currentVersion, latestVersion, nil
}

func TriggerUpgrade(ctx context.Context, dynamicClient dynamic.Interface, namespace string) error {
	logger.Info("Triggering operator upgrade...")

	subscriptionGVR := schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Resource: "subscriptions",
	}

	subscription, err := dynamicClient.Resource(subscriptionGVR).Namespace(namespace).Get(ctx, "certman-operator", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	latestVersion, err := GetLatestAvailableCSVVersion(ctx, dynamicClient, namespace)
	if err != nil {
		return fmt.Errorf("failed to get latest CSV version: %w", err)
	}

	latestCSV := "certman-operator." + latestVersion

	err = unstructured.SetNestedField(subscription.Object, latestCSV, "spec", "startingCSV")
	if err != nil {
		return fmt.Errorf("failed to set startingCSV in subscription: %w", err)
	}

	_, err = dynamicClient.Resource(subscriptionGVR).Namespace(namespace).Update(ctx, subscription, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	logger.Info("Subscription updated with latest CSV", "csv", latestCSV)
	return nil
}

func WaitForUpgradeCompletion(ctx context.Context, dynamicClient dynamic.Interface, namespace string, expectedVersion string, timeout time.Duration) error {
	logger.Info("Waiting for upgrade to complete", "expectedVersion", expectedVersion)

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("upgrade timeout after %v", timeout)
		case <-ticker.C:
			currentVersion, err := GetCurrentCSVVersion(ctx, dynamicClient, namespace)
			if err != nil {
				logger.Error(err, "Failed to get current CSV version during upgrade wait")
				continue
			}

			if currentVersion == expectedVersion {
				logger.Info("Upgrade completed successfully", "version", currentVersion)
				return nil
			}

			logger.Info("Upgrade in progress", "current", currentVersion, "expected", expectedVersion)
		}
	}
}

func UpgradeOperatorToLatest(ctx context.Context, dynamicClient dynamic.Interface, namespace string) error {
	logger.Info("Starting operator upgrade process...")

	hasUpgrade, currentVersion, latestVersion, err := CheckForUpgrade(ctx, dynamicClient, namespace)
	if err != nil {
		return fmt.Errorf("failed to check for upgrade: %w", err)
	}

	if !hasUpgrade {
		logger.Info("No upgrade available, operator is already at latest version", "version", currentVersion)
		return nil
	}

	if err := TriggerUpgrade(ctx, dynamicClient, namespace); err != nil {
		return fmt.Errorf("failed to trigger upgrade: %w", err)
	}

	if err := WaitForUpgradeCompletion(ctx, dynamicClient, namespace, latestVersion, 30*time.Second); err != nil {
		return fmt.Errorf("upgrade did not complete: %w", err)
	}

	logger.Info("Operator upgrade completed successfully", "from", currentVersion, "to", latestVersion)
	return nil
}

func CheckPodStatus(ctx context.Context, clientset *kubernetes.Clientset, namespace string) bool {
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil || len(pods.Items) == 0 {
		return false
	}
	for _, pod := range pods.Items {
		if strings.Contains(pod.Name, "certman-operator") {
			fmt.Printf("Phase: %s", pod.Status.Phase)
			return pod.Status.Phase == corev1.PodRunning
		}
	}
	return false
}

func CleanupCertmanResources(ctx context.Context, dynamicClient dynamic.Interface, namespace string) error {
	deleteResource := func(gvr schema.GroupVersionResource, name string) {
		err := dynamicClient.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			logger.Error(err, "Failed to delete resource", "name", name)
		} else {
			logger.Info("Deleted resource", "name", name)
		}
	}

	// Delete Subscription
	deleteResource(schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Resource: "subscriptions",
	}, "certman-operator")

	// Delete CatalogSource
	deleteResource(schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Resource: "catalogsources",
	}, "certman-operator-catalog")

	// Delete OperatorGroup
	deleteResource(schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1",
		Resource: "operatorgroups",
	}, "certman-operator-og")

	// Delete CSVs
	csvList, err := dynamicClient.Resource(schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Resource: "clusterserviceversions",
	}).Namespace(namespace).List(ctx, metav1.ListOptions{})

	if err != nil {
		logger.Error(err, "Failed to list CSV for cleanup")
		return err
	}

	for _, csv := range csvList.Items {
		name := csv.GetName()
		logger.Info("CSV name: ", name)
		if strings.HasPrefix(name, "certman-operator.") {
			deleteResource(schema.GroupVersionResource{
				Group:    "operators.coreos.com",
				Version:  "v1alpha1",
				Resource: "clusterserviceversions",
			}, name)
		}
	}

	return nil
}
