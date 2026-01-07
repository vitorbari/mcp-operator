package mcp

import (
	"testing"
)

func TestParseRequest_Initialize(t *testing.T) {
	body := []byte(`{
		"jsonrpc": "2.0",
		"method": "initialize",
		"params": {"protocolVersion": "2024-11-05"},
		"id": 1
	}`)

	parsed, err := ParseRequest(body)
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	if parsed.Method != MethodInitialize {
		t.Errorf("Method = %q, want %q", parsed.Method, MethodInitialize)
	}

	if parsed.IsNotification {
		t.Error("Expected IsNotification to be false")
	}

	// ID should be float64 after JSON unmarshaling
	if parsed.ID.(float64) != 1 {
		t.Errorf("ID = %v, want 1", parsed.ID)
	}
}

func TestParseRequest_ToolsList(t *testing.T) {
	body := []byte(`{
		"jsonrpc": "2.0",
		"method": "tools/list",
		"id": "abc-123"
	}`)

	parsed, err := ParseRequest(body)
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	if parsed.Method != MethodToolsList {
		t.Errorf("Method = %q, want %q", parsed.Method, MethodToolsList)
	}

	if parsed.ID.(string) != "abc-123" {
		t.Errorf("ID = %v, want 'abc-123'", parsed.ID)
	}
}

func TestParseRequest_ToolsCall(t *testing.T) {
	body := []byte(`{
		"jsonrpc": "2.0",
		"method": "tools/call",
		"params": {
			"name": "get_weather",
			"arguments": {"location": "San Francisco"}
		},
		"id": 42
	}`)

	parsed, err := ParseRequest(body)
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	if parsed.Method != MethodToolsCall {
		t.Errorf("Method = %q, want %q", parsed.Method, MethodToolsCall)
	}

	if parsed.ToolName != "get_weather" {
		t.Errorf("ToolName = %q, want 'get_weather'", parsed.ToolName)
	}
}

func TestParseRequest_ResourcesRead(t *testing.T) {
	body := []byte(`{
		"jsonrpc": "2.0",
		"method": "resources/read",
		"params": {
			"uri": "file:///path/to/document.txt"
		},
		"id": 1
	}`)

	parsed, err := ParseRequest(body)
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	if parsed.Method != MethodResourcesRead {
		t.Errorf("Method = %q, want %q", parsed.Method, MethodResourcesRead)
	}

	if parsed.ResourceURI != "file:///path/to/document.txt" {
		t.Errorf("ResourceURI = %q, want 'file:///path/to/document.txt'", parsed.ResourceURI)
	}
}

func TestParseRequest_Notification(t *testing.T) {
	body := []byte(`{
		"jsonrpc": "2.0",
		"method": "notifications/cancelled",
		"params": {"requestId": 123}
	}`)

	parsed, err := ParseRequest(body)
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	if !parsed.IsNotification {
		t.Error("Expected IsNotification to be true")
	}

	if parsed.ID != nil {
		t.Errorf("ID = %v, want nil", parsed.ID)
	}
}

func TestParseRequest_EmptyBody(t *testing.T) {
	_, err := ParseRequest([]byte{})
	if err != ErrEmptyBody {
		t.Errorf("Expected ErrEmptyBody, got %v", err)
	}
}

func TestParseRequest_InvalidJSON(t *testing.T) {
	body := []byte(`{not valid json}`)
	_, err := ParseRequest(body)
	if err != ErrInvalidJSON {
		t.Errorf("Expected ErrInvalidJSON, got %v", err)
	}
}

func TestParseRequest_MissingMethod(t *testing.T) {
	body := []byte(`{"jsonrpc": "2.0", "id": 1}`)
	_, err := ParseRequest(body)
	if err != ErrMissingMethod {
		t.Errorf("Expected ErrMissingMethod, got %v", err)
	}
}

func TestParseRequest_ToolsCallMissingParams(t *testing.T) {
	body := []byte(`{
		"jsonrpc": "2.0",
		"method": "tools/call",
		"id": 1
	}`)

	parsed, err := ParseRequest(body)
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	if parsed.ToolName != "" {
		t.Errorf("ToolName = %q, want empty string", parsed.ToolName)
	}
}

func TestParseRequest_ToolsCallInvalidParams(t *testing.T) {
	body := []byte(`{
		"jsonrpc": "2.0",
		"method": "tools/call",
		"params": "invalid",
		"id": 1
	}`)

	parsed, err := ParseRequest(body)
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	// Should not fail, just return empty tool name
	if parsed.ToolName != "" {
		t.Errorf("ToolName = %q, want empty string", parsed.ToolName)
	}
}

func TestParseResponse_Success(t *testing.T) {
	body := []byte(`{
		"jsonrpc": "2.0",
		"result": {"tools": []},
		"id": 1
	}`)

	parsed, err := ParseResponse(body)
	if err != nil {
		t.Fatalf("ParseResponse failed: %v", err)
	}

	if parsed.IsError {
		t.Error("Expected IsError to be false")
	}

	if parsed.ID.(float64) != 1 {
		t.Errorf("ID = %v, want 1", parsed.ID)
	}
}

func TestParseResponse_Error(t *testing.T) {
	body := []byte(`{
		"jsonrpc": "2.0",
		"error": {
			"code": -32600,
			"message": "Invalid Request"
		},
		"id": 1
	}`)

	parsed, err := ParseResponse(body)
	if err != nil {
		t.Fatalf("ParseResponse failed: %v", err)
	}

	if !parsed.IsError {
		t.Error("Expected IsError to be true")
	}

	if parsed.ErrorCode != -32600 {
		t.Errorf("ErrorCode = %d, want -32600", parsed.ErrorCode)
	}

	if parsed.ErrorMessage != "Invalid Request" {
		t.Errorf("ErrorMessage = %q, want 'Invalid Request'", parsed.ErrorMessage)
	}
}

func TestParseResponse_EmptyBody(t *testing.T) {
	_, err := ParseResponse([]byte{})
	if err != ErrEmptyBody {
		t.Errorf("Expected ErrEmptyBody, got %v", err)
	}
}

func TestParseResponse_InvalidJSON(t *testing.T) {
	body := []byte(`{not valid json}`)
	_, err := ParseResponse(body)
	if err != ErrInvalidJSON {
		t.Errorf("Expected ErrInvalidJSON, got %v", err)
	}
}

func TestParseResponse_WithStringID(t *testing.T) {
	body := []byte(`{
		"jsonrpc": "2.0",
		"result": {},
		"id": "request-abc"
	}`)

	parsed, err := ParseResponse(body)
	if err != nil {
		t.Fatalf("ParseResponse failed: %v", err)
	}

	if parsed.ID.(string) != "request-abc" {
		t.Errorf("ID = %v, want 'request-abc'", parsed.ID)
	}
}

func TestParseResponse_NullID(t *testing.T) {
	body := []byte(`{
		"jsonrpc": "2.0",
		"error": {"code": -32700, "message": "Parse error"},
		"id": null
	}`)

	parsed, err := ParseResponse(body)
	if err != nil {
		t.Fatalf("ParseResponse failed: %v", err)
	}

	if parsed.ID != nil {
		t.Errorf("ID = %v, want nil", parsed.ID)
	}

	if parsed.ErrorCode != -32700 {
		t.Errorf("ErrorCode = %d, want -32700", parsed.ErrorCode)
	}
}
