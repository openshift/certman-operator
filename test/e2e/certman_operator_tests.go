// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS. //go:build osde2e
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	certmanv1alpha1 "github.com/openshift/certman-operator/api/v1alpha1"
	configv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var scheme = runtime.NewScheme()

var awsSecretBackup *corev1.Secret

var _ = Describe("Certman Operator", Ordered, func() {
	var (
		k8s        *openshift.Client
		clientset  *kubernetes.Clientset
		secretName string
	)

	const (
		pollingDuration = 15 * time.Minute
		namespace       = "openshift-config"
		operatorNS      = "certman-operator"
		awsSecretName   = "aws"
	)

	BeforeAll(func(ctx context.Context) {
		log.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

		Expect(certmanv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		var err error
		k8s, err = openshift.New(GinkgoLogr)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to setup k8s client")

		clientset, err = kubernetes.NewForConfig(k8s.GetConfig())
		Expect(err).ShouldNot(HaveOccurred(), "Unable to setup clientset")
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

	Describe("AWS Secret Deletion Scenario", func() {

		It("Performs AWS secret deletion scenario end-to-end", func(ctx context.Context) {

			By("ensuring ClusterDeployment exists")
			gvr := hivev1.SchemeGroupVersion.WithResource("clusterdeployments")

			dynamicClient, err := dynamic.NewForConfig(k8s.GetConfig())
			Expect(err).ToNot(HaveOccurred(), "Failed to create dynamic client")

			Eventually(func() bool {
				list, err := dynamicClient.Resource(gvr).Namespace(operatorNS).List(ctx, metav1.ListOptions{})
				if err != nil {
					return false
				}
				return len(list.Items) > 0
			}, pollingDuration, 30*time.Second).Should(BeTrue(), "No ClusterDeployment found in certman-operator namespace")

			By("ensuring AWS secret exists")
			Eventually(func() bool {
				secret, err := clientset.CoreV1().Secrets(operatorNS).Get(ctx, awsSecretName, metav1.GetOptions{})
				if err != nil {
					return false
				}
				awsSecretBackup = secret.DeepCopy()
				return true
			}, pollingDuration, 30*time.Second).Should(BeTrue(), "AWS secret does not exist")

			By("deleting AWS secret")
			err = clientset.CoreV1().Secrets(operatorNS).Delete(ctx, awsSecretName, metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred(), "Failed to delete AWS secret")

			By("verifying operator pod is running and has not restarted after secret deletion")
			pods, err := clientset.CoreV1().Pods(operatorNS).List(ctx, metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred(), "Failed to list certman-operator pods")
			Expect(pods.Items).ToNot(BeEmpty(), "No pods found in certman-operator namespace")

			found := false
			for _, pod := range pods.Items {
				if strings.Contains(pod.Name, "certman-operator") {
					found = true

					fmt.Printf("Found pod %s, status: %s\n", pod.Name, pod.Status.Phase)
					Expect(pod.Status.Phase).To(Equal(corev1.PodRunning), "Pod should be in Running state")

					Expect(pod.Status.ContainerStatuses).ToNot(BeEmpty(), "Expected container statuses to be present")
					fmt.Printf("RestartCount: %d\n", pod.Status.ContainerStatuses[0].RestartCount)
					Expect(pod.Status.ContainerStatuses[0].RestartCount).To(BeZero(), "Pod should not restart after secret deletion")

					logs, err := clientset.CoreV1().Pods(operatorNS).GetLogs(pod.Name, &corev1.PodLogOptions{}).Do(ctx).Raw()
					Expect(err).ToNot(HaveOccurred(), "Failed to get pod logs")
					Expect(string(logs)).ToNot(ContainSubstring("panic"), "Operator logs should not contain panic")
					fmt.Println("Pod logs checked no panic found")
				}
			}
			Expect(found).To(BeTrue(), "No certman-operator pod matched by name")

			By("recreating AWS secret after testing")
			awsSecretBackup.ObjectMeta.ResourceVersion = ""
			awsSecretBackup.ObjectMeta.UID = ""
			awsSecretBackup.ObjectMeta.CreationTimestamp = metav1.Time{}
			awsSecretBackup.ObjectMeta.ManagedFields = nil

			_, err = clientset.CoreV1().Secrets(operatorNS).Create(ctx, awsSecretBackup, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred(), "Failed to recreate AWS secret")

		})

	})

})
