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

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	"github.com/openshift/certman-operator/pkg/awsclient"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	controllerName = "controller_certificaterequest"
	finalizerName  = "certificaterequests.certman.managed.openshift.io"
)

var log = logf.Log.WithName(controllerName)

// Add creates a new CertificateRequest Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileCertificateRequest{
		client:           mgr.GetClient(),
		scheme:           mgr.GetScheme(),
		awsClientBuilder: awsclient.NewClient,
		recorder:         mgr.GetRecorder(controllerName),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {

	c, err := controller.New("certificaterequest-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource CertificateRequest
	err = c.Watch(&source.Kind{Type: &certmanv1alpha1.CertificateRequest{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileCertificateRequest{}

// ReconcileCertificateRequest reconciles a CertificateRequest object
type ReconcileCertificateRequest struct {
	client           client.Client
	scheme           *runtime.Scheme
	recorder         record.EventRecorder
	awsClientBuilder func(kubeClient client.Client, secretName, namespace, region string) (awsclient.Client, error)
}

// Reconcile reads that state of the cluster for a CertificateRequest object and makes changes based on the state read
// and what is in the CertificateRequest.Spec
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

	// Check if CertificateResource has been deleted
	if cr.DeletionTimestamp.IsZero() {
		// add finalizer
		if !containsString(cr.ObjectMeta.Finalizers, finalizerName) {
			reqLogger.Info("Adding finalizer to the certificate request.")
			cr.ObjectMeta.Finalizers = append(cr.ObjectMeta.Finalizers, finalizerName)
			if err := r.client.Update(context.Background(), cr); err != nil {
				return reconcile.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if containsString(cr.ObjectMeta.Finalizers, finalizerName) {
			reqLogger.Info("Revoking certificate and deleting secret")
			if err := r.revokeCertificateAndDeleteSecret(cr); err != nil {
				return reconcile.Result{}, err
			}

			reqLogger.Info("Removing finalizers")
			cr.ObjectMeta.Finalizers = removeString(cr.ObjectMeta.Finalizers, finalizerName)
			if err := r.client.Update(context.Background(), cr); err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}

	certificateSecret := newSecret(cr)

	// Set CertificateRequest cr as the owner and controller
	if err := controllerutil.SetControllerReference(cr, certificateSecret, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	found := &corev1.Secret{}

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: certificateSecret.Name, Namespace: certificateSecret.Namespace}, found)

	// Issue New Certifcates
	if err != nil && errors.IsNotFound(err) {

		reqLogger.Info("Requesting new certificates")

		r.IssueCertificate(cr, certificateSecret)

		err = r.client.Create(context.TODO(), certificateSecret)
		if err != nil {
			return reconcile.Result{}, err
		}

		r.updateStatus(cr)

		reqLogger.Info("Certificate issued.")
		r.recorder.Event(cr, "Normal", "Created", fmt.Sprintf("Certificates issued and stored in secret  %s/%s", certificateSecret.Namespace, certificateSecret.Name))

		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, err
	}

	// Renew Certificates
	shouldRenew, err := r.ShouldRenewCertificates(cr, cr.Spec.RenewBeforeDays)
	if err != nil {
		return reconcile.Result{}, err
	}

	if shouldRenew {
		r.IssueCertificate(cr, found)

		err = r.client.Update(context.TODO(), found)
		if err != nil {
			return reconcile.Result{}, err
		}

		reqLogger.Info("cert issued")
		r.recorder.Event(cr, "Normal", "Updated", fmt.Sprintf("Certificates renewed and stored in secret  %s/%s", certificateSecret.Namespace, certificateSecret.Name))
	}

	r.updateStatus(cr)

	reqLogger.Info("Skip reconcile as valid certificates exist", "Secret.Namespace", found.Namespace, "Secret.Name", found.Name)
	return reconcile.Result{}, nil
}

func newSecret(cr *certmanv1alpha1.CertificateRequest) *corev1.Secret {
	return &corev1.Secret{
		Type: corev1.SecretTypeOpaque,
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.CertificateSecret.Name,
			Namespace: cr.Namespace,
		},
	}
}

func (r *ReconcileCertificateRequest) getAwsClient(cr *certmanv1alpha1.CertificateRequest) (awsclient.Client, error) {
	awsapi, err := r.awsClientBuilder(r.client, cr.Spec.PlatformSecrets.AWS.Credentials.Name, cr.Namespace, "us-east-1")
	return awsapi, err
}

func (r *ReconcileCertificateRequest) revokeCertificateAndDeleteSecret(cr *certmanv1alpha1.CertificateRequest) error {

	err := r.RevokeCertificate(cr)
	if err != nil {
		return err
	}

	return nil
}

func (r *ReconcileCertificateRequest) updateStatus(cr *certmanv1alpha1.CertificateRequest) error {

	if cr != nil {
		certificate, err := GetCertificate(r.client, cr)
		if err != nil {
			return err
		}

		if !cr.Status.Issued ||
			cr.Status.IssuerName != certificate.Issuer.CommonName ||
			cr.Status.NotBefore.Time != certificate.NotBefore ||
			cr.Status.NotAfter.Time != certificate.NotAfter ||
			cr.Status.SerialNumber != certificate.SerialNumber.String() {
			cr.Status.Issued = true
			cr.Status.IssuerName = certificate.Issuer.CommonName
			cr.Status.NotBefore.Time = certificate.NotBefore
			cr.Status.NotAfter.Time = certificate.NotAfter
			cr.Status.SerialNumber = certificate.SerialNumber.String()

			return r.client.Update(context.TODO(), cr)
		}
	}

	return nil
}
