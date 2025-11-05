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
	"strings"
	"time"

	"github.com/vitorbari/mcp-operator/internal/mcp"
)

// Validator validates MCP protocol compliance
type Validator struct {
	baseURL          string
	detector         *TransportDetector
	timeout          time.Duration
	transportFactory TransportFactory
	versionDetector  *ProtocolVersionDetector
	metricsRecorder  MetricsRecorder
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

	// RequiresAuth indicates whether the server requires authentication
	RequiresAuth bool

	// AuthMethod describes the authentication method required (if detected)
	AuthMethod string
}

// ServerInfo contains server implementation details
type ServerInfo struct {
	Name    string
	Version string
}

// ValidationIssue represents a validation problem with actionable guidance
type ValidationIssue struct {
	// Level is the severity: "error", "warning", "info"
	Level string

	// Message is a human-readable description
	Message string

	// Code is a machine-readable issue code
	Code string

	// Suggestions are actionable steps to resolve the issue (populated automatically)
	Suggestions []string

	// DocumentationURL provides more information (populated automatically)
	DocumentationURL string

	// RelatedIssues are issue codes that commonly occur together (populated automatically)
	RelatedIssues []string
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
	CodeAuthRequired           = "AUTH_REQUIRED"
	CodeAuthOnInitialize       = "AUTH_ON_INITIALIZE"
	CodeProtocolMismatch       = "PROTOCOL_MISMATCH"
)

// Option configures a Validator during creation
type Option func(*Validator)

// WithTimeout sets the validation timeout
// This timeout applies to the entire validation operation including detection and initialization
func WithTimeout(d time.Duration) Option {
	return func(v *Validator) {
		v.timeout = d
		v.detector = NewTransportDetector(d)
	}
}

// WithFactory sets a custom transport factory
// This allows using custom transport implementations beyond the default HTTP and SSE
func WithFactory(f TransportFactory) Option {
	return func(v *Validator) {
		v.transportFactory = f
	}
}

// WithHTTPClient sets a custom HTTP client for all transports
// This is useful for customizing connection pooling, timeouts, or proxy settings
func WithHTTPClient(client *http.Client) Option {
	return func(v *Validator) {
		v.transportFactory = NewTransportFactory(client)
	}
}

// WithMetricsRecorder sets a custom metrics recorder
// This is primarily useful for testing or custom metrics collection
func WithMetricsRecorder(m MetricsRecorder) Option {
	return func(v *Validator) {
		v.metricsRecorder = m
	}
}

// WithMetricsEnabled enables or disables Prometheus metrics collection
// When disabled, the validator will use a no-op recorder that doesn't collect metrics
// When enabled (default), metrics are collected and must be registered via RegisterMetrics()
func WithMetricsEnabled(enabled bool) Option {
	return func(v *Validator) {
		if enabled {
			v.metricsRecorder = NewMetricsRecorder(true)
		} else {
			v.metricsRecorder = NewNoOpMetricsRecorder()
		}
	}
}

// newIssue creates a ValidationIssue with suggestions populated from the catalog
func newIssue(level, code, message string) ValidationIssue {
	issue := ValidationIssue{
		Level:   level,
		Code:    code,
		Message: message,
	}

	// Enhance with catalog information if available
	catalog := NewIssueCatalog()
	if template, exists := catalog.issues[code]; exists {
		issue.Suggestions = template.Suggestions
		issue.DocumentationURL = template.DocumentationURL
		issue.RelatedIssues = template.RelatedIssues
	}

	return issue
}

// newErrorIssue creates an error-level ValidationIssue
func newErrorIssue(code, message string) ValidationIssue {
	return newIssue(LevelError, code, message)
}

// newWarningIssue creates a warning-level ValidationIssue
func newWarningIssue(code, message string) ValidationIssue {
	return newIssue(LevelWarning, code, message)
}

// NewValidator creates a new MCP protocol validator with sensible defaults
// Additional configuration can be provided via functional options
//
// Example usage:
//
//	// Simple case with defaults
//	validator := NewValidator("http://localhost:8080")
//
//	// With custom timeout
//	validator := NewValidator("http://localhost:8080", WithTimeout(60*time.Second))
//
//	// With custom HTTP client for connection pooling
//	validator := NewValidator("http://localhost:8080", WithHTTPClient(myClient))
func NewValidator(baseURL string, opts ...Option) *Validator {
	defaultTimeout := 5 * time.Second

	// Create HTTP client with connection pooling and sensible defaults
	httpClient := &http.Client{
		Timeout: defaultTimeout,
		Transport: &http.Transport{
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	v := &Validator{
		baseURL:          baseURL,
		timeout:          defaultTimeout,
		detector:         NewTransportDetector(defaultTimeout),
		transportFactory: NewTransportFactory(httpClient),
		versionDetector:  NewProtocolVersionDetector(),
		metricsRecorder:  NewMetricsRecorder(true), // Enabled by default
	}

	// Apply functional options
	for _, opt := range opts {
		opt(v)
	}

	return v
}

// NewValidatorWithTimeout creates a validator with custom timeout
// Note: Consider using NewValidator with WithTimeout option instead for better composability
func NewValidatorWithTimeout(baseURL string, timeout time.Duration) *Validator {
	return NewValidator(baseURL, WithTimeout(timeout))
}

// NewValidatorWithFactory creates a validator with a custom transport factory
// Note: Consider using NewValidator with WithTimeout and WithFactory options instead
func NewValidatorWithFactory(baseURL string, timeout time.Duration, factory TransportFactory) *Validator {
	return NewValidator(baseURL, WithTimeout(timeout), WithFactory(factory))
}

// SetMetricsRecorder allows replacing the metrics recorder (useful for testing)
func (v *Validator) SetMetricsRecorder(recorder MetricsRecorder) {
	v.metricsRecorder = recorder
}

// recordValidationMetrics records metrics for a validation result
func (v *Validator) recordValidationMetrics(result *ValidationResult) {
	transportName := string(result.DetectedTransport)
	if transportName == "" {
		transportName = "unknown"
	}
	v.metricsRecorder.RecordValidation(transportName, result.Success, result.Duration)

	// Record protocol version if detected
	if result.ProtocolVersion != "" {
		v.metricsRecorder.RecordProtocolVersion(result.ProtocolVersion)
	}

	// Record errors
	for _, issue := range result.Issues {
		if issue.Level == LevelError {
			v.metricsRecorder.RecordError(issue.Code, transportName)
		}
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
			switch transportType {
			case TransportStreamableHTTP:
				path = DefaultMCPPath
			case TransportSSE:
				path = DefaultSSEPath
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
			result.Issues = append(result.Issues, newErrorIssue(
				IssueCodeTransportDetectionFailed,
				fmt.Sprintf("Failed to detect transport: %v", err),
			))
			result.Duration = time.Since(startTime)
			v.recordValidationMetrics(result)
			return result, nil
		}

		result.DetectedTransport = transportType
		result.Endpoint = endpoint
	}

	// Step 2: Create transport using factory
	transportOpts := TransportOptions{
		Timeout:                 v.timeout,
		HTTPClient:              nil,
		EnableSessionManagement: false,
	}

	transport, err := v.transportFactory.CreateTransport(transportType, endpoint, transportOpts)
	if err != nil {
		result.Success = false
		result.Issues = append(result.Issues, newErrorIssue(
			"TRANSPORT_CREATION_FAILED",
			fmt.Sprintf("Failed to create transport: %v", err),
		))
		result.Duration = time.Since(startTime)
		v.recordValidationMetrics(result)
		return result, nil
	}
	defer func() {
		_ = transport.Close()
	}()

	// Step 3: Validate using the transport
	validationErr := v.validateWithTransport(ctx, transport, opts, result)
	if validationErr != nil {
		// Check if this is an auth error
		if isAuthError(validationErr) {
			result.RequiresAuth = true
			result.AuthMethod = extractAuthMethod(validationErr, nil)
			result.Success = false
			result.Issues = append(result.Issues, newIssue(
				LevelInfo,
				CodeAuthRequired,
				"Server requires authentication",
			))
		} else {
			result.Success = false
			result.Issues = append(result.Issues, newErrorIssue(
				CodeInitializeFailed,
				fmt.Sprintf("Validation failed: %v", validationErr),
			))
		}
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

	// Record metrics
	v.recordValidationMetrics(result)

	return result, nil
}

// validateWithTransport performs validation using the Transport interface
// This method works with any transport implementation (HTTP, SSE, stdio, etc.)
func (v *Validator) validateWithTransport(
	ctx context.Context,
	transport Transport,
	opts ValidationOptions,
	result *ValidationResult,
) error {
	// Step 1: Initialize transport
	initResult, err := transport.Initialize(ctx)
	if err != nil {
		// Check if this is an auth error during initialization
		if isAuthError(err) {
			result.RequiresAuth = true
			result.AuthMethod = extractAuthMethod(err, nil)
			result.Success = false
			// Add warning that auth is required on initialize (non-standard)
			result.Issues = append(result.Issues, newIssue(
				LevelWarning,
				CodeAuthOnInitialize,
				"Server requires authentication for initialization (non-standard behavior)",
			))
			return err
		}

		result.Success = false
		result.Issues = append(result.Issues, newErrorIssue(
			CodeInitializeFailed,
			fmt.Sprintf("Failed to initialize: %v", err),
		))
		return err
	}

	// Step 2: Check protocol version
	result.ProtocolVersion = initResult.ProtocolVersion
	if !isValidProtocolVersion(initResult.ProtocolVersion) {
		result.Success = false
		result.Issues = append(result.Issues, newErrorIssue(
			CodeInvalidProtocolVersion,
			fmt.Sprintf("Unsupported protocol version: %s (expected %s or %s)",
				initResult.ProtocolVersion, ProtocolVersion20241105, ProtocolVersion20250326),
		))
	}

	// Step 3: Check server info is present
	if initResult.ServerInfo.Name == "" {
		result.Success = false
		result.Issues = append(result.Issues, newErrorIssue(
			CodeMissingServerInfo,
			"Server info is missing or incomplete",
		))
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
		result.Issues = append(result.Issues, newWarningIssue(
			CodeNoCapabilities,
			"Server advertises no capabilities",
		))
	}

	// Step 5: Check required capabilities
	for _, required := range opts.RequiredCapabilities {
		if !contains(capabilities, required) {
			result.Success = false
			result.Issues = append(result.Issues, newErrorIssue(
				CodeMissingCapability,
				fmt.Sprintf("Required capability '%s' is not advertised by server", required),
			))
		}
	}

	// Step 6: Test capability endpoints (only for transports that support it)
	// Currently only Streamable HTTP has the methods for testing capabilities
	if transport.Name() == TransportStreamableHTTP {
		// We need to cast to the concrete type to access ListTools, ListResources, ListPrompts
		if httpTransport, ok := transport.(*streamableHTTPTransport); ok {
			testCapabilityEndpoints(ctx, httpTransport.client, initResult.Capabilities, result)
		}
	}

	return nil
}

// isValidProtocolVersion checks if the protocol version is supported
func isValidProtocolVersion(version string) bool {
	detector := NewProtocolVersionDetector()
	return detector.IsSupported(version)
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

// testCapabilityEndpoints tests that advertised capabilities actually work
func testCapabilityEndpoints(
	ctx context.Context,
	client *StreamableHTTPClient,
	caps mcp.ServerCapabilities,
	result *ValidationResult,
) {
	// Test tools/list if tools capability is advertised
	if caps.Tools != nil {
		if _, err := client.ListTools(ctx); err != nil {
			result.Issues = append(result.Issues, newWarningIssue(
				CodeToolsListFailed,
				fmt.Sprintf("Tools capability advertised but tools/list failed: %v", err),
			))
		}
	}

	// Test resources/list if resources capability is advertised
	if caps.Resources != nil {
		if _, err := client.ListResources(ctx); err != nil {
			result.Issues = append(result.Issues, newWarningIssue(
				CodeResourcesListFailed,
				fmt.Sprintf("Resources capability advertised but resources/list failed: %v", err),
			))
		}
	}

	// Test prompts/list if prompts capability is advertised
	if caps.Prompts != nil {
		if _, err := client.ListPrompts(ctx); err != nil {
			result.Issues = append(result.Issues, newWarningIssue(
				CodePromptsListFailed,
				fmt.Sprintf("Prompts capability advertised but prompts/list failed: %v", err),
			))
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

// EnhanceIssues returns enhanced versions of the validation issues with suggestions
// Note: Issues are now pre-enhanced during validation, so this method simply converts them
// to the EnhancedValidationIssue type for backward compatibility
func (r *ValidationResult) EnhanceIssues() []EnhancedValidationIssue {
	enhanced := make([]EnhancedValidationIssue, len(r.Issues))
	for i, issue := range r.Issues {
		enhanced[i] = EnhancedValidationIssue{
			ValidationIssue:  issue,
			Suggestions:      issue.Suggestions,
			DocumentationURL: issue.DocumentationURL,
			RelatedIssues:    issue.RelatedIssues,
		}
	}
	return enhanced
}

// EnhanceIssuesWithCatalog returns enhanced issues using a custom catalog
// Note: Issues are now pre-enhanced during validation, so this is kept for backward compatibility
func (r *ValidationResult) EnhanceIssuesWithCatalog(catalog *IssueCatalog) []EnhancedValidationIssue {
	return r.EnhanceIssues()
}

// isAuthError checks if an error indicates authentication is required
func isAuthError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()
	// Check for HTTP 401 or 403 status codes in error message
	return strings.Contains(errMsg, "401") ||
		strings.Contains(errMsg, "403") ||
		strings.Contains(errMsg, "Unauthorized") ||
		strings.Contains(errMsg, "Forbidden")
}

// extractAuthMethod attempts to extract the authentication method from error or headers
func extractAuthMethod(err error, headers http.Header) string {
	if headers != nil {
		if wwwAuth := headers.Get("WWW-Authenticate"); wwwAuth != "" {
			// Extract auth scheme from WWW-Authenticate header
			// Format: "Bearer realm=..." or "Basic realm=..."
			parts := strings.Fields(wwwAuth)
			if len(parts) > 0 {
				return parts[0]
			}
		}
	}

	if err != nil {
		errMsg := err.Error()
		// Try to detect common auth methods from error messages
		if strings.Contains(strings.ToLower(errMsg), "bearer") {
			return "Bearer"
		}
		if strings.Contains(strings.ToLower(errMsg), "basic") {
			return "Basic"
		}
		if strings.Contains(strings.ToLower(errMsg), "oauth") {
			return "OAuth"
		}
	}

	return "Unknown"
}
