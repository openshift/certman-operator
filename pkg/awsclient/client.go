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

package awsclient

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
)

const (
	awsCredsSecretIDKey     = "aws_access_key_id"
	awsCredsSecretAccessKey = "aws_secret_access_key"
	resourceRecordTTL       = 60
)

// Client is a wrapper object for actual AWS SDK clients to allow for easier testing.
type Client interface {
	// Route53 client
	CreateHostedZone(input *route53.CreateHostedZoneInput) (*route53.CreateHostedZoneOutput, error)
	DeleteHostedZone(input *route53.DeleteHostedZoneInput) (*route53.DeleteHostedZoneOutput, error)
	ListHostedZones(input *route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error)
	GetHostedZone(*route53.GetHostedZoneInput) (*route53.GetHostedZoneOutput, error)
	ChangeResourceRecordSets(*route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error)
	ListResourceRecordSets(*route53.ListResourceRecordSetsInput) (*route53.ListResourceRecordSetsOutput, error)
}

// awsClient implements the Client interface
type awsClient struct {
	route53Client route53iface.Route53API
}

func (c *awsClient) ListHostedZones(input *route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error) {
	return c.route53Client.ListHostedZones(input)
}

func (c *awsClient) CreateHostedZone(input *route53.CreateHostedZoneInput) (*route53.CreateHostedZoneOutput, error) {
	return c.route53Client.CreateHostedZone(input)
}

func (c *awsClient) DeleteHostedZone(input *route53.DeleteHostedZoneInput) (*route53.DeleteHostedZoneOutput, error) {
	return c.route53Client.DeleteHostedZone(input)
}

func (c *awsClient) GetHostedZone(input *route53.GetHostedZoneInput) (*route53.GetHostedZoneOutput, error) {
	return c.route53Client.GetHostedZone(input)
}

func (c *awsClient) ChangeResourceRecordSets(input *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error) {
	return c.route53Client.ChangeResourceRecordSets(input)
}

func (c *awsClient) ListResourceRecordSets(input *route53.ListResourceRecordSetsInput) (*route53.ListResourceRecordSetsOutput, error) {
	return c.route53Client.ListResourceRecordSets(input)
}

// SearchForHostedZone finds a hostedzone when given an aws client and a domain string
// Returns a hostedzone object
func SearchForHostedZone(r53svc Client, baseDomain string) (hostedZone route53.HostedZone, err error) {
	hostedZoneOutput, err := r53svc.ListHostedZones(&route53.ListHostedZonesInput{})
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
func BuildR53Input(hostedZone string) *route53.ChangeResourceRecordSetsInput {
	input := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{},
		},
		HostedZoneId: &hostedZone,
	}
	return input
}

// CreateR53TXTRecordChange creates an route53 Change object for a TXT record with a given name
// and a given action to take.
func CreateR53TXTRecordChange(name *string, action string, value *string) (change route53.Change, err error) {
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

// NewClient returns an awsclient.Client object to the caller. If NewClient is passed a non-null
// secretName, an attempt to retrieve the secret from the namespace argument will be performed.
// AWS credentials are returned as these secrets and a new session is initiated prior to returning
// a route53Client. If secrets fail to return, the IAM role of the masters is used to create a
// new session for the client.
func NewClient(kubeClient client.Client, secretName, namespace, region string) (Client, error) {

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

	return &awsClient{
		route53Client: route53.New(s),
	}, nil
}
