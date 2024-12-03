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
	createTestCert := func(notAfter time.Time, commonName string) *x509.Certificate {
		return &x509.Certificate{
			NotAfter: notAfter,
			Subject:  pkix.Name{CommonName: commonName},
		}
	}

	// Reset the metric before each test
	MetricCertValidDuration = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "certman_operator_certificate_valid_duration_days",
		Help: "The number of days for which the certificate remains valid",
	}, []string{"cn", "cluster"})

	tests := []struct {
		name        string
		cert        *x509.Certificate
		clusterName string
		expected    float64
	}{
		{
			name:        "Cert expired yesterday",
			cert:        createTestCert(time.Now().Add(-24*time.Hour), "test.example.com"),
			clusterName: "test-cluster",
			expected:    0,
		},
		{
			name:        "Cert expires tomorrow",
			cert:        createTestCert(time.Now().Add(24*time.Hour), "test.example.com"),
			clusterName: "test-cluster",
			expected:    1,
		},
		{
			name:        "Cert expires in 30 days",
			cert:        createTestCert(time.Now().Add(30*24*time.Hour), "test.example.com"),
			clusterName: "test-cluster",
			expected:    30,
		},
		{
			name:        "Nil certificate",
			cert:        nil,
			clusterName: "test-cluster",
			expected:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			UpdateCertValidDuration(nil, tt.cert, time.Now(), tt.clusterName, "") // Pass nil kubeClient
			var cn string
			if tt.cert != nil {
				cn = tt.cert.Subject.CommonName
			} else {
				cn = tt.clusterName
			}

			metric, err := MetricCertValidDuration.GetMetricWithLabelValues(cn, tt.clusterName)
			if err != nil {
				t.Fatalf("Error getting metric: %v", err)
			}

			if value := testutil.ToFloat64(metric); value != tt.expected {
				t.Errorf("Expected %.2f, but got %.2f", tt.expected, value)
			}
		})
	}
}
