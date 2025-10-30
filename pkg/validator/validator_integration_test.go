//go:build integration
// +build integration

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
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Integration tests using actual MCP server Docker containers
// These tests validate the validator against known MCP servers using testcontainers
//
// IMPORTANT: These tests use the "integration" build tag and are SKIPPED by default
// due to a known issue with testcontainers command override (see VALIDATOR_PERFECTION_REPORT.md)
//
// To enable these tests, use the -tags flag:
//   go test -tags integration -v ./pkg/validator/

// TestRealWorldMCPEverythingServerSSE tests against the mcp-everything-server using SSE transport
func TestRealWorldMCPEverythingServerSSE(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real-world server test in short mode")
	}

	ctx := context.Background()

	// Start mcp-everything-server container in SSE mode
	req := testcontainers.ContainerRequest{
		Image:        "tzolov/mcp-everything-server:v3",
		ExposedPorts: []string{"3001/tcp"},
		Cmd:          []string{"node", "dist/index.js", "sse"},
		WaitingFor:   wait.ForListeningPort("3001/tcp").WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start mcp-everything-server container: %v", err)
	}
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Get container endpoint
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "3001")
	if err != nil {
		t.Fatalf("Failed to get container port: %v", err)
	}

	baseURL := fmt.Sprintf("http://%s:%s", host, port.Port())
	t.Logf("Testing validator against mcp-everything-server (SSE) at: %s", baseURL)

	// Give the server a moment to fully initialize
	time.Sleep(2 * time.Second)

	// Create validator
	validator := NewValidatorWithTimeout(baseURL, 30*time.Second)

	// Run validation
	testCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	result, err := validator.Validate(testCtx, ValidationOptions{
		ConfiguredPath: "/sse", // Explicitly test SSE endpoint
	})
	if err != nil {
		t.Fatalf("Validation returned error: %v", err)
	}

	// Verify validation succeeded
	if !result.Success {
		t.Errorf("Validation failed. Issues: %v", result.Issues)
	}

	// Verify transport detection
	if result.DetectedTransport != TransportSSE {
		t.Errorf("Expected SSE transport, got: %s", result.DetectedTransport)
	}

	// Verify protocol version
	validVersions := []string{ProtocolVersion20241105, ProtocolVersion20250326}
	foundValid := false
	for _, v := range validVersions {
		if result.ProtocolVersion == v {
			foundValid = true
			break
		}
	}
	if !foundValid {
		t.Errorf("Unexpected protocol version: %s", result.ProtocolVersion)
	}

	// Verify server info
	if result.ServerInfo == nil {
		t.Error("Server info is nil")
	} else {
		if result.ServerInfo.Name == "" {
			t.Error("Server name is empty")
		}
		t.Logf("Server: %s v%s", result.ServerInfo.Name, result.ServerInfo.Version)
	}

	// Verify capabilities
	if len(result.Capabilities) == 0 {
		t.Error("No capabilities reported")
	}
	t.Logf("Capabilities: %v", result.Capabilities)

	// MCP Everything Server should have multiple capabilities
	expectedCaps := []string{"tools", "resources", "prompts"}
	for _, cap := range expectedCaps {
		if !contains(result.Capabilities, cap) {
			t.Errorf("Expected capability %s not found", cap)
		}
	}

	t.Logf("✅ mcp-everything-server (SSE) validation successful in %v", result.Duration)
}

// TestRealWorldMCPEverythingServerHTTP tests against the mcp-everything-server using Streamable HTTP transport
func TestRealWorldMCPEverythingServerHTTP(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real-world server test in short mode")
	}

	ctx := context.Background()

	// Start mcp-everything-server container in Streamable HTTP mode (default)
	req := testcontainers.ContainerRequest{
		Image:        "tzolov/mcp-everything-server:v3",
		ExposedPorts: []string{"3001/tcp"},
		Cmd:          []string{"node", "dist/index.js"},
		WaitingFor:   wait.ForListeningPort("3001/tcp").WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start mcp-everything-server container: %v", err)
	}
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Get container endpoint
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "3001")
	if err != nil {
		t.Fatalf("Failed to get container port: %v", err)
	}

	baseURL := fmt.Sprintf("http://%s:%s", host, port.Port())
	t.Logf("Testing validator against mcp-everything-server (HTTP) at: %s", baseURL)

	// Give the server a moment to fully initialize
	time.Sleep(2 * time.Second)

	// Create validator
	validator := NewValidatorWithTimeout(baseURL, 30*time.Second)

	// Run validation
	testCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	result, err := validator.Validate(testCtx, ValidationOptions{
		ConfiguredPath: "/mcp", // Explicitly test Streamable HTTP endpoint
	})
	if err != nil {
		t.Fatalf("Validation returned error: %v", err)
	}

	// Verify validation succeeded
	if !result.Success {
		t.Errorf("Validation failed. Issues: %v", result.Issues)
	}

	// Verify transport detection
	if result.DetectedTransport != TransportStreamableHTTP {
		t.Errorf("Expected Streamable HTTP transport, got: %s", result.DetectedTransport)
	}

	// Verify protocol version
	validVersions := []string{ProtocolVersion20241105, ProtocolVersion20250326}
	foundValid := false
	for _, v := range validVersions {
		if result.ProtocolVersion == v {
			foundValid = true
			break
		}
	}
	if !foundValid {
		t.Errorf("Unexpected protocol version: %s", result.ProtocolVersion)
	}

	// Verify server info
	if result.ServerInfo == nil {
		t.Error("Server info is nil")
	} else {
		if result.ServerInfo.Name == "" {
			t.Error("Server name is empty")
		}
		t.Logf("Server: %s v%s", result.ServerInfo.Name, result.ServerInfo.Version)
	}

	// Verify capabilities
	if len(result.Capabilities) == 0 {
		t.Error("No capabilities reported")
	}
	t.Logf("Capabilities: %v", result.Capabilities)

	// MCP Everything Server should have multiple capabilities
	expectedCaps := []string{"tools", "resources", "prompts"}
	for _, cap := range expectedCaps {
		if !contains(result.Capabilities, cap) {
			t.Errorf("Expected capability %s not found", cap)
		}
	}

	t.Logf("✅ mcp-everything-server (HTTP) validation successful in %v", result.Duration)
}

// TestRealWorldTransportAutoDetection tests the validator's ability to auto-detect transport
func TestRealWorldTransportAutoDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real-world server test in short mode")
	}

	ctx := context.Background()

	// Start mcp-everything-server container in Streamable HTTP mode (default)
	req := testcontainers.ContainerRequest{
		Image:        "tzolov/mcp-everything-server:v3",
		ExposedPorts: []string{"3001/tcp"},
		Cmd:          []string{"node", "dist/index.js"},
		WaitingFor:   wait.ForListeningPort("3001/tcp").WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start mcp-everything-server container: %v", err)
	}
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Get container endpoint
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "3001")
	if err != nil {
		t.Fatalf("Failed to get container port: %v", err)
	}

	baseURL := fmt.Sprintf("http://%s:%s", host, port.Port())
	t.Logf("Testing validator auto-detection against: %s", baseURL)

	// Give the server a moment to fully initialize
	time.Sleep(2 * time.Second)

	// Create validator
	validator := NewValidatorWithTimeout(baseURL, 30*time.Second)

	// Run validation WITHOUT specifying a path - let it auto-detect
	testCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	result, err := validator.Validate(testCtx, ValidationOptions{
		// No ConfiguredPath - should auto-detect
	})
	if err != nil {
		t.Fatalf("Validation returned error: %v", err)
	}

	// Verify validation succeeded
	if !result.Success {
		t.Errorf("Auto-detection validation failed. Issues: %v", result.Issues)
	}

	// Verify a transport was detected (prefer Streamable HTTP over SSE)
	if result.DetectedTransport == TransportUnknown {
		t.Error("Failed to auto-detect transport")
	}

	// The validator should prefer Streamable HTTP (newer standard)
	if result.DetectedTransport != TransportStreamableHTTP {
		t.Logf("Note: Auto-detected %s transport instead of preferred Streamable HTTP", result.DetectedTransport)
	}

	t.Logf("✅ Auto-detected transport: %s in %v", result.DetectedTransport, result.Duration)
}

// TestRealWorldRequiredCapabilities tests validation with required capabilities
func TestRealWorldRequiredCapabilities(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real-world server test in short mode")
	}

	ctx := context.Background()

	// Start mcp-everything-server container
	req := testcontainers.ContainerRequest{
		Image:        "tzolov/mcp-everything-server:v3",
		ExposedPorts: []string{"3001/tcp"},
		Cmd:          []string{"node", "dist/index.js"},
		WaitingFor:   wait.ForListeningPort("3001/tcp").WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start mcp-everything-server container: %v", err)
	}
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Get container endpoint
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "3001")
	if err != nil {
		t.Fatalf("Failed to get container port: %v", err)
	}

	baseURL := fmt.Sprintf("http://%s:%s", host, port.Port())

	// Give the server a moment to fully initialize
	time.Sleep(2 * time.Second)

	// Create validator
	validator := NewValidatorWithTimeout(baseURL, 30*time.Second)

	testCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// Test 1: Required capabilities that exist (should pass)
	t.Run("ValidRequiredCapabilities", func(t *testing.T) {
		result, err := validator.Validate(testCtx, ValidationOptions{
			RequiredCapabilities: []string{"tools", "resources"},
		})
		if err != nil {
			t.Fatalf("Validation returned error: %v", err)
		}

		if !result.Success {
			t.Errorf("Validation should succeed when required capabilities exist. Issues: %v", result.Issues)
		}

		t.Logf("✅ Required capabilities validation passed")
	})

	// Test 2: Required capability that doesn't exist (should fail)
	t.Run("MissingRequiredCapability", func(t *testing.T) {
		result, err := validator.Validate(testCtx, ValidationOptions{
			RequiredCapabilities: []string{"nonexistent_capability"},
		})
		if err != nil {
			t.Fatalf("Validation returned error: %v", err)
		}

		if result.Success {
			t.Error("Validation should fail when required capability is missing")
		}

		if !hasIssue(result.Issues, CodeMissingCapability) {
			t.Error("Expected missing capability issue")
		}

		t.Logf("✅ Missing required capability correctly detected")
	})
}

// TestRealWorldStrictMode tests validation in strict mode
func TestRealWorldStrictMode(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real-world server test in short mode")
	}

	ctx := context.Background()

	// Start mcp-everything-server container
	req := testcontainers.ContainerRequest{
		Image:        "tzolov/mcp-everything-server:v3",
		ExposedPorts: []string{"3001/tcp"},
		Cmd:          []string{"node", "dist/index.js"},
		WaitingFor:   wait.ForListeningPort("3001/tcp").WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start mcp-everything-server container: %v", err)
	}
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Get container endpoint
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "3001")
	if err != nil {
		t.Fatalf("Failed to get container port: %v", err)
	}

	baseURL := fmt.Sprintf("http://%s:%s", host, port.Port())

	// Give the server a moment to fully initialize
	time.Sleep(2 * time.Second)

	// Create validator
	validator := NewValidatorWithTimeout(baseURL, 30*time.Second)

	testCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// Validate in strict mode
	result, err := validator.Validate(testCtx, ValidationOptions{
		StrictMode: true,
	})
	if err != nil {
		t.Fatalf("Validation returned error: %v", err)
	}

	// In strict mode, any error should cause failure
	if !result.Success && result.HasErrors() {
		t.Errorf("Strict mode validation failed with errors: %v", result.ErrorMessages())
	}

	// Verify issues
	if len(result.Issues) > 0 {
		hasErrors := false
		for _, issue := range result.Issues {
			if issue.Level == LevelError {
				hasErrors = true
				t.Errorf("Unexpected error in strict mode: %s - %s", issue.Code, issue.Message)
			}
		}
		if !hasErrors {
			t.Logf("Strict mode validation passed with warnings: %v", result.Issues)
		}
	}

	t.Logf("✅ Strict mode validation completed in %v", result.Duration)
}
