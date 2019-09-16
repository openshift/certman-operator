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
	"math"

	certman "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"

	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetSecret returns the specified Secret from the specified namespace.
func GetSecret(kubeClient client.Client, secretName, namespace string) (*corev1.Secret, error) {

	s := &corev1.Secret{}

	err := kubeClient.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: namespace}, s)

	if err != nil {
		return nil, err
	}

	return s, nil
}

// ExponentialBackOff will sleep for a minimum of 2 seconds, maxiumum of 2 hours,
// depending on the number of API failures encountered for a specific ConditionStatus.
// It increases by a power of 2 for each API failure.
// For example: 2 seconds, 4 seconds, 8 seconds, 16 seconds...
func ExponentialBackOff(cr *certman.CertificateRequest, queryType string) error {
	for _, condition := range cr.Status.Conditions {
		if string(condition.Type) == queryType {
			failCount, err := strconv.Atoi(string(condition.Status))
			if err != nil {
				return err
			}
			// Sleeptime is a minimum of 2 seconds (1<<1), maximum of 2 hours (7200).
			sleeptime := math.Min(7200, float64(uint(1)<<uint(failCount)))
			println("Exponential backoff: sleeping", sleeptime, "seconds.")
			time.Sleep(time.Duration(sleeptime) * time.Second)
		}
	}
	return nil
}

// AddToFailCount increments the fields CertificateRequestStatus.FailCountLetsEncrypt and .FailCountAWS
// to indicate the number of times an API request has failed.
func AddToFailCount(cr *certman.CertificateRequest, queryType string) error {
	fmt.Println("DEBUG: AddToFailCount()")
	fmt.Println("DEBUG: queryType:")
	fmt.Println(queryType)
	fmt.Println("DEBUG: cr.Status.FailCountLetsEncrypt")
	fmt.Println(cr.Status.FailCountLetsEncrypt)
	fmt.Println("DEBUG: cr.Status.FailCountAWS")
	fmt.Println(cr.Status.FailCountAWS)
	if queryType == "FailCountLetsEncrypt" {
		fmt.Println("DEBUG: Incrementing cr.Status.FailCountLetsEncrypt")
		cr.Status.FailCountLetsEncrypt = cr.Status.FailCountLetsEncrypt + 1
	} else if queryType == "FailCountAWS" {
		fmt.Println("DEBUG: Incrementing cr.Status.FailCountAWS")
		cr.Status.FailCountAWS = cr.Status.FailCountAWS + 1
	} else {
		return errors.New("Invalid queryType passed. Options are FailCountAWS or FailCountLetsEncrypt")
	}

	return nil
}

// ResetFailCount sets the fields CertificateRequestStatus.FailCountLetsEncrypt
// and .FailCountAWS to zero. This is used to indicate that the CertificateRequest
// was processed successfully.
func ResetFailCount(cr *certman.CertificateRequest, queryType string) error {
	if queryType == "FailCountLetsEncrypt" {
		fmt.Println("DEBUG: Resetting cr.Status.FailCountLetsEncrypt")
		cr.Status.FailCountLetsEncrypt = 0
	} else if queryType == "FailCountAWS" {
		fmt.Println("DEBUG: Resetting cr.Status.FailCountAWS")
		cr.Status.FailCountAWS = 0
	} else {
		return errors.New("Invalid queryType passed. Options are FailCountAWS or FailCountLetsEncrypt")
	}

	return nil
}
