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
func mockMCPServer(
	t *testing.T,
	handler func(method string, params json.RawMessage) (interface{}, *RPCError),
) *httptest.Server {
	return mockMCPServerWithNotifications(t, handler, nil)
}

// mockMCPServerWithNotifications creates a test HTTP server that handles both requests and notifications
func mockMCPServerWithNotifications(
	t *testing.T,
	handler func(method string, params json.RawMessage) (interface{}, *RPCError),
	notificationHandler func(method string),
) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse as generic JSON to check if it has an ID (request) or not (notification)
		var generic map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&generic); err != nil {
			t.Logf("Failed to decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		method, _ := generic["method"].(string)

		// Check if this is a notification (no ID field)
		if _, hasID := generic["id"]; !hasID {
			// This is a notification
			if notificationHandler != nil {
				notificationHandler(method)
			}
			// Notifications respond with 202 Accepted
			w.WriteHeader(http.StatusAccepted)
			return
		}

		// This is a request - handle normally
		var request JSONRPCRequest
		genericBytes, _ := json.Marshal(generic)
		if err := json.Unmarshal(genericBytes, &request); err != nil {
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
	client := NewClient("http://example.com/mcp", WithTimeout(timeout))
	if client.httpClient.Timeout != timeout {
		t.Errorf("Expected timeout %v, got %v", timeout, client.httpClient.Timeout)
	}
}

func TestNewClient_MultipleOptions(t *testing.T) {
	timeout := 45 * time.Second
	client := NewClient("http://example.com/mcp",
		WithTimeout(timeout),
	)

	if client.endpoint != "http://example.com/mcp" {
		t.Errorf("Expected endpoint http://example.com/mcp, got %s", client.endpoint)
	}
	if client.httpClient.Timeout != timeout {
		t.Errorf("Expected timeout %v, got %v", timeout, client.httpClient.Timeout)
	}
}

func TestNewClientWithBearerToken(t *testing.T) {
	token := "test-token-123"
	client := NewClient("http://example.com/mcp", WithBearerToken(token))

	expectedAuth := "Bearer " + token
	if client.customHeaders["Authorization"] != expectedAuth {
		t.Errorf("Expected Authorization header %q, got %q", expectedAuth, client.customHeaders["Authorization"])
	}
}

func TestNewClientWithHeaders(t *testing.T) {
	headers := map[string]string{
		"X-API-Key":       "key123",
		"X-Custom-Header": "value",
	}
	client := NewClient("http://example.com/mcp", WithHeaders(headers))

	for key, expectedValue := range headers {
		if client.customHeaders[key] != expectedValue {
			t.Errorf("Expected header %s=%q, got %q", key, expectedValue, client.customHeaders[key])
		}
	}
}

func TestNewClient_WithBearerTokenAndHeaders(t *testing.T) {
	token := "test-token"
	headers := map[string]string{
		"X-Custom": "value",
	}

	client := NewClient("http://example.com/mcp",
		WithBearerToken(token),
		WithHeaders(headers),
	)

	// Both bearer token and custom headers should be set
	if client.customHeaders["Authorization"] != "Bearer "+token {
		t.Errorf("Bearer token not set correctly")
	}
	if client.customHeaders["X-Custom"] != "value" {
		t.Errorf("Custom header not set correctly")
	}
}

func TestNewClientWithClientInfo(t *testing.T) {
	client := NewClient("http://example.com/mcp",
		WithClientInfo("my-app", "1.2.3"))

	if client.clientInfo == nil {
		t.Fatal("clientInfo is nil")
	}
	if client.clientInfo.Name != "my-app" {
		t.Errorf("Expected client name %q, got %q", "my-app", client.clientInfo.Name)
	}
	if client.clientInfo.Version != "1.2.3" {
		t.Errorf("Expected client version %q, got %q", "1.2.3", client.clientInfo.Version)
	}
}

func TestClient_InitializeWithCustomClientInfo(t *testing.T) {
	var receivedClientInfo Implementation

	server := mockMCPServerWithNotifications(
		t,
		func(method string, params json.RawMessage) (interface{}, *RPCError) {
			if method != "initialize" {
				return nil, &RPCError{Code: -32601, Message: "Method not found"}
			}

			// Capture the client info from params
			var initParams InitializeParams
			if err := json.Unmarshal(params, &initParams); err != nil {
				return nil, &RPCError{Code: -32602, Message: "Invalid params"}
			}
			receivedClientInfo = initParams.ClientInfo

			return &InitializeResult{
				ProtocolVersion: DefaultProtocolVersion,
				ServerInfo: Implementation{
					Name:    "test-server",
					Version: "1.0.0",
				},
			}, nil
		},
		func(method string) {},
	)
	defer server.Close()

	client := NewClient(server.URL,
		WithClientInfo("custom-client", "2.0.0"))

	_, err := client.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Verify custom client info was sent
	if receivedClientInfo.Name != "custom-client" {
		t.Errorf("Expected client name %q, got %q", "custom-client", receivedClientInfo.Name)
	}
	if receivedClientInfo.Version != "2.0.0" {
		t.Errorf("Expected client version %q, got %q", "2.0.0", receivedClientInfo.Version)
	}
}

func TestClient_InitializeWithDefaultClientInfo(t *testing.T) {
	var receivedClientInfo Implementation

	server := mockMCPServerWithNotifications(
		t,
		func(method string, params json.RawMessage) (interface{}, *RPCError) {
			if method != "initialize" {
				return nil, &RPCError{Code: -32601, Message: "Method not found"}
			}

			var initParams InitializeParams
			if err := json.Unmarshal(params, &initParams); err != nil {
				return nil, &RPCError{Code: -32602, Message: "Invalid params"}
			}
			receivedClientInfo = initParams.ClientInfo

			return &InitializeResult{
				ProtocolVersion: DefaultProtocolVersion,
				ServerInfo: Implementation{
					Name:    "test-server",
					Version: "1.0.0",
				},
			}, nil
		},
		func(method string) {},
	)
	defer server.Close()

	// Create client without WithClientInfo - should use defaults
	client := NewClient(server.URL)

	_, err := client.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Verify default client info was sent
	if receivedClientInfo.Name != "go-mcp-client" {
		t.Errorf("Expected default client name %q, got %q", "go-mcp-client", receivedClientInfo.Name)
	}
	if receivedClientInfo.Version != "1.0.0" {
		t.Errorf("Expected default client version %q, got %q", "1.0.0", receivedClientInfo.Version)
	}
}

func TestClient_CustomHeadersSentInRequests(t *testing.T) {
	var receivedAuthHeader string
	var receivedCustomHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture headers
		receivedAuthHeader = r.Header.Get("Authorization")
		receivedCustomHeader = r.Header.Get("X-Custom")

		// Parse request to get method
		var request map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Logf("Failed to decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		method := request["method"].(string)

		// Handle initialize request (has ID)
		if method == MethodInitialize {
			requestID := int(request["id"].(float64))
			response := JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      requestID,
				Result: &InitializeResult{
					ProtocolVersion: DefaultProtocolVersion,
					ServerInfo: Implementation{
						Name:    "test-server",
						Version: "1.0.0",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			err := json.NewEncoder(w).Encode(response)
			if err != nil {
				t.Fatalf("Failed to encode response: %v", err)
			}
			return
		}

		// Handle initialized notification (no ID)
		if method == MethodNotificationInitialized {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL,
		WithBearerToken("my-token"),
		WithHeaders(map[string]string{
			"X-Custom": "test-value",
		}),
	)

	_, err := client.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Verify headers were sent
	if receivedAuthHeader != "Bearer my-token" {
		t.Errorf("Expected Authorization header %q, got %q", "Bearer my-token", receivedAuthHeader)
	}
	if receivedCustomHeader != "test-value" {
		t.Errorf("Expected X-Custom header %q, got %q", "test-value", receivedCustomHeader)
	}
}

func TestClient_Initialize(t *testing.T) {
	initializedNotificationReceived := false

	server := mockMCPServerWithNotifications(
		t,
		func(method string, params json.RawMessage) (interface{}, *RPCError) {
			if method != MethodInitialize {
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

			if initParams.ClientInfo.Name != "go-mcp-client" {
				t.Errorf("Expected client name go-mcp-client, got %s", initParams.ClientInfo.Name)
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
		},
		func(method string) {
			// Notification handler
			if method == MethodNotificationInitialized {
				initializedNotificationReceived = true
			}
		},
	)
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

	// Verify that the initialized notification was sent
	if !initializedNotificationReceived {
		t.Error("Expected initialized notification to be sent after initialize")
	}
}

func TestClient_ListTools(t *testing.T) {
	server := mockMCPServer(t, func(method string, params json.RawMessage) (interface{}, *RPCError) {
		if method != MethodToolsList {
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
		if method != MethodResourcesList {
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
		if method != MethodPromptsList {
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

	client := NewClient(server.URL, WithTimeout(100*time.Millisecond))
	ctx := context.Background()

	_, err := client.Initialize(ctx)
	if err == nil {
		t.Fatal("Expected timeout error, got nil")
	}
}

func TestClient_Ping(t *testing.T) {
	server := mockMCPServer(t, func(method string, params json.RawMessage) (interface{}, *RPCError) {
		if method != MethodInitialize {
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
