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

package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	// MetricsEndpoint is the port to export metrics on
	MetricsEndpoint = ":8080"
)

var (
	metricCertsIssuedInLastDayOpenshiftCom = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "certman_operator_certs_in_last_day_openshift_com",
		Help: "Report how many certs have been issued for Openshift.com in the last 24 hours",
	}, []string{"name"})
	metricCertsIssuedInLastDayOpenshiftAppsCom = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "certman_operator_certs_in_last_day_openshift_apps_com",
		Help: "Report how many certs have been issued for Openshiftapps.com in the last 24 hours",
	}, []string{"name"})
	metricCertsIssuedInLastWeekOpenshiftCom = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "certman_operator_certs_in_last_week_openshift_com",
		Help: "Report how many certs have been issued for Openshift.com in the last 7 days",
	}, []string{"name"})
	metricCertsIssuedInLastWeekOpenshiftAppsCom = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "certman_operator_certs_in_last_week_openshift_apps_com",
		Help: "Report how many certs have been issued for Openshiftapps.com in the last 7 days",
	}, []string{"name"})
	metricDuplicateCertsIssuedInLastWeek = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "certman_operator_duplicate_certs_in_last_week",
		Help: "Report how many certs have had duplicate issues",
	}, []string{"name"})

	metricsList = []prometheus.Collector{
		metricCertsIssuedInLastDayOpenshiftCom,
		metricCertsIssuedInLastDayOpenshiftAppsCom,
		metricCertsIssuedInLastWeekOpenshiftCom,
		metricCertsIssuedInLastWeekOpenshiftAppsCom,
		metricDuplicateCertsIssuedInLastWeek,
	}
)

// StartMetrics register metrics and exposes them
func StartMetrics() {
	// Register metrics and start serving them on /metrics endpoint
	RegisterMetrics()
	http.Handle("/metrics", prometheus.Handler())
	go http.ListenAndServe(MetricsEndpoint, nil)
}

// RegisterMetrics for the operator
func RegisterMetrics() error {
	for _, metric := range metricsList {
		err := prometheus.Register(metric)
		if err != nil {
			return err
		}
	}
	return nil
}

// UpdateCertsIssuedInLastDayGuage sets the gauge metric with the number of certs issued in last day
func UpdateCertsIssuedInLastDayGuage() {

	//Set these to certman calls
	openshiftCertCount := GetCountOfCertsIssued("openshift.com", 1)
	openshiftAppCertCount := GetCountOfCertsIssued("openshiftapps.com", 1)

	metricCertsIssuedInLastDayOpenshiftCom.With(prometheus.Labels{"name": "certman-operator"}).Set(float64(openshiftCertCount))
	metricCertsIssuedInLastDayOpenshiftAppsCom.With(prometheus.Labels{"name": "certman-operator"}).Set(float64(openshiftAppCertCount))
}

// UpdateCertsIssuedInLastWeekGuage sets the gauge metric with the number of certs issued in last week
func UpdateCertsIssuedInLastWeekGuage() {

	//Set these to certman calls
	openshiftCertCount := GetCountOfCertsIssued("openshift.com", 7)
	openshiftAppCertCount := GetCountOfCertsIssued("openshiftapps.com", 7)

	metricCertsIssuedInLastWeekOpenshiftCom.With(prometheus.Labels{"name": "certman-operator"}).Set(float64(openshiftCertCount))
	metricCertsIssuedInLastWeekOpenshiftAppsCom.With(prometheus.Labels{"name": "certman-operator"}).Set(float64(openshiftAppCertCount))
}

// UpdateDuplicateCertsIssuedInLastWeek ...
func UpdateDuplicateCertsIssuedInLastWeek() {

}
