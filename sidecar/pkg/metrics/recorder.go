package metrics

import (
	"context"
	"net/http"
	"strconv"
	"time"

	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// Recorder provides methods to record metrics for the MCP proxy.
type Recorder struct {
	instruments   *Instruments
	meterProvider *sdkmetric.MeterProvider
	registry      *prom.Registry
	versionAttr   attribute.KeyValue
	targetAttr    attribute.KeyValue
}

// NewRecorder creates a new Recorder with the given version and target.
// It initializes OpenTelemetry with a Prometheus exporter.
func NewRecorder(version, target string) (*Recorder, error) {
	// Create a custom Prometheus registry
	registry := prom.NewRegistry()

	// Create the Prometheus exporter with the custom registry
	exporter, err := prometheus.New(prometheus.WithRegisterer(registry))
	if err != nil {
		return nil, err
	}

	// Create the MeterProvider with the Prometheus exporter
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exporter),
	)

	// Get a meter from the provider
	meter := meterProvider.Meter(MeterName,
		metric.WithInstrumentationVersion(version),
	)

	// Create instruments
	instruments, err := NewInstruments(meter)
	if err != nil {
		return nil, err
	}

	// Register the proxy info as a gauge callback
	_, err = meter.Float64ObservableGauge(
		"mcp.proxy.info",
		metric.WithDescription("Static proxy information (always 1)."),
		metric.WithFloat64Callback(func(_ context.Context, o metric.Float64Observer) error {
			o.Observe(1, metric.WithAttributes(
				attribute.String("version", version),
				attribute.String("target", target),
			))
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}

	return &Recorder{
		instruments:   instruments,
		meterProvider: meterProvider,
		registry:      registry,
		versionAttr:   attribute.String("version", version),
		targetAttr:    attribute.String("target", target),
	}, nil
}

// Handler returns an http.Handler that serves the Prometheus metrics endpoint.
func (r *Recorder) Handler() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// Shutdown gracefully shuts down the meter provider.
func (r *Recorder) Shutdown(ctx context.Context) error {
	return r.meterProvider.Shutdown(ctx)
}

// Instruments returns the underlying instruments for testing purposes.
func (r *Recorder) Instruments() *Instruments {
	return r.instruments
}

// MeterProvider returns the underlying MeterProvider for testing purposes.
func (r *Recorder) MeterProvider() *sdkmetric.MeterProvider {
	return r.meterProvider
}

// RecordRequest records metrics for a completed HTTP request.
// The method parameter is the MCP JSON-RPC method name (e.g., "tools/call", "initialize").
// Use "unknown" if the method could not be parsed.
func (r *Recorder) RecordRequest(ctx context.Context, method string, status int, duration time.Duration, reqSize, respSize int64) {
	attrs := []attribute.KeyValue{
		attribute.String("status", strconv.Itoa(status)),
		attribute.String("method", method),
	}

	// Record request count by status and method
	r.instruments.RequestsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))

	// Record request duration
	r.instruments.RequestDuration.Record(ctx, duration.Seconds())

	// Record request size (if positive)
	if reqSize > 0 {
		r.instruments.RequestSize.Record(ctx, float64(reqSize))
	}

	// Record response size (if positive)
	if respSize > 0 {
		r.instruments.ResponseSize.Record(ctx, float64(respSize))
	}
}

// RecordToolCall records a tool call request.
func (r *Recorder) RecordToolCall(ctx context.Context, toolName string) {
	r.instruments.ToolCallsTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("tool_name", toolName),
	))
}

// RecordResourceRead records a resource read request.
func (r *Recorder) RecordResourceRead(ctx context.Context, resourceURI string) {
	r.instruments.ResourceReadsTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("resource_uri", resourceURI),
	))
}

// RecordError records a JSON-RPC error response.
func (r *Recorder) RecordError(ctx context.Context, method string, errorCode int) {
	r.instruments.RequestErrorsTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("method", method),
		attribute.String("error_code", strconv.Itoa(errorCode)),
	))
}

// IncrementConnections increments the active connections counter.
func (r *Recorder) IncrementConnections(ctx context.Context) {
	r.instruments.ActiveConnections.Add(ctx, 1)
}

// DecrementConnections decrements the active connections counter.
func (r *Recorder) DecrementConnections(ctx context.Context) {
	r.instruments.ActiveConnections.Add(ctx, -1)
}

// SSEConnectionOpened records that an SSE connection was opened.
func (r *Recorder) SSEConnectionOpened(ctx context.Context) {
	r.instruments.SSEConnectionsTotal.Add(ctx, 1)
	r.instruments.SSEConnectionsActive.Add(ctx, 1)
}

// SSEConnectionClosed records that an SSE connection was closed with its duration.
func (r *Recorder) SSEConnectionClosed(ctx context.Context, duration time.Duration) {
	r.instruments.SSEConnectionsActive.Add(ctx, -1)
	r.instruments.SSEConnectionDuration.Record(ctx, duration.Seconds())
}

// SSEEventReceived records that an SSE event was received.
func (r *Recorder) SSEEventReceived(ctx context.Context, eventType string) {
	r.instruments.SSEEventsTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("event_type", eventType),
	))
}
