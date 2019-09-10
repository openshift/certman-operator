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

	certman "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"

	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetSecret(kubeClient client.Client, secretName, namespace string) (*corev1.Secret, error) {

	s := &corev1.Secret{}

	err := kubeClient.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: namespace}, s)

	if err != nil {
		return nil, err
	}

	return s, nil
}

// ExponentialBackOff will sleep for a specific amount of time, determined by the number of API failures
// which are recorded in the CertificateRequestConditions under Status.
func ExponentialBackOff(cr *certman.CertificateRequest, queryType string) error {
	for i, condition := range cr.Status.Conditions {
		if string(condition.Type) == queryType {
			failCount, err := strconv.Atoi(string(condition.Status))
			if err != nil {
				return err
			}
			// Sleep an exponential number of seconds after each API call failure.
			// For example: 2 seconds, 4 seconds, 16 seconds, 32 seconds...
			seconds := 200 * time.Millisecond
			sleeptime := seconds << uint(failCount)
			println("Exponential backoff: sleeping", sleeptime, "seconds.")
			time.Sleep(sleeptime)
		}
	}
	return nil
}

// AddToFailCount increments the CertificateRequestConditions.Status number to indicate an API failure.
// Specify querytype `FailureCountLetsEncrypt` or FailureCountAWS` to update CertificateRequestCondition.
func AddToFailCount(cr *certman.CertificateRequest, queryType string) error {
	for i, condition := range cr.Status.Conditions {
		if string(condition.Type) == queryType {
			failCount, err := strconv.Atoi(string(condition.Status))
			if err != nil {
				return err
			}
			failCount = failCount + 1
			failCountString := strconv.Itoa(failCount)

			condition.Status = corev1.ConditionStatus{failCountString} // this fails
			condition.Status = failCountString                         // this does too
		}
	}
	return nil
}
