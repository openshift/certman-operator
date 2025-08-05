// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS. //go:build osde2e
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Certman Operator", Ordered, func() {
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
	)

	BeforeAll(func(ctx context.Context) {
		log.SetLogger(GinkgoLogr)
		var err error
		k8s, err = openshift.New(GinkgoLogr)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to setup k8s client")
		clientset, err = kubernetes.NewForConfig(k8s.GetConfig())
		Expect(err).ShouldNot(HaveOccurred(), "Unable to setup Config client")

		dynamicClient, err = dynamic.NewForConfig(k8s.GetConfig())
		Expect(err).ShouldNot(HaveOccurred(), "Unable to create dynamic client")
		Expect(dynamicClient).ShouldNot(BeNil(), "dynamic client is nil")
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

	It("Delete the Cluster Deployment", func(ctx context.Context) {
		logger.Info("Test - Delete Cluster Deployment")
		clusterDeploymentGVR := schema.GroupVersionResource{
			Group:    "hive.openshift.io",
			Version:  "v1",
			Resource: "clusterdeployments",
		}
		certRequestGVR := schema.GroupVersionResource{
			Group:    "certman.managed.openshift.io",
			Version:  "v1alpha1",
			Resource: "certificaterequests",
		}

		Eventually(func() bool {
			logger.Info("Checking if ClusterDeployment exist or not")
			cdList, err := dynamicClient.Resource(clusterDeploymentGVR).Namespace("certman-operator").List(ctx, metav1.ListOptions{})
			if err != nil {
				logger.Error(err, "Failed to list ClusterDeployments")
				return false
			}
			if len(cdList.Items) == 0 {
				logger.Info("No ClusterDeployment found in certman-operator namespace.")
				return false
			}

			cd := cdList.Items[0]
			cdName := cd.GetName()
			finalizers := cd.GetFinalizers()
			logger.Info("Found ClusterDeployment", "name", cdName, "finalizers", finalizers)

			hasCertFinalizer := false
			for _, f := range finalizers {
				if f == "certificaterequests.certman.managed.openshift.io" {
					hasCertFinalizer = true
					break
				}
			}

			if !hasCertFinalizer {
				logger.Info("ClusterDeployment does not have the certman finalizer", "name", cdName)
				return false
			}

			logger.Info("Found the specified finalizer. Deleting ClusterDeployment", "name", cdName)
			err = dynamicClient.Resource(clusterDeploymentGVR).Namespace("certman-operator").Delete(ctx, cdName, metav1.DeleteOptions{})
			if err != nil {
				logger.Error(err, "Failed to delete ClusterDeployment", "name", cdName)
				return false
			}

			time.Sleep(2 * time.Second)

			logger.Info("Checking if CertificateRequests are deleted")

			crList, err := dynamicClient.Resource(certRequestGVR).Namespace("certman-operator").List(ctx, metav1.ListOptions{})
			if err != nil {
				logger.Error(err, "Failed to list CertificateRequests")
				return false
			}

			if len(crList.Items) > 0 {
				for _, cr := range crList.Items {
					crName := cr.GetName()
					finalizers := cr.GetFinalizers()

					if len(finalizers) > 0 {
						logger.Info("CertificateRequest not deleted due to finalizers. Removing finalizers", "name", crName)
						cr.SetFinalizers([]string{})
						_, err := dynamicClient.Resource(certRequestGVR).Namespace("certman-operator").Update(ctx, &cr, metav1.UpdateOptions{})
						if err != nil {
							logger.Error(err, "Failed to remove finalizers from CertificateRequest", "name", crName)
							return false
						}

					}

					logger.Info("Rechecking CertificateRequest deletion ", "name", crName)
					crList, err = dynamicClient.Resource(certRequestGVR).Namespace("certman-operator").List(ctx, metav1.ListOptions{})
					if err != nil {
						logger.Error(err, "Failed to re-list CertificateRequests")
						return false
					}
					if len(crList.Items) > 0 {
						logger.Info("CertificateRequests still present")
						return false
					}
				}
			}

			logger.Info("All CertificateRequests successfully deleted")

			logger.Info("Checking if primary-cert-bundle-secret is deleted or not")

			secretList, err := clientset.CoreV1().Secrets("certman-operator").List(ctx, metav1.ListOptions{})
			if err != nil {
				logger.Error(err, "Failed to list Secrets in certman-operator")
				return false
			}
			for _, s := range secretList.Items {
				if s.Name == "primary-cert-bundle-secret" {
					Fail("primary-cert-bundle-secret still exists.")
				}
			}
			logger.Info("primary-cert-bundle-secret successfully deleted")

			return true
		}, pollingDuration, 15*time.Second).Should(BeTrue(), "Delete the Cluster Deployment")
	})
})
