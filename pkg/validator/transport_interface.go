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

	"github.com/vitorbari/mcp-operator/internal/mcp"
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
}

// TransportOptions contains configuration for creating transports
type TransportOptions struct {
	// Timeout for transport operations
	Timeout time.Duration

	// HTTPClient to use (optional, will create default if nil)
	HTTPClient *http.Client

	// EnableSessionManagement enables session support if available
	EnableSessionManagement bool
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

// streamableHTTPTransport wraps StreamableHTTPClient to implement Transport interface
type streamableHTTPTransport struct {
	client    *StreamableHTTPClient
	transport TransportType
}

func newStreamableHTTPTransport(endpoint string, httpClient *http.Client, opts TransportOptions) Transport {
	// Create a new client with the provided HTTP client
	client := &StreamableHTTPClient{
		endpoint:   endpoint,
		httpClient: httpClient,
		timeout:    opts.Timeout,
	}

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
	return t.client.Close()
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
	_ = opts

	client := &SSEClient{
		httpClient: &http.Client{
			Timeout: 0, // SSE needs long-lived connections
		},
		sseEndpoint: endpoint,
		requestID:   1,
	}

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
