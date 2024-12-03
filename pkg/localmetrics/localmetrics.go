// Copyright 2019 RedHat
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package localmetrics

import (
	"context"
	"crypto/x509"
	"math"
	"time"

	certmanv1alpha1 "github.com/openshift/certman-operator/api/v1alpha1"
	"github.com/openshift/certman-operator/controllers/utils"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	MetricCertsIssuedInLastDayDevshiftOrg = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "certman_operator_certs_in_last_day_devshift_org",
		Help: "Report how many certs have been issued for Devshift.org in the last 24 hours",
	}, []string{"name"})
	MetricCertsIssuedInLastDayOpenshiftAppsCom = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "certman_operator_certs_in_last_day_openshift_apps_com",
		Help: "Report how many certs have been issued for Openshiftapps.com in the last 24 hours",
	}, []string{"name"})
	MetricCertsIssuedInLastWeekDevshiftOrg = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "certman_operator_certs_in_last_week_devshift_org",
		Help: "Report how many certs have been issued for Devshift.org in the last 7 days",
	}, []string{"name"})
	MetricCertsIssuedInLastWeekOpenshiftAppsCom = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "certman_operator_certs_in_last_week_openshift_apps_com",
		Help: "Report how many certs have been issued for Openshiftapps.com in the last 7 days",
	}, []string{"name"})
	MetricDuplicateCertsIssuedInLastWeek = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "certman_operator_duplicate_certs_in_last_week",
		Help: "Report how many certs have had duplicate issues",
	}, []string{"name"})
	MetricIssueCertificateDuration = prometheus.NewSummary(prometheus.SummaryOpts{
		Name:        "certman_operator_certificate_issue_duration",
		Help:        "Runtime of issue certificate function in seconds",
		ConstLabels: prometheus.Labels{"name": "certman-operator"},
	})
	MetricCertificateRequestReconcileDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:        "certman_operator_certificate_request_reconcile_duration_seconds",
		Help:        "The duration it takes to reconcile a CertificateRequest",
		ConstLabels: prometheus.Labels{"name": "certman-operator"},
	})
	MetricClusterDeploymentReconcileDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:        "certman_operator_cluster_deployment_reconcile_duration_seconds",
		Help:        "The duration it takes to reconcile a ClusterDeployment",
		ConstLabels: prometheus.Labels{"name": "certman-operator"},
	})
	MetricCertRequestsCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "certman_operator_certificate_requests_count",
		Help:        "Report the current count of Certificate Requests",
		ConstLabels: prometheus.Labels{"name": "certman-operator"},
	})
	MetricCertIssuanceRate = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "certman_operator_issued_certificates_count",
		Help: "Counter on the number of issued certificate",
	}, []string{"name", "action"})
	MetricCertValidDuration = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "certman_operator_certificate_valid_duration_days",
		Help:        "The number of days for which the certificate remains valid",
		ConstLabels: prometheus.Labels{"name": "certman-operator"},
	}, []string{"cn", "cluster"})
	MetricLetsEncryptMaintenanceErrorCount = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "certman_operator_lets_encrypt_maintenance_error_count",
		Help: "The number of Let's Encrypt maintenance errors received",
	})
	MetricDnsErrorCount = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cloudflare_failed_requests_count",
		Help: "Counter on the number of failed DNS requests",
	})

	MetricsList = []prometheus.Collector{
		MetricCertsIssuedInLastDayDevshiftOrg,
		MetricCertsIssuedInLastDayOpenshiftAppsCom,
		MetricCertsIssuedInLastWeekDevshiftOrg,
		MetricCertsIssuedInLastWeekOpenshiftAppsCom,
		MetricDuplicateCertsIssuedInLastWeek,
		MetricIssueCertificateDuration,
		MetricCertificateRequestReconcileDuration,
		MetricClusterDeploymentReconcileDuration,
		MetricCertRequestsCount,
		MetricCertIssuanceRate,
		MetricDnsErrorCount,
		MetricCertValidDuration,
		MetricLetsEncryptMaintenanceErrorCount,
	}
	areCountInitialized = false
	logger              = logf.Log.WithName("localmetrics")
)

// Init Initialize the counter at start of the operator
// Current version does not support well multiple instances of the operator to run on the same Hive cluster
// In case of error, we don't raise the error as not impactful and Init will be retried next call, pushing correct value
func CheckInitCounter(c client.Client) {
	if !areCountInitialized {
		ctx := context.TODO()
		counter := 0.0

		var certRequestList certmanv1alpha1.CertificateRequestList

		if err := c.List(ctx, &certRequestList, &client.ListOptions{}); err != nil {
			logger.Error(err, "Failed to Init counter for Certificate Request")
		}

		for _, cr := range certRequestList.Items {
			if utils.ContainsString(cr.ObjectMeta.Finalizers, certmanv1alpha1.CertmanOperatorFinalizerLabel) {
				counter++
			}
		}

		MetricCertRequestsCount.Set(counter)
		areCountInitialized = true
	}
}

// UpdateCertsIssuedInLastDayGauge sets the gauge metric with the number of certs issued in last day
func UpdateCertsIssuedInLastDayGauge() {

	// Set these to certman calls
	devshiftCertCount := GetCountOfCertsIssued("devshift.org", 1)
	openshiftAppCertCount := GetCountOfCertsIssued("openshiftapps.com", 1)

	MetricCertsIssuedInLastDayDevshiftOrg.With(prometheus.Labels{"name": "certman-operator"}).Set(float64(devshiftCertCount))
	MetricCertsIssuedInLastDayOpenshiftAppsCom.With(prometheus.Labels{"name": "certman-operator"}).Set(float64(openshiftAppCertCount))
}

// UpdateCertsIssuedInLastWeekGauge sets the gauge metric with the number of certs issued in last week
func UpdateCertsIssuedInLastWeekGauge() {

	// Set these to certman calls
	devshiftCertCount := GetCountOfCertsIssued("devshift.org", 7)
	openshiftAppCertCount := GetCountOfCertsIssued("openshiftapps.com", 7)

	MetricCertsIssuedInLastWeekDevshiftOrg.With(prometheus.Labels{"name": "certman-operator"}).Set(float64(devshiftCertCount))
	MetricCertsIssuedInLastWeekOpenshiftAppsCom.With(prometheus.Labels{"name": "certman-operator"}).Set(float64(openshiftAppCertCount))
}

// UpdateMetrics updates all the metrics every N hours
func UpdateMetrics(hour int) {
	d := time.Duration(hour) * time.Hour
	for range time.Tick(d) {
		UpdateCertsIssuedInLastDayGauge()
		UpdateCertsIssuedInLastWeekGauge()
	}
}

// IncrementCertRequestsCounter Increment the count of certificate requests
func IncrementCertRequestsCounter() {
	MetricCertRequestsCount.Inc()
}

// DecrementCertRequestsCounter Decrement the count of certificate requests
func DecrementCertRequestsCounter() {
	MetricCertRequestsCount.Dec()
}

// AddCertificateIssuance Increment the count of issued certificate
func AddCertificateIssuance(action string) {
	MetricCertIssuanceRate.With(prometheus.Labels{"name": "certman-operator", "action": action}).Inc()
}

// UpdateCertValidDuration updates the Prometheus metric for certificate validity duration.
func UpdateCertValidDuration(kubeClient client.Client, cert *x509.Certificate, now time.Time, clusterName, namespace string) {
	if kubeClient == nil {
		// If kubeClient is nil, set the metric value based on the certificate's expiration date
		if cert != nil {
			diff := cert.NotAfter.Sub(now)
			days := math.Max(0, math.Round(diff.Hours()/24))
			MetricCertValidDuration.With(prometheus.Labels{
				"cn":      cert.Subject.CommonName,
				"cluster": clusterName,
			}).Set(days)
		} else {
			MetricCertValidDuration.With(prometheus.Labels{
				"cn":      clusterName,
				"cluster": clusterName,
			}).Set(0)
		}
		return
	}

	// Check if the cluster is decommissioned
	if isClusterDecommissioned(kubeClient, clusterName, namespace) {
		logger.Info("Cluster is decommissioned, skipping UpdateCertValidDuration", "clusterName", clusterName)
		return
	}

	// Set metric based on certificate expiration date if available
	var days float64
	var cn string
	if cert != nil {
		diff := cert.NotAfter.Sub(now)
		days = math.Max(0, math.Round(diff.Hours()/24))
		cn = cert.Subject.CommonName
	} else {
		days = 0
		cn = clusterName
	}

	MetricCertValidDuration.With(prometheus.Labels{
		"cn":      cn,
		"cluster": clusterName,
	}).Set(days)
}

// Helper function to check if a cluster is decommissioned
func isClusterDecommissioned(kubeClient client.Client, clusterName, namespace string) bool {
	decommissioned, err := IsClusterDecommissioned(kubeClient, clusterName, namespace)
	if err != nil {
		logger.Error(err, "Error checking if cluster is decommissioned", "clusterName", clusterName)
		return false
	}
	return decommissioned
}

// IsClusterDecommissioned checks if the cluster has been deleted based on its hibernation reason
func IsClusterDecommissioned(kubeClient client.Client, clusterName, namespace string) (bool, error) {
	cluster := &hivev1.ClusterDeployment{}
	err := kubeClient.Get(context.TODO(), types.NamespacedName{Name: clusterName, Namespace: namespace}, cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			return true, nil // If the cluster resource is not found, consider it decommissioned
		}
		return false, err
	}

	// Check conditions for HibernatingReasonClusterDeploymentDeleted
	for _, condition := range cluster.Status.Conditions {
		if condition.Type == "Hibernating" && condition.Reason == "ClusterDeploymentDeleted" {
			return true, nil
		}
	}

	return false, nil
}

// IncrementLetsEncryptMaintenanceErrorCount Increment the count of Let's Encrypt maintenance errors
func IncrementLetsEncryptMaintenanceErrorCount() {
	MetricLetsEncryptMaintenanceErrorCount.Inc()
}

// IncrementDnsErrorCount Increment the count of DNS errors
func IncrementDnsErrorCount() {
	MetricDnsErrorCount.Inc()
}
