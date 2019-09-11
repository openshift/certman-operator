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

	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/go-logr/logr"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	"github.com/openshift/certman-operator/pkg/awsclient"
	"github.com/openshift/certman-operator/pkg/controller/controllerutils"

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
	client           client.Client
	scheme           *runtime.Scheme
	recorder         record.EventRecorder
	awsClientBuilder func(kubeClient client.Client, secretName, namespace, region string) (awsclient.Client, error)
}

// Reconcile reads that state of the cluster for a CertificateRequest object and makes changes based on the state read
// and what is in the CertificateRequest.Spec
func (r *ReconcileCertificateRequest) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	reqLogger.Info("reconciling CertificateRequest")

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

	// Check if CertificateResource is being deleted, if lt's deleted, revoke the certificate and remove the finalizer if it exists.
	if !cr.DeletionTimestamp.IsZero() {
		// The object is being deleted
		if controllerutils.ContainsString(cr.ObjectMeta.Finalizers, certmanv1alpha1.CertmanOperatorFinalizerLabel) {
			reqLogger.Info("revoking certificate and deleting secret")
			if err := r.revokeCertificateAndDeleteSecret(reqLogger, cr); err != nil {
				reqLogger.Error(err, err.Error())
				return reconcile.Result{}, err
			}

			reqLogger.Info("removing finalizers")
			cr.ObjectMeta.Finalizers = controllerutils.RemoveString(cr.ObjectMeta.Finalizers, certmanv1alpha1.CertmanOperatorFinalizerLabel)
			if err := r.client.Update(context.TODO(), cr); err != nil {
				reqLogger.Error(err, err.Error())
				return reconcile.Result{}, err
			}
		}
		reqLogger.Info("certificaterequest has been deleted")
		return reconcile.Result{}, nil

	}

	// Add finalizer if not exists
	if !controllerutils.ContainsString(cr.ObjectMeta.Finalizers, certmanv1alpha1.CertmanOperatorFinalizerLabel) {
		reqLogger.Info("adding finalizer to the certificate request")
		cr.ObjectMeta.Finalizers = append(cr.ObjectMeta.Finalizers, certmanv1alpha1.CertmanOperatorFinalizerLabel)
		if err := r.client.Update(context.TODO(), cr); err != nil {
			reqLogger.Error(err, err.Error())
			return reconcile.Result{}, err
		}
	}

	// Check credentials and exit the reconcile loop if needed.
	if err := TestAuth(cr, r); err != nil {
		reqLogger.Error(err, err.Error())
		return reconcile.Result{}, err
	}

	certificateSecret := newSecret(cr)

	// Set CertificateRequest cr as the owner and controller
	if err := controllerutil.SetControllerReference(cr, certificateSecret, r.scheme); err != nil {
		reqLogger.Error(err, err.Error())
		return reconcile.Result{}, err
	}

	found := &corev1.Secret{}

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: certificateSecret.Name, Namespace: certificateSecret.Namespace}, found)

	// Issue New Certifcates if the secret not exists
	if err != nil && errors.IsNotFound(err) {

		reqLogger.Info("requesting new certificates as secret was not found")

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
	} else if err != nil {
		reqLogger.Error(err, err.Error())
		return reconcile.Result{}, err
	}

	reqLogger.Info("checking if certificates need to be renewed or reissued")

	// Renew Certificates
	shouldRenewOrReIssue, err := r.ShouldRenewOrReIssue(reqLogger, cr)
	if err != nil {
		reqLogger.Error(err, err.Error())
		return reconcile.Result{}, err
	}

	if shouldRenewOrReIssue {
		err := r.IssueCertificate(reqLogger, cr, found)
		if err != nil {
			return reconcile.Result{}, err
		}

		err = r.client.Update(context.TODO(), found)
		if err != nil {
			return reconcile.Result{}, err
		}

		err = r.updateStatus(reqLogger, cr)
		if err != nil {
			reqLogger.Error(err, err.Error())
		}

		reqLogger.Info("certificate has been renewed/re-issued.")
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

// getAwsClient returns awsclient to the caller
func (r *ReconcileCertificateRequest) getAwsClient(cr *certmanv1alpha1.CertificateRequest) (awsclient.Client, error) {
	awsapi, err := r.awsClientBuilder(r.client, cr.Spec.PlatformSecrets.AWS.Credentials.Name, cr.Namespace, "us-east-1") //todo why is this region var hardcoded???
	return awsapi, err
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

// TestAuth examines the credentials associated with a ReconcileCertificateRequest
// and returns an error if the credentials are missing or if they're missing required permission.
func TestAuth(cr *certmanv1alpha1.CertificateRequest, r *ReconcileCertificateRequest) error {
	platformSecretName := cr.Spec.PlatformSecrets.AWS.Credentials.Name

	awscreds := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: platformSecretName, Namespace: cr.Namespace}, awscreds)
	if err != nil {
		fmt.Println("platformSecrets were not found. Unable to search for certificates in cloud provider platform")
		return err
	}
	// Ensure that platform Secret can authenticate to AWS.
	r53svc, err := r.getAwsClient(cr)
	if err != nil {
		return err
	}

	hostedZoneOutput, err := r53svc.ListHostedZones(&route53.ListHostedZonesInput{})
	if err != nil {
		fmt.Println("platformSecrets are either invalid, or don't have permission to list Route53 HostedZones")
		return err
	}

	println("Successfully authenticated with cloudprovider. Hosted zones found:")
	println(hostedZoneOutput)

	return nil
}
