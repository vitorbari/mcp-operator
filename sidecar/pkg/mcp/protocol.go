// Package mcp provides types and utilities for parsing MCP (Model Context Protocol) messages.
package mcp

import "encoding/json"

// JSON-RPC method constants for common MCP operations.
const (
	MethodInitialize    = "initialize"
	MethodToolsList     = "tools/list"
	MethodToolsCall     = "tools/call"
	MethodResourcesList = "resources/list"
	MethodResourcesRead = "resources/read"
	MethodPromptsList   = "prompts/list"
	MethodPromptsGet    = "prompts/get"
)

// JSONRPCRequest represents a JSON-RPC 2.0 request message.
type JSONRPCRequest struct {
	// JSONRPC specifies the version of the JSON-RPC protocol (always "2.0").
	JSONRPC string `json:"jsonrpc"`

	// Method is the name of the method to be invoked.
	Method string `json:"method"`

	// Params holds the parameter values to be used during the invocation.
	Params json.RawMessage `json:"params,omitempty"`

	// ID is the request identifier. Can be string, number, or null.
	// If omitted, the request is a notification.
	ID interface{} `json:"id,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response message.
type JSONRPCResponse struct {
	// JSONRPC specifies the version of the JSON-RPC protocol (always "2.0").
	JSONRPC string `json:"jsonrpc"`

	// Result contains the result of the method invocation (mutually exclusive with Error).
	Result json.RawMessage `json:"result,omitempty"`

	// Error contains the error object if an error occurred (mutually exclusive with Result).
	Error *JSONRPCError `json:"error,omitempty"`

	// ID is the request identifier that this response corresponds to.
	ID interface{} `json:"id,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error object.
type JSONRPCError struct {
	// Code is a number indicating the error type.
	Code int `json:"code"`

	// Message is a short description of the error.
	Message string `json:"message"`

	// Data contains additional information about the error.
	Data interface{} `json:"data,omitempty"`
}

// ToolCallParams represents the parameters for a tools/call request.
type ToolCallParams struct {
	// Name is the name of the tool to call.
	Name string `json:"name"`

	// Arguments contains the arguments to pass to the tool.
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// ResourceReadParams represents the parameters for a resources/read request.
type ResourceReadParams struct {
	// URI is the URI of the resource to read.
	URI string `json:"uri"`
}
