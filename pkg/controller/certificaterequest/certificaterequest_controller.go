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
	"fmt"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	cClient "github.com/openshift/certman-operator/pkg/clients"
	"github.com/openshift/certman-operator/pkg/controller/utils"
	"github.com/openshift/certman-operator/pkg/localmetrics"
)

const (
	controllerName          = "controller_certificaterequest"
	maxConcurrentReconciles = 10
)

var log = logf.Log.WithName(controllerName)

// Add creates a new CertificateRequest Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler.
func newReconciler(mgr manager.Manager) reconcile.Reconciler {

	return &ReconcileCertificateRequest{
		client:        mgr.GetClient(),
		scheme:        mgr.GetScheme(),
		clientBuilder: cClient.NewClient,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {

	o := controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: maxConcurrentReconciles,
	}

	c, err := controller.New("certificaterequest-controller", mgr, o)
	if err != nil {
		return err
	}

	// Watch for changes to primary resource CertificateRequest
	err = c.Watch(&source.Kind{Type: &certmanv1alpha1.CertificateRequest{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &certmanv1alpha1.CertificateRequest{},
	})

	if err != nil {
		return err
	}
	return nil
}

var _ reconcile.Reconciler = &ReconcileCertificateRequest{}

// ReconcileCertificateRequest reconciles a CertificateRequest object
type ReconcileCertificateRequest struct {
	client        client.Client
	scheme        *runtime.Scheme
	clientBuilder func(kubeClient client.Client, platfromSecret certmanv1alpha1.Platform, namespace string) (cClient.Client, error)
}

// Reconcile reads that state of the cluster for a CertificateRequest object and makes changes based on the state read
// and what is in the CertificateRequest.Spec
func (r *ReconcileCertificateRequest) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	reqLogger.Info("reconciling CertificateRequest")

	timer := prometheus.NewTimer(localmetrics.MetricCertificateRequestReconcileDuration)
	defer func() {
		reconcileDuration := timer.ObserveDuration()
		reqLogger.WithValues("Duration", reconcileDuration).Info("Reconcile complete.")
	}()

	// Init the certificate request counter if nor already done
	localmetrics.CheckInitCounter(r.client)

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
		reqLogger.Error(err, err.Error())
		return reconcile.Result{}, err
	}

	// Handle the presence of a deletion timestamp.
	if !cr.DeletionTimestamp.IsZero() {
		return r.finalizeCertificateRequest(reqLogger, cr)
	}

	// Add finalizer if not exists
	if !utils.ContainsString(cr.ObjectMeta.Finalizers, certmanv1alpha1.CertmanOperatorFinalizerLabel) {
		reqLogger.Info("adding finalizer to the certificate request")
		localmetrics.IncrementCertRequestsCounter()
		cr.ObjectMeta.Finalizers = append(cr.ObjectMeta.Finalizers, certmanv1alpha1.CertmanOperatorFinalizerLabel)
		if err := r.client.Update(context.TODO(), cr); err != nil {
			reqLogger.Error(err, err.Error())
			return reconcile.Result{}, err
		}
	}

	found := &corev1.Secret{}

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Spec.CertificateSecret.Name, Namespace: cr.Namespace}, found)

	// Issue new certificates if the secret does not already exist
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("requesting new certificates as secret was not found")
			return r.createCertificateSecret(reqLogger, cr)
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

	if shouldReissue {
		err := r.IssueCertificate(reqLogger, cr, found)
		if err != nil {
			return reconcile.Result{}, err
		}

		localmetrics.AddCertificateIssuance("renewal")
		err = r.client.Update(context.TODO(), found)
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
		reqLogger.Error(err, err.Error())
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
func (r *ReconcileCertificateRequest) getClient(cr *certmanv1alpha1.CertificateRequest) (cClient.Client, error) {
	client, err := r.clientBuilder(r.client, cr.Spec.Platform, cr.Namespace)
	return client, err
}

// Helper function for Reconcile handles CertificateRequests with a deletion timestamp by
// revoking the certificate and removing the finalizer if it exists.
func (r *ReconcileCertificateRequest) finalizeCertificateRequest(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) (reconcile.Result, error) {
	if utils.ContainsString(cr.ObjectMeta.Finalizers, certmanv1alpha1.CertmanOperatorFinalizerLabel) {
		reqLogger.Info("revoking certificate and deleting secret")
		if err := r.revokeCertificateAndDeleteSecret(reqLogger, cr); err != nil {
			reqLogger.Error(err, err.Error())
			return reconcile.Result{}, err
		}

		reqLogger.Info("removing finalizers")
		cr.ObjectMeta.Finalizers = utils.RemoveString(cr.ObjectMeta.Finalizers, certmanv1alpha1.CertmanOperatorFinalizerLabel)
		if err := r.client.Update(context.TODO(), cr); err != nil {
			reqLogger.Error(err, err.Error())
			return reconcile.Result{}, err
		}
	}
	
	localmetrics.DecrementCertRequestsCounter()
	reqLogger.Info("certificaterequest has been deleted")
	return reconcile.Result{}, nil
}

// Helper function for Reconcile creates a Secret object containing a newly issued certificate.
func (r *ReconcileCertificateRequest) createCertificateSecret(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) (reconcile.Result, error) {
	certificateSecret := newSecret(cr)

	// Set CertificateRequest cr as the owner and controller
	if err := controllerutil.SetControllerReference(cr, certificateSecret, r.scheme); err != nil {
		reqLogger.Error(err, err.Error())
		return reconcile.Result{}, err
	}

	err := r.IssueCertificate(reqLogger, cr, certificateSecret)
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

	err = r.client.Create(context.TODO(), certificateSecret)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			reqLogger.Info("secret already exists. will update the existing secret with new certificates")
			err = r.client.Update(context.TODO(), certificateSecret)
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
	}

	reqLogger.Info(fmt.Sprintf("certificates issued and stored in secret %s/%s", certificateSecret.Namespace, certificateSecret.Name))
	return reconcile.Result{}, nil
}

// revokeCertificateAndDeleteSecret revokes certificate if it exists
func (r *ReconcileCertificateRequest) revokeCertificateAndDeleteSecret(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) error {
	//todo - actually delete secret when revoking

	if SecretExists(r.client, cr.Spec.CertificateSecret.Name, cr.Namespace) {
		err := r.RevokeCertificate(reqLogger, cr)
		if err != nil {
			return err //todo - handle error from certificate missing
		}
	}
	return nil
}
