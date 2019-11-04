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
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/go-logr/logr"
	option "google.golang.org/api/option"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cTypes "github.com/openshift/certman-operator/pkg/clients/types"
	"github.com/openshift/certman-operator/pkg/controller/utils"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	dnsv1 "google.golang.org/api/dns/v1"
)

const (
	resourceRecordTTL = 60
)

// client implements the Client interface
type gcpClient struct {
	client  dnsv1.Service
	project string
}

//func (c *gcpClient) ListHostedZones() (*cTypes.ManagedZoneList, error) {
//	results, err := c.client.ManagedZones.List(c.project).Do()
//	if err != nil {
//		return nil, err
//	}
//	var list cTypes.ManagedZoneList
//	for _, z := range results.ManagedZones {
//		list.ManagedZones = append(list.ManagedZones, cTypes.ManagedZone{
//			Name:    z.Name,
//			DNSName: z.DnsName,
//		})
//	}
//	return &list, nil
//}

func (c *gcpClient) ChangeResourceRecordSets(input *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error) {
	return nil, nil
}

func (c *gcpClient) ListResourceRecordSets(input *route53.ListResourceRecordSetsInput) (*route53.ListResourceRecordSetsOutput, error) {
	return nil, nil
}

// SearchForHostedZone finds a hostedzone when given an aws client and a domain string
// Returns a hostedzone object
func (c *gcpClient) SearchForHostedZone(baseDomain string) (hostedZone route53.HostedZone, err error) {
	return route53.HostedZone{}, nil
}

// BuildR53Input contructs an Input object for a hostedzone. Contains no recordsets.
func (c *gcpClient) BuildR53Input(hostedZone string) *route53.ChangeResourceRecordSetsInput {
	return &route53.ChangeResourceRecordSetsInput{}
}

// CreateR53TXTRecordChange creates an route53 Change object for a TXT record with a given name
// and a given action to take. Valid actions are strings matching valid route53 ChangeActions.
func (c *gcpClient) createR53TXTRecordChange(name *string, action string, value *string) (change route53.Change, err error) {
	return route53.Change{}, nil
}

func (c *gcpClient) AnswerDnsChallenge(reqLogger logr.Logger, acmeChallengeToken string, domain string, cr *certmanv1alpha1.CertificateRequest) (fqdn string, err error) {
	fqdn = cTypes.AcmeChallengeSubDomain + "." + domain
	reqLogger.Info(fmt.Sprintf("fqdn acme challenge domain is %v", fqdn))

	return fqdn, errors.New("unknown error prevented from answering DNS challenge")
}

// ValidateDnsWriteAccess spawns a route53 client to retrieve the baseDomain's hostedZoneOutput
// and attempts to write a test TXT ResourceRecord to it. If successful, will return `true, nil`.
func (c *gcpClient) ValidateDnsWriteAccess(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) (bool, error) {
	return false, nil
}

// DeleteAllAcmeChallengeResourceRecords to delete all records in a hosted zone that begin with the prefix defined by the const acmeChallengeSubDomain
func (c *gcpClient) DeleteAllAcmeChallengeResourceRecords(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) error {
	// This function is for record clean up. If we are unable to find the records to delete them we silently accept these errors
	// without raising an error. If the record was already deleted that's fine.

	return nil
}

func (c *gcpClient) DeleteAcmeChallengeResourceRecords(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) error {
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
