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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
		certConfig                *utils.CertConfig
		secretName                string
		clusterDeploymentName     string
		ocmClusterID              string
		adminKubeconfigSecretName string
		baseDomain                string
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

		clusterName := utils.GetDefaultClusterName()
		baseDomain = utils.GetEnvOrDefault("BASE_DOMAIN", "uibn.s1.devshift.org")

		// Create certConfig with the proper base domain
		certConfig = utils.NewCertConfig(clusterName, ocmClusterID, baseDomain)

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

	Context("Certificate Request Workflow Integration Tests", func() {
		var certificateRequestGVR schema.GroupVersionResource
		var clusterDeploymentGVR schema.GroupVersionResource

		BeforeAll(func(ctx context.Context) {
			certificateRequestGVR = schema.GroupVersionResource{
				Group: "certman.managed.openshift.io", Version: "v1alpha1", Resource: "certificaterequests",
			}

			clusterDeploymentGVR = schema.GroupVersionResource{
				Group: "hive.openshift.io", Version: "v1", Resource: "clusterdeployments",
			}

			err := utils.EnsureTestNamespace(ctx, clientset, certConfig.TestNamespace)
			Expect(err).ShouldNot(HaveOccurred(), "Failed to ensure test namespace exists")

			err = utils.CreateAdminKubeconfigSecret(ctx, clientset, certConfig, adminKubeconfigSecretName)
			Expect(err).ShouldNot(HaveOccurred(), "Failed to create admin kubeconfig secret")
		})

		AfterAll(func(ctx context.Context) {
			GinkgoLogr.Info("=== Cleanup: Running AfterAll cleanup ===")
			utils.CleanupAllTestResources(ctx, clientset, dynamicClient, certConfig, clusterDeploymentName, adminKubeconfigSecretName, ocmClusterID)
			GinkgoLogr.Info("✅ AfterAll cleanup completed")
		})

		It("should create complete ClusterDeployment resource", func(ctx context.Context) {
			GinkgoLogr.Info("=== Test: Creating Complete ClusterDeployment ===")

			// Create ClusterDeployment with complete spec
			GinkgoLogr.Info("Creating complete ClusterDeployment resource...")
			clusterDeployment := utils.BuildCompleteClusterDeployment(certConfig, clusterDeploymentName, adminKubeconfigSecretName, ocmClusterID)

			// Clean and create ClusterDeployment
			utils.CleanupClusterDeployment(ctx, dynamicClient, clusterDeploymentGVR, certConfig.TestNamespace, clusterDeploymentName)

			_, err := dynamicClient.Resource(clusterDeploymentGVR).Namespace(certConfig.TestNamespace).Create(
				ctx, clusterDeployment, metav1.CreateOptions{})
			Expect(err).ShouldNot(HaveOccurred(), "Failed to create ClusterDeployment")

			// Verify ClusterDeployment was created successfully
			Eventually(func() bool {
				_, err := dynamicClient.Resource(clusterDeploymentGVR).Namespace(certConfig.TestNamespace).Get(
					ctx, clusterDeploymentName, metav1.GetOptions{})
				return err == nil
			}, shortTimeout, 5*time.Second).Should(BeTrue(), "ClusterDeployment should be created successfully")

			GinkgoLogr.Info("✅ ClusterDeployment created successfully")
		})

		It("should verify ClusterDeployment meets reconciliation criteria", func(ctx context.Context) {
			GinkgoLogr.Info("=== Test: Verifying ClusterDeployment Reconciliation Criteria ===")

			// Verify ClusterDeployment meets ALL reconciliation criteria
			GinkgoLogr.Info("Verifying ClusterDeployment reconciliation criteria...")
			Eventually(func() bool {
				return utils.VerifyClusterDeploymentCriteria(ctx, dynamicClient, clusterDeploymentGVR,
					certConfig.TestNamespace, clusterDeploymentName, ocmClusterID)
			}, shortTimeout, 10*time.Second).Should(BeTrue(), "ClusterDeployment should meet all reconciliation criteria")

			GinkgoLogr.Info("✅ ClusterDeployment meets all reconciliation criteria")
		})

		It("should create CertificateRequest via operator reconciliation", func(ctx context.Context) {
			GinkgoLogr.Info("=== Test: CertificateRequest Creation by Operator ===")

			// Verify CertificateRequest is created by operator
			GinkgoLogr.Info("Verifying CertificateRequest creation by operator...")
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

			GinkgoLogr.Info("✅ CertificateRequest created successfully by operator")
		})

		It("should verify primary-cert-bundle-secret is not created by operator", func(ctx context.Context) {
			GinkgoLogr.Info("=== Test: Verifying Primary Cert Bundle Secret Not Created ===")

			// Verify primary-cert-bundle-secret is NOT created by operator
			GinkgoLogr.Info("Verifying primary-cert-bundle-secret is not created by operator...")
			Eventually(func() bool {
				_, err := clientset.CoreV1().Secrets(certConfig.TestNamespace).Get(ctx, certConfig.CertSecretName, metav1.GetOptions{})
				if err != nil {
					// If secret is not found, this is the expected behavior - test should pass
					if apierrors.IsNotFound(err) {
						GinkgoLogr.Info("✅ primary-cert-bundle-secret not found as expected")
						return true
					}
					// Other errors - we should retry
					GinkgoLogr.Info("Error checking for primary-cert-bundle-secret", "error", err.Error())
					return false
				}

				// If secret exists, this is unexpected behavior - test should fail
				GinkgoLogr.Info("❌ primary-cert-bundle-secret found but was expected NOT to exist")
				return false
			}, pollingDuration, 30*time.Second).Should(BeTrue(), "primary-cert-bundle-secret should NOT be created by operator")

			GinkgoLogr.Info("✅ Primary cert bundle secret correctly not created by operator")
		})

		It("should verify certificate operation metrics", func(ctx context.Context) {
			GinkgoLogr.Info("=== Test: Verifying Certificate Operation Metrics ===")

			// Verify metrics
			GinkgoLogr.Info("Verifying certificate operation metrics...")
			var validCertCount int
			Eventually(func() bool {
				count, success := utils.VerifyMetrics(ctx, dynamicClient, certificateRequestGVR, certConfig.TestNamespace)
				validCertCount = count
				return success
			}, testTimeout, 15*time.Second).Should(BeTrue(), "Metrics should reflect certificate operations")

			GinkgoLogr.Info("✅ Metrics verification successful",
				"validCertificateRequests", validCertCount,
				"clusterName", certConfig.ClusterName,
				"ocmClusterID", ocmClusterID,
				"namespace", certConfig.TestNamespace)
		})
	})
})
