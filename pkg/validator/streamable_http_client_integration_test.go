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
	"os"
	"testing"
	"time"

	"github.com/vitorbari/mcp-operator/internal/mcp"
)

// Integration tests for Streamable HTTP client against real MCP servers
// These tests are skipped unless MCP_HTTP_TEST_ENDPOINT environment variable is set
//
// Example usage:
//   export MCP_HTTP_TEST_ENDPOINT=http://localhost:3001/mcp
//   go test -v -run TestStreamableHTTPClient ./pkg/validator/

func getHTTPTestEndpoint(t *testing.T) string {
	endpoint := os.Getenv("MCP_HTTP_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("Skipping integration test: MCP_HTTP_TEST_ENDPOINT not set")
	}
	return endpoint
}

func TestStreamableHTTPClientInitialize(t *testing.T) {
	endpoint := getHTTPTestEndpoint(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := NewStreamableHTTPClient(endpoint, 30*time.Second)
	defer func() {
		_ = client.Close()
	}()

	// Test initialize request
	result, err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Verify result
	if result == nil {
		t.Fatal("Expected non-nil initialize result")
	}

	if result.ProtocolVersion == "" {
		t.Error("Expected protocol version to be set")
	}

	if result.ServerInfo.Name == "" {
		t.Error("Expected server name to be set")
	}

	t.Logf("Protocol version: %s", result.ProtocolVersion)
	t.Logf("Server: %s v%s", result.ServerInfo.Name, result.ServerInfo.Version)
	t.Logf("Capabilities: tools=%v, resources=%v, prompts=%v",
		result.Capabilities.Tools != nil,
		result.Capabilities.Resources != nil,
		result.Capabilities.Prompts != nil)
}

func TestStreamableHTTPClientMultipleRequests(t *testing.T) {
	endpoint := getHTTPTestEndpoint(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := NewStreamableHTTPClient(endpoint, 60*time.Second)
	defer func() {
		_ = client.Close()
	}()

	// Send multiple initialize requests to test request ID handling
	for i := 0; i < 3; i++ {
		result, err := client.Initialize(ctx)
		if err != nil {
			t.Fatalf("Failed to initialize on request %d: %v", i+1, err)
		}

		if result == nil {
			t.Fatalf("Expected non-nil result on request %d", i+1)
		}

		t.Logf("Request %d: Server %s, Protocol %s",
			i+1, result.ServerInfo.Name, result.ProtocolVersion)
	}

	// Verify request ID incremented
	expectedRequestID := int32(3) // Started at 0, did 3 requests
	if client.requestID.Load() != expectedRequestID {
		t.Errorf("Expected request ID to be %d, got %d", expectedRequestID, client.requestID.Load())
	}
}

func TestStreamableHTTPClientListCapabilities(t *testing.T) {
	endpoint := getHTTPTestEndpoint(t)

	tests := []struct {
		name           string
		capabilityName string
		hasCapability  func(*mcp.InitializeResult) bool
		listFunc       func(*StreamableHTTPClient, context.Context) (interface{}, error)
		getCount       func(interface{}) int
		logItems       func(*testing.T, interface{})
	}{
		{
			name:           "ListTools",
			capabilityName: "tools",
			hasCapability:  func(r *mcp.InitializeResult) bool { return r.Capabilities.Tools != nil },
			listFunc: func(c *StreamableHTTPClient, ctx context.Context) (interface{}, error) {
				return c.ListTools(ctx)
			},
			getCount: func(result interface{}) int { return len(result.(*mcp.ListToolsResult).Tools) },
			logItems: func(t *testing.T, result interface{}) {
				for i, tool := range result.(*mcp.ListToolsResult).Tools {
					t.Logf("  Tool %d: %s - %s", i+1, tool.Name, tool.Description)
				}
			},
		},
		{
			name:           "ListResources",
			capabilityName: "resources",
			hasCapability:  func(r *mcp.InitializeResult) bool { return r.Capabilities.Resources != nil },
			listFunc: func(c *StreamableHTTPClient, ctx context.Context) (interface{}, error) {
				return c.ListResources(ctx)
			},
			getCount: func(result interface{}) int { return len(result.(*mcp.ListResourcesResult).Resources) },
			logItems: func(t *testing.T, result interface{}) {
				for i, resource := range result.(*mcp.ListResourcesResult).Resources {
					t.Logf("  Resource %d: %s - %s", i+1, resource.Name, resource.Description)
				}
			},
		},
		{
			name:           "ListPrompts",
			capabilityName: "prompts",
			hasCapability:  func(r *mcp.InitializeResult) bool { return r.Capabilities.Prompts != nil },
			listFunc: func(c *StreamableHTTPClient, ctx context.Context) (interface{}, error) {
				return c.ListPrompts(ctx)
			},
			getCount: func(result interface{}) int { return len(result.(*mcp.ListPromptsResult).Prompts) },
			logItems: func(t *testing.T, result interface{}) {
				for i, prompt := range result.(*mcp.ListPromptsResult).Prompts {
					t.Logf("  Prompt %d: %s - %s", i+1, prompt.Name, prompt.Description)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			client := NewStreamableHTTPClient(endpoint, 30*time.Second)
			defer func() {
				_ = client.Close()
			}()

			// Initialize first
			initResult, err := client.Initialize(ctx)
			if err != nil {
				t.Fatalf("Failed to initialize: %v", err)
			}

			// Only test if the server advertises the capability
			if !tt.hasCapability(initResult) {
				t.Skipf("Server does not advertise %s capability", tt.capabilityName)
			}

			// Test the list function
			result, err := tt.listFunc(client, ctx)
			if err != nil {
				t.Fatalf("Failed to list %s: %v", tt.capabilityName, err)
			}

			if result == nil {
				t.Fatalf("Expected non-nil %s list result", tt.capabilityName)
			}

			count := tt.getCount(result)
			t.Logf("Found %d %s", count, tt.capabilityName)
			tt.logItems(t, result)
		})
	}
}

func TestStreamableHTTPClientTimeout(t *testing.T) {
	endpoint := getHTTPTestEndpoint(t)

	// Use a very short timeout to test timeout handling
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	client := NewStreamableHTTPClient(endpoint, 1*time.Millisecond)
	defer func() {
		_ = client.Close()
	}()

	// This should timeout quickly
	_, err := client.Initialize(ctx)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	if err != context.DeadlineExceeded && err.Error() != "context deadline exceeded" {
		t.Logf("Got error (expected timeout-related): %v", err)
	}
}

func TestStreamableHTTPClientInvalidEndpoint(t *testing.T) {
	// Test with an invalid endpoint (using localhost with wrong port to fail fast)
	invalidEndpoint := "http://localhost:1/mcp"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := NewStreamableHTTPClient(invalidEndpoint, 2*time.Second)
	defer func() {
		_ = client.Close()
	}()

	// This should fail with connection error
	_, err := client.Initialize(ctx)
	if err == nil {
		t.Error("Expected error when connecting to invalid endpoint, got nil")
	}

	t.Logf("Got expected error for invalid endpoint: %v", err)
}

func TestStreamableHTTPClientPing(t *testing.T) {
	endpoint := getHTTPTestEndpoint(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := NewStreamableHTTPClient(endpoint, 30*time.Second)
	defer func() {
		_ = client.Close()
	}()

	// Test Ping convenience method
	err := client.Ping(ctx)
	if err != nil {
		t.Fatalf("Failed to ping server: %v", err)
	}

	t.Log("Successfully pinged MCP server")
}

func TestStreamableHTTPClientProtocolVersions(t *testing.T) {
	endpoint := getHTTPTestEndpoint(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := NewStreamableHTTPClient(endpoint, 30*time.Second)
	defer func() {
		_ = client.Close()
	}()

	result, err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Verify we get a valid protocol version
	validVersions := []string{ProtocolVersion20241105, ProtocolVersion20250326}
	foundValid := false
	for _, validVersion := range validVersions {
		if result.ProtocolVersion == validVersion {
			foundValid = true
			break
		}
	}

	if !foundValid {
		t.Errorf("Expected protocol version to be one of %v, got %s",
			validVersions, result.ProtocolVersion)
	}

	t.Logf("Server uses protocol version: %s", result.ProtocolVersion)
}

// BenchmarkStreamableHTTPClientInitialize benchmarks the Streamable HTTP initialize performance
func BenchmarkStreamableHTTPClientInitialize(b *testing.B) {
	endpoint := os.Getenv("MCP_HTTP_TEST_ENDPOINT")
	if endpoint == "" {
		b.Skip("Skipping benchmark: MCP_HTTP_TEST_ENDPOINT not set")
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client := NewStreamableHTTPClient(endpoint, 30*time.Second)

		_, err := client.Initialize(ctx)
		if err != nil {
			b.Fatalf("Failed to initialize: %v", err)
		}

		_ = client.Close()
	}
}
