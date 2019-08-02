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
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/eggsampler/acme"
	"github.com/go-logr/logr"
	"github.com/openshift/certman-operator/config"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetLetsEncryptClient(staging bool) (acme.Client, error) {

	if staging {
		return acme.NewClient(acme.LetsEncryptStaging)
	}

	return acme.NewClient(acme.LetsEncryptProduction)
}

func GetLetsEncryptAccount(reqLogger logr.Logger, kubeClient client.Client, staging bool, namespace string) (letsEncryptAccount acme.Account, err error) {

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
	reqLogger.Info("start it here")
	switch keyBlock.Type {
	case "RSA PRIVATE KEY":
		privateKey, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
		reqLogger.Info("RSA type key")
		return letsEncryptAccount, err
	case "EC PRIVATE KEY":
		privateKey, err = x509.ParseECPrivateKey(keyBlock.Bytes)
		reqLogger.Info("EC type key")
		return letsEncryptAccount, err
	}
	reqLogger.Info("made it here")
	letsEncryptAccount = acme.Account{PrivateKey: privateKey, URL: accountUrl}

	return letsEncryptAccount, nil
}

func Account(reqLogger logr.Logger, kubeClient client.Client, staging bool, namespace string) (letsEncryptAccount acme.Account, err error) {

	accountUrl, err := GetLetsEncryptAccountUrl(kubeClient, true)
	if err != nil {
		return letsEncryptAccount, err
	}

	privateKey, err := GetLetsEncryptAccountPrivateKey(kubeClient, true)
	if err != nil {
		return letsEncryptAccount, err
	}
	letsEncryptAccount2 := acme.Account{PrivateKey: privateKey, URL: accountUrl}
	return letsEncryptAccount2, nil
}

func GetLetsEncryptAccountPrivateKey(kubeClient client.Client, staging bool) (privateKey crypto.Signer, err error) {

	secretName := LetsEncryptProductionAccountSecretName

	if staging {
		secretName = LetsEncryptStagingAccountSecretName
	}

	secret, err := GetSecret(kubeClient, secretName, config.OperatorNamespace)
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

func GetLetsEncryptAccountUrl(kubeClient client.Client, staging bool) (url string, err error) {

	secretName := LetsEncryptProductionAccountSecretName

	if staging {
		secretName = LetsEncryptStagingAccountSecretName
	}

	secret, err := GetSecret(kubeClient, secretName, config.OperatorNamespace)
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

	return contacts
}

func EncodeDNS01KeyAuthorization(keyauth string) string {
	encode := acme.EncodeDNS01KeyAuthorization(keyauth)
	return encode
}
func ChallengeType(auth acme.Authorization) (acme.Challenge, error) {
	challenge, ok := auth.ChallengeMap["dns-01"]
	if !ok {
		var err error
		return challenge, err
	}
	return challenge, nil
}

func CreateOrder(reqLogger logr.Logger, leAccount acme.Account, leClient acme.Client, domains []string) (acme.Order, error) {
	var certDomains []string
	var ids []acme.Identifier

	for _, domain := range domains {
		reqLogger.Info(fmt.Sprintf("%v domain will be added to certificate request", domain))
		certDomains = append(certDomains, domain)
		ids = append(ids, acme.Identifier{Type: "dns", Value: domain})
	}
	letsEncryptOrder, err := leClient.NewOrder(leAccount, ids)
	if err != nil {
		return letsEncryptOrder, err
	}
	return letsEncryptOrder, nil
}
