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
	"testing"

	"github.com/openshift/certman-operator/config"
	"github.com/stretchr/testify/assert"

	cTypes "github.com/openshift/certman-operator/pkg/clients/types"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

/* Fake objects/vars */

const (
	// Fake strings.
	fakeOperatorName      = "Catatafish"
	fakeOperatorNamespace = "Lemmiwinks"
	fakeEmailAddress      = fakeOperatorName + "@" + fakeOperatorNamespace + ".com"
)

/* Testing objects/vars/types */
var (
	sliceOfStrings = []string{"Lemmiwinks", "Catatafish", "Sparrow Prince", "Frog King", "Wikileaks"}
	// testConfigMap is a configmap that can be used to return and validate functions.
	testConfigMap = &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.OperatorName,
			Namespace: config.OperatorNamespace,
		},
		Data: map[string]string{
			cTypes.DefaultNotificationEmailAddress: fakeEmailAddress,
		},
	}

	// fakeServiceAccountJSONCreds is a json blob based on google service_account key.
	// https://console.cloud.google.com/iam-admin/serviceaccounts
	// This satisfys google.CredentialsFromJSON.
	fakeServiceAccountJSONCreds = []byte(`{
  "type": "service_account",
  "project_id": "canvas-syntax-248604",
  "private_key_id": "xxxxxxx",
  "private_key": "xxxxxx",
  "client_email": "test-824@canvas-syntax-248604.iam.gserviceaccount.com",
  "client_id": "xxxxxxx",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://oauth2.googleapis.com/token",
  "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
  "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/test-824%40canvas-syntax-248604.iam.gserviceaccount.com"
}`)

	// testSecret is a secret that can be used to return and validate functions.
	testSecret = &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.OperatorName,
			Namespace: config.OperatorNamespace,
		},
		Data: map[string][]byte{
			"osServiceAccount.json": fakeServiceAccountJSONCreds,
		},
	}

	// Pass this to func to return configmap.
	testNamespaceName = types.NamespacedName{
		Name:      config.OperatorName,
		Namespace: config.OperatorNamespace,
	}
)

func TestGetConfig(t *testing.T) {

	// Use anonymous struct to iterate through use cases.
	testUnits := []struct {
		name        string
		runtimeObjs []runtime.Object
		validate    func(client.Client, *testing.T)
	}{
		{
			name:        "Validate getConfig",
			runtimeObjs: []runtime.Object{testConfigMap},
			validate: func(c client.Client, t *testing.T) {
				_, err := getConfig(c, testNamespaceName)
				assert.NoError(t, err)
			},
		},
		{
			name:        "Validate incorrect operator Name",
			runtimeObjs: []runtime.Object{testConfigMap},
			validate: func(c client.Client, t *testing.T) {
				// Break the test based on operator name.
				breakNamespaceName := testNamespaceName
				breakNamespaceName.Name = fakeOperatorName
				_, err := getConfig(c, breakNamespaceName)
				assert.Error(t, err)
			},
		},
		{
			name:        "Validate incorrect operator Namespace",
			runtimeObjs: []runtime.Object{testConfigMap},
			validate: func(c client.Client, t *testing.T) {
				// Break the test based on operator namespace.
				breakNamespaceName := testNamespaceName
				breakNamespaceName.Namespace = fakeOperatorNamespace
				_, err := getConfig(c, breakNamespaceName)
				assert.Error(t, err)
			},
		},
	}

	// Execute each use case of testUnits slice.
	for _, tt := range testUnits {
		t.Run(tt.name, func(t *testing.T) {

			s := scheme.Scheme
			fakeClient := fake.NewFakeClientWithScheme(s, tt.runtimeObjs...)

			tt.validate(fakeClient, t)

		})
	}
}

func TestGetDefaultNotificationEmailAddress(t *testing.T) {

	// Use anonymous struct to iterate through use cases.
	testUnits := []struct {
		name        string
		runtimeObjs []runtime.Object
		validate    func(client.Client, *testing.T)
	}{
		{
			name:        "Validate GetDefaultNotificationEmailAddress",
			runtimeObjs: []runtime.Object{testConfigMap},
			validate: func(c client.Client, t *testing.T) {
				email, err := GetDefaultNotificationEmailAddress(c)
				assert.NoError(t, err)
				assert.Equal(t, email, testConfigMap.Data[cTypes.DefaultNotificationEmailAddress])
			},
		},
		{
			name:        "Validate GetDefaultNotificationEmailAddress email not set",
			runtimeObjs: []runtime.Object{testConfigMap},
			validate: func(c client.Client, t *testing.T) {
				testConfigMap.Data[cTypes.DefaultNotificationEmailAddress] = ""
				err := c.Update(context.TODO(), testConfigMap)
				assert.NoError(t, err)
				email, err := GetDefaultNotificationEmailAddress(c)
				assert.Error(t, err)
				assert.Equal(t, email, testConfigMap.Data[cTypes.DefaultNotificationEmailAddress])
			},
		},
	}

	// Execute each use case of testUnits slice.
	for _, tt := range testUnits {
		t.Run(tt.name, func(t *testing.T) {

			s := scheme.Scheme
			fakeClient := fake.NewFakeClientWithScheme(s, tt.runtimeObjs...)

			tt.validate(fakeClient, t)
		})
	}
}

func TestGetCredentialsJSON(t *testing.T) {

	testUnits := []struct {
		name        string
		runtimeObjs []runtime.Object
		validate    func(client.Client, *testing.T)
	}{
		{
			name:        "Validate GetCredentialsJSON",
			runtimeObjs: []runtime.Object{testSecret},
			validate: func(c client.Client, t *testing.T) {
				_, err := GetCredentialsJSON(c, testNamespaceName)
				assert.NoError(t, err)
			},
		},
		{
			name:        "Validate GetCredentialsJSON incorrect namespace",
			runtimeObjs: []runtime.Object{testSecret},
			validate: func(c client.Client, t *testing.T) {
				testNamespaceName.Namespace = fakeOperatorNamespace
				_, err := GetCredentialsJSON(c, testNamespaceName)
				assert.Error(t, err)
			},
		},
	}
	// Execute each use case of testUnits slice.
	for _, tt := range testUnits {
		t.Run(tt.name, func(t *testing.T) {

			s := scheme.Scheme
			fakeClient := fake.NewFakeClientWithScheme(s, tt.runtimeObjs...)

			tt.validate(fakeClient, t)
		})
	}
}

func TestContainsString(t *testing.T) {
	t.Run("Validate ContainsString", func(t *testing.T) {
		validated := ContainsString(sliceOfStrings, "Sparrow Prince")
		assert.True(t, validated)
	})

	t.Run("Validate ContainsString fail", func(t *testing.T) {
		validated := ContainsString(sliceOfStrings, "Stan Darsh")
		assert.False(t, validated)

	})
}

func TestRemoveString(t *testing.T) {
	t.Run("Validate RemoveString", func(t *testing.T) {
		testSliceOfStrings := []string{"Catatafish", "Sparrow Prince", "Frog King", "Wikileaks"}
		result := RemoveString(sliceOfStrings, "Lemmiwinks")
		assert.Equal(t, result, testSliceOfStrings)
	})

	t.Run("Validate RemoveString with string not found", func(t *testing.T) {
		result := RemoveString(sliceOfStrings, "Stan Darsh")
		assert.Equal(t, result, sliceOfStrings)
	})
}
