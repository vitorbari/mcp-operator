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

package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mockMCPServer creates a test HTTP server that responds to MCP protocol requests
func mockMCPServer(t *testing.T, handler func(method string, params json.RawMessage) (interface{}, *RPCError)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Logf("Failed to decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Marshal params to raw JSON for handler
		paramsBytes, _ := json.Marshal(request.Params)

		// Call handler to get result or error
		result, rpcErr := handler(request.Method, paramsBytes)

		response := JSONRPCResponse{
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

func TestNewClient(t *testing.T) {
	client := NewClient("http://example.com/mcp")
	if client == nil {
		t.Fatal("NewClient returned nil")
	}
	if client.endpoint != "http://example.com/mcp" {
		t.Errorf("Expected endpoint http://example.com/mcp, got %s", client.endpoint)
	}
	if client.httpClient.Timeout != DefaultTimeout {
		t.Errorf("Expected timeout %v, got %v", DefaultTimeout, client.httpClient.Timeout)
	}
}

func TestNewClientWithTimeout(t *testing.T) {
	timeout := 10 * time.Second
	client := NewClientWithTimeout("http://example.com/mcp", timeout)
	if client.httpClient.Timeout != timeout {
		t.Errorf("Expected timeout %v, got %v", timeout, client.httpClient.Timeout)
	}
}

func TestClient_Initialize(t *testing.T) {
	server := mockMCPServer(t, func(method string, params json.RawMessage) (interface{}, *RPCError) {
		if method != "initialize" {
			return nil, &RPCError{Code: -32601, Message: "Method not found"}
		}

		// Verify params
		var initParams InitializeParams
		if err := json.Unmarshal(params, &initParams); err != nil {
			return nil, &RPCError{Code: -32602, Message: "Invalid params"}
		}

		if initParams.ProtocolVersion != DefaultProtocolVersion {
			t.Errorf("Expected protocol version %s, got %s", DefaultProtocolVersion, initParams.ProtocolVersion)
		}

		if initParams.ClientInfo.Name != "mcp-operator-validator" {
			t.Errorf("Expected client name mcp-operator-validator, got %s", initParams.ClientInfo.Name)
		}

		return InitializeResult{
			ProtocolVersion: DefaultProtocolVersion,
			Capabilities: ServerCapabilities{
				Tools: &ToolsCapability{},
				Resources: &ResourcesCapability{
					Subscribe: true,
				},
				Prompts: &PromptsCapability{},
			},
			ServerInfo: Implementation{
				Name:    "test-server",
				Version: "1.0.0",
			},
		}, nil
	})
	defer server.Close()

	client := NewClient(server.URL)
	ctx := context.Background()

	result, err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if result.ProtocolVersion != DefaultProtocolVersion {
		t.Errorf("Expected protocol version %s, got %s", DefaultProtocolVersion, result.ProtocolVersion)
	}

	if result.ServerInfo.Name != "test-server" {
		t.Errorf("Expected server name test-server, got %s", result.ServerInfo.Name)
	}

	if result.Capabilities.Tools == nil {
		t.Error("Expected tools capability to be present")
	}

	if result.Capabilities.Resources == nil {
		t.Error("Expected resources capability to be present")
	}

	if !result.Capabilities.Resources.Subscribe {
		t.Error("Expected resources.subscribe to be true")
	}
}

func TestClient_ListTools(t *testing.T) {
	server := mockMCPServer(t, func(method string, params json.RawMessage) (interface{}, *RPCError) {
		if method != "tools/list" {
			return nil, &RPCError{Code: -32601, Message: "Method not found"}
		}

		return ListToolsResult{
			Tools: []Tool{
				{
					Name:        "test-tool",
					Description: "A test tool",
					InputSchema: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"query": map[string]interface{}{
								"type": "string",
							},
						},
					},
				},
			},
		}, nil
	})
	defer server.Close()

	client := NewClient(server.URL)
	ctx := context.Background()

	result, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	if len(result.Tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(result.Tools))
	}

	if result.Tools[0].Name != "test-tool" {
		t.Errorf("Expected tool name test-tool, got %s", result.Tools[0].Name)
	}
}

func TestClient_ListResources(t *testing.T) {
	server := mockMCPServer(t, func(method string, params json.RawMessage) (interface{}, *RPCError) {
		if method != "resources/list" {
			return nil, &RPCError{Code: -32601, Message: "Method not found"}
		}

		return ListResourcesResult{
			Resources: []Resource{
				{
					URI:         "file:///test.txt",
					Name:        "test.txt",
					Description: "A test file",
					MimeType:    "text/plain",
				},
			},
		}, nil
	})
	defer server.Close()

	client := NewClient(server.URL)
	ctx := context.Background()

	result, err := client.ListResources(ctx)
	if err != nil {
		t.Fatalf("ListResources failed: %v", err)
	}

	if len(result.Resources) != 1 {
		t.Fatalf("Expected 1 resource, got %d", len(result.Resources))
	}

	if result.Resources[0].Name != "test.txt" {
		t.Errorf("Expected resource name test.txt, got %s", result.Resources[0].Name)
	}
}

func TestClient_ListPrompts(t *testing.T) {
	server := mockMCPServer(t, func(method string, params json.RawMessage) (interface{}, *RPCError) {
		if method != "prompts/list" {
			return nil, &RPCError{Code: -32601, Message: "Method not found"}
		}

		return ListPromptsResult{
			Prompts: []Prompt{
				{
					Name:        "test-prompt",
					Description: "A test prompt",
					Arguments: []Argument{
						{
							Name:        "query",
							Description: "Search query",
							Required:    true,
						},
					},
				},
			},
		}, nil
	})
	defer server.Close()

	client := NewClient(server.URL)
	ctx := context.Background()

	result, err := client.ListPrompts(ctx)
	if err != nil {
		t.Fatalf("ListPrompts failed: %v", err)
	}

	if len(result.Prompts) != 1 {
		t.Fatalf("Expected 1 prompt, got %d", len(result.Prompts))
	}

	if result.Prompts[0].Name != "test-prompt" {
		t.Errorf("Expected prompt name test-prompt, got %s", result.Prompts[0].Name)
	}

	if len(result.Prompts[0].Arguments) != 1 {
		t.Fatalf("Expected 1 argument, got %d", len(result.Prompts[0].Arguments))
	}
}

func TestClient_JSONRPCError(t *testing.T) {
	server := mockMCPServer(t, func(method string, params json.RawMessage) (interface{}, *RPCError) {
		return nil, &RPCError{
			Code:    -32601,
			Message: "Method not found",
		}
	})
	defer server.Close()

	client := NewClient(server.URL)
	ctx := context.Background()

	_, err := client.Initialize(ctx)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if err.Error() != "initialize failed: JSON-RPC error -32601: Method not found" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestClient_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	ctx := context.Background()

	_, err := client.Initialize(ctx)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}

func TestClient_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClientWithTimeout(server.URL, 100*time.Millisecond)
	ctx := context.Background()

	_, err := client.Initialize(ctx)
	if err == nil {
		t.Fatal("Expected timeout error, got nil")
	}
}

func TestClient_Ping(t *testing.T) {
	server := mockMCPServer(t, func(method string, params json.RawMessage) (interface{}, *RPCError) {
		if method != "initialize" {
			return nil, &RPCError{Code: -32601, Message: "Method not found"}
		}

		return InitializeResult{
			ProtocolVersion: DefaultProtocolVersion,
			ServerInfo: Implementation{
				Name:    "test-server",
				Version: "1.0.0",
			},
		}, nil
	})
	defer server.Close()

	client := NewClient(server.URL)
	ctx := context.Background()

	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func TestClient_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	ctx := context.Background()

	_, err := client.Initialize(ctx)
	if err == nil {
		t.Fatal("Expected JSON parsing error, got nil")
	}
}
