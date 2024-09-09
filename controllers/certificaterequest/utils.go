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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SecretExists returns a boolean to the caller basd on the secretName and namespace args.
func SecretExists(kubeClient client.Client, secretName, namespace string) (bool, error) {
	s := &corev1.Secret{}
	err := kubeClient.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: namespace}, s)
	// If the secret is not found, we return false with no error,
	if err == nil {
		return true, nil
	}
	// as non-existence is not considered an error condition here.
	if errors.IsNotFound(err) {
		return false, nil
	}
	// distinguish between non-existence and other types of errors.
	return false, err
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

// returns a pointer to a boolean
func boolPointer(b bool) *bool {
	return &b
}
