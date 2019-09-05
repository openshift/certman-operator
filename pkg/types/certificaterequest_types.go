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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"github.com/openshift/certman-operator/pkg/apis/certman/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/types"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
)

// CertificateRequestSpec defines the desired state of CertificateRequest
// +k8s:openapi-gen=true
type CertificateRequestSpec struct {

	// ACMEDNSDomain is the DNS zone that will house the TXT records needed for the
	// certificate to be created.
	// In Route53 this would be the public Route53 hosted zone (the Domain Name not the ZoneID)
	ACMEDNSDomain string `json:"acmeDNSDomain"`

	// CertificateSecret is the reference to the secret where certificates are stored.
	CertificateSecret corev1.ObjectReference `json:"certificateSecret"`

	// PlatformSecrets contains the credentials and secrets for the cluster infrastructure.
	PlatformSecrets PlatformSecrets `json:"platformSecrets"`

	// DNSNames is a list of subject alt names to be used on the Certificate.
	DnsNames []string `json:"dnsNames"`

	// Let's Encrypt will use this to contact you about expiring certificates, and issues related to your account.
	Email string `json:"email"`

	// Certificate renew before expiration duration in days.
	// +optional
	RenewBeforeDays int `json:"renewBeforeDays,omitempty"`

	// APIURL is the URL where the cluster's API can be accessed.
	// +optional
	APIURL string `json:"apiURL,omitempty"`

	// WebConsoleURL is the URL for the cluster's web console UI.
	// +optional
	WebConsoleURL string `json:"webConsoleURL,omitempty"`
}

// CertificateRequestCondition defines conditions required for certificate requests.
type CertificateRequestCondition struct {

	// Type is the type of the condition.
	Type CertificateRequestConditionType `json:"type"`

	// Status is the status of the condition.
	Status corev1.ConditionStatus `json:"status"`

	// LastProbeTime is the last time we probed the condition.
	// +optional
	LastProbeTime *metav1.Time `json:"lastProbeTime,omitempty"`

	// LastTransitionTime is the last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`

	// Reason is a unique, one-word, CamelCase reason for the condition's last transition.
	// +optional
	Reason *string `json:"reason,omitempty"`

	// Message is a human-readable message indicating details about last transition.
	// +optional
	Message *string `json:"message,omitempty"`
}

// CertificateRequestConditionType is the condition that populates the Type var
// within the CertificateRequestCondition struct
type CertificateRequestConditionType string

// CertificateRequestStatus defines the observed state of CertificateRequest
// +k8s:openapi-gen=true
type CertificateRequestStatus struct {

	// Issued is true once certificates have been issued.
	Issued bool `json:"issued,omitempty"`

	// Status
	// +optional
	Status string `json:"status,omitempty"`

	// The expiration time of the certificate stored in the secret named by this resource in spec.secretName.
	// +optional
	NotAfter string `json:"notAfter,omitempty"`

	// The earliest time and date on which the certificate stored in the secret named by this resource in spec.secretName is valid.
	// +optional
	NotBefore string `json:"notBefore,omitempty"`

	// The entity that verified the information and signed the certificate.
	// +optional
	IssuerName string `json:"issuerName,omitempty"`

	// The serial number of the certificate stored in the secret named by this resource in spec.secretName.
	// +optional
	SerialNumber string `json:"serialNumber,omitempty"`

	// Conditions includes more detailed status for the Certificate Request
	// +optional
	Conditions []CertificateRequestCondition `json:"conditions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CertificateRequest is the Schema for the certificaterequests API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="IssuerName",type="string",JSONPath=".status.issuerName"
// +kubebuilder:printcolumn:name="NotBefore",type="string",JSONPath=".status.notBefore"
// +kubebuilder:printcolumn:name="NotAfter",type="string",JSONPath=".status.notAfter"
// +kubebuilder:printcolumn:name="Secret",type="string",JSONPath=".spec.certificateSecret.name"
type CertificateRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CertificateRequestSpec   `json:"spec,omitempty"`
	Status CertificateRequestStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CertificateRequestList contains a list of CertificateRequest
type CertificateRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CertificateRequest `json:"items"`
}

// PlatformSecrets defines the secrets to be used by various clouds.
type PlatformSecrets struct {
	AWS *AWSPlatformSecrets `json:"aws"`
}

// AWSPlatformSecrets contains secrets for clusters on the AWS platform.
type AWSPlatformSecrets struct {
	// Credentials refers to a secret that contains the AWS account access
	// credentials.
	Credentials corev1.LocalObjectReference `json:"credentials"`
}

const (
	// CertmanOperatorFinalizerLabel is a K8's finalizer. An arbitray string that when
	// present ensures a hard delete of a resource is not possible.
	CertmanOperatorFinalizerLabel = "certificaterequests.certman.managed.openshift.io"
)

func init() {
	// Register adds its arguments (objects) to SchemeBuilder so they can be added to a Scheme.
	v1alpha1.SchemeBuilder.Register(&CertificateRequest{}, &CertificateRequestList{})
}

// ReconcileCertificateRequest reconciles a CertificateRequest object
type ReconcileCertificateRequest struct {
	client           client.Client
	scheme           *runtime.Scheme
	recorder         record.EventRecorder
	awsClientBuilder func(kubeClient client.Client, secretName, namespace, region string) (awsclient.Client, error)
}

///////////////////////
// AWS client types  //
///////////////////////

const (
	awsCredsSecretIDKey     = "aws_access_key_id"
	awsCredsSecretAccessKey = "aws_secret_access_key"
)

// AWSClient is a wrapper object for actual AWS SDK clients to allow for easier testing.
type AWSClient interface {
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

// getAwsClient returns awsclient to the caller
func (r *ReconcileCertificateRequest) getAwsClient(cr *CertificateRequest) (awsclient.Client, error) {
	awsapi, err := r.awsClientBuilder(r.client, cr.Spec.PlatformSecrets.AWS.Credentials.Name, cr.Namespace, "us-east-1") //todo why is this region var hardcoded???
	return awsapi, err
}

// TestAuth ensures that AWS credentials are present,
// and that they contain the permisionssion required
// for running this operator.
func (c *awsClient) TestAuth(cr *CertificateRequest, r *ReconcileCertificateRequest) error {
	platformSecretName := cr.Spec.PlatformSecrets.AWS.Credentials.Name

		awscreds := &corev1.Secret{}
		err := r.client.Get(context.TODO(), types.NamespacedName{Name: platformSecretName, Namespace: request.Namespace}, awscreds)
	    if err != nil {
			fmt.Println("platformSecrets were not found. Unable to search for certificates in cloud provider platform")
			return err
		}
	// Ensure that platform Secret can authenticate to AWS.
	   r53svc, err := r.getAwsClient(cr)
		if err != nil {
			return err
		}
	
	hostedZoneOutput, err := r53svc.ListHostedZones(&route53.ListHostedZonesInput{})
	if err != nil {
	fmt.Println("platformSecrets are either invalid, or don't have permission to list Route53 HostedZones")
	return err
}
	
println("Successfully authenticated with cloudprovider. Hosted zones found:")
println(hostedZoneOutput)
	
	return nil
}

func (c *awsClient) AWSListHostedZones(input *route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error) {
	return c.route53Client.ListHostedZones(input)
}

func (c *awsClient) AWSCreateHostedZone(input *route53.CreateHostedZoneInput) (*route53.CreateHostedZoneOutput, error) {
	return c.route53Client.CreateHostedZone(input)
}

func (c *awsClient) AWSDeleteHostedZone(input *route53.DeleteHostedZoneInput) (*route53.DeleteHostedZoneOutput, error) {
	return c.route53Client.DeleteHostedZone(input)
}

func (c *awsClient) AWSGetHostedZone(input *route53.GetHostedZoneInput) (*route53.GetHostedZoneOutput, error) {
	return c.route53Client.GetHostedZone(input)
}

func (c *awsClient) AWSChangeResourceRecordSets(input *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error) {
	return c.route53Client.ChangeResourceRecordSets(input)
}

func (c *awsClient) AWSListResourceRecordSets(input *route53.ListResourceRecordSetsInput) (*route53.ListResourceRecordSetsOutput, error) {
	return c.route53Client.ListResourceRecordSets(input)
}

// NewAWSClient returns an awsclient.Client object to the caller. If NewClient is passed a non-null
// secretName, an attempt to retrieve the secret from the namespace argument will be performed.
// AWS credentials are returned as these secrets and a new session is initiated prior to returning
// a route53Client. If secrets fail to return, the IAM role of the masters is used to create a
// new session for the client.
func AWSNewClient(kubeClient client.Client, secretName, namespace, region string) (Client, error) {

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