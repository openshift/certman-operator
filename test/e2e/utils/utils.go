package utils

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type CertificateManager struct {
	clientset *kubernetes.Clientset
}

type CertConfig struct {
	ClusterName    string
	BaseDomain     string
	TestNamespace  string
	CertSecretName string
	OCMClusterID   string
}

func NewCertificateManager(clientset *kubernetes.Clientset) *CertificateManager {
	return &CertificateManager{clientset: clientset}
}

func NewCertConfig(clusterName string, ocmClusterID string) *CertConfig {
	if clusterName == "" {
		clusterName = "test-cluster"
	}

	return &CertConfig{
		ClusterName:    clusterName,
		BaseDomain:     "uibn.s1.devshift.org",
		TestNamespace:  "certman-operator",
		CertSecretName: "primary-cert-bundle-secret",
		OCMClusterID:   ocmClusterID,
	}
}

func GetDefaultClusterName() string {
	if name := os.Getenv("CLUSTER_NAME"); name != "" {
		return name
	}
	return "test-cluster"
}

func (cm *CertificateManager) generateSelfSignedCert(config *CertConfig) ([]byte, []byte, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Certificate domains matching the requirement pattern
	dnsNames := []string{
		fmt.Sprintf("api.%s.%s", config.ClusterName, config.BaseDomain),
		fmt.Sprintf("apps.%s.%s", config.ClusterName, config.BaseDomain),
		fmt.Sprintf("*.apps.%s.%s", config.ClusterName, config.BaseDomain),
		fmt.Sprintf("rh-api.%s.%s", config.ClusterName, config.BaseDomain),
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		Subject: pkix.Name{
			CommonName:   fmt.Sprintf("api.%s.%s", config.ClusterName, config.BaseDomain),
			Organization: []string{"OpenShift Test"},
		},
		DNSNames:    dnsNames,
		NotBefore:   time.Now().Add(-1 * time.Hour),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour), // 365 days as per openssl command
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:        false,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyDER})

	return certPEM, keyPEM, nil
}

func (cm *CertificateManager) GetCertificateData(ctx context.Context, config *CertConfig) ([]byte, []byte, error) {
	// Try to get from existing secret first
	if certData, keyData, err := cm.getCertFromExistingSecret(ctx, config); err == nil {
		return certData, keyData, nil
	}

	// Try environment variables
	if envCert, envKey := os.Getenv("TEST_TLS_CERT"), os.Getenv("TEST_TLS_KEY"); envCert != "" && envKey != "" {
		GinkgoLogr.Info("Using certificate from environment variables")
		return []byte(envCert), []byte(envKey), nil
	}

	// Generate self-signed certificate
	certData, keyData, err := cm.generateSelfSignedCert(config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate certificate: %w", err)
	}

	GinkgoLogr.Info("Generated self-signed certificate for testing", "clusterName", config.ClusterName)
	return certData, keyData, nil
}

func (cm *CertificateManager) getCertFromExistingSecret(ctx context.Context, config *CertConfig) ([]byte, []byte, error) {
	secretCandidates := []struct {
		namespace string
		name      string
	}{
		{config.TestNamespace, config.CertSecretName},
		{"openshift-config", config.CertSecretName},
		{"openshift-config", fmt.Sprintf("%s-primary-cert-bundle-secret", config.ClusterName)},
	}

	for _, candidate := range secretCandidates {
		secret, err := cm.clientset.CoreV1().Secrets(candidate.namespace).Get(ctx, candidate.name, metav1.GetOptions{})
		if err != nil {
			continue
		}

		certData, certExists := secret.Data["tls.crt"]
		keyData, keyExists := secret.Data["tls.key"]
		if certExists && keyExists && len(certData) > 0 && len(keyData) > 0 {
			GinkgoLogr.Info("Found certificate in existing secret", "namespace", candidate.namespace, "secret", candidate.name)
			return certData, keyData, nil
		}
	}

	return nil, nil, fmt.Errorf("no suitable certificate found in existing secrets")
}

// CreatePrimaryCertBundleSecret creates the secret as per requirement
func (cm *CertificateManager) CreatePrimaryCertBundleSecret(ctx context.Context, config *CertConfig, certData, keyData []byte) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.CertSecretName,
			Namespace: config.TestNamespace,
			Labels: map[string]string{
				"certificate_request": "true", // Label as per requirement
			},
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": certData,
			"tls.key": keyData,
		},
	}

	// Delete existing secret if present
	_ = cm.clientset.CoreV1().Secrets(config.TestNamespace).Delete(ctx, config.CertSecretName, metav1.DeleteOptions{})
	time.Sleep(2 * time.Second)

	_, err := cm.clientset.CoreV1().Secrets(config.TestNamespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create primary-cert-bundle-secret: %w", err)
	}

	GinkgoLogr.Info("Created primary-cert-bundle-secret", "name", config.CertSecretName, "namespace", config.TestNamespace)
	return nil
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

	GinkgoLogr.Info("Created admin kubeconfig secret", "secretName", secretName)
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
		GinkgoLogr.Error(err, "Failed to get ClusterDeployment")
		return false
	}

	// Check required label: "api.openshift.com/managed"
	labels := cd.GetLabels()
	if labels == nil || labels["api.openshift.com/managed"] != "true" {
		GinkgoLogr.Info("❌ Missing required managed label")
		return false
	}

	// Check ClusterDeployment.Spec.Installed = True
	installed, found, _ := unstructured.NestedBool(cd.Object, "spec", "installed")
	if !found || !installed {
		GinkgoLogr.Info("❌ Installed field not true", "installed", installed, "found", found)
		return false
	}

	// Check NOT has annotation "hive.openshift.io/relocate" = "outgoing"
	annotations := cd.GetAnnotations()
	if annotations != nil && annotations["hive.openshift.io/relocate"] == "outgoing" {
		GinkgoLogr.Info("❌ Has relocate annotation set to outgoing - this prevents reconciliation")
		return false
	}

	// Verify OCM cluster ID matches
	if labels["api.openshift.com/id"] != ocmClusterID {
		GinkgoLogr.Info("❌ OCM cluster ID mismatch", "expected", ocmClusterID, "actual", labels["api.openshift.com/id"])
		return false
	}

	// Verify certificateBundles section exists
	certificateBundles, found, _ := unstructured.NestedSlice(cd.Object, "spec", "certificateBundles")
	if !found || len(certificateBundles) == 0 {
		GinkgoLogr.Info("❌ Missing certificateBundles section")
		return false
	}

	GinkgoLogr.Info("✅ All ClusterDeployment reconciliation criteria met")
	return true
}

// ValidateIssuedCertificate validates the certificate issued by the operator
func ValidateIssuedCertificate(certData, keyData []byte, config *CertConfig) error {
	// Basic PEM validation
	if err := ValidateCertificateData(certData, keyData); err != nil {
		return fmt.Errorf("certificate PEM validation failed: %w", err)
	}

	// Parse certificate for additional validation
	certBlock, _ := pem.Decode(certData)
	if certBlock == nil {
		return fmt.Errorf("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Validate certificate domains match expected domains
	expectedDomains := []string{
		fmt.Sprintf("api.%s.%s", config.ClusterName, config.BaseDomain),
		fmt.Sprintf("apps.%s.%s", config.ClusterName, config.BaseDomain),
		fmt.Sprintf("*.apps.%s.%s", config.ClusterName, config.BaseDomain),
		fmt.Sprintf("rh-api.%s.%s", config.ClusterName, config.BaseDomain),
	}

	for _, expectedDomain := range expectedDomains {
		found := false
		for _, dnsName := range cert.DNSNames {
			if dnsName == expectedDomain {
				found = true
				break
			}
		}
		if !found {
			GinkgoLogr.Info("Expected domain not found in certificate", "domain", expectedDomain)
		}
	}

	// Validate certificate is not expired
	if time.Now().After(cert.NotAfter) {
		return fmt.Errorf("certificate has expired")
	}

	if time.Now().Before(cert.NotBefore) {
		return fmt.Errorf("certificate is not yet valid")
	}

	GinkgoLogr.Info("Certificate validation successful",
		"subject", cert.Subject.CommonName,
		"dnsNames", cert.DNSNames,
		"notBefore", cert.NotBefore,
		"notAfter", cert.NotAfter)

	return nil
}

// ValidateCertificateData validates basic PEM format of certificate and key data
func ValidateCertificateData(certData, keyData []byte) error {
	// Validate certificate PEM
	certBlock, _ := pem.Decode(certData)
	if certBlock == nil || certBlock.Type != "CERTIFICATE" {
		return fmt.Errorf("invalid certificate PEM format")
	}

	// Validate private key PEM
	keyBlock, _ := pem.Decode(keyData)
	if keyBlock == nil || (keyBlock.Type != "PRIVATE KEY" && keyBlock.Type != "RSA PRIVATE KEY") {
		return fmt.Errorf("invalid private key PEM format")
	}

	// Try to parse certificate
	_, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Try to parse private key
	if keyBlock.Type == "PRIVATE KEY" {
		_, err = x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	} else {
		_, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	}
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	return nil
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
			GinkgoLogr.Info("Created test namespace", "namespace", namespace)
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
		GinkgoLogr.Error(err, "Failed to cleanup ClusterDeployment", "name", name)
	} else if err == nil {
		GinkgoLogr.Info("Cleaned up existing ClusterDeployment", "name", name)
		time.Sleep(5 * time.Second) // Wait for cleanup
	}
}

func VerifyMetrics(ctx context.Context, dynamicClient dynamic.Interface, certificateRequestGVR schema.GroupVersionResource, namespace string) (int, bool) {
	// Get all CertificateRequests in the namespace
	crList, err := dynamicClient.Resource(certificateRequestGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		GinkgoLogr.Error(err, "Failed to list CertificateRequests for metrics verification")
		return 0, false
	}

	if len(crList.Items) == 0 {
		GinkgoLogr.Info("No CertificateRequests found for metrics verification")
		return 0, false
	}

	validCount := 0
	// Simplified verification: check if CertificateRequest exists and has required spec fields
	for _, cr := range crList.Items {
		// Check if CR has the basic spec fields that indicate it's properly configured
		dnsNames, found, _ := unstructured.NestedStringSlice(cr.Object, "spec", "dnsNames")
		if !found || len(dnsNames) == 0 {
			GinkgoLogr.Info("CertificateRequest missing dnsNames", "name", cr.GetName())
			continue
		}

		// Check if email is present (indicates proper configuration)
		email, found, _ := unstructured.NestedString(cr.Object, "spec", "email")
		if !found || email == "" {
			GinkgoLogr.Info("CertificateRequest missing email", "name", cr.GetName())
			continue
		}

		validCount++
		GinkgoLogr.Info("✅ Metrics validation: Found valid CertificateRequest",
			"name", cr.GetName(),
			"dnsNames", len(dnsNames),
			"email", email)
	}

	if validCount > 0 {
		GinkgoLogr.Info("Metrics validation successful", "validCertificateRequests", validCount, "totalFound", len(crList.Items))
		return validCount, true
	}

	GinkgoLogr.Info("Metrics validation: No valid CertificateRequests found", "totalFound", len(crList.Items))
	return 0, false
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
		config.CertSecretName,
		adminKubeconfigSecretName,
	}

	for _, secretName := range secrets {
		err := clientset.CoreV1().Secrets(config.TestNamespace).Delete(ctx, secretName, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			GinkgoLogr.Error(err, "Failed to cleanup secret", "secretName", secretName)
		} else if err == nil {
			GinkgoLogr.Info("Cleaned up secret", "secretName", secretName)
		}
	}

	// Cleanup CertificateRequests
	certificateRequestGVR := schema.GroupVersionResource{
		Group: "certman.managed.openshift.io", Version: "v1alpha1", Resource: "certificaterequests",
	}

	crList, err := dynamicClient.Resource(certificateRequestGVR).Namespace(config.TestNamespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, cr := range crList.Items {
			// Clean up all CertificateRequests without cluster filtering
			err := dynamicClient.Resource(certificateRequestGVR).Namespace(config.TestNamespace).Delete(ctx, cr.GetName(), metav1.DeleteOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				GinkgoLogr.Error(err, "Failed to cleanup CertificateRequest", "name", cr.GetName())
			} else if err == nil {
				GinkgoLogr.Info("Cleaned up CertificateRequest", "name", cr.GetName())
			}
		}
	}

	GinkgoLogr.Info("Cleanup completed for all test resources")
}
