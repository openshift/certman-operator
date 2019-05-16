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

	"github.com/eggsampler/acme"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	"github.com/openshift/certman-operator/pkg/controller/controllerutils"
)

func (r *ReconcileCertificateRequest) RevokeCertificate(cr *certmanv1alpha1.CertificateRequest) error {

	useLetsEncryptStagingEndpoint := controllerutils.UsetLetsEncryptStagingEnvironment(r.client)

	letsEncryptClient, err := GetLetsEncryptClient(useLetsEncryptStagingEndpoint)
	if err != nil {
		log.Error(err, "Error occurred getting Let's Encrypt client.")
		return err
	}

	accountUrl, err := GetLetsEncryptAccountUrl(r.client, useLetsEncryptStagingEndpoint)
	if err != nil {
		return err
	}

	privateKey, err := GetLetsEncryptAccountPrivateKey(r.client, useLetsEncryptStagingEndpoint)
	if err != nil {
		return err
	}

	letsEncryptAccount := acme.Account{PrivateKey: privateKey, URL: accountUrl}

	certificate, err := GetCertificate(r.client, cr)
	if err != nil {
		log.Error(err, "Error occurred loading current certificate.")
		return err
	}

	if certificate.Issuer.CommonName == LetsEncryptCertIssuingAuthority || certificate.Issuer.CommonName == StagingLetsEncryptCertIssuingAuthority {
		if err := letsEncryptClient.RevokeCertificate(letsEncryptAccount, certificate, letsEncryptAccount.PrivateKey, acme.ReasonUnspecified); err != nil {
			if !strings.Contains(err.Error(), "urn:ietf:params:acme:error:alreadyRevoked") {
				return err
			}
		}
		log.Info("Certificates were successfully revoked.")
	} else {
		return fmt.Errorf("Certificate was not issued by Let's Encrypt. Certman operator cannot revoke this certificate.")
	}

	err = r.DeleteAcmeChallengeResourceRecords(cr)
	if err != nil {
		log.Error(err, "Error occurred deleting acme challenge resource records from Route53")
		return err
	}

	return nil
}
