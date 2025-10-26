# MCP Validator

A standalone Go package for validating Model Context Protocol (MCP) servers.

## Overview

The MCP Validator provides comprehensive validation of MCP servers, supporting both **Streamable HTTP** (2025-03-26) and **SSE** (2024-11-05) transport protocols. It can be used independently of the mcp-operator or integrated into any Go application.

## Features

- ✅ **Transport Auto-Detection** - Automatically detects and prefers Streamable HTTP over SSE
- ✅ **Protocol Version Validation** - Supports MCP versions 2024-11-05 and 2025-03-26
- ✅ **Capability Discovery** - Identifies tools, resources, prompts, and logging capabilities
- ✅ **Comprehensive Testing** - Tests capability endpoints (tools/list, resources/list, prompts/list)
- ✅ **Error Reporting** - Detailed issue reporting with severity levels
- ✅ **Timeout Management** - Configurable timeouts for validation operations
- ✅ **Strict Mode** - Optional strict validation mode for production use

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

### Advanced Usage with Options

```go
// Validate with required capabilities
result, err := v.Validate(context.Background(), validator.ValidationOptions{
    RequiredCapabilities: []string{"tools", "resources"},
    Timeout:              30 * time.Second,
    StrictMode:           true,
    ConfiguredPath:       "/mcp", // Explicitly test Streamable HTTP
})

// Check for specific issues
if result.HasErrors() {
    for _, msg := range result.ErrorMessages() {
        log.Printf("Error: %s", msg)
    }
}

// Skip auto-detection by specifying transport explicitly
result, err := v.Validate(context.Background(), validator.ValidationOptions{
    Transport:      validator.TransportStreamableHTTP, // Skip detection
    ConfiguredPath: "/api/mcp",
})
```

### Using Standalone Clients

The package also provides standalone transport clients:

#### Streamable HTTP Client

```go
import "github.com/vitorbari/mcp-operator/pkg/validator"

// Create client
client := validator.NewStreamableHTTPClient("http://localhost:3001/mcp", 30*time.Second)
defer client.Close()

// Initialize connection
result, err := client.Initialize(context.Background())
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Server: %s v%s\n", result.ServerInfo.Name, result.ServerInfo.Version)

// List available tools
tools, err := client.ListTools(context.Background())
if err != nil {
    log.Fatal(err)
}

for _, tool := range tools.Tools {
    fmt.Printf("Tool: %s - %s\n", tool.Name, tool.Description)
}
```

#### SSE Client

```go
import "github.com/vitorbari/mcp-operator/pkg/validator"

// Create SSE client
client := validator.NewSSEClient("http://localhost:3001/sse", 30*time.Second)
defer client.Close()

// Connect to SSE endpoint
err := client.Connect(context.Background())
if err != nil {
    log.Fatal(err)
}

// Initialize
result, err := client.Initialize(context.Background())
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Connected via SSE to: %s\n", result.ServerInfo.Name)
```

## API Reference

### Types

#### Validator

```go
// Create a new validator
func NewValidator(baseURL string) *Validator
func NewValidatorWithTimeout(baseURL string, timeout time.Duration) *Validator

// Validate an MCP server
func (v *Validator) Validate(ctx context.Context, opts ValidationOptions) (*ValidationResult, error)
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
```

#### ValidationResult

```go
type ValidationResult struct {
    Success           bool              // Overall validation result
    ProtocolVersion   string            // Detected MCP protocol version
    Capabilities      []string          // Discovered capabilities
    ServerInfo        *ServerInfo       // Server implementation details
    Issues            []ValidationIssue // Validation problems found
    Duration          time.Duration     // Validation duration
    DetectedTransport TransportType     // Transport protocol used
    Endpoint          string            // Full URL that was validated
}

// Convenience methods
func (r *ValidationResult) IsCompliant() bool
func (r *ValidationResult) HasErrors() bool
func (r *ValidationResult) ErrorMessages() []string
```

#### TransportType

```go
const (
    TransportStreamableHTTP TransportType = "streamable-http" // MCP 2025-03-26
    TransportSSE            TransportType = "sse"              // MCP 2024-11-05
    TransportUnknown        TransportType = "unknown"
)
```

### Standalone Clients

#### StreamableHTTPClient

```go
func NewStreamableHTTPClient(endpoint string, timeout time.Duration) *StreamableHTTPClient

func (c *StreamableHTTPClient) Initialize(ctx context.Context) (*mcp.InitializeResult, error)
func (c *StreamableHTTPClient) ListTools(ctx context.Context) (*mcp.ListToolsResult, error)
func (c *StreamableHTTPClient) ListResources(ctx context.Context) (*mcp.ListResourcesResult, error)
func (c *StreamableHTTPClient) ListPrompts(ctx context.Context) (*mcp.ListPromptsResult, error)
func (c *StreamableHTTPClient) Ping(ctx context.Context) error
func (c *StreamableHTTPClient) Close() error
```

#### SSEClient

```go
func NewSSEClient(endpoint string, timeout time.Duration) *SSEClient

func (c *SSEClient) Connect(ctx context.Context) error
func (c *SSEClient) Initialize(ctx context.Context) (*mcp.InitializeResult, error)
func (c *SSEClient) Close() error
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

End-to-end tests against actual MCP server implementations using testcontainers. These tests use build tags and are skipped by default:

```bash
# Run integration tests with the 'integration' build tag
go test -tags integration -v -run TestRealWorld ./pkg/validator/
```

See [README_TESTING.md](README_TESTING.md) for comprehensive testing documentation.

## Examples

### Example 1: Simple Health Check

```go
func healthCheck(serverURL string) bool {
    v := validator.NewValidator(serverURL)
    result, err := v.Validate(context.Background(), validator.ValidationOptions{
        Timeout: 5 * time.Second,
    })
    return err == nil && result.Success
}
```

### Example 2: Continuous Validation

```go
func monitorServer(serverURL string) {
    v := validator.NewValidatorWithTimeout(serverURL, 10*time.Second)

    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        result, err := v.Validate(context.Background(), validator.ValidationOptions{
            StrictMode: true,
        })

        if err != nil || !result.Success {
            log.Printf("⚠️  Server validation failed: %v", err)
            if result != nil {
                for _, issue := range result.Issues {
                    log.Printf("  - %s: %s", issue.Code, issue.Message)
                }
            }
        } else {
            log.Printf("✅ Server healthy - %v capabilities", len(result.Capabilities))
        }
    }
}
```

### Example 3: Pre-deployment Validation

```go
func validateBeforeDeployment(config ServerConfig) error {
    v := validator.NewValidator(config.URL)

    result, err := v.Validate(context.Background(), validator.ValidationOptions{
        RequiredCapabilities: config.RequiredCapabilities,
        StrictMode:           true,
        Timeout:              30 * time.Second,
    })

    if err != nil {
        return fmt.Errorf("validation error: %w", err)
    }

    if !result.Success {
        return fmt.Errorf("server not compliant: %v", result.ErrorMessages())
    }

    // Log success details
    log.Printf("Server validated successfully:")
    log.Printf("  Protocol: %s", result.ProtocolVersion)
    log.Printf("  Transport: %s", result.DetectedTransport)
    log.Printf("  Server: %s v%s", result.ServerInfo.Name, result.ServerInfo.Version)
    log.Printf("  Capabilities: %v", result.Capabilities)

    return nil
}
```

## Known MCP Servers

See [KNOWN_MCP_SERVERS.md](KNOWN_MCP_SERVERS.md) for a list of tested MCP servers.

## Protocol Support

- **MCP 2024-11-05** - Legacy SSE-based transport ✅
- **MCP 2025-03-26** - Streamable HTTP transport ✅ (Preferred)

The validator automatically detects and prefers the newer Streamable HTTP transport when both are available.

## Contributing

See the main [mcp-operator repository](https://github.com/vitorbari/mcp-operator) for contribution guidelines.

## License

Apache License 2.0 - See [LICENSE](../../LICENSE) for details.

## Related Documentation

- [Testing Guide](README_TESTING.md) - Comprehensive testing documentation
- [Known MCP Servers](KNOWN_MCP_SERVERS.md) - Catalog of tested servers
- [MCP Specification](https://modelcontextprotocol.io/) - Official MCP protocol documentation
