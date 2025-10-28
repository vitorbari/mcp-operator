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
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	if config.MaxAttempts != 3 {
		t.Errorf("Expected MaxAttempts = 3, got %d", config.MaxAttempts)
	}

	if config.InitialDelay != 1*time.Second {
		t.Errorf("Expected InitialDelay = 1s, got %v", config.InitialDelay)
	}

	if config.Multiplier != 2.0 {
		t.Errorf("Expected Multiplier = 2.0, got %f", config.Multiplier)
	}

	if len(config.RetryableErrors) == 0 {
		t.Error("Expected RetryableErrors to be populated")
	}
}

func TestNoRetryConfig(t *testing.T) {
	config := NoRetryConfig()

	if config.MaxAttempts != 1 {
		t.Errorf("Expected MaxAttempts = 1, got %d", config.MaxAttempts)
	}

	if len(config.RetryableErrors) != 0 {
		t.Error("Expected RetryableErrors to be empty")
	}
}

func TestNewRetryableValidator(t *testing.T) {
	validator := NewValidator("http://example.com")
	config := DefaultRetryConfig()

	retryable := NewRetryableValidator(validator, config)

	if retryable == nil {
		t.Fatal("NewRetryableValidator returned nil")
	}

	if retryable.GetValidator() != validator {
		t.Error("GetValidator() should return the wrapped validator")
	}

	if retryable.GetConfig().MaxAttempts != config.MaxAttempts {
		t.Error("GetConfig() should return the configuration")
	}
}

func TestRetryableValidator_SuccessFirstAttempt(t *testing.T) {
	// Create a mock server that succeeds immediately
	config := validServerConfig()
	server := mockMCPServer(t, config)
	defer server.Close()

	validator := NewValidator(server.URL)
	retryConfig := RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
	}
	retryable := NewRetryableValidator(validator, retryConfig)

	ctx := context.Background()
	result, err := retryable.Validate(ctx, ValidationOptions{})

	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected validation to succeed on first attempt")
	}
}

func TestRetryableValidator_RetryOnConnectionRefused(t *testing.T) {
	// Track number of attempts
	var attempts atomic.Int32

	// Create a server that fails first, then succeeds
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		currentAttempt := attempts.Add(1)

		if currentAttempt < 2 {
			// First attempt: close connection to simulate failure
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		// Second attempt: succeed
		config := validServerConfig()
		mockMCPServer(t, config).Config.Handler.ServeHTTP(w, r)
	}))
	defer server.Close()

	validator := NewValidator(server.URL)
	retryConfig := RetryConfig{
		MaxAttempts:     3,
		InitialDelay:    50 * time.Millisecond,
		MaxDelay:        500 * time.Millisecond,
		Multiplier:      2.0,
		RetryableErrors: []string{"503", "connection", "unavailable"},
	}
	retryable := NewRetryableValidator(validator, retryConfig)

	ctx := context.Background()
	startTime := time.Now()
	result, err := retryable.Validate(ctx, ValidationOptions{})
	duration := time.Since(startTime)

	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	// Should have taken some time due to retry
	if duration < 50*time.Millisecond {
		t.Errorf("Expected retry delay, but validation completed too quickly: %v", duration)
	}

	if !result.Success {
		t.Errorf("Expected validation to eventually succeed after retry, issues: %v", result.Issues)
	}

	// Should have attempted at least twice
	finalAttempts := attempts.Load()
	if finalAttempts < 2 {
		t.Errorf("Expected at least 2 attempts, got %d", finalAttempts)
	}
}

func TestRetryableValidator_MaxAttemptsReached(t *testing.T) {
	// Create a server that always fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	validator := NewValidator(server.URL)
	retryConfig := RetryConfig{
		MaxAttempts:     2,
		InitialDelay:    50 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		Multiplier:      2.0,
		RetryableErrors: []string{"503", "unavailable"},
	}
	retryable := NewRetryableValidator(validator, retryConfig)

	ctx := context.Background()
	result, err := retryable.Validate(ctx, ValidationOptions{})

	// Should fail after max attempts
	if err == nil && (result == nil || result.Success) {
		t.Error("Expected validation to fail after max attempts")
	}

	if result != nil {
		// Check for retry exhausted info
		hasRetryInfo := false
		for _, issue := range result.Issues {
			if issue.Code == "RETRIES_EXHAUSTED" {
				hasRetryInfo = true
				break
			}
		}
		if !hasRetryInfo {
			t.Error("Expected RETRIES_EXHAUSTED issue in result")
		}
	}
}

func TestRetryableValidator_NoRetryOnNonRetryableError(t *testing.T) {
	// Create a mock server with invalid protocol version (non-retryable)
	config := validServerConfig()
	config.protocolVersion = "invalid-version"
	server := mockMCPServer(t, config)
	defer server.Close()

	validator := NewValidator(server.URL)
	retryConfig := DefaultRetryConfig()
	retryable := NewRetryableValidator(validator, retryConfig)

	ctx := context.Background()
	startTime := time.Now()
	result, err := retryable.Validate(ctx, ValidationOptions{})
	duration := time.Since(startTime)

	// Should fail quickly without retries
	if duration > 2*time.Second {
		t.Errorf("Validation took too long, suggesting unwanted retries: %v", duration)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	if result.Success {
		t.Error("Expected validation to fail for invalid protocol version")
	}

	if err != nil {
		t.Logf("Error (expected): %v", err)
	}
}

func TestRetryableValidator_ContextCancellation(t *testing.T) {
	// Create a server that is slow to respond
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer server.Close()

	validator := NewValidator(server.URL)
	retryConfig := RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
	}
	retryable := NewRetryableValidator(validator, retryConfig)

	// Create a context that will be cancelled
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	startTime := time.Now()
	result, err := retryable.Validate(ctx, ValidationOptions{})
	duration := time.Since(startTime)

	// Should fail due to context cancellation
	if err == nil {
		t.Error("Expected error due to context cancellation")
	}

	// Should complete relatively quickly (not wait for all retries)
	if duration > 2*time.Second {
		t.Errorf("Validation took too long despite context cancellation: %v", duration)
	}

	t.Logf("Result: %+v, Error: %v, Duration: %v", result, err, duration)
}

func TestRetryableValidator_DisabledRetries(t *testing.T) {
	// Create a failing server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	validator := NewValidator(server.URL)
	retryConfig := NoRetryConfig() // Retries disabled
	retryable := NewRetryableValidator(validator, retryConfig)

	ctx := context.Background()
	startTime := time.Now()
	result, err := retryable.Validate(ctx, ValidationOptions{})
	duration := time.Since(startTime)

	// Should fail immediately without retries
	if duration > 2*time.Second {
		t.Errorf("Validation took too long with retries disabled: %v", duration)
	}

	if result != nil && result.Success {
		t.Error("Expected validation to fail")
	}

	t.Logf("Result: %+v, Error: %v, Duration: %v", result, err, duration)
}

func TestRetryableValidator_IsRetryable(t *testing.T) {
	validator := NewValidator("http://example.com")
	retryConfig := DefaultRetryConfig()
	retryable := NewRetryableValidator(validator, retryConfig)

	tests := []struct {
		name       string
		err        error
		result     *ValidationResult
		shouldRetry bool
	}{
		{
			name:        "ConnectionRefusedError",
			err:         fmt.Errorf("connection refused"),
			result:      nil,
			shouldRetry: true,
		},
		{
			name:        "TimeoutError",
			err:         fmt.Errorf("i/o timeout"),
			result:      nil,
			shouldRetry: true,
		},
		{
			name:        "TransportDetectionFailed",
			err:         nil,
			result:      &ValidationResult{Issues: []ValidationIssue{{Code: "TRANSPORT_DETECTION_FAILED"}}},
			shouldRetry: true,
		},
		{
			name:        "InvalidProtocolVersion",
			err:         nil,
			result:      &ValidationResult{Issues: []ValidationIssue{{Code: CodeInvalidProtocolVersion}}},
			shouldRetry: false,
		},
		{
			name:        "NoError",
			err:         nil,
			result:      &ValidationResult{Success: true},
			shouldRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := retryable.isRetryable(tt.err, tt.result)
			if result != tt.shouldRetry {
				t.Errorf("isRetryable() = %v, want %v", result, tt.shouldRetry)
			}
		})
	}
}

func TestRetryableValidator_CalculateNextDelay(t *testing.T) {
	retryConfig := RetryConfig{
		InitialDelay: 1 * time.Second,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
	}
	validator := NewValidator("http://example.com")
	retryable := NewRetryableValidator(validator, retryConfig)

	tests := []struct {
		retryCount  int
		expectedMin time.Duration
		expectedMax time.Duration
	}{
		{1, 1 * time.Second, 3 * time.Second},
		{2, 3 * time.Second, 5 * time.Second},
		{3, 5 * time.Second, 10 * time.Second},
		{10, 10 * time.Second, 10 * time.Second}, // Should cap at MaxDelay
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Retry%d", tt.retryCount), func(t *testing.T) {
			delay := retryable.calculateNextDelay(tt.retryCount)
			if delay < tt.expectedMin || delay > tt.expectedMax {
				t.Errorf("calculateNextDelay(%d) = %v, want between %v and %v",
					tt.retryCount, delay, tt.expectedMin, tt.expectedMax)
			}
		})
	}
}

func TestNewRetryableValidatorWithDefaults(t *testing.T) {
	validator := NewValidator("http://example.com")
	retryable := NewRetryableValidatorWithDefaults(validator)

	if retryable == nil {
		t.Fatal("NewRetryableValidatorWithDefaults returned nil")
	}

	config := retryable.GetConfig()
	defaultConfig := DefaultRetryConfig()

	if config.MaxAttempts != defaultConfig.MaxAttempts {
		t.Errorf("Expected MaxAttempts = %d, got %d", defaultConfig.MaxAttempts, config.MaxAttempts)
	}
}
