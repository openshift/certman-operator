/*
Copyright 2019 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package certificaterequest

import (
	"context"
	gerrors "errors"

	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	certmanv1alpha1 "github.com/openshift/certman-operator/api/v1alpha1"
	"github.com/openshift/certman-operator/controllers/utils"
	cClient "github.com/openshift/certman-operator/pkg/clients"
	"github.com/openshift/certman-operator/pkg/leclient"
	"github.com/openshift/certman-operator/pkg/localmetrics"
)

const (
	controllerName                        = "controller_certificaterequest"
	maxConcurrentReconciles               = 10
	hiveRelocationAnnotation              = "hive.openshift.io/relocate"
	hiveRelocationOutgoingValue           = "outgoing"
	hiveRelocationCertificateRequstStatus = "Not reconciling: ClusterDeployment is relocating"
	fedrampEnvVariable                    = "FEDRAMP"
	fedrampHostedZoneIDVariable           = "HOSTED_ZONE_ID"
	clusterDeploymentType                 = "ClusterDeployment"
)

var fedramp = os.Getenv(fedrampEnvVariable) == "true"
var fedrampHostedZoneID = os.Getenv(fedrampHostedZoneIDVariable)
var log = logf.Log.WithName(controllerName)

var _ reconcile.Reconciler = &CertificateRequestReconciler{}

// CertificateRequestReconciler reconciles a CertificateRequest object
type CertificateRequestReconciler struct {
	Client        client.Client
	Scheme        *runtime.Scheme
	ClientBuilder func(reqLogger logr.Logger, kubeClient client.Client, platfromSecret certmanv1alpha1.Platform, namespace string, clusterDeploymentName string) (cClient.Client, error)
}

// Reconcile reads that state of the cluster for a CertificateRequest object and makes changes based on the state read
// and what is in the CertificateRequest.Spec
func (r *CertificateRequestReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	reqLogger.Info("reconciling CertificateRequest")
	envvar, present := os.LookupEnv(fedrampEnvVariable)
	if len(envvar) == 0 || !present {
		reqLogger.Info("FEDRAMP environment variable unset, defaulting to false")
	} else {
		reqLogger.Info(fmt.Sprintf("running in FedRAMP environment: %t", fedramp))
	}

	if fedramp {
		envvar, present = os.LookupEnv(fedrampHostedZoneIDVariable)
		if len(envvar) == 0 || !present {
			err := gerrors.New("HOSTED_ZONE_ID environment variable is unset but is required in FedRAMP environment")
			reqLogger.Error(err, "HOSTED_ZONE_ID environment variable is unset but is required in FedRAMP environment")
			return reconcile.Result{}, nil
		}
		reqLogger.Info(fmt.Sprintf("running in FedRAMP zone: %s", fedrampHostedZoneID))
	}

	timer := prometheus.NewTimer(localmetrics.MetricCertificateRequestReconcileDuration)
	defer func() {
		reconcileDuration := timer.ObserveDuration()
		reqLogger.WithValues("Duration", reconcileDuration).Info("Reconcile complete.")
	}()

	// Init the certificate request counter if nor already done
	localmetrics.CheckInitCounter(r.Client)

	// Fetch the CertificateRequest cr
	cr := &certmanv1alpha1.CertificateRequest{}

	err := r.Client.Get(context.TODO(), request.NamespacedName, cr)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Update metrics to show it's missing and set CertValidDuration to 0
			localmetrics.UpdateMissingCertificates(request.Namespace, request.Name)
			localmetrics.UpdateCertificateRetrievalErrors(request.Namespace, request.Name)
			localmetrics.UpdateCertValidDuration(nil, time.Now(), request.Name)
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		localmetrics.UpdateCertificateRetrievalErrors(request.Namespace, request.Name)
		localmetrics.UpdateCertValidDuration(nil, time.Now(), request.Name)
		reqLogger.Error(err, err.Error())
		return reconcile.Result{}, err
	}

	// Initialize metrics for this CertificateRequest
	localmetrics.UpdateMissingCertificates(cr.Namespace, cr.Name)
	localmetrics.UpdateCertificateRetrievalErrors(cr.Namespace, cr.Name)

	// Handle the presence of a deletion timestamp.
	if !cr.DeletionTimestamp.IsZero() {
		// Set CertValidDuration to 0 for certificates being deleted
		localmetrics.UpdateCertValidDuration(nil, time.Now(), cr.Name)
		return r.finalizeCertificateRequest(reqLogger, cr)
	}

	// Add finalizer if not exists
	if !utils.ContainsString(cr.ObjectMeta.Finalizers, certmanv1alpha1.CertmanOperatorFinalizerLabel) {
		reqLogger.Info("adding finalizer to the certificate request")
		localmetrics.IncrementCertRequestsCounter()
		baseToPatch := client.MergeFrom(cr.DeepCopy())
		cr.ObjectMeta.Finalizers = append(cr.ObjectMeta.Finalizers, certmanv1alpha1.CertmanOperatorFinalizerLabel)
		if err := r.Client.Patch(context.TODO(), cr, baseToPatch); err != nil {
			reqLogger.Error(err, err.Error())
			return reconcile.Result{}, err
		}
	}

	// just in case something else ever adds itself as an owner of the
	// certificaterequest, loop through the owner references to find which one is
	// the clusterdeployment
	clusterDeploymentName := ""

	for _, o := range cr.ObjectMeta.OwnerReferences {
		if o.Kind == clusterDeploymentType {
			clusterDeploymentName = o.Name
		}
	}
	if clusterDeploymentName == "" {
		// assume there's only one clusterdeployment in a namespace and that it's the owner of this certificaterequest
		// we have to assume this so that if/when a CertificateRequest loses its OwnerReferences, it can still reconcile
		cdList := &hivev1.ClusterDeploymentList{}
		err = r.Client.List(context.TODO(), cdList)
		if err != nil {
			reqLogger.Error(err, err.Error())
			return reconcile.Result{}, err
		}

		// if we still can't find a clusterdeployment, throw an error
		if len(cdList.Items) == 0 {
			err = gerrors.New("ClusterDeployment not found")
			reqLogger.Error(err, "ClusterDeployment not found")
			return reconcile.Result{}, err
		}

		clusterDeploymentName = cdList.Items[0].Name
	}

	cd := &hivev1.ClusterDeployment{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Namespace: request.Namespace, Name: clusterDeploymentName}, cd)
	if err != nil {
		reqLogger.Error(err, err.Error())
		return reconcile.Result{}, err
	}

	// if the ownerreference isn't there, add it
	if len(cr.OwnerReferences) == 0 {
		baseToPatch := client.MergeFrom(cr.DeepCopy())
		missingOwnerReference := metav1.OwnerReference{
			APIVersion:         fmt.Sprintf("%s/%s", hivev1.HiveAPIGroup, hivev1.HiveAPIVersion),
			Kind:               "ClusterDeployment",
			Name:               cd.Name,
			UID:                cd.UID,
			Controller:         boolPointer(true),
			BlockOwnerDeletion: boolPointer(true),
		}
		cr.OwnerReferences = []metav1.OwnerReference{missingOwnerReference}

		reqLogger.WithValues("CertificateRequest.Name", cr.Name, "OwnerReference.Name", missingOwnerReference.Name).Info("adding OwnerReference to CertificateRequest")
		if err := r.Client.Patch(context.TODO(), cr, baseToPatch); err != nil {
			reqLogger.Error(err, err.Error())
			return reconcile.Result{}, err
		}
	}

	// fetch the clusterdeployment and bail out if there's an outgoing migration annotation
	relocating, err := relocationBailOut(r.Client, types.NamespacedName{Namespace: request.Namespace, Name: cd.Name})
	if err != nil {
		if !errors.IsNotFound(err) {
			// If the ClusterDeployment was deleted by some other means, then we should just proceed anyways (we could be deleting this object)
			// Otherwise raise an error and requeue.
			reqLogger.Error(err, err.Error())
			return reconcile.Result{}, err
		}
	}

	if relocating {
		reqLogger.Info("Not reconciling, clusterdeployment is relocating")

		cr.Status.Status = hiveRelocationCertificateRequstStatus
		err = r.Client.Update(context.TODO(), cr)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil
	}

	found := &corev1.Secret{}

	leClient, err := leclient.NewClient(r.Client)
	if err != nil {
		reqLogger.Error(err, "failed to get letsencrypt client")
		return reconcile.Result{}, err
	}

	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: cr.Spec.CertificateSecret.Name, Namespace: cr.Namespace}, found)

	// Issue new certificates if the secret does not already exist
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("requesting new certificates as secret was not found")
			return r.createCertificateSecret(reqLogger, cr, leClient)
		}

		reqLogger.Error(err, err.Error())
		return reconcile.Result{}, err
	}

	reqLogger.Info("checking if certificates need to be reissued")

	// Reissue Certificates
	shouldReissue, err := r.ShouldReissue(reqLogger, cr)
	if err != nil {
		reqLogger.Error(err, err.Error())
		return reconcile.Result{}, err
	}

	// fetch the clusterdeployment and bail out if there's an outgoing migration annotation again
	relocating, err = relocationBailOut(r.Client, types.NamespacedName{Namespace: request.Namespace, Name: clusterDeploymentName})
	if err != nil {
		return reconcile.Result{}, err
	}
	if relocating {
		reqLogger.Info("Not reconciling, clusterdeployment is relocating")

		cr.Status.Status = hiveRelocationCertificateRequstStatus
		err = r.Client.Update(context.TODO(), cr)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil
	}

	if shouldReissue {
		err := r.IssueCertificate(reqLogger, cr, found, leClient)
		if err != nil {
			return reconcile.Result{}, err
		}

		localmetrics.AddCertificateIssuance("renewal")
		err = r.Client.Update(context.TODO(), found)
		if err != nil {
			return reconcile.Result{}, err
		}

		err = r.updateStatus(reqLogger, cr)
		if err != nil {
			reqLogger.Error(err, err.Error())
		}

		reqLogger.Info("certificate has been reissued.")
		return reconcile.Result{}, nil
	}
	err = r.updateStatus(reqLogger, cr)
	if err != nil {
		reqLogger.Error(err, "Failed to update CertificateRequest status")
		localmetrics.UpdateCertificateRetrievalErrors(cr.Namespace, cr.Name)
		// Set CertValidDuration to 0 if we couldn't update the status
		localmetrics.UpdateCertValidDuration(nil, time.Now(), cr.Name)
	}
	// reqLogger.Info("Skip reconcile as valid certificates exist", "Secret.Namespace", found.Namespace, "Secret.Name", found.Name)
	return reconcile.Result{}, nil
}

// newSecret returns secret assigned to the secret name that is passed as the
// certificaterequest argument.
func newSecret(cr *certmanv1alpha1.CertificateRequest) *corev1.Secret {
	return &corev1.Secret{
		Type: corev1.SecretTypeTLS,
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.CertificateSecret.Name,
			Namespace: cr.Namespace,
		},
	}
}

// getClient returns cloud specific client to the caller
func (r *CertificateRequestReconciler) getClient(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) (cClient.Client, error) {
	clusterDeploymentName := ""
	for _, ownerRef := range cr.OwnerReferences {
		if ownerRef.Kind == "ClusterDeployment" {
			clusterDeploymentName = ownerRef.Name
		}
	}
	client, err := r.ClientBuilder(reqLogger, r.Client, cr.Spec.Platform, cr.Namespace, clusterDeploymentName)
	return client, err
}

// Helper function for Reconcile handles CertificateRequests with a deletion timestamp by
// revoking the certificate and removing the finalizer if it exists.
func (r *CertificateRequestReconciler) finalizeCertificateRequest(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) (reconcile.Result, error) {
	if utils.ContainsString(cr.ObjectMeta.Finalizers, certmanv1alpha1.CertmanOperatorFinalizerLabel) {
		reqLogger.Info("revoking certificate and deleting secret")
		if err := r.revokeCertificateAndDeleteSecret(reqLogger, cr); err != nil {
			reqLogger.Error(err, err.Error())
			return reconcile.Result{}, err
		}

		reqLogger.Info("removing finalizers")
		baseToPatch := client.MergeFrom(cr.DeepCopy())
		cr.ObjectMeta.Finalizers = utils.RemoveString(cr.ObjectMeta.Finalizers, certmanv1alpha1.CertmanOperatorFinalizerLabel)
		if err := r.Client.Patch(context.TODO(), cr, baseToPatch); err != nil {
			reqLogger.Error(err, err.Error())
			return reconcile.Result{}, err
		}
	}

	localmetrics.DecrementCertRequestsCounter()
	reqLogger.Info("certificaterequest has been deleted")
	return reconcile.Result{}, nil
}

// Helper function for Reconcile creates a Secret object containing a newly issued certificate.
func (r *CertificateRequestReconciler) createCertificateSecret(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest, leClient leclient.LetsEncryptClientInterface) (reconcile.Result, error) {
	certificateSecret := newSecret(cr)

	// Set CertificateRequest cr as the owner and controller
	if err := controllerutil.SetControllerReference(cr, certificateSecret, r.Scheme); err != nil {
		reqLogger.Error(err, err.Error())
		return reconcile.Result{}, err
	}

	err := r.IssueCertificate(reqLogger, cr, certificateSecret, leClient)
	if err != nil {
		updateErr := r.updateStatusError(reqLogger, cr, err)
		if updateErr != nil {
			reqLogger.Error(updateErr, updateErr.Error())
		}
		reqLogger.Error(err, err.Error())
		return reconcile.Result{}, err
	}

	reqLogger.Info("creating secret with certificates")
	localmetrics.AddCertificateIssuance("create")

	err = r.Client.Create(context.TODO(), certificateSecret)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			reqLogger.Info("secret already exists. will update the existing secret with new certificates")
			err = r.Client.Update(context.TODO(), certificateSecret)
			if err != nil {
				reqLogger.Error(err, err.Error())
				return reconcile.Result{}, err
			}
		} else {
			reqLogger.Error(err, err.Error())
			return reconcile.Result{}, err
		}
	}

	reqLogger.Info("updating certificate request status")
	err = r.updateStatus(reqLogger, cr)
	if err != nil {
		reqLogger.Error(err, "could not update the status of the CertificateRequest")
		localmetrics.UpdateCertificateRetrievalErrors(cr.Namespace, cr.Name)
		return reconcile.Result{}, err
	}

	reqLogger.Info(fmt.Sprintf("certificates issued and stored in secret %s/%s", certificateSecret.Namespace, certificateSecret.Name))
	return reconcile.Result{}, nil
}

// revokeCertificateAndDeleteSecret revokes certificate if it exists
func (r *CertificateRequestReconciler) revokeCertificateAndDeleteSecret(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) error {
	//todo - actually delete secret when revoking

	exists, err := SecretExists(r.Client, cr.Spec.CertificateSecret.Name, cr.Namespace)
	if err != nil {
		return fmt.Errorf("error checking if secret exists: %w", err)
	}
	if !exists {
		reqLogger.Info("Secret does not exist")
	}

	error := r.RevokeCertificate(reqLogger, cr)
	if error != nil {
		// TODO: handle error from certificate missing
		return fmt.Errorf("error revoking certificate: %w", error)
	}

	reqLogger.Info("Certificate successfully revoked")
	return nil

}

// relocationBailOut checks to see if there's a cluster relocation in progress
func relocationBailOut(k client.Client, nsn types.NamespacedName) (relocating bool, err error) {
	relocating = false

	cd := &hivev1.ClusterDeployment{}
	err = k.Get(context.TODO(), nsn, cd)
	if err != nil {
		return
	}

	// bail out of the loop if there's an outgoing relocation annotation
	for a, v := range cd.Annotations {
		if a == hiveRelocationAnnotation && strings.Split(v, "/")[1] == hiveRelocationOutgoingValue {
			relocating = true
		}
	}

	return
}

// SetupWithManager sets up the controller with the Manager.
func (r *CertificateRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&certmanv1alpha1.CertificateRequest{}).
		Owns(&corev1.Secret{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: maxConcurrentReconciles,
			Reconciler:              r,
			RateLimiter:             workqueue.NewItemExponentialFailureRateLimiter(1*time.Second, 30*time.Second),
		}).
		Complete(nil)
}
