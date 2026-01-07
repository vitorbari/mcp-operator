package mcp

import (
	"encoding/json"
	"errors"
)

// Common parsing errors.
var (
	ErrEmptyBody     = errors.New("empty body")
	ErrInvalidJSON   = errors.New("invalid JSON")
	ErrMissingMethod = errors.New("missing method field")
)

// ParsedRequest contains the extracted information from a JSON-RPC request.
type ParsedRequest struct {
	// Method is the JSON-RPC method name.
	Method string

	// ID is the request identifier (nil for notifications).
	ID interface{}

	// IsNotification is true if the request has no ID (is a notification).
	IsNotification bool

	// ToolName is the tool name extracted from tools/call params.
	ToolName string

	// ResourceURI is the resource URI extracted from resources/read params.
	ResourceURI string
}

// ParsedResponse contains the extracted information from a JSON-RPC response.
type ParsedResponse struct {
	// ID is the response identifier.
	ID interface{}

	// IsError is true if the response contains an error.
	IsError bool

	// ErrorCode is the error code if IsError is true.
	ErrorCode int

	// ErrorMessage is the error message if IsError is true.
	ErrorMessage string
}

// ParseRequest parses a JSON-RPC request body and extracts relevant information.
func ParseRequest(body []byte) (*ParsedRequest, error) {
	if len(body) == 0 {
		return nil, ErrEmptyBody
	}

	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, ErrInvalidJSON
	}

	if req.Method == "" {
		return nil, ErrMissingMethod
	}

	parsed := &ParsedRequest{
		Method:         req.Method,
		ID:             req.ID,
		IsNotification: req.ID == nil,
	}

	// Extract additional info based on method
	switch req.Method {
	case MethodToolsCall:
		parsed.ToolName = extractToolName(req.Params)
	case MethodResourcesRead:
		parsed.ResourceURI = extractResourceURI(req.Params)
	}

	return parsed, nil
}

// ParseResponse parses a JSON-RPC response body and extracts relevant information.
func ParseResponse(body []byte) (*ParsedResponse, error) {
	if len(body) == 0 {
		return nil, ErrEmptyBody
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, ErrInvalidJSON
	}

	parsed := &ParsedResponse{
		ID:      resp.ID,
		IsError: resp.Error != nil,
	}

	if resp.Error != nil {
		parsed.ErrorCode = resp.Error.Code
		parsed.ErrorMessage = resp.Error.Message
	}

	return parsed, nil
}

// extractToolName extracts the tool name from tools/call params.
func extractToolName(params json.RawMessage) string {
	if len(params) == 0 {
		return ""
	}

	var toolParams ToolCallParams
	if err := json.Unmarshal(params, &toolParams); err != nil {
		return ""
	}

	return toolParams.Name
}

// extractResourceURI extracts the resource URI from resources/read params.
func extractResourceURI(params json.RawMessage) string {
	if len(params) == 0 {
		return ""
	}

	var resourceParams ResourceReadParams
	if err := json.Unmarshal(params, &resourceParams); err != nil {
		return ""
	}

	return resourceParams.URI
}
