package utils

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
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

func SetupHiveCRDs() error {
	tmpDir := "tmp/hive"
	repoURL := "https://github.com/openshift/hive.git"
	// Only clone if tmpDir does not exist
	if _, err := os.Stat(tmpDir); err != nil {
		cmd := exec.Command("git", "clone", "--depth=1", repoURL, tmpDir)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		cmd.Stdout = os.Stdout
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to clone Hive repo: %s: %w", stderr.String(), err)
		}
	} else {
		fmt.Printf("Directory %s already exists, skipping clone\n", tmpDir)
	}
	crdsPath := "tmp/hive/config/crds"
	cmd := exec.Command("oc", "apply", "-f", crdsPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to apply Hive CRDs: %w", err)
	}
	return nil
}
func SetupCertman(kerberosID string) error {
	tmpDir := "tmp/certman-operator"
	repoURL := "https://github.com/openshift/certman-operator.git"
	namespace := "certman-operator"
	configMapName := "certman-operator"
	email := fmt.Sprintf("%s@redhat.com", kerberosID)
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		fmt.Println("Cloning certman-operator repo...")
		cmd := exec.Command("git", "clone", "--depth=1", repoURL, tmpDir)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to clone certman-operator repo: %w", err)
		}
	} else {
		fmt.Println("Repo already cloned, skipping clone step")
	}
	if err := os.Chdir(tmpDir); err != nil {
		return fmt.Errorf("failed to change dir to %s: %w", tmpDir, err)
	}
	crdPath := "deploy/crds/certman.managed.openshift.io_certificaterequests.yaml"
	crdName := "certificaterequests.certman.managed.openshift.io"
	//  Create project/namespace if not exists
	if err := exec.Command("oc", "get", "namespace", namespace).Run(); err != nil {
		fmt.Printf("Namespace %s not found, creating...\n", namespace)
		if err := exec.Command("oc", "new-project", namespace).Run(); err != nil {
			return fmt.Errorf("failed to create namespace: %w", err)
		}
	} else {
		fmt.Printf("Namespace %s already exists\n", namespace)
	}
	// crete crds if not exists
	if err := exec.Command("oc", "get", "crd", crdName).Run(); err != nil {
		cmd := exec.Command("oc", "apply", "-f", crdPath)
		cmd.Env = os.Environ()
		cmd.Stdout = os.Stdout
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to apply CRD: %v\nstderr: %s", err, stderr.String())
		}
	}
	//create config map if not exixts
	cmd := exec.Command("oc", "get", "configmap", configMapName, "-n", namespace)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		email_notifier := fmt.Sprintf("default_notification_email_address=%s", email)
		cmd = exec.Command("oc", "create", "configmap", configMapName,
			"--from-literal", email_notifier,
			"-n", namespace)

		if err_create := cmd.Run(); err_create != nil {
			return fmt.Errorf("failed to create congifMap: %v", err_create)
		}
	}

	return nil
}
func InstallCertmanOperator() error {
	manifests := []string{
		"deploy/service_account.yaml",
		"deploy/role.yaml",
		"deploy/role_binding.yaml",
		"deploy/operator.yaml",
	}
	for _, relPath := range manifests {
		manifestPath := relPath
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			return fmt.Errorf("manifest not found: %s", manifestPath)
		}
		cmd := exec.Command("oc", "-n", "certman-operator", "apply", "-f", manifestPath)
		cmd.Stdout = os.Stdout
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to apply manifest %s: %v\nstderr: %s", relPath, err, stderr.String())
		}
		fmt.Printf("Applied: %s\n", relPath)
	}
	return nil
}
func SetupAWSCreds() error {
	namespace := "certman-operator"
	secretName := "aws"

	awsAccessKey, awsSecretKey := getSecretAndAccessKeys()

	if awsAccessKey == "" || awsSecretKey == "" {
		return fmt.Errorf("AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY must be set")
	}

	if err := exec.Command("oc", "-n", namespace, "get", "secret", secretName).Run(); err == nil {
		return nil
	}

	labelawsaccesskey := "--from-literal=aws_access_key_id=" + awsAccessKey
	labelawssecretkey := "--from-literal=aws_secret_access_key=" + awsSecretKey

	cmd := exec.Command("oc", "-n", namespace, "create", "secret", "generic", secretName, labelawsaccesskey, labelawssecretkey)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create AWS secret: %v", err)
	}

	return nil
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

// Retrieves the cluster external ID from the ClusterVersion resource.
func getClusterExternalIDFromClusterVersion(ctx context.Context, dynamicClient dynamic.Interface) (string, error) {
	clusterVersionGVR := schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "clusterversions",
	}

	// ClusterVersion is a cluster-scoped resource (no namespace)
	clusterVersion, err := dynamicClient.Resource(clusterVersionGVR).Get(ctx, "version", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("unable to retrieve ClusterVersion object: %w", err)
	}

	externalID, found, err := unstructured.NestedString(clusterVersion.Object, "spec", "clusterID")
	if err != nil {
		return "", fmt.Errorf("unable to read spec.clusterID: %w", err)
	}
	if !found || externalID == "" {
		return "", fmt.Errorf("spec.clusterID not found or is empty")
	}

	return externalID, nil
}

// Retrieves cluster ID and name from OCM based on the external ID from ClusterVersion.
func GetClusterInfoFromOCM(ctx context.Context, ocmConn *ocm.Client, dynamicClient dynamic.Interface) (string, string, error) {
	ginkgo.GinkgoLogr.Info("Fetching cluster info from OCM")

	externalID, err := getClusterExternalIDFromClusterVersion(ctx, dynamicClient)
	if err != nil {
		return "", "", fmt.Errorf("failed to retrieve external ID from ClusterVersion: %w", err)
	}

	ginkgo.GinkgoLogr.Info("External ID retrieved", "externalID", externalID)

	search := fmt.Sprintf("external_id = '%s'", externalID)
	response, err := ocmConn.ClustersMgmt().V1().Clusters().List().
		Search(search).
		Size(1).
		Send()
	if err != nil {
		return "", "", fmt.Errorf("error querying OCM for cluster by external ID: %w", err)
	}

	if response.Total() == 0 {
		return "", "", fmt.Errorf("no cluster found in OCM with external ID '%s'", externalID)
	}

	cluster := response.Items().Get(0)
	clusterID := cluster.ID()
	clusterName := cluster.Name()

	if clusterID == "" || clusterName == "" {
		return "", "", fmt.Errorf("cluster ID or name is empty for external ID '%s'", externalID)
	}

	ginkgo.GinkgoLogr.Info("Cluster found in OCM",
		"clusterID", clusterID,
		"clusterName", clusterName,
		"externalID", externalID,
	)

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
				"certificateBundles": []interface{}{
					map[string]interface{}{
						"certificateSecretRef": map[string]interface{}{
							"name": config.CertSecretName,
						},
						"generate": true,
						"name":     "primary-cert-bundle",
					},
				},
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

func VerifyClusterDeploymentCriteria(ctx context.Context, dynamicClient dynamic.Interface, gvr schema.GroupVersionResource, namespace, name, ocmClusterID string) bool {
	cd, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		ginkgo.GinkgoLogr.Error(err, "Failed to get ClusterDeployment")
		return false
	}

	labels := cd.GetLabels()
	if labels == nil || labels["api.openshift.com/managed"] != "true" {
		ginkgo.GinkgoLogr.Info("Missing required managed label")
		return false
	}

	installed, found, _ := unstructured.NestedBool(cd.Object, "spec", "installed")
	if !found || !installed {
		ginkgo.GinkgoLogr.Info("Installed field not set or false", "installed", installed)
		return false
	}

	annotations := cd.GetAnnotations()
	if annotations != nil && annotations["hive.openshift.io/relocate"] == "outgoing" {
		ginkgo.GinkgoLogr.Info("Relocate annotation is set to 'outgoing'")
		return false
	}

	if labels["api.openshift.com/id"] != ocmClusterID {
		ginkgo.GinkgoLogr.Info("OCM cluster ID mismatch", "expected", ocmClusterID, "actual", labels["api.openshift.com/id"])
		return false
	}

	certificateBundles, found, _ := unstructured.NestedSlice(cd.Object, "spec", "certificateBundles")
	if !found || len(certificateBundles) == 0 {
		ginkgo.GinkgoLogr.Info("Missing certificateBundles section")
		return false
	}

	ginkgo.GinkgoLogr.Info("All ClusterDeployment reconciliation criteria met")
	return true
}

func EnsureTestNamespace(ctx context.Context, clientset *kubernetes.Clientset, namespace string) error {
	_, err := clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
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

func CleanupClusterDeployment(ctx context.Context, dynamicClient dynamic.Interface, gvr schema.GroupVersionResource, namespace, name string) {
	err := dynamicClient.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		ginkgo.GinkgoLogr.Error(err, "Failed to cleanup ClusterDeployment", "name", name)
	} else if err == nil {
		ginkgo.GinkgoLogr.Info("Cleaned up existing ClusterDeployment", "name", name)
		time.Sleep(5 * time.Second)
	}
}

func VerifyMetrics(ctx context.Context, dynamicClient dynamic.Interface, certificateRequestGVR schema.GroupVersionResource, namespace string) (int, bool) {
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

	for _, cr := range crList.Items {
		dnsNames, found, _ := unstructured.NestedStringSlice(cr.Object, "spec", "dnsNames")
		if !found || len(dnsNames) == 0 {
			ginkgo.GinkgoLogr.Info("CertificateRequest missing dnsNames", "name", cr.GetName())
			continue
		}

		email, found, _ := unstructured.NestedString(cr.Object, "spec", "email")
		if !found || email == "" {
			ginkgo.GinkgoLogr.Info("CertificateRequest missing email", "name", cr.GetName())
			continue
		}

		validCount++
		ginkgo.GinkgoLogr.Info("Found valid CertificateRequest",
			"name", cr.GetName(),
			"dnsNames", len(dnsNames),
			"email", email)
	}

	if validCount > 0 {
		ginkgo.GinkgoLogr.Info("Metrics validation successful", "validCertificateRequests", validCount, "totalFound", len(crList.Items))
		return validCount, true
	}

	ginkgo.GinkgoLogr.Info("No valid CertificateRequests found", "totalFound", len(crList.Items))
	return 0, false
}

func CleanupAllTestResources(ctx context.Context, clientset *kubernetes.Clientset, dynamicClient dynamic.Interface, config *CertConfig, clusterDeploymentName, adminKubeconfigSecretName, ocmClusterID string) {
	clusterDeploymentGVR := schema.GroupVersionResource{
		Group: "hive.openshift.io", Version: "v1", Resource: "clusterdeployments",
	}
	CleanupClusterDeployment(ctx, dynamicClient, clusterDeploymentGVR, config.TestNamespace, clusterDeploymentName)

	secrets := []string{adminKubeconfigSecretName}
	for _, secretName := range secrets {
		err := clientset.CoreV1().Secrets(config.TestNamespace).Delete(ctx, secretName, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			ginkgo.GinkgoLogr.Error(err, "Failed to cleanup secret", "secretName", secretName)
		} else if err == nil {
			ginkgo.GinkgoLogr.Info("Cleaned up secret", "secretName", secretName)
		}
	}

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

func getSecretAndAccessKeys() (accesskey, secretkey string) {

	accesskey = SanitizeInput(os.Getenv("AWS_ACCESS_KEY"))
	secretkey = SanitizeInput(os.Getenv("AWS_SECRET_ACCESS_KEY"))
	return

}

func GetKerberosId() (kebros string) {
	kebros = SanitizeInput(os.Getenv("KERBEROS_ID"))
	return
}

// G204 lint issue for exec.command
func SanitizeInput(input string) string {
	return "\"" + strings.ReplaceAll(input, "\"", "\\\"") + "\""
}
