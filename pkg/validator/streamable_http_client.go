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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/vitorbari/mcp-operator/internal/mcp"
)

// StreamableHTTPClient is a standalone client for MCP Streamable HTTP transport
// This client implements the MCP protocol over HTTP POST requests with JSON-RPC 2.0
type StreamableHTTPClient struct {
	endpoint   string
	httpClient *http.Client
	requestID  atomic.Int32
	timeout    time.Duration
	sessionID  string // MCP session ID from initialize response
}

// NewStreamableHTTPClient creates a new Streamable HTTP client for the given endpoint
func NewStreamableHTTPClient(endpoint string, timeout time.Duration) *StreamableHTTPClient {
	return &StreamableHTTPClient{
		endpoint: endpoint,
		timeout:  timeout,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Initialize sends an initialize request to the MCP server
func (c *StreamableHTTPClient) Initialize(ctx context.Context) (*mcp.InitializeResult, error) {
	params := mcp.InitializeParams{
		ProtocolVersion: mcp.DefaultProtocolVersion,
		Capabilities: mcp.ClientCapabilities{
			Roots: &mcp.RootsCapability{
				ListChanged: true,
			},
			Sampling: &mcp.SamplingCapability{},
		},
		ClientInfo: mcp.Implementation{
			Name:    "mcp-operator-validator",
			Version: "0.1.0",
		},
	}

	var result mcp.InitializeResult
	if err := c.callWithResponse(ctx, "initialize", params, &result, func(headers http.Header) {
		// Capture session ID from initialize response
		if sessionID := headers.Get("mcp-session-id"); sessionID != "" {
			c.sessionID = sessionID
		} else if sessionID := headers.Get("Mcp-Session-Id"); sessionID != "" {
			c.sessionID = sessionID
		}
	}); err != nil {
		return nil, fmt.Errorf("initialize failed: %w", err)
	}

	return &result, nil
}

// ListTools lists available tools from the MCP server
func (c *StreamableHTTPClient) ListTools(ctx context.Context) (*mcp.ListToolsResult, error) {
	var result mcp.ListToolsResult
	if err := c.call(ctx, "tools/list", nil, &result); err != nil {
		return nil, fmt.Errorf("list tools failed: %w", err)
	}

	return &result, nil
}

// ListResources lists available resources from the MCP server
func (c *StreamableHTTPClient) ListResources(ctx context.Context) (*mcp.ListResourcesResult, error) {
	var result mcp.ListResourcesResult
	if err := c.call(ctx, "resources/list", nil, &result); err != nil {
		return nil, fmt.Errorf("list resources failed: %w", err)
	}

	return &result, nil
}

// ListPrompts lists available prompts from the MCP server
func (c *StreamableHTTPClient) ListPrompts(ctx context.Context) (*mcp.ListPromptsResult, error) {
	var result mcp.ListPromptsResult
	if err := c.call(ctx, "prompts/list", nil, &result); err != nil {
		return nil, fmt.Errorf("list prompts failed: %w", err)
	}

	return &result, nil
}

// Ping sends an initialize request to check if the server is responsive
// This is a convenience method for quick connectivity checks
func (c *StreamableHTTPClient) Ping(ctx context.Context) error {
	_, err := c.Initialize(ctx)
	return err
}

// Close cleans up the client resources
func (c *StreamableHTTPClient) Close() error {
	// HTTP client doesn't need explicit cleanup
	return nil
}

// call sends a JSON-RPC 2.0 request and decodes the response
func (c *StreamableHTTPClient) call(ctx context.Context, method string, params any, result any) error {
	return c.callWithResponse(ctx, method, params, result, nil)
}

// callWithResponse sends a JSON-RPC 2.0 request with an optional response header callback
func (c *StreamableHTTPClient) callWithResponse(
	ctx context.Context,
	method string,
	params any,
	result any,
	headerCallback func(http.Header),
) error {
	// Generate request ID
	requestID := int(c.requestID.Add(1))

	// Build JSON-RPC request
	request := mcp.JSONRPCRequest{
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
	// MCP Streamable HTTP requires accepting both JSON and SSE formats
	httpReq.Header.Set("Accept", "application/json, text/event-stream")

	// Include session ID if we have one
	if c.sessionID != "" {
		httpReq.Header.Set("mcp-session-id", c.sessionID)
	}

	// Send HTTP request
	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() {
		_ = httpResp.Body.Close()
	}()

	// Call header callback if provided
	if headerCallback != nil {
		headerCallback(httpResp.Header)
	}

	// Check HTTP status code
	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("HTTP error %d: %s", httpResp.StatusCode, string(body))
	}

	// Check content type to determine how to parse the response
	contentType := httpResp.Header.Get("Content-Type")
	var responseBody []byte

	if contentType == "text/event-stream" || contentType == "text/event-stream; charset=utf-8" {
		// Parse SSE format response
		responseBody, err = c.parseSSEResponse(httpResp.Body)
		if err != nil {
			return fmt.Errorf("failed to parse SSE response: %w", err)
		}
	} else {
		// Parse regular JSON response
		responseBody, err = io.ReadAll(httpResp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}
	}

	// Parse JSON-RPC response
	var rpcResponse mcp.JSONRPCResponse
	if err := json.Unmarshal(responseBody, &rpcResponse); err != nil {
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

// parseSSEResponse extracts the JSON data from an SSE formatted response
func (c *StreamableHTTPClient) parseSSEResponse(body io.Reader) ([]byte, error) {
	scanner := bufio.NewScanner(body)
	var dataBuilder strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		// SSE format has lines like:
		// event: message
		// id: ...
		// data: {...}
		//
		// We only care about the data lines
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			dataBuilder.WriteString(data)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading SSE stream: %w", err)
	}

	if dataBuilder.Len() == 0 {
		return nil, fmt.Errorf("no data found in SSE response")
	}

	return []byte(dataBuilder.String()), nil
}
