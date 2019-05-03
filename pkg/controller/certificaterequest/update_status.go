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
)

func (r *ReconcileCertificateRequest) updateStatus(cr *certmanv1alpha1.CertificateRequest) error {

	if cr != nil {
		certificate, err := GetCertificate(r.client, cr)
		if err != nil {
			return err
		}

		if certificate == nil {
			return fmt.Errorf("certificate is nil")
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

			err := r.client.Status().Update(context.TODO(), cr)
			if err != nil {
				log.Error(err, err.Error())
				return err
			}
		}
	}

	return nil
}
