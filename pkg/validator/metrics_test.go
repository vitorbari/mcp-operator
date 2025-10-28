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
	"sync"
	"testing"
	"time"
)

// MockMetricsRecorder is a test implementation of MetricsRecorder
type MockMetricsRecorder struct {
	mu                   sync.Mutex
	validations          []ValidationMetric
	detections           []DetectionMetric
	retries              []RetryMetric
	errors               []ErrorMetric
	protocolVersions     []string
}

type ValidationMetric struct {
	Transport string
	Success   bool
	Duration  time.Duration
}

type DetectionMetric struct {
	Transport string
	Success   bool
}

type RetryMetric struct {
	Transport string
	Retries   int
}

type ErrorMetric struct {
	ErrorCode string
	Transport string
}

func NewMockMetricsRecorder() *MockMetricsRecorder {
	return &MockMetricsRecorder{
		validations:      []ValidationMetric{},
		detections:       []DetectionMetric{},
		retries:          []RetryMetric{},
		errors:           []ErrorMetric{},
		protocolVersions: []string{},
	}
}

func (m *MockMetricsRecorder) RecordValidation(transport string, success bool, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.validations = append(m.validations, ValidationMetric{
		Transport: transport,
		Success:   success,
		Duration:  duration,
	})
}

func (m *MockMetricsRecorder) RecordDetection(transport string, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.detections = append(m.detections, DetectionMetric{
		Transport: transport,
		Success:   success,
	})
}

func (m *MockMetricsRecorder) RecordRetries(transport string, retries int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retries = append(m.retries, RetryMetric{
		Transport: transport,
		Retries:   retries,
	})
}

func (m *MockMetricsRecorder) RecordError(errorCode string, transport string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors = append(m.errors, ErrorMetric{
		ErrorCode: errorCode,
		Transport: transport,
	})
}

func (m *MockMetricsRecorder) RecordProtocolVersion(version string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.protocolVersions = append(m.protocolVersions, version)
}

func (m *MockMetricsRecorder) GetValidations() []ValidationMetric {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]ValidationMetric{}, m.validations...)
}

func (m *MockMetricsRecorder) GetRetries() []RetryMetric {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]RetryMetric{}, m.retries...)
}

func (m *MockMetricsRecorder) GetErrors() []ErrorMetric {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]ErrorMetric{}, m.errors...)
}

func (m *MockMetricsRecorder) GetProtocolVersions() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.protocolVersions...)
}

func TestNewMetricsRecorder(t *testing.T) {
	recorder := NewMetricsRecorder()
	if recorder == nil {
		t.Fatal("NewMetricsRecorder returned nil")
	}

	// Should be a PrometheusMetricsRecorder
	if _, ok := recorder.(*PrometheusMetricsRecorder); !ok {
		t.Errorf("Expected PrometheusMetricsRecorder, got %T", recorder)
	}
}

func TestNewNoOpMetricsRecorder(t *testing.T) {
	recorder := NewNoOpMetricsRecorder()
	if recorder == nil {
		t.Fatal("NewNoOpMetricsRecorder returned nil")
	}

	// Should not panic when called
	recorder.RecordValidation("http", true, time.Second)
	recorder.RecordDetection("http", true)
	recorder.RecordRetries("http", 2)
	recorder.RecordError("TEST_ERROR", "http")
	recorder.RecordProtocolVersion("2024-11-05")
}

func TestValidator_RecordsMetrics(t *testing.T) {
	// Create a mock server
	config := validServerConfig()
	server := mockMCPServer(t, config)
	defer server.Close()

	// Create validator with mock metrics recorder
	validator := NewValidator(server.URL)
	mockRecorder := NewMockMetricsRecorder()
	validator.SetMetricsRecorder(mockRecorder)

	// Perform validation
	ctx := context.Background()
	result, err := validator.Validate(ctx, ValidationOptions{})

	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected successful validation")
	}

	// Check that validation metrics were recorded
	validations := mockRecorder.GetValidations()
	if len(validations) != 1 {
		t.Fatalf("Expected 1 validation metric, got %d", len(validations))
	}

	validation := validations[0]
	if validation.Transport != string(TransportStreamableHTTP) {
		t.Errorf("Expected transport %s, got %s", TransportStreamableHTTP, validation.Transport)
	}

	if !validation.Success {
		t.Error("Expected validation success to be true")
	}

	if validation.Duration == 0 {
		t.Error("Expected duration to be recorded")
	}

	// Check that protocol version was recorded
	versions := mockRecorder.GetProtocolVersions()
	if len(versions) != 1 {
		t.Fatalf("Expected 1 protocol version, got %d", len(versions))
	}

	if versions[0] == "" {
		t.Error("Expected protocol version to be recorded")
	}
}

func TestValidator_RecordsErrors(t *testing.T) {
	// Create validator pointing to non-existent server
	validator := NewValidator("http://localhost:1")
	mockRecorder := NewMockMetricsRecorder()
	validator.SetMetricsRecorder(mockRecorder)

	// Perform validation (will fail)
	ctx := context.Background()
	result, _ := validator.Validate(ctx, ValidationOptions{})

	if result.Success {
		t.Error("Expected validation to fail")
	}

	// Validation metrics should still be recorded even on failure
	validations := mockRecorder.GetValidations()
	if len(validations) != 1 {
		t.Fatalf("Expected 1 validation metric, got %d", len(validations))
	}

	if validations[0].Success {
		t.Error("Expected validation to be recorded as failed")
	}

	// Check that error metrics were recorded
	errors := mockRecorder.GetErrors()
	if len(errors) == 0 {
		t.Fatal("Expected error metrics to be recorded")
	}

	// Should have recorded TRANSPORT_DETECTION_FAILED
	foundTransportError := false
	for _, err := range errors {
		if err.ErrorCode == "TRANSPORT_DETECTION_FAILED" {
			foundTransportError = true
			break
		}
	}

	if !foundTransportError {
		t.Error("Expected TRANSPORT_DETECTION_FAILED error to be recorded")
	}
}

func TestRetryableValidator_RecordsRetries(t *testing.T) {
	// Create a mock server that fails first, then succeeds
	config := validServerConfig()
	server := mockMCPServer(t, config)
	defer server.Close()

	// Create retryable validator with mock metrics recorder
	validator := NewValidator(server.URL)
	retryConfig := RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}
	retryable := NewRetryableValidator(validator, retryConfig)
	mockRecorder := NewMockMetricsRecorder()
	retryable.SetMetricsRecorder(mockRecorder)

	// Perform validation
	ctx := context.Background()
	result, _ := retryable.Validate(ctx, ValidationOptions{})

	if !result.Success {
		t.Error("Expected validation to succeed")
	}

	// Since server is healthy, no retries should occur
	retries := mockRecorder.GetRetries()
	if len(retries) != 0 {
		t.Errorf("Expected 0 retry records for successful first attempt, got %d", len(retries))
	}
}

func TestRetryableValidator_RecordsRetriesOnFailure(t *testing.T) {
	// Create validator pointing to non-existent server
	validator := NewValidator("http://localhost:1")
	retryConfig := RetryConfig{
		MaxAttempts:     2,
		InitialDelay:    10 * time.Millisecond,
		MaxDelay:        50 * time.Millisecond,
		Multiplier:      2.0,
		RetryableErrors: []string{"connection refused", "transport"},
	}
	retryable := NewRetryableValidator(validator, retryConfig)
	mockRecorder := NewMockMetricsRecorder()
	retryable.SetMetricsRecorder(mockRecorder)

	// Perform validation (will fail and retry)
	ctx := context.Background()
	retryable.Validate(ctx, ValidationOptions{})

	// Check that retry metrics were recorded
	retries := mockRecorder.GetRetries()
	if len(retries) != 1 {
		t.Fatalf("Expected 1 retry record, got %d", len(retries))
	}

	retry := retries[0]
	if retry.Retries < 1 {
		t.Errorf("Expected at least 1 retry attempt, got %d", retry.Retries)
	}
}

func TestPrometheusMetricsRecorder_DoesNotPanic(t *testing.T) {
	recorder := NewMetricsRecorder()

	// These should not panic
	recorder.RecordValidation("http", true, time.Second)
	recorder.RecordDetection("sse", true)
	recorder.RecordRetries("http", 2)
	recorder.RecordError("TEST_ERROR", "http")
	recorder.RecordProtocolVersion("2024-11-05")
}

func TestSetDefaultMetricsRecorder(t *testing.T) {
	// Save original recorder
	original := GetDefaultMetricsRecorder()
	defer SetDefaultMetricsRecorder(original)

	// Set a mock recorder
	mock := NewMockMetricsRecorder()
	SetDefaultMetricsRecorder(mock)

	// Verify it was set
	current := GetDefaultMetricsRecorder()
	if current != mock {
		t.Error("Default metrics recorder was not updated")
	}

	// Test convenience functions use the default recorder
	RecordValidation("http", true, time.Second)
	RecordDetection("sse", true)
	RecordRetries("http", 2)
	RecordError("TEST", "http")
	RecordProtocolVersion("2024-11-05")

	// Verify mock recorder received the calls
	if len(mock.GetValidations()) != 1 {
		t.Error("Expected validation to be recorded via convenience function")
	}

	if len(mock.GetProtocolVersions()) != 1 {
		t.Error("Expected protocol version to be recorded via convenience function")
	}
}

func TestMetrics_ConcurrentAccess(t *testing.T) {
	mock := NewMockMetricsRecorder()

	// Simulate concurrent metric recording
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mock.RecordValidation("http", true, time.Millisecond)
			mock.RecordError("TEST", "http")
			mock.RecordProtocolVersion("2024-11-05")
		}()
	}

	wg.Wait()

	// Verify all metrics were recorded
	if len(mock.GetValidations()) != 10 {
		t.Errorf("Expected 10 validations, got %d", len(mock.GetValidations()))
	}

	if len(mock.GetErrors()) != 10 {
		t.Errorf("Expected 10 errors, got %d", len(mock.GetErrors()))
	}

	if len(mock.GetProtocolVersions()) != 10 {
		t.Errorf("Expected 10 protocol versions, got %d", len(mock.GetProtocolVersions()))
	}
}

func TestValidator_MetricsWithFailedValidation(t *testing.T) {
	// Create a mock server with invalid protocol version
	config := validServerConfig()
	config.protocolVersion = "invalid-version"
	server := mockMCPServer(t, config)
	defer server.Close()

	// Create validator with mock metrics recorder
	validator := NewValidator(server.URL)
	mockRecorder := NewMockMetricsRecorder()
	validator.SetMetricsRecorder(mockRecorder)

	// Perform validation (will fail)
	ctx := context.Background()
	result, _ := validator.Validate(ctx, ValidationOptions{})

	if result.Success {
		t.Error("Expected validation to fail for invalid protocol")
	}

	// Check that validation was recorded as failure
	validations := mockRecorder.GetValidations()
	if len(validations) != 1 {
		t.Fatalf("Expected 1 validation metric, got %d", len(validations))
	}

	if validations[0].Success {
		t.Error("Expected validation success to be false")
	}

	// Check that error was recorded
	errors := mockRecorder.GetErrors()
	if len(errors) == 0 {
		t.Fatal("Expected error to be recorded")
	}

	foundProtocolError := false
	for _, err := range errors {
		if err.ErrorCode == CodeInvalidProtocolVersion {
			foundProtocolError = true
			break
		}
	}

	if !foundProtocolError {
		t.Error("Expected INVALID_PROTOCOL error to be recorded")
	}
}
