package client

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/prometheus/common/log"
	"sigs.k8s.io/controller-runtime/pkg/client"

	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	"github.com/openshift/certman-operator/pkg/clients/aws"
	"github.com/openshift/certman-operator/pkg/clients/azure"
	"github.com/openshift/certman-operator/pkg/clients/gcp"
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
func NewClient(kubeClient client.Client, platform certmanv1alpha1.Platform, namespace string) (Client, error) {
	// TODO: Add multicloud checking here
	if platform.AWS != nil {
		log.Info("build aws client")
		return aws.NewClient(kubeClient, platform.AWS.Credentials.Name, namespace, platform.AWS.Region)
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
	return nil, fmt.Errorf("Platform not supported")
}
