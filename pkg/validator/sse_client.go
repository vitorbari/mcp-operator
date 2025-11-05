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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vitorbari/mcp-operator/internal/mcp"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// SSEClient handles SSE-based MCP communication
type SSEClient struct {
	httpClient  *http.Client
	sseEndpoint string
	messagesURL string
	sseReader   io.ReadCloser
	requestID   int64
}

// NewSSEClient creates a new SSE client
func NewSSEClient(sseEndpoint string, timeout time.Duration) *SSEClient {
	return &SSEClient{
		httpClient: &http.Client{
			// Don't set a timeout on the HTTP client for SSE connections
			// SSE streams need to stay open, and we handle timeouts with context
			Timeout: 0,
		},
		sseEndpoint: sseEndpoint,
		requestID:   1,
	}
}

// Connect establishes SSE connection and discovers the messages endpoint
func (c *SSEClient) Connect(ctx context.Context) error {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Connecting to SSE endpoint", "endpoint", c.sseEndpoint)

	req, err := http.NewRequestWithContext(ctx, "GET", c.sseEndpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create SSE request: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.Error(err, "Failed to connect to SSE endpoint")
		return fmt.Errorf("failed to connect to SSE endpoint: %w", err)
	}

	logger.V(1).Info("SSE connection established",
		"status", resp.StatusCode,
		"content-type", resp.Header.Get("Content-Type"))

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return fmt.Errorf("SSE endpoint returned status %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		_ = resp.Body.Close()
		return fmt.Errorf("SSE endpoint returned wrong content type: %s", contentType)
	}

	c.sseReader = resp.Body

	// Read the endpoint event to get messages URL
	logger.V(1).Info("Reading SSE endpoint event")
	messagesPath, err := c.readEndpointEvent(ctx)
	if err != nil {
		_ = resp.Body.Close()
		logger.Error(err, "Failed to read endpoint event")
		return fmt.Errorf("failed to read endpoint event: %w", err)
	}

	// Build full messages URL from the SSE endpoint base URL and the messages path
	// Extract base URL from sseEndpoint (e.g., "http://host:port/sse" -> "http://host:port")
	baseURL := c.sseEndpoint
	if idx := strings.LastIndex(baseURL, "/"); idx > 7 { // 7 = len("http://")
		baseURL = baseURL[:idx]
	}
	c.messagesURL = baseURL + messagesPath
	logger.V(1).Info("SSE messages URL constructed", "messagesURL", c.messagesURL)
	return nil
}

// readEndpointEvent reads SSE stream until it finds the endpoint event
func (c *SSEClient) readEndpointEvent(ctx context.Context) (string, error) {
	scanner := bufio.NewScanner(c.sseReader)
	timeout := time.After(1 * time.Second)

	var currentEvent string
	var currentData strings.Builder

	eventChan := make(chan string, 1)
	errChan := make(chan error, 1)

	go func() {
		logger := log.FromContext(ctx)
		for scanner.Scan() {
			line := scanner.Text()

			if strings.HasPrefix(line, "event:") {
				currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			} else if strings.HasPrefix(line, "data:") {
				data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				currentData.WriteString(data)
			} else if line == "" {
				// Empty line marks end of event
				if currentEvent == "endpoint" {
					// The endpoint data is just a plain string (the URI path), not JSON
					uri := strings.TrimSpace(currentData.String())
					logger.V(1).Info("Found SSE endpoint URI", "uri", uri)
					if uri != "" {
						eventChan <- uri
						return
					}
				}
				// Reset for next event
				currentEvent = ""
				currentData.Reset()
			}
		}
		if err := scanner.Err(); err != nil {
			errChan <- fmt.Errorf("scanner error: %w", err)
		} else {
			errChan <- fmt.Errorf("SSE stream ended without endpoint event")
		}
	}()

	select {
	case uri := <-eventChan:
		return uri, nil
	case err := <-errChan:
		return "", err
	case <-timeout:
		return "", fmt.Errorf("timeout waiting for endpoint event")
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Initialize sends an initialize request via SSE transport
func (c *SSEClient) Initialize(ctx context.Context) (*mcp.InitializeResult, error) {
	logger := log.FromContext(ctx)

	if c.messagesURL == "" {
		return nil, fmt.Errorf("not connected: call Connect() first")
	}

	logger.V(1).Info("Sending SSE initialize request", "messagesURL", c.messagesURL)

	// Build initialize request
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      c.requestID,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": ProtocolVersion20241105, // SSE uses legacy protocol version
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]string{
				"name":    "mcp-operator-validator",
				"version": "1.0.0",
			},
		},
	}
	c.requestID++

	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// POST to messages URL
	req, err := http.NewRequestWithContext(ctx, "POST", c.messagesURL, strings.NewReader(string(requestBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.Error(err, "SSE initialize POST failed")
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	logger.V(1).Info("SSE initialize POST completed", "status", resp.StatusCode)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Read response from SSE stream
	logger.V(1).Info("Reading initialize response from SSE stream")
	response, err := c.readInitializeResponse(ctx)
	if err != nil {
		logger.Error(err, "Failed to read SSE initialize response")
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	logger.Info(
		"SSE initialize successful",
		"serverName", response.ServerInfo.Name,
		"protocolVersion", response.ProtocolVersion,
	)
	return response, nil
}

// readInitializeResponse reads the SSE stream to find the initialize response
func (c *SSEClient) readInitializeResponse(ctx context.Context) (*mcp.InitializeResult, error) {
	scanner := bufio.NewScanner(c.sseReader)
	timeout := time.After(2 * time.Second)

	var currentData strings.Builder

	responseChan := make(chan *mcp.InitializeResult, 1)
	errChan := make(chan error, 1)

	go func() {
		for scanner.Scan() {
			line := scanner.Text()

			if strings.HasPrefix(line, "data:") {
				data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				currentData.WriteString(data)
			} else if line == "" {
				// Empty line marks end of event
				if currentData.Len() > 0 {
					// Try to parse as JSON-RPC response
					var jsonRPCResp struct {
						JSONRPC string                  `json:"jsonrpc"`
						ID      int64                   `json:"id"`
						Result  *mcp.InitializeResult   `json:"result,omitempty"`
						Error   *map[string]interface{} `json:"error,omitempty"`
					}

					dataStr := currentData.String()
					if err := json.Unmarshal([]byte(dataStr), &jsonRPCResp); err != nil {
						// Not a JSON-RPC message, skip
						currentData.Reset()
						continue
					}

					// Check if this is the initialize response
					if jsonRPCResp.Result != nil {
						responseChan <- jsonRPCResp.Result
						return
					} else if jsonRPCResp.Error != nil {
						errChan <- fmt.Errorf("server returned error: %v", jsonRPCResp.Error)
						return
					}

					currentData.Reset()
				}
			}
		}
		if err := scanner.Err(); err != nil {
			errChan <- fmt.Errorf("scanner error: %w", err)
		} else {
			errChan <- fmt.Errorf("SSE stream ended without response")
		}
	}()

	select {
	case response := <-responseChan:
		return response, nil
	case err := <-errChan:
		return nil, err
	case <-timeout:
		return nil, fmt.Errorf("timeout waiting for initialize response")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close closes the SSE connection
func (c *SSEClient) Close() error {
	if c.sseReader != nil {
		return c.sseReader.Close()
	}
	return nil
}
