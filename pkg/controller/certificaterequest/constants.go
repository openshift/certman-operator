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

// Cloudflare DNS over HTTPS
const (
	CloudflareDnsOverHttpsEndpoint         = "https://cloudflare-dns.com/dns-query"
	CloudflareRequestContentType           = "application/dns-json"
	CloudflareRequestTimeout               = 60
	MaxAttemptsForDnsPropogationCheck      = 5
	WaitTimePeriodDnsPropogationCheck      = 60
	LetsEncryptAccountPrivateKey           = "private-key"
	LetsEncryptAccountUrl                  = "account-url"
	TlsCertificateSecretKey                = "crt"
	AcmeChallengeSubDomain                 = "_acme-challenge"
	OpenShiftDotCom                        = "openshift.com"
	OpenShiftAppsDotCom                    = "openshiftapps.com"
	AosSreEmailAddr                        = "sd-sre@redhat.com"
	RenewCertificateBeforeDays             = 32 // This helps us avoid getting email notifications from Let's Encrypt.
	ResourceRecordTTL                      = 60
	RSAKeyBitSize                          = 2048
	LetsEncryptProductionAccountSecretName = "lets-encrypt-account-production"
	LetsEncryptCertIssuingAuthority        = "Let's Encrypt Authority X3"
	LetsEncryptStagingAccountSecretName    = "lets-encrypt-account-staging"
	StagingLetsEncryptCertIssuingAuthority = "Fake LE Intermediate X1"
)
