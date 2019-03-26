package certificaterequest

import (
	"context"

	certmanv1alpha1 "github.com/certman-operator/pkg/apis/certman/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_certificaterequest")

// Add creates a new CertificateRequest Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileCertificateRequest{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("certificaterequest-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource CertificateRequest
	err = c.Watch(&source.Kind{Type: &certmanv1alpha1.CertificateRequest{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner CertificateRequest
	err = c.Watch(&source.Kind{Type: &certmanv1alpha1.CertificateRequest{}}, &handler.EnqueueRequestForObject{})

	//	IsController: true,
	//	OwnerType:    &certmanv1alpha1.CertificateRequest{},
	//})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileCertificateRequest{}

// ReconcileCertificateRequest reconciles a CertificateRequest object
type ReconcileCertificateRequest struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a CertificateRequest object and makes changes based on the state read
// and what is in the CertificateRequest.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCertificateRequest) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling CertificateRequest")

	// Fetch the CertificateRequest cr
	cr := &certmanv1alpha1.CertificateRequest{}

	err := r.client.Get(context.TODO(), request.NamespacedName, cr)

	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Define a new Pod object
	//pod := newPodForCR(cr)
	certificateSecret := newSecretForCR(cr)

	// Set CertificateRequest cr as the owner and controller
	if err := controllerutil.SetControllerReference(cr, certificateSecret, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Check if this Pod already exists
	found := &corev1.Secret{}

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: certificateSecret.Name, Namespace: certificateSecret.Namespace}, found)

	if err != nil && errors.IsNotFound(err) {

		reqLogger.Info("Creating a new Secret", "Secret.Namespace", certificateSecret.Namespace, "Secret.Name", certificateSecret.Name)

		err = r.client.Create(context.TODO(), certificateSecret)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Pod created successfully - don't requeue
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, err
	}

	// Pod already exists - don't requeue
	reqLogger.Info("Skip reconcile: Secret already exists", "Secret.Namespace", found.Namespace, "Secret.Name", found.Name, "Key", string(found.Data["some-key"]))

	return reconcile.Result{}, nil
}

// newPodForCR returns a busybox pod with the same name/namespace as the cr
//func newPodForCR(cr *certmanv1alpha1.CertificateRequest) *corev1.Pod {
//	labels := map[string]string{
//		"app": cr.Name,
//	}
//	return &corev1.Pod{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      cr.Name + "-pod",
//			Namespace: cr.Namespace,
//			Labels:    labels,
//		},
//		Spec: corev1.PodSpec{
//			Containers: []corev1.Container{
//				{
//					Name:    "busybox",
//					Image:   "busybox",
//					Command: []string{"sleep", "3600"},
//				},
//			},
//		},
//	}
//}

func newSecretForCR(cr *certmanv1alpha1.CertificateRequest) *corev1.Secret {

	return &corev1.Secret{
		Type: corev1.SecretTypeOpaque,
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.CertificateSecret.Name,
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"certificate_request": cr.Name,
			},
		},
		Data: map[string][]byte{
			"some-key": []byte("data"),
		},
	}

}
