package client

import (
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/openshift/certman-operator/pkg/clients/aws"

	"sigs.k8s.io/controller-runtime/pkg/client"
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

	SearchForHostedZone(baseDomain string) (route53.HostedZone, error)
	BuildR53Input(hostedZone string) *route53.ChangeResourceRecordSetsInput
	CreateR53TXTRecordChange(name *string, action string, value *string) (route53.Change, error)
}

// NewClient returns an individual cloud implementation based on CertificateRequest cloud coniguration
func NewClient(kubeClient client.Client, secretName, namespace, region string) (Client, error) {
	// TODO: Add multicloud checking here
	return aws.NewClient(kubeClient, secretName, namespace, region)
}
