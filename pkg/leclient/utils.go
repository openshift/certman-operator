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
	"errors"
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
		AddToFailCount(cr, "FailCountLetsEncrypt")
		return nil, err
	}

	return s, nil
}

// AddToFailCount increments the fields CertificateRequestStatus.FailCountLetsEncrypt and .FailCountAWS
// to indicate the number of times an API request has failed.
func AddToFailCount(cr *certman.CertificateRequest, queryType string) error {
	fmt.Println("DEBUG: cr.Status.FailCountLetsEncrypt:", cr.Status.FailCountLetsEncrypt)
	fmt.Println("DEBUG: cr.Status.FailCountAWS:", cr.Status.FailCountAWS)
	if queryType == "FailCountLetsEncrypt" {
		if cr.Status.FailCountLetsEncrypt >= 2147483647 {
			fmt.Println("DEBUG: FailCount not incremented due to overflow possibility")
			return nil
		}
		fmt.Println("DEBUG: Incrementing cr.Status.FailCountLetsEncrypt")
		cr.Status.FailCountLetsEncrypt = cr.Status.FailCountLetsEncrypt + 1
	} else if queryType == "FailCountAWS" {
		if cr.Status.FailCountAWS >= 2147483647 {
			fmt.Println("DEBUG: FailCount not incremented due to overflow possibility")
			return nil
		}
		fmt.Println("DEBUG: Incrementing cr.Status.FailCountAWS")
		cr.Status.FailCountAWS = cr.Status.FailCountAWS + 1
	} else {
		return errors.New("Invalid queryType passed. Options are FailCountAWS or FailCountLetsEncrypt")
	}

	return nil
}
