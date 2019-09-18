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
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// updateStatus attempts to retrieve a certificate and check its Issued state. If not Issued,
// the required CertificateRequest variables are populated and updated.
func (r *ReconcileCertificateRequest) updateStatus(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) error {

	if cr != nil {
		certificate, err := GetCertificate(r.client, cr)
		if err != nil {
			return err
		}

		if certificate == nil {
			return fmt.Errorf("no certificate found")
		}

		if !cr.Status.Issued ||
			cr.Status.IssuerName != certificate.Issuer.CommonName ||
			cr.Status.NotBefore != certificate.NotBefore.String() ||
			cr.Status.NotAfter != certificate.NotAfter.String() ||
			cr.Status.SerialNumber != certificate.SerialNumber.String() {

			cr.Status.Issued = true
			cr.Status.IssuerName = certificate.Issuer.CommonName
			cr.Status.NotBefore = certificate.NotBefore.String()
			cr.Status.NotAfter = certificate.NotAfter.String()
			cr.Status.SerialNumber = certificate.SerialNumber.String()
			cr.Status.Status = "Success"
			cr.Status.FailCountLetsEncrypt = 0
			cr.Status.FailCountAWS = 0

			r.commitCRStatus(cr, reqLogger)
		}
	}

	return nil
}

//Function for handling a generic ACME error from cert issuer.
//Function will add a condition to the CertificateRequest with the return body from issuing cert request.
func acmeError(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest, err error) (certmanv1alpha1.CertificateRequestCondition, error) {
	var found bool
	var newCondition certmanv1alpha1.CertificateRequestCondition
	//Check for this as an existing Condition. If found no new action will be taken.
	for index := range cr.Status.Conditions {
		if cr.Status.Conditions[index].Type == "acme error" {
			found = true
		}
	}
	//If Condition is not present then a new Condition will be constructed and returned.
	if found != true {
		m := fmt.Sprint(err)
		newCondition.Type = certmanv1alpha1.CertificateRequestConditionType("acme error")
		newCondition.Status = corev1.ConditionStatus("Error")
		newCondition.Message = &m

		reqLogger.Info("Added condition 'acme error'")
	}
	return newCondition, nil
}

func (r *ReconcileCertificateRequest) updateStatusError(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest, err error) error {

	if cr != nil {
		cr.Status.Issued = false
		cr.Status.Status = "Error"

		//Check the error for different strings to indicate reason for failure
		if strings.Contains(err.Error(), "acme") {
			newCondition, err2 := acmeError(reqLogger, cr, err)
			if err2 != nil {
				reqLogger.Error(err2, err2.Error())
			} else if newCondition.Status != "" {
				//If a new Condition has a status the new Condition is added to the Status.
				cr.Status.Conditions = append(cr.Status.Conditions, newCondition)
			}

		}
		// add more known failure cases here when discovered.
		// if strings.Contains(err.Error(), "string")

		r.commitCRStatus(cr, reqLogger)

	}
	return nil
}
