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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
		k8s           *openshift.Client
		clientset     *kubernetes.Clientset
		secretName    string
		dynamicClient dynamic.Interface
		logger        = log.Log
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

	It("Delete a labeled CertificateRequest and ensures it is recreated", func(ctx context.Context) {
		crGVR := schema.GroupVersionResource{
			Group:    "certman.managed.openshift.io",
			Version:  "v1alpha1",
			Resource: "certificaterequests",
		}

		log.Log.Info("STEP 1: Fetching existing CertificateRequest with owned=true label")
		crList, err := dynamicClient.Resource(crGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "certificaterequests.certman.managed.openshift.io",
		})

		if len(crList.Items) == 0 {
			log.Log.Info("No labeled CertificateRequest found, skipping test")
			Skip("SKIPPED: No labeled CertificateRequest found. This test only runs if a CR with 'owned=true' label is present.")
		}

		originalCR := crList.Items[0]
		originalCRName := originalCR.GetName()
		originalCRUID := originalCR.GetUID()
		initialIssuedCertCount := len(crList.Items)

		// Step 2: Delete the CertificateRequest
		log.Log.Info("STEP 2: Deleting the original CertificateRequest")
		err = dynamicClient.Resource(crGVR).Namespace(namespace).Delete(ctx, originalCRName, metav1.DeleteOptions{})
		Expect(err).ToNot(HaveOccurred(), "Failed to delete CertificateRequest")

		// Step 3: Handle deletion blocked by finalizer
		Eventually(func(g Gomega) bool {
			cr, err := dynamicClient.Resource(crGVR).Namespace(namespace).Get(ctx, originalCRName, metav1.GetOptions{})
			if err != nil {
				log.Log.Info("CR appears to be deleted already", "name", originalCRName)
				return true
			}
			if cr.GetDeletionTimestamp() == nil {
				log.Log.Info("CR not marked for deletion yet", "name", cr.GetName())
				return false
			}

			finalizers, found, err := unstructured.NestedStringSlice(cr.Object, "metadata", "finalizers")
			if err != nil {
				log.Log.Error(err, "Error retrieving finalizers")
				return false
			}
			if !found || len(finalizers) == 0 {
				log.Log.Info("No finalizers present", "name", cr.GetName())
				return false
			}

			crCopy := cr.DeepCopy()
			_ = unstructured.SetNestedStringSlice(crCopy.Object, []string{}, "metadata", "finalizers")

			_, err = dynamicClient.Resource(crGVR).Namespace(namespace).Update(ctx, crCopy, metav1.UpdateOptions{})
			if err != nil {
				log.Log.Error(err, "Failed to remove finalizer")
				return false
			}
			return true
		}, 1*time.Minute, 5*time.Second).Should(BeTrue(), "Finalizer should be removed")

		// Step 4: Wait for new CertificateRequest with new UID
		var newCRName string
		Eventually(func(g Gomega) bool {
			newList, err := dynamicClient.Resource(crGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				log.Log.Error(err, "Failed to list new CertificateRequests")
				return false
			}
			if len(newList.Items) == 0 {
				log.Log.Info("Still waiting for new CertificateRequest (none found)")
				return false
			}

			newCount := len(newList.Items)
			logger.Info("CertificateRequest count after reconciliation", "count", newCount)
			if newCount != initialIssuedCertCount {
				logger.Info("CertificateRequest count mismatch", "expected", initialIssuedCertCount, "got", newCount)
				return false
			}

			for _, cr := range newList.Items {
				log.Log.Info("Found CR candidate", "name", cr.GetName(), "uid", cr.GetUID())
				if cr.GetUID() != originalCRUID {
					newCRName = cr.GetName()
					log.Log.Info("New CertificateRequest detected", "name", newCRName, "uid", cr.GetUID())
					return true
				}
			}
			return false
		}, 4*time.Minute, 10*time.Second).Should(BeTrue(), "New CertificateRequest should appear")

		log.Log.Info("✅ Test completed: Secret successfully recreated with new CertificateRequest", "secret", secretName)
	})

})
