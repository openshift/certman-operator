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
	"crypto/x509"
	"encoding/pem"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetCertificate(kubeClient client.Client, cr *certmanv1alpha1.CertificateRequest) (*x509.Certificate, error) {

	crtSecret, err := GetSecret(kubeClient, cr.Spec.CertificateSecret.Name, cr.Namespace)
	if err != nil {
		return nil, err
	}

	keyBlock, _ := pem.Decode(crtSecret.Data[TlsCertificateSecretKey])

	certificate, err := x509.ParseCertificate(keyBlock.Bytes)
	if err != nil {
		return nil, err
	}

	return certificate, nil
}
