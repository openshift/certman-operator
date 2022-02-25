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
	"github.com/openshift/certman-operator/pkg/localmetrics"
	"github.com/pkg/errors"
)

type DnsServerQuestion struct {
	Name string `json:"name"`
	Type int    `json:"type"`
}

type DnsServerAnswer struct {
	Name string `json:"name"`
	Type int    `json:"type"`
	TTL  int    `json:"TTL"`
	Data string `json:"data"`
}

type DnsServerResponse struct {
	Status    int                 `json:"Status"`
	TC        bool                `json:"TC"`
	RC        bool                `json:"RC"`
	RA        bool                `json:"RA"`
	AD        bool                `json:"AD"`
	CD        bool                `json:"CD"`
	Questions []DnsServerQuestion `json:"Question"`
	Answers   []DnsServerAnswer   `json:"Answer"`
	Authority []DnsServerAnswer   `json:"Authority"`
}

// VerifyDnsResourceRecordUpdate verifies the presence of a TXT record with Cloudflare DNS.
func VerifyDnsResourceRecordUpdate(reqLogger logr.Logger, fqdn string, txtValue string) bool {
	var negativeCacheTTL int

	for attempt := 1; attempt < maxAttemptsForDnsPropagationCheck; attempt++ {
		var err error

		// Sleep before querying Cloudflare DNS.  If the previous attempt returned
		// a negative cache result, honor its TTL (within reason).  Otherwise wait
		// for a predetermined duration.
		sleepDuration := waitTimePeriodDnsPropagationCheck
		if attempt > 1 && negativeCacheTTL > 0 {
			// maxNegativeCacheTTL determines what is "reasonable".
			// If the SOA TTL exceeds this, give up immediately.
			if negativeCacheTTL > maxNegativeCacheTTL {
				reqLogger.Info("negative cache TTL is too long; giving up")
				return false
			}
			// If the TTL is shorter than the default wait time, disregard.
			if negativeCacheTTL > sleepDuration {
				sleepDuration = negativeCacheTTL
			}
		}

		reqLogger.Info(fmt.Sprintf("attempt %v to verify resource record %v has been updated with value %v", attempt, fqdn, txtValue))

		reqLogger.Info(fmt.Sprintf("will query DNS in %v seconds", sleepDuration))
		time.Sleep(time.Duration(sleepDuration) * time.Second)

		negativeCacheTTL = 0

		response, err := TryFetchResourceRecordUsingPublicDNS(reqLogger, fqdn)
		if err != nil {
			reqLogger.Error(err, "failed to fetch DNS records")
			continue
		}

		// Check for a negative cache result and note its TTL.
		if dnsRCode(response.Status) == dnsRCodeNameError && len(response.Authority) > 0 {
			negativeCacheTTL = response.Authority[0].TTL
			reqLogger.Info(fmt.Sprintf("got a negative cache response with a TTL of %v seconds", negativeCacheTTL))
			// Add 5 seconds to ensure Cloudflare's negative
			// cache record has expired on the next attempt.
			negativeCacheTTL += 5
			continue
		}

		// If there is no answer field, this is likely an expected NXDOMAIN response.
		if len(response.Answers) == 0 {
			reqLogger.Info("no answers received from cloudflare; likely not propagated")
			continue
		}

		// Trim any trailing dot from the answer name and quotes from the data.
		cfName := strings.TrimSuffix(response.Answers[0].Name, ".")
		cfData := strings.Trim(response.Answers[0].Data, "\"")

		if strings.EqualFold(cfName, fqdn) && cfData == txtValue {
			return true
		}

		reqLogger.Info("could not validate DNS propagation for " + fqdn)
	}

	errMsg := fmt.Sprintf("unable to verify that resource record %v has been updated to value %v after %v attempts.", fqdn, txtValue, maxAttemptsForDnsPropagationCheck)
	reqLogger.Error(errors.New(errMsg), errMsg)
	return false
}

//Added TryFetchResourceRecordUsingPublicDNS which will run FetchResourceRecordUsingPublicDNS with cloudflareDNSOverHttpsEndpoint first,
// and if that call fails (for instance, if cloudflare is down) will run FetchResourceRecordUsingPublicDNS with googleDNSOverHttpsEndpoint
func TryFetchResourceRecordUsingPublicDNS(reqLogger logr.Logger, name string) (*DnsServerResponse, error) {


	response, err := FetchResourceRecordUsingPublicDNS(reqLogger, name, cloudflareDNSOverHttpsEndpoint)
	if err != nil {
		response, err = FetchResourceRecordUsingPublicDNS(reqLogger, name, dnsOverHttpsEndpoint)
	}
	if err != nil {
		localmetrics.IncrementDnsErrorCount()
	}
	return response, err
}

// FetchResourceRecordUsingPublicDNS contacts dnsOverHttpsEndpoint and returns the json response.
func FetchResourceRecordUsingPublicDNS(reqLogger logr.Logger, name string, dnsOverHttpsEndpoint string) (*DnsServerResponse, error) {
	requestUrl := dnsOverHttpsEndpoint + "?name=" + name + "&type=TXT"

	reqLogger.Info(fmt.Sprintf("public DNS dns-over-https Request URL: %v", requestUrl))

	var request, err = http.NewRequest("GET", requestUrl, nil)
	if err != nil {
		reqLogger.Error(err, "error occurred creating new dns-over-https request")
		return nil, err
	}

	request.Header.Set("accept", dnsServerRequestContentType)

	netClient := &http.Client{
		Timeout: time.Second * dnsServerRequestTimeout,
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

	if 400 <= response.StatusCode && response.StatusCode <= 499 {
		reqLogger.Info("Client Error: " + response.Status)
	} else if 500 <= response.StatusCode && response.StatusCode <= 599 {
		reqLogger.Info("Server Error: " + response.Status)
	} else {
		reqLogger.Info("response Status from the public DNS Server : " + response.Status)
	}

	reqLogger.Info("response from the public DNS Server: " + string(responseBody))

	var dnsServerResponse DnsServerResponse

	err = json.Unmarshal(responseBody, &dnsServerResponse)
	if err != nil {
		reqLogger.Error(err, "there was problem parsing the json response from the public DNS Server.")
		return nil, err
	}

	return &dnsServerResponse, nil
}
