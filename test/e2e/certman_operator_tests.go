// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS.
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	utils "github.com/openshift/certman-operator/test/e2e/utils"
	configv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"
	corev1 "k8s.io/api/core/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var scheme = runtime.NewScheme()
var awsSecretBackup *corev1.Secret

var _ = ginkgo.Describe("Certman Operator", ginkgo.Ordered, ginkgo.ContinueOnFailure, func() {
	var (
		k8s                       *openshift.Client
		clientset                 *kubernetes.Clientset
		dynamicClient             dynamic.Interface
		secretName                string
		certConfig                *utils.CertConfig
		clusterDeploymentName     string
		ocmClusterID              string
		adminKubeconfigSecretName string
		clusterName               string
		certificateRequestGVR     schema.GroupVersionResource
		clusterDeploymentGVR      schema.GroupVersionResource
		logger                    = log.Log
	)

	const (
		pollingDuration = 15 * time.Minute
		shortTimeout    = 5 * time.Minute
		testTimeout     = 10 * time.Minute
		namespace       = "openshift-config"
		operatorNS      = "certman-operator"
		awsSecretName   = "aws"
	)

	ginkgo.BeforeAll(func(ctx context.Context) {
		log.SetLogger(ginkgo.GinkgoLogr)

		var err error

		certConfig = utils.LoadTestConfig()
		clusterName = certConfig.ClusterName
		ocmClusterID = certConfig.OCMClusterID
		clusterDeploymentName = fmt.Sprintf("%s-deployment", clusterName)
		adminKubeconfigSecretName = fmt.Sprintf("%s-admin-kubeconfig", clusterName)

		// Initialize primary k8s client
		k8s, err = openshift.New(ginkgo.GinkgoLogr)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Unable to setup k8s client")

		cfg := k8s.GetConfig()

		// Initialize clientset from k8s config
		clientset, err = kubernetes.NewForConfig(cfg)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Unable to setup clientset")

		// Initialize API Extensions client
		apiExtClient, err := apiextensionsclient.NewForConfig(cfg)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to create API Extensions client")

		// Initialize dynamic client
		dynamicClient, err = dynamic.NewForConfig(cfg)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to create dynamic client")

		// Initialize GVRs
		certificateRequestGVR = schema.GroupVersionResource{
			Group: "certman.managed.openshift.io", Version: "v1alpha1", Resource: "certificaterequests",
		}
		clusterDeploymentGVR = schema.GroupVersionResource{
			Group: "hive.openshift.io", Version: "v1", Resource: "clusterdeployments",
		}

		// Setup Hive CRDs
		gomega.Expect(utils.SetupHiveCRDs(ctx, apiExtClient)).To(gomega.Succeed(), "Failed to setup Hive CRDs")

		// Setup Certman using clientset
		gomega.Expect(utils.SetupCertman(ctx, clientset, apiExtClient, cfg)).To(gomega.Succeed(), "Failed to setup Certman")

		// Setup AWS credentials using clientset
		gomega.Expect(utils.SetupAWSCreds(ctx, clientset)).To(gomega.Succeed(), "Failed to setup AWS Secret")

		// Ensure test namespace exists
		err = utils.EnsureTestNamespace(ctx, clientset, certConfig.TestNamespace)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to ensure test namespace exists")

		// Create admin kubeconfig secret
		err = utils.CreateAdminKubeconfigSecret(ctx, clientset, certConfig, adminKubeconfigSecretName)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to create admin kubeconfig secret")

		ginkgo.GinkgoLogr.Info("Test setup completed",
			"namespace", certConfig.TestNamespace,
			"adminSecret", adminKubeconfigSecretName,
			"clusterDeployment", clusterDeploymentName)

		fmt.Println("Setup Done Successfully")
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

	ginkgo.It("Performs AWS secret deletion scenario end-to-end", func(ctx context.Context) {
		ginkgo.By("ensuring AWS secret exists")
		gomega.Eventually(func() bool {
			secret, err := clientset.CoreV1().Secrets(operatorNS).Get(ctx, awsSecretName, metav1.GetOptions{})
			if err != nil {
				return false
			}
			awsSecretBackup = secret.DeepCopy()
			return true
		}, pollingDuration, 30*time.Second).Should(gomega.BeTrue(), "AWS secret does not exist")

		ginkgo.By("deleting AWS secret")
		err := clientset.CoreV1().Secrets(operatorNS).Delete(ctx, awsSecretName, metav1.DeleteOptions{})
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to delete AWS secret")

		ginkgo.By("waiting for operator pod to be stable after AWS secret deletion")
		gomega.Eventually(func() bool {
			pods, err := clientset.CoreV1().Pods(operatorNS).List(ctx, metav1.ListOptions{})
			if err != nil || len(pods.Items) == 0 {
				return false
			}
			for _, pod := range pods.Items {
				if strings.Contains(pod.Name, "certman-operator") {
					if pod.Status.Phase != corev1.PodRunning {
						return false
					}
					if len(pod.Status.ContainerStatuses) == 0 || pod.Status.ContainerStatuses[0].RestartCount != 0 {
						return false
					}
					return true
				}
			}
			return false
		}, 10*time.Second, 1*time.Second).Should(gomega.BeTrue(), "Operator pod did not stabilize after AWS secret deletion")

		ginkgo.By("verifying operator pod is running and has not restarted after secret deletion")
		pods, err := clientset.CoreV1().Pods(operatorNS).List(ctx, metav1.ListOptions{})
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to list certman-operator pods")
		gomega.Expect(pods.Items).ToNot(gomega.BeEmpty(), "No pods found in certman-operator namespace")

		found := false
		for _, pod := range pods.Items {
			if strings.Contains(pod.Name, "certman-operator") {
				found = true

				fmt.Printf("Found pod %s, status: %s\n", pod.Name, pod.Status.Phase)
				gomega.Expect(pod.Status.Phase).To(gomega.Equal(corev1.PodRunning), "Pod should be in Running state")

				gomega.Expect(pod.Status.ContainerStatuses).ToNot(gomega.BeEmpty(), "Expected container statuses to be present")
				fmt.Printf("RestartCount: %d\n", pod.Status.ContainerStatuses[0].RestartCount)
				gomega.Expect(pod.Status.ContainerStatuses[0].RestartCount).To(gomega.BeZero(), "Pod should not restart after secret deletion")

				logs, err := clientset.CoreV1().Pods(operatorNS).GetLogs(pod.Name, &corev1.PodLogOptions{}).Do(ctx).Raw()
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to get pod logs")
				gomega.Expect(string(logs)).ToNot(gomega.ContainSubstring("panic"), "Operator logs should not contain panic")
				fmt.Println("Pod logs checked no panic found")
			}
		}
		gomega.Expect(found).To(gomega.BeTrue(), "No certman-operator pod matched by name")

		ginkgo.By("recreating AWS secret after testing")
		awsSecretBackup.ObjectMeta.ResourceVersion = ""
		awsSecretBackup.ObjectMeta.UID = ""
		awsSecretBackup.ObjectMeta.CreationTimestamp = metav1.Time{}
		awsSecretBackup.ObjectMeta.ManagedFields = nil

		_, err = clientset.CoreV1().Secrets(operatorNS).Create(ctx, awsSecretBackup, metav1.CreateOptions{})
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to recreate AWS secret")
	})

	ginkgo.It("should create CertificateRequest via operator reconciliation", func(ctx context.Context) {
		ginkgo.GinkgoLogr.Info("=== Test: Creating ClusterDeployment and CertificateRequest ===")

		// Step 1: Create ClusterDeployment
		ginkgo.GinkgoLogr.Info("Step 1: Creating complete ClusterDeployment resource...")
		clusterDeployment := utils.BuildCompleteClusterDeployment(certConfig, clusterDeploymentName, adminKubeconfigSecretName, ocmClusterID)

		// Clean and create ClusterDeployment using dynamic client
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

		// Step 2: Verify ClusterDeployment meets reconciliation criteria
		ginkgo.GinkgoLogr.Info("Step 2: Verifying ClusterDeployment reconciliation criteria...")
		gomega.Eventually(func() bool {
			return utils.VerifyClusterDeploymentCriteria(ctx, dynamicClient, clusterDeploymentGVR,
				certConfig.TestNamespace, clusterDeploymentName, ocmClusterID)
		}, shortTimeout, 10*time.Second).Should(gomega.BeTrue(), "ClusterDeployment should meet all reconciliation criteria")

		ginkgo.GinkgoLogr.Info("✅ ClusterDeployment meets all reconciliation criteria")

		// Step 3: Verify CertificateRequest is created by operator
		ginkgo.GinkgoLogr.Info("Step 3: Verifying CertificateRequest creation by operator...")
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

	ginkgo.AfterAll(func(ctx context.Context) {
		logger.Info("Cleanup: Running AfterAll cleanup")

		cfg := k8s.GetConfig()

		// Create fresh clients for cleanup
		apiExtClient, err := apiextensionsclient.NewForConfig(cfg)
		if err != nil {
			logger.Info("Failed to create API Extensions client, skipping cleanup", "error", err)
			return
		}

		kubeClient, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			logger.Info("Failed to create Kubernetes client, skipping cleanup", "error", err)
			return
		}

		cleanupDynamicClient, err := dynamic.NewForConfig(cfg)
		if err != nil {
			logger.Info("Failed to create dynamic client, skipping cleanup", "error", err)
			return
		}

		// Create RESTMapper for resource cleanup
		dc, err := discovery.NewDiscoveryClientForConfig(cfg)
		if err != nil {
			logger.Info("Failed to create discovery client, skipping Certman cleanup", "error", err)
		} else {
			gr, err := restmapper.GetAPIGroupResources(dc)
			if err != nil {
				logger.Info("Failed to get API group resources, skipping Certman cleanup", "error", err)
			} else {
				mapper := restmapper.NewDiscoveryRESTMapper(gr)

				// Cleanup Certman with RESTMapper
				if err := utils.CleanupCertman(ctx, kubeClient, apiExtClient, cleanupDynamicClient, mapper); err != nil {
					logger.Info("Error during Certman cleanup", "error", err)
				}
			}
		}

		if err := utils.CleanupHive(ctx, apiExtClient); err != nil {
			logger.Info("Error during Hive cleanup", "error", err)
		}

		if err := utils.CleanupAWSCreds(ctx, kubeClient); err != nil {
			logger.Info("Error during AWS secret cleanup", "error", err)
		} else {
			logger.Info("AWS secret cleanup succeeded")
		}

		logger.Info("Cleanup AfterAll cleanup completed")
	})
})
