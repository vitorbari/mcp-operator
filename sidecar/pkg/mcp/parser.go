package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// Common parsing errors.
var (
	ErrEmptyBody     = errors.New("empty body")
	ErrInvalidJSON   = errors.New("invalid JSON")
	ErrInvalidFormat = errors.New("invalid JSON-RPC format")
)

// ParsedRequest contains the parsed information from a JSON-RPC request.
type ParsedRequest struct {
	// Method is the JSON-RPC method name.
	Method string

	// ID is the request ID (nil for notifications).
	ID *JSONRPCID

	// IsBatch indicates if this was a batch request.
	IsBatch bool

	// BatchSize is the number of requests in a batch (1 for single requests).
	BatchSize int

	// IsNotification indicates if this is a notification (no ID).
	IsNotification bool

	// ToolName is the tool name extracted from tools/call params.
	ToolName string

	// ResourceURI is the resource URI extracted from resources/read params.
	ResourceURI string

	// PromptName is the prompt name extracted from prompts/get params.
	PromptName string

	// Requests contains all parsed requests (for batch support).
	Requests []JSONRPCRequest
}

// ParsedResponse contains the parsed information from a JSON-RPC response.
type ParsedResponse struct {
	// ID is the response ID.
	ID *JSONRPCID

	// IsError indicates if this response contains an error.
	IsError bool

	// ErrorCode is the error code (if IsError is true).
	ErrorCode int

	// ErrorMessage is the error message (if IsError is true).
	ErrorMessage string

	// IsBatch indicates if this was a batch response.
	IsBatch bool

	// BatchSize is the number of responses in a batch (1 for single responses).
	BatchSize int

	// Responses contains all parsed responses (for batch support).
	Responses []JSONRPCResponse
}

// ParseRequest parses a JSON-RPC request body.
// It supports both single requests and batch requests.
func ParseRequest(body []byte) (*ParsedRequest, error) {
	body = bytes.TrimSpace(body)

	if len(body) == 0 {
		return nil, ErrEmptyBody
	}

	result := &ParsedRequest{}

	// Check if it's a batch request (starts with '[')
	if body[0] == '[' {
		return parseBatchRequest(body)
	}

	// Parse single request
	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}

	// Validate JSON-RPC format
	if req.JSONRPC != JSONRPCVersion {
		return nil, fmt.Errorf("%w: invalid or missing jsonrpc version", ErrInvalidFormat)
	}

	if req.Method == "" {
		return nil, fmt.Errorf("%w: missing method", ErrInvalidFormat)
	}

	result.Method = req.Method
	result.ID = req.ID
	result.IsBatch = false
	result.BatchSize = 1
	result.IsNotification = req.IsNotification()
	result.Requests = []JSONRPCRequest{req}

	// Extract MCP-specific information
	extractMCPInfo(result, &req)

	return result, nil
}

// parseBatchRequest parses a batch JSON-RPC request.
func parseBatchRequest(body []byte) (*ParsedRequest, error) {
	var requests []JSONRPCRequest
	if err := json.Unmarshal(body, &requests); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}

	if len(requests) == 0 {
		return nil, fmt.Errorf("%w: empty batch request", ErrInvalidFormat)
	}

	result := &ParsedRequest{
		IsBatch:   true,
		BatchSize: len(requests),
		Requests:  requests,
	}

	// Use the first request's method and ID for primary identification
	firstReq := requests[0]
	result.Method = firstReq.Method
	result.ID = firstReq.ID
	result.IsNotification = firstReq.IsNotification()

	// Extract MCP-specific information from the first request
	extractMCPInfo(result, &firstReq)

	return result, nil
}

// extractMCPInfo extracts MCP-specific information from a request.
func extractMCPInfo(result *ParsedRequest, req *JSONRPCRequest) {
	if req.Params == nil {
		return
	}

	switch req.Method {
	case MethodToolsCall:
		var params ToolsCallParams
		if err := json.Unmarshal(req.Params, &params); err == nil {
			result.ToolName = params.Name
		}

	case MethodResourcesRead:
		var params ResourcesReadParams
		if err := json.Unmarshal(req.Params, &params); err == nil {
			result.ResourceURI = params.URI
		}

	case MethodPromptsGet:
		var params PromptsGetParams
		if err := json.Unmarshal(req.Params, &params); err == nil {
			result.PromptName = params.Name
		}
	}
}

// ParseResponse parses a JSON-RPC response body.
// It supports both single responses and batch responses.
func ParseResponse(body []byte) (*ParsedResponse, error) {
	body = bytes.TrimSpace(body)

	if len(body) == 0 {
		return nil, ErrEmptyBody
	}

	// Check if it's a batch response (starts with '[')
	if body[0] == '[' {
		return parseBatchResponse(body)
	}

	// Parse single response
	var resp JSONRPCResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}

	result := &ParsedResponse{
		ID:        resp.ID,
		IsError:   resp.IsError(),
		IsBatch:   false,
		BatchSize: 1,
		Responses: []JSONRPCResponse{resp},
	}

	if resp.Error != nil {
		result.ErrorCode = resp.Error.Code
		result.ErrorMessage = resp.Error.Message
	}

	return result, nil
}

// parseBatchResponse parses a batch JSON-RPC response.
func parseBatchResponse(body []byte) (*ParsedResponse, error) {
	var responses []JSONRPCResponse
	if err := json.Unmarshal(body, &responses); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}

	if len(responses) == 0 {
		return nil, fmt.Errorf("%w: empty batch response", ErrInvalidFormat)
	}

	result := &ParsedResponse{
		IsBatch:   true,
		BatchSize: len(responses),
		Responses: responses,
	}

	// Use the first response's ID and error status for primary identification
	firstResp := responses[0]
	result.ID = firstResp.ID
	result.IsError = firstResp.IsError()

	if firstResp.Error != nil {
		result.ErrorCode = firstResp.Error.Code
		result.ErrorMessage = firstResp.Error.Message
	}

	return result, nil
}

// IsMCPMethod returns true if the method is a known MCP method.
func IsMCPMethod(method string) bool {
	switch method {
	case MethodInitialize,
		MethodInitialized,
		MethodPing,
		MethodToolsList,
		MethodToolsCall,
		MethodResourcesList,
		MethodResourcesRead,
		MethodResourcesTemplates,
		MethodResourcesSubscribe,
		MethodResourcesUnsubscribe,
		MethodPromptsList,
		MethodPromptsGet,
		MethodLoggingSetLevel,
		MethodCompletionComplete:
		return true
	default:
		return false
	}
}

// MethodCategory returns the category of an MCP method.
func MethodCategory(method string) string {
	switch method {
	case MethodInitialize, MethodInitialized, MethodPing:
		return "core"
	case MethodToolsList, MethodToolsCall:
		return "tools"
	case MethodResourcesList, MethodResourcesRead, MethodResourcesTemplates,
		MethodResourcesSubscribe, MethodResourcesUnsubscribe:
		return "resources"
	case MethodPromptsList, MethodPromptsGet:
		return "prompts"
	case MethodLoggingSetLevel:
		return "logging"
	case MethodCompletionComplete:
		return "completion"
	default:
		return "unknown"
	}
}
