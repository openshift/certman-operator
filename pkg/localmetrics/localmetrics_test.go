package localmetrics

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestUpdateCertValidDuration(t *testing.T) {
	// Create a test certificate
	createTestCert := func(notAfter time.Time, commonName string) *x509.Certificate {
		return &x509.Certificate{
			NotAfter: notAfter,
			Subject:  pkix.Name{CommonName: commonName},
		}
	}

	// Reset the metric before each test
	MetricCertValidDuration.Reset()

	tests := []struct {
		name          string
		cert          *x509.Certificate
		expectedValue float64
		expectedErr   bool
	}{
		{
			name:          "Cert expired yesterday",
			cert:          createTestCert(time.Now().Add(-24*time.Hour), "test.example.com"),
			expectedValue: 0,
			expectedErr:   false,
		},
		{
			name:          "Cert expires tomorrow",
			cert:          createTestCert(time.Now().Add(24*time.Hour), "test.example.com"),
			expectedValue: 1,
			expectedErr:   false,
		},
		{
			name:          "Cert expires in 30 days",
			cert:          createTestCert(time.Now().Add(30*24*time.Hour), "test.example.com"),
			expectedValue: 30,
			expectedErr:   false,
		},
		{
			name:          "Nil certificate",
			cert:          nil,
			expectedValue: 0,
			expectedErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := UpdateCertValidDuration(tt.cert, "test-certificaterequest-name", "test-certificaterequest-namespace")
			if err != nil {
				if tt.expectedErr {
					// No need to test further if an error was expected
					return
				}
				t.Fatalf("unexpected error creating metric: %v", err)
			}
			cn := tt.cert.Subject.CommonName

			metric, err := MetricCertValidDuration.GetMetricWithLabelValues(cn, "test-certificaterequest-name", "test-certificaterequest-namespace")
			if err != nil {
				t.Fatalf("Error getting metric: %v", err)
			}

			if value := testutil.ToFloat64(metric); value != tt.expectedValue {
				t.Errorf("Expected %.2f, but got %.2f", tt.expectedValue, value)
			}
		})
	}
}
