// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS.
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	utils "github.com/openshift/certman-operator/test/e2e/utils"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// "Certman Operator Hive" exercises the already-deployed, PKO-managed certman-operator instance
// directly on a live Hive cluster (e.g. hivei01ue1), selected via GINKGO_FOCUS="Certman Operator
// Hive" -- distinct from the "Certman Operator" suite above, which installs/upgrades/deletes its
// own copy of the operator, Hive CRDs, and shared secrets, and therefore only runs safely against
// a fully disposable cluster.
//
// This is the suite for both the Prow PR presubmit and the SAPM-triggered promotion-int/
// promotion-stage postsubmit gates. In promotion-gate mode (OPERATOR_IMAGE unset) the operator is
// already deployed on the target hive via PKO, so there's nothing to install -- this suite only
// verifies that real, already-running instance. In presubmit mode (OPERATOR_IMAGE set) it deploys
// the PR's candidate image as its own namespace-scoped instance (see deployCandidateOperator)
// before verifying. Either way, it never touches the shared certman-operator namespace, Hive
// CRDs, or credentials outside the namespace it creates and owns for the duration of the run. It
// creates a uniquely-named ClusterDeployment and lets the operator reconcile a CertificateRequest
// for it, then tears down by deleting only that namespace.
var _ = ginkgo.Describe("Certman Operator Hive", ginkgo.Ordered, ginkgo.ContinueOnFailure, func() {
	var (
		k8s                       *openshift.Client
		clientset                 *kubernetes.Clientset
		dynamicClient             dynamic.Interface
		certConfig                *utils.CertConfig
		clusterDeploymentName     string
		ocmClusterID              string
		adminKubeconfigSecretName string
		certificateRequestGVR     schema.GroupVersionResource
		clusterDeploymentGVR      schema.GroupVersionResource
		// testOperatorNamespace is which namespace's certman-operator pod the reconcile tests
		// verify against: the real production namespace in promotion-gate mode, or this run's
		// own dedicated namespace when a candidate image is deployed for a presubmit.
		testOperatorNamespace = "certman-operator"
		logger                = log.Log
	)

	const (
		operatorNamespace = "certman-operator"
		shortTimeout      = 5 * time.Minute
		pollingDuration   = 5 * time.Minute
	)

	ginkgo.BeforeAll(func(ctx context.Context) {
		log.SetLogger(ginkgo.GinkgoLogr)

		var err error
		k8s, err = openshift.New(ginkgo.GinkgoLogr)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Unable to setup k8s client")

		cfg := k8s.GetConfig()

		clientset, err = kubernetes.NewForConfig(cfg)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Unable to setup clientset")

		dynamicClient, err = dynamic.NewForConfig(cfg)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to create dynamic client")

		certConfig, err = utils.LoadTestConfigFromInfrastructure(ctx, dynamicClient)
		if err != nil {
			ginkgo.GinkgoLogr.Info("Failed to load config from infrastructure, falling back to environment variables", "error", err)
			certConfig = utils.LoadTestConfig()
		}

		// Isolate every object this suite creates in its own namespace, unique per run, so
		// concurrent runs (a presubmit and a promotion gate, or multiple presubmits) against the
		// same shared Hive cluster never collide, and cleanup never has to touch the real
		// certman-operator namespace, CRDs, or credentials.
		runSuffix := rand.String(6)
		certConfig.TestNamespace = fmt.Sprintf("certman-e2e-%s", runSuffix)
		clusterDeploymentName = fmt.Sprintf("certman-e2e-%s-deployment", runSuffix)
		ocmClusterID = certConfig.OCMClusterID
		adminKubeconfigSecretName = fmt.Sprintf("%s-admin-kubeconfig", clusterDeploymentName)

		certificateRequestGVR = schema.GroupVersionResource{
			Group: "certman.managed.openshift.io", Version: "v1alpha1", Resource: "certificaterequests",
		}
		clusterDeploymentGVR = schema.GroupVersionResource{
			Group: "hive.openshift.io", Version: "v1", Resource: "clusterdeployments",
		}

		// Hive CRDs and the certman-operator Deployment are assumed to already exist and be
		// healthy on this cluster -- this suite never installs, upgrades, or reconfigures them.
		err = utils.EnsureTestNamespace(ctx, clientset, certConfig.TestNamespace)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to create dedicated test namespace")

		err = utils.CreateAdminKubeconfigSecret(ctx, clientset, certConfig, adminKubeconfigSecretName)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to create admin kubeconfig secret")

		// The ClusterDeployment references an "aws" credentials secret in its own namespace.
		// Create one scoped to this test's dedicated namespace if credentials were provided --
		// never touch the real credentials in the certman-operator namespace. Best effort: a
		// missing secret only blocks the operator from completing the ACME DNS-01 challenge,
		// not from creating and maintaining the CertificateRequest itself, which is what this
		// suite verifies.
		accessKey, secretKey := os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY")

		// Fallback: CI injects credentials as files in the cluster profile directory rather
		// than as environment variables. Try reading them from CLUSTER_PROFILE_DIR if the
		// env vars are not set.
		if accessKey == "" || secretKey == "" {
			if profileDir := os.Getenv("CLUSTER_PROFILE_DIR"); profileDir != "" {
				if data, readErr := os.ReadFile(filepath.Join(profileDir, ".awscred")); readErr == nil {
					ak, sk := parseAWSCredentialFile(string(data))
					if ak != "" && sk != "" {
						accessKey, secretKey = ak, sk
						logger.Info("Loaded AWS credentials from cluster profile .awscred file")
					}
				}
				// Also try individual credential files (common CI layout)
				if accessKey == "" {
					if data, readErr := os.ReadFile(filepath.Join(profileDir, "aws_access_key_id")); readErr == nil {
						accessKey = strings.TrimSpace(string(data))
					}
				}
				if secretKey == "" {
					if data, readErr := os.ReadFile(filepath.Join(profileDir, "aws_secret_access_key")); readErr == nil {
						secretKey = strings.TrimSpace(string(data))
					}
				}
				if accessKey != "" && secretKey != "" {
					logger.Info("Loaded AWS credentials from cluster profile directory", "dir", profileDir)
				}
			}
		}

		if accessKey != "" && secretKey != "" {
			awsSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws",
					Namespace: certConfig.TestNamespace,
				},
				StringData: map[string]string{
					"aws_access_key_id":     accessKey,
					"aws_secret_access_key": secretKey,
				},
				Type: corev1.SecretTypeOpaque,
			}
			_, err = clientset.CoreV1().Secrets(certConfig.TestNamespace).Create(ctx, awsSecret, metav1.CreateOptions{})
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to create scoped AWS secret")
		} else {
			logger.Info("AWS credentials not found in env vars or cluster profile, skipping scoped AWS secret creation")
		}

		// If OPERATOR_IMAGE is set (ci-operator injects this for PR presubmits via the
		// `dependencies` mechanism), deploy that candidate image as its own namespace-scoped
		// instance in this run's dedicated namespace, and verify against it instead of the real
		// production instance -- this is what actually validates the PR's code change. In
		// promotion-gate mode (OPERATOR_IMAGE unset), SAPM has already deployed the candidate via
		// PKO to the real certman-operator namespace, so there's nothing to deploy here.
		if candidateImage := os.Getenv("OPERATOR_IMAGE"); candidateImage != "" {
			err = deployCandidateOperator(ctx, clientset, certConfig.TestNamespace, candidateImage)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to deploy candidate certman-operator")
			testOperatorNamespace = certConfig.TestNamespace
			logger.Info("Deployed candidate certman-operator for presubmit validation",
				"namespace", certConfig.TestNamespace)
		}

		logger.Info("Certman Operator Hive suite setup completed",
			"namespace", certConfig.TestNamespace,
			"clusterDeployment", clusterDeploymentName)
	})

	ginkgo.It("certman-operator pod is running and healthy", func(ctx context.Context) {
		gomega.Eventually(func() bool {
			pods, err := clientset.CoreV1().Pods(testOperatorNamespace).List(ctx, metav1.ListOptions{LabelSelector: "name=certman-operator"})
			if err != nil || len(pods.Items) == 0 {
				return false
			}
			pod := pods.Items[0]
			if pod.Status.Phase != corev1.PodRunning {
				return false
			}
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodReady {
					return cond.Status == corev1.ConditionTrue
				}
			}
			return false
		}, shortTimeout, 15*time.Second).Should(gomega.BeTrue(), "certman-operator pod should be running and ready before starting reconcile tests")
	})

	ginkgo.It("should create ClusterDeployment and have the operator create a CertificateRequest", func(ctx context.Context) {
		// Finalizer is intentionally left unset here -- the "should maintain finalizers" spec
		// below verifies the operator adds it, which would be a no-op check if we seeded it.
		clusterDeployment := utils.BuildCompleteClusterDeployment(certConfig, clusterDeploymentName, adminKubeconfigSecretName, ocmClusterID)

		_, err := dynamicClient.Resource(clusterDeploymentGVR).Namespace(certConfig.TestNamespace).Create(
			ctx, clusterDeployment, metav1.CreateOptions{})
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to create ClusterDeployment")

		gomega.Eventually(func() bool {
			return utils.VerifyClusterDeploymentCriteria(ctx, dynamicClient, clusterDeploymentGVR,
				certConfig.TestNamespace, clusterDeploymentName, ocmClusterID)
		}, shortTimeout, 10*time.Second).Should(gomega.BeTrue(), "ClusterDeployment should meet reconciliation criteria")

		var foundCertificateRequestName string
		gomega.Eventually(func() bool {
			cr, err := utils.FindCertificateRequestForClusterDeployment(ctx, dynamicClient, certificateRequestGVR,
				certConfig.TestNamespace, clusterDeploymentName)
			if err != nil {
				return false
			}
			foundCertificateRequestName = cr.GetName()
			return true
		}, pollingDuration, 15*time.Second).Should(gomega.BeTrue(), "Operator should create a CertificateRequest for the ClusterDeployment")

		logger.Info("CertificateRequest created by operator", "name", foundCertificateRequestName)

		// In candidate mode, the real production instance also watches this namespace (its own
		// WATCH_NAMESPACE is "" i.e. cluster-wide), so it could satisfy the assertions above even
		// if the candidate is completely broken. Assert on the candidate pod's own logs -- scoped
		// to its own WATCH_NAMESPACE, they can only mention this run's randomly-named
		// ClusterDeployment if the candidate itself reconciled it -- to verify the candidate is
		// actually alive and functioning, not just that some certman-operator did the work.
		if testOperatorNamespace != operatorNamespace {
			gomega.Eventually(func() bool {
				logs, err := getCandidatePodLogs(ctx, clientset, testOperatorNamespace)
				if err != nil {
					return false
				}
				return strings.Contains(logs, clusterDeploymentName)
			}, shortTimeout, 15*time.Second).Should(gomega.BeTrue(),
				"Candidate operator's own logs should show it reconciled this run's ClusterDeployment")
		}
	})

	ginkgo.It("should maintain finalizers and owner references on ClusterDeployment and CertificateRequest", func(ctx context.Context) {
		gomega.Eventually(func() bool {
			cd, err := dynamicClient.Resource(clusterDeploymentGVR).Namespace(certConfig.TestNamespace).Get(
				ctx, clusterDeploymentName, metav1.GetOptions{})
			if err != nil {
				return false
			}
			for _, f := range cd.GetFinalizers() {
				if f == "certificaterequests.certman.managed.openshift.io" {
					return true
				}
			}
			return false
		}, shortTimeout, 15*time.Second).Should(gomega.BeTrue(), "Operator should maintain the certman finalizer on ClusterDeployment")

		cr, err := utils.FindCertificateRequestForClusterDeployment(ctx, dynamicClient, certificateRequestGVR,
			certConfig.TestNamespace, clusterDeploymentName)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "CertificateRequest should exist for this test's ClusterDeployment")

		hasOwner := false
		for _, ref := range cr.GetOwnerReferences() {
			if ref.Kind == "ClusterDeployment" && ref.Name == clusterDeploymentName {
				hasOwner = true
				break
			}
		}
		gomega.Expect(hasOwner).To(gomega.BeTrue(), "CertificateRequest should be owned by its ClusterDeployment")
	})

	ginkgo.AfterAll(func(ctx context.Context) {
		logger.Info("Cleanup: removing this run's ClusterDeployment, CertificateRequests, and dedicated namespace",
			"namespace", certConfig.TestNamespace)

		// Clean up the cluster-scoped ClusterRoleBinding created by deployCandidateOperator
		// (namespaced resources are removed when the namespace is deleted below).
		crbName := fmt.Sprintf("certman-candidate-%s", certConfig.TestNamespace)
		err := clientset.RbacV1().ClusterRoleBindings().Delete(ctx, crbName, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			logger.Info("WARNING: failed to delete candidate ClusterRoleBinding", "name", crbName, "error", err)
		}

		// Clean up the Role and RoleBinding created in the production namespace for
		// metrics service access (deployCandidateOperator creates these).
		prodMetricsRoleName := fmt.Sprintf("certman-candidate-metrics-%s", certConfig.TestNamespace)
		err = clientset.RbacV1().RoleBindings(operatorNamespace).Delete(ctx, prodMetricsRoleName, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			logger.Info("WARNING: failed to delete candidate metrics RoleBinding", "name", prodMetricsRoleName, "error", err)
		}
		err = clientset.RbacV1().Roles(operatorNamespace).Delete(ctx, prodMetricsRoleName, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			logger.Info("WARNING: failed to delete candidate metrics Role", "name", prodMetricsRoleName, "error", err)
		}

		// Force-cleanup only objects in this run's own namespace -- never the shared
		// certman-operator namespace, CRDs, or credentials.
		utils.CleanupClusterDeployment(ctx, dynamicClient, clusterDeploymentGVR, certConfig.TestNamespace, clusterDeploymentName)
		utils.ForceDeleteCertificateRequests(ctx, dynamicClient, certConfig.TestNamespace)

		err = clientset.CoreV1().Namespaces().Delete(ctx, certConfig.TestNamespace, metav1.DeleteOptions{})
		if !apierrors.IsNotFound(err) {
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Failed to delete dedicated test namespace")
		}

		// Namespace deletion is asynchronous -- wait for it to actually finish terminating
		// rather than assuming it's gone once the delete call is accepted. Tolerate transient
		// Get errors (keep polling) but fail the suite if the namespace is still around after
		// maxWait, so a stuck cleanup is never silently reported as success.
		maxWait := 2 * time.Minute
		checkInterval := 5 * time.Second
		// Best-effort wait for namespace termination. A stuck namespace
		// should not fail the suite when the actual specs passed.
		for elapsed := time.Duration(0); elapsed < maxWait; elapsed += checkInterval {
			_, getErr := clientset.CoreV1().Namespaces().Get(ctx, certConfig.TestNamespace, metav1.GetOptions{})
			if apierrors.IsNotFound(getErr) {
				logger.Info("Dedicated test namespace fully deleted", "namespace", certConfig.TestNamespace)
				return
			}
			time.Sleep(checkInterval)
		}
		logger.Info("WARNING: test namespace still terminating after timeout, leaving for cluster cleanup",
			"namespace", certConfig.TestNamespace, "timeout", maxWait)
	})
})

// getCandidatePodLogs returns the current logs of the certman-operator pod in namespace. Used to
// verify the candidate itself processed a reconcile, since shared CR/CD state alone can't
// distinguish the candidate's work from the cluster-wide production instance's.
func getCandidatePodLogs(ctx context.Context, clientset *kubernetes.Clientset, namespace string) (string, error) {
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: "name=certman-operator"})
	if err != nil {
		return "", err
	}
	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no certman-operator pod found in namespace %s", namespace)
	}
	raw, err := clientset.CoreV1().Pods(namespace).GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{}).Do(ctx).Raw()
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// deployCandidateOperator deploys a namespace-scoped instance of certman-operator using the
// given (ci-operator-built, PR-candidate) image, isolated entirely to namespace. It is bound to
// the existing cluster-wide "certman-operator" ClusterRole via a ClusterRoleBinding, giving it
// the same cross-namespace access as the production operator (secrets in Hive-managed namespaces,
// configmaps in certman-operator and aws-account-operator namespaces). WATCH_NAMESPACE restricts
// its cache so the candidate never sees or reconciles real production ClusterDeployments.
// Namespaced resources are cleaned up by deleting namespace; the ClusterRoleBinding is
// cluster-scoped and must be deleted explicitly in AfterAll.
func deployCandidateOperator(ctx context.Context, clientset *kubernetes.Clientset, namespace, image string) error {
	const serviceAccountName = "certman-operator-candidate"
	const productionNamespace = "certman-operator"

	_, err := clientset.CoreV1().ServiceAccounts(namespace).Create(ctx, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: serviceAccountName, Namespace: namespace},
	}, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create candidate service account: %w", err)
	}

	// The candidate needs the same cluster-wide ClusterRole as the production operator
	// (secrets across Hive-managed namespaces, configmaps from certman-operator and
	// aws-account-operator namespaces). A ClusterRoleBinding is required -- a namespaced
	// RoleBinding to a ClusterRole would only grant access within that single namespace.
	clusterRoleBindingName := fmt.Sprintf("certman-candidate-%s", namespace)
	_, err = clientset.RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: clusterRoleBindingName},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "certman-operator",
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      serviceAccountName,
			Namespace: namespace,
		}},
	}, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create candidate cluster role binding: %w", err)
	}

	// The ClusterRole grants cross-namespace access (secrets, CRDs), but the production
	// operator also gets namespace-scoped Roles for services (metrics endpoint),
	// pods and configmaps (leader election) deployed by PKO into the certman-operator
	// namespace. The candidate runs in its own namespace, so it needs equivalent
	// grants there -- otherwise it crashloops on metrics.ConfigureMetrics().
	const localRoleName = "certman-operator-candidate-local"

	_, err = clientset.RbacV1().Roles(namespace).Create(ctx, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: localRoleName, Namespace: namespace},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"services"},
				Verbs:     []string{"get", "create", "update", "delete"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "delete"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"create", "get", "update", "delete"},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create candidate local role: %w", err)
	}

	_, err = clientset.RbacV1().RoleBindings(namespace).Create(ctx, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: localRoleName, Namespace: namespace},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     localRoleName,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      serviceAccountName,
			Namespace: namespace,
		}},
	}, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create candidate local role binding: %w", err)
	}

	// The candidate operator also needs to create a metrics Service in the production
	// namespace (certman-operator). The certman-operator ClusterRole does not include
	// services; the production instance gets this via a PKO-deployed Role. Grant the
	// candidate equivalent access. Best-effort: log and continue on failure so local/
	// non-CI runs aren't blocked.
	prodMetricsRoleName := fmt.Sprintf("certman-candidate-metrics-%s", namespace)
	_, err = clientset.RbacV1().Roles(productionNamespace).Create(ctx, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: prodMetricsRoleName, Namespace: productionNamespace},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"services"},
				Verbs:     []string{"create", "get", "update", "delete", "list", "watch"},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		log.Log.Info("WARNING: failed to create metrics Role in production namespace, candidate may fail to create metrics Service",
			"namespace", productionNamespace, "error", err)
	}

	_, err = clientset.RbacV1().RoleBindings(productionNamespace).Create(ctx, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: prodMetricsRoleName, Namespace: productionNamespace},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     prodMetricsRoleName,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      serviceAccountName,
			Namespace: namespace,
		}},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		log.Log.Info("WARNING: failed to create metrics RoleBinding in production namespace, candidate may fail to create metrics Service",
			"namespace", productionNamespace, "error", err)
	}

	// Copy the CI pull secret from the production namespace so the candidate pod can pull the
	// CI-built image. The rosa-hive-operator-install CI step creates ci-pull-secret in the
	// production namespace, but the candidate runs in an ephemeral test namespace where no pull
	// secret exists. The global pull secret update via MCO takes longer to propagate than the
	// deployment timeout, so the pod would fail to pull without this. Best-effort: if the
	// secret doesn't exist (e.g. running locally, not in CI), log a warning and continue.
	ciPullSecret, err := clientset.CoreV1().Secrets(productionNamespace).Get(ctx, "ci-pull-secret", metav1.GetOptions{})
	if err != nil {
		log.Log.Info("WARNING: ci-pull-secret not found in production namespace, continuing without it (image pull may rely on global pull secret)",
			"namespace", productionNamespace, "error", err)
	} else {
		copiedSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ci-pull-secret",
				Namespace: namespace,
			},
			Data: ciPullSecret.Data,
			Type: ciPullSecret.Type,
		}
		_, err = clientset.CoreV1().Secrets(namespace).Create(ctx, copiedSecret, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to copy ci-pull-secret to test namespace: %w", err)
		}
		log.Log.Info("Copied ci-pull-secret to test namespace for candidate image pull", "namespace", namespace)
	}

	replicas := int32(1)
	_, err = clientset.AppsV1().Deployments(namespace).Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "certman-operator", Namespace: namespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"name": "certman-operator"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"name": "certman-operator"}},
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccountName,
					ImagePullSecrets:   []corev1.LocalObjectReference{{Name: "ci-pull-secret"}},
					Containers: []corev1.Container{{
						Name:    "certman-operator",
						Image:   image,
						Command: []string{"certman-operator"},
						Env: []corev1.EnvVar{
							// Restricting WATCH_NAMESPACE to this run's own namespace is what
							// keeps the candidate's reconcile loop from ever touching real
							// production ClusterDeployments/CertificateRequests elsewhere on
							// the cluster.
							{Name: "WATCH_NAMESPACE", Value: namespace},
							{Name: "POD_NAME", ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"},
							}},
							{Name: "OPERATOR_NAME", Value: "certman-operator"},
							{Name: "OPERATOR_NAMESPACE", Value: namespace},
						},
					}},
				},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create candidate deployment: %w", err)
	}

	gomega.Eventually(func() bool {
		d, err := clientset.AppsV1().Deployments(namespace).Get(ctx, "certman-operator", metav1.GetOptions{})
		if err != nil {
			return false
		}
		return d.Status.AvailableReplicas >= 1
	}, 3*time.Minute, 10*time.Second).Should(gomega.BeTrue(), "Candidate certman-operator deployment should become available")

	return nil
}

// parseAWSCredentialFile extracts aws_access_key_id and aws_secret_access_key from an INI-style
// AWS credentials file (e.g. .awscred in the cluster profile). Returns empty strings if the
// expected keys are not found.
func parseAWSCredentialFile(content string) (accessKey, secretKey string) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") || line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case "aws_access_key_id":
			accessKey = value
		case "aws_secret_access_key":
			secretKey = value
		}
	}
	return accessKey, secretKey
}
