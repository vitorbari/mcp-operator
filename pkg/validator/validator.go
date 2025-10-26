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
	"strings"
	"time"

	"github.com/vitorbari/mcp-operator/internal/mcp"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// Supported MCP protocol versions
	ProtocolVersion20241105 = "2024-11-05"
	ProtocolVersion20250326 = "2025-03-26"
)

// Validator validates MCP protocol compliance
type Validator struct {
	baseURL  string
	detector *TransportDetector
	timeout  time.Duration
}

// ValidationOptions configures validation behavior
type ValidationOptions struct {
	// RequiredCapabilities are capabilities that must be present
	// Valid values: "tools", "resources", "prompts"
	RequiredCapabilities []string

	// Timeout for validation operations
	Timeout time.Duration

	// StrictMode requires all checks to pass
	StrictMode bool

	// ConfiguredPath is the path configured in the MCPServer spec
	// If empty, auto-detection will try default paths
	ConfiguredPath string

	// Transport specifies the transport type to use
	// If set, skips auto-detection and uses the specified transport
	// Valid values: TransportStreamableHTTP, TransportSSE, or empty for auto-detection
	Transport TransportType
}

// ValidationResult contains the results of protocol validation
type ValidationResult struct {
	// Success indicates if validation passed
	Success bool

	// ProtocolVersion is the detected MCP protocol version
	ProtocolVersion string

	// Capabilities lists discovered server capabilities
	Capabilities []string

	// ServerInfo contains server implementation details
	ServerInfo *ServerInfo

	// Issues contains any validation problems found
	Issues []ValidationIssue

	// Duration is how long validation took
	Duration time.Duration

	// DetectedTransport indicates which transport protocol was used
	DetectedTransport TransportType

	// Endpoint is the full URL that was validated
	Endpoint string
}

// ServerInfo contains server implementation details
type ServerInfo struct {
	Name    string
	Version string
}

// ValidationIssue represents a validation problem
type ValidationIssue struct {
	// Level is the severity: "error", "warning", "info"
	Level string

	// Message is a human-readable description
	Message string

	// Code is a machine-readable issue code
	Code string
}

// Issue severity levels
const (
	LevelError   = "error"
	LevelWarning = "warning"
	LevelInfo    = "info"
)

// Issue codes
const (
	CodeInitializeFailed       = "INIT_FAILED"
	CodeInvalidProtocolVersion = "INVALID_PROTOCOL"
	CodeMissingServerInfo      = "MISSING_SERVER_INFO"
	CodeNoCapabilities         = "NO_CAPABILITIES"
	CodeMissingCapability      = "MISSING_CAPABILITY"
	CodeToolsListFailed        = "TOOLS_LIST_FAILED"
	CodeResourcesListFailed    = "RESOURCES_LIST_FAILED"
	CodePromptsListFailed      = "PROMPTS_LIST_FAILED"
)

// NewValidator creates a new MCP protocol validator
func NewValidator(baseURL string) *Validator {
	defaultTimeout := 30 * time.Second
	return &Validator{
		baseURL:  baseURL,
		detector: NewTransportDetector(defaultTimeout),
		timeout:  defaultTimeout,
	}
}

// NewValidatorWithTimeout creates a validator with custom timeout
func NewValidatorWithTimeout(baseURL string, timeout time.Duration) *Validator {
	return &Validator{
		baseURL:  baseURL,
		detector: NewTransportDetector(timeout),
		timeout:  timeout,
	}
}

// Validate performs complete MCP protocol validation
func (v *Validator) Validate(ctx context.Context, opts ValidationOptions) (*ValidationResult, error) {
	startTime := time.Now()

	result := &ValidationResult{
		Success: true,
		Issues:  []ValidationIssue{},
	}

	// Apply timeout if specified
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	// Step 1: Determine transport type and endpoint
	var transportType TransportType
	var endpoint string
	var err error

	if opts.Transport != "" && opts.Transport != TransportUnknown {
		// User specified transport - skip auto-detection
		transportType = opts.Transport

		// Build endpoint based on transport type and configured path
		path := opts.ConfiguredPath
		if path == "" {
			// Use default paths if none configured
			if transportType == TransportStreamableHTTP {
				path = "/mcp"
			} else if transportType == TransportSSE {
				path = "/sse"
			}
		}
		endpoint = strings.TrimRight(v.baseURL, "/") + path

		result.DetectedTransport = transportType
		result.Endpoint = endpoint
	} else {
		// Auto-detect transport type
		transportType, endpoint, err = v.detector.DetectTransport(ctx, v.baseURL, opts.ConfiguredPath)
		if err != nil {
			result.Success = false
			result.Issues = append(result.Issues, ValidationIssue{
				Level:   LevelError,
				Code:    "TRANSPORT_DETECTION_FAILED",
				Message: fmt.Sprintf("Failed to detect transport: %v", err),
			})
			result.Duration = time.Since(startTime)
			return result, nil
		}

		result.DetectedTransport = transportType
		result.Endpoint = endpoint
	}

	// Step 2: Validate using the detected transport
	var validationErr error
	switch transportType {
	case TransportStreamableHTTP:
		validationErr = v.validateStreamableHTTP(ctx, endpoint, opts, result)
	case TransportSSE:
		validationErr = v.validateSSE(ctx, endpoint, opts, result)
	default:
		result.Success = false
		result.Issues = append(result.Issues, ValidationIssue{
			Level:   LevelError,
			Code:    "UNKNOWN_TRANSPORT",
			Message: fmt.Sprintf("Unknown transport type: %s", transportType),
		})
	}

	if validationErr != nil {
		result.Success = false
		result.Issues = append(result.Issues, ValidationIssue{
			Level:   LevelError,
			Code:    CodeInitializeFailed,
			Message: fmt.Sprintf("Validation failed: %v", validationErr),
		})
	}

	// Final success determination in strict mode
	if opts.StrictMode && len(result.Issues) > 0 {
		for _, issue := range result.Issues {
			if issue.Level == LevelError {
				result.Success = false
				break
			}
		}
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// validateStreamableHTTP validates using Streamable HTTP transport
func (v *Validator) validateStreamableHTTP(ctx context.Context, endpoint string, opts ValidationOptions, result *ValidationResult) error {
	// Create Streamable HTTP client for this endpoint
	client := NewStreamableHTTPClient(endpoint, v.timeout)
	defer client.Close()

	// Step 1: Send initialize request
	initResult, err := client.Initialize(ctx)
	if err != nil {
		result.Success = false
		result.Issues = append(result.Issues, ValidationIssue{
			Level:   LevelError,
			Code:    CodeInitializeFailed,
			Message: fmt.Sprintf("Failed to initialize: %v", err),
		})
		return err
	}

	// Step 2: Check protocol version
	result.ProtocolVersion = initResult.ProtocolVersion
	if !isValidProtocolVersion(initResult.ProtocolVersion) {
		result.Success = false
		result.Issues = append(result.Issues, ValidationIssue{
			Level: LevelError,
			Code:  CodeInvalidProtocolVersion,
			Message: fmt.Sprintf("Unsupported protocol version: %s (expected %s or %s)",
				initResult.ProtocolVersion, ProtocolVersion20241105, ProtocolVersion20250326),
		})
	}

	// Step 3: Check server info is present
	if initResult.ServerInfo.Name == "" {
		result.Success = false
		result.Issues = append(result.Issues, ValidationIssue{
			Level:   LevelError,
			Code:    CodeMissingServerInfo,
			Message: "Server info is missing or incomplete",
		})
	} else {
		result.ServerInfo = &ServerInfo{
			Name:    initResult.ServerInfo.Name,
			Version: initResult.ServerInfo.Version,
		}
	}

	// Step 4: Discover capabilities
	capabilities := discoverCapabilities(initResult.Capabilities)
	result.Capabilities = capabilities

	if len(capabilities) == 0 {
		result.Issues = append(result.Issues, ValidationIssue{
			Level:   LevelWarning,
			Code:    CodeNoCapabilities,
			Message: "Server advertises no capabilities",
		})
	}

	// Step 5: Check required capabilities
	for _, required := range opts.RequiredCapabilities {
		if !contains(capabilities, required) {
			result.Success = false
			result.Issues = append(result.Issues, ValidationIssue{
				Level: LevelError,
				Code:  CodeMissingCapability,
				Message: fmt.Sprintf("Required capability '%s' is not advertised by server",
					required),
			})
		}
	}

	// Step 6: Test capability endpoints
	testStreamableHTTPCapabilityEndpoints(ctx, client, initResult.Capabilities, result)

	return nil
}

// validateSSE validates using SSE transport
func (v *Validator) validateSSE(ctx context.Context, endpoint string, opts ValidationOptions, result *ValidationResult) error {
	logger := log.FromContext(ctx)
	logger.Info("Starting SSE validation", "endpoint", endpoint)

	// Create SSE client
	client := NewSSEClient(endpoint, v.timeout)
	defer client.Close()

	// Step 1: Connect to SSE endpoint and get messages URL
	if err := client.Connect(ctx); err != nil {
		result.Success = false
		result.Issues = append(result.Issues, ValidationIssue{
			Level:   LevelError,
			Code:    "SSE_CONNECTION_FAILED",
			Message: fmt.Sprintf("Failed to connect to SSE endpoint: %v", err),
		})
		return err
	}

	// Step 2: Send initialize request
	initResult, err := client.Initialize(ctx)
	if err != nil {
		result.Success = false
		result.Issues = append(result.Issues, ValidationIssue{
			Level:   LevelError,
			Code:    CodeInitializeFailed,
			Message: fmt.Sprintf("Failed to initialize via SSE: %v", err),
		})
		return err
	}

	// Step 3: Check protocol version
	result.ProtocolVersion = initResult.ProtocolVersion
	if !isValidProtocolVersion(initResult.ProtocolVersion) {
		result.Success = false
		result.Issues = append(result.Issues, ValidationIssue{
			Level: LevelError,
			Code:  CodeInvalidProtocolVersion,
			Message: fmt.Sprintf("Unsupported protocol version: %s (expected %s or %s)",
				initResult.ProtocolVersion, ProtocolVersion20241105, ProtocolVersion20250326),
		})
	}

	// Step 4: Check server info is present
	if initResult.ServerInfo.Name == "" {
		result.Success = false
		result.Issues = append(result.Issues, ValidationIssue{
			Level:   LevelError,
			Code:    CodeMissingServerInfo,
			Message: "Server info is missing or incomplete",
		})
	} else {
		result.ServerInfo = &ServerInfo{
			Name:    initResult.ServerInfo.Name,
			Version: initResult.ServerInfo.Version,
		}
	}

	// Step 5: Discover capabilities
	capabilities := discoverCapabilities(initResult.Capabilities)
	result.Capabilities = capabilities

	if len(capabilities) == 0 {
		result.Issues = append(result.Issues, ValidationIssue{
			Level:   LevelWarning,
			Code:    CodeNoCapabilities,
			Message: "Server advertises no capabilities",
		})
	}

	// Step 6: Check required capabilities
	for _, required := range opts.RequiredCapabilities {
		if !contains(capabilities, required) {
			result.Success = false
			result.Issues = append(result.Issues, ValidationIssue{
				Level: LevelError,
				Code:  CodeMissingCapability,
				Message: fmt.Sprintf("Required capability '%s' is not advertised by server",
					required),
			})
		}
	}

	// Note: We skip capability endpoint testing for SSE as it would require
	// implementing full SSE client with ListTools/ListResources/ListPrompts methods.
	// The initialize request is sufficient to validate basic MCP compliance.

	return nil
}

// isValidProtocolVersion checks if the protocol version is supported
func isValidProtocolVersion(version string) bool {
	return version == ProtocolVersion20241105 || version == ProtocolVersion20250326
}

// discoverCapabilities extracts capability names from server capabilities
func discoverCapabilities(caps mcp.ServerCapabilities) []string {
	var capabilities []string

	if caps.Tools != nil {
		capabilities = append(capabilities, "tools")
	}
	if caps.Resources != nil {
		capabilities = append(capabilities, "resources")
	}
	if caps.Prompts != nil {
		capabilities = append(capabilities, "prompts")
	}
	if caps.Logging != nil {
		capabilities = append(capabilities, "logging")
	}

	return capabilities
}

// testStreamableHTTPCapabilityEndpoints tests that advertised capabilities actually work
func testStreamableHTTPCapabilityEndpoints(ctx context.Context, client *StreamableHTTPClient, caps mcp.ServerCapabilities, result *ValidationResult) {
	// Test tools/list if tools capability is advertised
	if caps.Tools != nil {
		if _, err := client.ListTools(ctx); err != nil {
			result.Issues = append(result.Issues, ValidationIssue{
				Level:   LevelWarning,
				Code:    CodeToolsListFailed,
				Message: fmt.Sprintf("Tools capability advertised but tools/list failed: %v", err),
			})
		}
	}

	// Test resources/list if resources capability is advertised
	if caps.Resources != nil {
		if _, err := client.ListResources(ctx); err != nil {
			result.Issues = append(result.Issues, ValidationIssue{
				Level:   LevelWarning,
				Code:    CodeResourcesListFailed,
				Message: fmt.Sprintf("Resources capability advertised but resources/list failed: %v", err),
			})
		}
	}

	// Test prompts/list if prompts capability is advertised
	if caps.Prompts != nil {
		if _, err := client.ListPrompts(ctx); err != nil {
			result.Issues = append(result.Issues, ValidationIssue{
				Level:   LevelWarning,
				Code:    CodePromptsListFailed,
				Message: fmt.Sprintf("Prompts capability advertised but prompts/list failed: %v", err),
			})
		}
	}
}

// contains checks if a string slice contains a value
func contains(slice []string, value string) bool {
	for _, item := range slice {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}

// IsCompliant is a convenience method to check if validation passed
func (r *ValidationResult) IsCompliant() bool {
	return r.Success
}

// HasErrors checks if the result contains any error-level issues
func (r *ValidationResult) HasErrors() bool {
	for _, issue := range r.Issues {
		if issue.Level == LevelError {
			return true
		}
	}
	return false
}

// ErrorMessages returns all error-level issue messages
func (r *ValidationResult) ErrorMessages() []string {
	var messages []string
	for _, issue := range r.Issues {
		if issue.Level == LevelError {
			messages = append(messages, issue.Message)
		}
	}
	return messages
}
