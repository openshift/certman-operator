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

package azure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2018-05-01/dns"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	cTypes "github.com/openshift/certman-operator/pkg/clients/types"
)

const (
	resourceRecordTTL = 60
	azureCredsSPKey   = "osServicePrincipal.json"
)

// client implements the Client interface
type azureClient struct {
	resourceGroupName string
	recordSetsClient  *dns.RecordSetsClient
	zonesClient       *dns.ZonesClient
}

func (c *azureClient) createTxtRecord(reqLogger logr.Logger, recordKey string, recordValue string, zoneName string) (result dns.RecordSet, err error) {
	recordSetProperties := &dns.RecordSet{
		RecordSetProperties: &dns.RecordSetProperties{
			TTL: to.Int64Ptr(resourceRecordTTL),
			TxtRecords: &[]dns.TxtRecord{
				{
					Value: &[]string{
						recordValue,
					},
				},
			},
		},
	}
	reqLogger.Info(fmt.Sprintf("updating hosted zone %v", zoneName))
	return c.recordSetsClient.CreateOrUpdate(context.TODO(), c.resourceGroupName, zoneName, recordKey, dns.TXT, *recordSetProperties, "", "")
}

func (c *azureClient) generateTxtRecordName(domain string, rootDomain string) string {
	// Remove base domain
	domain = strings.TrimSuffix(domain, rootDomain)

	//Remove . at the end if present
	domain = strings.TrimSuffix(domain, ".")

	//Remove * at the begining if present
	domain = strings.TrimPrefix(domain, "*")

	//Remove . at the begining if present
	domain = strings.TrimPrefix(domain, ".")

	return fmt.Sprintf("%s.%s", cTypes.AcmeChallengeSubDomain, domain)
}

func (c *azureClient) AnswerDNSChallenge(reqLogger logr.Logger, acmeChallengeToken string, domain string, cr *certmanv1alpha1.CertificateRequest) (fqdn string, err error) {
	reqLogger.Info(fmt.Sprintf("Cr domain is %v", cr.Spec.ACMEDNSDomain))
	reqLogger.Info(fmt.Sprintf("Domain is %v", domain))

	zone, err := c.zonesClient.Get(context.TODO(), c.resourceGroupName, cr.Spec.ACMEDNSDomain)
	if err != nil {
		reqLogger.Error(err, fmt.Sprintf("Error getting dns zone %v", cr.Spec.ACMEDNSDomain))
		return "", err
	}

	txtRecordName := c.generateTxtRecordName(domain, *zone.Name)
	recordSet, err := c.createTxtRecord(reqLogger, txtRecordName, acmeChallengeToken, *zone.Name)

	loc := (*(*recordSet.TxtRecords)[0].Value)[0]
	reqLogger.Info(fmt.Sprintf("record set added: %v", loc))
	if err != nil {
		reqLogger.Error(err, "Error adding acme challenge DNS entry")
		return "", err
	}
	return txtRecordName + "." + cr.Spec.ACMEDNSDomain, nil
}

func (c *azureClient) DeleteAcmeChallengeResourceRecords(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) error {
	zone, err := c.zonesClient.Get(context.TODO(), c.resourceGroupName, cr.Spec.ACMEDNSDomain)
	if err != nil {
		reqLogger.Error(err, fmt.Sprintf("Error getting dns zone %v", cr.Spec.ACMEDNSDomain))
		return err
	}

	for _, dnsName := range cr.Spec.DnsNames {
		txtRecordName := c.generateTxtRecordName(dnsName, cr.Spec.ACMEDNSDomain)
		reqLogger.Info(fmt.Sprintf("Deleting record set %v", txtRecordName))
		_, err = c.recordSetsClient.Delete(context.TODO(), c.resourceGroupName, *zone.Name, txtRecordName, dns.TXT, "")

		if err != nil {
			reqLogger.Error(err, "Error deleting acme challenge DNS entry")
			return err
		}
	}

	return nil
}

// ValidateDnsWriteAccess spawns a zones client to retrieve the baseDomain's hostedZoneOutput
// and attempts to write a test TXT ResourceRecord to it. If successful, will return `true, nil`.
func (c *azureClient) ValidateDNSWriteAccess(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) (bool, error) {

	zone, err := c.zonesClient.Get(context.TODO(), c.resourceGroupName, cr.Spec.ACMEDNSDomain)

	if err != nil {
		reqLogger.Error(err, fmt.Sprintf("Error getting dns zone %v", cr.Spec.ACMEDNSDomain))
		return false, err
	}

	recordKey := "_certman_access_test." + *zone.Name

	if zone.ZoneType != "Private" {
		// Build the test record
		_, err := c.createTxtRecord(reqLogger, recordKey, "\"txt_entry\"", *zone.Name)

		if err != nil {
			return false, err
		}

		// After successfull write test clean up the test record and test deletion of that record.
		_, err = c.recordSetsClient.Delete(context.TODO(), c.resourceGroupName, *zone.Name, recordKey, dns.TXT, "")

		if err != nil {
			reqLogger.Error(err, "Error while deleting Write Access record")
			return false, err
		}
		// If Write and Delete are successfull return clean.
		return true, nil
	}

	reqLogger.Error(err, "Private DNS zone is not allowed")
	return false, nil
}

// NewClient returns new Azure DNS client
func NewClient(kubeClient client.Client, secretName string, namespace string, resourceGroupName string) (*azureClient, error) {
	secret := &corev1.Secret{}

	err := kubeClient.Get(context.TODO(),
		types.NamespacedName{
			Name:      secretName,
			Namespace: namespace,
		},
		secret)

	if err != nil {
		return nil, err
	}

	var authMap map[string]string
	if secret.Data[azureCredsSPKey] == nil {
		return nil, fmt.Errorf("Secret %v doesn't have key %v", secretName, azureCredsSPKey)
	}

	if err := json.Unmarshal(secret.Data[azureCredsSPKey], &authMap); err != nil {
		return nil, err
	}

	clientID, ok := authMap["clientId"]
	if !ok {
		return nil, errors.New("missing clientId in auth")
	}
	clientSecret, ok := authMap["clientSecret"]
	if !ok {
		return nil, errors.New("missing clientSecret in auth")
	}
	tenantID, ok := authMap["tenantId"]
	if !ok {
		return nil, errors.New("missing tenantId in auth")
	}
	subscriptionID, ok := authMap["subscriptionId"]
	if !ok {
		return nil, errors.New("missing subscriptionId in auth")
	}
	config := auth.NewClientCredentialsConfig(clientID, clientSecret, tenantID)

	authorizer, err := config.Authorizer()
	if err != nil {
		return nil, err
	}

	recordSetsClient := dns.NewRecordSetsClientWithBaseURI(azure.PublicCloud.ResourceManagerEndpoint, subscriptionID)
	recordSetsClient.Authorizer = authorizer

	zonesClient := dns.NewZonesClientWithBaseURI(azure.PublicCloud.ResourceManagerEndpoint, subscriptionID)
	zonesClient.Authorizer = authorizer

	return &azureClient{
		resourceGroupName: resourceGroupName,
		recordSetsClient:  &recordSetsClient,
		zonesClient:       &zonesClient,
	}, nil
}
