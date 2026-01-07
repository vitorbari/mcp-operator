// Package metrics provides OpenTelemetry metrics collection for the MCP proxy.
package metrics

import (
	"go.opentelemetry.io/otel/metric"
)

const (
	// MeterName is the name of the OpenTelemetry meter.
	MeterName = "mcp-proxy"
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

// Instruments holds all OpenTelemetry metric instruments for the MCP proxy.
type Instruments struct {
	// RequestsTotal counts total requests by HTTP status code and MCP method.
	RequestsTotal metric.Int64Counter

	// RequestDuration tracks request duration in seconds.
	RequestDuration metric.Float64Histogram

	// RequestSize tracks request body size in bytes.
	RequestSize metric.Float64Histogram

	// ResponseSize tracks response body size in bytes.
	ResponseSize metric.Float64Histogram

	// ActiveConnections tracks the number of active connections.
	ActiveConnections metric.Int64UpDownCounter

	// ToolCallsTotal counts total tool call requests by tool name.
	ToolCallsTotal metric.Int64Counter

	// ResourceReadsTotal counts total resource read requests by resource URI.
	ResourceReadsTotal metric.Int64Counter

	// RequestErrorsTotal counts total JSON-RPC error responses by method and error code.
	RequestErrorsTotal metric.Int64Counter
}

// NewInstruments creates all metric instruments using the provided meter.
func NewInstruments(meter metric.Meter) (*Instruments, error) {
	requestsTotal, err := meter.Int64Counter(
		"mcp.requests.total",
		metric.WithDescription("Total number of HTTP requests by status code and MCP method."),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	requestDuration, err := meter.Float64Histogram(
		"mcp.request.duration",
		metric.WithDescription("HTTP request duration in seconds."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(DefaultDurationBuckets...),
	)
	if err != nil {
		return nil, err
	}

	requestSize, err := meter.Float64Histogram(
		"mcp.request.size",
		metric.WithDescription("HTTP request body size in bytes."),
		metric.WithUnit("By"),
		metric.WithExplicitBucketBoundaries(DefaultBytesBuckets...),
	)
	if err != nil {
		return nil, err
	}

	responseSize, err := meter.Float64Histogram(
		"mcp.response.size",
		metric.WithDescription("HTTP response body size in bytes."),
		metric.WithUnit("By"),
		metric.WithExplicitBucketBoundaries(DefaultBytesBuckets...),
	)
	if err != nil {
		return nil, err
	}

	activeConnections, err := meter.Int64UpDownCounter(
		"mcp.active_connections",
		metric.WithDescription("Number of active connections."),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return nil, err
	}

	toolCallsTotal, err := meter.Int64Counter(
		"mcp.tool_calls.total",
		metric.WithDescription("Total number of tool call requests by tool name."),
		metric.WithUnit("{call}"),
	)
	if err != nil {
		return nil, err
	}

	resourceReadsTotal, err := meter.Int64Counter(
		"mcp.resource_reads.total",
		metric.WithDescription("Total number of resource read requests by resource URI."),
		metric.WithUnit("{read}"),
	)
	if err != nil {
		return nil, err
	}

	requestErrorsTotal, err := meter.Int64Counter(
		"mcp.request_errors.total",
		metric.WithDescription("Total number of JSON-RPC error responses by method and error code."),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		return nil, err
	}

	return &Instruments{
		RequestsTotal:      requestsTotal,
		RequestDuration:    requestDuration,
		RequestSize:        requestSize,
		ResponseSize:       responseSize,
		ActiveConnections:  activeConnections,
		ToolCallsTotal:     toolCallsTotal,
		ResourceReadsTotal: resourceReadsTotal,
		RequestErrorsTotal: requestErrorsTotal,
	}, nil
}
