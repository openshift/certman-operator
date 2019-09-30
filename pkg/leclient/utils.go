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

package leclient

import (
	"context"
	"fmt"

	certman "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	"github.com/openshift/certman-operator/pkg/sleep"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetSecret returns the specified Secret from the specified namespace.
func GetSecret(kubeClient client.Client, secretName, namespace string, cr *certman.CertificateRequest) (*corev1.Secret, error) {

	sleep.ExponentialBackOff(cr.Status.FailCountLetsEncrypt)
	s := &corev1.Secret{}

	err := kubeClient.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: namespace}, s)

	if err != nil {
		AddToFailCount(cr)
		return nil, err
	}

	return s, nil
}

// AddToFailCount increments the fields CertificateRequestStatus.FailCount
// to indicate the number of times an API request has failed.
func AddToFailCount(cr *certman.CertificateRequest) error {
	fmt.Println("DEBUG: cr.Status.FailCount:", cr.Status.FailCount)
	if cr.Status.FailCount >= 2147483647 {
		fmt.Println("DEBUG: FailCount not incremented due to overflow possibility")
		return nil
	}
	fmt.Println("DEBUG: Incrementing cr.Status.FailCount")
	cr.Status.FailCount = cr.Status.FailCount + 1
	return nil
}
