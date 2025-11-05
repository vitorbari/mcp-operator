# MCP Validator

A Go package for validating Model Context Protocol (MCP) servers with support for both Streamable HTTP and SSE transports.

## Overview

The MCP Validator validates MCP server implementations against the official protocol specification. It supports **Streamable HTTP** (2025-03-26, 2025-06-18) and **SSE** (2024-11-05) transports with automatic protocol detection.

This package can be used standalone or integrated into the mcp-operator Kubernetes controller.

## Features

- **Transport Auto-Detection** - Automatically detects and prefers Streamable HTTP over SSE
- **Protocol Version Support** - MCP versions 2024-11-05, 2025-03-26, and 2025-06-18
- **Capability Discovery** - Identifies tools, resources, prompts, and logging capabilities
- **Retry Logic** - Automatic retry with exponential backoff for transient failures
- **Connection Pooling** - HTTP client reuse for improved performance
- **Prometheus Metrics** - Built-in metrics for monitoring validation operations
- **Enhanced Error Messages** - Actionable suggestions for fixing validation issues

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
    // Create validator
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
            if len(issue.Suggestions) > 0 {
                fmt.Printf("    Suggestion: %s\n", issue.Suggestions[0])
            }
        }
    }
}
```

### With Retry Logic

```go
// Create validator with retry support
v := validator.NewValidator("http://localhost:3001")
retryable := validator.NewRetryableValidatorWithDefaults(v)

// Automatic retry on transient failures
result, err := retryable.Validate(context.Background(), validator.ValidationOptions{})
```

### Fluent Configuration

```go
// Chain configuration options
opts := validator.ValidationOptions{}.
    WithStrictMode().
    WithRequiredCapabilities("tools", "resources").
    WithTransport(validator.TransportStreamableHTTP).
    WithPath("/mcp")

result, err := v.Validate(context.Background(), opts)
```

## Prometheus Metrics

### For Kubernetes Operators

```go
import (
    ctrl "sigs.k8s.io/controller-runtime"
    "github.com/vitorbari/mcp-operator/pkg/validator"
)

func main() {
    // Setup manager
    mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
        // ... options
    })

    // Register validator metrics
    if err := validator.RegisterMetrics(validator.MetricsConfig{
        Register: true,
        Registry: nil, // Uses controller-runtime metrics.Registry
    }); err != nil {
        panic(err)
    }

    // Create validator (metrics enabled by default)
    v := validator.NewValidator("http://mcp-server:8080")

    // Metrics exposed alongside controller-runtime metrics
}
```

### For Standalone Applications

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "github.com/vitorbari/mcp-operator/pkg/validator"
)

func main() {
    // Create registry
    registry := prometheus.NewRegistry()

    // Register validator metrics
    if err := validator.RegisterMetrics(validator.MetricsConfig{
        Register: true,
        Registry: registry,
    }); err != nil {
        panic(err)
    }

    // Create validator
    v := validator.NewValidator("http://localhost:8080")

    // Expose metrics
    http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
    go http.ListenAndServe(":9090", nil)

    // Perform validations
    result, _ := v.Validate(context.Background(), validator.ValidationOptions{})
}
```

### Disabling Metrics

```go
// Disable during creation
v := validator.NewValidator(
    "http://localhost:8080",
    validator.WithMetricsEnabled(false),
)

// Or use NoOp recorder
v := validator.NewValidator("http://localhost:8080")
v.SetMetricsRecorder(validator.NewNoOpMetricsRecorder())
```

### Available Metrics

- **`mcp_validator_duration_seconds`** (Histogram) - Validation duration by transport and success
- **`mcp_validator_validations_total`** (Counter) - Total validation count by transport and result
- **`mcp_validator_detection_attempts_total`** (Counter) - Transport detection attempts
- **`mcp_validator_retries`** (Histogram) - Retry attempts per validation
- **`mcp_validator_errors_total`** (Counter) - Errors by code and transport
- **`mcp_validator_protocol_versions_total`** (Counter) - Protocol version distribution

## Configuration

### Timeout Presets

```go
validator.DefaultTimeoutConfig()  // 30s overall, balanced
validator.FastTimeoutConfig()     // 10s overall, fail-fast
validator.SlowTimeoutConfig()     // 120s overall, patient
```

### HTTP Client Presets

```go
validator.DefaultHTTPClientConfig()     // 100 idle connections
validator.HighVolumeHTTPClientConfig()  // 200 idle connections, high concurrency
```

### Custom Configuration

```go
config := validator.ValidatorConfig{
    BaseURL: "http://localhost:3001",
    Timeouts: validator.TimeoutConfig{
        Overall:      60 * time.Second,
        Detection:    5 * time.Second,
        Connection:   10 * time.Second,
        Request:      20 * time.Second,
        TLSHandshake: 10 * time.Second,
    },
    HTTPClient: validator.HTTPClientConfig{
        MaxIdleConns:        200,
        MaxIdleConnsPerHost: 20,
        IdleConnTimeout:     120 * time.Second,
        DisableKeepAlives:   false,
    },
}

v := validator.NewValidatorWithConfig(config)
```

### Retry Configuration

```go
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
```

## Testing

### Unit Tests

```bash
go test ./pkg/validator/
```

### Integration Tests

```bash
# Start MCP server
docker run -d -p 3001:3001 tzolov/mcp-everything-server:v3

# Test SSE
export MCP_SSE_TEST_ENDPOINT=http://localhost:3001/sse
go test -v -run TestSSEClient ./pkg/validator/

# Test Streamable HTTP
export MCP_HTTP_TEST_ENDPOINT=http://localhost:3001/mcp
go test -v -run TestStreamableHTTPClient ./pkg/validator/
```

### End-to-End Tests

```bash
# Run with integration tag
go test -tags integration -v -run TestRealWorld ./pkg/validator/
```

## Examples

### Production Validation with Retry

```go
func validateServer(serverURL string) error {
    // Create validator
    v := validator.NewValidator(
        serverURL,
        validator.WithTimeout(30*time.Second),
    )

    // Add retry logic
    retryable := validator.NewRetryableValidatorWithDefaults(v)

    // Validate with strict mode
    opts := validator.ValidationOptions{}.
        WithStrictMode().
        WithRequiredCapabilities("tools")

    result, err := retryable.Validate(context.Background(), opts)
    if err != nil {
        return fmt.Errorf("validation error: %w", err)
    }

    if !result.Success {
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

### Concurrent Validation

```go
func validateMultipleServers(serverURLs []string) {
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
                log.Printf("✅ %s validated", serverURL)
            }
        }(url)
    }
    wg.Wait()
}
```

## API Reference

### Core Types

```go
// Create validator
func NewValidator(baseURL string, opts ...Option) *Validator
func NewValidatorWithTimeout(baseURL string, timeout time.Duration) *Validator
func NewValidatorWithConfig(config ValidatorConfig) *Validator

// Validate MCP server
func (v *Validator) Validate(ctx context.Context, opts ValidationOptions) (*ValidationResult, error)
```

### Options

```go
// Functional options
func WithTimeout(d time.Duration) Option
func WithHTTPClient(client *http.Client) Option
func WithMetricsEnabled(enabled bool) Option
```

### Validation Options

```go
type ValidationOptions struct {
    RequiredCapabilities []string
    Timeout              time.Duration
    StrictMode           bool
    ConfiguredPath       string
    Transport            TransportType
}

// Fluent methods
func (opts ValidationOptions) WithStrictMode() ValidationOptions
func (opts ValidationOptions) WithRequiredCapabilities(caps ...string) ValidationOptions
func (opts ValidationOptions) WithTransport(t TransportType) ValidationOptions
func (opts ValidationOptions) WithPath(path string) ValidationOptions
```

### Validation Result

```go
type ValidationResult struct {
    Success           bool
    ProtocolVersion   string
    Capabilities      []string
    ServerInfo        *ServerInfo
    Issues            []ValidationIssue  // Pre-enhanced with suggestions
    Duration          time.Duration
    DetectedTransport TransportType
    Endpoint          string
}

func (r *ValidationResult) IsCompliant() bool
func (r *ValidationResult) HasErrors() bool
func (r *ValidationResult) ErrorMessages() []string
```

### Validation Issues

```go
type ValidationIssue struct {
    Level            string    // "error", "warning", "info"
    Message          string    // Human-readable description
    Code             string    // Machine-readable code
    Suggestions      []string  // Actionable steps (auto-populated)
    DocumentationURL string    // Reference link (auto-populated)
}
```

## Protocol Support

- **MCP 2024-11-05** - SSE transport
- **MCP 2025-03-26** - Streamable HTTP transport
- **MCP 2025-06-18** - Latest with OAuth 2.1 (Preferred)

The validator automatically detects and prefers Streamable HTTP when available.

## License

Apache License 2.0
