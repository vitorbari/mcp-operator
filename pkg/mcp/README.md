# MCP Client Library

A Go client library for the [Model Context Protocol (MCP)](https://spec.modelcontextprotocol.io/).

## Overview

The Model Context Protocol is a standardized protocol that enables AI models to securely interact with external tools, resources, and data sources. This package provides a Go implementation of an MCP client that communicates with MCP servers using JSON-RPC 2.0 over HTTP.

## Features

- **Protocol Initialization**: Capability negotiation and version detection
- **Tool Discovery & Invocation**: List and execute server-provided tools
- **Resource Access**: List and read server resources (files, databases, APIs)
- **Prompt Management**: Discover and use prompt templates
- **Automatic Request ID Management**: Built-in request tracking
- **Configurable Timeouts**: Customize HTTP request timeouts
- **Full JSON-RPC 2.0 Support**: Standards-compliant implementation

## Installation

```bash
go get github.com/vitorbari/mcp-operator/pkg/mcp
```

## Quick Start

### Basic Connection

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/vitorbari/mcp-operator/pkg/mcp"
)

func main() {
    // Create a client
    client := mcp.NewClient("http://localhost:8080/mcp")

    // Initialize connection and negotiate capabilities
    result, err := client.Initialize(context.Background())
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Connected to: %s v%s\n",
        result.ServerInfo.Name,
        result.ServerInfo.Version)
    fmt.Printf("Protocol version: %s\n", result.ProtocolVersion)
}
```

### Custom Timeout

The client supports functional options for configuration:

```go
// Custom timeout using functional option
client := mcp.NewClient(
    "http://localhost:8080/mcp",
    mcp.WithTimeout(60 * time.Second),
)
```

## Working with Tools

Tools are executable functions provided by the MCP server.

### List Available Tools

```go
tools, err := client.ListTools(ctx)
if err != nil {
    log.Fatal(err)
}

for _, tool := range tools.Tools {
    fmt.Printf("Tool: %s\n", tool.Name)
    fmt.Printf("  Description: %s\n", tool.Description)
    fmt.Printf("  Schema: %+v\n", tool.InputSchema)
}
```

### Call a Tool

```go
params := map[string]interface{}{
    "query": "What is the weather in San Francisco?",
    "units": "celsius",
}

result, err := client.CallTool(ctx, "get_weather", params)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Result: %+v\n", result)
```

## Working with Resources

Resources are data sources like files, database records, or API endpoints.

### List Available Resources

```go
resources, err := client.ListResources(ctx)
if err != nil {
    log.Fatal(err)
}

for _, resource := range resources.Resources {
    fmt.Printf("Resource: %s\n", resource.Name)
    fmt.Printf("  URI: %s\n", resource.URI)
    fmt.Printf("  Type: %s\n", resource.MimeType)
}
```

### Read a Resource

```go
content, err := client.ReadResource(ctx, "file:///data/config.json")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Content: %s\n", content)
```

## Working with Prompts

Prompts are pre-configured prompt templates that can accept arguments.

### List Available Prompts

```go
prompts, err := client.ListPrompts(ctx)
if err != nil {
    log.Fatal(err)
}

for _, prompt := range prompts.Prompts {
    fmt.Printf("Prompt: %s\n", prompt.Name)
    fmt.Printf("  Description: %s\n", prompt.Description)

    for _, arg := range prompt.Arguments {
        required := ""
        if arg.Required {
            required = " (required)"
        }
        fmt.Printf("  - %s%s: %s\n", arg.Name, required, arg.Description)
    }
}
```

### Get a Prompt

```go
args := map[string]string{
    "topic": "Kubernetes operators",
    "audience": "developers",
}

prompt, err := client.GetPrompt(ctx, "explain_concept", args)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Generated prompt: %s\n", prompt)
```

## Use Cases

This library is designed for:

### 1. **Protocol Validation**
Build validators to ensure MCP servers are compliant with the specification:

```go
validator := NewMCPValidator(serverURL)
if err := validator.ValidateCompliance(ctx); err != nil {
    log.Printf("Server is not compliant: %v", err)
}
```

### 2. **Monitoring & Observability**
Monitor MCP server health and capabilities:

```go
func monitorServer(client *mcp.Client) error {
    result, err := client.Initialize(ctx)
    if err != nil {
        return fmt.Errorf("server unreachable: %w", err)
    }

    // Check capabilities
    hasTools := result.Capabilities.Tools != nil
    hasResources := result.Capabilities.Resources != nil

    log.Printf("Server healthy - Tools: %v, Resources: %v",
        hasTools, hasResources)
    return nil
}
```

### 3. **Testing Frameworks**
Create automated tests for MCP server implementations:

```go
func TestServerToolExecution(t *testing.T) {
    client := mcp.NewClient(testServerURL)

    // Test initialization
    _, err := client.Initialize(context.Background())
    require.NoError(t, err)

    // Test tool listing
    tools, err := client.ListTools(context.Background())
    require.NoError(t, err)
    assert.Greater(t, len(tools.Tools), 0)

    // Test tool execution
    result, err := client.CallTool(ctx, "test_tool", nil)
    require.NoError(t, err)
    assert.NotNil(t, result)
}
```

### 4. **Debugging Utilities**
Build tools to inspect and debug MCP servers:

```go
func inspectServer(serverURL string) {
    client := mcp.NewClient(serverURL)

    result, _ := client.Initialize(ctx)
    fmt.Printf("Server: %s v%s\n",
        result.ServerInfo.Name,
        result.ServerInfo.Version)

    tools, _ := client.ListTools(ctx)
    fmt.Printf("Available tools: %d\n", len(tools.Tools))

    resources, _ := client.ListResources(ctx)
    fmt.Printf("Available resources: %d\n", len(resources.Resources))
}
```

### 5. **Custom Applications**
Integrate MCP capabilities into your applications:

```go
type AIAssistant struct {
    mcpClient *mcp.Client
}

func (a *AIAssistant) ExecuteWithTools(query string) (string, error) {
    // List available tools
    tools, err := a.mcpClient.ListTools(ctx)
    if err != nil {
        return "", err
    }

    // Determine which tool to use based on query
    toolName := a.selectTool(query, tools.Tools)

    // Execute the tool
    result, err := a.mcpClient.CallTool(ctx, toolName,
        map[string]interface{}{"query": query})

    return a.formatResponse(result), err
}
```

## Protocol Support

- **MCP Version**: 2024-11-05
- **Transport**: HTTP with JSON-RPC 2.0
- **Authentication**: Currently supports unauthenticated connections

## Error Handling

The client returns standard Go errors with context:

```go
result, err := client.CallTool(ctx, "nonexistent_tool", nil)
if err != nil {
    // Check for specific error types
    if rpcErr, ok := err.(*mcp.RPCError); ok {
        fmt.Printf("RPC Error %d: %s\n", rpcErr.Code, rpcErr.Message)
    } else {
        fmt.Printf("Transport error: %v\n", err)
    }
}
```

## Thread Safety

The `Client` is safe for concurrent use. Each request automatically manages its own request ID using atomic operations.

```go
client := mcp.NewClient(serverURL)

// Safe to call from multiple goroutines
go client.ListTools(ctx)
go client.ListResources(ctx)
go client.ListPrompts(ctx)
```

## Contributing

This package is part of the [MCP Operator](https://github.com/vitorbari/mcp-operator) project. Contributions are welcome!

## Resources

- [MCP Specification](https://spec.modelcontextprotocol.io/)
- [JSON-RPC 2.0 Specification](https://www.jsonrpc.org/specification)
- [MCP Operator Documentation](https://github.com/vitorbari/mcp-operator)

## License

Licensed under the Apache License, Version 2.0. See LICENSE file for details.
