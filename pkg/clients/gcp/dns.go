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

package gcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	dnsv1 "google.golang.org/api/dns/v1"
	option "google.golang.org/api/option"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	cTypes "github.com/openshift/certman-operator/pkg/clients/types"
	"github.com/openshift/certman-operator/pkg/controller/utils"
)

const (
	resourceRecordTTL = 60
)

// client implements the Client interface
type gcpClient struct {
	client  dnsv1.Service
	project string
}

func (c *gcpClient) GetDNSName() string {
	return "Cloud DNS"
}

func (c *gcpClient) AnswerDNSChallenge(reqLogger logr.Logger, acmeChallengeToken string, domain string, cr *certmanv1alpha1.CertificateRequest) (fqdn string, err error) {
	fqdn = fmt.Sprintf("%s.%s", cTypes.AcmeChallengeSubDomain, domain)
	reqLogger.Info(fmt.Sprintf("fqdn acme challenge domain is %v", fqdn))

	var fqdnName string
	if !strings.HasSuffix(fqdn, ".") {
		fqdnName = fqdn + "."
	} else {
		fqdnName = fqdn
	}

	// Calls function to get the hostedzone of the domain of our CertificateRequest
	zone, err := c.getManagedZone(cr.Spec.ACMEDNSDomain)
	if err != nil {
		reqLogger.Error(err, "Unable to find appropriate managedzone")
		return "", err
	}

	dnsRecord := &dnsv1.ResourceRecordSet{
		Kind:    "dns#resourceRecordSet",
		Name:    fqdnName,
		Rrdatas: []string{fmt.Sprintf("\"%s\"", acmeChallengeToken)},
		Ttl:     int64(resourceRecordTTL),
		Type:    "TXT",
	}

	// add/update challenge record
	err = c.upsertDnsRecord(zone, dnsRecord)
	if err != nil {
		return "", err
	}
	return fqdn, nil
}

// ValidateDNSWriteAccess client to retrieve the baseDomain's hostedZoneOutput
// and attempts to write a test TXT ResourceRecord to it. If successful, will return `true, nil`.
func (c *gcpClient) ValidateDNSWriteAccess(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) (bool, error) {
	var err error

	// Calls function to get the hostedzone of the domain of our CertificateRequest
	zone, err := c.getManagedZone(cr.Spec.ACMEDNSDomain)
	if err != nil {
		reqLogger.Error(err, "Unable to find appropriate managedzone")
		return false, err
	}

	dnsRecord := &dnsv1.ResourceRecordSet{
		Kind:    "dns#resourceRecordSet",
		Name:    fmt.Sprintf("%s.%s", cTypes.WriteValidationSubDomain, zone.DnsName),
		Rrdatas: []string{"txt_entry"},
		Ttl:     int64(resourceRecordTTL),
		Type:    "TXT",
	}

	// test if we can add a record
	err = c.upsertDnsRecord(zone, dnsRecord)
	if err != nil {
		return false, err
	}

	// clean up record
	err = c.deleteDnsRecords(zone, []*dnsv1.ResourceRecordSet{dnsRecord})
	if err != nil {
		return false, err
	}

	return true, nil
}

// DeleteAcmeChallengeResourceRecords to delete all records in a hosted zone that begin with the prefix defined by the const acmeChallengeSubDomain
func (c *gcpClient) DeleteAcmeChallengeResourceRecords(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) error {
	// This function is for record clean up. If we are unable to find the records to delete them we silently accept these errors
	// without raising an error. If the record was already deleted that's fine.

	// Calls function to get the hostedzone of the domain of our CertificateRequest
	zone, err := c.getManagedZone(cr.Spec.ACMEDNSDomain)
	if err != nil {
		reqLogger.Error(err, "Unable to find appropriate managedzone")
		return err
	}

	var changes []*dnsv1.ResourceRecordSet
	// Get a list of RecordSets from our hostedzone that match our search criteria
	// Criteria - record name starts with our acmechallenge prefix or the write testing prefix, record is a TXT type
	req := c.client.ResourceRecordSets.List(c.project, zone.Name)
	if err := req.Pages(context.Background(), func(page *dnsv1.ResourceRecordSetsListResponse) error {
		for _, resourceRecordSet := range page.Rrsets {
			if resourceRecordSet.Type == "TXT" {
				if strings.Contains(resourceRecordSet.Name, cTypes.AcmeChallengeSubDomain) || strings.Contains(resourceRecordSet.Name, cTypes.WriteValidationSubDomain) {
					changes = append(changes, resourceRecordSet)
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}

	// clean up records
	err = c.deleteDnsRecords(zone, changes)
	if err != nil {
		return err
	}

	return nil
}

// NewClient reuturn new GCP DNS client
func NewClient(kubeClient client.Client, secretName, namespace string) (*gcpClient, error) {
	ctx := context.Background()
	secret := &corev1.Secret{}
	err := kubeClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: secretName}, secret)
	if err != nil {
		return nil, err
	}

	config, err := utils.GetCredentialsJSON(kubeClient, types.NamespacedName{Namespace: namespace, Name: secretName})
	if err != nil {
		return nil, err
	}

	service, err := dnsv1.NewService(context.Background(), option.WithCredentials(config))
	if err != nil {
		return nil, err
	}

	return &gcpClient{
		client:  *service,
		project: config.ProjectID,
	}, nil
}

// getManagedZone finds and returns the ManagedZone matching the baseDomain provided
func (c *gcpClient) getManagedZone(baseDomain string) (*dnsv1.ManagedZone, error) {
	// list DNS zones in the project
	zoneList, err := c.client.ManagedZones.List(c.project).Do()
	if err != nil {
		return nil, err
	}

	// ensure base domain has a trailing dot
	if !strings.HasSuffix(baseDomain, ".") {
		baseDomain = baseDomain + "."
	}

	// loop through all zones, and return the matching zone
	for _, zone := range zoneList.ManagedZones {
		// Find our specific zone
		if strings.EqualFold(baseDomain, zone.DnsName) && zone.Visibility == "public" {
			// always return from this, as we only expect one managed zone from the baseDomain
			return c.client.ManagedZones.Get(c.project, zone.Name).Do()
		}
	}

	return nil, fmt.Errorf("unable to find zone matching baseDomain: %s", baseDomain)
}

// upsertDnsRecord takes a DNS record set, and ensures that it exists
func (c *gcpClient) upsertDnsRecord(zone *dnsv1.ManagedZone, record *dnsv1.ResourceRecordSet) error {
	var err error

	// build the change
	change := &dnsv1.Change{
		Additions: []*dnsv1.ResourceRecordSet{record},
	}

	// get list of current records in the zone
	res, err := c.client.ResourceRecordSets.List(c.project, zone.Name).Do()
	if err != nil {
		return fmt.Errorf("Error retrieving record sets for %q: %s", zone.Name, err)
	}

	// check if record already exists
	var deletions []*dnsv1.ResourceRecordSet

	for _, existingRecord := range res.Rrsets {
		if existingRecord.Type != record.Type || existingRecord.Name != record.Name {
			continue
		}
		deletions = append(deletions, existingRecord)
	}

	// if there is a deletion, then add the deletion set to the change
	if len(deletions) > 0 {
		change.Deletions = deletions
	}

	// submit change
	_, err = c.client.Changes.Create(c.project, zone.Name, change).Do()
	if err != nil {
		return err
	}

	return nil
}

// deleteDnsRecords takes a slice of DNS record sets, and deletes them
func (c *gcpClient) deleteDnsRecords(zone *dnsv1.ManagedZone, records []*dnsv1.ResourceRecordSet) error {
	var err error

	// build the change
	change := &dnsv1.Change{
		Deletions: records,
	}

	// submit change
	_, err = c.client.Changes.Create(c.project, zone.Name, change).Do()
	if err != nil {
		return err
	}

	return nil
}
