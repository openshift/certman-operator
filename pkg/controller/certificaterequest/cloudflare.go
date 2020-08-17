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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
)

type CloudflareQuestion struct {
	Name string `json:"name"`
	Type int    `json:"type"`
}

type CloudflareAnswer struct {
	Name string `json:"name"`
	Type int    `json:"type"`
	TTL  int    `json:"TTL"`
	Data string `json:"data"`
}

type CloudflareResponse struct {
	Status    int                  `json:"Status"`
	TC        bool                 `json:"TC"`
	RC        bool                 `json:"RC"`
	RA        bool                 `json:"RA"`
	AD        bool                 `json:"AD"`
	CD        bool                 `json:"CD"`
	Questions []CloudflareQuestion `json:"Question"`
	Answers   []CloudflareAnswer   `json:"Answer"`
	Authority []CloudflareAnswer   `json:"Authority"`
}

// VerifyDnsResourceRecordUpdate verifies the presence of a TXT record with Cloudflare DNS. Returns 0 on success.
func VerifyDnsResourceRecordUpdate(reqLogger logr.Logger, fqdn string, txtValue string) int {
	var negativeCacheTTL int
	var err error

	// Set default wait period to 30s
	waitPeriod := defaultWaitPeriodDNSPropagationCheck

	reqLogger.Info(fmt.Sprintf("attempting to verify resource record %v has been updated with value %v", fqdn, txtValue))

	negativeCacheTTL = 0

	// Fetch DNS records via Cloudflare DNS
	response, err := FetchResourceRecordUsingCloudflareDNS(reqLogger, fqdn)
	if err != nil {
		reqLogger.Error(err, "failed to fetch DNS records")
		return waitPeriod
	}

	// Check for a negative cache result and note its TTL.
	if dnsRCode(response.Status) == dnsRCodeNameError && len(response.Authority) > 0 {
		negativeCacheTTL = response.Authority[0].TTL
		reqLogger.Info(fmt.Sprintf("got a negative cache response with a TTL of %v seconds", negativeCacheTTL))
		// Add 5 seconds to ensure Cloudflare's negative
		// cache record has expired on the next attempt.
		negativeCacheTTL += 5
	}

	// maxNegativeCacheTTL determines what is "reasonable".
	// If the SOA TTL exceeds this, give up immediately.
	if negativeCacheTTL > maxNegativeCacheTTL {
		reqLogger.Info("negative cache TTL is too long; giving up")
		return maxNegativeCacheTTL
	}

	// If the TTL is shorter than the default wait time, disregard.
	if negativeCacheTTL > waitPeriod {
		waitPeriod = negativeCacheTTL
	}

	// If there is no answer field, this is likely an expected NXDOMAIN response.
	if len(response.Answers) == 0 {
		reqLogger.Info("no answers received from cloudflare; likely not propagated")
		return waitPeriod
	}

	// Trim any trailing dot from the answer name and quotes from the data.
	cfName := strings.TrimSuffix(response.Answers[0].Name, ".")
	cfData := strings.Trim(response.Answers[0].Data, "\"")

	// Success if returned record value equals expetect txt value
	if strings.EqualFold(cfName, fqdn) && cfData == txtValue {
		reqLogger.Info(fmt.Sprintf("record %v has been updated to value %v.", fqdn, txtValue))
		return 0
	}

	reqLogger.Info(fmt.Sprintf("unable to verify that resource record %v has been updated to value %v.", fqdn, txtValue))
	return waitPeriod
}

// FetchResourceRecordUsingCloudflareDNS contacts cloudflareDnsOverHttpsEndpoint and returns the json response.
func FetchResourceRecordUsingCloudflareDNS(reqLogger logr.Logger, name string) (*CloudflareResponse, error) {
	requestURL := cloudflareDNSOverHTTPSEndpoint + "?name=" + name + "&type=TXT"

	reqLogger.Info(fmt.Sprintf("cloudflare dns-over-https Request URL: %v", requestURL))

	var request, err = http.NewRequest("GET", requestURL, nil)
	if err != nil {
		reqLogger.Error(err, "error occurred creating new cloudflare dns-over-https request")
		return nil, err
	}

	request.Header.Set("accept", cloudflareRequestContentType)

	netClient := &http.Client{
		Timeout: time.Second * cloudflareRequestTimeout,
	}

	response, err := netClient.Do(request)
	if err != nil {
		reqLogger.Error(err, "error occurred executing request")
		return nil, err
	}
	defer response.Body.Close()

	responseBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		reqLogger.Error(err, "")
		return nil, err
	}

	reqLogger.Info("response from Cloudflare: " + string(responseBody))

	var cloudflareResponse CloudflareResponse

	err = json.Unmarshal(responseBody, &cloudflareResponse)
	if err != nil {
		reqLogger.Error(err, "there was problem parsing the json response from cloudflare.")
		return nil, err
	}

	return &cloudflareResponse, nil
}
