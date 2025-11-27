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
	"fmt"
	"time"

	"github.com/vitorbari/mcp-operator/pkg/mcp"
)

// ExampleNewClient_withBearerToken demonstrates creating an MCP client with Bearer token authentication
func ExampleNewClient_withBearerToken() {
	// Create a client with Bearer token authentication
	client := mcp.NewClient(
		"https://api.example.com/mcp",
		mcp.WithBearerToken("your-secret-token"),
		mcp.WithTimeout(60*time.Second),
	)

	// Use the client - the Bearer token will be sent with all requests
	ctx := context.Background()
	result, err := client.Initialize(ctx)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Connected to: %s\n", result.ServerInfo.Name)
}

// ExampleNewClient_withCustomHeaders demonstrates creating an MCP client with custom headers
func ExampleNewClient_withCustomHeaders() {
	// Create a client with custom headers
	client := mcp.NewClient(
		"https://api.example.com/mcp",
		mcp.WithHeaders(map[string]string{
			"X-API-Key":    "api-key-123",
			"X-Request-ID": "req-456",
			"X-Tenant-ID":  "tenant-789",
		}),
	)

	// Use the client - custom headers will be sent with all requests
	ctx := context.Background()
	tools, err := client.ListTools(ctx)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Found %d tools\n", len(tools.Tools))
}

// ExampleNewClient_withBearerTokenAndCustomHeaders demonstrates combining Bearer token and custom headers
func ExampleNewClient_withBearerTokenAndCustomHeaders() {
	// Create a client with both Bearer token and custom headers
	client := mcp.NewClient(
		"https://api.example.com/mcp",
		mcp.WithBearerToken("your-secret-token"),
		mcp.WithHeaders(map[string]string{
			"X-Client-Version": "1.0.0",
			"X-Platform":       "kubernetes",
		}),
	)

	// All requests will include both the Authorization header and custom headers
	ctx := context.Background()
	_, err := client.Initialize(ctx)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Connected with authentication and custom headers")
}

// ExampleNewClient_withClientInfo demonstrates custom client identification
func ExampleNewClient_withClientInfo() {
	// Create a client with custom identification
	client := mcp.NewClient(
		"http://localhost:8080/mcp",
		mcp.WithClientInfo("analytics-service", "2.1.0"),
	)

	// The custom client info will be sent during initialization
	ctx := context.Background()
	result, err := client.Initialize(ctx)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Connected as: analytics-service v2.1.0 to %s\n", result.ServerInfo.Name)
}

// ExampleNewClient_allOptions demonstrates using all configuration options together
func ExampleNewClient_allOptions() {
	// Create a fully configured client
	client := mcp.NewClient(
		"https://api.example.com/mcp",
		mcp.WithClientInfo("production-service", "3.0.0"),
		mcp.WithBearerToken("prod-token-xyz"),
		mcp.WithTimeout(90*time.Second),
		mcp.WithHeaders(map[string]string{
			"X-Environment": "production",
			"X-Region":      "us-west-2",
		}),
	)

	// Use the fully configured client
	ctx := context.Background()
	_, err := client.Initialize(ctx)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Production client ready")
}
