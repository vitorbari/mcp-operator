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
)

// Integration tests for SSE client against real MCP servers
// These tests are skipped unless MCP_SSE_TEST_ENDPOINT environment variable is set
//
// Example usage:
//   export MCP_SSE_TEST_ENDPOINT=http://localhost:8080/sse
//   go test -v -run TestSSEClient ./pkg/validator/

func getSSETestEndpoint(t *testing.T) string {
	endpoint := os.Getenv("MCP_SSE_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("Skipping integration test: MCP_SSE_TEST_ENDPOINT not set")
	}
	return endpoint
}

func TestSSEClientConnect(t *testing.T) {
	endpoint := getSSETestEndpoint(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := NewSSEClient(endpoint, 30*time.Second)
	defer func() {
		_ = client.Close()
	}()

	// Test connection establishment
	err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect to SSE endpoint: %v", err)
	}

	// Verify messages URL was set
	if client.messagesURL == "" {
		t.Error("Expected messagesURL to be set after connection")
	}

	t.Logf("Successfully connected to SSE endpoint: %s", endpoint)
	t.Logf("Messages URL: %s", client.messagesURL)
}

func TestSSEClientInitialize(t *testing.T) {
	endpoint := getSSETestEndpoint(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := NewSSEClient(endpoint, 30*time.Second)
	defer func() {
		_ = client.Close()
	}()

	// Connect first
	err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

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

func TestSSEClientEndpointDiscovery(t *testing.T) {
	endpoint := getSSETestEndpoint(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := NewSSEClient(endpoint, 30*time.Second)
	defer func() {
		_ = client.Close()
	}()

	// Connect and verify endpoint discovery
	err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Verify the messages URL was discovered
	if client.messagesURL == "" {
		t.Fatal("Expected messages URL to be discovered")
	}

	// The messages URL should be an absolute URL
	if len(client.messagesURL) < len(endpoint) {
		t.Error("Expected messages URL to be an absolute URL")
	}

	t.Logf("Discovered messages URL: %s", client.messagesURL)
}

func TestSSEClientMultipleRequests(t *testing.T) {
	endpoint := getSSETestEndpoint(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := NewSSEClient(endpoint, 60*time.Second)
	defer func() {
		_ = client.Close()
	}()

	// Connect
	err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

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
	expectedRequestID := int64(4) // Started at 1, did 3 requests
	if client.requestID != expectedRequestID {
		t.Errorf("Expected request ID to be %d, got %d", expectedRequestID, client.requestID)
	}
}

func TestSSEClientTimeout(t *testing.T) {
	endpoint := getSSETestEndpoint(t)

	// Use a very short timeout to test timeout handling
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	client := NewSSEClient(endpoint, 1*time.Millisecond)
	defer func() {
		_ = client.Close()
	}()

	// This should timeout quickly
	err := client.Connect(ctx)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	if err != context.DeadlineExceeded && err.Error() != "context deadline exceeded" {
		t.Logf("Got error (expected timeout-related): %v", err)
	}
}

func TestSSEClientInvalidEndpoint(t *testing.T) {
	// Test with an invalid endpoint
	invalidEndpoint := "http://invalid-endpoint-that-does-not-exist.local/sse"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := NewSSEClient(invalidEndpoint, 5*time.Second)
	defer func() {
		_ = client.Close()
	}()

	// This should fail with connection error
	err := client.Connect(ctx)
	if err == nil {
		t.Error("Expected error when connecting to invalid endpoint, got nil")
	}

	t.Logf("Got expected error for invalid endpoint: %v", err)
}

func TestSSEClientProtocolVersions(t *testing.T) {
	endpoint := getSSETestEndpoint(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := NewSSEClient(endpoint, 30*time.Second)
	defer func() {
		_ = client.Close()
	}()

	// Connect and initialize
	err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	result, err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Verify we get a valid protocol version
	// SSE typically uses the legacy 2024-11-05 version
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

// BenchmarkSSEClientInitialize benchmarks the SSE initialize performance
func BenchmarkSSEClientInitialize(b *testing.B) {
	endpoint := os.Getenv("MCP_SSE_TEST_ENDPOINT")
	if endpoint == "" {
		b.Skip("Skipping benchmark: MCP_SSE_TEST_ENDPOINT not set")
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client := NewSSEClient(endpoint, 30*time.Second)

		err := client.Connect(ctx)
		if err != nil {
			b.Fatalf("Failed to connect: %v", err)
		}

		_, err = client.Initialize(ctx)
		if err != nil {
			b.Fatalf("Failed to initialize: %v", err)
		}

		_ = client.Close()
	}
}
