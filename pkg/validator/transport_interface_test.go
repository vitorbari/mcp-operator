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
	"net/http/httptest"
	"testing"
	"time"
)

func TestTransportFactory_CreateTransport(t *testing.T) {
	factory := NewTransportFactory(nil)
	opts := DefaultTransportOptions()

	tests := []struct {
		name          string
		transportType TransportType
		wantErr       bool
		wantName      TransportType
	}{
		{
			name:          "CreateStreamableHTTP",
			transportType: TransportStreamableHTTP,
			wantErr:       false,
			wantName:      TransportStreamableHTTP,
		},
		{
			name:          "CreateSSE",
			transportType: TransportSSE,
			wantErr:       false,
			wantName:      TransportSSE,
		},
		{
			name:          "UnsupportedTransport",
			transportType: "websocket",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport, err := factory.CreateTransport(tt.transportType, "http://localhost:3001", opts)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error for unsupported transport, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("CreateTransport returned unexpected error: %v", err)
			}
			defer func() {
				_ = transport.Close()
			}()

			if transport.Name() != tt.wantName {
				t.Errorf("Transport.Name() = %v, want %v", transport.Name(), tt.wantName)
			}
		})
	}
}

func TestTransportFactory_SupportedTransports(t *testing.T) {
	factory := NewTransportFactory(nil)
	supported := factory.SupportedTransports()

	expectedCount := 2 // HTTP and SSE
	if len(supported) != expectedCount {
		t.Errorf("Expected %d supported transports, got %d", expectedCount, len(supported))
	}

	// Check that both HTTP and SSE are supported
	hasHTTP := false
	hasSSE := false
	for _, transport := range supported {
		switch transport {
		case TransportStreamableHTTP:
			hasHTTP = true
		case TransportSSE:
			hasSSE = true
		}
	}

	if !hasHTTP {
		t.Error("Expected StreamableHTTP to be in supported transports")
	}
	if !hasSSE {
		t.Error("Expected SSE to be in supported transports")
	}
}

func TestStreamableHTTPTransport_Interface(t *testing.T) {
	// Create a mock server
	config := validServerConfig()
	server := mockMCPServer(t, config)
	defer server.Close()

	factory := NewTransportFactory(nil)
	opts := DefaultTransportOptions()

	transport, err := factory.CreateTransport(TransportStreamableHTTP, server.URL, opts)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer func() {
		_ = transport.Close()
	}()

	// Test Name()
	if transport.Name() != TransportStreamableHTTP {
		t.Errorf("Name() = %v, want %v", transport.Name(), TransportStreamableHTTP)
	}

	// Test SupportsSessionManagement()
	if !transport.SupportsSessionManagement() {
		t.Error("StreamableHTTP should support session management")
	}

	// Test Initialize()
	ctx := context.Background()
	result, err := transport.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize() returned error: %v", err)
	}

	if result.ServerInfo.Name != "test-server" {
		t.Errorf("ServerInfo.Name = %v, want test-server", result.ServerInfo.Name)
	}
}

func TestSSETransport_Interface(t *testing.T) {
	// SSE tests require a real SSE server or testcontainers
	// For now, we'll skip this as it would require more setup
	t.Skip("SSE transport test requires SSE server setup")

	factory := NewTransportFactory(nil)
	opts := DefaultTransportOptions()

	transport, err := factory.CreateTransport(TransportSSE, "http://localhost:3001/sse", opts)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer func() {
		_ = transport.Close()
	}()

	// Test Name()
	if transport.Name() != TransportSSE {
		t.Errorf("Name() = %v, want %v", transport.Name(), TransportSSE)
	}

	// Test SupportsSessionManagement()
	if transport.SupportsSessionManagement() {
		t.Error("SSE should not support session management")
	}
}

func TestDefaultTransportOptions(t *testing.T) {
	opts := DefaultTransportOptions()

	if opts.Timeout == 0 {
		t.Error("DefaultTransportOptions() Timeout should not be zero")
	}

	if opts.Timeout != 30*time.Second {
		t.Errorf("DefaultTransportOptions() Timeout = %v, want 30s", opts.Timeout)
	}
}

func TestValidator_WithCustomFactory(t *testing.T) {
	// Create a mock server
	config := validServerConfig()
	server := mockMCPServer(t, config)
	defer server.Close()

	// Create validator with custom factory
	factory := NewTransportFactory(nil)
	validator := NewValidatorWithFactory(server.URL, 30*time.Second, factory)

	// Validate using explicit transport
	ctx := context.Background()
	result, err := validator.Validate(ctx, ValidationOptions{
		Transport: TransportStreamableHTTP,
	})

	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected validation to succeed, got failure with issues: %v", result.Issues)
	}
}

// TestTransportInterfaceBackwardCompatibility ensures the refactoring maintains compatibility
func TestTransportInterfaceBackwardCompatibility(t *testing.T) {
	// Create a mock server
	config := validServerConfig()
	server := mockMCPServer(t, config)
	defer server.Close()

	// Test that existing validators still work
	validator := NewValidator(server.URL)
	ctx := context.Background()

	result, err := validator.Validate(ctx, ValidationOptions{})
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected validation to succeed, got failure with issues: %v", result.Issues)
	}

	// Verify all expected fields are populated
	if result.ServerInfo == nil {
		t.Error("ServerInfo should not be nil")
	}
	if result.ProtocolVersion == "" {
		t.Error("ProtocolVersion should not be empty")
	}
	if len(result.Capabilities) == 0 {
		t.Error("Capabilities should not be empty")
	}
	if result.DetectedTransport == "" {
		t.Error("DetectedTransport should not be empty")
	}
}

// TestTransportClose ensures Close() is called properly
func TestTransportClose(t *testing.T) {
	server := httptest.NewServer(nil)
	defer server.Close()

	factory := NewTransportFactory(nil)
	opts := DefaultTransportOptions()

	transport, err := factory.CreateTransport(TransportStreamableHTTP, server.URL, opts)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	// Close should not error
	if err := transport.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	// Calling Close() multiple times should be safe
	if err := transport.Close(); err != nil {
		t.Errorf("Second Close() returned error: %v", err)
	}
}
