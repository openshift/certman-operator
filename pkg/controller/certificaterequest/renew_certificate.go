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
	"time"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
)

func (r *ReconcileCertificateRequest) ShouldRenewCertificates(cr *certmanv1alpha1.CertificateRequest, renewBeforeDays int) (bool, error) {

	if renewBeforeDays <= 0 {
		renewBeforeDays = RenewCertificateBeforeDays
	}

	certificate, err := GetCertificate(r.client, cr)
	if err != nil {
		log.Error(err, "There was problem loading existing certificate")
		return false, err
	}

	if certificate != nil {
		notAfterTime := certificate.NotAfter
		currentTime := time.Now().In(time.UTC)
		timeDiff := notAfterTime.Sub(currentTime)
		daysCertificateValidFor := int(timeDiff.Hours() / 24)
		return daysCertificateValidFor <= renewBeforeDays, nil
	}

	return false, nil
}
