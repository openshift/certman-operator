// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS.
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"crypto/x509"
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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
		pollingDuration = 5 * time.Minute
		shortTimeout    = 5 * time.Minute
		testTimeout     = 10 * time.Minute
		namespace       = "openshift-config"
		operatorNS      = "certman-operator"
		awsSecretName   = "aws"
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
			ginkgo.GinkgoLogr.Info("✅ ClusterDeployment created successfully",
				"name", createdCD.GetName(),
				"namespace", createdCD.GetNamespace(),
				"clusterName", utils.GetClusterNameFromCD(createdCD),
				"baseDomain", utils.GetBaseDomainFromCD(createdCD),
				"apiURLOverride", utils.GetAPIURLOverrideFromCD(createdCD),
				"statusAPIURL", utils.GetStatusAPIURLFromCD(createdCD),
				"infraID", utils.GetInfraIDFromCD(createdCD))
		} else {
			ginkgo.GinkgoLogr.Info("✅ ClusterDeployment created successfully")
		}

		// Step 2: Verify ClusterDeployment meets reconciliation criteria
		ginkgo.GinkgoLogr.Info("Step 2: Verifying ClusterDeployment reconciliation criteria...")
		gomega.Eventually(func() bool {
			return utils.VerifyClusterDeploymentCriteria(ctx, dynamicClient, clusterDeploymentGVR,
				certConfig.TestNamespace, clusterDeploymentName, ocmClusterID)
		}, shortTimeout, 10*time.Second).Should(gomega.BeTrue(), "ClusterDeployment should meet all reconciliation criteria")

		ginkgo.GinkgoLogr.Info("✅ ClusterDeployment meets all reconciliation criteria")

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
						ginkgo.GinkgoLogr.Info("✅ Found CertificateRequest owned by our ClusterDeployment with valid spec",
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
						ginkgo.GinkgoLogr.Info("✅ Found properly configured CertificateRequest (fallback: no ownerRefs)",
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
		ginkgo.GinkgoLogr.Info("✅ CertificateRequest created successfully by operator",
			"crName", foundCertificateRequest.GetName())
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
			ginkgo.GinkgoLogr.Info("✅ Secret found with certificate data",
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

		ginkgo.GinkgoLogr.Info("✅ Certificate and primary-cert-bundle-secret verified successfully",
			"secretName", certificateSecretName,
			"dnsNames", cert.DNSNames)
	})

	ginkgo.It("should verify certificate operation metrics", func(ctx context.Context) {
		// Verify metrics: exactly 1 certificate request
		var certRequestsCount int
		gomega.Eventually(func() bool {
			count, _, success := utils.VerifyMetrics(ctx, clientset, certConfig.TestNamespace)
			if success {
				certRequestsCount = count
				return count == 1
			}
			return false
		}, testTimeout, 15*time.Second).Should(gomega.BeTrue(), "Metrics should show exactly 1 certificate request")

		gomega.Expect(certRequestsCount).To(gomega.Equal(1), "Should have exactly 1 certificate request")

		ginkgo.GinkgoLogr.Info("✅ Metrics verification successful",
			"certificateRequestsCount", certRequestsCount)
	})

	ginkgo.It("should properly cleanup resources when ClusterDeployment is deleted", func(ctx context.Context) {
		logger.Info("Test - ClusterDeployment deletion cleanup")

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

		var cdName string
		var cdNamespace string

		ginkgo.By("verifying ClusterDeployment exists with certman-operator finalizer")
		gomega.Eventually(func() bool {
			listOpts := metav1.ListOptions{
				LabelSelector: "api.openshift.com/managed=true",
			}
			cdList, err := dynamicClient.Resource(clusterDeploymentGVR).Namespace(operatorNS).List(ctx, listOpts)
			if err != nil {
				logger.Error(err, "Failed to list ClusterDeployments")
				return false
			}
			if len(cdList.Items) == 0 {
				logger.Info("No managed ClusterDeployment found in namespace", "namespace", operatorNS)
				return false
			}

			if len(cdList.Items) > 1 {
				logger.Info("Warning: Multiple managed ClusterDeployments found, using the first one", "count", len(cdList.Items))
			}

			cd := cdList.Items[0]
			cdName = cd.GetName()
			cdNamespace = cd.GetNamespace()
			finalizers := cd.GetFinalizers()
			logger.Info("Found ClusterDeployment", "name", cdName, "namespace", cdNamespace, "finalizers", finalizers)

			for _, f := range finalizers {
				if f == "certificaterequests.certman.managed.openshift.io" {
					return true
				}
			}
			logger.Info("ClusterDeployment does not have the certman finalizer yet", "name", cdName)
			return false
		}, pollingDuration, 15*time.Second).Should(gomega.BeTrue(), "ClusterDeployment should exist with certman-operator finalizer")

		ginkgo.By("verifying CertificateRequests exist before deletion")
		var certRequestNames []string
		gomega.Eventually(func() bool {
			crList, err := dynamicClient.Resource(certRequestGVR).Namespace(cdNamespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				logger.Error(err, "Failed to list CertificateRequests")
				return false
			}
			if len(crList.Items) == 0 {
				logger.Info("No CertificateRequests found yet")
				return false
			}

			certRequestNames = []string{}
			for _, cr := range crList.Items {
				certRequestNames = append(certRequestNames, cr.GetName())
			}
			logger.Info("Found CertificateRequests", "count", len(certRequestNames), "names", certRequestNames)
			return true
		}, pollingDuration, 15*time.Second).Should(gomega.BeTrue(), "CertificateRequests should exist before ClusterDeployment deletion")

		ginkgo.By("verifying primary-cert-bundle-secret exists before deletion")
		gomega.Eventually(func() bool {
			_, err := clientset.CoreV1().Secrets(cdNamespace).Get(ctx, "primary-cert-bundle-secret", metav1.GetOptions{})
			if err != nil {
				logger.Info("primary-cert-bundle-secret not found yet")
				return false
			}
			logger.Info("Found primary-cert-bundle-secret")
			return true
		}, pollingDuration, 15*time.Second).Should(gomega.BeTrue(), "primary-cert-bundle-secret should exist before ClusterDeployment deletion")

		ginkgo.By("deleting ClusterDeployment")
		err := dynamicClient.Resource(clusterDeploymentGVR).Namespace(cdNamespace).Delete(ctx, cdName, metav1.DeleteOptions{})
		gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to delete ClusterDeployment")
		logger.Info("Successfully initiated ClusterDeployment deletion", "name", cdName)

		ginkgo.By("verifying certman-operator finalizer does not block ClusterDeployment deletion")
		gomega.Eventually(func() bool {
			cd, err := dynamicClient.Resource(clusterDeploymentGVR).Namespace(cdNamespace).Get(ctx, cdName, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					logger.Info("ClusterDeployment has been deleted")
					return true
				}
				logger.Error(err, "Error getting ClusterDeployment")
				return false
			}

			finalizers := cd.GetFinalizers()
			hasCertmanFinalizer := false
			for _, f := range finalizers {
				if f == "certificaterequests.certman.managed.openshift.io" {
					hasCertmanFinalizer = true
					break
				}
			}

			if !hasCertmanFinalizer {
				logger.Info("certman-operator finalizer has been removed from ClusterDeployment")
				return true
			}

			logger.Info("Waiting for certman-operator to remove its finalizer", "currentFinalizers", finalizers)
			return false
		}, pollingDuration, 15*time.Second).Should(gomega.BeTrue(), "certman-operator finalizer should not block ClusterDeployment deletion")

		ginkgo.By("verifying CertificateRequests are deleted when ClusterDeployment is deleted")
		gomega.Eventually(func() bool {
			crList, err := dynamicClient.Resource(certRequestGVR).Namespace(cdNamespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				logger.Error(err, "Failed to list CertificateRequests")
				return false
			}

			if len(crList.Items) > 0 {
				remainingCRs := []string{}
				for _, cr := range crList.Items {
					remainingCRs = append(remainingCRs, cr.GetName())
				}
				logger.Info("CertificateRequests still present, waiting for cleanup", "remaining", remainingCRs)
				return false
			}

			logger.Info("All CertificateRequests have been deleted")
			return true
		}, pollingDuration, 15*time.Second).Should(gomega.BeTrue(), "CertificateRequests should be deleted when ClusterDeployment is deleted")

		ginkgo.By("verifying primary-cert-bundle-secret is deleted when ClusterDeployment is deleted")
		gomega.Eventually(func() bool {
			_, err := clientset.CoreV1().Secrets(cdNamespace).Get(ctx, "primary-cert-bundle-secret", metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					logger.Info("primary-cert-bundle-secret has been deleted")
					return true
				}
				logger.Error(err, "Error checking for primary-cert-bundle-secret")
				return false
			}
			logger.Info("primary-cert-bundle-secret still exists, waiting for cleanup")
			return false
		}, pollingDuration, 15*time.Second).Should(gomega.BeTrue(), "primary-cert-bundle-secret should be deleted when ClusterDeployment is deleted")

		logger.Info("ClusterDeployment deletion cleanup test completed successfully")
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

		logger.Info("Cleaning up certman-operator resources")

		if err := utils.CleanupCertmanResources(ctx, dynamicClient, operatorNS); err != nil {
			logger.Error(err, "Error during certman-operator resources cleanup")
		}

		logger.Info("Cleanup: AfterAll cleanup completed")
	})
})
