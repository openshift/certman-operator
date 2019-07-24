package example

import (
	"context"
	"time"

	"github.com/openshift/operator-custom-metrics/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"

	log "github.com/sirupsen/logrus"
)

//Metrics endpoint and path which is to be used to expose metrics.
const (
	metricsEndPoint = "8080"
	metricsPath     = "/metrics"
)

//Metric variables which are to be collected.
var (
	opsProcessed = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "myapp_processed_ops_total",
		Help: "The total number of processed events Test",
	})
)

// RecordMetrics updates the values of the metrics which are to be collected.
func RecordMetrics() {
	go func() {
		for {
			opsProcessed.Inc()
			time.Sleep(2 * time.Second)
		}
	}()
}

//TestConfigMetrics creates a metricsConfig object and passes its reference to the library.
func TestConfigMetrics() {

	prTest := metrics.NewBuilder().
		WithPort(metricsEndPoint).
		WithPath(metricsPath).
		WithCollectors(opsProcessed).
		WithMetricsFunction(RecordMetrics).
		GetConfig()

	// Start metrics server with the exposed metrics.
	if err := metrics.ConfigureMetrics(context.TODO(), *prTest); err != nil {
		log.Error(err, "Fail")
	}
}
