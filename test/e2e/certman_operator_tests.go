// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS.
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"net/http"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	utils "github.com/openshift/certman-operator/test/e2e/utils"
	configv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var scheme = runtime.NewScheme()
var awsSecretBackup *corev1.Secret
var _ = ginkgo.Describe("Certman Operator", ginkgo.Ordered, ginkgo.ContinueOnFailure, func() {
	var (
		k8s        *openshift.Client
		clientset  *kubernetes.Clientset
		secretName string
		logger     = log.Log
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
		clientset, err = kubernetes.NewForConfig(k8s.GetConfig())
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Unable to setup Config client")

		apiExtClient, err := apiextensionsclient.NewForConfig(k8s.GetConfig())
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to create API Extensions client")

		kubeClient, err := kubernetes.NewForConfig(k8s.GetConfig())
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to create Kubernetes core client")

		gomega.Expect(utils.SetupHiveCRDs(ctx, apiExtClient)).To(gomega.Succeed(), "Failed to setup Hive CRDs")

		gomega.Expect(utils.SetupCertman(ctx, kubeClient, apiExtClient)).To(gomega.Succeed(), "Failed to setup Certman")

		gomega.Expect(utils.SetupAWSCreds(ctx, kubeClient)).To(gomega.Succeed(), "Failed to setup AWS Secret")
		fmt.Println("Setup Done Successfully")
	})

	ginkgo.It("should install the certman operator successfully", func() {
		ctx := context.TODO()
		manifestURLs := []string{
			"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/service_account.yaml",
			"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/role.yaml",
			"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/role_binding.yaml",
			"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/operator.yaml",
		}

		cfg := k8s.GetConfig()
		scheme := runtime.NewScheme()
		utilruntime.Must(corev1.AddToScheme(scheme))
		utilruntime.Must(appsv1.AddToScheme(scheme))
		utilruntime.Must(rbacv1.AddToScheme(scheme))

		_, err := client.New(cfg, client.Options{Scheme: scheme})
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to create controller-runtime client")

		for _, url := range manifestURLs {
			fmt.Printf("Downloading manifest from: %s\n", url)

			resp, err := http.Get(url)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to download manifest")
			defer resp.Body.Close()

			data, err := io.ReadAll(resp.Body)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to read manifest")

			decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)

			for {
				var rawObj map[string]interface{}
				if err := decoder.Decode(&rawObj); err != nil {
					if err == io.EOF {
						break
					}
					gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to decode YAML")
				}
				if len(rawObj) == 0 {
					continue
				}

				obj := &unstructured.Unstructured{Object: rawObj}

				dc, err := discovery.NewDiscoveryClientForConfig(cfg)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

				gr, err := restmapper.GetAPIGroupResources(dc)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

				mapper := restmapper.NewDiscoveryRESTMapper(gr)

				mapping, err := mapper.RESTMapping(obj.GroupVersionKind().GroupKind(), obj.GroupVersionKind().Version)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

				dynamicClient, err := dynamic.NewForConfig(cfg)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

				var dri dynamic.ResourceInterface
				if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
					if obj.GetNamespace() == "" {
						obj.SetNamespace("certman-operator")
					}
					dri = dynamicClient.Resource(mapping.Resource).Namespace(obj.GetNamespace())
				} else {
					dri = dynamicClient.Resource(mapping.Resource)
				}

				_, err = dri.Create(ctx, obj, metav1.CreateOptions{})
				if apierrors.IsAlreadyExists(err) {
					fmt.Printf("Resource %s/%s already exists, skipping.\n", obj.GetNamespace(), obj.GetName())
					continue
				}
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to create resource")

				fmt.Printf("Successfully applied resource: %s/%s\n", obj.GetNamespace(), obj.GetName())

				time.Sleep(10 * time.Second)

			}

		}
		fmt.Println("Installation is Done for certman-operator")
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

		time.Sleep(30 * time.Second)

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

	ginkgo.AfterAll(func(ctx context.Context) {
		logger.Info("Cleanup: Running AfterAll cleanup")

		cfg := k8s.GetConfig()

		apiExtClient, err := apiextensionsclient.NewForConfig(cfg)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to create API Extensions client")

		kubeClient, err := kubernetes.NewForConfig(cfg)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to create Kubernetes client")

		dynamicClient, err := dynamic.NewForConfig(cfg)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to create dynamic client")

		dc, err := discovery.NewDiscoveryClientForConfig(cfg)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to create discovery client")

		gr, err := restmapper.GetAPIGroupResources(dc)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to get API group resources")

		mapper := restmapper.NewDiscoveryRESTMapper(gr)

		// --- Delete Hive CRD ---
		const hiveCRDName = "clusterdeployments.hive.openshift.io"
		err = apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Delete(ctx, hiveCRDName, metav1.DeleteOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				logger.Info("Hive CRD not found; nothing to delete", "crd", hiveCRDName)
			} else {
				logger.Info("Error deleting Hive CRD", "crd", hiveCRDName, "error", err)
			}
		} else {
			logger.Info("Hive CRD deleted successfully", "crd", hiveCRDName)
		}

		const certmanCRDName = "certificaterequests.certman.managed.openshift.io"
		err = apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Delete(ctx, certmanCRDName, metav1.DeleteOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				logger.Info("Certman CRD not found; nothing to delete", "crd", certmanCRDName)
			} else {
				logger.Info("Error deleting Certman CRD", "crd", certmanCRDName, "error", err)
			}
		} else {
			logger.Info("Certman CRD deleted successfully", "crd", certmanCRDName)
		}

		const operatorNS = "certman-operator"
		err = kubeClient.CoreV1().Namespaces().Delete(ctx, operatorNS, metav1.DeleteOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				logger.Info("Namespace not found; nothing to delete", "namespace", operatorNS)
			} else {
				logger.Info("Error deleting namespace", "namespace", operatorNS, "error", err)
			}
		} else {
			logger.Info("Namespace deleted successfully", "namespace", operatorNS)
		}

		manifestURLs := []string{
			"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/service_account.yaml",
			"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/role.yaml",
			"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/role_binding.yaml",
			"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/operator.yaml",
		}

		for _, url := range manifestURLs {
			logger.Info("Downloading manifest", "url", url)

			resp, err := http.Get(url)
			if err != nil {
				logger.Info("Failed to download manifest", "url", url, "error", err)
				continue
			}
			defer resp.Body.Close()

			data, err := io.ReadAll(resp.Body)
			if err != nil {
				logger.Info("Failed to read manifest body", "url", url, "error", err)
				continue
			}

			decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)

			for {
				var rawObj map[string]interface{}
				if err := decoder.Decode(&rawObj); err != nil {
					if err == io.EOF {
						break
					}
					logger.Info("Failed to decode YAML", "url", url, "error", err)
					break
				}
				if len(rawObj) == 0 {
					continue
				}

				obj := &unstructured.Unstructured{Object: rawObj}
				mapping, err := mapper.RESTMapping(obj.GroupVersionKind().GroupKind(), obj.GroupVersionKind().Version)
				if err != nil {
					logger.Info("Failed to get RESTMapping for object", "gvk", obj.GroupVersionKind(), "error", err)
					continue
				}

				var dri dynamic.ResourceInterface
				if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
					if obj.GetNamespace() == "" {
						obj.SetNamespace(operatorNS)
					}
					dri = dynamicClient.Resource(mapping.Resource).Namespace(obj.GetNamespace())
				} else {
					dri = dynamicClient.Resource(mapping.Resource)
				}

				err = dri.Delete(ctx, obj.GetName(), metav1.DeleteOptions{})
				if err != nil {
					if apierrors.IsNotFound(err) {
						logger.Info("Resource not found; skipping delete", "name", obj.GetName())
					} else {
						logger.Info("Failed to delete resource", "name", obj.GetName(), "error", err)
					}
				} else {
					logger.Info("Deleted resource", "name", obj.GetName())
				}
			}
		}

		logger.Info("Cleanup: AfterAll cleanup completed")
	})

})
