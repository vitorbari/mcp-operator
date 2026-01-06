package mcp

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestParseRequest_SingleRequest(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		wantMethod     string
		wantIsBatch    bool
		wantBatchSize  int
		wantNotify     bool
		wantErr        bool
	}{
		{
			name: "initialize request",
			body: `{
				"jsonrpc": "2.0",
				"method": "initialize",
				"params": {"protocolVersion": "2024-11-05"},
				"id": 1
			}`,
			wantMethod:    MethodInitialize,
			wantIsBatch:   false,
			wantBatchSize: 1,
			wantNotify:    false,
			wantErr:       false,
		},
		{
			name: "tools/list request",
			body: `{
				"jsonrpc": "2.0",
				"method": "tools/list",
				"id": 2
			}`,
			wantMethod:    MethodToolsList,
			wantIsBatch:   false,
			wantBatchSize: 1,
			wantNotify:    false,
			wantErr:       false,
		},
		{
			name: "notification (no id)",
			body: `{
				"jsonrpc": "2.0",
				"method": "notifications/initialized"
			}`,
			wantMethod:    MethodInitialized,
			wantIsBatch:   false,
			wantBatchSize: 1,
			wantNotify:    true,
			wantErr:       false,
		},
		{
			name: "string id",
			body: `{
				"jsonrpc": "2.0",
				"method": "ping",
				"id": "request-123"
			}`,
			wantMethod:    MethodPing,
			wantIsBatch:   false,
			wantBatchSize: 1,
			wantNotify:    false,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseRequest([]byte(tt.body))

			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			if result.Method != tt.wantMethod {
				t.Errorf("Method = %v, want %v", result.Method, tt.wantMethod)
			}

			if result.IsBatch != tt.wantIsBatch {
				t.Errorf("IsBatch = %v, want %v", result.IsBatch, tt.wantIsBatch)
			}

			if result.BatchSize != tt.wantBatchSize {
				t.Errorf("BatchSize = %v, want %v", result.BatchSize, tt.wantBatchSize)
			}

			if result.IsNotification != tt.wantNotify {
				t.Errorf("IsNotification = %v, want %v", result.IsNotification, tt.wantNotify)
			}
		})
	}
}

func TestParseRequest_ToolsCall(t *testing.T) {
	body := `{
		"jsonrpc": "2.0",
		"method": "tools/call",
		"params": {
			"name": "get_weather",
			"arguments": {
				"location": "San Francisco"
			}
		},
		"id": 1
	}`

	result, err := ParseRequest([]byte(body))
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}

	if result.Method != MethodToolsCall {
		t.Errorf("Method = %v, want %v", result.Method, MethodToolsCall)
	}

	if result.ToolName != "get_weather" {
		t.Errorf("ToolName = %v, want %v", result.ToolName, "get_weather")
	}
}

func TestParseRequest_ResourcesRead(t *testing.T) {
	body := `{
		"jsonrpc": "2.0",
		"method": "resources/read",
		"params": {
			"uri": "file:///path/to/document.txt"
		},
		"id": 1
	}`

	result, err := ParseRequest([]byte(body))
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}

	if result.Method != MethodResourcesRead {
		t.Errorf("Method = %v, want %v", result.Method, MethodResourcesRead)
	}

	if result.ResourceURI != "file:///path/to/document.txt" {
		t.Errorf("ResourceURI = %v, want %v", result.ResourceURI, "file:///path/to/document.txt")
	}
}

func TestParseRequest_PromptsGet(t *testing.T) {
	body := `{
		"jsonrpc": "2.0",
		"method": "prompts/get",
		"params": {
			"name": "code_review",
			"arguments": {
				"language": "go"
			}
		},
		"id": 1
	}`

	result, err := ParseRequest([]byte(body))
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}

	if result.Method != MethodPromptsGet {
		t.Errorf("Method = %v, want %v", result.Method, MethodPromptsGet)
	}

	if result.PromptName != "code_review" {
		t.Errorf("PromptName = %v, want %v", result.PromptName, "code_review")
	}
}

func TestParseRequest_BatchRequest(t *testing.T) {
	body := `[
		{"jsonrpc": "2.0", "method": "tools/list", "id": 1},
		{"jsonrpc": "2.0", "method": "resources/list", "id": 2},
		{"jsonrpc": "2.0", "method": "prompts/list", "id": 3}
	]`

	result, err := ParseRequest([]byte(body))
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}

	if !result.IsBatch {
		t.Error("IsBatch should be true")
	}

	if result.BatchSize != 3 {
		t.Errorf("BatchSize = %v, want %v", result.BatchSize, 3)
	}

	if len(result.Requests) != 3 {
		t.Errorf("len(Requests) = %v, want %v", len(result.Requests), 3)
	}

	// Check first request method
	if result.Method != MethodToolsList {
		t.Errorf("Method = %v, want %v", result.Method, MethodToolsList)
	}
}

func TestParseRequest_Errors(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr error
	}{
		{
			name:    "empty body",
			body:    "",
			wantErr: ErrEmptyBody,
		},
		{
			name:    "whitespace only",
			body:    "   \n\t  ",
			wantErr: ErrEmptyBody,
		},
		{
			name:    "invalid JSON",
			body:    `{"jsonrpc": "2.0", "method":`,
			wantErr: ErrInvalidJSON,
		},
		{
			name:    "missing jsonrpc version",
			body:    `{"method": "initialize", "id": 1}`,
			wantErr: ErrInvalidFormat,
		},
		{
			name:    "wrong jsonrpc version",
			body:    `{"jsonrpc": "1.0", "method": "initialize", "id": 1}`,
			wantErr: ErrInvalidFormat,
		},
		{
			name:    "missing method",
			body:    `{"jsonrpc": "2.0", "id": 1}`,
			wantErr: ErrInvalidFormat,
		},
		{
			name:    "empty batch",
			body:    `[]`,
			wantErr: ErrInvalidFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRequest([]byte(tt.body))

			if err == nil {
				t.Error("ParseRequest() expected error, got nil")
				return
			}

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ParseRequest() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseResponse_Success(t *testing.T) {
	body := `{
		"jsonrpc": "2.0",
		"result": {"tools": []},
		"id": 1
	}`

	result, err := ParseResponse([]byte(body))
	if err != nil {
		t.Fatalf("ParseResponse() error = %v", err)
	}

	if result.IsError {
		t.Error("IsError should be false")
	}

	if result.IsBatch {
		t.Error("IsBatch should be false")
	}

	if result.BatchSize != 1 {
		t.Errorf("BatchSize = %v, want %v", result.BatchSize, 1)
	}
}

func TestParseResponse_Error(t *testing.T) {
	body := `{
		"jsonrpc": "2.0",
		"error": {
			"code": -32601,
			"message": "Method not found"
		},
		"id": 1
	}`

	result, err := ParseResponse([]byte(body))
	if err != nil {
		t.Fatalf("ParseResponse() error = %v", err)
	}

	if !result.IsError {
		t.Error("IsError should be true")
	}

	if result.ErrorCode != MethodNotFound {
		t.Errorf("ErrorCode = %v, want %v", result.ErrorCode, MethodNotFound)
	}

	if result.ErrorMessage != "Method not found" {
		t.Errorf("ErrorMessage = %v, want %v", result.ErrorMessage, "Method not found")
	}
}

func TestParseResponse_BatchResponse(t *testing.T) {
	body := `[
		{"jsonrpc": "2.0", "result": {"tools": []}, "id": 1},
		{"jsonrpc": "2.0", "result": {"resources": []}, "id": 2}
	]`

	result, err := ParseResponse([]byte(body))
	if err != nil {
		t.Fatalf("ParseResponse() error = %v", err)
	}

	if !result.IsBatch {
		t.Error("IsBatch should be true")
	}

	if result.BatchSize != 2 {
		t.Errorf("BatchSize = %v, want %v", result.BatchSize, 2)
	}

	if len(result.Responses) != 2 {
		t.Errorf("len(Responses) = %v, want %v", len(result.Responses), 2)
	}
}

func TestParseResponse_Errors(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr error
	}{
		{
			name:    "empty body",
			body:    "",
			wantErr: ErrEmptyBody,
		},
		{
			name:    "invalid JSON",
			body:    `{"jsonrpc": "2.0", "result":`,
			wantErr: ErrInvalidJSON,
		},
		{
			name:    "empty batch",
			body:    `[]`,
			wantErr: ErrInvalidFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseResponse([]byte(tt.body))

			if err == nil {
				t.Error("ParseResponse() expected error, got nil")
				return
			}

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ParseResponse() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsMCPMethod(t *testing.T) {
	tests := []struct {
		method string
		want   bool
	}{
		{MethodInitialize, true},
		{MethodToolsList, true},
		{MethodToolsCall, true},
		{MethodResourcesList, true},
		{MethodResourcesRead, true},
		{MethodPromptsList, true},
		{MethodPromptsGet, true},
		{MethodPing, true},
		{"custom/method", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			if got := IsMCPMethod(tt.method); got != tt.want {
				t.Errorf("IsMCPMethod(%q) = %v, want %v", tt.method, got, tt.want)
			}
		})
	}
}

func TestMethodCategory(t *testing.T) {
	tests := []struct {
		method string
		want   string
	}{
		{MethodInitialize, "core"},
		{MethodPing, "core"},
		{MethodToolsList, "tools"},
		{MethodToolsCall, "tools"},
		{MethodResourcesList, "resources"},
		{MethodResourcesRead, "resources"},
		{MethodPromptsList, "prompts"},
		{MethodPromptsGet, "prompts"},
		{MethodLoggingSetLevel, "logging"},
		{MethodCompletionComplete, "completion"},
		{"custom/method", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			if got := MethodCategory(tt.method); got != tt.want {
				t.Errorf("MethodCategory(%q) = %v, want %v", tt.method, got, tt.want)
			}
		})
	}
}

func TestJSONRPCID_Unmarshal(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantString string
		wantNumber float64
		wantIsStr  bool
		wantIsNull bool
	}{
		{
			name:       "string id",
			input:      `"request-123"`,
			wantString: "request-123",
			wantIsStr:  true,
		},
		{
			name:       "number id",
			input:      `42`,
			wantNumber: 42,
			wantIsStr:  false,
		},
		{
			name:       "float number id",
			input:      `3.14`,
			wantNumber: 3.14,
			wantIsStr:  false,
		},
		{
			name:       "null id",
			input:      `null`,
			wantIsNull: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var id JSONRPCID
			err := json.Unmarshal([]byte(tt.input), &id)
			if err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			if id.IsNull != tt.wantIsNull {
				t.Errorf("IsNull = %v, want %v", id.IsNull, tt.wantIsNull)
			}

			if tt.wantIsNull {
				return
			}

			if id.IsString != tt.wantIsStr {
				t.Errorf("IsString = %v, want %v", id.IsString, tt.wantIsStr)
			}

			if tt.wantIsStr && id.String != tt.wantString {
				t.Errorf("String = %v, want %v", id.String, tt.wantString)
			}

			if !tt.wantIsStr && id.Number != tt.wantNumber {
				t.Errorf("Number = %v, want %v", id.Number, tt.wantNumber)
			}
		})
	}
}

func TestJSONRPCID_Marshal(t *testing.T) {
	tests := []struct {
		name string
		id   JSONRPCID
		want string
	}{
		{
			name: "string id",
			id:   JSONRPCID{String: "test-123", IsString: true},
			want: `"test-123"`,
		},
		{
			name: "number id",
			id:   JSONRPCID{Number: 42, IsString: false},
			want: `42`,
		},
		{
			name: "null id",
			id:   JSONRPCID{IsNull: true},
			want: `null`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.id)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}

			if string(data) != tt.want {
				t.Errorf("Marshal() = %v, want %v", string(data), tt.want)
			}
		})
	}
}

// Test with real MCP spec examples
func TestParseRequest_MCPSpecExamples(t *testing.T) {
	t.Run("initialize from spec", func(t *testing.T) {
		body := `{
			"jsonrpc": "2.0",
			"id": 1,
			"method": "initialize",
			"params": {
				"protocolVersion": "2024-11-05",
				"capabilities": {
					"roots": {
						"listChanged": true
					},
					"sampling": {}
				},
				"clientInfo": {
					"name": "ExampleClient",
					"version": "1.0.0"
				}
			}
		}`

		result, err := ParseRequest([]byte(body))
		if err != nil {
			t.Fatalf("ParseRequest() error = %v", err)
		}

		if result.Method != MethodInitialize {
			t.Errorf("Method = %v, want %v", result.Method, MethodInitialize)
		}

		if result.IsNotification {
			t.Error("initialize should not be a notification")
		}
	})

	t.Run("tools/call from spec", func(t *testing.T) {
		body := `{
			"jsonrpc": "2.0",
			"id": 2,
			"method": "tools/call",
			"params": {
				"name": "get_weather",
				"arguments": {
					"location": "New York"
				}
			}
		}`

		result, err := ParseRequest([]byte(body))
		if err != nil {
			t.Fatalf("ParseRequest() error = %v", err)
		}

		if result.Method != MethodToolsCall {
			t.Errorf("Method = %v, want %v", result.Method, MethodToolsCall)
		}

		if result.ToolName != "get_weather" {
			t.Errorf("ToolName = %v, want %v", result.ToolName, "get_weather")
		}
	})

	t.Run("notification from spec", func(t *testing.T) {
		body := `{
			"jsonrpc": "2.0",
			"method": "notifications/initialized"
		}`

		result, err := ParseRequest([]byte(body))
		if err != nil {
			t.Fatalf("ParseRequest() error = %v", err)
		}

		if result.Method != MethodInitialized {
			t.Errorf("Method = %v, want %v", result.Method, MethodInitialized)
		}

		if !result.IsNotification {
			t.Error("notifications/initialized should be a notification")
		}
	})
}

func TestParseResponse_MCPSpecExamples(t *testing.T) {
	t.Run("initialize response", func(t *testing.T) {
		body := `{
			"jsonrpc": "2.0",
			"id": 1,
			"result": {
				"protocolVersion": "2024-11-05",
				"capabilities": {
					"logging": {},
					"prompts": {
						"listChanged": true
					},
					"resources": {
						"subscribe": true,
						"listChanged": true
					},
					"tools": {
						"listChanged": true
					}
				},
				"serverInfo": {
					"name": "ExampleServer",
					"version": "1.0.0"
				}
			}
		}`

		result, err := ParseResponse([]byte(body))
		if err != nil {
			t.Fatalf("ParseResponse() error = %v", err)
		}

		if result.IsError {
			t.Error("IsError should be false for success response")
		}
	})

	t.Run("error response", func(t *testing.T) {
		body := `{
			"jsonrpc": "2.0",
			"id": 1,
			"error": {
				"code": -32602,
				"message": "Invalid params",
				"data": {"details": "Missing required field 'name'"}
			}
		}`

		result, err := ParseResponse([]byte(body))
		if err != nil {
			t.Fatalf("ParseResponse() error = %v", err)
		}

		if !result.IsError {
			t.Error("IsError should be true for error response")
		}

		if result.ErrorCode != InvalidParams {
			t.Errorf("ErrorCode = %v, want %v", result.ErrorCode, InvalidParams)
		}
	})
}
