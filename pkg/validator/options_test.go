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
	"net/http"
	"testing"
	"time"
)

func TestDefaultTimeoutConfig(t *testing.T) {
	config := DefaultTimeoutConfig()

	if config.Overall != 30*time.Second {
		t.Errorf("Expected Overall timeout of 30s, got %v", config.Overall)
	}

	if config.Detection != 10*time.Second {
		t.Errorf("Expected Detection timeout of 10s, got %v", config.Detection)
	}

	if config.Connection != 10*time.Second {
		t.Errorf("Expected Connection timeout of 10s, got %v", config.Connection)
	}

	if config.Request != 30*time.Second {
		t.Errorf("Expected Request timeout of 30s, got %v", config.Request)
	}

	if config.TLSHandshake != 10*time.Second {
		t.Errorf("Expected TLSHandshake timeout of 10s, got %v", config.TLSHandshake)
	}
}

func TestFastTimeoutConfig(t *testing.T) {
	config := FastTimeoutConfig()

	if config.Overall >= 15*time.Second {
		t.Errorf("FastTimeoutConfig should have Overall < 15s, got %v", config.Overall)
	}

	if config.Connection >= 5*time.Second {
		t.Errorf("FastTimeoutConfig should have Connection < 5s, got %v", config.Connection)
	}
}

func TestSlowTimeoutConfig(t *testing.T) {
	config := SlowTimeoutConfig()

	if config.Overall < 60*time.Second {
		t.Errorf("SlowTimeoutConfig should have Overall >= 60s, got %v", config.Overall)
	}

	if config.Connection < 15*time.Second {
		t.Errorf("SlowTimeoutConfig should have Connection >= 15s, got %v", config.Connection)
	}
}

func TestDefaultHTTPClientConfig(t *testing.T) {
	config := DefaultHTTPClientConfig()

	if config.MaxIdleConns != 100 {
		t.Errorf("Expected MaxIdleConns = 100, got %d", config.MaxIdleConns)
	}

	if config.MaxIdleConnsPerHost != 10 {
		t.Errorf("Expected MaxIdleConnsPerHost = 10, got %d", config.MaxIdleConnsPerHost)
	}

	if config.IdleConnTimeout != 90*time.Second {
		t.Errorf("Expected IdleConnTimeout = 90s, got %v", config.IdleConnTimeout)
	}

	if config.DisableKeepAlives {
		t.Error("Expected keep-alives to be enabled by default")
	}
}

func TestHighVolumeHTTPClientConfig(t *testing.T) {
	config := HighVolumeHTTPClientConfig()

	if config.MaxIdleConns < 100 {
		t.Errorf("HighVolumeHTTPClientConfig should have MaxIdleConns >= 100, got %d", config.MaxIdleConns)
	}

	if config.MaxIdleConnsPerHost < 10 {
		t.Errorf("HighVolumeHTTPClientConfig should have MaxIdleConnsPerHost >= 10, got %d", config.MaxIdleConnsPerHost)
	}
}

func TestDefaultValidatorConfig(t *testing.T) {
	baseURL := "http://localhost:8080"
	config := DefaultValidatorConfig(baseURL)

	if config.BaseURL != baseURL {
		t.Errorf("Expected BaseURL = %s, got %s", baseURL, config.BaseURL)
	}

	if config.Timeouts.Overall == 0 {
		t.Error("Expected Timeouts to be populated")
	}

	if config.HTTPClient.MaxIdleConns == 0 {
		t.Error("Expected HTTPClient config to be populated")
	}
}

func TestNewValidatorWithConfig(t *testing.T) {
	config := ValidatorConfig{
		BaseURL:  "http://localhost:8080",
		Timeouts: DefaultTimeoutConfig(),
		HTTPClient: HTTPClientConfig{
			MaxIdleConns:        50,
			MaxIdleConnsPerHost: 5,
		},
	}

	validator := NewValidatorWithConfig(config)

	if validator == nil {
		t.Fatal("NewValidatorWithConfig returned nil")
	}

	if validator.baseURL != config.BaseURL {
		t.Errorf("Expected baseURL = %s, got %s", config.BaseURL, validator.baseURL)
	}

	if validator.timeout != config.Timeouts.Overall {
		t.Errorf("Expected timeout = %v, got %v", config.Timeouts.Overall, validator.timeout)
	}
}

func TestNewValidatorWithConfig_DefaultTimeouts(t *testing.T) {
	config := ValidatorConfig{
		BaseURL: "http://localhost:8080",
		// Timeouts left at zero - should use defaults
	}

	validator := NewValidatorWithConfig(config)

	if validator.timeout == 0 {
		t.Error("Expected default timeout to be applied")
	}
}

func TestNewValidatorWithConfig_CustomMetrics(t *testing.T) {
	mockRecorder := NewMockMetricsRecorder()

	config := ValidatorConfig{
		BaseURL:         "http://localhost:8080",
		MetricsRecorder: mockRecorder,
	}

	validator := NewValidatorWithConfig(config)

	if validator.metricsRecorder != mockRecorder {
		t.Error("Expected custom metrics recorder to be used")
	}
}

func TestValidationOptions_WithTimeouts(t *testing.T) {
	opts := ValidationOptions{}
	timeouts := TimeoutConfig{
		Overall: 45 * time.Second,
	}

	newOpts := opts.WithTimeouts(timeouts)

	if newOpts.Timeout != 45*time.Second {
		t.Errorf("Expected timeout = 45s, got %v", newOpts.Timeout)
	}

	// Original should be unchanged
	if opts.Timeout != 0 {
		t.Error("Original ValidationOptions should not be modified")
	}
}

func TestValidationOptions_WithStrictMode(t *testing.T) {
	opts := ValidationOptions{}

	newOpts := opts.WithStrictMode()

	if !newOpts.StrictMode {
		t.Error("Expected StrictMode to be true")
	}

	if opts.StrictMode {
		t.Error("Original ValidationOptions should not be modified")
	}
}

func TestValidationOptions_WithRequiredCapabilities(t *testing.T) {
	opts := ValidationOptions{}

	newOpts := opts.WithRequiredCapabilities("tools", "resources")

	if len(newOpts.RequiredCapabilities) != 2 {
		t.Errorf("Expected 2 capabilities, got %d", len(newOpts.RequiredCapabilities))
	}

	if newOpts.RequiredCapabilities[0] != "tools" {
		t.Errorf("Expected first capability to be 'tools', got %s", newOpts.RequiredCapabilities[0])
	}

	if len(opts.RequiredCapabilities) != 0 {
		t.Error("Original ValidationOptions should not be modified")
	}
}

func TestValidationOptions_WithTransport(t *testing.T) {
	opts := ValidationOptions{}

	newOpts := opts.WithTransport(TransportStreamableHTTP)

	if newOpts.Transport != TransportStreamableHTTP {
		t.Errorf("Expected transport = %s, got %s", TransportStreamableHTTP, newOpts.Transport)
	}

	if opts.Transport != "" {
		t.Error("Original ValidationOptions should not be modified")
	}
}

func TestValidationOptions_WithPath(t *testing.T) {
	opts := ValidationOptions{}

	newOpts := opts.WithPath("/custom/mcp")

	if newOpts.ConfiguredPath != "/custom/mcp" {
		t.Errorf("Expected path = /custom/mcp, got %s", newOpts.ConfiguredPath)
	}

	if opts.ConfiguredPath != "" {
		t.Error("Original ValidationOptions should not be modified")
	}
}

func TestValidationOptions_Chaining(t *testing.T) {
	// Test that options can be chained
	opts := ValidationOptions{}.
		WithStrictMode().
		WithRequiredCapabilities("tools").
		WithTransport(TransportStreamableHTTP).
		WithPath("/mcp")

	if !opts.StrictMode {
		t.Error("Expected StrictMode to be true")
	}

	if len(opts.RequiredCapabilities) != 1 {
		t.Error("Expected 1 required capability")
	}

	if opts.Transport != TransportStreamableHTTP {
		t.Error("Expected transport to be set")
	}

	if opts.ConfiguredPath != "/mcp" {
		t.Error("Expected path to be set")
	}
}

func TestCreateHTTPClient_WithDefaults(t *testing.T) {
	timeouts := DefaultTimeoutConfig()
	clientConfig := DefaultHTTPClientConfig()

	client := createHTTPClient(timeouts, clientConfig)

	if client == nil {
		t.Fatal("createHTTPClient returned nil")
	}

	if client.Timeout != timeouts.Request {
		t.Errorf("Expected client timeout = %v, got %v", timeouts.Request, client.Timeout)
	}

	// Check that transport is configured
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Expected http.Transport")
	}

	if transport.MaxIdleConns != clientConfig.MaxIdleConns {
		t.Errorf("Expected MaxIdleConns = %d, got %d", clientConfig.MaxIdleConns, transport.MaxIdleConns)
	}
}

func TestNewValidatorWithConfig_HTTPClientReuse(t *testing.T) {
	// Create multiple validators with same config
	config := DefaultValidatorConfig("http://localhost:8080")

	v1 := NewValidatorWithConfig(config)
	v2 := NewValidatorWithConfig(config)

	// Both should be valid
	if v1 == nil || v2 == nil {
		t.Fatal("Validators should not be nil")
	}

	// They should be different validator instances
	if v1 == v2 {
		t.Error("Expected different validator instances")
	}
}

func TestValidatorWithConfig_ActualValidation(t *testing.T) {
	// Create a mock server
	serverConfig := validServerConfig()
	server := mockMCPServer(t, serverConfig)
	defer server.Close()

	// Create validator with custom config
	config := ValidatorConfig{
		BaseURL: server.URL,
		Timeouts: TimeoutConfig{
			Overall:    10 * time.Second,
			Connection: 5 * time.Second,
			Request:    10 * time.Second,
		},
		HTTPClient: DefaultHTTPClientConfig(),
	}

	validator := NewValidatorWithConfig(config)

	// Perform validation
	ctx := context.Background()
	result, err := validator.Validate(ctx, ValidationOptions{})

	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected successful validation, issues: %v", result.Issues)
	}
}

func TestTimeoutDialer(t *testing.T) {
	dialer := &TimeoutDialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	// Test with a resolvable address (Google DNS)
	ctx := context.Background()
	conn, err := dialer.DialContext(ctx, "tcp", "8.8.8.8:53")

	if err != nil {
		// This might fail in some network environments, but the dialer should be valid
		t.Logf("Dial failed (may be expected in restricted network): %v", err)
	} else {
		defer func() {
			_ = conn.Close()
		}()
		if conn == nil {
			t.Error("Expected non-nil connection")
		}
	}
}

func TestTimeoutDialer_DefaultValues(t *testing.T) {
	// Dialer with zero values should use defaults
	dialer := &TimeoutDialer{}

	// Should not panic
	ctx := context.Background()
	_, err := dialer.DialContext(ctx, "tcp", "8.8.8.8:53")

	// We don't care if it succeeds or fails, just that it doesn't panic
	if err != nil {
		t.Logf("Dial failed (expected in some environments): %v", err)
	}
}
