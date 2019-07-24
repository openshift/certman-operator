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

// StartMetrics starts the server based on the metricsConfig provided by the user.
func StartMetrics(config metricsConfig) {
	// Register metrics only when the metric list is provided by the operator
	if config.collectorList != nil {
		RegisterMetrics(config.collectorList)
	}

	// Execute recordMetricsFunction if provided by the user
	if config.recordMetricsFunction != nil {
		config.recordMetricsFunction()
	}

	http.Handle(config.metricsPath, prometheus.Handler())
	log.Info("Port: %v", config.metricsPort)
	metricsPort := ":" + (config.metricsPort)
	go http.ListenAndServe(metricsPort, nil)
}

// RegisterMetrics takes the list of metrics to be registered from the user and
// registeres to prometheus.
func RegisterMetrics(list []prometheus.Collector) error {
	for _, metric := range list {
		err := prometheus.Register(metric)
		if err != nil {
			return err
		}
	}
	return nil
}
