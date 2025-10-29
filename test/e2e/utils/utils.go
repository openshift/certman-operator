package utils

import (
	"bytes"
	"context"
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
)

type CertConfig struct {
	ClusterName    string
	BaseDomain     string
	TestNamespace  string
	CertSecretName string
	OCMClusterID   string
}

func LoadTestConfig() *CertConfig {
	clusterName := GetEnvOrDefault("CLUSTER_NAME", "test-cluster")
	baseDomain := GetEnvOrDefault("BASE_DOMAIN", "example.com")
	ocmClusterID := GetEnvOrDefault("OCM_CLUSTER_ID", "test-cluster-id")

	return NewCertConfig(clusterName, ocmClusterID, baseDomain)
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
	gomega.Expect(labels).ToNot(gomega.BeNil(), "❌ Labels should not be nil")
	gomega.Expect(labels["api.openshift.com/managed"]).To(gomega.Equal("true"), "❌ Missing required managed label")

	// Check ClusterDeployment.Spec.Installed = True
	installed, found, _ := unstructured.NestedBool(cd.Object, "spec", "installed")
	gomega.Expect(found).To(gomega.BeTrue(), "❌ Installed field not found")
	gomega.Expect(installed).To(gomega.BeTrue(), "❌ Installed field not true")

	// Check NOT has annotation "hive.openshift.io/relocate" = "outgoing"
	annotations := cd.GetAnnotations()
	if annotations != nil {
		gomega.Expect(annotations["hive.openshift.io/relocate"]).ToNot(gomega.Equal("outgoing"),
			"❌ Has relocate annotation set to outgoing - this prevents reconciliation")
	}

	// Verify OCM cluster ID matches
	gomega.Expect(labels["api.openshift.com/id"]).To(gomega.Equal(ocmClusterID),
		"❌ OCM cluster ID mismatch", "expected", ocmClusterID, "actual", labels["api.openshift.com/id"])

	// Verify certificateBundles section exists
	certificateBundles, found, _ := unstructured.NestedSlice(cd.Object, "spec", "certificateBundles")
	gomega.Expect(found).To(gomega.BeTrue(), "❌ certificateBundles section not found")
	gomega.Expect(certificateBundles).ToNot(gomega.BeEmpty(), "❌ certificateBundles section is empty")

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

	_, err := kubeClient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		log.Printf("Namespace '%s' not found. Creating namespace", namespace)
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}

		if _, err := kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{}); err != nil {
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
	if awsAccessKey == "" || awsSecretKey == "" {
		return fmt.Errorf("AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY must be set")
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

	accesskey = SanitizeInput(GetEnvOrDefault("AWS_ACCESS_KEY", "testAccessKey"))
	secretkey = SanitizeInput(GetEnvOrDefault("AWS_SECRET_ACCESS_KEY", "testSecretAccessKey"))
	return

}

// G204 lint issue for exec.command
func SanitizeInput(input string) string {
	return "\"" + strings.ReplaceAll(input, "\"", "\\\"") + "\""
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
