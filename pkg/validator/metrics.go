/*
Copyright 2025 Vitor Bari.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package validator

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// validationDuration tracks how long validations take
	validationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mcp_validator_duration_seconds",
			Help:    "Time spent validating MCP servers",
			Buckets: []float64{0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0},
		},
		[]string{"transport", "success"},
	)

	// detectionAttempts counts transport detection attempts
	detectionAttempts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_validator_detection_attempts_total",
			Help: "Number of transport detection attempts",
		},
		[]string{"transport", "success"},
	)

	// validationRetries tracks retry behavior
	validationRetries = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mcp_validator_retries",
			Help:    "Number of retry attempts during validation",
			Buckets: []float64{0, 1, 2, 3, 5, 10},
		},
		[]string{"transport"},
	)

	// validationErrors counts different error types
	validationErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_validator_errors_total",
			Help: "Number of validation errors by type",
		},
		[]string{"error_code", "transport"},
	)

	// protocolVersions tracks protocol version distribution
	protocolVersions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_validator_protocol_versions_total",
			Help: "Distribution of MCP protocol versions detected",
		},
		[]string{"version"},
	)

	// validationTotal counts all validation operations
	validationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_validator_validations_total",
			Help: "Total number of validation operations",
		},
		[]string{"transport", "result"},
	)
)

// MetricsConfig controls metric registration for the validator
type MetricsConfig struct {
	// Register controls whether to register metrics with Prometheus
	Register bool
	// Registry is the Prometheus registry to use (defaults to controller-runtime metrics.Registry)
	Registry prometheus.Registerer
}

// RegisterMetrics explicitly registers validator metrics with a Prometheus registry.
// This must be called by applications that want validator metrics.
// It's safe to call multiple times - already registered metrics are ignored.
func RegisterMetrics(config MetricsConfig) error {
	if !config.Register {
		return nil
	}

	registry := config.Registry
	if registry == nil {
		// Default to controller-runtime metrics registry
		registry = metrics.Registry
	}

	collectors := []prometheus.Collector{
		validationDuration,
		detectionAttempts,
		validationRetries,
		validationErrors,
		protocolVersions,
		validationTotal,
	}

	for _, collector := range collectors {
		if err := registry.Register(collector); err != nil {
			// If already registered, that's okay - ignore the error
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return err
			}
		}
	}

	return nil
}

// MetricsRecorder handles recording validation metrics
// This interface allows for testing and optional metric collection
type MetricsRecorder interface {
	RecordValidation(transport string, success bool, duration time.Duration)
	RecordDetection(transport string, success bool)
	RecordRetries(transport string, retries int)
	RecordError(errorCode string, transport string)
	RecordProtocolVersion(version string)
}

// PrometheusMetricsRecorder implements MetricsRecorder using Prometheus
type PrometheusMetricsRecorder struct {
	enabled bool
}

// NewMetricsRecorder creates a new metrics recorder
// If enabled is false, the recorder will be a no-op (no metrics recorded)
func NewMetricsRecorder(enabled bool) MetricsRecorder {
	return &PrometheusMetricsRecorder{enabled: enabled}
}

// RecordValidation records a validation operation
func (r *PrometheusMetricsRecorder) RecordValidation(transport string, success bool, duration time.Duration) {
	if !r.enabled {
		return
	}

	successLabel := "false"
	if success {
		successLabel = "true"
	}

	validationDuration.WithLabelValues(transport, successLabel).Observe(duration.Seconds())
	validationTotal.WithLabelValues(transport, successLabel).Inc()
}

// RecordDetection records a transport detection attempt
func (r *PrometheusMetricsRecorder) RecordDetection(transport string, success bool) {
	if !r.enabled {
		return
	}

	successLabel := "false"
	if success {
		successLabel = "true"
	}

	detectionAttempts.WithLabelValues(transport, successLabel).Inc()
}

// RecordRetries records the number of retry attempts
func (r *PrometheusMetricsRecorder) RecordRetries(transport string, retries int) {
	if !r.enabled {
		return
	}

	validationRetries.WithLabelValues(transport).Observe(float64(retries))
}

// RecordError records a validation error by type
func (r *PrometheusMetricsRecorder) RecordError(errorCode string, transport string) {
	if !r.enabled {
		return
	}

	validationErrors.WithLabelValues(errorCode, transport).Inc()
}

// RecordProtocolVersion records a detected protocol version
func (r *PrometheusMetricsRecorder) RecordProtocolVersion(version string) {
	if !r.enabled {
		return
	}

	protocolVersions.WithLabelValues(version).Inc()
}

// NoOpMetricsRecorder is a no-op implementation for testing
type NoOpMetricsRecorder struct{}

// NewNoOpMetricsRecorder creates a metrics recorder that does nothing
func NewNoOpMetricsRecorder() MetricsRecorder {
	return &NoOpMetricsRecorder{}
}

func (r *NoOpMetricsRecorder) RecordValidation(transport string, success bool, duration time.Duration) {
}

func (r *NoOpMetricsRecorder) RecordDetection(transport string, success bool) {}

func (r *NoOpMetricsRecorder) RecordRetries(transport string, retries int) {}

func (r *NoOpMetricsRecorder) RecordError(errorCode string, transport string) {}

func (r *NoOpMetricsRecorder) RecordProtocolVersion(version string) {}

// Global default metrics recorder (enabled by default for backward compatibility)
var defaultMetricsRecorder MetricsRecorder = NewMetricsRecorder(true)

// SetDefaultMetricsRecorder allows replacing the default recorder (useful for testing)
func SetDefaultMetricsRecorder(recorder MetricsRecorder) {
	defaultMetricsRecorder = recorder
}

// GetDefaultMetricsRecorder returns the current default recorder
func GetDefaultMetricsRecorder() MetricsRecorder {
	return defaultMetricsRecorder
}

// Convenience functions using the default recorder

// RecordValidation records a validation operation using the default recorder
func RecordValidation(transport string, success bool, duration time.Duration) {
	defaultMetricsRecorder.RecordValidation(transport, success, duration)
}

// RecordDetection records a detection attempt using the default recorder
func RecordDetection(transport string, success bool) {
	defaultMetricsRecorder.RecordDetection(transport, success)
}

// RecordRetries records retry attempts using the default recorder
func RecordRetries(transport string, retries int) {
	defaultMetricsRecorder.RecordRetries(transport, retries)
}

// RecordError records an error using the default recorder
func RecordError(errorCode string, transport string) {
	defaultMetricsRecorder.RecordError(errorCode, transport)
}

// RecordProtocolVersion records a protocol version using the default recorder
func RecordProtocolVersion(version string) {
	defaultMetricsRecorder.RecordProtocolVersion(version)
}
