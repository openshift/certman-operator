// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS. //go:build osde2e
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Certman Operator", Ordered, func() {
	var (
		k8s        *openshift.Client
		clientset  *kubernetes.Clientset
		secretName string
	)
	const (
		pollingDuration            = 15 * time.Minute
		namespace                  = "openshift-config"
		namespace_certman_operator = "certman-operator"
	)

	BeforeAll(func(ctx context.Context) {
		log.SetLogger(GinkgoLogr)
		var err error
		k8s, err = openshift.New(GinkgoLogr)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to setup k8s client")
		clientset, err = kubernetes.NewForConfig(k8s.GetConfig())
		Expect(err).ShouldNot(HaveOccurred(), "Unable to setup Config client")
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

	It("delete secret, primary-cert-bundle-secret, if exists", func(ctx context.Context) {
		secretNameToDelete := "primary-cert-bundle-secret"
		pollingDuration := 2 * time.Minute
		pollInterval := 30 * time.Second

		originalSecret, err := clientset.CoreV1().Secrets(namespace_certman_operator).Get(ctx, secretNameToDelete, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			log.Log.Info("Secret does not exist, skipping deletion test.")
			return
		}
		Expect(err).ShouldNot(HaveOccurred(), "Error retrieving the original secret")

		originalTimestamp := originalSecret.CreationTimestamp.Time
		log.Log.Info(fmt.Sprintf("Original secret creation timestamp: %v", originalTimestamp))

		err = clientset.CoreV1().Secrets(namespace_certman_operator).Delete(ctx, secretNameToDelete, metav1.DeleteOptions{})
		Expect(err).ShouldNot(HaveOccurred(), "Failed to delete the secret")

		Eventually(func() bool {
			newSecret, err := clientset.CoreV1().Secrets(namespace_certman_operator).Get(ctx, secretNameToDelete, metav1.GetOptions{})
			if err != nil {
				return false
			}
			return newSecret.CreationTimestamp.Time.After(originalTimestamp)
		}, pollingDuration, pollInterval).Should(BeTrue(),
			fmt.Sprintf("Secret %q was not re-created within %v or timestamp did not change", secretNameToDelete, pollingDuration))
	})

})
