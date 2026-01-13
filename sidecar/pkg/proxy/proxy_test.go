package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vitorbari/mcp-operator/sidecar/pkg/metrics"
)

// newTestLogger creates a logger for testing that discards output.
func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNew(t *testing.T) {
	tests := []struct {
		name       string
		listenAddr string
		targetAddr string
		wantScheme string
		wantHost   string
		wantErr    bool
	}{
		{
			name:       "valid target with scheme",
			listenAddr: ":8080",
			targetAddr: "http://localhost:3001",
			wantScheme: "http",
			wantHost:   "localhost:3001",
			wantErr:    false,
		},
		{
			name:       "valid target without scheme",
			listenAddr: ":8080",
			targetAddr: "localhost:3001",
			wantScheme: "http",
			wantHost:   "localhost:3001",
			wantErr:    false,
		},
		{
			name:       "https target",
			listenAddr: ":8080",
			targetAddr: "https://example.com:443",
			wantScheme: "https",
			wantHost:   "example.com:443",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := newTestLogger()
			p, err := New(tt.listenAddr, tt.targetAddr, logger)

			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			if p.target.Scheme != tt.wantScheme {
				t.Errorf("New() scheme = %v, want %v", p.target.Scheme, tt.wantScheme)
			}

			if p.target.Host != tt.wantHost {
				t.Errorf("New() host = %v, want %v", p.target.Host, tt.wantHost)
			}

			if p.listenAddr != tt.listenAddr {
				t.Errorf("New() listenAddr = %v, want %v", p.listenAddr, tt.listenAddr)
			}
		})
	}
}

func TestProxy_SuccessfulForwarding(t *testing.T) {
	// Create a mock target server
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Response-Header", "test-value")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello from target"))
	}))
	defer targetServer.Close()

	// Create proxy pointing to target
	logger := newTestLogger()
	p, err := New(":0", targetServer.URL, logger)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	// Create a test request
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	// Serve the request through the proxy's handler
	handler := p.metricsMiddleware(p.reverseProxy)
	handler.ServeHTTP(rr, req)

	// Check response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	body := rr.Body.String()
	if body != "Hello from target" {
		t.Errorf("Expected body 'Hello from target', got '%s'", body)
	}

	// Check response headers are preserved
	if rr.Header().Get("X-Response-Header") != "test-value" {
		t.Errorf("Expected X-Response-Header 'test-value', got '%s'", rr.Header().Get("X-Response-Header"))
	}
}

func TestProxy_TargetUnavailable(t *testing.T) {
	// Create proxy pointing to non-existent target
	logger := newTestLogger()
	p, err := New(":0", "http://localhost:59999", logger) // Port unlikely to be in use
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	// Create a test request
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	// Serve the request through the proxy's handler
	handler := p.metricsMiddleware(p.reverseProxy)
	handler.ServeHTTP(rr, req)

	// Should return 502 Bad Gateway
	if rr.Code != http.StatusBadGateway {
		t.Errorf("Expected status %d, got %d", http.StatusBadGateway, rr.Code)
	}
}

func TestProxy_HeadersPreserved(t *testing.T) {
	var receivedHeaders http.Header

	// Create a mock target server that captures headers
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer targetServer.Close()

	// Create proxy pointing to target
	logger := newTestLogger()
	p, err := New(":0", targetServer.URL, logger)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	// Create a test request with custom headers
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Custom-Header", "custom-value")
	req.Header.Set("Authorization", "Bearer token123")
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.100:12345"

	rr := httptest.NewRecorder()

	// Serve the request through the proxy's handler
	handler := p.metricsMiddleware(p.reverseProxy)
	handler.ServeHTTP(rr, req)

	// Check custom headers are preserved
	if receivedHeaders.Get("X-Custom-Header") != "custom-value" {
		t.Errorf("Expected X-Custom-Header 'custom-value', got '%s'", receivedHeaders.Get("X-Custom-Header"))
	}

	if receivedHeaders.Get("Authorization") != "Bearer token123" {
		t.Errorf("Expected Authorization header to be preserved")
	}

	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", receivedHeaders.Get("Content-Type"))
	}

	// Check X-Forwarded-For is set
	xff := receivedHeaders.Get("X-Forwarded-For")
	if xff == "" {
		t.Error("Expected X-Forwarded-For header to be set")
	}
	if !strings.Contains(xff, "192.168.1.100") {
		t.Errorf("Expected X-Forwarded-For to contain client IP, got '%s'", xff)
	}

	// Check X-Forwarded-Proto is set
	if receivedHeaders.Get("X-Forwarded-Proto") != "http" {
		t.Errorf("Expected X-Forwarded-Proto 'http', got '%s'", receivedHeaders.Get("X-Forwarded-Proto"))
	}
}

func TestProxy_RequestBodyForwarded(t *testing.T) {
	var receivedBody []byte

	// Create a mock target server that captures the body
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Failed to read request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer targetServer.Close()

	// Create proxy pointing to target
	logger := newTestLogger()
	p, err := New(":0", targetServer.URL, logger)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	// Create a test request with JSON body
	requestBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"params": map[string]string{
			"name": "test-tool",
		},
		"id": 1,
	}
	bodyBytes, _ := json.Marshal(requestBody)

	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	// Serve the request through the proxy's handler
	handler := p.metricsMiddleware(p.reverseProxy)
	handler.ServeHTTP(rr, req)

	// Check response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	// Verify the body was forwarded correctly
	if !bytes.Equal(receivedBody, bodyBytes) {
		t.Errorf("Request body not forwarded correctly.\nExpected: %s\nGot: %s", string(bodyBytes), string(receivedBody))
	}
}

func TestProxy_XForwardedForChaining(t *testing.T) {
	var receivedXFF string

	// Create a mock target server
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedXFF = r.Header.Get("X-Forwarded-For")
		w.WriteHeader(http.StatusOK)
	}))
	defer targetServer.Close()

	// Create proxy pointing to target
	logger := newTestLogger()
	p, err := New(":0", targetServer.URL, logger)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	// Create a test request with existing X-Forwarded-For header
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")
	req.RemoteAddr = "192.168.1.100:12345"
	rr := httptest.NewRecorder()

	// Serve the request through the proxy's handler
	handler := p.metricsMiddleware(p.reverseProxy)
	handler.ServeHTTP(rr, req)

	// Check X-Forwarded-For is chained correctly
	if !strings.HasPrefix(receivedXFF, "10.0.0.1, 10.0.0.2, ") {
		t.Errorf("Expected X-Forwarded-For to start with existing IPs, got '%s'", receivedXFF)
	}
}

func TestProxy_StartAndShutdown(t *testing.T) {
	// Create a mock target server
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer targetServer.Close()

	// Create proxy
	logger := newTestLogger()
	p, err := New("127.0.0.1:0", targetServer.URL, logger)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	// Start proxy with a context we can cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Start proxy in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- p.Start(ctx)
	}()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Cancel context to trigger shutdown
	cancel()

	// Wait for shutdown with timeout
	select {
	case err := <-errChan:
		if err != nil && err != context.Canceled {
			t.Errorf("Unexpected error from Start(): %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Server did not shut down within timeout")
	}
}

func TestProxy_ResponseWithDifferentStatusCodes(t *testing.T) {
	tests := []struct {
		name           string
		targetStatus   int
		expectedStatus int
	}{
		{"OK", http.StatusOK, http.StatusOK},
		{"Created", http.StatusCreated, http.StatusCreated},
		{"Bad Request", http.StatusBadRequest, http.StatusBadRequest},
		{"Not Found", http.StatusNotFound, http.StatusNotFound},
		{"Internal Server Error", http.StatusInternalServerError, http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock target server
			targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.targetStatus)
			}))
			defer targetServer.Close()

			// Create proxy
			logger := newTestLogger()
			p, err := New(":0", targetServer.URL, logger)
			if err != nil {
				t.Fatalf("Failed to create proxy: %v", err)
			}

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rr := httptest.NewRecorder()

			handler := p.metricsMiddleware(p.reverseProxy)
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		xRealIP    string
		xff        string
		remoteAddr string
		expected   string
	}{
		{
			name:       "X-Real-IP present",
			xRealIP:    "10.0.0.1",
			xff:        "",
			remoteAddr: "192.168.1.1:12345",
			expected:   "10.0.0.1",
		},
		{
			name:       "X-Forwarded-For single IP",
			xRealIP:    "",
			xff:        "10.0.0.2",
			remoteAddr: "192.168.1.1:12345",
			expected:   "10.0.0.2",
		},
		{
			name:       "X-Forwarded-For multiple IPs",
			xRealIP:    "",
			xff:        "10.0.0.1, 10.0.0.2, 10.0.0.3",
			remoteAddr: "192.168.1.1:12345",
			expected:   "10.0.0.1",
		},
		{
			name:       "Fall back to RemoteAddr",
			xRealIP:    "",
			xff:        "",
			remoteAddr: "192.168.1.1:12345",
			expected:   "192.168.1.1",
		},
		{
			name:       "RemoteAddr without port",
			xRealIP:    "",
			xff:        "",
			remoteAddr: "192.168.1.1",
			expected:   "192.168.1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.xRealIP != "" {
				req.Header.Set("X-Real-IP", tt.xRealIP)
			}
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			req.RemoteAddr = tt.remoteAddr

			got := getClientIP(req)
			if got != tt.expected {
				t.Errorf("getClientIP() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLoggingResponseWriter(t *testing.T) {
	t.Run("captures status code", func(t *testing.T) {
		rr := httptest.NewRecorder()
		lrw := &loggingResponseWriter{ResponseWriter: rr, statusCode: http.StatusOK}

		lrw.WriteHeader(http.StatusNotFound)

		if lrw.statusCode != http.StatusNotFound {
			t.Errorf("Expected status code %d, got %d", http.StatusNotFound, lrw.statusCode)
		}
	})

	t.Run("captures bytes written", func(t *testing.T) {
		rr := httptest.NewRecorder()
		lrw := &loggingResponseWriter{ResponseWriter: rr, statusCode: http.StatusOK}

		data := []byte("Hello, World!")
		n, err := lrw.Write(data)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if n != len(data) {
			t.Errorf("Expected %d bytes written, got %d", len(data), n)
		}
		if lrw.bytesWritten != int64(len(data)) {
			t.Errorf("Expected bytesWritten %d, got %d", len(data), lrw.bytesWritten)
		}
	})

	t.Run("default status code is 200", func(t *testing.T) {
		rr := httptest.NewRecorder()
		lrw := &loggingResponseWriter{ResponseWriter: rr, statusCode: http.StatusOK}

		// Write without calling WriteHeader
		lrw.Write([]byte("test"))

		if lrw.statusCode != http.StatusOK {
			t.Errorf("Expected default status code %d, got %d", http.StatusOK, lrw.statusCode)
		}
	})
}

// TestProxy_SSEResponseRecordsRequestMetrics is a regression test to ensure that
// SSE responses still record regular request metrics. This test prevents a bug where
// SSE responses would skip request metrics recording.
func TestProxy_SSEResponseRecordsRequestMetrics(t *testing.T) {
	// Create a mock target server that returns SSE content type
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		// Send a simple SSE event
		w.Write([]byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"result\":{}}\n\n"))
	}))
	defer targetServer.Close()

	// Create a real metrics recorder to verify metrics are recorded
	recorder, err := metrics.NewRecorder("test", targetServer.URL)
	if err != nil {
		t.Fatalf("Failed to create metrics recorder: %v", err)
	}
	defer recorder.Shutdown(context.Background())

	// Create proxy with recorder
	logger := newTestLogger()
	p, err := NewWithRecorder(":0", targetServer.URL, logger, recorder)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	// Create a test request with JSON body (simulating MCP request)
	reqBody := []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	// Serve the request through the proxy's handler
	handler := p.metricsMiddleware(p.reverseProxy)
	handler.ServeHTTP(rr, req)

	// Verify the response was successful and is SSE
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	contentType := rr.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/event-stream") {
		t.Errorf("Expected SSE content type, got %s", contentType)
	}

	// Verify the SSE data was passed through
	body := rr.Body.String()
	if !strings.Contains(body, "event: message") {
		t.Errorf("Expected SSE event in response, got %s", body)
	}

	// Now fetch metrics and verify request metrics were recorded
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRr := httptest.NewRecorder()
	recorder.Handler().ServeHTTP(metricsRr, metricsReq)

	metricsBody := metricsRr.Body.String()

	// Verify mcp_requests_total was recorded (this is the key regression test)
	if !strings.Contains(metricsBody, "mcp_requests_total") {
		t.Error("Expected mcp_requests_total metric to be recorded for SSE response")
	}

	// Verify the method was parsed from the request
	if !strings.Contains(metricsBody, `method="tools/list"`) {
		t.Error("Expected method=tools/list to be recorded in request metrics")
	}

	// POST requests with SSE content type should NOT record SSE connection metrics
	// because they are Streamable HTTP request-responses, not long-lived SSE streams.
	// SSE connection metrics should only be recorded for GET requests.
	if strings.Contains(metricsBody, `mcp_sse_connections_total{`) {
		t.Error("POST requests with SSE content type should NOT record mcp_sse_connections_total")
	}
}

// TestProxy_NonSSEResponseRecordsMetrics verifies that non-SSE responses
// continue to record metrics normally (sanity check).
func TestProxy_NonSSEResponseRecordsMetrics(t *testing.T) {
	// Create a mock target server that returns JSON
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","result":{"tools":[]},"id":1}`))
	}))
	defer targetServer.Close()

	// Create a real metrics recorder
	recorder, err := metrics.NewRecorder("test", targetServer.URL)
	if err != nil {
		t.Fatalf("Failed to create metrics recorder: %v", err)
	}
	defer recorder.Shutdown(context.Background())

	// Create proxy with recorder
	logger := newTestLogger()
	p, err := NewWithRecorder(":0", targetServer.URL, logger, recorder)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	// Create a test request with JSON body
	reqBody := []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	// Serve the request
	handler := p.metricsMiddleware(p.reverseProxy)
	handler.ServeHTTP(rr, req)

	// Verify response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	// Fetch metrics
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRr := httptest.NewRecorder()
	recorder.Handler().ServeHTTP(metricsRr, metricsReq)

	metricsBody := metricsRr.Body.String()

	// Verify request metrics were recorded
	if !strings.Contains(metricsBody, "mcp_requests_total") {
		t.Error("Expected mcp_requests_total metric to be recorded")
	}

	if !strings.Contains(metricsBody, `method="tools/list"`) {
		t.Error("Expected method=tools/list to be recorded")
	}

	// Verify SSE metrics were NOT recorded for non-SSE response
	if strings.Contains(metricsBody, `mcp_sse_connections_total{`) {
		t.Error("Non-SSE responses should NOT record mcp_sse_connections_total")
	}
}

// TestProxy_GETSSERequestRecordsSSEConnectionMetrics verifies that GET requests
// with SSE content type correctly record SSE connection metrics.
// This is the expected behavior for true long-lived SSE streams (e.g., server notifications).
func TestProxy_GETSSERequestRecordsSSEConnectionMetrics(t *testing.T) {
	// Create a mock target server that returns SSE content type for GET requests
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		// Send a simple SSE event and close (simulating a short-lived SSE for testing)
		w.Write([]byte("event: endpoint\ndata: /messages\n\n"))
	}))
	defer targetServer.Close()

	// Create a real metrics recorder
	recorder, err := metrics.NewRecorder("test", targetServer.URL)
	if err != nil {
		t.Fatalf("Failed to create metrics recorder: %v", err)
	}
	defer recorder.Shutdown(context.Background())

	// Create proxy with recorder
	logger := newTestLogger()
	p, err := NewWithRecorder(":0", targetServer.URL, logger, recorder)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	// Create a GET request (this is how SSE streams are established)
	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	rr := httptest.NewRecorder()

	// Serve the request
	handler := p.metricsMiddleware(p.reverseProxy)
	handler.ServeHTTP(rr, req)

	// Verify the response was successful and is SSE
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	contentType := rr.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/event-stream") {
		t.Errorf("Expected SSE content type, got %s", contentType)
	}

	// Fetch metrics
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRr := httptest.NewRecorder()
	recorder.Handler().ServeHTTP(metricsRr, metricsReq)

	metricsBody := metricsRr.Body.String()

	// Verify SSE connection metrics were recorded for GET request
	if !strings.Contains(metricsBody, `mcp_sse_connections_total{`) {
		t.Error("Expected mcp_sse_connections_total metric to be recorded for GET SSE request")
	}

	// Verify connection duration was recorded (connection closed)
	if !strings.Contains(metricsBody, `mcp_sse_connection_duration_seconds_count{`) {
		t.Error("Expected mcp_sse_connection_duration_seconds metric to be recorded")
	}
}

// TestProxy_POSTSSEResponseDoesNotRecordSSEConnectionMetrics is a regression test
// to ensure that POST requests with text/event-stream content type (Streamable HTTP)
// do NOT record SSE connection metrics. Only GET requests should be counted as SSE connections.
//
// Bug context: In MCP Streamable HTTP transport, POST request-responses use
// text/event-stream content type for the response body. These are NOT long-lived
// SSE connections but regular request-response cycles. The proxy was incorrectly
// counting every POST response as an SSE connection, leading to inflated metrics.
func TestProxy_POSTSSEResponseDoesNotRecordSSEConnectionMetrics(t *testing.T) {
	// Create a mock target server that returns SSE content type for POST (Streamable HTTP style)
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate Streamable HTTP response - uses text/event-stream content type
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"result\":{\"tools\":[]},\"id\":1}\n\n"))
	}))
	defer targetServer.Close()

	// Create a real metrics recorder
	recorder, err := metrics.NewRecorder("test", targetServer.URL)
	if err != nil {
		t.Fatalf("Failed to create metrics recorder: %v", err)
	}
	defer recorder.Shutdown(context.Background())

	// Create proxy with recorder
	logger := newTestLogger()
	p, err := NewWithRecorder(":0", targetServer.URL, logger, recorder)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	// Make multiple POST requests (simulating Streamable HTTP traffic)
	for i := 0; i < 5; i++ {
		reqBody := []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`)
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		handler := p.metricsMiddleware(p.reverseProxy)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
		}
	}

	// Fetch metrics
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRr := httptest.NewRecorder()
	recorder.Handler().ServeHTTP(metricsRr, metricsReq)

	metricsBody := metricsRr.Body.String()

	// Verify request metrics were recorded (5 requests)
	if !strings.Contains(metricsBody, `mcp_requests_total{method="tools/list"`) {
		t.Error("Expected mcp_requests_total to be recorded for POST requests")
	}

	// CRITICAL: Verify SSE connection metrics were NOT recorded for POST requests
	// This is the regression test for the bug where POST requests were incorrectly
	// counted as SSE connections.
	if strings.Contains(metricsBody, `mcp_sse_connections_total{`) {
		t.Error("POST requests with SSE content type should NOT record mcp_sse_connections_total - this is a regression!")
	}

	if strings.Contains(metricsBody, `mcp_sse_connections_active{`) {
		t.Error("POST requests with SSE content type should NOT record mcp_sse_connections_active - this is a regression!")
	}
}

// TestProxy_MixedGETAndPOSTSSERequests verifies that in a mixed traffic scenario
// (both GET SSE streams and POST Streamable HTTP requests), only GET requests
// are counted as SSE connections.
func TestProxy_MixedGETAndPOSTSSERequests(t *testing.T) {
	// Create a mock target server that returns SSE content type for both GET and POST
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			w.Write([]byte("event: endpoint\ndata: /messages\n\n"))
		} else {
			w.Write([]byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"result\":{},\"id\":1}\n\n"))
		}
	}))
	defer targetServer.Close()

	// Create a real metrics recorder
	recorder, err := metrics.NewRecorder("test", targetServer.URL)
	if err != nil {
		t.Fatalf("Failed to create metrics recorder: %v", err)
	}
	defer recorder.Shutdown(context.Background())

	// Create proxy with recorder
	logger := newTestLogger()
	p, err := NewWithRecorder(":0", targetServer.URL, logger, recorder)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	handler := p.metricsMiddleware(p.reverseProxy)

	// Make 3 GET requests (SSE streams)
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/sse", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	// Make 10 POST requests (Streamable HTTP)
	for i := 0; i < 10; i++ {
		reqBody := []byte(`{"jsonrpc":"2.0","method":"initialize","id":1}`)
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	// Fetch metrics
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRr := httptest.NewRecorder()
	recorder.Handler().ServeHTTP(metricsRr, metricsReq)

	metricsBody := metricsRr.Body.String()

	// Verify SSE connection total is 3 (only GET requests)
	// The metric format is: mcp_sse_connections_total{...} 3
	if !strings.Contains(metricsBody, `mcp_sse_connections_total{`) {
		t.Error("Expected mcp_sse_connections_total metric to exist")
	}

	// Parse the metric value to verify it's exactly 3
	for _, line := range strings.Split(metricsBody, "\n") {
		if strings.HasPrefix(line, "mcp_sse_connections_total{") {
			// Extract the value (last space-separated field)
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				value := parts[len(parts)-1]
				if value != "3" {
					t.Errorf("Expected mcp_sse_connections_total to be 3 (GET requests only), got %s", value)
				}
			}
		}
	}
}
