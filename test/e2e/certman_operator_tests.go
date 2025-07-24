//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift/certman-operator/test/e2e/utils"
	configv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Certman Operator", Ordered, func() {
	var (
		k8s                       *openshift.Client
		clientset                 *kubernetes.Clientset
		dynamicClient             dynamic.Interface
		certManager               *utils.CertificateManager
		certConfig                *utils.CertConfig
		secretName                string
		clusterDeploymentName     string
		ocmClusterID              string
		adminKubeconfigSecretName string
	)

	const (
		pollingDuration = 15 * time.Minute
		namespace       = "openshift-config"
		testTimeout     = 5 * time.Minute
		shortTimeout    = 2 * time.Minute
	)

	BeforeAll(func(ctx context.Context) {
		log.SetLogger(GinkgoLogr)
		var err error

		ocmClusterID = os.Getenv("OCM_CLUSTER_ID")
		Expect(ocmClusterID).ShouldNot(BeEmpty(), "OCM_CLUSTER_ID environment variable must be set")

		k8s, err = openshift.New(GinkgoLogr)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to setup k8s client")

		clientset, err = kubernetes.NewForConfig(k8s.GetConfig())
		Expect(err).ShouldNot(HaveOccurred(), "Unable to setup Config client")

		dynamicClient, err = dynamic.NewForConfig(k8s.GetConfig())
		Expect(err).ShouldNot(HaveOccurred(), "Unable to setup dynamic client")

		certManager = utils.NewCertificateManager(clientset)
		clusterName := utils.GetDefaultClusterName()
		certConfig = utils.NewCertConfig(clusterName, ocmClusterID)
		clusterDeploymentName = clusterName
		adminKubeconfigSecretName = fmt.Sprintf("%s-admin-kubeconfig", clusterName)

		GinkgoLogr.Info("Test configuration initialized",
			"clusterName", certConfig.ClusterName,
			"testNamespace", certConfig.TestNamespace,
			"baseDomain", certConfig.BaseDomain,
			"ocmClusterID", ocmClusterID)
	})

	It("certificate secret exists under openshift-config namespace", func(ctx context.Context) {
		Eventually(func() bool {
			listOpts := metav1.ListOptions{LabelSelector: "certificate_request"}
			secrets, err := clientset.CoreV1().Secrets(namespace).List(ctx, listOpts)
			if err != nil || len(secrets.Items) < 1 {
				return false
			}
			secretName = secrets.Items[0].Name
			return true
		}, pollingDuration, 30*time.Second).Should(BeTrue(), "Certificate secret should exist under openshift-config namespace")
	})

	It("certificate secret should be applied to apiserver object", func(ctx context.Context) {
		Eventually(func() bool {
			configClient, err := configv1.NewForConfig(k8s.GetConfig())
			if err != nil {
				return false
			}
			apiserver, err := configClient.APIServers().Get(ctx, "cluster", metav1.GetOptions{})
			if err != nil || len(apiserver.Spec.ServingCerts.NamedCertificates) < 1 {
				return false
			}
			return apiserver.Spec.ServingCerts.NamedCertificates[0].ServingCertificate.Name == secretName
		}, pollingDuration, 30*time.Second).Should(BeTrue(), "Certificate secret should be applied to apiserver object")
	})

	Context("Complete CertificateRequest Integration Test", func() {
		var certificateRequestGVR schema.GroupVersionResource

		BeforeAll(func(ctx context.Context) {
			certificateRequestGVR = schema.GroupVersionResource{
				Group: "certman.managed.openshift.io", Version: "v1alpha1", Resource: "certificaterequests",
			}

			err := utils.EnsureTestNamespace(ctx, clientset, certConfig.TestNamespace)
			Expect(err).ShouldNot(HaveOccurred(), "Failed to ensure test namespace exists")

			err = utils.CreateAdminKubeconfigSecret(ctx, clientset, certConfig, adminKubeconfigSecretName)
			Expect(err).ShouldNot(HaveOccurred(), "Failed to create admin kubeconfig secret")
		})

		It("should complete full certificate integration workflow", func(ctx context.Context) {
			GinkgoLogr.Info("=== Starting Complete Certificate Integration Test ===")

			// Step 1: Create certificate secret as per requirements
			GinkgoLogr.Info("Step 1: Creating primary certificate bundle secret...")
			certData, keyData, err := certManager.GetCertificateData(ctx, certConfig)
			Expect(err).ShouldNot(HaveOccurred(), "Failed to get certificate data")

			err = certManager.CreatePrimaryCertBundleSecret(ctx, certConfig, certData, keyData)
			Expect(err).ShouldNot(HaveOccurred(), "Failed to create primary cert bundle secret")
			GinkgoLogr.Info("✅ Step 1 PASSED: Primary cert bundle secret created")

			// Step 2: Create ClusterDeployment with complete spec
			GinkgoLogr.Info("Step 2: Creating complete ClusterDeployment resource...")
			clusterDeployment := utils.BuildCompleteClusterDeployment(certConfig, clusterDeploymentName, adminKubeconfigSecretName, ocmClusterID)

			clusterDeploymentGVR := schema.GroupVersionResource{
				Group: "hive.openshift.io", Version: "v1", Resource: "clusterdeployments",
			}

			// Clean and create ClusterDeployment
			utils.CleanupClusterDeployment(ctx, dynamicClient, clusterDeploymentGVR, certConfig.TestNamespace, clusterDeploymentName)

			_, err = dynamicClient.Resource(clusterDeploymentGVR).Namespace(certConfig.TestNamespace).Create(
				ctx, clusterDeployment, metav1.CreateOptions{})
			Expect(err).ShouldNot(HaveOccurred(), "Failed to create ClusterDeployment")
			GinkgoLogr.Info("✅ Step 2 PASSED: Complete ClusterDeployment created")

			// Step 3: Verify ClusterDeployment meets ALL reconciliation criteria
			GinkgoLogr.Info("Step 3: Verifying ClusterDeployment reconciliation criteria...")
			Eventually(func() bool {
				return utils.VerifyClusterDeploymentCriteria(ctx, dynamicClient, clusterDeploymentGVR,
					certConfig.TestNamespace, clusterDeploymentName, ocmClusterID)
			}, shortTimeout, 10*time.Second).Should(BeTrue(), "ClusterDeployment should meet all reconciliation criteria")
			GinkgoLogr.Info("✅ Step 3 PASSED: ClusterDeployment meets reconciliation criteria")

			// Step 4: Verify CertificateRequest is created (main requirement)
			GinkgoLogr.Info("Step 4: Verifying CertificateRequest creation...")
			Eventually(func() bool {
				crList, err := dynamicClient.Resource(certificateRequestGVR).Namespace(certConfig.TestNamespace).List(ctx, metav1.ListOptions{})
				if err != nil {
					GinkgoLogr.Error(err, "Failed to list CertificateRequests")
					return false
				}

				// List all CertificateRequests
				GinkgoLogr.Info("Found CertificateRequests", "totalCRs", len(crList.Items))
				for _, cr := range crList.Items {
					GinkgoLogr.Info("CertificateRequest found", "name", cr.GetName())
				}

				// Return true if any CertificateRequests exist
				return len(crList.Items) > 0
			}, pollingDuration, 30*time.Second).Should(BeTrue(), "CertificateRequest should be created by operator")
			GinkgoLogr.Info("✅ Step 4 PASSED: CertificateRequest created successfully")

			// Step 5: Verify certificate is issued in primary-cert-bundle-secret
			GinkgoLogr.Info("Step 5: Verifying certificate issuance...")
			Eventually(func() bool {
				secret, err := clientset.CoreV1().Secrets(certConfig.TestNamespace).Get(ctx, certConfig.CertSecretName, metav1.GetOptions{})
				if err != nil {
					GinkgoLogr.Error(err, "Failed to get primary-cert-bundle-secret")
					return false
				}

				certData, certExists := secret.Data["tls.crt"]
				keyData, keyExists := secret.Data["tls.key"]

				if !certExists || !keyExists || len(certData) == 0 || len(keyData) == 0 {
					GinkgoLogr.Info("Certificate or key data missing or empty")
					return false
				}

				// Validate certificate is properly issued
				err = utils.ValidateIssuedCertificate(certData, keyData, certConfig)
				if err != nil {
					GinkgoLogr.Error(err, "Certificate validation failed")
					return false
				}

				return true
			}, pollingDuration, 30*time.Second).Should(BeTrue(), "Certificate should be issued successfully")
			GinkgoLogr.Info("✅ Step 5 PASSED: Certificate issued and validated")

			GinkgoLogr.Info("Step 6: Verifying metrics...")
			var validCertCount int
			Eventually(func() bool {
				count, success := utils.VerifyMetrics(ctx, dynamicClient, certificateRequestGVR, certConfig.TestNamespace)
				validCertCount = count
				return success
			}, testTimeout, 15*time.Second).Should(BeTrue(), "Metrics should reflect certificate operations")

			GinkgoLogr.Info("✅ Step 6 PASSED: Metrics verification successful",
				"validCertificateRequests", validCertCount)
			GinkgoLogr.Info("🎉 === COMPLETE INTEGRATION TEST PASSED ===",
				"clusterName", certConfig.ClusterName,
				"ocmClusterID", ocmClusterID,
				"namespace", certConfig.TestNamespace,
				"certificateRequestsFound", validCertCount)
		})

		AfterAll(func(ctx context.Context) {
			GinkgoLogr.Info("Cleaning up test resources...")
			utils.CleanupAllTestResources(ctx, clientset, dynamicClient, certConfig, clusterDeploymentName, adminKubeconfigSecretName, ocmClusterID)
			GinkgoLogr.Info("Cleanup completed")
		})
	})
})
