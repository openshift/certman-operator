package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CertificateRequestSpec defines the desired state of CertificateRequest
// +k8s:openapi-gen=true
type CertificateRequestSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book.kubebuilder.io/beyond_basics/generating_crd.html

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

	// Certificate renew before expiration duration in days.
	// +optional
	RenewBeforeDays int `json:"renewBeforeDays,omitempty"`
}

type CertificateRequestCondition struct {

	// Type is the type of the condition.
	Type CertificateRequestConditionType `json:"type"`
	// Status is the status of the condition.
	Status corev1.ConditionStatus `json:"status"`
	// LastProbeTime is the last time we probed the condition.
	// +optional
	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty"`
	// LastTransitionTime is the last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// Reason is a unique, one-word, CamelCase reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Message is a human-readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
}

type CertificateRequestConditionType string

// CertificateRequestStatus defines the observed state of CertificateRequest
// +k8s:openapi-gen=true
type CertificateRequestStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book.kubebuilder.io/beyond_basics/generating_crd.html

	// Issued is true once certificates have been issued.
	Issued bool `json:"issued,omitempty"`

	// The expiration time of the certificate stored in the secret named by this resource in spec.secretName.
	// +optional
	NotAfter *metav1.Time `json:"notAfter,omitempty"`

	// The earliest time and date on which the certificate stored in the secret named by this resource in spec.secretName is valid.
	// +optional
	NotBefore *metav1.Time `json:"notBefore,omitempty"`

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
	// +optional
	AWS *AWSPlatformSecrets `json:"aws,omitempty"`
}

// AWSPlatformSecrets contains secrets for clusters on the AWS platform.
type AWSPlatformSecrets struct {
	// Credentials refers to a secret that contains the AWS account access
	// credentials.
	Credentials corev1.LocalObjectReference `json:"credentials"`
}

func init() {
	SchemeBuilder.Register(&CertificateRequest{}, &CertificateRequestList{})
}
