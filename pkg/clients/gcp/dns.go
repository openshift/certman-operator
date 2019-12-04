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
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	dnsv1 "google.golang.org/api/dns/v1"
	"google.golang.org/api/googleapi"
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

func (c *gcpClient) AnswerDNSChallenge(reqLogger logr.Logger, acmeChallengeToken string, domain string, cr *certmanv1alpha1.CertificateRequest) (fqdn string, err error) {
	fqdn = fmt.Sprintf("%s.%s", cTypes.AcmeChallengeSubDomain, domain)
	reqLogger.Info(fmt.Sprintf("fqdn acme challenge domain is %v", fqdn))

	// TODO: validate this logic post hive DNS GCP support merges
	output, err := c.client.ManagedZones.List(c.project).Do()
	if err != nil {
		return "", err
	}

	baseDomain := cr.Spec.ACMEDNSDomain
	if !strings.HasSuffix(baseDomain, ".") {
		baseDomain = baseDomain + "."
	}

	var fqdnName string
	if !strings.HasSuffix(fqdn, ".") {
		fqdnName = fqdn + "."
	} else {
		fqdnName = fqdn
	}

	// TODO: This duplicate code as AnswerDnsChallenge. Collapse
	for _, zoneInfo := range output.ManagedZones {
		// Find our specific hostedzone
		if strings.EqualFold(baseDomain, zoneInfo.DnsName) {
			zone, err := c.client.ManagedZones.Get(c.project, zoneInfo.Name).Do()
			if err != nil {
				return "", err
			}

			change := []*dnsv1.ResourceRecordSet{
				{
					Kind:    "dns#resourceRecordSet",
					Name:    fqdnName,
					Rrdatas: []string{fmt.Sprintf("\"%s\"", acmeChallengeToken)},
					Ttl:     int64(resourceRecordTTL),
					Type:    "TXT",
				},
			}

			input := &dnsv1.Change{
				Additions: change,
			}
			_, err = c.client.Changes.Create(c.project, zone.Name, input).Do()
			if err != nil {
				ae, ok := err.(*googleapi.Error)
				// google uses 409 for "already exists"
				if ok && ae.Code == http.StatusConflict {
					return fqdn, nil
				}
				return "", err
			}
		}
	}
	return fqdn, nil
}

// searchForManagedZone finds a managedZone when given an aws client and a domain string
// Returns a managed zone object
func (c *gcpClient) searchForManagedZone(baseDomain string) (managedZone dnsv1.ManagedZone, err error) {
	output, err := c.client.ManagedZones.List(c.project).Do()
	if err != nil {
		return managedZone, err
	}

	for _, zone := range output.ManagedZones {
		if strings.EqualFold(baseDomain, zone.DnsName) && zone.PrivateVisibilityConfig == nil {
			managedZone = *zone
		}
	}
	return managedZone, err
}

// ValidateDNSWriteAccess client to retrieve the baseDomain's hostedZoneOutput
// and attempts to write a test TXT ResourceRecord to it. If successful, will return `true, nil`.
func (c *gcpClient) ValidateDNSWriteAccess(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) (bool, error) {
	output, err := c.client.ManagedZones.List(c.project).Do()
	if err != nil {
		return false, err
	}

	baseDomain := cr.Spec.ACMEDNSDomain
	if !strings.HasSuffix(baseDomain, ".") {
		baseDomain = baseDomain + "."
	}

	// TODO: This duplicate code as AnswerDnsChallenge. Collapse
	for _, zoneInfo := range output.ManagedZones {
		// Find our specific hostedzone
		if strings.EqualFold(baseDomain, zoneInfo.DnsName) {
			zone, err := c.client.ManagedZones.Get(c.project, zoneInfo.Name).Do()
			if err != nil {
				return false, err
			}

			change := []*dnsv1.ResourceRecordSet{
				{
					Kind:    "dns#resourceRecordSet",
					Name:    fmt.Sprintf("_certman_access_test.%s", zone.DnsName),
					Rrdatas: []string{"txt_entry"},
					Ttl:     int64(resourceRecordTTL),
					Type:    "TXT",
				},
			}

			// Build the test record
			input := &dnsv1.Change{
				Additions: change,
			}
			_, err = c.client.Changes.Create(c.project, zone.Name, input).Do()
			if err != nil {
				// TODO: Error not found only
				// TODO: Handle already exist error
				return false, err
			}

			input = &dnsv1.Change{
				Deletions: change,
			}
			_, err = c.client.Changes.Create(c.project, zone.Name, input).Do()
			if err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

// DeleteAcmeChallengeResourceRecords to delete all records in a hosted zone that begin with the prefix defined by the const acmeChallengeSubDomain
func (c *gcpClient) DeleteAcmeChallengeResourceRecords(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) error {
	// This function is for record clean up. If we are unable to find the records to delete them we silently accept these errors
	// without raising an error. If the record was already deleted that's fine.

	// Make sure that the domain ends with a dot.
	baseDomain := cr.Spec.ACMEDNSDomain
	if !strings.HasSuffix(baseDomain, ".") {
		baseDomain = baseDomain + "."
	}

	// Calls function to get the hostedzone of the domain of our CertificateRequest
	managedZone, err := c.searchForManagedZone(baseDomain)
	if err != nil {
		reqLogger.Error(err, "Unable to find appropriate managedzone.")
		return err
	}

	var change []*dnsv1.ResourceRecordSet
	// Get a list of RecordSets from our hostedzone that match our search criteria
	// Criteria - record name starts with our acmechallenge prefix, record is a TXT type
	req := c.client.ResourceRecordSets.List(c.project, managedZone.Name)
	if err := req.Pages(context.Background(), func(page *dnsv1.ResourceRecordSetsListResponse) error {
		for _, resourceRecordSet := range page.Rrsets {
			if strings.Contains(resourceRecordSet.Name, cTypes.AcmeChallengeSubDomain) &&
				resourceRecordSet.Type == "TXT" {
				change = append(change, resourceRecordSet)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	_, err = c.client.Changes.Create(c.project, managedZone.Name, &dnsv1.Change{
		Deletions: change,
	}).Do()
	if err != nil {
		return err
	}

	return nil
}

// NewClient reuturn new GCP DNS client
func NewClient(kubeClient client.Client, secretName, namespace, project string) (*gcpClient, error) {
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
		project: project,
	}, nil
}
