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
	"strconv"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
)

// SecretExists returns a boolean to the caller basd on the secretName and namespace args.
func SecretExists(kubeClient client.Client, secretName, namespace string) bool {

	s := &corev1.Secret{}

	err := kubeClient.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: namespace}, s)
	if err != nil {
		return false
	}

	return true
}

// GetSecret returns a secret based on a secretName and namespace.
func GetSecret(kubeClient client.Client, secretName, namespace string) (*corev1.Secret, error) {

	s := &corev1.Secret{}

	err := kubeClient.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: namespace}, s)

	if err != nil {
		return nil, err
	}

	return s, nil
}

// GetDNSVerifyAttempts returns the attempts based on the annotation
func GetDNSVerifyAttempts(cr *certmanv1alpha1.CertificateRequest) int {
	if metav1.HasAnnotation(cr.ObjectMeta, dnsCheckAttemptsAnnotation) {
		attempts, err := strconv.Atoi(cr.Annotations[dnsCheckAttemptsAnnotation])
		if err != nil {
			return 0
		}
		return attempts
	}
	return 0
}

// IncrementDNSVerifyAttempts updates the given CR's challenge attempts via annotation key
func (r *ReconcileCertificateRequest) IncrementDNSVerifyAttempts(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) error {
	attempts := GetDNSVerifyAttempts(cr)
	attempts++

	// Make sure we have the latest version before we update
	err := r.client.Get(context.TODO(), types.NamespacedName{
		Namespace: cr.Namespace,
		Name:      cr.Name,
	}, cr)

	if metav1.HasAnnotation(cr.ObjectMeta, dnsCheckAttemptsAnnotation) {
		i, err := strconv.Atoi(cr.Annotations[dnsCheckAttemptsAnnotation])
		if err != nil {
			return err
		}
		if i == attempts {
			reqLogger.Info("Already has value")
			return nil
		}
	}
	metav1.SetMetaDataAnnotation(&cr.ObjectMeta, dnsCheckAttemptsAnnotation, strconv.Itoa(attempts))
	err = r.client.Update(context.TODO(), cr)
	if err != nil {
		reqLogger.Error(err, "Error updating DNS verify attempts annotation")
		return err
	}
	return nil
}
