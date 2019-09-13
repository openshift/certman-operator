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
	letsEncryptAccountPrivateKey           = "private-key"
	letsEncryptAccountURL                  = "account-url"
	StagingLetsEncryptCertIssuingAuthority = "Fake LE Intermediate X1"
	LetsEncryptCertIssuingAuthority        = "Let's Encrypt Authority X3"
	// Deprecated, use letsEncryptAccountSecretName instead
	letsEncryptProductionAccountSecretName = "lets-encrypt-account-production"
	// Deprecated, use letsEncryptAccountSecretName instead
	letsEncryptStagingAccountSecretName = "lets-encrypt-account-staging"
	letsEncryptAccountSecretName        = "lets-encrypt-account"
)
