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
	"github.com/pkg/errors"
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
}

// VerifyDnsResourceRecordUpdate is used to export the verifyDnsResourceRecordUpdate
// result back to the caller.
func VerifyDnsResourceRecordUpdate(reqLogger logr.Logger, fqdn string, txtValue string) bool {
	return verifyDnsResourceRecordUpdate(reqLogger, fqdn, txtValue, 1)
}

// After checking attempt count is < then maxAttemptsForDnsPropogationCheck, verifyDnsResourceRecordUpdate
// will validate the challange with the presence of the DNS record with cloudflare. If validated, returns true.
// If the check fails, this function calls itself and iterates the attempt var +1.
func verifyDnsResourceRecordUpdate(reqLogger logr.Logger, fqdn string, txtValue string, attempt int) bool {

	if attempt > maxAttemptsForDnsPropogationCheck {
		errMsg := fmt.Sprintf("unable to verify that resource record %v has been updated to value %v after %v attempts.", fqdn, txtValue, maxAttemptsForDnsPropogationCheck)
		reqLogger.Error(errors.New(errMsg), errMsg)
		return false
	}

	reqLogger.Info(fmt.Sprintf("will query DNS in %v seconds. Attempt %v to verify resource record %v has been updated with value %v", waitTimePeriodDnsPropogationCheck, attempt, fqdn, txtValue))

	time.Sleep(time.Duration(waitTimePeriodDnsPropogationCheck) * time.Second)

	dnsChangesPorpogated, err := ValidateResourceRecordUpdatesUsingCloudflareDns(reqLogger, fqdn, txtValue)
	if err != nil {
		reqLogger.Error(err, "could not validate DNS propagation.")
		return false
	}

	if dnsChangesPorpogated {
		return dnsChangesPorpogated
	}

	return verifyDnsResourceRecordUpdate(reqLogger, fqdn, txtValue, attempt+1)
}

// Contacts cloudflareDnsOverHttpsEndpoint and validates the json response.
func ValidateResourceRecordUpdatesUsingCloudflareDns(reqLogger logr.Logger, name string, value string) (bool, error) {

	requestUrl := cloudflareDnsOverHttpsEndpoint + "?name=" + name + "&type=TXT"

	reqLogger.Info(fmt.Sprintf("cloudflare dns-over-https Request URL: %v", requestUrl))

	var request, err = http.NewRequest("GET", requestUrl, nil)

	if err != nil {
		reqLogger.Error(err, "error occurred creating new cloudflare dns-over-https request")
		return false, err
	}

	request.Header.Set("accept", cloudflareRequestContentType)

	netClient := &http.Client{
		Timeout: time.Second * cloudflareRequestTimeout,
	}

	response, err := netClient.Do(request)
	if err != nil {
		reqLogger.Error(err, "error occurred executing request")
		return false, err
	}
	defer response.Body.Close()

	responseBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		reqLogger.Error(err, "")
		return false, err
	}

	reqLogger.Info("response from Cloudflare: " + string(responseBody))

	var cloudflareResponse CloudflareResponse

	err = json.Unmarshal(responseBody, &cloudflareResponse)
	if err != nil {
		reqLogger.Error(err, "there was problem parsing the json response from cloudflare.")
		return false, err
	}

	if len(cloudflareResponse.Answers) == 0 {
		reqLogger.Error(err, "no answers received from cloudflare")
		return false, errors.New("no answers received from cloudflare")
	}

	if (len(cloudflareResponse.Answers) > 0) && (strings.EqualFold(cloudflareResponse.Answers[0].Name, name+".")) {
		cfData := cloudflareResponse.Answers[0].Data
		// trim quotes from value
		if len(cfData) >= 2 {
			if cfData[0] == '"' && cfData[len(cfData)-1] == '"' {
				cfData = cfData[1 : len(cfData)-1]
			}
		}
		return cfData == value, nil
	}

	return false, errors.New("could not validate DNS propogation for " + name)
}
