// Copyright 2018 RedHat
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
		Help: "Report how many certs have been issued for Openshift.com in the last day",
	}, []string{"name"})
	metricCertsIssuedInLastDayOpenshiftAppsCom = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "certman_operator_certs_in_last_day_openshift_apps_com",
		Help: "Report how many certs have been issued for Openshiftapps.com in the last day",
	}, []string{"name"})
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
	err := prometheus.Register(metricCertsIssuedInLastDayOpenshiftCom)
	err = prometheus.Register(metricCertsIssuedInLastDayOpenshiftAppsCom)
	return err
}

// UpdateCertsIssuedInLastDayGuage sets the gauge metric with the number of certs issued in last day for openshift.com
func UpdateCertsIssuedInLastDayGuage() {

	//Set these to certman calls
	openshiftCertCount := float64(0)
	openshiftAppCertCount := float64(0)

	metricCertsIssuedInLastDayOpenshiftCom.Reset()
	metricCertsIssuedInLastDayOpenshiftCom.With(prometheus.Labels{"name": "certman-operator"}).Set(openshiftCertCount)
	metricCertsIssuedInLastDayOpenshiftAppsCom.Reset()
	metricCertsIssuedInLastDayOpenshiftAppsCom.With(prometheus.Labels{"name": "certman-operator"}).Set(openshiftAppCertCount)
}
