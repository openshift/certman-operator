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

	"github.com/openshift/certman-operator/config"
	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	"github.com/openshift/certman-operator/pkg/leclient"
)

// RevokeCertificate validates which letsencrypt endpoint is to be used along with corresponding account.
// Then revokes certificate upon matching the CommonName of LetsEncryptCertIssuingAuthority.
// Associated ACME challenge resources are also removed.
func (r *ReconcileCertificateRequest) RevokeCertificate(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) error {

	leClient, err := leclient.GetLetsEncryptClient(r.client, cr)

	if err != nil {
		reqLogger.Error(err, "failed to get letsencrypt client")
		return err
	}

	err = leClient.GetAccount(r.client, config.OperatorNamespace, cr)
	if err != nil {
		return err
	}
	certificate, err := GetCertificate(r.client, cr)
	if err != nil {
		reqLogger.Error(err, "error occurred loading current certificate")
		return err
	}

	if certificate.Issuer.CommonName == leclient.LetsEncryptCertIssuingAuthority || certificate.Issuer.CommonName == leclient.StagingLetsEncryptCertIssuingAuthority {
		if err := leClient.RevokeCertificate(cr, certificate); err != nil {
			if !strings.Contains(err.Error(), "urn:ietf:params:acme:error:alreadyRevoked") {
				return err
			}
		}
		reqLogger.Info("certificate have been successfully revoked")
	} else {
		return fmt.Errorf("certificate was not issued by Let's Encrypt and cannot be revoked by the operator")
	}

	err = r.DeleteAcmeChallengeResourceRecords(reqLogger, cr)
	if err != nil {
		reqLogger.Error(err, "error occurred deleting acme challenge resource records from Route53")
	}

	return nil
}
