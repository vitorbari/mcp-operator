package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Recorder provides methods to record metrics for the MCP proxy.
type Recorder struct {
	collector *Collector
	registry  *prometheus.Registry
}

// NewRecorder creates a new Recorder with the given version and target.
// It initializes all metrics and sets the proxy_info gauge.
func NewRecorder(version, target string) *Recorder {
	collector := NewCollector()
	registry := prometheus.NewRegistry()

	// Register the collector with the registry
	collector.MustRegister(registry)

	// Set the static proxy info gauge
	collector.ProxyInfo.WithLabelValues(version, target).Set(1)

	return &Recorder{
		collector: collector,
		registry:  registry,
	}
}

// Registry returns the Prometheus registry for this recorder.
// Use this with promhttp.HandlerFor() to expose metrics.
func (r *Recorder) Registry() *prometheus.Registry {
	return r.registry
}

// Collector returns the underlying Collector for testing purposes.
func (r *Recorder) Collector() *Collector {
	return r.collector
}

// RecordRequest records metrics for a completed HTTP request.
func (r *Recorder) RecordRequest(status int, duration time.Duration, reqSize, respSize int64) {
	// Record request count by status
	statusStr := strconv.Itoa(status)
	r.collector.RequestsTotal.WithLabelValues(statusStr).Inc()

	// Record request duration
	r.collector.RequestDuration.Observe(duration.Seconds())

	// Record request size (if positive)
	if reqSize > 0 {
		r.collector.RequestSize.Observe(float64(reqSize))
	}

	// Record response size (if positive)
	if respSize > 0 {
		r.collector.ResponseSize.Observe(float64(respSize))
	}
}

// IncrementConnections increments the active connections gauge.
func (r *Recorder) IncrementConnections() {
	r.collector.ActiveConnections.Inc()
}

// DecrementConnections decrements the active connections gauge.
func (r *Recorder) DecrementConnections() {
	r.collector.ActiveConnections.Dec()
}
