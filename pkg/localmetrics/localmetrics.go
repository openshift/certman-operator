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
	"time"

	"github.com/prometheus/client_golang/prometheus"
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

	MetricsList = []prometheus.Collector{
		MetricCertsIssuedInLastDayDevshiftOrg,
		MetricCertsIssuedInLastDayOpenshiftAppsCom,
		MetricCertsIssuedInLastWeekDevshiftOrg,
		MetricCertsIssuedInLastWeekOpenshiftAppsCom,
		MetricDuplicateCertsIssuedInLastWeek,
		MetricIssueCertificateDuration,
		MetricCertificateRequestReconcileDuration,
		MetricClusterDeploymentReconcileDuration,
	}
)

// UpdateCertsIssuedInLastDayGauge sets the gauge metric with the number of certs issued in last day
func UpdateCertsIssuedInLastDayGauge() {

	//Set these to certman calls
	devshiftCertCount := GetCountOfCertsIssued("devshift.org", 1)
	openshiftAppCertCount := GetCountOfCertsIssued("openshiftapps.com", 1)

	MetricCertsIssuedInLastDayDevshiftOrg.With(prometheus.Labels{"name": "certman-operator"}).Set(float64(devshiftCertCount))
	MetricCertsIssuedInLastDayOpenshiftAppsCom.With(prometheus.Labels{"name": "certman-operator"}).Set(float64(openshiftAppCertCount))
}

// UpdateCertsIssuedInLastWeekGuage sets the gauge metric with the number of certs issued in last week
func UpdateCertsIssuedInLastWeekGauge() {

	//Set these to certman calls
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
