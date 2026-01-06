package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNewRecorder(t *testing.T) {
	recorder := NewRecorder("1.0.0", "http://localhost:3001")

	if recorder == nil {
		t.Fatal("NewRecorder returned nil")
	}

	if recorder.Registry() == nil {
		t.Error("Registry should not be nil")
	}

	if recorder.Collector() == nil {
		t.Error("Collector should not be nil")
	}
}

func TestRecorder_ProxyInfo(t *testing.T) {
	recorder := NewRecorder("1.0.0", "http://localhost:3001")

	// Check that proxy_info is set to 1
	expected := `
		# HELP mcp_proxy_info Static proxy information (always 1).
		# TYPE mcp_proxy_info gauge
		mcp_proxy_info{target="http://localhost:3001",version="1.0.0"} 1
	`

	if err := testutil.CollectAndCompare(recorder.Collector().ProxyInfo, strings.NewReader(expected)); err != nil {
		t.Errorf("proxy_info metric mismatch: %v", err)
	}
}

func TestRecorder_RecordRequest(t *testing.T) {
	recorder := NewRecorder("1.0.0", "http://localhost:3001")

	// Record some requests
	recorder.RecordRequest(200, 100*time.Millisecond, 1000, 5000)
	recorder.RecordRequest(200, 50*time.Millisecond, 500, 2500)
	recorder.RecordRequest(404, 10*time.Millisecond, 100, 200)
	recorder.RecordRequest(500, 200*time.Millisecond, 1500, 100)

	// Check request count by status
	t.Run("requests_total", func(t *testing.T) {
		// Count for status 200
		count200 := testutil.ToFloat64(recorder.Collector().RequestsTotal.WithLabelValues("200"))
		if count200 != 2 {
			t.Errorf("requests_total{status=200} = %v, want 2", count200)
		}

		// Count for status 404
		count404 := testutil.ToFloat64(recorder.Collector().RequestsTotal.WithLabelValues("404"))
		if count404 != 1 {
			t.Errorf("requests_total{status=404} = %v, want 1", count404)
		}

		// Count for status 500
		count500 := testutil.ToFloat64(recorder.Collector().RequestsTotal.WithLabelValues("500"))
		if count500 != 1 {
			t.Errorf("requests_total{status=500} = %v, want 1", count500)
		}
	})

	t.Run("request_duration_seconds", func(t *testing.T) {
		// Check histogram has observations using CollectAndCount
		count := testutil.CollectAndCount(recorder.Collector().RequestDuration)
		// Each histogram generates multiple time series (one per bucket + sum + count)
		// We just verify it's not zero
		if count == 0 {
			t.Error("request_duration_seconds should have observations")
		}
	})

	t.Run("request_size_bytes", func(t *testing.T) {
		count := testutil.CollectAndCount(recorder.Collector().RequestSize)
		if count == 0 {
			t.Error("request_size_bytes should have observations")
		}
	})

	t.Run("response_size_bytes", func(t *testing.T) {
		count := testutil.CollectAndCount(recorder.Collector().ResponseSize)
		if count == 0 {
			t.Error("response_size_bytes should have observations")
		}
	})
}

func TestRecorder_RecordRequest_ZeroSizes(t *testing.T) {
	recorder := NewRecorder("1.0.0", "http://localhost:3001")

	// Record request with zero sizes (should not observe)
	recorder.RecordRequest(200, 10*time.Millisecond, 0, 0)

	// Request count should still increment
	count := testutil.ToFloat64(recorder.Collector().RequestsTotal.WithLabelValues("200"))
	if count != 1 {
		t.Errorf("requests_total{status=200} = %v, want 1", count)
	}

	// Duration should still be recorded
	durationCount := testutil.CollectAndCount(recorder.Collector().RequestDuration)
	if durationCount == 0 {
		t.Error("request_duration_seconds should have observations even with zero sizes")
	}
}

func TestRecorder_ActiveConnections(t *testing.T) {
	recorder := NewRecorder("1.0.0", "http://localhost:3001")

	// Initial value should be 0
	initial := testutil.ToFloat64(recorder.Collector().ActiveConnections)
	if initial != 0 {
		t.Errorf("initial active_connections = %v, want 0", initial)
	}

	// Increment
	recorder.IncrementConnections()
	recorder.IncrementConnections()
	recorder.IncrementConnections()

	after3 := testutil.ToFloat64(recorder.Collector().ActiveConnections)
	if after3 != 3 {
		t.Errorf("active_connections after 3 increments = %v, want 3", after3)
	}

	// Decrement
	recorder.DecrementConnections()

	after2 := testutil.ToFloat64(recorder.Collector().ActiveConnections)
	if after2 != 2 {
		t.Errorf("active_connections after 1 decrement = %v, want 2", after2)
	}

	// Decrement more
	recorder.DecrementConnections()
	recorder.DecrementConnections()

	afterAll := testutil.ToFloat64(recorder.Collector().ActiveConnections)
	if afterAll != 0 {
		t.Errorf("active_connections after all decrements = %v, want 0", afterAll)
	}
}

func TestRecorder_HistogramBuckets(t *testing.T) {
	recorder := NewRecorder("1.0.0", "http://localhost:3001")

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
		recorder.RecordRequest(200, d, 100, 100)
	}

	// Verify metrics are being collected
	count := testutil.ToFloat64(recorder.Collector().RequestsTotal.WithLabelValues("200"))
	if count != float64(len(durations)) {
		t.Errorf("requests_total = %v, want %v", count, len(durations))
	}

	// Verify histogram has collected observations
	histogramCount := testutil.CollectAndCount(recorder.Collector().RequestDuration)
	if histogramCount == 0 {
		t.Error("request_duration_seconds histogram should have observations")
	}
}

func TestCollector_Register(t *testing.T) {
	// Create a recorder which registers metrics
	recorder := NewRecorder("test", "http://test")

	// Should be able to gather metrics
	families, err := recorder.Registry().Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Should have at least the proxy_info metric
	found := false
	for _, family := range families {
		if family.GetName() == "mcp_proxy_info" {
			found = true
			break
		}
	}

	if !found {
		t.Error("mcp_proxy_info metric not found in gathered metrics")
	}
}

func TestRecorder_MetricsEndpointFormat(t *testing.T) {
	recorder := NewRecorder("1.0.0", "http://localhost:3001")

	// Record some data
	recorder.RecordRequest(200, 100*time.Millisecond, 1000, 5000)
	recorder.IncrementConnections()

	// Gather all metrics
	families, err := recorder.Registry().Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Check we have the expected metric families
	expectedMetrics := map[string]bool{
		"mcp_requests_total":           false,
		"mcp_request_duration_seconds": false,
		"mcp_request_size_bytes":       false,
		"mcp_response_size_bytes":      false,
		"mcp_active_connections":       false,
		"mcp_proxy_info":               false,
	}

	for _, family := range families {
		if _, ok := expectedMetrics[family.GetName()]; ok {
			expectedMetrics[family.GetName()] = true
		}
	}

	for name, found := range expectedMetrics {
		if !found {
			t.Errorf("Expected metric %s not found", name)
		}
	}
}

func TestRecorder_MultipleStatusCodes(t *testing.T) {
	recorder := NewRecorder("1.0.0", "http://localhost:3001")

	// Record requests with various status codes
	statusCodes := []int{200, 201, 204, 301, 400, 401, 403, 404, 500, 502, 503}

	for _, status := range statusCodes {
		recorder.RecordRequest(status, 10*time.Millisecond, 100, 100)
	}

	// Verify total count across all status codes
	families, err := recorder.Registry().Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	var totalCount float64
	for _, family := range families {
		if family.GetName() == "mcp_requests_total" {
			for _, metric := range family.GetMetric() {
				totalCount += metric.GetCounter().GetValue()
			}
		}
	}

	if totalCount != float64(len(statusCodes)) {
		t.Errorf("total requests = %v, want %v", totalCount, len(statusCodes))
	}
}
