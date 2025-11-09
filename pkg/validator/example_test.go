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

package validator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/vitorbari/mcp-operator/pkg/mcp"
	"github.com/vitorbari/mcp-operator/pkg/validator"
)

// ExampleValidator demonstrates basic validator usage
func ExampleValidator() {
	// Create a test MCP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request mcp.JSONRPCRequest
		_ = json.NewDecoder(r.Body).Decode(&request)

		var result any
		switch request.Method {
		case "initialize":
			result = mcp.InitializeResult{
				ProtocolVersion: "2024-11-05",
				Capabilities: mcp.ServerCapabilities{
					Tools:     &mcp.ToolsCapability{},
					Resources: &mcp.ResourcesCapability{Subscribe: true},
				},
				ServerInfo: mcp.Implementation{
					Name:    "example-server",
					Version: "1.0.0",
				},
			}
		case "tools/list":
			result = mcp.ListToolsResult{Tools: []mcp.Tool{}}
		case "resources/list":
			result = mcp.ListResourcesResult{Resources: []mcp.Resource{}}
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

	// Create validator
	v := validator.NewValidator(server.URL)

	// Validate the server
	result, err := v.Validate(context.Background(), validator.ValidationOptions{})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Validation passed: %v\n", result.IsCompliant())
	fmt.Printf("Protocol version: %s\n", result.ProtocolVersion)
	fmt.Printf("Server: %s v%s\n", result.ServerInfo.Name, result.ServerInfo.Version)
	fmt.Printf("Capabilities: %v\n", result.Capabilities)
	fmt.Printf("Issues found: %d\n", len(result.Issues))

	// Output:
	// Validation passed: true
	// Protocol version: 2024-11-05
	// Server: example-server v1.0.0
	// Capabilities: [tools resources]
	// Issues found: 0
}

// ExampleValidator_requiredCapabilities demonstrates capability validation
func ExampleValidator_requiredCapabilities() {
	// Server only has tools capability
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request mcp.JSONRPCRequest
		_ = json.NewDecoder(r.Body).Decode(&request)

		result := mcp.InitializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities: mcp.ServerCapabilities{
				Tools: &mcp.ToolsCapability{}, // Only tools
			},
			ServerInfo: mcp.Implementation{
				Name:    "tools-only-server",
				Version: "1.0.0",
			},
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

	v := validator.NewValidator(server.URL)

	// Require both tools and resources capabilities
	result, _ := v.Validate(context.Background(), validator.ValidationOptions{
		RequiredCapabilities: []string{"tools", "resources"},
	})

	fmt.Printf("Validation passed: %v\n", result.IsCompliant())
	fmt.Printf("Has errors: %v\n", result.HasErrors())
	if result.HasErrors() {
		fmt.Printf("First error: %s\n", result.ErrorMessages()[0])
	}

	// Output:
	// Validation passed: false
	// Has errors: true
	// First error: Required capability 'resources' is not advertised by server
}

// ExampleValidator_strictMode demonstrates strict mode validation
func ExampleValidator_strictMode() {
	// Server with invalid protocol version
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request mcp.JSONRPCRequest
		_ = json.NewDecoder(r.Body).Decode(&request)

		result := mcp.InitializeResult{
			ProtocolVersion: "1.0.0", // Invalid version
			Capabilities: mcp.ServerCapabilities{
				Tools: &mcp.ToolsCapability{},
			},
			ServerInfo: mcp.Implementation{
				Name:    "old-server",
				Version: "1.0.0",
			},
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

	v := validator.NewValidator(server.URL)

	// Validate with strict mode
	result, _ := v.Validate(context.Background(), validator.ValidationOptions{
		StrictMode: true,
	})

	fmt.Printf("Validation passed: %v\n", result.IsCompliant())
	fmt.Printf("Number of issues: %d\n", len(result.Issues))
	for _, issue := range result.Issues {
		fmt.Printf("- [%s] %s\n", issue.Level, issue.Message)
	}

	// Output:
	// Validation passed: false
	// Number of issues: 1
	// - [error] Unsupported protocol version: 1.0.0 (expected 2024-11-05 or 2025-03-26)
}
