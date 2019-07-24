package metrics

import "github.com/prometheus/client_golang/prometheus"

// Default variables for metrics-path and metrics-port.
const (
	defaultMetricsPath = "/customMetrics"
	defaultMetricsPort = "8089"
)

// metricsConfigBuilder builds a new metricsConfig object.
type metricsConfigBuilder struct {
	config metricsConfig
}

// convertmetrics is a user-defined function, indicating how the metrics are collected.
type convertMetrics func()

// NewBuilder sets the default values to the metricsConfig object.
func NewBuilder() *metricsConfigBuilder {
	return &metricsConfigBuilder{
		config: metricsConfig{
			metricsPath:           defaultMetricsPath,
			metricsPort:           defaultMetricsPort,
			collectorList:         nil,
			recordMetricsFunction: nil,
		},
	}
}

// GetConfig returns the reference to the built metricsConfig.
func (p *metricsConfigBuilder) GetConfig() *metricsConfig {
	return &p.config
}

// WithPort updates the metrics port to the value provided by the user.
func (b *metricsConfigBuilder) WithPort(port string) *metricsConfigBuilder {
	b.config.metricsPort = port
	return b
}

// WithPath updates the metrics path to the value provided by the user.
func (b *metricsConfigBuilder) WithPath(path string) *metricsConfigBuilder {
	b.config.metricsPath = path
	return b
}

// WithCollectors appends the prometheus-collector provided by the user to a list of Collectors.
func (b *metricsConfigBuilder) WithCollectors(collector prometheus.Collector) *metricsConfigBuilder {
	if b.config.collectorList == nil {
		b.config.collectorList = make([]prometheus.Collector, 0)
	}
	b.config.collectorList = append(b.config.collectorList, collector)
	return b
}

// WithMetricsFunction updates the configuration with the user-defined function to update the metrics.
func (b *metricsConfigBuilder) WithMetricsFunction(recordMetricsFunction convertMetrics) *metricsConfigBuilder {
	b.config.recordMetricsFunction = recordMetricsFunction
	return b
}
