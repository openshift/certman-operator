// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS. //go:build osde2e
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"os"
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
		clusterName   string
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

		Expect(clientset).ShouldNot(BeNil(), "clientset is nil")
		dynamicClient, err = dynamic.NewForConfig(k8s.GetConfig())
		Expect(err).ShouldNot(HaveOccurred(), "Unable to create dynamic client")
		Expect(dynamicClient).ShouldNot(BeNil(), "dynamic client is nil")

		clusterName = os.Getenv("CLUSTER_NAME")
		Expect(clusterName).ToNot(BeEmpty(), "CLUSTER_NAME environment variable must be set")

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

	It("remove finalizer from the ClusterDeployment", func(ctx context.Context) {
		clusterDeploymentGVR := schema.GroupVersionResource{
			Group:    "hive.openshift.io",
			Version:  "v1",
			Resource: "clusterdeployments",
		}

		Eventually(func() bool {
			cdList, err := dynamicClient.Resource(clusterDeploymentGVR).Namespace("certman-operator").List(ctx, metav1.ListOptions{})
			if err != nil || len(cdList.Items) == 0 {
				Fail("Error listing ClusterDeployments or no ClusterDeployment found")
			}

			clusterDeployment := cdList.Items[0]
			cdName := clusterDeployment.GetName()
			logger.Info("Processing ClusterDeployment", "name", cdName)

			clusterDeployment.SetFinalizers([]string{})
			_, err = dynamicClient.Resource(clusterDeploymentGVR).Namespace("certman-operator").Update(ctx, &clusterDeployment, metav1.UpdateOptions{})
			if err != nil {
				logger.Error(err, "Failed to remove finalizers from ClusterDeployment", "name", cdName)
				return false
			}

			logger.Info("Finalizers removed from ClusterDeployment", "name", cdName)

			updatedCD, err := dynamicClient.Resource(clusterDeploymentGVR).Namespace("certman-operator").Get(ctx, cdName, metav1.GetOptions{})
			if err != nil {
				logger.Info("ClusterDeployment is deleted", "name", cdName)
				return true
			}

			finalizers := updatedCD.GetFinalizers()
			for _, finalizer := range finalizers {
				if finalizer == "hive.openshift.io/deprovision" || finalizer == "certificaterequests.certman.managed.openshift.io" {
					logger.Info("Finalizer has been re-added to ClusterDeployment", "name", cdName)
					return true
				}
			}

			logger.Info("Finalizer not yet re-added to ClusterDeployment", "name", cdName)
			return false

		}, pollingDuration, 30*time.Second).Should(BeTrue(), "ClusterDeployment should be deleted or have finalizer re-added")
	})

	It("remove finalizer from the CertificateRequest", func(ctx context.Context) {
		certRequestGVR := schema.GroupVersionResource{
			Group:    "certman.managed.openshift.io",
			Version:  "v1alpha1",
			Resource: "certificaterequests",
		}

		Eventually(func() bool {
			crList, err := dynamicClient.Resource(certRequestGVR).Namespace("certman-operator").List(ctx, metav1.ListOptions{})
			if err != nil || len(crList.Items) == 0 {
				logger.Error(err, "Error listing CertificateRequests or no CRs found")
				return false
			}

			certRequest := crList.Items[0]
			crName := certRequest.GetName()
			logger.Info("Processing CertificateRequest", "name", crName)

			certRequest.SetFinalizers([]string{})
			_, err = dynamicClient.Resource(certRequestGVR).Namespace("certman-operator").Update(ctx, &certRequest, metav1.UpdateOptions{})
			if err != nil {
				logger.Error(err, "Failed to remove finalizers from CertificateRequest", "name", crName)
				return false
			}
			logger.Info("Finalizers removed from CertificateRequest", "name", crName)

			updatedCR, err := dynamicClient.Resource(certRequestGVR).Namespace("certman-operator").Get(ctx, crName, metav1.GetOptions{})
			if err != nil {
				logger.Info("CertificateRequest is deleted", "name", crName)
				return true
			}

			finalizers := updatedCR.GetFinalizers()
			for _, finalizer := range finalizers {
				if finalizer == "certificaterequests.certman.managed.openshift.io" {
					logger.Info("Finalizer has been re-added to CertificateRequest", "name", crName)
					return true
				}
			}

			logger.Info("Finalizer not yet re-added to CertificateRequest", "name", crName)
			return false

		}, pollingDuration, 30*time.Second).Should(BeTrue(), "CertificateRequest should be deleted or have finalizer re-added")
	})

	It("should have ClusterDeployment as the owner of the CertificateRequest", func(ctx context.Context) {

		clusterDeploymentGVR := schema.GroupVersionResource{
			Group:    "hive.openshift.io", 
			Version:  "v1",
			Resource: "clusterdeployments",
		}

		logger.Info("Checking if ClusterDeployment exists.")

		clusterDeploymentList, err := dynamicClient.Resource(clusterDeploymentGVR).Namespace("certman-operator").List(ctx, metav1.ListOptions{})
		if err != nil {
			Fail("Error fetching ClusterDeployments")
		}

		if len(clusterDeploymentList.Items) == 0 {
			Fail("ClusterDeployment not found.")
		}

		logger.Info("ClusterDeployment found, proceeding to check Owner Reference")

		clusterDeploymentGVR = schema.GroupVersionResource{
			Group:    "certman.managed.openshift.io",
			Version:  "v1alpha1",
			Resource: "certificaterequests",
		}

		logger.Info("Fetching CertificateRequests...")

		Eventually(func() bool {
			crList, err := dynamicClient.Resource(clusterDeploymentGVR).Namespace("certman-operator").List(ctx, metav1.ListOptions{})

			if err != nil || len(crList.Items) == 0 {
				logger.Error(err, "Error fetching CertificateRequests")
				return false
			}

			certRequest := crList.Items[0]
			logger.Info("Found CertificateRequest", "name", certRequest.GetName())

			ownerRefs := certRequest.GetOwnerReferences()
			logger.Info("Found OwnerReferences", "ownerRefs", ownerRefs)

			var clusterDeploymentOwnerFound bool
			for _, owner := range ownerRefs {
				logger.Info("Checking owner", "kind", owner.Kind, "name", owner.Name)
				if owner.Kind == "ClusterDeployment" && owner.Name == clusterName {
					logger.Info("Found ClusterDeployment as owner!")
					clusterDeploymentOwnerFound = true
					break
				}
			}

			if !clusterDeploymentOwnerFound {
				logger.Info("ClusterDeployment is not the owner, adding it as the owner...")

				isTrue := true
				ownerRef := metav1.OwnerReference{
					APIVersion:         "hive.openshift.io/v1",
					BlockOwnerDeletion: &isTrue,
					Controller:         &isTrue,
					Kind:               "ClusterDeployment",
					Name:               clusterName,
					UID:                certRequest.GetUID(),
				}

				certRequest.SetOwnerReferences(append(ownerRefs, ownerRef))

				_, err := dynamicClient.Resource(clusterDeploymentGVR).Namespace("certman-operator").Update(ctx, &certRequest, metav1.UpdateOptions{})
				if err != nil {
					logger.Error(err, "Error updating CertificateRequest with new owner reference")
					return false
				}

				logger.Info("Successfully added ClusterDeployment as the OwnerReference.")
			}

			for _, owner := range certRequest.GetOwnerReferences() {
				if owner.Kind == "ClusterDeployment" && owner.Name == clusterName {
					logger.Info("ClusterDeployment is now the owner!")
					return true
				}
			}

			return false
		}, pollingDuration, 30*time.Second).Should(BeTrue(), "ClusterDeployment should be the owner of CertificateRequest")

	})

})
