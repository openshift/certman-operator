package certificaterequest

import (
	"crypto/x509"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	certmanv1alpha1 "github.com/openshift/certman-operator/api/v1alpha1"
)

func TestUpdateStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, certmanv1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	type testCase struct {
		name           string
		setup          func() (client.Client, *certmanv1alpha1.CertificateRequest, *x509.Certificate)
		expectError    bool
		expectedStatus string
		expectedIssued bool
		verifyExtra    func(*certmanv1alpha1.CertificateRequest, *x509.Certificate)
	}

	tests := []testCase{
		{
			name: "valid_certificate_updates_status_to_success",
			setup: func() (client.Client, *certmanv1alpha1.CertificateRequest, *x509.Certificate) {
				certPEM, parsedCert, err := generateValidCertPEM()
				require.NoError(t, err)

				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cert-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						corev1.TLSCertKey: certPEM,
					},
				}

				cr := &certmanv1alpha1.CertificateRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cr",
						Namespace: "default",
					},
					Spec: certmanv1alpha1.CertificateRequestSpec{
						CertificateSecret: corev1.ObjectReference{
							Name:      "test-cert-secret",
							Namespace: "default",
						},
					},
					Status: certmanv1alpha1.CertificateRequestStatus{
						Issued: false,
					},
				}

				cl := fake.NewClientBuilder().
					WithScheme(scheme).
					WithRuntimeObjects(cr, secret).
					WithStatusSubresource(&certmanv1alpha1.CertificateRequest{}).
					Build()

				return cl, cr, parsedCert
			},
			expectError:    false,
			expectedStatus: "Success",
			expectedIssued: true,
			verifyExtra: func(cr *certmanv1alpha1.CertificateRequest, parsedCert *x509.Certificate) {
				assert.Equal(t, parsedCert.Issuer.CommonName, cr.Status.IssuerName)
				assert.Equal(t, parsedCert.SerialNumber.String(), cr.Status.SerialNumber)
			},
		},
		{
			name: "nil_certificate_request",
			setup: func() (client.Client, *certmanv1alpha1.CertificateRequest, *x509.Certificate) {
				cl := fake.NewClientBuilder().WithScheme(scheme).Build()
				return cl, nil, nil
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cl, cr, parsedCert := tc.setup()
			rcr := &CertificateRequestReconciler{
				Client: cl,
				Scheme: scheme,
			}
			err := rcr.updateStatus(logr.Discard(), cr)

			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedStatus, cr.Status.Status)
				assert.Equal(t, tc.expectedIssued, cr.Status.Issued)
				tc.verifyExtra(cr, parsedCert)
			}
		})
	}
}

func TestAcmeError(t *testing.T) {
	errMsg := "example acme failure" // Example error message for testing
	mockErr := errors.New(errMsg)

	tests := []struct {
		name            string
		existingConds   []certmanv1alpha1.CertificateRequestCondition
		expectedType    certmanv1alpha1.CertificateRequestConditionType
		expectedStatus  corev1.ConditionStatus
		expectedMessage string
		expectCondition bool
	}{
		{
			name:            "condition_not_present_should_create_new",
			existingConds:   []certmanv1alpha1.CertificateRequestCondition{},
			expectedType:    "acme error",
			expectedStatus:  "Error",
			expectedMessage: errMsg,
			expectCondition: true,
		},
		{
			name: "condition_already_present_no_new_condition",
			existingConds: []certmanv1alpha1.CertificateRequestCondition{
				{
					Type:   "acme error",
					Status: "Error",
				},
			},
			expectCondition: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := &certmanv1alpha1.CertificateRequest{
				Status: certmanv1alpha1.CertificateRequestStatus{
					Conditions: tt.existingConds,
				},
			}

			logger := logr.Discard()

			cond, err := acmeError(logger, cr, mockErr)
			require.NoError(t, err)

			if tt.expectCondition {
				require.Equal(t, tt.expectedType, cond.Type)
				require.Equal(t, tt.expectedStatus, cond.Status)
				require.NotNil(t, cond.Message)
				require.Equal(t, tt.expectedMessage, *cond.Message)
			} else {
				require.Empty(t, cond.Type)
				require.Empty(t, cond.Status)
				require.Nil(t, cond.Message)
			}
		})
	}
}

func TestUpdateStatusError(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, certmanv1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	tests := []struct {
		name       string
		inputError error
		expectErr  bool
	}{
		{
			name:       "acme_error_should_set_condition_and_update_status",
			inputError: fmt.Errorf("acme: random error"),
			expectErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := &certmanv1alpha1.CertificateRequest{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "certman.managed.openshift.io/v1alpha1",
					Kind:       "CertificateRequest",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cr",
					Namespace: "default",
				},
			}

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cert-secret",
					Namespace: "default",
				},
			}
			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(cr, secret).
				WithStatusSubresource(&certmanv1alpha1.CertificateRequest{}).
				Build()

			reconciler := &CertificateRequestReconciler{
				Client: cl,
				Scheme: scheme,
			}
			err := reconciler.updateStatusError(logr.Discard(), cr, tt.inputError)
			if (err != nil) != tt.expectErr {
				t.Errorf("GetCertificate() Got unexpected error: %v", err)
			}
		})
	}
}
