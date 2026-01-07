// Package health provides health check endpoints for Kubernetes probes.
package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// HealthChecker manages health checks against a target server.
type HealthChecker struct {
	targetAddr    string
	healthy       atomic.Bool
	ready         atomic.Bool
	lastCheck     time.Time
	lastError     error
	lastLatency   time.Duration
	checkInterval time.Duration
	startTime     time.Time

	mu     sync.RWMutex
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// HealthResponse is the JSON response for health endpoints.
type HealthResponse struct {
	Status        string                 `json:"status"`
	Checks        map[string]CheckResult `json:"checks,omitempty"`
	UptimeSeconds float64                `json:"uptime_seconds"`
}

// CheckResult represents the result of a single health check.
type CheckResult struct {
	Status    string  `json:"status"`
	LatencyMs float64 `json:"latency_ms,omitempty"`
	LastCheck string  `json:"last_check,omitempty"`
	Error     string  `json:"error,omitempty"`
}

// NewHealthChecker creates a new HealthChecker for the given target.
func NewHealthChecker(targetAddr string, checkInterval time.Duration) *HealthChecker {
	hc := &HealthChecker{
		targetAddr:    targetAddr,
		checkInterval: checkInterval,
		startTime:     time.Now(),
	}
	// Process is always considered healthy (liveness)
	hc.healthy.Store(true)
	// Start as not ready until first check
	hc.ready.Store(false)
	return hc
}

// Start begins the background health checking routine.
func (hc *HealthChecker) Start(ctx context.Context) {
	ctx, hc.cancel = context.WithCancel(ctx)

	hc.wg.Add(1)
	go func() {
		defer hc.wg.Done()
		hc.runChecks(ctx)
	}()
}

// Stop stops the background health checking routine.
func (hc *HealthChecker) Stop() {
	if hc.cancel != nil {
		hc.cancel()
	}
	hc.wg.Wait()
}

// runChecks runs the health check loop.
func (hc *HealthChecker) runChecks(ctx context.Context) {
	// Run initial check immediately
	hc.checkTarget()

	ticker := time.NewTicker(hc.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hc.checkTarget()
		}
	}
}

// checkTarget attempts to connect to the target and updates the ready state.
func (hc *HealthChecker) checkTarget() {
	start := time.Now()

	// Attempt TCP connection to target
	conn, err := net.DialTimeout("tcp", hc.targetAddr, 5*time.Second)

	latency := time.Since(start)

	hc.mu.Lock()
	hc.lastCheck = time.Now()
	hc.lastLatency = latency

	if err != nil {
		hc.lastError = err
		hc.ready.Store(false)
	} else {
		conn.Close()
		hc.lastError = nil
		hc.ready.Store(true)
	}
	hc.mu.Unlock()
}

// IsHealthy returns the liveness status.
func (hc *HealthChecker) IsHealthy() bool {
	return hc.healthy.Load()
}

// IsReady returns the readiness status.
func (hc *HealthChecker) IsReady() bool {
	return hc.ready.Load()
}

// LivenessHandler returns an HTTP handler for the /healthz endpoint.
func (hc *HealthChecker) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := HealthResponse{
			Status:        "healthy",
			UptimeSeconds: time.Since(hc.startTime).Seconds(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

// ReadinessHandler returns an HTTP handler for the /readyz endpoint.
func (hc *HealthChecker) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hc.mu.RLock()
		lastCheck := hc.lastCheck
		lastError := hc.lastError
		lastLatency := hc.lastLatency
		hc.mu.RUnlock()

		isReady := hc.ready.Load()

		checkResult := CheckResult{
			LatencyMs: float64(lastLatency.Milliseconds()),
		}
		if !lastCheck.IsZero() {
			checkResult.LastCheck = lastCheck.Format(time.RFC3339)
		}

		if isReady {
			checkResult.Status = "up"
		} else {
			checkResult.Status = "down"
			if lastError != nil {
				checkResult.Error = lastError.Error()
			}
		}

		response := HealthResponse{
			UptimeSeconds: time.Since(hc.startTime).Seconds(),
			Checks: map[string]CheckResult{
				"target": checkResult,
			},
		}

		w.Header().Set("Content-Type", "application/json")

		if isReady {
			response.Status = "healthy"
			w.WriteHeader(http.StatusOK)
		} else {
			response.Status = "unhealthy"
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(response)
	}
}

// TargetAddr returns the target address being checked.
func (hc *HealthChecker) TargetAddr() string {
	return hc.targetAddr
}

// LastCheckResult returns the last check timestamp and error.
func (hc *HealthChecker) LastCheckResult() (time.Time, error) {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.lastCheck, hc.lastError
}

// String returns a string representation of the health checker.
func (hc *HealthChecker) String() string {
	return fmt.Sprintf("HealthChecker{target: %s, ready: %v, interval: %v}",
		hc.targetAddr, hc.ready.Load(), hc.checkInterval)
}
