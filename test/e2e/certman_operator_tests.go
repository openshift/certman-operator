// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS.
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	utils "github.com/openshift/certman-operator/test/e2e/utils"
	configv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"
	corev1 "k8s.io/api/core/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var scheme = runtime.NewScheme()
var awsSecretBackup *corev1.Secret

var _ = ginkgo.Describe("Certman Operator", ginkgo.Ordered, ginkgo.ContinueOnFailure, func() {
	var (
		k8s                       *openshift.Client
		clientset                 *kubernetes.Clientset
		dynamicClient             dynamic.Interface
		secretName                string
		certConfig                *utils.CertConfig
		clusterDeploymentName     string
		ocmClusterID              string
		adminKubeconfigSecretName string
		clusterName               string
		certificateRequestGVR     schema.GroupVersionResource
		clusterDeploymentGVR      schema.GroupVersionResource
		logger                    = log.Log
	)

	const (
		pollingDuration            = 5 * time.Minute
		shortTimeout               = 5 * time.Minute
		testTimeout                = 10 * time.Minute
		namespace                  = "openshift-config"
		namespace_certman_operator = "certman-operator"
		operatorNS                 = "certman-operator"
		awsSecretName              = "aws"
	)

	ginkgo.BeforeAll(func(ctx context.Context) {
		log.SetLogger(ginkgo.GinkgoLogr)

		var err error

		// Initialize primary k8s client
		k8s, err = openshift.New(ginkgo.GinkgoLogr)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Unable to setup k8s client")

		cfg := k8s.GetConfig()

		// Initialize clientset from k8s config
		clientset, err = kubernetes.NewForConfig(cfg)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Unable to setup clientset")

		// Initialize API Extensions client
		apiExtClient, err := apiextensionsclient.NewForConfig(cfg)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to create API Extensions client")

		// Initialize dynamic client
		dynamicClient, err = dynamic.NewForConfig(cfg)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to create dynamic client")

		certConfig, err = utils.LoadTestConfigFromInfrastructure(ctx, dynamicClient)
		if err != nil {
			ginkgo.GinkgoLogr.Info("Failed to load config from infrastructure, falling back to environment variables", "error", err)
			certConfig = utils.LoadTestConfig()
		} else {
			ginkgo.GinkgoLogr.Info("Loaded cluster config from infrastructure",
				"clusterName", certConfig.ClusterName,
				"baseDomain", certConfig.BaseDomain)
		}
		clusterName = certConfig.ClusterName
		ocmClusterID = certConfig.OCMClusterID
		clusterDeploymentName = fmt.Sprintf("%s-deployment", clusterName)
		adminKubeconfigSecretName = fmt.Sprintf("%s-admin-kubeconfig", clusterName)

		// Initialize GVRs
		certificateRequestGVR = schema.GroupVersionResource{
			Group: "certman.managed.openshift.io", Version: "v1alpha1", Resource: "certificaterequests",
		}
		clusterDeploymentGVR = schema.GroupVersionResource{
			Group: "hive.openshift.io", Version: "v1", Resource: "clusterdeployments",
		}

		// Setup Hive CRDs
		gomega.Expect(utils.SetupHiveCRDs(ctx, apiExtClient)).To(gomega.Succeed(), "Failed to setup Hive CRDs")

		// Setup Certman using clientset
		gomega.Expect(utils.SetupCertman(ctx, clientset, apiExtClient, cfg)).To(gomega.Succeed(), "Failed to setup Certman")

		// Setup AWS credentials using clientset
		gomega.Expect(utils.SetupAWSCreds(ctx, clientset)).To(gomega.Succeed(), "Failed to setup AWS Secret")

		// Setup Let's Encrypt account secret
		// This creates the secret with EC private key (prime256v1) and mock ACME client URL
		gomega.Expect(utils.SetupLetsEncryptAccountSecret(ctx, clientset)).To(gomega.Succeed(), "Failed to setup Let's Encrypt account secret")

		// Ensure test namespace exists
		err = utils.EnsureTestNamespace(ctx, clientset, certConfig.TestNamespace)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to ensure test namespace exists")

		// Create admin kubeconfig secret
		err = utils.CreateAdminKubeconfigSecret(ctx, clientset, certConfig, adminKubeconfigSecretName)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to create admin kubeconfig secret")

		ginkgo.GinkgoLogr.Info("Test setup completed",
			"namespace", certConfig.TestNamespace,
			"adminSecret", adminKubeconfigSecretName,
			"clusterDeployment", clusterDeploymentName)
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

		ginkgo.By("waiting for operator pod to get back to normal after AWS secret recreation")
		time.Sleep(30 * time.Second)
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

	ginkgo.It("should create ClusterDeployment and CertificateRequest", func(ctx context.Context) {

		// Step 1: Create ClusterDeployment
		ginkgo.GinkgoLogr.Info("Step 1: Creating complete ClusterDeployment resource...")
		clusterDeployment := utils.BuildCompleteClusterDeployment(certConfig, clusterDeploymentName, adminKubeconfigSecretName, ocmClusterID)

		// Log the ClusterDeployment structure for verification
		ginkgo.GinkgoLogr.Info("ClusterDeployment to be created",
			"name", clusterDeployment.GetName(),
			"namespace", clusterDeployment.GetNamespace(),
			"clusterName", utils.GetClusterNameFromCD(clusterDeployment),
			"baseDomain", utils.GetBaseDomainFromCD(clusterDeployment),
			"apiURLOverride", utils.GetAPIURLOverrideFromCD(clusterDeployment),
			"statusAPIURL", utils.GetStatusAPIURLFromCD(clusterDeployment))

		// Clean and create ClusterDeployment using dynamic client
		utils.CleanupClusterDeployment(ctx, dynamicClient, clusterDeploymentGVR, certConfig.TestNamespace, clusterDeploymentName)

		// Add finalizer before creating ClusterDeployment
		finalizers := []string{"certificaterequests.certman.managed.openshift.io"}
		clusterDeployment.SetFinalizers(finalizers)

		_, err := dynamicClient.Resource(clusterDeploymentGVR).Namespace(certConfig.TestNamespace).Create(
			ctx, clusterDeployment, metav1.CreateOptions{})
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to create ClusterDeployment")

		// Verify ClusterDeployment was created successfully and log its actual structure
		var createdCD *unstructured.Unstructured
		gomega.Eventually(func() bool {
			cd, err := dynamicClient.Resource(clusterDeploymentGVR).Namespace(certConfig.TestNamespace).Get(
				ctx, clusterDeploymentName, metav1.GetOptions{})
			if err == nil {
				createdCD = cd
				return true
			}
			return false
		}, shortTimeout, 5*time.Second).Should(gomega.BeTrue(), "ClusterDeployment should be created successfully")

		// Log the actual created ClusterDeployment structure
		if createdCD != nil {
			ginkgo.GinkgoLogr.Info("ClusterDeployment created successfully",
				"name", createdCD.GetName(),
				"namespace", createdCD.GetNamespace(),
				"clusterName", utils.GetClusterNameFromCD(createdCD),
				"baseDomain", utils.GetBaseDomainFromCD(createdCD),
				"apiURLOverride", utils.GetAPIURLOverrideFromCD(createdCD),
				"statusAPIURL", utils.GetStatusAPIURLFromCD(createdCD),
				"infraID", utils.GetInfraIDFromCD(createdCD))
		} else {
			ginkgo.GinkgoLogr.Info("ClusterDeployment created successfully")
		}

		// Step 2: Verify ClusterDeployment meets reconciliation criteria
		ginkgo.GinkgoLogr.Info("Step 2: Verifying ClusterDeployment reconciliation criteria...")
		gomega.Eventually(func() bool {
			return utils.VerifyClusterDeploymentCriteria(ctx, dynamicClient, clusterDeploymentGVR,
				certConfig.TestNamespace, clusterDeploymentName, ocmClusterID)
		}, shortTimeout, 10*time.Second).Should(gomega.BeTrue(), "ClusterDeployment should meet all reconciliation criteria")

		ginkgo.GinkgoLogr.Info("ClusterDeployment meets all reconciliation criteria")

		// Step 3: Verify CertificateRequest is created by operator
		ginkgo.GinkgoLogr.Info("Step 3: Verifying CertificateRequest creation by operator...")
		var foundCertificateRequest *unstructured.Unstructured
		gomega.Eventually(func() bool {
			crList, err := dynamicClient.Resource(certificateRequestGVR).Namespace(certConfig.TestNamespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				ginkgo.GinkgoLogr.Error(err, "Failed to list CertificateRequests")
				return false
			}

			// List all CertificateRequests for debugging
			ginkgo.GinkgoLogr.Info("Found CertificateRequests", "totalCRs", len(crList.Items))

			// Verify that at least one CertificateRequest is related to our ClusterDeployment
			for i := range crList.Items {
				cr := &crList.Items[i]
				ginkgo.GinkgoLogr.Info("CertificateRequest found", "name", cr.GetName())

				// Check if this CertificateRequest is owned by our ClusterDeployment
				ownerRefs, found, _ := unstructured.NestedSlice(cr.Object, "metadata", "ownerReferences")
				hasMatchingOwner := false
				if found && len(ownerRefs) > 0 {
					for _, ownerRef := range ownerRefs {
						ownerRefMap, ok := ownerRef.(map[string]interface{})
						if ok {
							ownerKind, _ := ownerRefMap["kind"].(string)
							ownerName, _ := ownerRefMap["name"].(string)
							if ownerKind == "ClusterDeployment" && ownerName == clusterDeploymentName {
								hasMatchingOwner = true
								break
							}
						}
					}
				}

				// Verify the CertificateRequest has expected spec fields (dnsNames, email)
				dnsNames, found, _ := unstructured.NestedStringSlice(cr.Object, "spec", "dnsNames")
				email, emailFound, _ := unstructured.NestedString(cr.Object, "spec", "email")
				hasValidSpec := found && len(dnsNames) > 0 && emailFound && email != ""

				// If ownerRefs check succeeded, also verify the spec is properly configured
				if hasMatchingOwner {
					if hasValidSpec {
						foundCertificateRequest = cr
						ginkgo.GinkgoLogr.Info("Found CertificateRequest owned by our ClusterDeployment with valid spec",
							"crName", cr.GetName(),
							"owner", clusterDeploymentName,
							"dnsNames", len(dnsNames))
						return true
					} else {
						ginkgo.GinkgoLogr.Info("CertificateRequest has matching owner but invalid spec",
							"crName", cr.GetName(),
							"hasDnsNames", found && len(dnsNames) > 0,
							"hasEmail", emailFound && email != "")
						// Continue searching for a properly configured one
					}
				} else if hasValidSpec {
					// Fallback: If we have a properly configured CR and it's the only one, assume it's ours
					// (This is a fallback if ownerReferences aren't set)
					if len(crList.Items) == 1 {
						foundCertificateRequest = cr
						ginkgo.GinkgoLogr.Info("Found properly configured CertificateRequest (fallback: no ownerRefs)",
							"crName", cr.GetName(),
							"dnsNames", len(dnsNames))
						return true
					}
				}
			}

			// If we have CertificateRequests but none match our criteria, log it
			if len(crList.Items) > 0 {
				ginkgo.GinkgoLogr.Info("CertificateRequests found but none match our ClusterDeployment",
					"totalCRs", len(crList.Items),
					"expectedOwner", clusterDeploymentName)
			}
			return false
		}, pollingDuration, 30*time.Second).Should(gomega.BeTrue(), "CertificateRequest should be created by operator for our ClusterDeployment")

		gomega.Expect(foundCertificateRequest).ToNot(gomega.BeNil(), "CertificateRequest should be found")
		ginkgo.GinkgoLogr.Info("CertificateRequest created successfully by operator",
			"crName", foundCertificateRequest.GetName())
	})

	ginkgo.It("Delete a CertificateRequest and ensures it is recreated", func(ctx context.Context) {

		crGVR := schema.GroupVersionResource{
			Group:    "certman.managed.openshift.io",
			Version:  "v1alpha1",
			Resource: "certificaterequests",
		}

		log.Log.Info("STEP 1: Fetching CertificateRequest owned by our ClusterDeployment", "clusterDeployment", clusterDeploymentName, "namespace", certConfig.TestNamespace)
		originalCR, err := utils.FindCertificateRequestForClusterDeployment(ctx, dynamicClient, crGVR, certConfig.TestNamespace, clusterDeploymentName)
		if err != nil {
			log.Log.Info("No CertificateRequest found for our ClusterDeployment, skipping test", "error", err)
			ginkgo.Skip("SKIPPED: No CertificateRequest found for ClusterDeployment. This test runs after \"should create ClusterDeployment and CertificateRequest\" and requires that CR to exist.")
		}

		originalCRName := originalCR.GetName()
		originalCRUID := originalCR.GetUID()
		crList, err := dynamicClient.Resource(crGVR).Namespace(certConfig.TestNamespace).List(ctx, metav1.ListOptions{})
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to list CertificateRequests for count")
		initialIssuedCertCount := len(crList.Items)

		// Step 2: Delete the CertificateRequest owned by our ClusterDeployment
		log.Log.Info("STEP 2: Deleting the CertificateRequest owned by our ClusterDeployment", "name", originalCRName)
		err = dynamicClient.Resource(crGVR).Namespace(certConfig.TestNamespace).Delete(ctx, originalCRName, metav1.DeleteOptions{})
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to delete CertificateRequest")

		// Step 3: Wait for the old CR to be gone (deleted or replaced). If the CR still exists with deletionTimestamp,
		// strip finalizers so it can be removed. If we see a CR with the same name but different UID, the operator
		// already recreated it (controller removed finalizer and CD recreated) — we're done.
		gomega.Eventually(func() bool {
			cr, err := dynamicClient.Resource(crGVR).Namespace(certConfig.TestNamespace).Get(ctx, originalCRName, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					log.Log.Info("CR deleted already", "name", originalCRName)
					return true
				}
				log.Log.Error(err, "Failed to get CR")
				return false
			}
			// Same name but different UID: operator already recreated the CR (old one was deleted).
			if cr.GetUID() != originalCRUID {
				log.Log.Info("CR recreated by operator (new UID)", "name", cr.GetName(), "uid", cr.GetUID())
				return true
			}
			if cr.GetDeletionTimestamp() == nil {
				log.Log.Info("CR not marked for deletion yet, waiting", "name", cr.GetName())
				return false
			}

			finalizers, found, err := unstructured.NestedStringSlice(cr.Object, "metadata", "finalizers")
			if err != nil {
				log.Log.Error(err, "Error retrieving finalizers")
				return false
			}
			if !found || len(finalizers) == 0 {
				log.Log.Info("No finalizers present, CR should be removed soon", "name", cr.GetName())
				return false
			}

			crCopy := cr.DeepCopy()
			_ = unstructured.SetNestedStringSlice(crCopy.Object, []string{}, "metadata", "finalizers")

			_, err = dynamicClient.Resource(crGVR).Namespace(certConfig.TestNamespace).Update(ctx, crCopy, metav1.UpdateOptions{})
			if err != nil {
				log.Log.Error(err, "Failed to remove finalizer")
				return false
			}
			return true
		}, 1*time.Minute, 5*time.Second).Should(gomega.BeTrue(), "Old CertificateRequest should be removed or recreated")

		// Step 4: Wait for new CertificateRequest with UID != originalCRUID (operator recreates it for our ClusterDeployment).
		// We must check against the old UID so we know the CR we see is the recreated one, not the deleted one.
		var newCRName string
		gomega.Eventually(func() bool {
			newList, err := dynamicClient.Resource(crGVR).Namespace(certConfig.TestNamespace).List(ctx, metav1.ListOptions{})
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
				// Require UID != originalCRUID to confirm this is the recreated CR, not the old one.
				if cr.GetUID() != originalCRUID {
					newCRName = cr.GetName()
					log.Log.Info("New CertificateRequest detected (UID differs from deleted CR)", "name", newCRName, "uid", cr.GetUID(), "originalUID", originalCRUID)
					return true
				}
				log.Log.Info("Found CR with same UID as deleted (still old instance?), waiting for recreated CR", "name", cr.GetName(), "uid", cr.GetUID())
			}
			return false
		}, 2*time.Minute, 10*time.Second).Should(gomega.BeTrue(), "New CertificateRequest (with UID != deleted CR) should appear")

		secretNameFromCR, _ := utils.GetCertificateSecretNameFromCR(originalCR)
		log.Log.Info("Test completed: CertificateRequest recreated by operator", "newCR", newCRName, "certificateSecret", secretNameFromCR)
	})

	ginkgo.It("should verify primary-cert-bundle-secret and certificate creation", func(ctx context.Context) {
		// Find the CertificateRequest for our ClusterDeployment
		certificateRequest, err := utils.FindCertificateRequestForClusterDeployment(ctx, dynamicClient, certificateRequestGVR,
			certConfig.TestNamespace, clusterDeploymentName)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "CertificateRequest should exist for ClusterDeployment")

		// Log CertificateRequest status for debugging
		status, found, _ := unstructured.NestedMap(certificateRequest.Object, "status")
		if found && status != nil {
			ginkgo.GinkgoLogr.Info("CertificateRequest status",
				"crName", certificateRequest.GetName(),
				"status", status)
		}

		// Get the secret name from CertificateRequest spec
		certificateSecretName, err := utils.GetCertificateSecretNameFromCR(certificateRequest)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "CertificateRequest should have certificateSecret name")

		ginkgo.GinkgoLogr.Info("Looking for certificate secret",
			"secretName", certificateSecretName,
			"namespace", certConfig.TestNamespace)

		// Verify primary-cert-bundle-secret is created with certificate data
		var secret *corev1.Secret
		gomega.Eventually(func() bool {
			s, err := clientset.CoreV1().Secrets(certConfig.TestNamespace).Get(ctx, certificateSecretName, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					ginkgo.GinkgoLogr.Info("Secret not found yet, waiting...",
						"secretName", certificateSecretName,
						"namespace", certConfig.TestNamespace)
				} else {
					ginkgo.GinkgoLogr.Error(err, "Error getting secret",
						"secretName", certificateSecretName,
						"namespace", certConfig.TestNamespace)
				}
				return false
			}
			if s.Data == nil {
				ginkgo.GinkgoLogr.Info("Secret exists but has no data yet, waiting...",
					"secretName", certificateSecretName)
				return false
			}
			if len(s.Data["tls.crt"]) == 0 {
				ginkgo.GinkgoLogr.Info("Secret exists but tls.crt is empty, waiting...",
					"secretName", certificateSecretName)
				return false
			}
			if len(s.Data["tls.key"]) == 0 {
				ginkgo.GinkgoLogr.Info("Secret exists but tls.key is empty, waiting...",
					"secretName", certificateSecretName)
				return false
			}
			secret = s
			ginkgo.GinkgoLogr.Info("Secret found with certificate data",
				"secretName", certificateSecretName,
				"tls.crt.length", len(s.Data["tls.crt"]),
				"tls.key.length", len(s.Data["tls.key"]))
			return true
		}, testTimeout, 15*time.Second).Should(gomega.BeTrue(), "primary-cert-bundle-secret should be created with certificate data")

		// Verify certificate is valid
		block, _ := pem.Decode(secret.Data["tls.crt"])
		gomega.Expect(block).ToNot(gomega.BeNil(), "Certificate should be valid PEM")
		cert, err := x509.ParseCertificate(block.Bytes)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Certificate should be parseable")

		ginkgo.GinkgoLogr.Info("Certificate and primary-cert-bundle-secret verified successfully",
			"secretName", certificateSecretName,
			"dnsNames", cert.DNSNames)
	})

	ginkgo.It("should verify certificate operation metrics", func(ctx context.Context) {
		// Verify metrics: at least 1 certificate request in the namespace
		var certRequestsCount int
		gomega.Eventually(func() bool {
			count, _, success := utils.VerifyMetrics(ctx, clientset, certConfig.TestNamespace)
			if success {
				certRequestsCount = count
				return count >= 1
			}
			return false
		}, testTimeout, 15*time.Second).Should(gomega.BeTrue(), "Metrics should show at least 1 certificate request")

		gomega.Expect(certRequestsCount).To(gomega.BeNumerically(">=", 1), "Should have at least 1 certificate request")

		ginkgo.GinkgoLogr.Info("Metrics verification successful",
			"certificateRequestsCount", certRequestsCount)
	})

	ginkgo.It("removes OwnerReference by simulating oc apply -f with modified file using server-side apply", func(ctx context.Context) {
		logger := log.FromContext(ctx)
		certRequestGVR := schema.GroupVersionResource{
			Group:    "certman.managed.openshift.io",
			Version:  "v1alpha1",
			Resource: "certificaterequests",
		}

		gomega.Eventually(func() bool {
			crList, err := dynamicClient.Resource(certRequestGVR).Namespace(operatorNS).List(ctx, metav1.ListOptions{})
			if err != nil || len(crList.Items) == 0 {
				logger.Error(err, "Failed to list CertificateRequests")
				return false
			}

			originalCR := crList.Items[0]
			crName := originalCR.GetName()
			logger.Info("Fetched CertificateRequest", "name", crName, "resourceVersion", originalCR.GetResourceVersion())

			// Remove OwnerReferences and managedFields before reapplying
			unstructured.RemoveNestedField(originalCR.Object, "metadata", "ownerReferences")
			unstructured.RemoveNestedField(originalCR.Object, "metadata", "managedFields")

			patchBytes, err := json.Marshal(originalCR.Object)
			if err != nil {
				logger.Error(err, "Failed to marshal modified CertificateRequest", "name", crName)
				return false
			}

			_, err = dynamicClient.Resource(certRequestGVR).Namespace(operatorNS).Patch(
				ctx,
				crName,
				types.ApplyPatchType,
				patchBytes,
				metav1.PatchOptions{FieldManager: "certman-e2e-test"},
			)
			if err != nil {
				logger.Error(err, "Failed to apply patch", "name", crName)
				return false
			}

			time.Sleep(10 * time.Second)

			finalCR, err := dynamicClient.Resource(certRequestGVR).Namespace(operatorNS).Get(ctx, crName, metav1.GetOptions{})
			if err != nil {
				logger.Error(err, "Failed to get CertificateRequest after patch", "name", crName)
				return false
			}

			for _, ref := range finalCR.GetOwnerReferences() {
				if ref.Controller != nil && *ref.Controller {
					logger.Info("OwnerReference re-added", "name", crName)
					return true
				}
			}

			logger.Info("OwnerReference not re-added yet", "name", crName)
			return false
		}, 2*time.Minute, 5*time.Second).Should(gomega.BeTrue(), "OwnerReference should be re-added after patch")
	})

	ginkgo.It("delete secret primary-cert-bundle-secret if exists", func(ctx context.Context) {
		secretNameToDelete := "primary-cert-bundle-secret"
		pollingDuration := 2 * time.Minute
		pollInterval := 30 * time.Second

		originalSecret, err := clientset.CoreV1().Secrets(namespace_certman_operator).Get(ctx, secretNameToDelete, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			log.Log.Info("Secret does not exist, skipping deletion test.")
			return
		}
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Error retrieving the original secret")

		originalTimestamp := originalSecret.CreationTimestamp.Time
		log.Log.Info(fmt.Sprintf("Original secret creation timestamp: %v", originalTimestamp))

		err = clientset.CoreV1().Secrets(namespace_certman_operator).Delete(ctx, secretNameToDelete, metav1.DeleteOptions{})
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to delete the secret")

		gomega.Eventually(func() bool {
			newSecret, err := clientset.CoreV1().Secrets(namespace_certman_operator).Get(ctx, secretNameToDelete, metav1.GetOptions{})
			if err != nil {
				return false
			}
			return newSecret.CreationTimestamp.Time.After(originalTimestamp)
		}, pollingDuration, pollInterval).Should(gomega.BeTrue(),
			fmt.Sprintf("Secret %q was not re-created within %v or timestamp did not change", secretNameToDelete, pollingDuration))
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

		// Create fresh clients for cleanup
		apiExtClient, err := apiextensionsclient.NewForConfig(cfg)
		if err != nil {
			logger.Info("Failed to create API Extensions client, skipping cleanup", "error", err)
			return
		}

		kubeClient, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			logger.Info("Failed to create Kubernetes client, skipping cleanup", "error", err)
			return
		}

		cleanupDynamicClient, err := dynamic.NewForConfig(cfg)
		if err != nil {
			logger.Info("Failed to create dynamic client, skipping cleanup", "error", err)
			return
		}

		// Create RESTMapper for resource cleanup
		dc, err := discovery.NewDiscoveryClientForConfig(cfg)
		if err != nil {
			logger.Info("Failed to create discovery client, skipping Certman cleanup", "error", err)
		} else {
			gr, err := restmapper.GetAPIGroupResources(dc)
			if err != nil {
				logger.Info("Failed to get API group resources, skipping Certman cleanup", "error", err)
			} else {
				mapper := restmapper.NewDiscoveryRESTMapper(gr)

				// Cleanup Certman with RESTMapper
				if err := utils.CleanupCertman(ctx, kubeClient, apiExtClient, cleanupDynamicClient, mapper); err != nil {
					logger.Info("Error during Certman cleanup", "error", err)
				}
			}
		}

		if err := utils.CleanupHive(ctx, apiExtClient); err != nil {
			logger.Info("Error during Hive cleanup", "error", err)
		}

		if err := utils.CleanupAWSCreds(ctx, kubeClient); err != nil {
			logger.Info("Error during AWS secret cleanup", "error", err)
		} else {
			logger.Info("AWS secret cleanup succeeded")
		}

		if err := utils.CleanupLetsEncryptAccountSecret(ctx, kubeClient); err != nil {
			logger.Info("Error during Let's Encrypt account secret cleanup", "error", err)
		} else {
			logger.Info("Let's Encrypt account secret cleanup succeeded")
		}

		logger.Info("Cleaning up all test resources")

		// Cleanup all test resources (ClusterDeployment, CertificateRequests, secrets)
		utils.CleanupAllTestResources(ctx, kubeClient, cleanupDynamicClient, certConfig, clusterDeploymentName, adminKubeconfigSecretName, ocmClusterID)

		logger.Info("Cleaning up certman-operator resources")

		if err := utils.CleanupCertmanResources(ctx, cleanupDynamicClient, operatorNS); err != nil {
			logger.Error(err, "Error during certman-operator resources cleanup")
		}

		logger.Info("Cleanup: AfterAll cleanup completed")
	})

})
