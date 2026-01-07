package health

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewHealthChecker(t *testing.T) {
	hc := NewHealthChecker("localhost:3001", 10*time.Second)

	if hc == nil {
		t.Fatal("NewHealthChecker returned nil")
	}

	if hc.TargetAddr() != "localhost:3001" {
		t.Errorf("expected target addr localhost:3001, got %s", hc.TargetAddr())
	}

	// Liveness should be true (process is running)
	if !hc.IsHealthy() {
		t.Error("expected IsHealthy to be true")
	}

	// Readiness should be false until first check
	if hc.IsReady() {
		t.Error("expected IsReady to be false before first check")
	}
}

func TestHealthChecker_LivenessAlwaysReturns200(t *testing.T) {
	hc := NewHealthChecker("localhost:99999", 10*time.Second)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rr := httptest.NewRecorder()

	handler := hc.LivenessHandler()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	body, _ := io.ReadAll(rr.Body)
	var response HealthResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if response.Status != "healthy" {
		t.Errorf("expected status 'healthy', got %s", response.Status)
	}

	if response.UptimeSeconds <= 0 {
		t.Error("expected positive uptime_seconds")
	}
}

func TestHealthChecker_ReadinessReturns200WhenTargetUp(t *testing.T) {
	// Start a test server to act as the target
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start test listener: %v", err)
	}
	defer listener.Close()

	targetAddr := listener.Addr().String()
	hc := NewHealthChecker(targetAddr, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hc.Start(ctx)

	// Wait for first check
	time.Sleep(200 * time.Millisecond)

	req := httptest.NewRequest("GET", "/readyz", nil)
	rr := httptest.NewRecorder()

	handler := hc.ReadinessHandler()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		body, _ := io.ReadAll(rr.Body)
		t.Errorf("expected status 200, got %d: %s", rr.Code, string(body))
	}

	body, _ := io.ReadAll(rr.Body)
	var response HealthResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if response.Status != "healthy" {
		t.Errorf("expected status 'healthy', got %s", response.Status)
	}

	if response.Checks["target"].Status != "up" {
		t.Errorf("expected target status 'up', got %s", response.Checks["target"].Status)
	}

	hc.Stop()
}

func TestHealthChecker_ReadinessReturns503WhenTargetDown(t *testing.T) {
	// Use a port that nothing is listening on
	hc := NewHealthChecker("127.0.0.1:59999", 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hc.Start(ctx)

	// Wait for first check
	time.Sleep(200 * time.Millisecond)

	req := httptest.NewRequest("GET", "/readyz", nil)
	rr := httptest.NewRecorder()

	handler := hc.ReadinessHandler()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rr.Code)
	}

	body, _ := io.ReadAll(rr.Body)
	var response HealthResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if response.Status != "unhealthy" {
		t.Errorf("expected status 'unhealthy', got %s", response.Status)
	}

	if response.Checks["target"].Status != "down" {
		t.Errorf("expected target status 'down', got %s", response.Checks["target"].Status)
	}

	if response.Checks["target"].Error == "" {
		t.Error("expected error message when target is down")
	}

	hc.Stop()
}

func TestHealthChecker_BackgroundCheckingUpdatesState(t *testing.T) {
	// Start a test server to act as the target
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start test listener: %v", err)
	}

	targetAddr := listener.Addr().String()
	hc := NewHealthChecker(targetAddr, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hc.Start(ctx)

	// Wait for first check
	time.Sleep(200 * time.Millisecond)

	// Should be ready
	if !hc.IsReady() {
		t.Error("expected IsReady to be true when target is up")
	}

	lastCheck1, _ := hc.LastCheckResult()
	if lastCheck1.IsZero() {
		t.Error("expected last check time to be set")
	}

	// Close the listener (target goes down)
	listener.Close()

	// Wait for next check
	time.Sleep(200 * time.Millisecond)

	// Should not be ready now
	if hc.IsReady() {
		t.Error("expected IsReady to be false when target is down")
	}

	_, lastErr := hc.LastCheckResult()
	if lastErr == nil {
		t.Error("expected last error to be set when target is down")
	}

	hc.Stop()
}

func TestHealthChecker_Stop(t *testing.T) {
	hc := NewHealthChecker("localhost:3001", 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hc.Start(ctx)

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	// Stop should not hang
	done := make(chan struct{})
	go func() {
		hc.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Stop() did not return in time")
	}
}

func TestHealthChecker_ResponseFormat(t *testing.T) {
	// Start a test server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start test listener: %v", err)
	}
	defer listener.Close()

	hc := NewHealthChecker(listener.Addr().String(), 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hc.Start(ctx)
	defer hc.Stop()

	// Wait for check
	time.Sleep(200 * time.Millisecond)

	t.Run("liveness response format", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/healthz", nil)
		rr := httptest.NewRecorder()
		hc.LivenessHandler().ServeHTTP(rr, req)

		var resp HealthResponse
		json.NewDecoder(rr.Body).Decode(&resp)

		if resp.Status == "" {
			t.Error("status field missing")
		}
		if resp.UptimeSeconds <= 0 {
			t.Error("uptime_seconds should be positive")
		}
	})

	t.Run("readiness response format", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/readyz", nil)
		rr := httptest.NewRecorder()
		hc.ReadinessHandler().ServeHTTP(rr, req)

		var resp HealthResponse
		json.NewDecoder(rr.Body).Decode(&resp)

		if resp.Status == "" {
			t.Error("status field missing")
		}
		if resp.UptimeSeconds <= 0 {
			t.Error("uptime_seconds should be positive")
		}
		if resp.Checks == nil {
			t.Error("checks map missing")
		}
		if _, ok := resp.Checks["target"]; !ok {
			t.Error("target check missing")
		}
		if resp.Checks["target"].LastCheck == "" {
			t.Error("last_check missing")
		}
	})
}

func TestHealthChecker_String(t *testing.T) {
	hc := NewHealthChecker("localhost:3001", 10*time.Second)
	str := hc.String()

	if str == "" {
		t.Error("String() should return non-empty string")
	}
}

func TestHealthChecker_TargetRecovery(t *testing.T) {
	// This test verifies that when a target comes back up, readiness is restored

	// Start without target
	hc := NewHealthChecker("127.0.0.1:59998", 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hc.Start(ctx)
	defer hc.Stop()

	// Wait for check
	time.Sleep(200 * time.Millisecond)

	// Should not be ready
	if hc.IsReady() {
		t.Error("should not be ready when target is down")
	}

	// Start a listener on that port
	listener, err := net.Listen("tcp", "127.0.0.1:59998")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	// Wait for next check
	time.Sleep(200 * time.Millisecond)

	// Should be ready now
	if !hc.IsReady() {
		t.Error("should be ready when target is up")
	}
}
