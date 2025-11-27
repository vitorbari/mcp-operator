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

// Package mcp provides a Go client library for the Model Context Protocol (MCP).
//
// The Model Context Protocol is a standardized protocol for AI models to interact
// with external tools, resources, and data sources. This package implements a client
// that can communicate with MCP servers using JSON-RPC 2.0 over HTTP.
//
// # Use Cases
//
// This library can be used for:
//
//   - Building MCP protocol validators and compliance checkers
//   - Creating monitoring tools for MCP server deployments
//   - Implementing MCP server testing frameworks
//   - Developing debugging utilities for MCP implementations
//   - Integrating MCP capabilities into custom applications
//
// # Features
//
//   - Protocol initialization and capability negotiation
//   - Tool discovery and invocation
//   - Resource listing and reading
//   - Prompt discovery and execution
//   - Automatic request ID management
//   - Configurable timeouts
//   - Bearer token authentication support
//   - Custom header management
//   - Custom client identification
//   - Full JSON-RPC 2.0 support
//
// # Basic Usage
//
// Creating a client and initializing a connection:
//
//	client := mcp.NewClient("http://localhost:8080/mcp")
//	result, err := client.Initialize(context.Background())
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Connected to: %s v%s\n", result.ServerInfo.Name, result.ServerInfo.Version)
//
// Creating a client with custom timeout:
//
//	client := mcp.NewClient("http://localhost:8080/mcp",
//	    mcp.WithTimeout(60*time.Second))
//
// Creating a client with Bearer token authentication:
//
//	client := mcp.NewClient("http://localhost:8080/mcp",
//	    mcp.WithBearerToken("your-token-here"))
//
// Creating a client with custom identification:
//
//	client := mcp.NewClient("http://localhost:8080/mcp",
//	    mcp.WithClientInfo("my-app", "1.2.3"))
//
// Listing available tools:
//
//	tools, err := client.ListTools(context.Background())
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for _, tool := range tools.Tools {
//	    fmt.Printf("Tool: %s - %s\n", tool.Name, tool.Description)
//	}
//
// Calling a tool:
//
//	params := map[string]interface{}{
//	    "query": "What is MCP?",
//	}
//	result, err := client.CallTool(context.Background(), "search", params)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Result: %+v\n", result)
//
// # Protocol Support
//
// This client supports MCP protocol version 2024-11-05 and is compatible with
// servers implementing the standard MCP specification.
//
// For more information about the Model Context Protocol, see:
// https://spec.modelcontextprotocol.io/
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

const (
	// DefaultProtocolVersion is the MCP protocol version to use
	DefaultProtocolVersion = "2024-11-05"

	// DefaultTimeout is the default timeout for HTTP requests
	DefaultTimeout = 30 * time.Second

	// HeaderSessionID is the HTTP header name for MCP session management
	// Used in Streamable HTTP transport to maintain session state
	HeaderSessionID = "Mcp-Session-Id"

	// MCP JSON-RPC method names
	MethodInitialize              = "initialize"
	MethodNotificationInitialized = "notifications/initialized"
	MethodToolsList               = "tools/list"
	MethodResourcesList           = "resources/list"
	MethodPromptsList             = "prompts/list"
)

// Client is an MCP protocol client
type Client struct {
	endpoint      string
	httpClient    *http.Client
	requestID     atomic.Int32
	sessionID     string            // MCP session ID for Streamable HTTP transport
	customHeaders map[string]string // Custom headers for authentication and other purposes
	clientInfo    *Implementation   // Client identification sent during initialization
}

// Option is a functional option for configuring the Client
type Option func(*Client)

// WithTimeout sets a custom timeout for HTTP requests
//
// Example:
//
//	client := mcp.NewClient("http://localhost:8080/mcp",
//	    mcp.WithTimeout(60*time.Second))
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

// WithBearerToken sets a Bearer token for authentication
//
// Example:
//
//	client := mcp.NewClient("http://localhost:8080/mcp",
//	    mcp.WithBearerToken("your-token-here"))
func WithBearerToken(token string) Option {
	return func(c *Client) {
		c.customHeaders["Authorization"] = "Bearer " + token
	}
}

// WithHeaders sets custom HTTP headers for all requests
//
// Example:
//
//	client := mcp.NewClient("http://localhost:8080/mcp",
//	    mcp.WithHeaders(map[string]string{
//	        "X-API-Key": "key123",
//	        "X-Custom-Header": "value",
//	    }))
func WithHeaders(headers map[string]string) Option {
	return func(c *Client) {
		for k, v := range headers {
			c.customHeaders[k] = v
		}
	}
}

// WithClientInfo sets custom client identification for the initialize handshake
//
// By default, the client identifies as "go-mcp-client" version "1.0.0".
// Use this option to customize the client name and version sent to the server.
//
// Example:
//
//	client := mcp.NewClient("http://localhost:8080/mcp",
//	    mcp.WithClientInfo("my-app", "1.2.3"))
func WithClientInfo(name, version string) Option {
	return func(c *Client) {
		c.clientInfo = &Implementation{
			Name:    name,
			Version: version,
		}
	}
}

// NewClient creates a new MCP client for the given endpoint.
//
// By default, the client uses a 30-second timeout. This can be customized
// using the WithTimeout option:
//
//	// Default timeout (30s)
//	client := mcp.NewClient("http://localhost:8080/mcp")
//
//	// Custom timeout
//	client := mcp.NewClient("http://localhost:8080/mcp",
//	    mcp.WithTimeout(60*time.Second))
func NewClient(endpoint string, opts ...Option) *Client {
	c := &Client{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		customHeaders: make(map[string]string),
	}

	// Apply functional options
	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Initialize sends an initialize request to the MCP server and sends the
// initialized notification to complete the handshake.
//
// The MCP protocol requires a three-step initialization:
// 1. Client sends initialize request
// 2. Server responds with capabilities
// 3. Client sends initialized notification
//
// This method handles all three steps automatically.
func (c *Client) Initialize(ctx context.Context) (*InitializeResult, error) {
	// Use custom client info if provided, otherwise use generic default
	clientInfo := Implementation{
		Name:    "go-mcp-client",
		Version: "1.0.0",
	}
	if c.clientInfo != nil {
		clientInfo = *c.clientInfo
	}

	params := InitializeParams{
		ProtocolVersion: DefaultProtocolVersion,
		Capabilities: ClientCapabilities{
			Roots: &RootsCapability{
				ListChanged: true,
			},
			Sampling: &SamplingCapability{},
		},
		ClientInfo: clientInfo,
	}

	var result InitializeResult
	if err := c.call(ctx, MethodInitialize, params, &result); err != nil {
		return nil, fmt.Errorf("initialize failed: %w", err)
	}

	// Send initialized notification to complete the handshake
	// This is a notification (no response expected)
	if err := c.notify(ctx, MethodNotificationInitialized); err != nil {
		return nil, fmt.Errorf("initialized notification failed: %w", err)
	}

	return &result, nil
}

// ListTools lists available tools from the MCP server
func (c *Client) ListTools(ctx context.Context) (*ListToolsResult, error) {
	var result ListToolsResult
	if err := c.call(ctx, MethodToolsList, nil, &result); err != nil {
		return nil, fmt.Errorf("list tools failed: %w", err)
	}

	return &result, nil
}

// ListResources lists available resources from the MCP server
func (c *Client) ListResources(ctx context.Context) (*ListResourcesResult, error) {
	var result ListResourcesResult
	if err := c.call(ctx, MethodResourcesList, nil, &result); err != nil {
		return nil, fmt.Errorf("list resources failed: %w", err)
	}

	return &result, nil
}

// ListPrompts lists available prompts from the MCP server
func (c *Client) ListPrompts(ctx context.Context) (*ListPromptsResult, error) {
	var result ListPromptsResult
	if err := c.call(ctx, MethodPromptsList, nil, &result); err != nil {
		return nil, fmt.Errorf("list prompts failed: %w", err)
	}

	return &result, nil
}

// notify sends a JSON-RPC 2.0 notification (no response expected)
func (c *Client) notify(ctx context.Context, method string) error {
	// Build JSON-RPC notification (no ID field)
	notification := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}

	// Marshal notification to JSON
	notificationBody, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(notificationBody))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")

	// Apply custom headers (e.g., authentication)
	for key, value := range c.customHeaders {
		httpReq.Header.Set(key, value)
	}

	// Include session ID if we have one
	if c.sessionID != "" {
		httpReq.Header.Set(HeaderSessionID, c.sessionID)
	}

	// Send HTTP request
	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() {
		_ = httpResp.Body.Close()
	}()

	// For notifications, we accept 200 OK, 202 Accepted, or 204 No Content
	if httpResp.StatusCode != http.StatusOK &&
		httpResp.StatusCode != http.StatusAccepted &&
		httpResp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("HTTP error %d: %s", httpResp.StatusCode, string(body))
	}

	// Drain and close the response body
	_, _ = io.Copy(io.Discard, httpResp.Body)

	return nil
}

// call sends a JSON-RPC 2.0 request and decodes the response
func (c *Client) call(ctx context.Context, method string, params any, result any) error {
	// Generate request ID
	requestID := int(c.requestID.Add(1))

	// Build JSON-RPC request
	request := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  method,
		Params:  params,
	}

	// Marshal request to JSON
	requestBody, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	// Accept both JSON and SSE responses (required by MCP Streamable HTTP transport)
	httpReq.Header.Set("Accept", "application/json, text/event-stream")

	// Apply custom headers (e.g., authentication)
	for key, value := range c.customHeaders {
		httpReq.Header.Set(key, value)
	}

	// Include session ID if we have one (for Streamable HTTP session management)
	if c.sessionID != "" {
		httpReq.Header.Set(HeaderSessionID, c.sessionID)
	}

	// Send HTTP request
	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() {
		_ = httpResp.Body.Close()
	}()

	// Capture session ID from response if present (Streamable HTTP session management)
	if sessionID := httpResp.Header.Get(HeaderSessionID); sessionID != "" {
		c.sessionID = sessionID
	}

	// Check HTTP status code
	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("HTTP error %d: %s", httpResp.StatusCode, string(body))
	}

	// Read response body
	responseBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Check if response is SSE format (Streamable HTTP transport)
	contentType := httpResp.Header.Get("Content-Type")
	var jsonData []byte

	if contentType == "text/event-stream" {
		// Parse SSE format: extract JSON from "data: " line
		jsonData, err = parseSSEResponse(responseBody)
		if err != nil {
			return fmt.Errorf("failed to parse SSE response: %w", err)
		}
	} else {
		// Plain JSON response
		jsonData = responseBody
	}

	// Parse JSON-RPC response
	var rpcResponse JSONRPCResponse
	if err := json.Unmarshal(jsonData, &rpcResponse); err != nil {
		return fmt.Errorf("failed to parse JSON-RPC response: %w", err)
	}

	// Check for JSON-RPC error
	if rpcResponse.Error != nil {
		return fmt.Errorf("JSON-RPC error %d: %s", rpcResponse.Error.Code, rpcResponse.Error.Message)
	}

	// Check response ID matches request ID
	if rpcResponse.ID != requestID {
		return fmt.Errorf("response ID mismatch: expected %d, got %d", requestID, rpcResponse.ID)
	}

	// Decode result into the provided interface
	if result != nil {
		resultBytes, err := json.Marshal(rpcResponse.Result)
		if err != nil {
			return fmt.Errorf("failed to marshal result: %w", err)
		}

		if err := json.Unmarshal(resultBytes, result); err != nil {
			return fmt.Errorf("failed to decode result: %w", err)
		}
	}

	return nil
}

// Ping sends an initialize request to check if the server is responsive
// This is a convenience method for quick connectivity checks
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.Initialize(ctx)
	return err
}

// parseSSEResponse extracts JSON-RPC data from Server-Sent Events format
// SSE format example:
//
//	event: message
//	id: abc123
//	data: {"jsonrpc":"2.0","id":1,"result":{...}}
func parseSSEResponse(body []byte) ([]byte, error) {
	lines := strings.Split(string(body), "\n")

	for _, line := range lines {
		// Look for data line containing JSON-RPC response
		if jsonData, found := strings.CutPrefix(line, "data: "); found {
			return []byte(jsonData), nil
		}
	}

	return nil, fmt.Errorf("no data line found in SSE response")
}
