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

const (
	letsEncryptAccountPrivateKey = "private-key"
	letsEncryptAccountUrl        = "account-url"
	// if letsEncryptAccountUrl is this value then a mock acme client will be used
	mockAcmeAccountUrl = "proto://use.mock.acme.client"
	// Deprecated, use letsEncryptAccountSecretName instead
	letsEncryptProductionAccountSecretName = "lets-encrypt-account-production"
	// Deprecated, use letsEncryptAccountSecretName instead
	letsEncryptStagingAccountSecretName = "lets-encrypt-account-staging"
	letsEncryptAccountSecretName        = "lets-encrypt-account"
)
