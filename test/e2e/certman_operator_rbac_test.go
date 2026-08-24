// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS.
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// "Certman Operator RBAC" validates that the RBAC grants shipped in deploy/ and deploy_pko/ are
// sufficient for the operator to run without crashlooping. Specifically, it checks that the
// certman-operator Deployment has ready replicas (proving it didn't crashloop on missing
// permissions) and that the metrics Service exists (proving the operator had services RBAC to
// create it). This suite would have caught the PR #505 regression that stripped services,
// pods, and configmaps permissions from the ClusterRole.
var _ = ginkgo.Describe("Certman Operator RBAC", ginkgo.Ordered, func() {
	var (
		clientset *kubernetes.Clientset
	)

	const (
		operatorNamespace = "certman-operator"
		operatorName      = "certman-operator"
		shortTimeout      = 2 * time.Minute
	)

	ginkgo.BeforeAll(func() {
		k8s, err := openshift.New(ginkgo.GinkgoLogr)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Unable to setup k8s client")

		clientset, err = kubernetes.NewForConfig(k8s.GetConfig())
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Unable to setup clientset")
	})

	ginkgo.It("certman-operator deployment exists and has ready replicas", func(ctx context.Context) {
		gomega.Eventually(func() bool {
			deploy, err := clientset.AppsV1().Deployments(operatorNamespace).Get(
				ctx, operatorName, metav1.GetOptions{})
			if err != nil {
				return false
			}
			return deploy.Status.ReadyReplicas > 0 &&
				deploy.Status.UnavailableReplicas == 0 &&
				deploymentNotCrashlooping(deploy)
		}, shortTimeout, 10*time.Second).Should(gomega.BeTrue(),
			"certman-operator deployment should have ready replicas and not be crashlooping "+
				"(a missing RBAC grant such as services would cause a crashloop here)")
	})

	ginkgo.It("metrics Service exists in the certman-operator namespace", func(ctx context.Context) {
		gomega.Eventually(func() error {
			_, err := clientset.CoreV1().Services(operatorNamespace).Get(
				ctx, operatorName, metav1.GetOptions{})
			return err
		}, shortTimeout, 10*time.Second).Should(gomega.Succeed(),
			"metrics Service should exist in certman-operator namespace "+
				"(created by metrics.ConfigureMetrics at main.go:234, requires services RBAC)")
	})
})

// deploymentNotCrashlooping returns true if the Deployment has an Available condition set to
// True, indicating the operator started successfully and is not stuck in a crash loop due to
// missing permissions.
func deploymentNotCrashlooping(deploy *appsv1.Deployment) bool {
	for _, cond := range deploy.Status.Conditions {
		if cond.Type == appsv1.DeploymentAvailable {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}
