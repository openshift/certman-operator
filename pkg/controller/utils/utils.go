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

package utils

import (
	"context"
	"fmt"

	"golang.org/x/oauth2/google"
	dnsv1 "google.golang.org/api/dns/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cTypes "github.com/openshift/certman-operator/pkg/clients/types"

	"github.com/openshift/certman-operator/config"
)

// After instantiating a configmap object, GetDefaultNotificationEmailAddress validates
// if there is a default email address present or not.
func GetDefaultNotificationEmailAddress(kubeClient client.Client) (string, error) {
	cm, err := getConfig(kubeClient, types.NamespacedName{Name: config.OperatorName, Namespace: config.OperatorNamespace})
	if err != nil {
		return "", err
	}

	if cm.Data[cTypes.DefaultNotificationEmailAddress] == "" {
		return "", fmt.Errorf("Default notification email not found in configmap.")
	}

	return cm.Data[cTypes.DefaultNotificationEmailAddress], nil
}

func GetCredentialsJSON(kubeClient client.Client, namespacesedName types.NamespacedName) (*google.Credentials, error) {
	secret, err := getSecret(kubeClient, namespacesedName)
	if err != nil {
		return nil, err
	}
	sa := secret.Data["osServiceAccount.json"]
	cred, err := google.CredentialsFromJSON(context.Background(), sa, dnsv1.NdevClouddnsReadwriteScope)
	if err != nil {
		return nil, err
	}
	return cred, err
}

// getConfig retrieves config from kubernetes and returns a ConfigMap object.
func getConfig(kubeClient client.Client, namespacesedName types.NamespacedName) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	err := kubeClient.Get(context.TODO(), namespacesedName, cm)
	if err != nil {
		return nil, err
	}

	return cm, nil
}

// getSecret retrieves config from kubernetes and returns a ConfigMap object.
func getSecret(kubeClient client.Client, namespacesedName types.NamespacedName) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := kubeClient.Get(context.TODO(), namespacesedName, secret)
	if err != nil {
		return nil, err
	}

	return secret, nil
}
