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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	awsclient "github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aaov1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	certmanv1alpha1 "github.com/openshift/certman-operator/api/v1alpha1"
	"github.com/openshift/certman-operator/config"
	cTypes "github.com/openshift/certman-operator/pkg/clients/types"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

const (
	awsCredsSecretIDKey         = "aws_access_key_id"
	awsCredsSecretAccessKey     = "aws_secret_access_key" //#nosec - G101: Potential hardcoded credentials
	awsCredsSecretName          = "certman-operator-aws-credentials"
	fedrampEnvVariable          = "FEDRAMP"
	fedrampHostedZoneIDVariable = "HOSTED_ZONE_ID"
	fedrampAWSRegion            = "us-east-1"
	resourceRecordTTL           = 60
	clientMaxRetries            = 25
	retryerMaxRetries           = 10
	retryerMinThrottleDelaySec  = 1
	assumeRolePollingRetries    = 100
	assumeRolePollingDelayMilli = 500
	clusterDeploymentSTSLabel   = "api.openshift.com/sts"
	configMapSTSJumpRoleField   = "sts-jump-role"
)

var fedramp = os.Getenv(fedrampEnvVariable) == "true"
var fedrampHostedZoneID = os.Getenv(fedrampHostedZoneIDVariable)

// awsClient implements the Client interface
type awsClient struct {
	client     route53iface.Route53API
	kubeClient client.Client
	namespace  string
}

func (c *awsClient) GetDNSName() string {
	return "Route53"
}

func (c *awsClient) AnswerDNSChallenge(reqLogger logr.Logger, acmeChallengeToken string, domain string, cr *certmanv1alpha1.CertificateRequest) (fqdn string, err error) {
	fqdn = fmt.Sprintf("%s.%s", cTypes.AcmeChallengeSubDomain, domain)
	reqLogger.Info(fmt.Sprintf("fqdn acme challenge domain is %v", fqdn))

	if fedramp {
		zone, err := c.client.GetHostedZone(&route53.GetHostedZoneInput{Id: &fedrampHostedZoneID})
		if err != nil {
			reqLogger.Error(err, err.Error())
			return "", err
		}

		input := &route53.ChangeResourceRecordSetsInput{
			ChangeBatch: &route53.ChangeBatch{
				Changes: []*route53.Change{
					{
						Action: aws.String(route53.ChangeActionUpsert),
						ResourceRecordSet: &route53.ResourceRecordSet{
							Name: &fqdn,
							ResourceRecords: []*route53.ResourceRecord{
								{
									Value: aws.String(fmt.Sprintf("\"%s\"", acmeChallengeToken)),
								},
							},
							TTL:  aws.Int64(resourceRecordTTL),
							Type: aws.String(route53.RRTypeTxt),
						},
					},
				},
				Comment: aws.String(""),
			},
			HostedZoneId: zone.HostedZone.Id,
		}

		reqLogger.Info(fmt.Sprintf("updating hosted zone %v", zone.HostedZone.Name))

		result, err := c.client.ChangeResourceRecordSets(input)
		if err != nil {
			reqLogger.Error(err, result.GoString(), "fqdn", fqdn)
			return "", err
		}

		return fqdn, nil
	}

	dnsZones := hivev1.DNSZoneList{}
	err = c.kubeClient.List(context.TODO(), &dnsZones, &client.ListOptions{Namespace: c.namespace})
	if err != nil {
		reqLogger.Error(err, err.Error())
		return "", err
	}

	if len(dnsZones.Items) != 1 {
		return "", fmt.Errorf("%d dnsZone objects in a specific namespace found, expected 1 dnsZone", len(dnsZones.Items))
	}
	dnsZone := dnsZones.Items[0]
	dnsZoneId := filepath.Base(*dnsZone.Status.AWS.ZoneID)

	zone, err := c.client.GetHostedZone(&route53.GetHostedZoneInput{Id: &dnsZoneId})
	if err != nil {
		reqLogger.Error(err, err.Error())
		return "", err
	}

	// TODO: This duplicate code as AnswerDnsChallenge. Collapse
	if !*zone.HostedZone.Config.PrivateZone {
		input := &route53.ChangeResourceRecordSetsInput{
			ChangeBatch: &route53.ChangeBatch{
				Changes: []*route53.Change{
					{
						Action: aws.String(route53.ChangeActionUpsert),
						ResourceRecordSet: &route53.ResourceRecordSet{
							Name: &fqdn,
							ResourceRecords: []*route53.ResourceRecord{
								{
									Value: aws.String(fmt.Sprintf("\"%s\"", acmeChallengeToken)),
								},
							},
							TTL:  aws.Int64(resourceRecordTTL),
							Type: aws.String(route53.RRTypeTxt),
						},
					},
				},
				Comment: aws.String(""),
			},
			HostedZoneId: zone.HostedZone.Id,
		}

		reqLogger.Info(fmt.Sprintf("updating hosted zone %v", zone.HostedZone.Name))

		result, err := c.client.ChangeResourceRecordSets(input)
		if err != nil {
			reqLogger.Error(err, result.GoString(), "fqdn", fqdn)
			return "", err
		}

		return fqdn, nil
	}

	return "", errors.New("unknown error prevented from answering DNS challenge")
}

// ValidateDnsWriteAccess spawns a route53 client to retrieve the baseDomain's hostedZoneOutput
// and attempts to write a test TXT ResourceRecord to it. If successful, will return `true, nil`.
func (c *awsClient) ValidateDNSWriteAccess(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) (bool, error) {

	var hostedZones []*route53.HostedZone
	var err error
	if fedramp {
		zone, err := c.client.GetHostedZone(&route53.GetHostedZoneInput{Id: &fedrampHostedZoneID})
		if err != nil {
			reqLogger.Error(err, err.Error())
			return false, err
		}
		baseDomain := cr.Spec.ACMEDNSDomain
		if !strings.HasSuffix(baseDomain, ".") {
			baseDomain = baseDomain + "."
		}
		// the fedramp recordset includes the subdomain because one isn't created by hive
		input := &route53.ChangeResourceRecordSetsInput{
			ChangeBatch: &route53.ChangeBatch{
				Changes: []*route53.Change{
					{
						Action: aws.String(route53.ChangeActionUpsert),
						ResourceRecordSet: &route53.ResourceRecordSet{
							Name: aws.String("_certman_access_test." + baseDomain),
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
			HostedZoneId: zone.HostedZone.Id,
		}

		reqLogger.Info(fmt.Sprintf("updating hosted zone %v", zone.HostedZone.Name))

		// Initiate the Write test
		_, err = c.client.ChangeResourceRecordSets(input)
		if err != nil {
			return false, err
		}

		// After successful write test clean up the test record and test deletion of that record.
		input.ChangeBatch.Changes[0].Action = aws.String(route53.ChangeActionDelete)
		_, err = c.client.ChangeResourceRecordSets(input)
		if err != nil {
			reqLogger.Error(err, "Error while deleting Write Access record")
			return false, err
		}
		// If Write and Delete are successful return clean.
		return true, nil
	}

	hostedZones, err = listAllHostedZones(c.client, &route53.ListHostedZonesInput{})
	if err != nil {
		reqLogger.Error(err, err.Error())
		return false, err
	}

	baseDomain := cr.Spec.ACMEDNSDomain
	if !strings.HasSuffix(baseDomain, ".") {
		baseDomain = baseDomain + "."
	}

	for _, hostedzone := range hostedZones {
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

				// After successful write test clean up the test record and test deletion of that record.
				input.ChangeBatch.Changes[0].Action = aws.String(route53.ChangeActionDelete)
				_, err = c.client.ChangeResourceRecordSets(input)
				if err != nil {
					reqLogger.Error(err, "Error while deleting Write Access record")
					return false, err
				}
				// If Write and Delete are successful return clean.
				return true, nil
			}
		}
	}

	return false, nil
}

// DeleteAcmeChallengeResourceRecords spawns an AWS client, constructs baseDomain to retrieve the HostedZones. The ResourceRecordSets are
// then requested, if returned and validated, the record is updated to an empty struct to remove the ACME challenge.
func (c *awsClient) DeleteAcmeChallengeResourceRecords(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) error {

	var hostedZones []*route53.HostedZone

	if fedramp {
		zone, err := c.client.GetHostedZone(&route53.GetHostedZoneInput{Id: &fedrampHostedZoneID})
		if err != nil {
			reqLogger.Error(err, err.Error())
			return err
		}
		hostedZones = []*route53.HostedZone{zone.HostedZone}
	} else {
		hostedZoneOutput, err := c.client.ListHostedZones(&route53.ListHostedZonesInput{})
		if err != nil {
			return err
		}
		hostedZones = hostedZoneOutput.HostedZones
	}

	baseDomain := cr.Spec.ACMEDNSDomain
	if !strings.HasSuffix(baseDomain, ".") {
		baseDomain = baseDomain + "."
	}

	for _, hostedzone := range hostedZones {
		// For fedramp clusters, there will only be one hostedZone and the baseDomain won't match
		// the hostedZone name, so just use the first hostedZone in the loop.
		if strings.EqualFold(baseDomain, *hostedzone.Name) || fedramp {
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

// NewClient returns an awsclient.Client object to the caller. If NewClient is passed a non-null
// secretName, an attempt to retrieve the secret from the namespace argument will be performed.
// AWS credentials are returned as these secrets and a new session is initiated prior to returning
// a client. If secrets fail to return, the IAM role of the masters is used to create a
// new session for the client.
func NewClient(reqLogger logr.Logger, kubeClient client.Client, secretName, namespace, region, clusterDeploymentName string) (*awsClient, error) {
	awsConfig := &aws.Config{
		Region: aws.String(region),
		// MaxRetries to limit the number of attempts on failed API calls
		MaxRetries: aws.Int(clientMaxRetries),
		// Set MinThrottleDelay to 1 second
		Retryer: awsclient.DefaultRetryer{
			// Set NumMaxRetries to 10 (default is 3) for failed retries
			NumMaxRetries: retryerMaxRetries,
			// Set MinThrottleDelay to 1s (default is 500ms)
			MinThrottleDelay: retryerMinThrottleDelaySec * time.Second,
		},
	}

	// If this is a fedramp cluster, get AWS credentials from 'certman-operator' namespace
	if fedramp {
		awsConfig.Region = aws.String(fedrampAWSRegion)
		secret := &corev1.Secret{}
		err := kubeClient.Get(context.TODO(),
			types.NamespacedName{
				Name:      awsCredsSecretName,
				Namespace: "certman-operator",
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
		s, err := session.NewSession(awsConfig)
		if err != nil {
			return nil, err
		}

		c := &awsClient{
			kubeClient: kubeClient,
			client:     route53.New(s),
			namespace:  namespace,
		}

		return c, err
	}

	// Check if ClusterDeployment is labelled for STS
	clusterDeployment := &hivev1.ClusterDeployment{}
	err := kubeClient.Get(context.TODO(), types.NamespacedName{
		Name:      clusterDeploymentName,
		Namespace: namespace,
	}, clusterDeployment)
	if err != nil {
		return nil, err
	}
	if stsEnabled, ok := clusterDeployment.Labels[clusterDeploymentSTSLabel]; ok && stsEnabled == "true" {
		// Get STS jump role from from aws-account-operator ConfigMap
		cm := &corev1.ConfigMap{}
		err := kubeClient.Get(context.TODO(), types.NamespacedName{
			Name:      aaov1alpha1.DefaultConfigMap,
			Namespace: aaov1alpha1.AccountCrNamespace,
		}, cm)

		if err != nil {
			return nil, fmt.Errorf("error getting aws-account-operator configmap to get the sts jump role: %v", err)
		}

		stsAccessARN := cm.Data[configMapSTSJumpRoleField]
		if stsAccessARN == "" {
			return nil, fmt.Errorf("aws-account-operator configmap missing sts-jump-role: %v", aaov1alpha1.ErrInvalidConfigMap)
		}

		// Get STS Creds
		secret := &corev1.Secret{}
		err = kubeClient.Get(context.TODO(),
			types.NamespacedName{
				Name:      awsCredsSecretName,
				Namespace: config.OperatorNamespace,
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
			string(accessKeyID),
			string(secretAccessKey),
			"",
		)

		s, err := session.NewSession(awsConfig)
		if err != nil {
			return nil, fmt.Errorf("unable to setup STS client: %v", err)
		}

		hiveAwsClient := sts.New(s)

		jumpRoleCreds, err := getSTSCredentials(reqLogger, hiveAwsClient, stsAccessARN, "", "certmanOperator")
		if err != nil {
			return nil, fmt.Errorf("unable to assume jump role %s: %v", stsAccessARN, err)
		}

		jumpConfig := &aws.Config{
			Region:     aws.String(region),
			MaxRetries: aws.Int(clientMaxRetries),
			Retryer: awsclient.DefaultRetryer{
				NumMaxRetries:    retryerMaxRetries,
				MinThrottleDelay: retryerMinThrottleDelaySec * time.Second,
			},
			Credentials: credentials.NewStaticCredentials(
				*jumpRoleCreds.Credentials.AccessKeyId,
				*jumpRoleCreds.Credentials.SecretAccessKey,
				*jumpRoleCreds.Credentials.SessionToken,
			),
		}

		js, err := session.NewSession(jumpConfig)
		if err != nil {
			return nil, fmt.Errorf("unable to setup AWS client with STS jump role %s: %v", stsAccessARN, err)
		}

		jumpRoleClient := sts.New(js)

		// Get Account's STS role from AccountClaim
		accountClaim := &aaov1alpha1.AccountClaim{}
		err = kubeClient.Get(context.TODO(), types.NamespacedName{
			Name:      clusterDeploymentName,
			Namespace: namespace,
		}, accountClaim)
		if err != nil {
			return nil, err
		}

		if accountClaim.Spec.ManualSTSMode {
			if accountClaim.Spec.STSRoleARN == "" {
				return nil, fmt.Errorf("STSRoleARN missing from AccountClaim %s", accountClaim.Name)
			}

		}

		customerAccountCreds, err := getSTSCredentials(reqLogger, jumpRoleClient, accountClaim.Spec.STSRoleARN, accountClaim.Spec.STSExternalID, "RH-Account-Initilization")

		if err != nil {
			return nil, fmt.Errorf("unable to assume customer role %s: %v", accountClaim.Spec.STSRoleARN, err)
		}

		customerAccountConfig := &aws.Config{
			Region:     aws.String(region),
			MaxRetries: aws.Int(clientMaxRetries),
			Retryer: awsclient.DefaultRetryer{
				NumMaxRetries:    retryerMaxRetries,
				MinThrottleDelay: retryerMinThrottleDelaySec * time.Second,
			},
			Credentials: credentials.NewStaticCredentials(
				*customerAccountCreds.Credentials.AccessKeyId,
				*customerAccountCreds.Credentials.SecretAccessKey,
				*customerAccountCreds.Credentials.SessionToken,
			),
		}

		cs, err := session.NewSession(customerAccountConfig)
		if err != nil {
			return nil, fmt.Errorf("unable to setup AWS client with customer role credentials %s: %v", accountClaim.Spec.STSRoleARN, err)
		}

		c := &awsClient{
			kubeClient: kubeClient,
			client:     route53.New(cs),
			namespace:  namespace,
		}

		return c, err
	}

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
		kubeClient: kubeClient,
		client:     route53.New(s),
		namespace:  namespace,
	}
	return c, err
}

func getSTSCredentials(reqLogger logr.Logger, client *sts.STS, roleArn string, externalID string, roleSessionName string) (*sts.AssumeRoleOutput, error) {
	// Default duration in seconds of the session token 3600. We need to have the roles policy
	// changed if we want it to be longer than 3600 seconds
	var roleSessionDuration int64 = 3600
	reqLogger.Info(fmt.Sprintf("Creating STS credentials for AWS ARN: %s", roleArn))
	// Build input for AssumeRole
	assumeRoleInput := sts.AssumeRoleInput{
		DurationSeconds: &roleSessionDuration,
		RoleArn:         &roleArn,
		RoleSessionName: &roleSessionName,
	}
	if externalID != "" {
		assumeRoleInput.ExternalId = &externalID
	}

	assumeRoleOutput := &sts.AssumeRoleOutput{}
	var err error
	for i := 0; i < assumeRolePollingRetries; i++ {
		time.Sleep(assumeRolePollingDelayMilli * time.Millisecond)
		assumeRoleOutput, err = client.AssumeRole(&assumeRoleInput)
		if err == nil {
			break
		}
		if i == (assumeRolePollingRetries - 1) {
			return nil, fmt.Errorf("timed out while assuming role %s", roleArn)
		}
	}
	if err != nil {
		// Log AWS error
		if aerr, ok := err.(awserr.Error); ok {
			reqLogger.Error(aerr, "New AWS Error while getting STS credentials,\nAWS Error Code: %s,\nAWS Error Message: %s", aerr.Code(), aerr.Message())
		}
		return &sts.AssumeRoleOutput{}, err
	}
	return assumeRoleOutput, nil
}

// listAllHostedZones is a wrapper around the Route53API function
// ListHostedZones() that keeps looping if the results are truncated
// and returns all the hosted zones
func listAllHostedZones(r53 route53iface.Route53API, lhzi *route53.ListHostedZonesInput) ([]*route53.HostedZone, error) {
	var hostedZones []*route53.HostedZone

	more := true
	for more {
		output, err := r53.ListHostedZones(lhzi)
		if err != nil {
			return []*route53.HostedZone{}, err
		}

		if output.IsTruncated == nil {
			more = false
		} else {
			more = *output.IsTruncated
			lhzi.Marker = output.NextMarker
		}

		hostedZones = append(hostedZones, output.HostedZones...)
	}

	return hostedZones, nil
}
