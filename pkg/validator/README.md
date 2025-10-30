# MCP Validator

A production-ready Go package for validating Model Context Protocol (MCP) servers with advanced features including retry logic, metrics, and connection pooling.

## Overview

The MCP Validator provides comprehensive validation of MCP servers, supporting **Streamable HTTP** (2025-03-26, 2025-06-18) and **SSE** (2024-11-05) transport protocols. It can be used independently of the mcp-operator or integrated into any Go application.

## Features

### Core Validation
- ✅ **Transport Auto-Detection** - Automatically detects and prefers Streamable HTTP over SSE with **2-second** fast-fail detection
- ✅ **Protocol Version Support** - Supports MCP versions 2024-11-05, 2025-03-26, and 2025-06-18
- ✅ **Explicit Transport Selection** - Optionally specify `streamable-http` or `sse` to skip auto-detection
- ✅ **Capability Discovery** - Identifies tools, resources, prompts, and logging capabilities
- ✅ **Comprehensive Testing** - Tests capability endpoints (tools/list, resources/list, prompts/list)
- ✅ **Transport Interface** - Pluggable transport system for extensibility

### Reliability
- ✅ **Retry Logic** - Automatic retry with exponential backoff for transient failures
- ✅ **Enhanced Error Messages** - Actionable suggestions for fixing issues with 12 issue templates
- ✅ **Timeout Management** - Granular timeout controls (detection, connection, request, TLS)
- ✅ **Strict Mode** - Optional strict validation mode for production use

### Performance
- ✅ **HTTP Client Reuse** - Connection pooling for 50-90% reduction in connection overhead
- ✅ **Keep-Alive Support** - HTTP/1.1 persistent connections enabled by default
- ✅ **Configurable Pools** - Control max connections, idle connections, and timeouts
- ✅ **Optimized for Concurrency** - High-volume configuration preset available

### Observability
- ✅ **Prometheus Metrics** - 6 built-in metrics for monitoring validation operations
- ✅ **Protocol Version Tracking** - Track version distribution across servers
- ✅ **Error Metrics** - Monitor error types and frequencies
- ✅ **Retry Metrics** - Measure retry behavior and tune configuration

## Installation

```bash
go get github.com/vitorbari/mcp-operator/pkg/validator
```

## Quick Start

### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/vitorbari/mcp-operator/pkg/validator"
)

func main() {
    // Create validator for your MCP server
    v := validator.NewValidator("http://localhost:3001")

    // Run validation
    result, err := v.Validate(context.Background(), validator.ValidationOptions{})
    if err != nil {
        log.Fatal(err)
    }

    // Check results
    if result.Success {
        fmt.Printf("✅ Server is MCP compliant!\n")
        fmt.Printf("Protocol: %s\n", result.ProtocolVersion)
        fmt.Printf("Transport: %s\n", result.DetectedTransport)
        fmt.Printf("Capabilities: %v\n", result.Capabilities)
    } else {
        fmt.Printf("❌ Validation failed:\n")
        for _, issue := range result.Issues {
            fmt.Printf("  [%s] %s\n", issue.Level, issue.Message)
        }
    }
}
```

### Advanced Configuration

```go
// Use functional options for custom configuration
v := validator.NewValidator(
    "http://localhost:3001",
    validator.WithTimeout(60*time.Second),
    validator.WithHTTPClient(customHTTPClient),
)

result, err := v.Validate(context.Background(), validator.ValidationOptions{})

// Or use comprehensive configuration
config := validator.ValidatorConfig{
    BaseURL: "http://localhost:3001",
    Timeouts: validator.DefaultTimeoutConfig(),
    HTTPClient: validator.DefaultHTTPClientConfig(),
}

v := validator.NewValidatorWithConfig(config)
result, err := v.Validate(context.Background(), validator.ValidationOptions{})
```

### With Retry Logic

```go
// Create validator with retry support
v := validator.NewValidator("http://localhost:3001")
retryable := validator.NewRetryableValidatorWithDefaults(v)

// Automatic retry on transient failures
result, err := retryable.Validate(context.Background(), validator.ValidationOptions{})
```

### Enhanced Error Messages

```go
result, _ := v.Validate(context.Background(), validator.ValidationOptions{})

if !result.Success {
    // Issues automatically include actionable suggestions
    for _, issue := range result.Issues {
        fmt.Printf("\n[%s] %s: %s\n", issue.Level, issue.Code, issue.Message)

        if len(issue.Suggestions) > 0 {
            fmt.Println("Suggestions:")
            for i, suggestion := range issue.Suggestions {
                fmt.Printf("  %d. %s\n", i+1, suggestion)
            }
        }

        if issue.DocumentationURL != "" {
            fmt.Printf("\nDocumentation: %s\n", issue.DocumentationURL)
        }
    }

    // Or use EnhanceIssues() for backward compatibility
    enhanced := result.EnhanceIssues()
    // ... use enhanced issues
}
```

### Fluent API

```go
// Chain configuration options
opts := validator.ValidationOptions{}.
    WithStrictMode().
    WithRequiredCapabilities("tools", "resources").
    WithTransport(validator.TransportStreamableHTTP).
    WithPath("/mcp").
    WithTimeouts(validator.FastTimeoutConfig())

result, err := v.Validate(context.Background(), opts)
```

## Configuration Presets

### Timeout Configurations

```go
// Default timeouts (30s overall, balanced)
config := validator.DefaultTimeoutConfig()

// Fast timeouts (10s overall, fail-fast)
config := validator.FastTimeoutConfig()

// Slow timeouts (120s overall, patient)
config := validator.SlowTimeoutConfig()
```

### HTTP Client Configurations

```go
// Default (100 idle connections, good for most cases)
clientConfig := validator.DefaultHTTPClientConfig()

// High-volume (200 idle connections, for concurrent validation)
clientConfig := validator.HighVolumeHTTPClientConfig()
```

## Advanced Usage

### Custom Retry Configuration

```go
v := validator.NewValidator("http://localhost:3001")

retryConfig := validator.RetryConfig{
    MaxAttempts:  5,
    InitialDelay: 1 * time.Second,
    MaxDelay:     30 * time.Second,
    Multiplier:   2.0,
    RetryableErrors: []string{
        "connection refused",
        "timeout",
        "temporary failure",
    },
}

retryable := validator.NewRetryableValidator(v, retryConfig)
result, err := retryable.Validate(context.Background(), validator.ValidationOptions{})
```

### Custom HTTP Client

```go
config := validator.ValidatorConfig{
    BaseURL: "http://localhost:3001",
    HTTPClient: validator.HTTPClientConfig{
        MaxIdleConns:        200,
        MaxIdleConnsPerHost: 20,
        IdleConnTimeout:     120 * time.Second,
        DisableKeepAlives:   false,
    },
}

v := validator.NewValidatorWithConfig(config)
```

### Custom Metrics Recorder

```go
// Use NoOp recorder for testing
v := validator.NewValidator("http://localhost:3001")
v.SetMetricsRecorder(validator.NewNoOpMetricsRecorder())

// Or implement custom MetricsRecorder interface
type CustomRecorder struct{}
func (r *CustomRecorder) RecordValidation(...) {}
// ... implement other methods

v.SetMetricsRecorder(&CustomRecorder{})
```

## API Reference

### Types

#### Validator

```go
// Create a new validator with optional configuration
func NewValidator(baseURL string, opts ...Option) *Validator
func NewValidatorWithTimeout(baseURL string, timeout time.Duration) *Validator
func NewValidatorWithConfig(config ValidatorConfig) *Validator

// Functional options for NewValidator
func WithTimeout(d time.Duration) Option
func WithFactory(f TransportFactory) Option
func WithHTTPClient(client *http.Client) Option
func WithMetricsRecorder(m MetricsRecorder) Option

// Validate an MCP server
func (v *Validator) Validate(ctx context.Context, opts ValidationOptions) (*ValidationResult, error)

// Configure metrics
func (v *Validator) SetMetricsRecorder(recorder MetricsRecorder)
```

#### RetryableValidator

```go
// Create with retry support
func NewRetryableValidator(validator *Validator, config RetryConfig) *RetryableValidator
func NewRetryableValidatorWithDefaults(validator *Validator) *RetryableValidator

// Validate with automatic retry
func (r *RetryableValidator) Validate(ctx context.Context, opts ValidationOptions) (*ValidationResult, error)

// Get configuration
func (r *RetryableValidator) GetConfig() RetryConfig
func (r *RetryableValidator) GetValidator() *Validator
```

#### ValidationOptions

```go
type ValidationOptions struct {
    RequiredCapabilities []string      // Capabilities that must be present
    Timeout              time.Duration // Timeout for validation operations
    StrictMode           bool          // Requires all checks to pass
    ConfiguredPath       string        // Path to test (e.g., "/mcp" or "/sse")
    Transport            TransportType // Transport to use (skips auto-detection if set)
}

// Fluent API methods
func (opts ValidationOptions) WithTimeouts(timeouts TimeoutConfig) ValidationOptions
func (opts ValidationOptions) WithStrictMode() ValidationOptions
func (opts ValidationOptions) WithRequiredCapabilities(capabilities ...string) ValidationOptions
func (opts ValidationOptions) WithTransport(transport TransportType) ValidationOptions
func (opts ValidationOptions) WithPath(path string) ValidationOptions
```

#### ValidationIssue

```go
type ValidationIssue struct {
    Level            string   // Severity: "error", "warning", "info"
    Message          string   // Human-readable description
    Code             string   // Machine-readable issue code
    Suggestions      []string // Actionable steps (auto-populated)
    DocumentationURL string   // More information (auto-populated)
    RelatedIssues    []string // Related issue codes (auto-populated)
}
```

Issues are automatically enhanced with suggestions during validation. No separate enhancement step needed!

#### ValidationResult

```go
type ValidationResult struct {
    Success           bool              // Overall validation result
    ProtocolVersion   string            // Detected MCP protocol version
    Capabilities      []string          // Discovered capabilities
    ServerInfo        *ServerInfo       // Server implementation details
    Issues            []ValidationIssue // Validation problems (pre-enhanced with suggestions)
    Duration          time.Duration     // Validation duration
    DetectedTransport TransportType     // Transport protocol used
    Endpoint          string            // Full URL that was validated
}

// Convenience methods
func (r *ValidationResult) IsCompliant() bool
func (r *ValidationResult) HasErrors() bool
func (r *ValidationResult) ErrorMessages() []string
func (r *ValidationResult) EnhanceIssues() []EnhancedValidationIssue // For backward compatibility
func (r *ValidationResult) EnhanceIssuesWithCatalog(catalog *IssueCatalog) []EnhancedValidationIssue
```

#### ValidatorConfig

```go
type ValidatorConfig struct {
    BaseURL          string
    Timeouts         TimeoutConfig
    HTTPClient       HTTPClientConfig
    MetricsRecorder  MetricsRecorder
    TransportFactory TransportFactory
}

func DefaultValidatorConfig(baseURL string) ValidatorConfig
```

#### TimeoutConfig

```go
type TimeoutConfig struct {
    Overall      time.Duration // Overall validation timeout
    Detection    time.Duration // Transport detection timeout
    Connection   time.Duration // TCP connection timeout
    Request      time.Duration // HTTP request timeout
    TLSHandshake time.Duration // TLS handshake timeout
}

func DefaultTimeoutConfig() TimeoutConfig
func FastTimeoutConfig() TimeoutConfig
func SlowTimeoutConfig() TimeoutConfig
```

#### RetryConfig

```go
type RetryConfig struct {
    MaxAttempts     int
    InitialDelay    time.Duration
    MaxDelay        time.Duration
    Multiplier      float64
    RetryableErrors []string
}

func DefaultRetryConfig() RetryConfig
func NoRetryConfig() RetryConfig
```

## Prometheus Metrics

The validator exposes the following Prometheus metrics via controller-runtime:

### `mcp_validator_duration_seconds` (Histogram)
Labels: `transport`, `success`
Tracks validation operation duration.

### `mcp_validator_validations_total` (Counter)
Labels: `transport`, `result`
Total count of validation operations.

### `mcp_validator_detection_attempts_total` (Counter)
Labels: `transport`, `success`
Count of transport auto-detection attempts.

### `mcp_validator_retries` (Histogram)
Labels: `transport`
Number of retry attempts per validation.

### `mcp_validator_errors_total` (Counter)
Labels: `error_code`, `transport`
Count of validation errors by type.

### `mcp_validator_protocol_versions_total` (Counter)
Labels: `version`
Track protocol version distribution.

### Example PromQL Queries

```promql
# Average validation duration by transport
rate(mcp_validator_duration_seconds_sum[5m]) / rate(mcp_validator_duration_seconds_count[5m])

# Validation failure rate
rate(mcp_validator_validations_total{result="false"}[5m]) / rate(mcp_validator_validations_total[5m])

# Validations per minute
rate(mcp_validator_validations_total[1m]) * 60

# Most common errors
topk(5, sum by (error_code) (rate(mcp_validator_errors_total[5m])))
```

## Testing

The validator comes with comprehensive test suites:

### Unit Tests

```bash
go test ./pkg/validator/
```

### Integration Tests

Test against real MCP servers:

```bash
# Start an MCP server
docker run -d -p 3001:3001 tzolov/mcp-everything-server:v3

# Run SSE integration tests
export MCP_SSE_TEST_ENDPOINT=http://localhost:3001/sse
go test -v -run TestSSEClient ./pkg/validator/

# Run HTTP integration tests
export MCP_HTTP_TEST_ENDPOINT=http://localhost:3001/mcp
go test -v -run TestStreamableHTTPClient ./pkg/validator/
```

### Integration Tests (End-to-End)

End-to-end tests against actual MCP server implementations using testcontainers:

```bash
# Run integration tests with the 'integration' build tag
go test -tags integration -v -run TestRealWorld ./pkg/validator/
```

## Examples

### Example 1: Production-Ready Validation with Retry

```go
func validateServer(serverURL string) error {
    // Create validator with functional options
    v := validator.NewValidator(
        serverURL,
        validator.WithTimeout(30*time.Second),
    )

    // Wrap with retry logic
    retryable := validator.NewRetryableValidatorWithDefaults(v)

    // Validate with strict mode and required capabilities
    opts := validator.ValidationOptions{}.
        WithStrictMode().
        WithRequiredCapabilities("tools").
        WithTimeouts(validator.DefaultTimeoutConfig())

    result, err := retryable.Validate(context.Background(), opts)
    if err != nil {
        return fmt.Errorf("validation error: %w", err)
    }

    if !result.Success {
        // Issues already include actionable suggestions
        for _, issue := range result.Issues {
            log.Printf("[%s] %s", issue.Code, issue.Message)
            if len(issue.Suggestions) > 0 {
                log.Printf("  Suggestion: %s", issue.Suggestions[0])
            }
        }
        return fmt.Errorf("server not compliant")
    }

    return nil
}
```

### Example 2: High-Volume Concurrent Validation

```go
func validateMultipleServers(serverURLs []string) {
    // Use high-volume configuration
    config := validator.ValidatorConfig{
        Timeouts:   validator.FastTimeoutConfig(),
        HTTPClient: validator.HighVolumeHTTPClientConfig(),
    }

    var wg sync.WaitGroup
    for _, url := range serverURLs {
        wg.Add(1)
        go func(serverURL string) {
            defer wg.Done()

            config.BaseURL = serverURL
            v := validator.NewValidatorWithConfig(config)

            result, err := v.Validate(context.Background(), validator.ValidationOptions{})
            if err != nil || !result.Success {
                log.Printf("❌ %s failed validation", serverURL)
            } else {
                log.Printf("✅ %s validated successfully", serverURL)
            }
        }(url)
    }
    wg.Wait()
}
```

### Example 3: Monitoring with Metrics

```go
// Metrics are automatically recorded to Prometheus
// Access them via controller-runtime metrics endpoint

func setupMetrics() {
    http.Handle("/metrics", promhttp.Handler())
    go http.ListenAndServe(":8080", nil)
}

func main() {
    setupMetrics()

    // All validations automatically record metrics
    v := validator.NewValidator("http://localhost:3001")
    result, _ := v.Validate(context.Background(), validator.ValidationOptions{})

    // Metrics are available at http://localhost:8080/metrics
}
```

## Protocol Support

- **MCP 2024-11-05** - Legacy SSE-based transport ✅
- **MCP 2025-03-26** - Streamable HTTP transport ✅
- **MCP 2025-06-18** - Latest with OAuth 2.1 support ✅ (Preferred)

The validator automatically detects and prefers the newer Streamable HTTP transport when both are available.
