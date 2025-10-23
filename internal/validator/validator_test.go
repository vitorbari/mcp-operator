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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vitorbari/mcp-operator/internal/mcp"
)

// mockMCPServer creates a test server that responds to MCP requests
func mockMCPServer(t *testing.T, config mockServerConfig) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request mcp.JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Logf("Failed to decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var result any
		var rpcErr *mcp.RPCError

		switch request.Method {
		case "initialize":
			if config.initializeFails {
				rpcErr = &mcp.RPCError{Code: -32603, Message: "Internal error"}
			} else {
				result = mcp.InitializeResult{
					ProtocolVersion: config.protocolVersion,
					Capabilities:    config.capabilities,
					ServerInfo:      config.serverInfo,
				}
			}
		case "tools/list":
			if config.toolsListFails {
				rpcErr = &mcp.RPCError{Code: -32603, Message: "Tools list failed"}
			} else {
				result = mcp.ListToolsResult{Tools: []mcp.Tool{}}
			}
		case "resources/list":
			if config.resourcesListFails {
				rpcErr = &mcp.RPCError{Code: -32603, Message: "Resources list failed"}
			} else {
				result = mcp.ListResourcesResult{Resources: []mcp.Resource{}}
			}
		case "prompts/list":
			if config.promptsListFails {
				rpcErr = &mcp.RPCError{Code: -32603, Message: "Prompts list failed"}
			} else {
				result = mcp.ListPromptsResult{Prompts: []mcp.Prompt{}}
			}
		default:
			rpcErr = &mcp.RPCError{Code: -32601, Message: "Method not found"}
		}

		response := mcp.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      request.ID,
			Result:  result,
			Error:   rpcErr,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Logf("Failed to encode response: %v", err)
		}
	}))
}

type mockServerConfig struct {
	protocolVersion    string
	serverInfo         mcp.Implementation
	capabilities       mcp.ServerCapabilities
	initializeFails    bool
	toolsListFails     bool
	resourcesListFails bool
	promptsListFails   bool
}

func validServerConfig() mockServerConfig {
	return mockServerConfig{
		protocolVersion: ProtocolVersion20241105,
		serverInfo: mcp.Implementation{
			Name:    "test-server",
			Version: "1.0.0",
		},
		capabilities: mcp.ServerCapabilities{
			Tools:     &mcp.ToolsCapability{},
			Resources: &mcp.ResourcesCapability{Subscribe: true},
			Prompts:   &mcp.PromptsCapability{},
		},
	}
}

func TestNewValidator(t *testing.T) {
	v := NewValidator("http://example.com/mcp")
	if v == nil {
		t.Fatal("NewValidator returned nil")
	}
	if v.client == nil {
		t.Error("Validator client is nil")
	}
}

func TestNewValidatorWithTimeout(t *testing.T) {
	timeout := 10 * time.Second
	v := NewValidatorWithTimeout("http://example.com/mcp", timeout)
	if v == nil {
		t.Fatal("NewValidatorWithTimeout returned nil")
	}
}

func TestValidator_ValidServer(t *testing.T) {
	config := validServerConfig()
	server := mockMCPServer(t, config)
	defer server.Close()

	validator := NewValidator(server.URL)
	ctx := context.Background()

	result, err := validator.Validate(ctx, ValidationOptions{})
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected validation to succeed, got failure with issues: %v", result.Issues)
	}

	if result.ProtocolVersion != ProtocolVersion20241105 {
		t.Errorf("Expected protocol version %s, got %s", ProtocolVersion20241105, result.ProtocolVersion)
	}

	if result.ServerInfo.Name != "test-server" {
		t.Errorf("Expected server name test-server, got %s", result.ServerInfo.Name)
	}

	expectedCaps := []string{"tools", "resources", "prompts"}
	if len(result.Capabilities) != len(expectedCaps) {
		t.Errorf("Expected %d capabilities, got %d: %v", len(expectedCaps), len(result.Capabilities), result.Capabilities)
	}

	if result.Duration == 0 {
		t.Error("Expected non-zero duration")
	}
}

func TestValidator_InvalidProtocolVersion(t *testing.T) {
	config := validServerConfig()
	config.protocolVersion = "1.0.0" // Invalid version
	server := mockMCPServer(t, config)
	defer server.Close()

	validator := NewValidator(server.URL)
	ctx := context.Background()

	result, err := validator.Validate(ctx, ValidationOptions{})
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	if result.Success {
		t.Error("Expected validation to fail for invalid protocol version")
	}

	if !hasIssue(result.Issues, CodeInvalidProtocolVersion) {
		t.Error("Expected invalid protocol version issue")
	}
}

func TestValidator_MissingServerInfo(t *testing.T) {
	config := validServerConfig()
	config.serverInfo = mcp.Implementation{} // Empty server info
	server := mockMCPServer(t, config)
	defer server.Close()

	validator := NewValidator(server.URL)
	ctx := context.Background()

	result, err := validator.Validate(ctx, ValidationOptions{})
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	if result.Success {
		t.Error("Expected validation to fail for missing server info")
	}

	if !hasIssue(result.Issues, CodeMissingServerInfo) {
		t.Error("Expected missing server info issue")
	}
}

func TestValidator_NoCapabilities(t *testing.T) {
	config := validServerConfig()
	config.capabilities = mcp.ServerCapabilities{} // No capabilities
	server := mockMCPServer(t, config)
	defer server.Close()

	validator := NewValidator(server.URL)
	ctx := context.Background()

	result, err := validator.Validate(ctx, ValidationOptions{})
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	if !hasIssue(result.Issues, CodeNoCapabilities) {
		t.Error("Expected no capabilities warning")
	}

	if len(result.Capabilities) != 0 {
		t.Errorf("Expected 0 capabilities, got %d", len(result.Capabilities))
	}
}

func TestValidator_RequiredCapabilities(t *testing.T) {
	config := validServerConfig()
	// Only advertise tools, not resources
	config.capabilities = mcp.ServerCapabilities{
		Tools: &mcp.ToolsCapability{},
	}
	server := mockMCPServer(t, config)
	defer server.Close()

	validator := NewValidator(server.URL)
	ctx := context.Background()

	result, err := validator.Validate(ctx, ValidationOptions{
		RequiredCapabilities: []string{"tools", "resources"},
	})
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	if result.Success {
		t.Error("Expected validation to fail when required capability is missing")
	}

	if !hasIssue(result.Issues, CodeMissingCapability) {
		t.Error("Expected missing capability issue")
	}
}

func TestValidator_InitializeFails(t *testing.T) {
	config := validServerConfig()
	config.initializeFails = true
	server := mockMCPServer(t, config)
	defer server.Close()

	validator := NewValidator(server.URL)
	ctx := context.Background()

	result, err := validator.Validate(ctx, ValidationOptions{})
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	if result.Success {
		t.Error("Expected validation to fail when initialize fails")
	}

	if !hasIssue(result.Issues, CodeInitializeFailed) {
		t.Error("Expected initialize failed issue")
	}
}

func TestValidator_CapabilityEndpointsFail(t *testing.T) {
	config := validServerConfig()
	config.toolsListFails = true
	config.resourcesListFails = true
	config.promptsListFails = true
	server := mockMCPServer(t, config)
	defer server.Close()

	validator := NewValidator(server.URL)
	ctx := context.Background()

	result, err := validator.Validate(ctx, ValidationOptions{})
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	// Should still succeed but have warnings
	if !result.Success {
		t.Error("Expected validation to succeed with warnings")
	}

	if !hasIssue(result.Issues, CodeToolsListFailed) {
		t.Error("Expected tools list failed warning")
	}

	if !hasIssue(result.Issues, CodeResourcesListFailed) {
		t.Error("Expected resources list failed warning")
	}

	if !hasIssue(result.Issues, CodePromptsListFailed) {
		t.Error("Expected prompts list failed warning")
	}
}

func TestValidator_StrictMode(t *testing.T) {
	config := validServerConfig()
	config.protocolVersion = "1.0.0" // Invalid version
	server := mockMCPServer(t, config)
	defer server.Close()

	validator := NewValidator(server.URL)
	ctx := context.Background()

	result, err := validator.Validate(ctx, ValidationOptions{
		StrictMode: true,
	})
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	if result.Success {
		t.Error("Expected validation to fail in strict mode with errors")
	}

	if !result.HasErrors() {
		t.Error("Expected HasErrors to return true")
	}

	errorMsgs := result.ErrorMessages()
	if len(errorMsgs) == 0 {
		t.Error("Expected error messages")
	}
}

func TestValidator_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer server.Close()

	validator := NewValidator(server.URL)
	ctx := context.Background()

	result, err := validator.Validate(ctx, ValidationOptions{
		Timeout: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	if result.Success {
		t.Error("Expected validation to fail on timeout")
	}

	if !hasIssue(result.Issues, CodeInitializeFailed) {
		t.Error("Expected initialize failed issue")
	}
}

func TestValidationResult_IsCompliant(t *testing.T) {
	result := &ValidationResult{Success: true}
	if !result.IsCompliant() {
		t.Error("Expected IsCompliant to return true")
	}

	result.Success = false
	if result.IsCompliant() {
		t.Error("Expected IsCompliant to return false")
	}
}

func TestValidationResult_HasErrors(t *testing.T) {
	result := &ValidationResult{
		Issues: []ValidationIssue{
			{Level: LevelWarning, Message: "warning"},
			{Level: LevelInfo, Message: "info"},
		},
	}

	if result.HasErrors() {
		t.Error("Expected HasErrors to return false when no errors present")
	}

	result.Issues = append(result.Issues, ValidationIssue{
		Level:   LevelError,
		Message: "error",
	})

	if !result.HasErrors() {
		t.Error("Expected HasErrors to return true when errors present")
	}
}

func TestValidationResult_ErrorMessages(t *testing.T) {
	result := &ValidationResult{
		Issues: []ValidationIssue{
			{Level: LevelError, Message: "error 1"},
			{Level: LevelWarning, Message: "warning"},
			{Level: LevelError, Message: "error 2"},
		},
	}

	msgs := result.ErrorMessages()
	if len(msgs) != 2 {
		t.Errorf("Expected 2 error messages, got %d", len(msgs))
	}

	if msgs[0] != "error 1" || msgs[1] != "error 2" {
		t.Errorf("Unexpected error messages: %v", msgs)
	}
}

func TestIsValidProtocolVersion(t *testing.T) {
	tests := []struct {
		version string
		valid   bool
	}{
		{ProtocolVersion20241105, true},
		{ProtocolVersion20250326, true},
		{"1.0.0", false},
		{"2024-11-04", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			if got := isValidProtocolVersion(tt.version); got != tt.valid {
				t.Errorf("isValidProtocolVersion(%q) = %v, want %v", tt.version, got, tt.valid)
			}
		})
	}
}

func TestDiscoverCapabilities(t *testing.T) {
	caps := mcp.ServerCapabilities{
		Tools:     &mcp.ToolsCapability{},
		Resources: &mcp.ResourcesCapability{},
		Prompts:   &mcp.PromptsCapability{},
		Logging:   &mcp.LoggingCapability{},
	}

	discovered := discoverCapabilities(caps)
	expected := []string{"tools", "resources", "prompts", "logging"}

	if len(discovered) != len(expected) {
		t.Errorf("Expected %d capabilities, got %d", len(expected), len(discovered))
	}

	for _, exp := range expected {
		if !contains(discovered, exp) {
			t.Errorf("Expected capability %s not found", exp)
		}
	}
}

func TestContains(t *testing.T) {
	slice := []string{"tools", "resources", "prompts"}

	if !contains(slice, "tools") {
		t.Error("Expected contains to return true for 'tools'")
	}

	if !contains(slice, "TOOLS") {
		t.Error("Expected contains to be case-insensitive")
	}

	if contains(slice, "logging") {
		t.Error("Expected contains to return false for 'logging'")
	}
}

// hasIssue checks if the issues list contains an issue with the given code
func hasIssue(issues []ValidationIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
