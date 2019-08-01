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
	MetricCertsIssuedInLastDayOpenshiftCom = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "certman_operator_certs_in_last_day_openshift_com",
		Help: "Report how many certs have been issued for Openshift.com in the last 24 hours",
	}, []string{"name"})
	MetricCertsIssuedInLastDayOpenshiftAppsCom = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "certman_operator_certs_in_last_day_openshift_apps_com",
		Help: "Report how many certs have been issued for Openshiftapps.com in the last 24 hours",
	}, []string{"name"})
	MetricCertsIssuedInLastWeekOpenshiftCom = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "certman_operator_certs_in_last_week_openshift_com",
		Help: "Report how many certs have been issued for Openshift.com in the last 7 days",
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
		Name: "certman_operator_certificate_creation_duration",
		Help: "Runtime of issue certificate function in seconds",
	})

	MetricsList = []prometheus.Collector{
		MetricCertsIssuedInLastDayOpenshiftCom,
		MetricCertsIssuedInLastDayOpenshiftAppsCom,
		MetricCertsIssuedInLastWeekOpenshiftCom,
		MetricCertsIssuedInLastWeekOpenshiftAppsCom,
		MetricDuplicateCertsIssuedInLastWeek,
		MetricIssueCertificateDuration,
	}
)

// UpdateCertsIssuedInLastDayGuage sets the gauge metric with the number of certs issued in last day
func UpdateCertsIssuedInLastDayGuage() {

	//Set these to certman calls
	openshiftCertCount := GetCountOfCertsIssued("openshift.com", 1)
	openshiftAppCertCount := GetCountOfCertsIssued("openshiftapps.com", 1)

	MetricCertsIssuedInLastDayOpenshiftCom.With(prometheus.Labels{"name": "certman-operator"}).Set(float64(openshiftCertCount))
	MetricCertsIssuedInLastDayOpenshiftAppsCom.With(prometheus.Labels{"name": "certman-operator"}).Set(float64(openshiftAppCertCount))
}

// UpdateCertsIssuedInLastWeekGuage sets the gauge metric with the number of certs issued in last week
func UpdateCertsIssuedInLastWeekGuage() {

	//Set these to certman calls
	openshiftCertCount := GetCountOfCertsIssued("openshift.com", 7)
	openshiftAppCertCount := GetCountOfCertsIssued("openshiftapps.com", 7)

	MetricCertsIssuedInLastWeekOpenshiftCom.With(prometheus.Labels{"name": "certman-operator"}).Set(float64(openshiftCertCount))
	MetricCertsIssuedInLastWeekOpenshiftAppsCom.With(prometheus.Labels{"name": "certman-operator"}).Set(float64(openshiftAppCertCount))
}

// UpdateDuplicateCertsIssuedInLastWeek ...
func UpdateDuplicateCertsIssuedInLastWeek() {

}

// UpdateMetrics updates all the metrics every N hours
func UpdateMetrics(hour int) {

	d := time.Duration(hour) * time.Hour
	for range time.Tick(d) {
		UpdateCertsIssuedInLastDayGuage()
		UpdateCertsIssuedInLastWeekGuage()
		UpdateDuplicateCertsIssuedInLastWeek()
	}
}

func UpdateCertificateCreationDurationMetric(time time.Duration) {
	MetricIssueCertificateDuration.Observe(float64(time.Seconds()))
}
