package client

import (
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	certmanv1alpha1 "github.com/openshift/certman-operator/api/v1alpha1"
	"github.com/openshift/certman-operator/pkg/clients/aws"
	"github.com/openshift/certman-operator/pkg/clients/azure"
	"github.com/openshift/certman-operator/pkg/clients/gcp"
	mockclient "github.com/openshift/certman-operator/pkg/clients/mock"
)

var (
	log logr.Logger = logf.Log.WithName("client")
)

// Client is a wrapper object for actual AWS SDK clients to allow for easier testing.
type Client interface {
	// Client methods
	GetDNSName() string
	AnswerDNSChallenge(reqLogger logr.Logger, acmeChallengeToken string, domain string, cr *certmanv1alpha1.CertificateRequest) (string, error)
	ValidateDNSWriteAccess(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) (bool, error)
	DeleteAcmeChallengeResourceRecords(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) error
}

// NewClient returns an individual cloud implementation based on CertificateRequest cloud coniguration
func NewClient(reqLogger logr.Logger, kubeClient client.Client, platform certmanv1alpha1.Platform, namespace string, clusterDeploymentName string) (Client, error) {
	// TODO: Add multicloud checking here
	if platform.AWS != nil {
		log.Info("build aws client")
		return aws.NewClient(reqLogger, kubeClient, platform.AWS.Credentials.Name, namespace, platform.AWS.Region, clusterDeploymentName)
	}
	if platform.GCP != nil {
		log.Info("build gcp client")
		// TODO: Add project as configurable
		return gcp.NewClient(kubeClient, platform.GCP.Credentials.Name, namespace)
	}
	if platform.Azure != nil {
		log.Info("Build Azure client")
		return azure.NewClient(kubeClient, platform.Azure.Credentials.Name, namespace, platform.Azure.ResourceGroupName)
	}
	// NOTE this allows a mock client to be created from a Mock platform secret defined in the platform
	// this allows for better testing of controllers but should be avoided in a live system for obvious reasons
	if platform.Mock != nil {
		log.Info("Build Mock client")
		opts := &mockclient.MockClientOptions{}
		opts.AnswerDNSChallengeFQDN = platform.Mock.AnswerDNSChallengeFQDN
		opts.AnswerDNSChallengeErrorString = platform.Mock.AnswerDNSChallengeErrorString
		opts.ValidateDNSWriteAccessBool = platform.Mock.ValidateDNSWriteAccessBool
		opts.ValidateDNSWriteAccessErrorString = platform.Mock.ValidateDNSWriteAccessErrorString
		opts.DeleteAcmeChallengeResourceRecordsErrorString = platform.Mock.DeleteAcmeChallengeResourceRecordsErrorString

		return mockclient.NewMockClient(opts), nil
	}
	return nil, fmt.Errorf("Platform not supported")
}
