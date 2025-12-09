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

	"k8s.io/apimachinery/pkg/runtime/schema"

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

	ginkgo.It("should install the certman-operator via catalogsource", func(ctx context.Context) {

		gomega.Eventually(func() bool {

			if !utils.CreateCertmanResources(ctx, dynamicClient, operatorNS) {
				logger.Info("Failed to create certman-operator resources")
				return false
			}

			logger.Info("Resources created successfully. Waiting for csv to get installed...")

			time.Sleep(30 * time.Second)

			currentVersion, err := utils.GetCurrentCSVVersion(ctx, dynamicClient, operatorNS)
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to get current CSV version")

			logger.Info("Current operator version. Waiting for certman operator pod to be in running state", "version", currentVersion)

			time.Sleep(30 * time.Second)

			if !utils.CheckPodStatus(ctx, clientset, operatorNS) {
				logger.Info("certman-operator pod is not in running state")
				return false
			}

			logger.Info("certman-operator pod is running successfully")
			return true

		}, pollingDuration, 30*time.Second).Should(gomega.BeTrue(), "certman-operator should be installed and running successfully")
	})

	ginkgo.It("should check for upgrades and upgrade certman-operator if available", func(ctx context.Context) {

		ginkgo.By("checking current operator version")
		currentVersion, err := utils.GetCurrentCSVVersion(ctx, dynamicClient, operatorNS)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to get current CSV version")

		logger.Info("Current operator version", "version", currentVersion)

		ginkgo.By("checking for available upgrades")
		hasUpgrade, currentVer, latestVer, err := utils.CheckForUpgrade(ctx, dynamicClient, operatorNS)
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to check for upgrades")

		if hasUpgrade {
			logger.Info("Upgrade available", "current", currentVer, "latest", latestVer)

			ginkgo.By("performing operator upgrade")
			err = utils.UpgradeOperatorToLatest(ctx, dynamicClient, operatorNS)
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to upgrade operator")

			ginkgo.By("verifying operator is running after upgrade")
			gomega.Eventually(func() bool {
				return utils.CheckPodStatus(ctx, clientset, operatorNS)
			}, pollingDuration, 30*time.Second).Should(gomega.BeTrue(), "Operator should be running after upgrade")

			ginkgo.By("verifying upgraded version")
			upgradedVersion, err := utils.GetCurrentCSVVersion(ctx, dynamicClient, operatorNS)
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to get upgraded CSV version")

			logger.Info("Operator upgraded successfully", "from", currentVer, "to", upgradedVersion)

			gomega.Expect(upgradedVersion).ToNot(gomega.Equal(currentVer), "Version should have changed after upgrade")
		} else {
			logger.Info("No upgrade available", "version", currentVer)
		}
	})

	ginkgo.It("should automatically ensure finalizer is present on ClusterDeployment when not being deleted", func(ctx context.Context) {
		clusterDeploymentGVR := schema.GroupVersionResource{
			Group:    "hive.openshift.io",
			Version:  "v1",
			Resource: "clusterdeployments",
		}

		ginkgo.By("fetching ClusterDeployment")
		cdList, err := dynamicClient.Resource(clusterDeploymentGVR).Namespace("certman-operator").List(ctx, metav1.ListOptions{})
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Error listing ClusterDeployments")
		gomega.Expect(len(cdList.Items)).To(gomega.BeNumerically(">", 0), "No ClusterDeployment found")

		clusterDeployment := cdList.Items[0]
		cdName := clusterDeployment.GetName()
		logger.Info("Processing ClusterDeployment", "name", cdName)

		// Verify ClusterDeployment is not being deleted
		deletionTimestamp := clusterDeployment.GetDeletionTimestamp()
		gomega.Expect(deletionTimestamp).To(gomega.BeNil(), "ClusterDeployment should not be deleted for this test")

		// Check if the certman finalizer is missing (simulating a scenario where it was removed externally)
		// The operator should automatically add it back when reconciling
		finalizers := clusterDeployment.GetFinalizers()
		hasCertmanFinalizer := false
		for _, finalizer := range finalizers {
			if finalizer == "certificaterequests.certman.managed.openshift.io" {
				hasCertmanFinalizer = true
				break
			}
		}

		if !hasCertmanFinalizer {
			logger.Info("Certman finalizer is missing, waiting for operator to add it", "name", cdName)
		} else {
			logger.Info("Certman finalizer already present, verifying operator maintains it", "name", cdName)
		}

		ginkgo.By("verifying operator ensures the finalizer is present")
		gomega.Eventually(func() bool {
			updatedCD, err := dynamicClient.Resource(clusterDeploymentGVR).Namespace("certman-operator").Get(ctx, cdName, metav1.GetOptions{})
			if err != nil {
				logger.Error(err, "Failed to get ClusterDeployment", "name", cdName)
				return false
			}

			// Verify it's still not being deleted
			if updatedCD.GetDeletionTimestamp() != nil {
				logger.Info("ClusterDeployment is being deleted, skipping finalizer check", "name", cdName)
				return false
			}

			updatedFinalizers := updatedCD.GetFinalizers()
			for _, finalizer := range updatedFinalizers {
				if finalizer == "certificaterequests.certman.managed.openshift.io" {
					logger.Info("Operator has ensured finalizer is present on ClusterDeployment", "name", cdName, "finalizer", finalizer)
					return true
				}
			}

			logger.Info("Finalizer not yet present on ClusterDeployment, waiting for operator to add it", "name", cdName)
			return false

		}, pollingDuration, 30*time.Second).Should(gomega.BeTrue(), "Operator should ensure ClusterDeployment has the certman finalizer when not being deleted")
	})

	ginkgo.It("should automatically ensure finalizer is present on CertificateRequest when not being deleted", func(ctx context.Context) {
		certRequestGVR := schema.GroupVersionResource{
			Group:    "certman.managed.openshift.io",
			Version:  "v1alpha1",
			Resource: "certificaterequests",
		}

		ginkgo.By("fetching CertificateRequest")
		crList, err := dynamicClient.Resource(certRequestGVR).Namespace("certman-operator").List(ctx, metav1.ListOptions{})
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Error listing CertificateRequests")
		gomega.Expect(len(crList.Items)).To(gomega.BeNumerically(">", 0), "No CertificateRequest found")

		certRequest := crList.Items[0]
		crName := certRequest.GetName()
		logger.Info("Processing CertificateRequest", "name", crName)

		// Verify CertificateRequest is not being deleted
		deletionTimestamp := certRequest.GetDeletionTimestamp()
		gomega.Expect(deletionTimestamp).To(gomega.BeNil(), "CertificateRequest should not be deleted for this test")

		// Check if the certman finalizer is missing (simulating a scenario where it was removed externally)
		// The operator should automatically add it back when reconciling
		finalizers := certRequest.GetFinalizers()
		hasCertmanFinalizer := false
		for _, finalizer := range finalizers {
			if finalizer == "certificaterequests.certman.managed.openshift.io" {
				hasCertmanFinalizer = true
				break
			}
		}

		if !hasCertmanFinalizer {
			logger.Info("Certman finalizer is missing, waiting for operator to add it", "name", crName)
		} else {
			logger.Info("Certman finalizer already present, verifying operator maintains it", "name", crName)
		}

		ginkgo.By("verifying operator ensures the finalizer is present")
		gomega.Eventually(func() bool {
			updatedCR, err := dynamicClient.Resource(certRequestGVR).Namespace("certman-operator").Get(ctx, crName, metav1.GetOptions{})
			if err != nil {
				logger.Error(err, "Failed to get CertificateRequest", "name", crName)
				return false
			}

			// Verify it's still not being deleted
			if updatedCR.GetDeletionTimestamp() != nil {
				logger.Info("CertificateRequest is being deleted, skipping finalizer check", "name", crName)
				return false
			}

			updatedFinalizers := updatedCR.GetFinalizers()
			for _, finalizer := range updatedFinalizers {
				if finalizer == "certificaterequests.certman.managed.openshift.io" {
					logger.Info("Operator has ensured finalizer is present on CertificateRequest", "name", crName, "finalizer", finalizer)
					return true
				}
			}

			logger.Info("Finalizer not yet present on CertificateRequest, waiting for operator to add it", "name", crName)
			return false

		}, pollingDuration, 30*time.Second).Should(gomega.BeTrue(), "Operator should ensure CertificateRequest has the certman finalizer when not being deleted")
	})

	ginkgo.It("should have ClusterDeployment as the owner of the CertificateRequest", func(ctx context.Context) {
		logger.Info("waiting to ckeck if finalizer is there or not")
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

		ginkgo.By("fetching ClusterDeployment to get its name and UID")
		clusterDeploymentList, err := dynamicClient.Resource(clusterDeploymentGVR).Namespace("certman-operator").List(ctx, metav1.ListOptions{})
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Error fetching ClusterDeployments")
		gomega.Expect(len(clusterDeploymentList.Items)).To(gomega.BeNumerically(">", 0), "ClusterDeployment not found")

		clusterDeployment := clusterDeploymentList.Items[0]
		cdName := clusterDeployment.GetName()
		cdUID := clusterDeployment.GetUID()
		logger.Info("Found ClusterDeployment", "name", cdName, "uid", cdUID)

		ginkgo.By("fetching CertificateRequest")
		crList, err := dynamicClient.Resource(certRequestGVR).Namespace("certman-operator").List(ctx, metav1.ListOptions{})
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Error fetching CertificateRequests")
		gomega.Expect(len(crList.Items)).To(gomega.BeNumerically(">", 0), "No CertificateRequest found")

		certRequest := crList.Items[0]
		crName := certRequest.GetName()
		logger.Info("Found CertificateRequest", "name", crName)

		ginkgo.By("removing owner reference from CertificateRequest to test operator functionality")
		certRequest.SetOwnerReferences([]metav1.OwnerReference{})
		_, err = dynamicClient.Resource(certRequestGVR).Namespace("certman-operator").Update(ctx, &certRequest, metav1.UpdateOptions{})
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to remove owner reference from CertificateRequest")
		logger.Info("Owner reference removed from CertificateRequest", "name", crName)

		ginkgo.By("verifying operator automatically adds ClusterDeployment as owner reference")
		gomega.Eventually(func() bool {
			updatedCR, err := dynamicClient.Resource(certRequestGVR).Namespace("certman-operator").Get(ctx, crName, metav1.GetOptions{})
			if err != nil {
				logger.Error(err, "Failed to get CertificateRequest", "name", crName)
				return false
			}

			ownerRefs := updatedCR.GetOwnerReferences()
			for _, owner := range ownerRefs {
				if owner.Kind == "ClusterDeployment" && owner.Name == cdName {
					logger.Info("ClusterDeployment has been added as owner by operator", "name", crName, "owner", owner.Name)
					return true
				}
			}

			logger.Info("Owner reference not yet added by operator", "name", crName)
			return false
		}, pollingDuration, 30*time.Second).Should(gomega.BeTrue(), "ClusterDeployment should be automatically added as owner of CertificateRequest by operator")
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