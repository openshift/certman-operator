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
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/go-logr/logr"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
)

// AnswerDnsChallenge constructs a fqdn from acmeChallengeSubDomain and domain. An route53 AWS client is then spawned to retrieve HostedZones.
// If zone.HostedZone.Config.PrivateZone is not defined, an attempt to populate this zone will be performed. Only upon success is `fqdn, nil` returned.
func (r *ReconcileCertificateRequest) AnswerDnsChallenge(reqLogger logr.Logger, acmeChallengeToken string, domain string, cr *certmanv1alpha1.CertificateRequest) (fqdn string, err error) {

	fqdn = acmeChallengeSubDomain + "." + domain

	reqLogger.Info(fmt.Sprintf("fqdn acme challenge domain is %v", fqdn))

	r53svc, err := r.getAwsClient(cr)
	if err != nil {
		reqLogger.Error(err, err.Error())
		return fqdn, err
	}

	hostedZoneOutput, err := r53svc.ListHostedZones(&route53.ListHostedZonesInput{})
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
			zone, err := r53svc.GetHostedZone(&route53.GetHostedZoneInput{Id: hostedzone.Id})
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

				result, err := r53svc.ChangeResourceRecordSets(input)
				if err != nil {
					reqLogger.Error(err, result.GoString(), "fqdn", fqdn)
					return fqdn, err
				}

				return fqdn, nil
			}
		}
	}

	return fqdn, errors.New("unknown error prevented from answering DNS challenge")
}

// ValidateDnsWriteAccess spawns a route53 client to retrieve the baseDomain's hostedZoneOutput
// and attempts to write a test TXT ResourceRecord to it. If successful, will return `true, nil`.
func (r *ReconcileCertificateRequest) ValidateDNSWriteAccess(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) (bool, error) {

	r53svc, err := r.getAwsClient(cr)
	if err != nil {
		return false, err
	}

	hostedZoneOutput, err := r53svc.ListHostedZones(&route53.ListHostedZonesInput{})
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

			zone, err := r53svc.GetHostedZone(&route53.GetHostedZoneInput{Id: hostedzone.Id})
			if err != nil {
				return false, err
			}

			if !*zone.HostedZone.Config.PrivateZone {
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

				_, err := r53svc.ChangeResourceRecordSets(input)
				if err != nil {
					return false, err
				}

				return true, nil
			}
		}
	}

	return false, nil
}

// DeleteAcmeChallengeResourceRecords spawns an AWS client, constructs baseDomain to retrieve the HostedZones. The ResourceRecordSets are
// then requested, if returned and validated, the record is updated to an empty struct to remove the ACME challange.
func (r *ReconcileCertificateRequest) DeleteAcmeChallengeResourceRecords(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) error {

	r53svc, err := r.getAwsClient(cr)
	if err != nil {
		return err
	}

	hostedZoneOutput, err := r53svc.ListHostedZones(&route53.ListHostedZonesInput{})
	if err != nil {
		return err
	}

	baseDomain := cr.Spec.ACMEDNSDomain

	if string(baseDomain[len(baseDomain)-1]) != "." {
		baseDomain = baseDomain + "."
	}

	for _, hostedzone := range hostedZoneOutput.HostedZones {
		if strings.EqualFold(baseDomain, *hostedzone.Name) {
			zone, err := r53svc.GetHostedZone(&route53.GetHostedZoneInput{Id: hostedzone.Id})
			if err != nil {
				return err
			}

			if !*zone.HostedZone.Config.PrivateZone {

				for _, domain := range cr.Spec.DNSNames {

					domain = strings.TrimPrefix(domain, "*")

					fqdn := acmeChallengeSubDomain + domain
					fqdnWithDot := fqdn + "."

					reqLogger.Info(fmt.Sprintf("deleting resource record %v", fqdn))

					resp, err := r53svc.ListResourceRecordSets(&route53.ListResourceRecordSetsInput{
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

							result, err := r53svc.ChangeResourceRecordSets(input)
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

// func newTXTRecordSet(fqdn, value string, ttl int) *route53.ResourceRecordSet {
// 	return &route53.ResourceRecordSet{
// 		Name: aws.String(fqdn),
// 		Type: aws.String(route53.RRTypeTxt),
// 		TTL:  aws.Int64(int64(ttl)),
// 		ResourceRecords: []*route53.ResourceRecord{
// 			{Value: aws.String(value)},
// 		},
// 	}
// }
