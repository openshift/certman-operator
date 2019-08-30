package metrics

import "github.com/prometheus/client_golang/prometheus"

// Default variables for metrics-path and metrics-port.
const (
	defaultMetricsPath = "/customMetrics"
	defaultMetricsPort = "8089"
	defaultServiceName = "operator-metrics"
)

// metricsConfigBuilder builds a new metricsConfig object.
type metricsConfigBuilder struct {
	config metricsConfig
}

// NewBuilder sets the default values to the metricsConfig object.
func NewBuilder() *metricsConfigBuilder {
	return &metricsConfigBuilder{
		config: metricsConfig{
			metricsPath:   defaultMetricsPath,
			metricsPort:   defaultMetricsPort,
			serviceName:   defaultServiceName,
			collectorList: nil,
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

//WithName specifies the name of the service
func (b *metricsConfigBuilder) WithServiceName(name string) *metricsConfigBuilder {
	b.config.serviceName = name
	return b
}

// WithCollector appends the prometheus-collector provided by the user to a list of Collectors.
func (b *metricsConfigBuilder) WithCollector(collector prometheus.Collector) *metricsConfigBuilder {
	if b.config.collectorList == nil {
		b.config.collectorList = make([]prometheus.Collector, 0)
	}
	b.config.collectorList = append(b.config.collectorList, collector)
	return b
}

// WithCollectors updates the collectorList to the list of collectors provided by the user.
func (b *metricsConfigBuilder) WithCollectors(collectors []prometheus.Collector) *metricsConfigBuilder {
	b.config.collectorList = collectors
	return b
}

func (b *metricsConfigBuilder) WithRoute() *metricsConfigBuilder {
	b.config.withRoute = true
	return b
}

func (b *metricsConfigBuilder) WithServiceMonitor() *metricsConfigBuilder {
	b.config.withServiceMonitor = true
	return b
}
