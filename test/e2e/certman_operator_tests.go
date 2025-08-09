// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS. //go:build osde2e
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Certman Operator", Ordered, func() {
	var (
		k8s           *openshift.Client
		clientset     *kubernetes.Clientset
		secretName    string
		dynamicClient dynamic.Interface
	)

	const (
		pollingDuration = 15 * time.Minute
		namespace       = "openshift-config"
	)

	BeforeAll(func(ctx context.Context) {
		log.SetLogger(GinkgoLogr)
		var err error
		k8s, err = openshift.New(GinkgoLogr)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to setup k8s client")
		clientset, err = kubernetes.NewForConfig(k8s.GetConfig())
		Expect(err).ShouldNot(HaveOccurred(), "Unable to setup Config client")
		dynamicClient, err = dynamic.NewForConfig(k8s.GetConfig())
		Expect(err).ShouldNot(HaveOccurred(), "Unable to setup dynamic client")
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

	It("removes OwnerReference by simulating oc apply -f with modified file using server-side apply", func(ctx context.Context) {
		logger := log.FromContext(ctx)
		certRequestGVR := schema.GroupVersionResource{
			Group:    "certman.managed.openshift.io",
			Version:  "v1alpha1",
			Resource: "certificaterequests",
		}
		crNamespace := "certman-operator"

		Eventually(func() bool {
			crList, err := dynamicClient.Resource(certRequestGVR).Namespace(crNamespace).List(ctx, metav1.ListOptions{})
			if err != nil || len(crList.Items) == 0 {
				logger.Error(err, "Failed to list CertificateRequests")
				return false
			}

			originalCR := crList.Items[0]
			crName := originalCR.GetName()
			logger.Info("Fetched CertificateRequest", "name", crName, "resourceVersion", originalCR.GetResourceVersion())

			unstructured.RemoveNestedField(originalCR.Object, "metadata", "ownerReferences")
			unstructured.RemoveNestedField(originalCR.Object, "metadata", "managedFields")

			if _, found, _ := unstructured.NestedFieldNoCopy(originalCR.Object, "metadata", "ownerReferences"); found {
				logger.Info("ownerReferences still present after removal attempt", "name", crName)
			} else {
				logger.Info("ownerReferences removed before patch", "name", crName)
			}

			patchBytes, err := json.Marshal(originalCR.Object)
			if err != nil {
				logger.Error(err, "Failed to marshal modified CertificateRequest", "name", crName)
				return false
			}

			patchedCR, err := dynamicClient.Resource(certRequestGVR).Namespace(crNamespace).Patch(ctx, crName, types.ApplyPatchType, patchBytes, metav1.PatchOptions{
				FieldManager: "certman-e2e-test",
			})
			if err != nil {
				logger.Error(err, "Failed to apply patch", "name", crName)
				return false
			}
			logger.Info("Patch applied", "name", crName, "newResourceVersion", patchedCR.GetResourceVersion())

			time.Sleep(10 * time.Second)

			finalCR, err := dynamicClient.Resource(certRequestGVR).Namespace(crNamespace).Get(ctx, crName, metav1.GetOptions{})
			if err != nil {
				logger.Error(err, "Failed to get CertificateRequest after patch", "name", crName)
				return false
			}
			logger.Info("Fetched final CertificateRequest", "name", crName, "resourceVersion", finalCR.GetResourceVersion())

			for _, ref := range finalCR.GetOwnerReferences() {
				if ref.Controller != nil && *ref.Controller {
					logger.Info("OwnerReference re-added", "name", crName)
					return true
				}
			}
			logger.Info("OwnerReference not re-added yet", "name", crName)
			return false
		}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "OwnerReference should be re-added after patch")
	})
})
