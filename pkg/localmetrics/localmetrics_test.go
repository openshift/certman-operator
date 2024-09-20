package localmetrics

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestUpdateCertValidDuration(t *testing.T) {
	// Create a test certificate
	createTestCert := func(notAfter time.Time) *x509.Certificate {
		return &x509.Certificate{
			NotAfter: notAfter,
			Subject:  pkix.Name{CommonName: "test.example.com"},
		}
	}

	// Reset the metric before each test
	MetricCertValidDuration = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "certman_operator_certificate_valid_duration_days",
		Help: "The number of days for which the certificate remains valid",
	}, []string{"cn"})

	tests := []struct {
		name     string
		cert     *x509.Certificate
		expected float64
	}{
		{
			name:     "Cert expired yesterday",
			cert:     createTestCert(time.Now().Add(-24 * time.Hour)),
			expected: 0,
		},
		{
			name:     "Cert expires tomorrow",
			cert:     createTestCert(time.Now().Add(24 * time.Hour)),
			expected: 1,
		},
		{
			name:     "Cert expires in 30 days",
			cert:     createTestCert(time.Now().Add(30 * 24 * time.Hour)),
			expected: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Pass the third argument, the fallback common name
			UpdateCertValidDuration(tt.cert, time.Now(), tt.cert.Subject.CommonName)

			metric, err := MetricCertValidDuration.GetMetricWithLabelValues(tt.cert.Subject.CommonName)
			if err != nil {
				t.Fatalf("Error getting metric: %v", err)
			}

			if value := int(testutil.ToFloat64(metric)); value != int(tt.expected) {
				t.Errorf("Expected %d (%.2f), but got %d", int(tt.expected), tt.expected, value)
			}
		})
	}
}
