package clusterdeployment

import (
	"context"
	"fmt"
	"reflect"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"

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

	logr "github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_clusterdeployment")

const (
	controllerName = "clusterdeployment"
)

// Add creates a new ClusterDeployment Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileClusterDeployment{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName+"-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1alpha1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterDeployment{}

// ReconcileClusterDeployment reconciles a ClusterDeployment object
type ReconcileClusterDeployment struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a ClusterDeployment object and sets up
// any needed CertificateRequest objects.
func (r *ReconcileClusterDeployment) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ClusterDeployment")

	// Fetch the ClusterDeployment instance
	cd := &hivev1alpha1.ClusterDeployment{}
	err := r.client.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Error(err, "error looking up clusterDeployment")
		return reconcile.Result{}, err
	}

	// Do not make certificate request if the cluster is not a Red Hat managed cluster.
	if val, ok := cd.Labels["api.openshift.com/managed"]; ok {
		if val != "true" {
			reqLogger.Info("Not a managed cluster", "Namespace", request.Namespace, "Name", request.Name)
			return reconcile.Result{}, nil
		}
	} else {
		// Managed tag is not present which implies it is not a managed cluster
		reqLogger.Info("Not a managed cluster", "Namespace", request.Namespace, "Name", request.Name)
		return reconcile.Result{}, nil
	}

	if err := r.syncCertificateRequests(cd, reqLogger); err != nil {
		reqLogger.Error(err, "error syncing CertificateRequests")
		return reconcile.Result{}, err
	}

	reqLogger.Info("done syncing")
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterDeployment) syncCertificateRequests(cd *hivev1alpha1.ClusterDeployment, logger logr.Logger) error {
	desiredCRs := []certmanv1alpha1.CertificateRequest{}

	// get a list of current CertificateRequests
	currentCRs, err := r.getCurrentCertificateRequests(cd, logger)
	if err != nil {
		return err
	}

	// for each certbundle with generate==true make a CertificateRequest
	for _, cb := range cd.Spec.CertificateBundles {
		if cb.Generate == true {
			domains := getDomainsForCertBundle(cb, cd)

			certReq := createCertificateRequest(cb.Name, domains, cd)
			desiredCRs = append(desiredCRs, certReq)
		}
	}

	deleteCRs := []certmanv1alpha1.CertificateRequest{}

	// find any extra certificateRequests and mark them for deletion
	for i, currentCR := range currentCRs {
		found := false
		for _, desiredCR := range desiredCRs {
			if desiredCR.Name == currentCR.Name {
				found = true
				break
			}
		}
		if !found {
			deleteCRs = append(deleteCRs, currentCRs[i])
		}
	}

	// create/update the desired certificaterequests
	for _, desiredCR := range desiredCRs {
		currentCR := &certmanv1alpha1.CertificateRequest{}
		searchKey := types.NamespacedName{Name: desiredCR.Name, Namespace: desiredCR.Namespace}

		if err := r.client.Get(context.TODO(), searchKey, currentCR); err != nil {
			if errors.IsNotFound(err) {
				// create
				if err := controllerutil.SetControllerReference(cd, &desiredCR, r.scheme); err != nil {
					logger.Error(err, "error setting owner reference", "certrequest", desiredCR.Name)
					return err
				}

				if err := r.client.Create(context.TODO(), &desiredCR); err != nil {
					logger.Error(err, "error creating certificaterequest")
					return err
				}
			} else {
				logger.Error(err, "error checking for existing certificaterequest")
				return err
			}
		} else {
			// update or no update needed
			if !reflect.DeepEqual(currentCR.Spec, desiredCR.Spec) {
				if err := r.client.Update(context.TODO(), currentCR); err != nil {
					logger.Error(err, "error updating certificaterequest", "certrequest", currentCR.Name)
					return err
				}
			} else {
				logger.Info("no update needed for certificaterequest", "certrequest", desiredCR.Name)
			}
		}
	}

	// delete the extra certificaterequests
	for _, deleteCR := range deleteCRs {
		if err := r.client.Delete(context.TODO(), &deleteCR); err != nil {
			logger.Error(err, "error deleting CertificateRequest that is no longer needed", "certrequest", deleteCR.Name)
			return err
		}
	}

	return nil
}

func (r *ReconcileClusterDeployment) getCurrentCertificateRequests(cd *hivev1alpha1.ClusterDeployment, logger logr.Logger) ([]certmanv1alpha1.CertificateRequest, error) {
	certReqsForCluster := []certmanv1alpha1.CertificateRequest{}

	// get all CRs in the cluster's namespace
	currentCRs := &certmanv1alpha1.CertificateRequestList{}
	if err := r.client.List(context.TODO(), &client.ListOptions{Namespace: cd.Namespace}, currentCRs); err != nil {
		logger.Error(err, "error listing current CertificateRequests")
		return certReqsForCluster, err
	}

	// now filter out the ones that are owned by the cluster we're processing
	for i, cr := range currentCRs.Items {
		if metav1.IsControlledBy(&cr, cd) {
			certReqsForCluster = append(certReqsForCluster, currentCRs.Items[i])
		}
	}

	return certReqsForCluster, nil
}

func getDomainsForCertBundle(cb hivev1alpha1.CertificateBundleSpec, cd *hivev1alpha1.ClusterDeployment) []string {
	domains := []string{}

	// first check for the special-case default control plane reference
	if cd.Spec.ControlPlaneConfig.ServingCertificates.Default == cb.Name {
		domains = append(domains, fmt.Sprintf("api.%s.%s", cd.Spec.ClusterName, cd.Spec.BaseDomain))
	}

	// now check the rest of the control plane
	for _, additionalCert := range cd.Spec.ControlPlaneConfig.ServingCertificates.Additional {
		if additionalCert.Name == cb.Name {
			domains = append(domains, additionalCert.Domain)
		}
	}

	// and lastly the ingress list
	for _, ingress := range cd.Spec.Ingress {
		if ingress.ServingCertificate == cb.Name {
			domains = append(domains, ingress.Domain)
		}
	}

	return domains
}

func createCertificateRequest(certBundleName string, domains []string, cd *hivev1alpha1.ClusterDeployment) certmanv1alpha1.CertificateRequest {
	name := fmt.Sprintf("%s-%s", cd.Name, certBundleName)

	cr := certmanv1alpha1.CertificateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cd.Namespace,
		},
		Spec: certmanv1alpha1.CertificateRequestSpec{
			ACMEDNSDomain: cd.Spec.BaseDomain,
			CertificateSecret: corev1.ObjectReference{
				Kind:      "secret",
				Namespace: cd.Namespace,
				Name:      name,
			},
			PlatformSecrets: certmanv1alpha1.PlatformSecrets{
				AWS: &certmanv1alpha1.AWSPlatformSecrets{
					Credentials: corev1.LocalObjectReference{
						Name: cd.Spec.PlatformSecrets.AWS.Credentials.Name,
					},
				},
			},
			DnsNames: domains,
		},
	}

	return cr
}
