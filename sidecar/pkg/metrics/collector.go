// Package metrics provides Prometheus metrics collection for the MCP proxy.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// Namespace for all MCP proxy metrics.
	namespace = "mcp"
)

var (
	// DefaultDurationBuckets are histogram buckets for request duration in seconds.
	// Ranges from 1ms to 10s to cover typical MCP request latencies.
	DefaultDurationBuckets = []float64{
		0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
	}

	// DefaultBytesBuckets are histogram buckets for request/response sizes in bytes.
	// Ranges from 100 bytes to 1MB to cover typical MCP message sizes.
	DefaultBytesBuckets = []float64{
		100, 1000, 10000, 100000, 1000000,
	}
)

// Collector holds all Prometheus metrics for the MCP proxy.
type Collector struct {
	// RequestsTotal counts total requests by HTTP status code.
	RequestsTotal *prometheus.CounterVec

	// RequestDuration tracks request duration in seconds.
	RequestDuration prometheus.Histogram

	// RequestSize tracks request body size in bytes.
	RequestSize prometheus.Histogram

	// ResponseSize tracks response body size in bytes.
	ResponseSize prometheus.Histogram

	// ActiveConnections tracks the number of active connections.
	ActiveConnections prometheus.Gauge

	// ProxyInfo is a static gauge with proxy metadata (always set to 1).
	ProxyInfo *prometheus.GaugeVec
}

// NewCollector creates a new Collector with all metrics registered.
func NewCollector() *Collector {
	c := &Collector{
		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "requests_total",
				Help:      "Total number of HTTP requests by status code.",
			},
			[]string{"status"},
		),

		RequestDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "request_duration_seconds",
				Help:      "HTTP request duration in seconds.",
				Buckets:   DefaultDurationBuckets,
			},
		),

		RequestSize: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "request_size_bytes",
				Help:      "HTTP request body size in bytes.",
				Buckets:   DefaultBytesBuckets,
			},
		),

		ResponseSize: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "response_size_bytes",
				Help:      "HTTP response body size in bytes.",
				Buckets:   DefaultBytesBuckets,
			},
		),

		ActiveConnections: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "active_connections",
				Help:      "Number of active connections.",
			},
		),

		ProxyInfo: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "proxy_info",
				Help:      "Static proxy information (always 1).",
			},
			[]string{"version", "target"},
		),
	}

	return c
}

// Register registers all metrics with the given registry.
func (c *Collector) Register(registry prometheus.Registerer) error {
	collectors := []prometheus.Collector{
		c.RequestsTotal,
		c.RequestDuration,
		c.RequestSize,
		c.ResponseSize,
		c.ActiveConnections,
		c.ProxyInfo,
	}

	for _, collector := range collectors {
		if err := registry.Register(collector); err != nil {
			return err
		}
	}

	return nil
}

// MustRegister registers all metrics with the given registry and panics on error.
func (c *Collector) MustRegister(registry prometheus.Registerer) {
	registry.MustRegister(
		c.RequestsTotal,
		c.RequestDuration,
		c.RequestSize,
		c.ResponseSize,
		c.ActiveConnections,
		c.ProxyInfo,
	)
}
