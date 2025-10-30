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

package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/vitorbari/mcp-operator/internal/mcp"
)

// ExampleClient demonstrates basic MCP client usage
func ExampleClient() {
	// Create a test MCP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request mcp.JSONRPCRequest
		_ = json.NewDecoder(r.Body).Decode(&request)

		var result interface{}
		switch request.Method {
		case "initialize":
			result = mcp.InitializeResult{
				ProtocolVersion: "2024-11-05",
				Capabilities: mcp.ServerCapabilities{
					Tools:     &mcp.ToolsCapability{},
					Resources: &mcp.ResourcesCapability{Subscribe: true},
					Prompts:   &mcp.PromptsCapability{},
				},
				ServerInfo: mcp.Implementation{
					Name:    "example-server",
					Version: "1.0.0",
				},
			}
		case "tools/list":
			result = mcp.ListToolsResult{
				Tools: []mcp.Tool{
					{
						Name:        "search",
						Description: "Search for information",
						InputSchema: map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"query": map[string]string{"type": "string"},
							},
						},
					},
				},
			}
		}

		response := mcp.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      request.ID,
			Result:  result,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create MCP client
	client := mcp.NewClient(server.URL)
	ctx := context.Background()

	// Initialize connection
	initResult, err := client.Initialize(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Connected to: %s v%s\n", initResult.ServerInfo.Name, initResult.ServerInfo.Version)
	fmt.Printf("Protocol version: %s\n", initResult.ProtocolVersion)

	// Check capabilities
	fmt.Printf("Supports tools: %v\n", initResult.Capabilities.Tools != nil)
	fmt.Printf("Supports resources: %v\n", initResult.Capabilities.Resources != nil)

	// List available tools
	if initResult.Capabilities.Tools != nil {
		tools, err := client.ListTools(ctx)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Available tools: %d\n", len(tools.Tools))
		if len(tools.Tools) > 0 {
			fmt.Printf("First tool: %s\n", tools.Tools[0].Name)
		}
	}

	// Output:
	// Connected to: example-server v1.0.0
	// Protocol version: 2024-11-05
	// Supports tools: true
	// Supports resources: true
	// Available tools: 1
	// First tool: search
}

// ExampleClient_ping demonstrates health checking
func ExampleClient_ping() {
	// Create a test MCP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := mcp.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      1,
			Result: mcp.InitializeResult{
				ProtocolVersion: "2024-11-05",
				ServerInfo: mcp.Implementation{
					Name:    "test-server",
					Version: "1.0.0",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Quick health check
	client := mcp.NewClient(server.URL)
	if err := client.Ping(context.Background()); err != nil {
		fmt.Println("Server is down")
	} else {
		fmt.Println("Server is healthy")
	}

	// Output:
	// Server is healthy
}
