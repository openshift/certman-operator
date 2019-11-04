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

package aws

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/go-logr/logr"
	cTypes "github.com/openshift/certman-operator/pkg/clients/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
)

const (
	awsCredsSecretIDKey     = "aws_access_key_id"
	awsCredsSecretAccessKey = "aws_secret_access_key"
	resourceRecordTTL       = 60
)

// awsClient implements the Client interface
type awsClient struct {
	client route53iface.Route53API
}

// searchForHostedZone finds a hostedzone when given an aws client and a domain string
// Returns a hostedzone object
func (c *awsClient) searchForHostedZone(baseDomain string) (hostedZone route53.HostedZone, err error) {
	hostedZoneOutput, err := c.client.ListHostedZones(&route53.ListHostedZonesInput{})
	if err != nil {
		return hostedZone, err
	}

	for _, zone := range hostedZoneOutput.HostedZones {
		if strings.EqualFold(baseDomain, *zone.Name) && !*zone.Config.PrivateZone {
			hostedZone = *zone
		}
	}
	return hostedZone, err
}

// BuildR53Input contructs an Input object for a hostedzone. Contains no recordsets.
func (c *awsClient) buildR53Input(hostedZone string) *route53.ChangeResourceRecordSetsInput {
	input := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{},
		},
		HostedZoneId: &hostedZone,
	}
	return input
}

// createR53TXTRecordChange creates an route53 Change object for a TXT record with a given name
// and a given action to take. Valid actions are strings matching valid route53 ChangeActions.
func (c *awsClient) createR53TXTRecordChange(name *string, action string, value *string) (change route53.Change, err error) {
	// Checking the string 'action' to see if it matches any of the valid route53 acctions.
	// If an incorrect string value is passed this function will exit and raise an error.
	if strings.EqualFold("DELETE", action) {
		action = route53.ChangeActionDelete
	} else if strings.EqualFold("CREATE", action) {
		action = route53.ChangeActionCreate
	} else if strings.EqualFold("UPSERT", action) {
		action = route53.ChangeActionUpsert
	} else {
		return change, fmt.Errorf("Invaild record action passed %v. Must be DELETE, CREATE, or UPSERT", action)
	}
	change = route53.Change{
		Action: aws.String(action),
		ResourceRecordSet: &route53.ResourceRecordSet{
			Name: aws.String(*name),
			ResourceRecords: []*route53.ResourceRecord{
				{
					Value: aws.String(*value),
				},
			},
			TTL:  aws.Int64(resourceRecordTTL),
			Type: aws.String(route53.RRTypeTxt),
		},
	}
	return change, nil
}

func (c *awsClient) AnswerDnsChallenge(reqLogger logr.Logger, acmeChallengeToken string, domain string, cr *certmanv1alpha1.CertificateRequest) (fqdn string, err error) {
	fqdn = cTypes.AcmeChallengeSubDomain + "." + domain
	reqLogger.Info(fmt.Sprintf("fqdn acme challenge domain is %v", fqdn))

	hostedZoneOutput, err := c.client.ListHostedZones(&route53.ListHostedZonesInput{})
	if err != nil {
		reqLogger.Error(err, err.Error())
		return fqdn, err
	}

	baseDomain := cr.Spec.ACMEDNSDomain

	if string(baseDomain[len(baseDomain)-1]) != "." {
		baseDomain = baseDomain + "."
	}

	for _, hostedzone := range hostedZoneOutput.HostedZones {
		if strings.EqualFold(baseDomain, *hostedzone.Name) {
			zone, err := c.client.GetHostedZone(&route53.GetHostedZoneInput{Id: hostedzone.Id})
			if err != nil {
				reqLogger.Error(err, err.Error())
				return fqdn, err
			}

			if !*zone.HostedZone.Config.PrivateZone {
				input := &route53.ChangeResourceRecordSetsInput{
					ChangeBatch: &route53.ChangeBatch{
						Changes: []*route53.Change{
							{
								Action: aws.String(route53.ChangeActionUpsert),
								ResourceRecordSet: &route53.ResourceRecordSet{
									Name: aws.String(fqdn),
									ResourceRecords: []*route53.ResourceRecord{
										{
											Value: aws.String("\"" + acmeChallengeToken + "\""),
										},
									},
									TTL:  aws.Int64(resourceRecordTTL),
									Type: aws.String(route53.RRTypeTxt),
								},
							},
						},
						Comment: aws.String(""),
					},
					HostedZoneId: hostedzone.Id,
				}

				reqLogger.Info(fmt.Sprintf("updating hosted zone %v", hostedzone.Name))

				result, err := c.client.ChangeResourceRecordSets(input)
				if err != nil {
					reqLogger.Error(err, result.GoString(), "fqdn", fqdn)
					return fqdn, err
				}

				return fqdn, nil
			}
		}
	}

	return "", errors.New("unknown error prevented from answering DNS challenge")
}

// ValidateDnsWriteAccess spawns a route53 client to retrieve the baseDomain's hostedZoneOutput
// and attempts to write a test TXT ResourceRecord to it. If successful, will return `true, nil`.
func (c *awsClient) ValidateDnsWriteAccess(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) (bool, error) {
	hostedZoneOutput, err := c.client.ListHostedZones(&route53.ListHostedZonesInput{})
	if err != nil {
		return false, err
	}

	baseDomain := cr.Spec.ACMEDNSDomain

	if string(baseDomain[len(baseDomain)-1]) != "." {
		baseDomain = baseDomain + "."
	}

	for _, hostedzone := range hostedZoneOutput.HostedZones {
		// Find our specific hostedzone
		if strings.EqualFold(baseDomain, *hostedzone.Name) {

			zone, err := c.client.GetHostedZone(&route53.GetHostedZoneInput{Id: hostedzone.Id})
			if err != nil {
				return false, err
			}

			if !*zone.HostedZone.Config.PrivateZone {
				// Build the test record
				input := &route53.ChangeResourceRecordSetsInput{
					ChangeBatch: &route53.ChangeBatch{
						Changes: []*route53.Change{
							{
								Action: aws.String(route53.ChangeActionUpsert),
								ResourceRecordSet: &route53.ResourceRecordSet{
									Name: aws.String("_certman_access_test." + *hostedzone.Name),
									ResourceRecords: []*route53.ResourceRecord{
										{
											Value: aws.String("\"txt_entry\""),
										},
									},
									TTL:  aws.Int64(resourceRecordTTL),
									Type: aws.String(route53.RRTypeTxt),
								},
							},
						},
						Comment: aws.String(""),
					},
					HostedZoneId: hostedzone.Id,
				}

				reqLogger.Info(fmt.Sprintf("updating hosted zone %v", hostedzone.Name))

				// Initiate the Write test
				_, err := c.client.ChangeResourceRecordSets(input)
				if err != nil {
					return false, err
				}

				// After successfull write test clean up the test record and test deletion of that record.
				input.ChangeBatch.Changes[0].Action = aws.String(route53.ChangeActionDelete)
				_, err = c.client.ChangeResourceRecordSets(input)
				if err != nil {
					reqLogger.Error(err, "Error while deleting Write Access record")
					return false, err
				}
				// If Write and Delete are successfull return clean.
				return true, nil
			}
		}
	}

	return false, nil
}

// DeleteAcmeChallengeResourceRecords spawns an AWS client, constructs baseDomain to retrieve the HostedZones. The ResourceRecordSets are
// then requested, if returned and validated, the record is updated to an empty struct to remove the ACME challange.
func (c *awsClient) DeleteAcmeChallengeResourceRecords(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) error {
	hostedZoneOutput, err := c.client.ListHostedZones(&route53.ListHostedZonesInput{})
	if err != nil {
		return err
	}

	baseDomain := cr.Spec.ACMEDNSDomain

	if string(baseDomain[len(baseDomain)-1]) != "." {
		baseDomain = baseDomain + "."
	}

	for _, hostedzone := range hostedZoneOutput.HostedZones {
		if strings.EqualFold(baseDomain, *hostedzone.Name) {
			zone, err := c.client.GetHostedZone(&route53.GetHostedZoneInput{Id: hostedzone.Id})
			if err != nil {
				return err
			}

			if !*zone.HostedZone.Config.PrivateZone {

				for _, domain := range cr.Spec.DnsNames {
					// Format domain strings, no leading '*', must lead with '.'
					domain = strings.TrimPrefix(domain, "*")
					if !strings.HasPrefix(domain, ".") {
						domain = "." + domain
					}
					fqdn := cTypes.AcmeChallengeSubDomain + domain
					fqdnWithDot := fqdn + "."

					reqLogger.Info(fmt.Sprintf("deleting resource record %v", fqdn))

					resp, err := c.client.ListResourceRecordSets(&route53.ListResourceRecordSetsInput{
						HostedZoneId:    aws.String(*hostedzone.Id), // Required
						StartRecordName: aws.String(fqdn),
						StartRecordType: aws.String(route53.RRTypeTxt),
					})

					if err != nil {
						return err
					}
					if len(resp.ResourceRecordSets) > 0 &&
						*resp.ResourceRecordSets[0].Name == fqdnWithDot &&
						*resp.ResourceRecordSets[0].Type == route53.RRTypeTxt &&
						len(resp.ResourceRecordSets[0].ResourceRecords) > 0 {
						for _, rr := range resp.ResourceRecordSets[0].ResourceRecords {
							input := &route53.ChangeResourceRecordSetsInput{
								ChangeBatch: &route53.ChangeBatch{
									Changes: []*route53.Change{
										{
											Action: aws.String(route53.ChangeActionDelete),
											ResourceRecordSet: &route53.ResourceRecordSet{
												Name: aws.String(fqdn),
												ResourceRecords: []*route53.ResourceRecord{
													{
														Value: aws.String(*rr.Value),
													},
												},
												TTL:  aws.Int64(resourceRecordTTL),
												Type: aws.String(route53.RRTypeTxt),
											},
										},
									},
									Comment: aws.String(""),
								},
								HostedZoneId: hostedzone.Id,
							}

							reqLogger.Info(fmt.Sprintf("updating hosted zone %v", hostedzone.Name))

							result, err := c.client.ChangeResourceRecordSets(input)
							if err != nil {
								reqLogger.Error(err, result.GoString())
								return nil
							}
						}
					}
				}
			}
		}
	}

	return nil
}

// DeleteAllAcmeChallengeResourceRecords to delete all records in a hosted zone that begin with the prefix defined by the const acmeChallengeSubDomain
func (c *awsClient) DeleteAllAcmeChallengeResourceRecords(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) error {
	// This function is for record clean up. If we are unable to find the records to delete them we silently accept these errors
	// without raising an error. If the record was already deleted that's fine.

	// Make sure that the domain ends with a dot.
	baseDomain := cr.Spec.ACMEDNSDomain
	if string(baseDomain[len(baseDomain)-1]) != "." {
		baseDomain = baseDomain + "."
	}

	// Calls function to get the hostedzone of the domain of our CertificateRequest
	hostedzone, err := c.searchForHostedZone(baseDomain)
	if err != nil {
		reqLogger.Error(err, "Unable to find appropriate hostedzone.")
		return err
	}

	// Get a list of RecordSets from our hostedzone that match our search criteria
	// Criteria - record name starts with our acmechallenge prefix, record is a TXT type
	listRecordSets, err := c.client.ListResourceRecordSets(&route53.ListResourceRecordSetsInput{
		HostedZoneId:    aws.String(*hostedzone.Id), // Required
		StartRecordName: aws.String(cTypes.AcmeChallengeSubDomain + "*"),
		StartRecordType: aws.String(route53.RRTypeTxt),
	})
	if err != nil {
		reqLogger.Error(err, "Unable to retrieve acme records for hostedzone.")
		return err
	}

	// Construct an Input object and populate it with records we intend to change
	// In this case we're adding all acme challenge records found above and setting their action to Delete
	input := c.buildR53Input(*hostedzone.Id)
	for _, record := range listRecordSets.ResourceRecordSets {
		if strings.Contains(*record.Name, cTypes.AcmeChallengeSubDomain) {
			change, err := c.createR53TXTRecordChange(record.Name, route53.ChangeActionDelete, record.ResourceRecords[0].Value)
			if err != nil {
				reqLogger.Error(err, "Error creating record change object")
			}
			input.ChangeBatch.Changes = append(input.ChangeBatch.Changes, &change)
		}
	}

	// Sent the completed Input object to Route53 to delete the acme records
	result, err := c.client.ChangeResourceRecordSets(input)
	if err != nil {
		reqLogger.Error(err, result.GoString())
		return nil
	}

	return nil
}

// NewClient returns an awsclient.Client object to the caller. If NewClient is passed a non-null
// secretName, an attempt to retrieve the secret from the namespace argument will be performed.
// AWS credentials are returned as these secrets and a new session is initiated prior to returning
// a client. If secrets fail to return, the IAM role of the masters is used to create a
// new session for the client.
func NewClient(kubeClient client.Client, secretName, namespace, region string) (*awsClient, error) {
	awsConfig := &aws.Config{Region: aws.String(region)}
	if secretName != "" {
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

		accessKeyID, ok := secret.Data[awsCredsSecretIDKey]
		if !ok {
			return nil, fmt.Errorf("AWS credentials secret %v did not contain key %v",
				secretName, awsCredsSecretIDKey)
		}

		secretAccessKey, ok := secret.Data[awsCredsSecretAccessKey]
		if !ok {
			return nil, fmt.Errorf("AWS credentials secret %v did not contain key %v",
				secretName, awsCredsSecretAccessKey)
		}

		awsConfig.Credentials = credentials.NewStaticCredentials(
			strings.Trim(string(accessKeyID), "\n"),
			strings.Trim(string(secretAccessKey), "\n"),
			"",
		)
	}

	//// Otherwise default to relying on the IAM role of the masters where the actuator is running:
	s, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, err
	}

	c := &awsClient{
		client: route53.New(s),
	}

	return c, err
}
