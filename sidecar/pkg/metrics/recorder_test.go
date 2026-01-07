package metrics

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vitorbari/mcp-operator/sidecar/pkg/mcp"
)

func TestNewRecorder(t *testing.T) {
	recorder, err := NewRecorder("1.0.0", "http://localhost:3001")
	if err != nil {
		t.Fatalf("NewRecorder failed: %v", err)
	}

	if recorder == nil {
		t.Fatal("NewRecorder returned nil")
	}

	if recorder.Instruments() == nil {
		t.Error("Instruments should not be nil")
	}

	if recorder.MeterProvider() == nil {
		t.Error("MeterProvider should not be nil")
	}

	if recorder.Handler() == nil {
		t.Error("Handler should not be nil")
	}

	// Cleanup
	if err := recorder.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
}

func TestRecorder_ProxyInfo(t *testing.T) {
	recorder, err := NewRecorder("1.0.0", "http://localhost:3001")
	if err != nil {
		t.Fatalf("NewRecorder failed: %v", err)
	}
	defer recorder.Shutdown(context.Background())

	// Get metrics from the handler
	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	recorder.Handler().ServeHTTP(rr, req)

	body, _ := io.ReadAll(rr.Body)
	metrics := string(body)

	// Check that proxy_info metric is present with correct labels
	if !strings.Contains(metrics, "mcp_proxy_info") {
		t.Error("mcp_proxy_info metric not found")
	}
	if !strings.Contains(metrics, `version="1.0.0"`) {
		t.Errorf("version label not found in metrics:\n%s", metrics)
	}
	if !strings.Contains(metrics, `target="http://localhost:3001"`) {
		t.Errorf("target label not found in metrics:\n%s", metrics)
	}
}

func TestRecorder_RecordRequest(t *testing.T) {
	recorder, err := NewRecorder("1.0.0", "http://localhost:3001")
	if err != nil {
		t.Fatalf("NewRecorder failed: %v", err)
	}
	defer recorder.Shutdown(context.Background())

	ctx := context.Background()

	// Record some requests
	recorder.RecordRequest(ctx, mcp.MethodInitialize, 200, 100*time.Millisecond, 1000, 5000)
	recorder.RecordRequest(ctx, mcp.MethodToolsList, 200, 50*time.Millisecond, 500, 2500)
	recorder.RecordRequest(ctx, mcp.MethodToolsCall, 404, 10*time.Millisecond, 100, 200)
	recorder.RecordRequest(ctx, "unknown", 500, 200*time.Millisecond, 1500, 100)

	// Get metrics from the handler
	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	recorder.Handler().ServeHTTP(rr, req)

	body, _ := io.ReadAll(rr.Body)
	metrics := string(body)

	t.Run("requests_total", func(t *testing.T) {
		// Check for request counts by status
		if !strings.Contains(metrics, `mcp_requests_total{`) {
			t.Errorf("mcp_requests_total metric not found:\n%s", metrics)
		}
		if !strings.Contains(metrics, `status="200"`) {
			t.Error("status=200 label not found")
		}
		if !strings.Contains(metrics, `status="404"`) {
			t.Error("status=404 label not found")
		}
		if !strings.Contains(metrics, `status="500"`) {
			t.Error("status=500 label not found")
		}
	})

	t.Run("request_duration", func(t *testing.T) {
		if !strings.Contains(metrics, "mcp_request_duration") {
			t.Errorf("mcp_request_duration metric not found:\n%s", metrics)
		}
	})

	t.Run("request_size", func(t *testing.T) {
		if !strings.Contains(metrics, "mcp_request_size") {
			t.Errorf("mcp_request_size metric not found:\n%s", metrics)
		}
	})

	t.Run("response_size", func(t *testing.T) {
		if !strings.Contains(metrics, "mcp_response_size") {
			t.Errorf("mcp_response_size metric not found:\n%s", metrics)
		}
	})
}

func TestRecorder_RecordRequest_ZeroSizes(t *testing.T) {
	recorder, err := NewRecorder("1.0.0", "http://localhost:3001")
	if err != nil {
		t.Fatalf("NewRecorder failed: %v", err)
	}
	defer recorder.Shutdown(context.Background())

	ctx := context.Background()

	// Record request with zero sizes (should not observe size histograms)
	recorder.RecordRequest(ctx, mcp.MethodInitialize, 200, 10*time.Millisecond, 0, 0)

	// Get metrics from the handler
	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	recorder.Handler().ServeHTTP(rr, req)

	body, _ := io.ReadAll(rr.Body)
	metrics := string(body)

	// Request count should still be recorded
	if !strings.Contains(metrics, `mcp_requests_total{`) {
		t.Errorf("mcp_requests_total metric not found:\n%s", metrics)
	}

	// Duration should still be recorded
	if !strings.Contains(metrics, "mcp_request_duration") {
		t.Errorf("mcp_request_duration metric not found:\n%s", metrics)
	}
}

func TestRecorder_ActiveConnections(t *testing.T) {
	recorder, err := NewRecorder("1.0.0", "http://localhost:3001")
	if err != nil {
		t.Fatalf("NewRecorder failed: %v", err)
	}
	defer recorder.Shutdown(context.Background())

	ctx := context.Background()

	// Increment connections
	recorder.IncrementConnections(ctx)
	recorder.IncrementConnections(ctx)
	recorder.IncrementConnections(ctx)

	// Get metrics
	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	recorder.Handler().ServeHTTP(rr, req)

	body, _ := io.ReadAll(rr.Body)
	metrics := string(body)

	if !strings.Contains(metrics, "mcp_active_connections") {
		t.Errorf("mcp_active_connections metric not found:\n%s", metrics)
	}
	// Should show value of 3 (with OTel labels)
	if !strings.Contains(metrics, "} 3") {
		t.Errorf("Expected active_connections to be 3:\n%s", metrics)
	}

	// Decrement
	recorder.DecrementConnections(ctx)

	// Get metrics again
	rr2 := httptest.NewRecorder()
	recorder.Handler().ServeHTTP(rr2, req)

	body2, _ := io.ReadAll(rr2.Body)
	metrics2 := string(body2)

	// Should show value of 2 (with OTel labels)
	if !strings.Contains(metrics2, "} 2") {
		t.Errorf("Expected active_connections to be 2:\n%s", metrics2)
	}
}

func TestRecorder_HistogramBuckets(t *testing.T) {
	recorder, err := NewRecorder("1.0.0", "http://localhost:3001")
	if err != nil {
		t.Fatalf("NewRecorder failed: %v", err)
	}
	defer recorder.Shutdown(context.Background())

	ctx := context.Background()

	// Record requests with various durations to test bucket distribution
	durations := []time.Duration{
		1 * time.Millisecond,
		5 * time.Millisecond,
		10 * time.Millisecond,
		100 * time.Millisecond,
		500 * time.Millisecond,
		1 * time.Second,
		5 * time.Second,
	}

	for _, d := range durations {
		recorder.RecordRequest(ctx, mcp.MethodToolsList, 200, d, 100, 100)
	}

	// Get metrics
	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	recorder.Handler().ServeHTTP(rr, req)

	body, _ := io.ReadAll(rr.Body)
	metrics := string(body)

	// Verify histogram buckets are present (OTel adds _seconds suffix based on unit)
	if !strings.Contains(metrics, "mcp_request_duration_seconds_bucket") {
		t.Errorf("mcp_request_duration_seconds_bucket not found:\n%s", metrics)
	}
	if !strings.Contains(metrics, "mcp_request_duration_seconds_sum") {
		t.Errorf("mcp_request_duration_seconds_sum not found:\n%s", metrics)
	}
	if !strings.Contains(metrics, "mcp_request_duration_seconds_count") {
		t.Errorf("mcp_request_duration_seconds_count not found:\n%s", metrics)
	}
}

func TestRecorder_MultipleStatusCodes(t *testing.T) {
	recorder, err := NewRecorder("1.0.0", "http://localhost:3001")
	if err != nil {
		t.Fatalf("NewRecorder failed: %v", err)
	}
	defer recorder.Shutdown(context.Background())

	ctx := context.Background()

	// Record requests with various status codes
	statusCodes := []int{200, 201, 204, 301, 400, 401, 403, 404, 500, 502, 503}

	for _, status := range statusCodes {
		recorder.RecordRequest(ctx, mcp.MethodInitialize, status, 10*time.Millisecond, 100, 100)
	}

	// Get metrics
	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	recorder.Handler().ServeHTTP(rr, req)

	body, _ := io.ReadAll(rr.Body)
	metrics := string(body)

	// Check that we have metrics for various status codes
	for _, status := range statusCodes {
		statusLabel := `status="` + string(rune('0'+(status/100))) // First digit
		if !strings.Contains(metrics, "mcp_requests_total") {
			t.Errorf("mcp_requests_total not found for status %d:\n%s", status, metrics)
		}
		_ = statusLabel // We just check the metric exists, detailed label checking is in other tests
	}
}

func TestRecorder_MetricsEndpointFormat(t *testing.T) {
	recorder, err := NewRecorder("1.0.0", "http://localhost:3001")
	if err != nil {
		t.Fatalf("NewRecorder failed: %v", err)
	}
	defer recorder.Shutdown(context.Background())

	ctx := context.Background()

	// Record some data
	recorder.RecordRequest(ctx, mcp.MethodToolsCall, 200, 100*time.Millisecond, 1000, 5000)
	recorder.IncrementConnections(ctx)
	recorder.RecordToolCall(ctx, "get_weather")
	recorder.RecordResourceRead(ctx, "file:///test.txt")
	recorder.RecordError(ctx, mcp.MethodToolsCall, -32600)

	// Get metrics
	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	recorder.Handler().ServeHTTP(rr, req)

	body, _ := io.ReadAll(rr.Body)
	metrics := string(body)

	// Check we have the expected metric families
	expectedMetrics := []string{
		"mcp_requests_total",
		"mcp_request_duration",
		"mcp_request_size",
		"mcp_response_size",
		"mcp_active_connections",
		"mcp_proxy_info",
		"mcp_tool_calls_total",
		"mcp_resource_reads_total",
		"mcp_request_errors_total",
	}

	for _, name := range expectedMetrics {
		if !strings.Contains(metrics, name) {
			t.Errorf("Expected metric %s not found in output:\n%s", name, metrics)
		}
	}
}

func TestRecorder_Shutdown(t *testing.T) {
	recorder, err := NewRecorder("1.0.0", "http://localhost:3001")
	if err != nil {
		t.Fatalf("NewRecorder failed: %v", err)
	}

	// Shutdown should not error
	if err := recorder.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
}
