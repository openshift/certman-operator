package client

import (
	"fmt"

	"github.com/go-logr/logr"
	certmanv1alpha1 "github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	"github.com/openshift/certman-operator/pkg/clients/aws"
	"github.com/openshift/certman-operator/pkg/clients/gcp"
	"github.com/prometheus/common/log"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Client is a wrapper object for actual AWS SDK clients to allow for easier testing.
type Client interface {
	// Client methods
	AnswerDNSChallenge(reqLogger logr.Logger, acmeChallengeToken string, domain string, cr *certmanv1alpha1.CertificateRequest) (string, error)
	ValidateDNSWriteAccess(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) (bool, error)
	DeleteAcmeChallengeResourceRecords(reqLogger logr.Logger, cr *certmanv1alpha1.CertificateRequest) error
}

// NewClient returns an individual cloud implementation based on CertificateRequest cloud coniguration
func NewClient(kubeClient client.Client, platfromSecret certmanv1alpha1.PlatformSecrets, namespace string) (Client, error) {
	// TODO: Add multicloud checking here
	if platfromSecret.AWS != nil {
		log.Info("build aws client")
		// TOFIX: Region hardcoded!!!
		return aws.NewClient(kubeClient, platfromSecret.AWS.Credentials.Name, namespace, "us-east-1")
	}
	if platfromSecret.GCP != nil {
		log.Info("build gcp client")
		// TODO: Add project as configurable
		return gcp.NewClient(kubeClient, platfromSecret.GCP.Credentials.Name, namespace, "openshift-sd-testing")
	}
	return nil, fmt.Errorf("Platform not supported")
}
