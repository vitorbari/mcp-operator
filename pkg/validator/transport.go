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
	"io"
	"net/http"
	"strings"
	"time"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// TransportType represents the detected MCP transport protocol
type TransportType string

const (
	// TransportStreamableHTTP is the new standard (2025-03-26)
	TransportStreamableHTTP TransportType = "streamable-http"

	// TransportSSE is the legacy SSE-based transport (2024-11-05)
	TransportSSE TransportType = "sse"

	// TransportUnknown indicates transport could not be detected
	TransportUnknown TransportType = "unknown"

	// DefaultMCPPath is the default path for MCP Streamable HTTP endpoints
	DefaultMCPPath = "/mcp"

	// DefaultSSEPath is the default path for SSE endpoints
	DefaultSSEPath = "/sse"
)

// TransportDetector detects which MCP transport protocol a server supports
type TransportDetector struct {
	httpClient *http.Client
}

// NewTransportDetector creates a new transport detector
func NewTransportDetector(timeout time.Duration) *TransportDetector {
	// Use a much shorter timeout for detection probes
	// Detection should be fast - if a server doesn't respond quickly, try next transport
	detectionTimeout := 2 * time.Second
	if timeout < detectionTimeout {
		detectionTimeout = timeout
	}

	return &TransportDetector{
		httpClient: &http.Client{
			Timeout: detectionTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse // Don't follow redirects
			},
		},
	}
}

// DetectTransport attempts to detect which transport protocol the server supports
// It tries Streamable HTTP first (preferred), then falls back to SSE
func (d *TransportDetector) DetectTransport(
	ctx context.Context,
	baseURL, configuredPath string,
) (TransportType, string, error) {
	log := logf.FromContext(ctx)

	log.Info("Starting transport detection",
		"baseURL", baseURL,
		"configuredPath", configuredPath)

	// Determine paths to try
	streamablePath := DefaultMCPPath
	ssePath := DefaultSSEPath

	// If a path is configured, use it for detection
	if configuredPath != "" {
		log.V(1).Info("Using configured path for detection",
			"configuredPath", configuredPath)
		// Try the configured path for both transports
		streamablePath = configuredPath
		ssePath = configuredPath
	}

	// Build full URLs
	streamableURL := strings.TrimRight(baseURL, "/") + streamablePath
	sseURL := strings.TrimRight(baseURL, "/") + ssePath

	log.V(1).Info("Attempting transport detection",
		"streamableHTTP_url", streamableURL,
		"sse_url", sseURL)

	// Try Streamable HTTP first (newer standard, preferred)
	log.V(1).Info("Trying Streamable HTTP transport", "url", streamableURL)
	if d.tryStreamableHTTP(ctx, streamableURL) {
		log.Info("Detected Streamable HTTP transport", "url", streamableURL)
		return TransportStreamableHTTP, streamableURL, nil
	}
	log.Info("Streamable HTTP detection failed, trying SSE", "url", streamableURL)

	// Fall back to SSE (legacy but widely used)
	log.V(1).Info("Trying SSE transport", "url", sseURL)
	if d.trySSE(ctx, sseURL) {
		log.Info("Detected SSE transport", "url", sseURL)
		return TransportSSE, sseURL, nil
	}
	log.Info("SSE detection failed", "url", sseURL)

	// Neither transport worked
	log.Error(nil, "Transport detection failed",
		"streamableURL", streamableURL,
		"sseURL", sseURL)
	return TransportUnknown, "", fmt.Errorf(
		"could not detect transport: tried Streamable HTTP at %s and SSE at %s",
		streamableURL, sseURL,
	)
}

// tryStreamableHTTP checks if the endpoint supports Streamable HTTP transport
func (d *TransportDetector) tryStreamableHTTP(ctx context.Context, endpoint string) bool {
	log := logf.FromContext(ctx)

	// Create a minimal JSON-RPC 2.0 initialize request for detection
	// This is what a real MCP client would send
	detectRequest := `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "initialize",
		"params": {
			"protocolVersion": "2024-11-05",
			"capabilities": {},
			"clientInfo": {
				"name": "mcp-operator-validator",
				"version": "0.1.0"
			}
		}
	}`

	// Create a test POST request
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(detectRequest))
	if err != nil {
		log.V(1).Info("Failed to create Streamable HTTP request",
			"endpoint", endpoint,
			"error", err)
		return false
	}

	req.Header.Set("Content-Type", "application/json")
	// MCP Streamable HTTP requires accepting both JSON and SSE formats
	req.Header.Set("Accept", "application/json, text/event-stream")

	log.V(1).Info("Sending Streamable HTTP detection request",
		"endpoint", endpoint,
		"method", "POST")

	// Send request
	resp, err := d.httpClient.Do(req)
	if err != nil {
		log.Info("Streamable HTTP request failed",
			"endpoint", endpoint,
			"error", err)
		return false
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	log.V(1).Info("Received Streamable HTTP response",
		"endpoint", endpoint,
		"status", resp.StatusCode,
		"contentType", resp.Header.Get("Content-Type"))

	// Check if endpoint accepts POST requests
	// Should NOT return 404 (Not Found) or 405 (Method Not Allowed)
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		log.Info("Streamable HTTP endpoint not found or method not allowed",
			"endpoint", endpoint,
			"status", resp.StatusCode)
		return false
	}

	// For Streamable HTTP, we expect a 200 OK with JSON response
	// A 400 Bad Request means the server doesn't understand the JSON-RPC format
	if resp.StatusCode == http.StatusBadRequest {
		log.Info("Streamable HTTP endpoint returned bad request",
			"endpoint", endpoint,
			"status", resp.StatusCode)
		return false
	}

	// Check content type - should be JSON
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		log.V(1).Info("Streamable HTTP detected via JSON content-type",
			"endpoint", endpoint,
			"contentType", contentType)
		return true
	}

	// If we get 200 OK, it's likely Streamable HTTP even without proper content-type
	if resp.StatusCode == http.StatusOK {
		log.Info("Streamable HTTP detected via 200 OK response",
			"endpoint", endpoint,
			"contentType", contentType)
		return true
	}

	log.Info("Streamable HTTP detection inconclusive",
		"endpoint", endpoint,
		"status", resp.StatusCode,
		"contentType", contentType)
	return false
}

// trySSE checks if the endpoint supports SSE transport
func (d *TransportDetector) trySSE(ctx context.Context, endpoint string) bool {
	log := logf.FromContext(ctx)

	// Create a GET request
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		log.V(1).Info("Failed to create SSE request",
			"endpoint", endpoint,
			"error", err)
		return false
	}

	req.Header.Set("Accept", "text/event-stream")

	log.V(1).Info("Sending SSE detection request",
		"endpoint", endpoint,
		"method", "GET")

	// Send request
	resp, err := d.httpClient.Do(req)
	if err != nil {
		log.Info("SSE request failed",
			"endpoint", endpoint,
			"error", err)
		return false
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	log.V(1).Info("Received SSE response",
		"endpoint", endpoint,
		"status", resp.StatusCode,
		"contentType", resp.Header.Get("Content-Type"))

	// Check if endpoint returns SSE
	// Check content-type first - should be text/event-stream
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		log.Info("SSE endpoint did not return event-stream content-type",
			"endpoint", endpoint,
			"contentType", contentType)
		return false
	}

	// Accept both 200 OK and 401 Unauthorized (auth required) as valid SSE responses
	// This allows protocol detection to work even when authentication is required
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusUnauthorized {
		log.Info("SSE endpoint returned unexpected status",
			"endpoint", endpoint,
			"status", resp.StatusCode)
		return false
	}

	// Try to read a bit of the response to see if it looks like SSE
	// SSE format has lines starting with "data:", "event:", "id:", etc.
	buf := make([]byte, 256)
	n, _ := io.ReadFull(resp.Body, buf)
	if n > 0 {
		body := string(buf[:n])
		// Check for SSE markers
		if strings.Contains(body, "data:") || strings.Contains(body, "event:") || strings.Contains(body, "id:") {
			log.V(1).Info("SSE detected via stream markers",
				"endpoint", endpoint,
				"preview", body[:min(50, len(body))])
			return true
		}
		log.V(1).Info("SSE content-type correct but no stream markers found",
			"endpoint", endpoint,
			"preview", body[:min(50, len(body))])
	}

	// Even if we couldn't read SSE markers, if content type is correct, assume SSE
	log.Info("SSE detected via content-type",
		"endpoint", endpoint,
		"contentType", contentType)
	return true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
