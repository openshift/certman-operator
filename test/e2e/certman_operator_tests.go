// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS.
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	ocmConfig "github.com/openshift-online/ocm-common/pkg/ocm/config"
	ocmConnBuilder "github.com/openshift-online/ocm-common/pkg/ocm/connection-builder"
	certmanv1alpha1 "github.com/openshift/certman-operator/api/v1alpha1"
	configv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"github.com/openshift/osde2e-common/pkg/clients/ocm"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"

	utils "github.com/openshift/certman-operator/test/e2e/utils"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var scheme = runtime.NewScheme()
var awsSecretBackup *corev1.Secret

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
		ocmConn                   *ocm.Client
	)

	const (
		pollingDuration = 15 * time.Minute
		namespace       = "openshift-config"
		operatorNS      = "certman-operator"
		awsSecretName   = "aws"
		testTimeout     = 5 * time.Minute
		shortTimeout    = 2 * time.Minute
	)

	ginkgo.BeforeAll(func(ctx context.Context) {
		kerberosID := os.Getenv("KERBEROS_ID")
		if kerberosID == "" {
			kerberosID = "default-kerberos-id"
		}

		gomega.Expect(utils.SetupHiveCRDs()).To(gomega.Succeed())
		gomega.Expect(utils.SetupCertman(kerberosID)).To(gomega.Succeed())
		gomega.Expect(utils.SetupAWSCreds()).To(gomega.Succeed())
		gomega.Expect(utils.InstallCertmanOperator()).To(gomega.Succeed())

		fmt.Println("Installation is Done for certman-operator")

		log.SetLogger(zap.New(zap.WriteTo(ginkgo.GinkgoWriter), zap.UseDevMode(true)))
		log.SetLogger(ginkgo.GinkgoLogr)

		gomega.Expect(certmanv1alpha1.AddToScheme(scheme)).To(gomega.Succeed())
		gomega.Expect(corev1.AddToScheme(scheme)).To(gomega.Succeed())

		var err error
		cfg, err := ocmConfig.Load()
		connection, err := ocmConnBuilder.NewConnection().Config(cfg).AsAgent("certman-local-ocm-client").Build()
		if err != nil {
			clientID := os.Getenv("OCM_CLIENT_ID")
			clientSecret := os.Getenv("OCM_CLIENT_SECRET")

			gomega.Expect(clientID).NotTo(gomega.BeEmpty(), "OCM_CLIENT_ID must be set")
			gomega.Expect(clientSecret).NotTo(gomega.BeEmpty(), "OCM_CLIENT_SECRET must be set")

			var ocmEnv ocm.Environment
			switch os.Getenv("OCM_ENV") {
			case "stage":
				ocmEnv = ocm.Stage
			case "int":
				ocmEnv = ocm.Integration
			default:
				ginkgo.Fail("Unexpected OCM_ENV - use 'stage' or 'int'")
			}

			ocmConn, err = ocm.New(ctx, "", clientID, clientSecret, ocmEnv)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Unable to setup OCM client")
			ginkgo.DeferCleanup(ocmConn.Connection.Close)
		} else {
			ocmConn = &ocm.Client{Connection: connection}
			ginkgo.DeferCleanup(connection.Close)
		}

		k8s, err = openshift.New(ginkgo.GinkgoLogr)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Unable to setup k8s client")

		clientset, err = kubernetes.NewForConfig(k8s.GetConfig())
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Unable to setup Config client")

		dynamicClient, err = dynamic.NewForConfig(k8s.GetConfig())
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Unable to setup dynamic client")

		ocmClusterID, clusterName, err = utils.GetClusterInfoFromOCM(ctx, ocmConn, dynamicClient)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to get cluster info from OCM")

		ginkgo.GinkgoLogr.Info("Retrieved cluster info from OCM", "clusterID", ocmClusterID, "clusterName", clusterName)

		gomega.Expect(ocmClusterID).ShouldNot(gomega.BeEmpty(), "OCM cluster ID must be available")
		gomega.Expect(clusterName).ShouldNot(gomega.BeEmpty(), "Cluster name must be available")

		baseDomain = utils.GetEnvOrDefault("BASE_DOMAIN", "u1xh.s1.devshift.org")

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
			certificateRequestGVR = schema.GroupVersionResource{
				Group: "certman.managed.openshift.io", Version: "v1alpha1", Resource: "certificaterequests",
			}

			clusterDeploymentGVR = schema.GroupVersionResource{
				Group: "hive.openshift.io", Version: "v1", Resource: "clusterdeployments",
			}

			err := utils.EnsureTestNamespace(ctx, clientset, certConfig.TestNamespace)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to ensure test namespace exists")

			err = utils.CreateAdminKubeconfigSecret(ctx, clientset, certConfig, adminKubeconfigSecretName)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to create admin kubeconfig secret")

			ginkgo.GinkgoLogr.Info("Test setup completed",
				"namespace", certConfig.TestNamespace,
				"adminSecret", adminKubeconfigSecretName,
				"clusterDeployment", clusterDeploymentName)
		})

		ginkgo.It("should create CertificateRequest via operator reconciliation", func(ctx context.Context) {
			ginkgo.GinkgoLogr.Info("Creating ClusterDeployment and CertificateRequest")

			clusterDeployment := utils.BuildCompleteClusterDeployment(certConfig, clusterDeploymentName, adminKubeconfigSecretName, ocmClusterID)

			utils.CleanupClusterDeployment(ctx, dynamicClient, clusterDeploymentGVR, certConfig.TestNamespace, clusterDeploymentName)

			_, err := dynamicClient.Resource(clusterDeploymentGVR).Namespace(certConfig.TestNamespace).
				Create(ctx, clusterDeployment, metav1.CreateOptions{})
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to create ClusterDeployment")

			gomega.Eventually(func() bool {
				_, err := dynamicClient.Resource(clusterDeploymentGVR).Namespace(certConfig.TestNamespace).
					Get(ctx, clusterDeploymentName, metav1.GetOptions{})
				return err == nil
			}, shortTimeout, 5*time.Second).Should(gomega.BeTrue(), "ClusterDeployment should be created successfully")

			gomega.Eventually(func() bool {
				return utils.VerifyClusterDeploymentCriteria(ctx, dynamicClient, clusterDeploymentGVR,
					certConfig.TestNamespace, clusterDeploymentName, ocmClusterID)
			}, shortTimeout, 10*time.Second).Should(gomega.BeTrue(), "ClusterDeployment should meet reconciliation criteria")

			gomega.Eventually(func() bool {
				crList, err := dynamicClient.Resource(certificateRequestGVR).
					Namespace(certConfig.TestNamespace).List(ctx, metav1.ListOptions{})
				if err != nil {
					ginkgo.GinkgoLogr.Error(err, "Failed to list CertificateRequests")
					return false
				}

				ginkgo.GinkgoLogr.Info("Found CertificateRequests", "totalCRs", len(crList.Items))
				for _, cr := range crList.Items {
					ginkgo.GinkgoLogr.Info("CertificateRequest found", "name", cr.GetName())
				}

				return len(crList.Items) > 0
			}, pollingDuration, 30*time.Second).Should(gomega.BeTrue(), "CertificateRequest should be created by operator")
		})

		ginkgo.It("should verify certificate operation metrics", func(ctx context.Context) {
			var validCertCount int

			gomega.Eventually(func() bool {
				count, success := utils.VerifyMetrics(ctx, dynamicClient, certificateRequestGVR, certConfig.TestNamespace)
				validCertCount = count
				return success
			}, testTimeout, 15*time.Second).Should(gomega.BeTrue(), "Metrics should reflect certificate operations")

			ginkgo.GinkgoLogr.Info("Metrics verification successful",
				"validCertificateRequests", validCertCount,
				"clusterName", certConfig.ClusterName,
				"ocmClusterID", ocmClusterID,
				"namespace", certConfig.TestNamespace,
				"baseDomain", certConfig.BaseDomain)
		})
	})

	ginkgo.Describe("AWS Secret Deletion Scenario", func() {

		ginkgo.It("Performs AWS secret deletion scenario end-to-end", func(ctx context.Context) {

			ginkgo.By("ensuring ClusterDeployment exists")
			gvr := hivev1.SchemeGroupVersion.WithResource("clusterdeployments")

			gomega.Eventually(func() bool {
				list, err := dynamicClient.Resource(gvr).Namespace(operatorNS).List(ctx, metav1.ListOptions{})
				if err != nil {
					return false
				}
				return len(list.Items) > 0
			}, pollingDuration, 30*time.Second).Should(gomega.BeTrue(), "No ClusterDeployment found in certman-operator namespace")

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

			time.Sleep(60 * time.Second)

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

	})

	ginkgo.AfterAll(func(ctx context.Context) {
		ginkgo.GinkgoLogr.Info("=== Cleanup: Running AfterAll cleanup ===")
		// Use the comprehensive cleanup function from utilss
		utils.CleanupAllTestResources(ctx, clientset, dynamicClient, certConfig,
			clusterDeploymentName, adminKubeconfigSecretName, ocmClusterID)
		ginkgo.GinkgoLogr.Info("AfterAll cleanup completed")
	})

})
