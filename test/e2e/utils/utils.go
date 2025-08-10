package utils

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/openshift/osde2e-common/pkg/clients/ocm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type CertConfig struct {
	ClusterName    string
	BaseDomain     string
	TestNamespace  string
	CertSecretName string
	OCMClusterID   string
}

func NewCertConfig(clusterName string, ocmClusterID string, baseDomain string) *CertConfig {
	return &CertConfig{
		ClusterName:    clusterName,
		BaseDomain:     baseDomain,
		TestNamespace:  "certman-operator",
		CertSecretName: "primary-cert-bundle-secret",
		OCMClusterID:   ocmClusterID,
	}
}

func GetEnvOrDefault(envVar, defaultValue string) string {
	if value := os.Getenv(envVar); value != "" {
		return value
	}
	return defaultValue
}

// GetClusterIDFromClusterVersion retrieves the cluster ID from the ClusterVersion object
func getClusterExternalIDFromClusterVersion(ctx context.Context, dynamicClient dynamic.Interface) (string, error) {
	clusterVersionGVR := schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "clusterversions",
	}

	// ClusterVersion is a cluster-scoped resource, so no namespace is needed
	clusterVersion, err := dynamicClient.Resource(clusterVersionGVR).Get(ctx, "version", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get ClusterVersion object: %w", err)
	}

	// Get external ID from spec.clusterID
	specClusterID, found, err := unstructured.NestedString(clusterVersion.Object, "spec", "clusterID")
	if err != nil {
		return "", fmt.Errorf("failed to access spec.clusterID: %w", err)
	}
	if !found || specClusterID == "" {
		return "", fmt.Errorf("spec.clusterID not found or empty in ClusterVersion")
	}

	return specClusterID, nil
}

// GetClusterInfoFromOCM retrieves both cluster ID and name from OCM by first getting the external ID from ClusterVersion
func GetClusterInfoFromOCM(ctx context.Context, ocmConn *ocm.Client, dynamicClient dynamic.Interface) (string, string, error) {
	ginkgo.GinkgoLogr.Info("Getting cluster info from OCM using ClusterVersion external ID")

	// First get the external ID from ClusterVersion
	externalID, err := getClusterExternalIDFromClusterVersion(ctx, dynamicClient)
	if err != nil {
		return "", "", fmt.Errorf("failed to get external ID from ClusterVersion: %w", err)
	}

	ginkgo.GinkgoLogr.Info("Retrieved external ID from ClusterVersion", "externalID", externalID)

	search := fmt.Sprintf("external_id = '%s'", externalID)
	response, err := ocmConn.ClustersMgmt().V1().Clusters().List().
		Search(search).
		Size(1).
		Send()
	if err != nil {
		return "", "", fmt.Errorf("failed to search clusters in OCM by external ID: %w", err)
	}

	if response.Total() == 0 {
		return "", "", fmt.Errorf("no cluster found with external ID '%s' in OCM", externalID)
	}

	cluster := response.Items().Get(0)
	clusterID := cluster.ID()
	clusterName := cluster.Name()

	if clusterID == "" {
		return "", "", fmt.Errorf("cluster ID is empty for external ID '%s'", externalID)
	}
	if clusterName == "" {
		return "", "", fmt.Errorf("cluster name is empty for external ID '%s'", externalID)
	}

	ginkgo.GinkgoLogr.Info("Found cluster in OCM",
		"externalID", externalID,
		"clusterID", clusterID,
		"clusterName", clusterName)

	return clusterID, clusterName, nil
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

// BuildCompleteClusterDeployment creates ClusterDeployment matching the requirements exactly
func BuildCompleteClusterDeployment(config *CertConfig, clusterDeploymentName, adminKubeconfigSecretName, ocmClusterID string) *unstructured.Unstructured {
	infraID := fmt.Sprintf("%s-infra", config.ClusterName)

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      clusterDeploymentName,
				"namespace": config.TestNamespace,
				"labels": map[string]interface{}{
					"api.openshift.com/managed":             "true",
					"api.openshift.com/ccs":                 "true",
					"api.openshift.com/channel-group":       "stable",
					"api.openshift.com/environment":         "staging",
					"api.openshift.com/name":                config.ClusterName,
					"api.openshift.com/product":             "rosa",
					"api.openshift.com/id":                  ocmClusterID,
					"hive.openshift.io/cluster-platform":    "aws",
					"hive.openshift.io/cluster-region":      "us-east-1",
					"hive.openshift.io/cluster-type":        "managed",
					"hive.openshift.io/version-major":       "4",
					"hive.openshift.io/version-major-minor": "4.19",
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
				// CRITICAL: certificateBundles section as per requirement
				"certificateBundles": []interface{}{
					map[string]interface{}{
						"certificateSecretRef": map[string]interface{}{
							"name": config.CertSecretName,
						},
						"generate": true,
						"name":     "primary-cert-bundle",
					},
				},
				// CRITICAL: controlPlaneConfig as per requirement
				"controlPlaneConfig": map[string]interface{}{
					"apiURLOverride": fmt.Sprintf("rh-api.%s.%s:6443", config.ClusterName, config.BaseDomain),
					"servingCertificates": map[string]interface{}{
						"additional": []interface{}{
							map[string]interface{}{
								"domain": fmt.Sprintf("rh-api.%s.%s", config.ClusterName, config.BaseDomain),
								"name":   "primary-cert-bundle",
							},
						},
						"default": "primary-cert-bundle",
					},
				},
				// CRITICAL: ingress section as per requirement
				"ingress": []interface{}{
					map[string]interface{}{
						"domain":             fmt.Sprintf("apps.%s.%s", config.ClusterName, config.BaseDomain),
						"name":               "default",
						"servingCertificate": "primary-cert-bundle",
					},
				},
				"platform": map[string]interface{}{
					"aws": map[string]interface{}{
						"region": "us-east-1",
					},
				},
			},
		},
	}
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
	if labels == nil || labels["api.openshift.com/managed"] != "true" {
		ginkgo.GinkgoLogr.Info("❌ Missing required managed label")
		return false
	}

	// Check ClusterDeployment.Spec.Installed = True
	installed, found, _ := unstructured.NestedBool(cd.Object, "spec", "installed")
	if !found || !installed {
		ginkgo.GinkgoLogr.Info("❌ Installed field not true", "installed", installed, "found", found)
		return false
	}

	// Check NOT has annotation "hive.openshift.io/relocate" = "outgoing"
	annotations := cd.GetAnnotations()
	if annotations != nil && annotations["hive.openshift.io/relocate"] == "outgoing" {
		ginkgo.GinkgoLogr.Info("❌ Has relocate annotation set to outgoing - this prevents reconciliation")
		return false
	}

	// Verify OCM cluster ID matches
	if labels["api.openshift.com/id"] != ocmClusterID {
		ginkgo.GinkgoLogr.Info("❌ OCM cluster ID mismatch", "expected", ocmClusterID, "actual", labels["api.openshift.com/id"])
		return false
	}

	// Verify certificateBundles section exists
	certificateBundles, found, _ := unstructured.NestedSlice(cd.Object, "spec", "certificateBundles")
	if !found || len(certificateBundles) == 0 {
		ginkgo.GinkgoLogr.Info("❌ Missing certificateBundles section")
		return false
	}

	ginkgo.GinkgoLogr.Info("✅ All ClusterDeployment reconciliation criteria met")
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
		time.Sleep(5 * time.Second) // Wait for cleanup
	}
}

func VerifyMetrics(ctx context.Context, dynamicClient dynamic.Interface, certificateRequestGVR schema.GroupVersionResource, namespace string) (int, bool) {
	// Get all CertificateRequests in the namespace
	crList, err := dynamicClient.Resource(certificateRequestGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		ginkgo.GinkgoLogr.Error(err, "Failed to list CertificateRequests for metrics verification")
		return 0, false
	}

	if len(crList.Items) == 0 {
		ginkgo.GinkgoLogr.Info("No CertificateRequests found for metrics verification")
		return 0, false
	}

	validCount := 0
	// Simplified verification: check if CertificateRequest exists and has required spec fields
	for _, cr := range crList.Items {
		// Check if CR has the basic spec fields that indicate it's properly configured
		dnsNames, found, _ := unstructured.NestedStringSlice(cr.Object, "spec", "dnsNames")
		if !found || len(dnsNames) == 0 {
			ginkgo.GinkgoLogr.Info("CertificateRequest missing dnsNames", "name", cr.GetName())
			continue
		}

		// Check if email is present (indicates proper configuration)
		email, found, _ := unstructured.NestedString(cr.Object, "spec", "email")
		if !found || email == "" {
			ginkgo.GinkgoLogr.Info("CertificateRequest missing email", "name", cr.GetName())
			continue
		}

		validCount++
		ginkgo.GinkgoLogr.Info("✅ Metrics validation: Found valid CertificateRequest",
			"name", cr.GetName(),
			"dnsNames", len(dnsNames),
			"email", email)
	}

	if validCount > 0 {
		ginkgo.GinkgoLogr.Info("Metrics validation successful", "validCertificateRequests", validCount, "totalFound", len(crList.Items))
		return validCount, true
	}

	ginkgo.GinkgoLogr.Info("Metrics validation: No valid CertificateRequests found", "totalFound", len(crList.Items))
	return 0, false
}

// CleanupAllTestResources cleans up all resources created during testing
func CleanupAllTestResources(ctx context.Context, clientset *kubernetes.Clientset, dynamicClient dynamic.Interface, config *CertConfig, clusterDeploymentName, adminKubeconfigSecretName, ocmClusterID string) {
	// Cleanup ClusterDeployment
	clusterDeploymentGVR := schema.GroupVersionResource{
		Group: "hive.openshift.io", Version: "v1", Resource: "clusterdeployments",
	}
	CleanupClusterDeployment(ctx, dynamicClient, clusterDeploymentGVR, config.TestNamespace, clusterDeploymentName)

	// Cleanup secrets (but NOT the primary-cert-bundle-secret as it's managed by operator)
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
	certificateRequestGVR := schema.GroupVersionResource{
		Group: "certman.managed.openshift.io", Version: "v1alpha1", Resource: "certificaterequests",
	}

	crList, err := dynamicClient.Resource(certificateRequestGVR).Namespace(config.TestNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		ginkgo.GinkgoLogr.Error(err, "Failed to list CertificateRequests for cleanup")
	} else {
		for _, cr := range crList.Items {
			err := dynamicClient.Resource(certificateRequestGVR).Namespace(config.TestNamespace).Delete(ctx, cr.GetName(), metav1.DeleteOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				ginkgo.GinkgoLogr.Error(err, "Failed to cleanup CertificateRequest", "name", cr.GetName())
			} else if err == nil {
				ginkgo.GinkgoLogr.Info("Cleaned up CertificateRequest", "name", cr.GetName())
			}
		}
	}

	ginkgo.GinkgoLogr.Info("Test resource cleanup completed",
		"clusterName", config.ClusterName,
		"namespace", config.TestNamespace,
		"ocmClusterID", ocmClusterID)
}
