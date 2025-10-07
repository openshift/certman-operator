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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

var scheme = runtime.NewScheme()
var awsSecretBackup *corev1.Secret
var _ = ginkgo.Describe("Certman Operator", ginkgo.Ordered, ginkgo.ContinueOnFailure, func() {
	var (
		logger     = log.Log
		k8s        *openshift.Client
		clientset  *kubernetes.Clientset
		secretName string

		dynamicClient dynamic.Interface
	)

	const (
		pollingDuration = 15 * time.Minute
		namespace       = "openshift-config"
		operatorNS      = "certman-operator"
		awsSecretName   = "aws"
	)

	ginkgo.BeforeAll(func(ctx context.Context) {
		log.SetLogger(ginkgo.GinkgoLogr)

		var err error
		k8s, err = openshift.New(ginkgo.GinkgoLogr)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Unable to setup k8s client")

		cfg := k8s.GetConfig()

		clientset, err = kubernetes.NewForConfig(cfg)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Unable to setup Config client")

		apiExtClient, err := apiextensionsclient.NewForConfig(cfg)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to create API Extensions client")

		gomega.Expect(utils.SetupHiveCRDs(ctx, apiExtClient)).To(gomega.Succeed(), "Failed to setup Hive CRDs")

		gomega.Expect(utils.SetupCertman(ctx, clientset, apiExtClient, cfg)).To(gomega.Succeed(), "Failed to setup Certman")

		gomega.Expect(utils.SetupAWSCreds(ctx, clientset)).To(gomega.Succeed(), "Failed to setup AWS Secret")

		dynamicClient, err = dynamic.NewForConfig(k8s.GetConfig())
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Unable to create dynamic client")
		gomega.Expect(dynamicClient).ShouldNot(gomega.BeNil(), "dynamic client is nil")

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

	ginkgo.It("should install and upgrade certman-operator via catalogsource", func(ctx context.Context) {

		gomega.Eventually(func() bool {

			if !utils.CreateCertmanResources(ctx, dynamicClient, operatorNS) {
				logger.Info("Failed to create certman-operator resources")
				return false
			}

			logger.Info("Resources created successfully. Waiting for csv to get installed...")

			time.Sleep(30 * time.Second)

			if err := utils.LogCertmanCSVVersion(ctx, dynamicClient, operatorNS); err != nil {
				return false
			}

			logger.Info("Waiting for certman operator pod to be in running state")

			time.Sleep(30 * time.Second)

			if !utils.CheckPodStatus(ctx, clientset, operatorNS) {
				logger.Info("certman-operator pod is not in running state")
				return false
			}

			logger.Info("certman-operator pod is running successfully")
			return true

		}, pollingDuration, 30*time.Second).Should(gomega.BeTrue(), "certman-operator should be installed, upgraded, and running successfully")
	})

	ginkgo.AfterAll(func(ctx context.Context) {

		logger.Info("Cleanup: Running AfterAll cleanup")

		cfg := k8s.GetConfig()

		apiExtClient, err := apiextensionsclient.NewForConfig(cfg)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to create API Extensions client")

		dynamicClient, err := dynamic.NewForConfig(cfg)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to create dynamic client")

		dc, err := discovery.NewDiscoveryClientForConfig(cfg)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to create discovery client")

		gr, err := restmapper.GetAPIGroupResources(dc)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to get API group resources")

		mapper := restmapper.NewDiscoveryRESTMapper(gr)

		if err := utils.CleanupHive(ctx, apiExtClient); err != nil {
			logger.Info("Error during Hive cleanup", "error", err)
		}

		if err := utils.CleanupCertman(ctx, clientset, apiExtClient, dynamicClient, mapper); err != nil {
			logger.Info("Error during Certman cleanup", "error", err)
		}

		if err := utils.CleanupAWSCreds(ctx, clientset); err != nil {
			logger.Info("Error during AWS secret cleanup", "error", err)
		} else {
			logger.Info("AWS secret cleanup succeeded")
		}

		logger.Info("Cleaning up certman-operator resources")

		if err := utils.CleanupCertmanResources(ctx, dynamicClient, operatorNS); err != nil {
			logger.Error(err, "Error during certman-operator resources cleanup")
		}

		logger.Info("Cleanup: AfterAll cleanup completed")
	})

})
