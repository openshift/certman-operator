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
	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
)

type dnsRCode int

const (
	cloudflareDNSOverHTTPSEndpoint       = "https://cloudflare-dns.com/dns-query"
	cloudflareRequestContentType         = "application/dns-json"
	cloudflareRequestTimeout             = 60
	maxAttemptsForDNSPropagationCheck    = 10  // Try 10 times (5 minutes total)
	initialWaitPeriodDNSPropagationCheck = 60  // Initial 60 seconds wait period to prevent negative cache ttl response
	defaultWaitPeriodDNSPropagationCheck = 30  // Wait 30 seconds between checks
	maxNegativeCacheTTL                  = 600 // Sleep no more than 10 minutes
	reissueCertificateBeforeDays         = 45  // This helps us avoid getting email notifications from Let's Encrypt.
	resourceRecordTTL                    = 60
	rSAKeyBitSize                        = 2048

	// From golang.org/x/net/dns/dnsmessage
	dnsRCodeSuccess        dnsRCode = 0
	dnsRCodeFormatError    dnsRCode = 1
	dnsRCodeServerFailure  dnsRCode = 2
	dnsRCodeNameError      dnsRCode = 3
	dnsRCodeNotImplemented dnsRCode = 4
	dnsRCodeRefused        dnsRCode = 5

	// cr annotation dns check attempts label
	dnsCheckAttemptsAnnotation = certmanv1alpha1.CertmanOperatorFinalizerLabel + "/dns-check-attempts"
)
