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
	"fmt"
	"strings"
)

// Issue codes
const (
	IssueCodeTransportDetectionFailed = "TRANSPORT_DETECTION_FAILED"
)

// EnhancedValidationIssue extends ValidationIssue with actionable suggestions
// This provides developers with concrete steps to resolve validation problems
type EnhancedValidationIssue struct {
	ValidationIssue

	// Suggestions are actionable steps to resolve the issue
	Suggestions []string

	// Documentation URL for more information
	DocumentationURL string

	// RelatedIssues are issue codes that commonly occur together
	RelatedIssues []string
}

// IssueCatalog provides detailed information and suggestions for common validation issues
type IssueCatalog struct {
	issues map[string]IssueTemplate
}

// IssueTemplate contains metadata and suggestions for an issue code
type IssueTemplate struct {
	Code             string
	Title            string
	Description      string
	Suggestions      []string
	DocumentationURL string
	RelatedIssues    []string
}

// NewIssueCatalog creates a catalog with predefined issue templates
func NewIssueCatalog() *IssueCatalog {
	catalog := &IssueCatalog{
		issues: make(map[string]IssueTemplate),
	}

	// Initialize with common issues
	catalog.registerDefaultIssues()

	return catalog
}

// registerDefaultIssues adds templates for all known validation issues
func (c *IssueCatalog) registerDefaultIssues() {
	c.issues["TRANSPORT_DETECTION_FAILED"] = IssueTemplate{
		Code:        "TRANSPORT_DETECTION_FAILED",
		Title:       "Unable to detect MCP transport",
		Description: "The validator could not determine which transport protocol the server uses",
		Suggestions: []string{
			"Verify the server is running and accessible at the configured URL",
			"Check that the server responds to either /mcp (HTTP) or /sse (SSE) endpoints",
			"Ensure firewall rules allow connections to the server port",
			"Try specifying the transport explicitly in ValidationOptions.Transport",
			"Check server logs for initialization errors",
		},
		DocumentationURL: "https://modelcontextprotocol.io/docs/concepts/transports",
		RelatedIssues:    []string{"TRANSPORT_CREATION_FAILED", "SSE_CONNECTION_FAILED"},
	}

	c.issues["TRANSPORT_CREATION_FAILED"] = IssueTemplate{
		Code:        "TRANSPORT_CREATION_FAILED",
		Title:       "Failed to create transport client",
		Description: "The transport type was detected but client creation failed",
		Suggestions: []string{
			"Verify the endpoint URL is correctly formatted",
			"Check that TLS/SSL certificates are valid (if using HTTPS)",
			"Ensure the server supports the detected transport protocol version",
			"Try increasing the timeout in ValidationOptions.Timeout",
		},
		DocumentationURL: "https://modelcontextprotocol.io/docs/concepts/transports",
		RelatedIssues:    []string{"TRANSPORT_DETECTION_FAILED"},
	}

	c.issues["SSE_CONNECTION_FAILED"] = IssueTemplate{
		Code:        "SSE_CONNECTION_FAILED",
		Title:       "SSE transport connection failed",
		Description: "Failed to establish Server-Sent Events connection",
		Suggestions: []string{
			"Verify the SSE endpoint path (default: /sse)",
			"Check that the server sends proper SSE headers (Content-Type: text/event-stream)",
			"Ensure the server keeps the connection open for SSE streaming",
			"Verify no proxy is interfering with long-lived connections",
			"Check server logs for connection errors",
		},
		DocumentationURL: "https://modelcontextprotocol.io/docs/concepts/transports#server-sent-events-sse",
		RelatedIssues:    []string{"TRANSPORT_DETECTION_FAILED"},
	}

	c.issues[CodeInitializeFailed] = IssueTemplate{
		Code:        CodeInitializeFailed,
		Title:       "MCP initialization handshake failed",
		Description: "The initialize request did not complete successfully",
		Suggestions: []string{
			"Verify the server implements the MCP initialize method",
			"Check server logs for errors during initialization",
			"Ensure the server returns valid InitializeResult with protocolVersion",
			"Verify serverInfo.name is set in the initialize response",
			"Check that the client and server protocol versions are compatible",
		},
		DocumentationURL: "https://modelcontextprotocol.io/docs/concepts/lifecycle#initialization",
		RelatedIssues:    []string{CodeInvalidProtocolVersion, CodeMissingServerInfo},
	}

	c.issues[CodeInvalidProtocolVersion] = IssueTemplate{
		Code:        CodeInvalidProtocolVersion,
		Title:       "Unsupported protocol version",
		Description: "The server uses an unsupported MCP protocol version",
		Suggestions: []string{
			fmt.Sprintf("Server must support one of: %s", strings.Join(SupportedProtocolVersions, ", ")),
			"Update the server to a compatible MCP protocol version",
			"Or update the operator to support the server's protocol version",
			"Check the server's protocolVersion in the initialize response",
		},
		DocumentationURL: "https://modelcontextprotocol.io/docs/specification/protocol",
		RelatedIssues:    []string{CodeInitializeFailed},
	}

	c.issues[CodeMissingServerInfo] = IssueTemplate{
		Code:        CodeMissingServerInfo,
		Title:       "Server information missing",
		Description: "The server did not provide required identification information",
		Suggestions: []string{
			"Ensure the server's initialize response includes serverInfo.name",
			"Add serverInfo.version for better observability (recommended)",
			"Check that the server implements the InitializeResult structure correctly",
		},
		DocumentationURL: "https://modelcontextprotocol.io/docs/concepts/lifecycle#initialization",
		RelatedIssues:    []string{CodeInitializeFailed},
	}

	c.issues[CodeNoCapabilities] = IssueTemplate{
		Code:        CodeNoCapabilities,
		Title:       "No capabilities advertised",
		Description: "The server advertises no MCP capabilities",
		Suggestions: []string{
			"This is unusual - most MCP servers should advertise at least one capability",
			"Verify the server implements tools, resources, or prompts capabilities",
			"Check the capabilities field in the initialize response",
			"The server may be under development or misconfigured",
		},
		DocumentationURL: "https://modelcontextprotocol.io/docs/concepts/capabilities",
		RelatedIssues:    []string{CodeMissingCapability},
	}

	c.issues[CodeMissingCapability] = IssueTemplate{
		Code:        CodeMissingCapability,
		Title:       "Required capability not advertised",
		Description: "The server does not advertise a capability that was specified as required",
		Suggestions: []string{
			"Verify the server implements the required capability",
			"Check the capabilities object in the initialize response",
			"Update the server to add the missing capability",
			"Or remove the capability from the required list if not actually needed",
		},
		DocumentationURL: "https://modelcontextprotocol.io/docs/concepts/capabilities",
		RelatedIssues:    []string{CodeNoCapabilities},
	}

	c.issues[CodeToolsListFailed] = IssueTemplate{
		Code:        CodeToolsListFailed,
		Title:       "Tools capability endpoint failed",
		Description: "The server advertises tools capability but tools/list request failed",
		Suggestions: []string{
			"Verify the server implements the tools/list method",
			"Check server logs for errors during tools/list handling",
			"Ensure the server returns a valid ListToolsResult",
			"The server may be partially implemented",
		},
		DocumentationURL: "https://modelcontextprotocol.io/docs/concepts/tools",
		RelatedIssues:    []string{CodeResourcesListFailed, CodePromptsListFailed},
	}

	c.issues[CodeResourcesListFailed] = IssueTemplate{
		Code:        CodeResourcesListFailed,
		Title:       "Resources capability endpoint failed",
		Description: "The server advertises resources capability but resources/list request failed",
		Suggestions: []string{
			"Verify the server implements the resources/list method",
			"Check server logs for errors during resources/list handling",
			"Ensure the server returns a valid ListResourcesResult",
			"The server may be partially implemented",
		},
		DocumentationURL: "https://modelcontextprotocol.io/docs/concepts/resources",
		RelatedIssues:    []string{CodeToolsListFailed, CodePromptsListFailed},
	}

	c.issues[CodePromptsListFailed] = IssueTemplate{
		Code:        CodePromptsListFailed,
		Title:       "Prompts capability endpoint failed",
		Description: "The server advertises prompts capability but prompts/list request failed",
		Suggestions: []string{
			"Verify the server implements the prompts/list method",
			"Check server logs for errors during prompts/list handling",
			"Ensure the server returns a valid ListPromptsResult",
			"The server may be partially implemented",
		},
		DocumentationURL: "https://modelcontextprotocol.io/docs/concepts/prompts",
		RelatedIssues:    []string{CodeToolsListFailed, CodeResourcesListFailed},
	}

	c.issues["RETRIES_EXHAUSTED"] = IssueTemplate{
		Code:        "RETRIES_EXHAUSTED",
		Title:       "Validation failed after multiple retries",
		Description: "The validation encountered retryable errors but exhausted all retry attempts",
		Suggestions: []string{
			"Check if the server is experiencing high load or startup delays",
			"Increase MaxAttempts in RetryConfig for slower-starting servers",
			"Increase InitialDelay and MaxDelay if network latency is high",
			"Review server logs to identify the root cause of failures",
			"Consider using a health check endpoint before validation",
		},
		DocumentationURL: "https://modelcontextprotocol.io/docs/best-practices/reliability",
		RelatedIssues:    []string{"TRANSPORT_DETECTION_FAILED", "SSE_CONNECTION_FAILED"},
	}

	c.issues["AUTH_REQUIRED"] = IssueTemplate{
		Code:        "AUTH_REQUIRED",
		Title:       "Server requires authentication",
		Description: "The server returned 401 Unauthorized or 403 Forbidden, indicating authentication is required",
		Suggestions: []string{
			"Provide authentication credentials to complete validation",
			"Check server documentation for supported authentication methods",
			"Common methods: Bearer token, API key, OAuth 2.1",
			"Configure authentication in the MCPServer spec or environment variables",
		},
		DocumentationURL: "https://modelcontextprotocol.io/specification/2025-03-26/basic/transports#authentication",
		RelatedIssues:    []string{"AUTH_ON_INITIALIZE"},
	}

	c.issues["AUTH_ON_INITIALIZE"] = IssueTemplate{
		Code:        "AUTH_ON_INITIALIZE",
		Title:       "Authentication required for initialization",
		Description: "Server requires authentication for the initialize request (non-standard behavior)",
		Suggestions: []string{
			"Best practice: Allow unauthenticated initialize requests for discoverability",
			"Consider restricting auth to tool/resource/prompt execution only",
			"This limits client integration and testing capabilities",
			"Provide authentication credentials to continue validation",
		},
		DocumentationURL: "https://modelcontextprotocol.io/specification/2025-03-26/basic/transports#authentication",
		RelatedIssues:    []string{"AUTH_REQUIRED"},
	}

	c.issues[CodeProtocolMismatch] = IssueTemplate{
		Code:        CodeProtocolMismatch,
		Title:       "Protocol mismatch detected",
		Description: "The configured protocol does not match the protocol detected from the server",
		Suggestions: []string{
			"Update spec.transport.protocol to match the detected protocol",
			"Or set spec.transport.protocol to 'auto' for automatic detection",
			"The server may have been updated to use a different protocol version",
			"Check server configuration and documentation for the correct transport protocol",
		},
		DocumentationURL: "https://modelcontextprotocol.io/docs/concepts/transports",
		RelatedIssues:    []string{CodeInvalidProtocolVersion, "TRANSPORT_DETECTION_FAILED"},
	}
}

// Enhance takes a ValidationIssue and returns an EnhancedValidationIssue with suggestions
func (c *IssueCatalog) Enhance(issue ValidationIssue) EnhancedValidationIssue {
	enhanced := EnhancedValidationIssue{
		ValidationIssue: issue,
	}

	// Look up issue template
	if template, ok := c.issues[issue.Code]; ok {
		enhanced.Suggestions = template.Suggestions
		enhanced.DocumentationURL = template.DocumentationURL
		enhanced.RelatedIssues = template.RelatedIssues
	}

	return enhanced
}

// EnhanceAll converts a slice of ValidationIssues to EnhancedValidationIssues
func (c *IssueCatalog) EnhanceAll(issues []ValidationIssue) []EnhancedValidationIssue {
	enhanced := make([]EnhancedValidationIssue, 0, len(issues))
	for _, issue := range issues {
		enhanced = append(enhanced, c.Enhance(issue))
	}
	return enhanced
}

// GetTemplate returns the issue template for a given code
func (c *IssueCatalog) GetTemplate(code string) (IssueTemplate, bool) {
	template, ok := c.issues[code]
	return template, ok
}

// RegisterIssue adds or updates an issue template in the catalog
// This allows custom issue types to be added
func (c *IssueCatalog) RegisterIssue(template IssueTemplate) {
	c.issues[template.Code] = template
}

// FormatSuggestions returns a formatted string with all suggestions
func (e *EnhancedValidationIssue) FormatSuggestions() string {
	if len(e.Suggestions) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Suggestions:\n")
	for i, suggestion := range e.Suggestions {
		sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, suggestion))
	}

	if e.DocumentationURL != "" {
		sb.WriteString(fmt.Sprintf("\nDocumentation: %s\n", e.DocumentationURL))
	}

	return sb.String()
}

// String returns a human-readable representation of the enhanced issue
func (e *EnhancedValidationIssue) String() string {
	var sb strings.Builder

	// Basic issue info
	sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", strings.ToUpper(e.Level), e.Code, e.Message))

	// Add suggestions if available
	if len(e.Suggestions) > 0 {
		sb.WriteString("\n")
		sb.WriteString(e.FormatSuggestions())
	}

	// Add related issues if any
	if len(e.RelatedIssues) > 0 {
		sb.WriteString(fmt.Sprintf("\nRelated issues: %s\n", strings.Join(e.RelatedIssues, ", ")))
	}

	return sb.String()
}

// DefaultIssueCatalog is a package-level catalog for convenience
var DefaultIssueCatalog = NewIssueCatalog()

// EnhanceIssue is a convenience function using the default catalog
func EnhanceIssue(issue ValidationIssue) EnhancedValidationIssue {
	return DefaultIssueCatalog.Enhance(issue)
}

// EnhanceIssues is a convenience function using the default catalog
func EnhanceIssues(issues []ValidationIssue) []EnhancedValidationIssue {
	return DefaultIssueCatalog.EnhanceAll(issues)
}
