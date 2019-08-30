package metrics

import "github.com/prometheus/client_golang/prometheus"

// metricsConfig allows user to specify how to send information to the prometheus instance.
type metricsConfig struct {
	metricsPath        string
	metricsPort        string
	serviceName        string
	collectorList      []prometheus.Collector
	withRoute          bool
	withServiceMonitor bool
}
