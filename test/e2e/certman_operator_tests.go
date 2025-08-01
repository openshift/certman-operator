// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS.
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"fmt"

	"os"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/openshift/certman-operator/test/e2e/utils"
	configv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"

	"github.com/openshift/osde2e-common/pkg/clients/ocm"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = ginkgo.Describe("Certman Operator", ginkgo.Ordered, ginkgo.ContinueOnFailure, func() {
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
		clusterName               string
	)

	const (
		pollingDuration = 15 * time.Minute
		namespace       = "openshift-config"
		testTimeout     = 5 * time.Minute
		shortTimeout    = 2 * time.Minute
	)

	ginkgo.BeforeAll(func(ctx context.Context) {
		log.SetLogger(ginkgo.GinkgoLogr)
		var err error

		// Initialize clients

		var ocmUrl ocm.Environment

		clientID := os.Getenv("OCM_CLIENT_ID")
		clientSecret := os.Getenv("OCM_CLIENT_SECRET")
		ocmUrl = ocm.Stage
		k8s, err = openshift.New(ginkgo.GinkgoLogr)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Unable to setup k8s client")

		ocmConn, err := ocm.New(ctx, "", clientID, clientSecret, ocmUrl)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Unable to setup OCM client")
		ginkgo.DeferCleanup(ocmConn.Connection.Close)

		clientset, err = kubernetes.NewForConfig(k8s.GetConfig())
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Unable to setup Config client")

		dynamicClient, err = dynamic.NewForConfig(k8s.GetConfig())
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Unable to setup dynamic client")

		// Get cluster configuration using utils functions
		cluster, err := ocmConn.ClustersMgmt().V1().Clusters().Cluster(ocmClusterID).Get().Send()
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		clusterName = cluster.Body().Name()

		baseDomain = utils.GetEnvOrDefault("BASE_DOMAIN", "uibn.s1.devshift.org")

		ocmClusterID, err = utils.GetClusterIDFromClusterVersion(ctx, dynamicClient)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to get cluster ID from ClusterVersion and OCM_CLUSTER_ID not set")
		ginkgo.GinkgoLogr.Info("Retrieved cluster ID from ClusterVersion", "clusterID", ocmClusterID)

		gomega.Expect(ocmClusterID).ShouldNot(gomega.BeEmpty(), "OCM cluster ID must be available")

		// Create certConfig with the proper configuration
		certConfig = utils.NewCertConfig(clusterName, ocmClusterID, baseDomain)

		clusterDeploymentName = clusterName
		adminKubeconfigSecretName = fmt.Sprintf("%s-admin-kubeconfig", clusterName)

		ginkgo.GinkgoLogr.Info("Test configuration initialized",
			"clusterName", certConfig.ClusterName,
			"testNamespace", certConfig.TestNamespace,
			"baseDomain", certConfig.BaseDomain,
			"ocmClusterID", ocmClusterID,
			"certSecretName", certConfig.CertSecretName)
	})

	ginkgo.It("certificate secret exists under openshift-config namespace", func(ctx context.Context) {
		gomega.Eventually(func() bool {
			listOpts := metav1.ListOptions{LabelSelector: "certificate_request"}
			secrets, err := clientset.CoreV1().Secrets(namespace).List(ctx, listOpts)
			if err != nil || len(secrets.Items) < 1 {
				return false
			}
			secretName = secrets.Items[0].Name
			return true
		}, pollingDuration, 30*time.Second).Should(gomega.BeTrue(), "Certificate secret should exist under openshift-config namespace")
	})

	ginkgo.It("certificate secret should be applied to apiserver object", func(ctx context.Context) {
		gomega.Eventually(func() bool {
			configClient, err := configv1.NewForConfig(k8s.GetConfig())
			if err != nil {
				return false
			}
			apiserver, err := configClient.APIServers().Get(ctx, "cluster", metav1.GetOptions{})
			if err != nil || len(apiserver.Spec.ServingCerts.NamedCertificates) < 1 {
				return false
			}
			return apiserver.Spec.ServingCerts.NamedCertificates[0].ServingCertificate.Name == secretName
		}, pollingDuration, 30*time.Second).Should(gomega.BeTrue(), "Certificate secret should be applied to apiserver object")
	})

	ginkgo.Context("Certificate Request Workflow Integration Tests", func() {
		var certificateRequestGVR schema.GroupVersionResource
		var clusterDeploymentGVR schema.GroupVersionResource

		ginkgo.BeforeAll(func(ctx context.Context) {
			// Initialize GVRs using the same pattern as utils
			certificateRequestGVR = schema.GroupVersionResource{
				Group: "certman.managed.openshift.io", Version: "v1alpha1", Resource: "certificaterequests",
			}

			clusterDeploymentGVR = schema.GroupVersionResource{
				Group: "hive.openshift.io", Version: "v1", Resource: "clusterdeployments",
			}

			// Ensure test namespace exists using utils function
			err := utils.EnsureTestNamespace(ctx, clientset, certConfig.TestNamespace)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to ensure test namespace exists")

			// Create admin kubeconfig secret using utils function
			err = utils.CreateAdminKubeconfigSecret(ctx, clientset, certConfig, adminKubeconfigSecretName)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to create admin kubeconfig secret")

			ginkgo.GinkgoLogr.Info("Test setup completed",
				"namespace", certConfig.TestNamespace,
				"adminSecret", adminKubeconfigSecretName,
				"clusterDeployment", clusterDeploymentName)
		})

		ginkgo.AfterAll(func(ctx context.Context) {
			ginkgo.GinkgoLogr.Info("=== Cleanup: Running AfterAll cleanup ===")
			// Use the comprehensive cleanup function from utils
			utils.CleanupAllTestResources(ctx, clientset, dynamicClient, certConfig,
				clusterDeploymentName, adminKubeconfigSecretName, ocmClusterID)
			ginkgo.GinkgoLogr.Info("✅ AfterAll cleanup completed")
		})

		ginkgo.It("should create complete ClusterDeployment resource", func(ctx context.Context) {
			ginkgo.GinkgoLogr.Info("=== Test: Creating Complete ClusterDeployment ===")

			// Create ClusterDeployment using utils function
			ginkgo.GinkgoLogr.Info("Creating complete ClusterDeployment resource...")
			clusterDeployment := utils.BuildCompleteClusterDeployment(certConfig, clusterDeploymentName, adminKubeconfigSecretName, ocmClusterID)

			// Clean and create ClusterDeployment using utils function
			utils.CleanupClusterDeployment(ctx, dynamicClient, clusterDeploymentGVR, certConfig.TestNamespace, clusterDeploymentName)

			_, err := dynamicClient.Resource(clusterDeploymentGVR).Namespace(certConfig.TestNamespace).Create(
				ctx, clusterDeployment, metav1.CreateOptions{})
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to create ClusterDeployment")

			// Verify ClusterDeployment was created successfully
			gomega.Eventually(func() bool {
				_, err := dynamicClient.Resource(clusterDeploymentGVR).Namespace(certConfig.TestNamespace).Get(
					ctx, clusterDeploymentName, metav1.GetOptions{})
				return err == nil
			}, shortTimeout, 5*time.Second).Should(gomega.BeTrue(), "ClusterDeployment should be created successfully")

			ginkgo.GinkgoLogr.Info("✅ ClusterDeployment created successfully")
		})

		ginkgo.It("should verify ClusterDeployment meets reconciliation criteria", func(ctx context.Context) {
			ginkgo.GinkgoLogr.Info("=== Test: Verifying ClusterDeployment Reconciliation Criteria ===")

			// Verify ClusterDeployment meets ALL reconciliation criteria using utils function
			ginkgo.GinkgoLogr.Info("Verifying ClusterDeployment reconciliation criteria...")
			gomega.Eventually(func() bool {
				return utils.VerifyClusterDeploymentCriteria(ctx, dynamicClient, clusterDeploymentGVR,
					certConfig.TestNamespace, clusterDeploymentName, ocmClusterID)
			}, shortTimeout, 10*time.Second).Should(gomega.BeTrue(), "ClusterDeployment should meet all reconciliation criteria")

			ginkgo.GinkgoLogr.Info("✅ ClusterDeployment meets all reconciliation criteria")
		})

		ginkgo.It("should create CertificateRequest via operator reconciliation", func(ctx context.Context) {
			ginkgo.GinkgoLogr.Info("=== Test: CertificateRequest Creation by Operator ===")

			// Verify CertificateRequest is created by operator
			ginkgo.GinkgoLogr.Info("Verifying CertificateRequest creation by operator...")
			gomega.Eventually(func() bool {
				crList, err := dynamicClient.Resource(certificateRequestGVR).Namespace(certConfig.TestNamespace).List(ctx, metav1.ListOptions{})
				if err != nil {
					ginkgo.GinkgoLogr.Error(err, "Failed to list CertificateRequests")
					return false
				}

				// List all CertificateRequests for debugging
				ginkgo.GinkgoLogr.Info("Found CertificateRequests", "totalCRs", len(crList.Items))
				for _, cr := range crList.Items {
					ginkgo.GinkgoLogr.Info("CertificateRequest found", "name", cr.GetName())
				}

				// Return true if any CertificateRequests exist
				return len(crList.Items) > 0
			}, pollingDuration, 30*time.Second).Should(gomega.BeTrue(), "CertificateRequest should be created by operator")

			ginkgo.GinkgoLogr.Info("✅ CertificateRequest created successfully by operator")
		})

		ginkgo.It("should verify primary-cert-bundle-secret is not created by operator", func(ctx context.Context) {
			ginkgo.GinkgoLogr.Info("=== Test: Verifying Primary Cert Bundle Secret Not Created ===")

			// Verify primary-cert-bundle-secret is NOT created by operator
			// This aligns with the utils pattern where this secret is expected NOT to be created
			ginkgo.GinkgoLogr.Info("Verifying primary-cert-bundle-secret is not created by operator...")
			gomega.Eventually(func() bool {
				_, err := clientset.CoreV1().Secrets(certConfig.TestNamespace).Get(ctx, certConfig.CertSecretName, metav1.GetOptions{})
				if err != nil {
					// If secret is not found, this is the expected behavior - test should pass
					if apierrors.IsNotFound(err) {
						ginkgo.GinkgoLogr.Info("✅ primary-cert-bundle-secret not found as expected")
						return true
					}
					// Other errors - we should retry
					ginkgo.GinkgoLogr.Info("Error checking for primary-cert-bundle-secret", "error", err.Error())
					return false
				}

				// If secret exists, this is unexpected behavior - test should fail
				ginkgo.GinkgoLogr.Info("❌ primary-cert-bundle-secret found but was expected NOT to exist")
				return false
			}, pollingDuration, 30*time.Second).Should(gomega.BeTrue(), "primary-cert-bundle-secret should NOT be created by operator")

			ginkgo.GinkgoLogr.Info("✅ Primary cert bundle secret correctly not created by operator")
		})

		ginkgo.It("should verify certificate operation metrics", func(ctx context.Context) {
			ginkgo.GinkgoLogr.Info("=== Test: Verifying Certificate Operation Metrics ===")

			// Verify metrics using the utils function
			ginkgo.GinkgoLogr.Info("Verifying certificate operation metrics...")
			var validCertCount int
			gomega.Eventually(func() bool {
				count, success := utils.VerifyMetrics(ctx, dynamicClient, certificateRequestGVR, certConfig.TestNamespace)
				validCertCount = count
				return success
			}, testTimeout, 15*time.Second).Should(gomega.BeTrue(), "Metrics should reflect certificate operations")

			ginkgo.GinkgoLogr.Info("✅ Metrics verification successful",
				"validCertificateRequests", validCertCount,
				"clusterName", certConfig.ClusterName,
				"ocmClusterID", ocmClusterID,
				"namespace", certConfig.TestNamespace,
				"baseDomain", certConfig.BaseDomain)
		})
	})
})
