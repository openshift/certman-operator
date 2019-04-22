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
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"strings"

	"github.com/eggsampler/acme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetLetsEncryptClient(staging bool) (acme.Client, error) {

	if staging {
		return acme.NewClient(acme.LetsEncryptStaging)
	}

	return acme.NewClient(acme.LetsEncryptProduction)
}

func GetLetsEncryptAccount(kubeClient client.Client, staging bool, namespace string) (letsEncryptAccount acme.Account, err error) {

	secretName := LetsEncryptProductionAccountSecretName

	if staging {
		secretName = LetsEncryptStagingAccountSecretName
	}

	secret, err := GetSecret(kubeClient, secretName, namespace)
	if err != nil {
		return letsEncryptAccount, err
	}

	urlBytes := secret.Data[LetsEncryptAccountUrl]
	accountUrl := string(urlBytes)

	keyBytes := secret.Data[LetsEncryptAccountPrivateKey]
	keyBlock, _ := pem.Decode(keyBytes)

	var privateKey crypto.Signer

	switch keyBlock.Type {
	case "RSA PRIVATE KEY":
		privateKey, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
		return letsEncryptAccount, err
	case "EC PRIVATE KEY":
		privateKey, err = x509.ParseECPrivateKey(keyBlock.Bytes)
		return letsEncryptAccount, err
	}

	letsEncryptAccount = acme.Account{PrivateKey: privateKey, URL: accountUrl}

	return letsEncryptAccount, nil
}

func GetLetsEncryptAccountPrivateKey(kubeClient client.Client, staging bool, namespace string) (privateKey crypto.Signer, err error) {

	secretName := LetsEncryptProductionAccountSecretName

	if staging {
		secretName = LetsEncryptStagingAccountSecretName
	}

	secret, err := GetSecret(kubeClient, secretName, namespace)
	if err != nil {
		return privateKey, err
	}

	keyBytes := secret.Data[LetsEncryptAccountPrivateKey]
	keyBlock, _ := pem.Decode(keyBytes)

	switch keyBlock.Type {
	case "RSA PRIVATE KEY":
		privateKey, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
		return privateKey, err
	case "EC PRIVATE KEY":
		privateKey, err = x509.ParseECPrivateKey(keyBlock.Bytes)
		return privateKey, err
	}

	return privateKey, nil
}

func GetLetsEncryptAccountUrl(kubeClient client.Client, staging bool, namespace string) (url string, err error) {

	secretName := LetsEncryptProductionAccountSecretName

	if staging {
		secretName = LetsEncryptStagingAccountSecretName
	}

	secret, err := GetSecret(kubeClient, secretName, namespace)
	if err != nil {
		return "", err
	}

	urlBytes := secret.Data[LetsEncryptAccountUrl]
	url = string(urlBytes)
	url = strings.TrimRight(url, "\n")

	return url, nil
}

func GetCertExpiryNotificationList(email string) []string {
	var contacts []string

	if email != "" {
		contacts = append(contacts, "mailto:"+email)
	}

	//todo add default email

	return contacts
}
