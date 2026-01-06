// Package mcp provides MCP (Model Context Protocol) JSON-RPC message parsing.
package mcp

import "encoding/json"

// JSON-RPC 2.0 version string.
const JSONRPCVersion = "2.0"

// MCP method constants for common operations.
const (
	// Core protocol methods
	MethodInitialize = "initialize"
	MethodInitialized = "notifications/initialized"
	MethodPing       = "ping"

	// Tools methods
	MethodToolsList = "tools/list"
	MethodToolsCall = "tools/call"

	// Resources methods
	MethodResourcesList     = "resources/list"
	MethodResourcesRead     = "resources/read"
	MethodResourcesTemplates = "resources/templates/list"
	MethodResourcesSubscribe = "resources/subscribe"
	MethodResourcesUnsubscribe = "resources/unsubscribe"

	// Prompts methods
	MethodPromptsList = "prompts/list"
	MethodPromptsGet  = "prompts/get"

	// Logging methods
	MethodLoggingSetLevel = "logging/setLevel"

	// Completion methods
	MethodCompletionComplete = "completion/complete"
)

// JSONRPCID represents a JSON-RPC request/response ID.
// It can be a string, number, or null.
type JSONRPCID struct {
	String  string
	Number  float64
	IsString bool
	IsNull  bool
}

// UnmarshalJSON implements json.Unmarshaler for JSONRPCID.
func (id *JSONRPCID) UnmarshalJSON(data []byte) error {
	// Check for null
	if string(data) == "null" {
		id.IsNull = true
		return nil
	}

	// Try string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		id.String = s
		id.IsString = true
		return nil
	}

	// Try number
	var n float64
	if err := json.Unmarshal(data, &n); err == nil {
		id.Number = n
		id.IsString = false
		return nil
	}

	return nil
}

// MarshalJSON implements json.Marshaler for JSONRPCID.
func (id JSONRPCID) MarshalJSON() ([]byte, error) {
	if id.IsNull {
		return []byte("null"), nil
	}
	if id.IsString {
		return json.Marshal(id.String)
	}
	return json.Marshal(id.Number)
}

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	// JSONRPC specifies the JSON-RPC protocol version (must be "2.0").
	JSONRPC string `json:"jsonrpc"`

	// Method is the name of the method to be invoked.
	Method string `json:"method"`

	// Params holds the parameter values to be used during invocation.
	// Can be an object or array.
	Params json.RawMessage `json:"params,omitempty"`

	// ID is the request identifier. If omitted, this is a notification.
	ID *JSONRPCID `json:"id,omitempty"`
}

// IsNotification returns true if this request is a notification (no ID).
func (r *JSONRPCRequest) IsNotification() bool {
	return r.ID == nil
}

// JSONRPCError represents a JSON-RPC 2.0 error object.
type JSONRPCError struct {
	// Code is a number indicating the error type.
	Code int `json:"code"`

	// Message is a short description of the error.
	Message string `json:"message"`

	// Data contains additional information about the error.
	Data json.RawMessage `json:"data,omitempty"`
}

// Standard JSON-RPC 2.0 error codes.
const (
	// ParseError indicates invalid JSON was received.
	ParseError = -32700

	// InvalidRequest indicates the JSON sent is not a valid Request object.
	InvalidRequest = -32600

	// MethodNotFound indicates the method does not exist or is not available.
	MethodNotFound = -32601

	// InvalidParams indicates invalid method parameter(s).
	InvalidParams = -32602

	// InternalError indicates an internal JSON-RPC error.
	InternalError = -32603
)

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	// JSONRPC specifies the JSON-RPC protocol version (must be "2.0").
	JSONRPC string `json:"jsonrpc"`

	// Result contains the result of the method invocation.
	// This member is required on success and must not exist on error.
	Result json.RawMessage `json:"result,omitempty"`

	// Error contains the error object if there was an error.
	// This member is required on error and must not exist on success.
	Error *JSONRPCError `json:"error,omitempty"`

	// ID is the request identifier that this response corresponds to.
	ID *JSONRPCID `json:"id"`
}

// IsError returns true if this response contains an error.
func (r *JSONRPCResponse) IsError() bool {
	return r.Error != nil
}

// ToolsCallParams represents the parameters for a tools/call request.
type ToolsCallParams struct {
	// Name is the name of the tool to call.
	Name string `json:"name"`

	// Arguments contains the arguments to pass to the tool.
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// ResourcesReadParams represents the parameters for a resources/read request.
type ResourcesReadParams struct {
	// URI is the URI of the resource to read.
	URI string `json:"uri"`
}

// PromptsGetParams represents the parameters for a prompts/get request.
type PromptsGetParams struct {
	// Name is the name of the prompt to get.
	Name string `json:"name"`

	// Arguments contains the arguments for the prompt template.
	Arguments map[string]string `json:"arguments,omitempty"`
}
