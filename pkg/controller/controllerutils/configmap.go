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

package controllerutils

import (
	"context"
	"fmt"

	"github.com/openshift/certman-operator/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetConfig(kubeClient client.Client) (*corev1.ConfigMap, error) {

	cm := &corev1.ConfigMap{}

	err := kubeClient.Get(context.TODO(), types.NamespacedName{Name: "certman-operator", Namespace: config.OperatorNamespace}, cm)
	if err != nil {
		return nil, err
	}

	return cm, nil
}

func UsetLetsEncryptStagingEnvironment(kubeClient client.Client) bool {
	cm, err := GetConfig(kubeClient)
	if err != nil {
		return true
	}

	if cm.Data["lets_encrypt_environment"] == "staging" || cm.Data["lets_encrypt_environment"] == "" {
		return true
	}

	return false
}

func GetDefaultNotificationEmailAddress(kubeClient client.Client) (string, error) {
	cm, err := GetConfig(kubeClient)
	if err != nil {
		return "", err
	}

	if cm.Data["default_notification_email_address"] == "" {
		return "", fmt.Errorf("Default notification email not found in configmap.")
	}

	return cm.Data["default_notification_email_address"], nil
}
