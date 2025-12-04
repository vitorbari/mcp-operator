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
	"net/http"
	"time"

	"github.com/vitorbari/mcp-operator/pkg/mcp"
)

// Transport defines the interface for MCP transport implementations
// This interface allows for pluggable transport types (HTTP, SSE, stdio, WebSocket, etc.)
type Transport interface {
	// Initialize performs the MCP initialization handshake
	Initialize(ctx context.Context) (*mcp.InitializeResult, error)

	// Name returns the transport type name
	Name() TransportType

	// SupportsSessionManagement indicates if this transport supports sessions
	SupportsSessionManagement() bool

	// Close cleans up any resources
	Close() error

	// Capability testing methods

	// ListTools lists available tools from the MCP server
	ListTools(ctx context.Context) (*mcp.ListToolsResult, error)

	// ListResources lists available resources from the MCP server
	ListResources(ctx context.Context) (*mcp.ListResourcesResult, error)

	// ListPrompts lists available prompts from the MCP server
	ListPrompts(ctx context.Context) (*mcp.ListPromptsResult, error)
}

// TransportOptions contains configuration for creating transports
type TransportOptions struct {
	// Timeout for transport operations
	Timeout time.Duration

	// HTTPClient to use (optional, will create default if nil)
	HTTPClient *http.Client

	// EnableSessionManagement enables session support if available
	EnableSessionManagement bool

	// ClientInfo specifies custom client identification
	// If nil, uses default "mcp-operator-validator" v0.1.0
	ClientInfo *mcp.Implementation

	// BearerToken for authentication
	BearerToken string

	// CustomHeaders for additional authentication or metadata
	CustomHeaders map[string]string
}

// DefaultTransportOptions returns sensible defaults for transport creation
func DefaultTransportOptions() TransportOptions {
	return TransportOptions{
		Timeout:                 30 * time.Second,
		HTTPClient:              nil, // Will create default
		EnableSessionManagement: false,
	}
}

// TransportFactory creates transport instances
// This factory pattern allows for easy extension with new transport types
type TransportFactory interface {
	// CreateTransport creates a transport instance for the given type and endpoint
	CreateTransport(transportType TransportType, endpoint string, opts TransportOptions) (Transport, error)

	// SupportedTransports returns the list of transport types this factory can create
	SupportedTransports() []TransportType
}

// DefaultTransportFactory is the default factory implementation
type DefaultTransportFactory struct {
	httpClient *http.Client
}

// NewTransportFactory creates a new transport factory
// If httpClient is nil, a default client will be created for each transport
func NewTransportFactory(httpClient *http.Client) TransportFactory {
	return &DefaultTransportFactory{
		httpClient: httpClient,
	}
}

// CreateTransport creates a transport instance
func (f *DefaultTransportFactory) CreateTransport(
	transportType TransportType,
	endpoint string,
	opts TransportOptions,
) (Transport, error) {
	// Use provided HTTP client or create one
	client := opts.HTTPClient
	if client == nil && f.httpClient != nil {
		client = f.httpClient
	}
	if client == nil {
		client = &http.Client{
			Timeout: opts.Timeout,
		}
	}

	switch transportType {
	case TransportStreamableHTTP:
		return newStreamableHTTPTransport(endpoint, client, opts), nil
	case TransportSSE:
		return newSSETransport(endpoint, client, opts), nil
	default:
		return nil, fmt.Errorf("unsupported transport type: %s", transportType)
	}
}

// SupportedTransports returns the list of supported transport types
func (f *DefaultTransportFactory) SupportedTransports() []TransportType {
	return []TransportType{
		TransportStreamableHTTP,
		TransportSSE,
	}
}

// streamableHTTPTransport wraps mcp.Client to implement Transport interface
type streamableHTTPTransport struct {
	client    *mcp.Client
	transport TransportType
}

func newStreamableHTTPTransport(endpoint string, httpClient *http.Client, opts TransportOptions) Transport {
	// Note: We don't directly use the provided httpClient because mcp.Client manages its own
	// TODO: Consider adding WithHTTPClient option to mcp.Client for advanced cases
	_ = httpClient

	// Prepare client options
	clientOpts := []mcp.Option{}

	// Set timeout (mcp.Client creates its own http.Client with this timeout)
	if opts.Timeout > 0 {
		clientOpts = append(clientOpts, mcp.WithTimeout(opts.Timeout))
	}

	// Set client info or use default
	if opts.ClientInfo != nil {
		clientOpts = append(clientOpts, mcp.WithClientInfo(opts.ClientInfo.Name, opts.ClientInfo.Version))
	} else {
		// Default for validator
		clientOpts = append(clientOpts, mcp.WithClientInfo("mcp-operator-validator", "0.1.0"))
	}

	// Add authentication if provided
	if opts.BearerToken != "" {
		clientOpts = append(clientOpts, mcp.WithBearerToken(opts.BearerToken))
	}
	if len(opts.CustomHeaders) > 0 {
		clientOpts = append(clientOpts, mcp.WithHeaders(opts.CustomHeaders))
	}

	// Create mcp.Client
	client := mcp.NewClient(endpoint, clientOpts...)

	return &streamableHTTPTransport{
		client:    client,
		transport: TransportStreamableHTTP,
	}
}

func (t *streamableHTTPTransport) Initialize(ctx context.Context) (*mcp.InitializeResult, error) {
	return t.client.Initialize(ctx)
}

func (t *streamableHTTPTransport) Name() TransportType {
	return t.transport
}

func (t *streamableHTTPTransport) SupportsSessionManagement() bool {
	// Streamable HTTP supports session management
	return true
}

func (t *streamableHTTPTransport) Close() error {
	// mcp.Client doesn't require explicit cleanup
	// HTTP connections are managed by the http.Client's connection pool
	return nil
}

func (t *streamableHTTPTransport) ListTools(ctx context.Context) (*mcp.ListToolsResult, error) {
	return t.client.ListTools(ctx)
}

func (t *streamableHTTPTransport) ListResources(ctx context.Context) (*mcp.ListResourcesResult, error) {
	return t.client.ListResources(ctx)
}

func (t *streamableHTTPTransport) ListPrompts(ctx context.Context) (*mcp.ListPromptsResult, error) {
	return t.client.ListPrompts(ctx)
}

// sseTransport wraps SSEClient to implement Transport interface
type sseTransport struct {
	client    *SSEClient
	transport TransportType
	endpoint  string
	connected bool
}

func newSSETransport(endpoint string, httpClient *http.Client, opts TransportOptions) Transport {
	// Create SSE client - note: SSE client manages its own HTTP client configuration
	// We don't use the provided httpClient here because SSE requires special timeout handling
	_ = httpClient

	// Use custom client info or default for validator
	var clientInfo *mcp.Implementation
	if opts.ClientInfo != nil {
		clientInfo = opts.ClientInfo
	}

	client := NewSSEClient(endpoint, opts.Timeout, clientInfo)

	return &sseTransport{
		client:    client,
		transport: TransportSSE,
		endpoint:  endpoint,
		connected: false,
	}
}

func (t *sseTransport) Initialize(ctx context.Context) (*mcp.InitializeResult, error) {
	// SSE requires Connect() to be called before Initialize()
	if !t.connected {
		if err := t.client.Connect(ctx); err != nil {
			return nil, fmt.Errorf("failed to connect SSE transport: %w", err)
		}
		t.connected = true
	}

	return t.client.Initialize(ctx)
}

func (t *sseTransport) Name() TransportType {
	return t.transport
}

func (t *sseTransport) SupportsSessionManagement() bool {
	// SSE transport does not support session management
	return false
}

func (t *sseTransport) Close() error {
	t.connected = false
	return t.client.Close()
}

func (t *sseTransport) ListTools(ctx context.Context) (*mcp.ListToolsResult, error) {
	// SSE client doesn't currently implement capability testing
	// This is acceptable as SSE is a legacy transport
	return nil, fmt.Errorf("ListTools not implemented for SSE transport")
}

func (t *sseTransport) ListResources(ctx context.Context) (*mcp.ListResourcesResult, error) {
	// SSE client doesn't currently implement capability testing
	return nil, fmt.Errorf("ListResources not implemented for SSE transport")
}

func (t *sseTransport) ListPrompts(ctx context.Context) (*mcp.ListPromptsResult, error) {
	// SSE client doesn't currently implement capability testing
	return nil, fmt.Errorf("ListPrompts not implemented for SSE transport")
}
