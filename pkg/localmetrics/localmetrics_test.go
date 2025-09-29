package localmetrics

import (
	"context"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"
	"time"

	certmanv1alpha1 "github.com/openshift/certman-operator/api/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

// mockClient implements client.Client for testing CheckInitCounter
type mockClient struct {
	client.Client
	listFunc func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error
}

func (m *mockClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return m.listFunc(ctx, list, opts...)
}

func resetCheckInitCounterState() {
	areCountInitialized = false
	MetricCertRequestsCount.Set(0)
}

func TestCheckInitCounter(t *testing.T) {
	// Save and restore global state
	defer func(orig bool) { areCountInitialized = orig }(areCountInitialized)

	tests := []struct {
		name               string
		certRequests       []certmanv1alpha1.CertificateRequest
		finalizerLabel     string
		listErr            error
		expectedMetric     float64
		expectedInitStatus bool
	}{
		{
			name:               "No CertificateRequests",
			certRequests:       []certmanv1alpha1.CertificateRequest{},
			finalizerLabel:     certmanv1alpha1.CertmanOperatorFinalizerLabel,
			expectedMetric:     0,
			expectedInitStatus: true,
		},
		{
			name: "One CertificateRequest with finalizer",
			certRequests: []certmanv1alpha1.CertificateRequest{
				{ObjectMeta: metav1.ObjectMeta{Finalizers: []string{certmanv1alpha1.CertmanOperatorFinalizerLabel}}},
			},
			finalizerLabel:     certmanv1alpha1.CertmanOperatorFinalizerLabel,
			expectedMetric:     1,
			expectedInitStatus: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetCheckInitCounterState()
			mockclient := &mockClient{
				listFunc: func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
					if tt.listErr != nil {
						return tt.listErr
					}
					crList, ok := list.(*certmanv1alpha1.CertificateRequestList)
					if !ok {
						t.Fatalf("expected CertificateRequestList, got %T", list)
					}
					crList.Items = tt.certRequests
					return nil
				},
			}
			CheckInitCounter(mockclient)
			got := testutil.ToFloat64(MetricCertRequestsCount)
			if got != tt.expectedMetric {
				t.Errorf("expected metric value %.0f, got %.0f", tt.expectedMetric, got)
			}
			if areCountInitialized != tt.expectedInitStatus {
				t.Errorf("expected areCountInitialized=%v, got %v", tt.expectedInitStatus, areCountInitialized)
			}
		})
	}
}

func TestClusterInFullSupport(t *testing.T) {
	t.Run("success_with_before_after_validation", func(t *testing.T) {

		// Reset the metric before the test
		MetricLimitedSupportCluster.Reset()

		testClusterName := "test-cluster-full-support"
		testNamespace := "test-namespace-full"

		ClusterInLimitedSupport(testClusterName, testNamespace)

		beforeMetric, err := MetricLimitedSupportCluster.GetMetricWithLabelValues(testClusterName, testNamespace)
		if err != nil {
			t.Fatalf("Failed to get initial metric: %v", err)
		}
		beforeValue := testutil.ToFloat64(beforeMetric)
		if beforeValue != 1.0 {
			t.Fatalf("Expected initial value to be 1.0 (limited support), got %f", beforeValue)
		}

		ClusterInFullSupport(testClusterName, testNamespace)

		afterMetric, err := MetricLimitedSupportCluster.GetMetricWithLabelValues(testClusterName, testNamespace)
		if err != nil {
			t.Fatalf("Expected metric to exist after moving to full support: %v", err)
		}
		afterValue := testutil.ToFloat64(afterMetric)
		expectedValue := 0.0
		if afterValue != expectedValue {
			t.Errorf("Expected metric value to be %f (full support), got %f", expectedValue, afterValue)
		}
		if beforeValue == 1.0 && afterValue == 0.0 {
			t.Log("SUCCESS: Cluster correctly moved from limited support to full support")
		} else {
			t.Errorf("FAILED: Expected transition from 1.0 to 0.0, got %f to %f", beforeValue, afterValue)
		}
	})
}

func TestClearCertValidDuration(t *testing.T) {
	// Reset the metric before the test
	MetricCertValidDuration.Reset()

	cn := "example.com"
	namespace := "default"
	name := "cert-req"

	MetricCertValidDuration.With(prometheus.Labels{
		"cn":                           cn,
		"certificaterequest_name":      name,
		"certificaterequest_namespace": namespace,
	}).Set(90)

	ClearCertValidDuration(namespace, name)

	count := testutil.CollectAndCount(MetricCertValidDuration)
	if count != 0 {
		t.Errorf("Expected metric to be deleted, but found %d metric(s)", count)
	}
}
