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
	certman "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	"github.com/openshift/certman-operator/pkg/leclient"
	"github.com/openshift/certman-operator/pkg/sleep"
)

const (
	awsCredsSecretIDKey     = "aws_access_key_id"
	awsCredsSecretAccessKey = "aws_secret_access_key"
)

// Client is a wrapper object for actual AWS SDK clients to allow for easier testing.
type Client interface {
	// Route53 client
	CreateHostedZone(cr *certman.CertificateRequest, input *route53.CreateHostedZoneInput) (*route53.CreateHostedZoneOutput, error)
	DeleteHostedZone(cr *certman.CertificateRequest, input *route53.DeleteHostedZoneInput) (*route53.DeleteHostedZoneOutput, error)
	ListHostedZones(cr *certman.CertificateRequest, input *route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error)
	GetHostedZone(*certman.CertificateRequest, *route53.GetHostedZoneInput) (*route53.GetHostedZoneOutput, error)
	ChangeResourceRecordSets(*certman.CertificateRequest, *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error)
	ListResourceRecordSets(*certman.CertificateRequest, *route53.ListResourceRecordSetsInput) (*route53.ListResourceRecordSetsOutput, error)
}

// awsClient implements the Client interface
type awsClient struct {
	route53Client route53iface.Route53API
}

func (c *awsClient) ListHostedZones(cr *certman.CertificateRequest, input *route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error) {
	// pause before making an API call
	sleep.ExponentialBackOff(cr.Status.FailCount)
	output, err := c.route53Client.ListHostedZones(input)
	if err != nil {
		fmt.Println("DEBUG: error in ListHostedZones()")
		leclient.AddToFailCount(cr)
	}
	return output, err
}

func (c *awsClient) CreateHostedZone(cr *certman.CertificateRequest, input *route53.CreateHostedZoneInput) (*route53.CreateHostedZoneOutput, error) {
	// pause before making an API call
	sleep.ExponentialBackOff(cr.Status.FailCount)
	output, err := c.route53Client.CreateHostedZone(input)
	if err != nil {
		fmt.Println("DEBUG: error in CreateHostedZone()")
		leclient.AddToFailCount(cr)
	}
	return output, err
}

func (c *awsClient) DeleteHostedZone(cr *certman.CertificateRequest, input *route53.DeleteHostedZoneInput) (*route53.DeleteHostedZoneOutput, error) {
	sleep.ExponentialBackOff(cr.Status.FailCount)
	output, err := c.route53Client.DeleteHostedZone(input)
	if err != nil {
		fmt.Println("DEBUG: error in DeleteHostedZone()")
		leclient.AddToFailCount(cr)
	}
	return output, err
}

func (c *awsClient) GetHostedZone(cr *certman.CertificateRequest, input *route53.GetHostedZoneInput) (*route53.GetHostedZoneOutput, error) {
	sleep.ExponentialBackOff(cr.Status.FailCount)
	output, err := c.route53Client.GetHostedZone(input)
	if err != nil {
		fmt.Println("DEBUG: error in GetHostedZone()")
		leclient.AddToFailCount(cr)
	}
	return output, err
}

func (c *awsClient) ChangeResourceRecordSets(cr *certman.CertificateRequest, input *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error) {
	sleep.ExponentialBackOff(cr.Status.FailCount)
	output, err := c.route53Client.ChangeResourceRecordSets(input)
	if err != nil {
		fmt.Println("DEBUG: error in ChangeResourceRecordSets()")
		leclient.AddToFailCount(cr)
	}
	return output, err
}

func (c *awsClient) ListResourceRecordSets(cr *certman.CertificateRequest, input *route53.ListResourceRecordSetsInput) (*route53.ListResourceRecordSetsOutput, error) {
	sleep.ExponentialBackOff(cr.Status.FailCount)
	output, err := c.route53Client.ListResourceRecordSets(input)
	if err != nil {
		fmt.Println("DEBUG: error in CreateHostedZone()")
		leclient.AddToFailCount(cr)
	}
	return output, err
}

// NewClient returns an awsclient.Client object to the caller. If NewClient is passed a non-null
// secretName, an attempt to retrieve the secret from the namespace argument will be performed.
// AWS credentials are returned as these secrets and a new session is initiated prior to returning
// a route53Client. If secrets fail to return, the IAM role of the masters is used to create a
// new session for the client.
func NewClient(kubeClient client.Client, secretName, namespace, region string, cr *certman.CertificateRequest) (Client, error) {

	awsConfig := &aws.Config{Region: aws.String(region)}

	sleep.ExponentialBackOff(cr.Status.FailCount)

	if secretName != "" {
		secret := &corev1.Secret{}
		err := kubeClient.Get(context.TODO(),
			types.NamespacedName{
				Name:      secretName,
				Namespace: namespace,
			},
			secret)

		if err != nil {
			leclient.AddToFailCount(cr)
			return nil, err
		}

		accessKeyID, ok := secret.Data[awsCredsSecretIDKey]
		if !ok {
			leclient.AddToFailCount(cr)
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
