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

package validator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestSSEDetection_WithAuth tests that SSE detection accepts 401 responses
// with text/event-stream content-type (auth-required servers)
func TestSSEDetection_WithAuth(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		contentType    string
		wwwAuth        string
		expectedDetect bool
		description    string
	}{
		{
			name:           "SSE with 200 OK",
			statusCode:     http.StatusOK,
			contentType:    "text/event-stream",
			expectedDetect: true,
			description:    "Standard SSE response should be detected",
		},
		{
			name:           "SSE with 401 Auth Required",
			statusCode:     http.StatusUnauthorized,
			contentType:    "text/event-stream",
			wwwAuth:        "Bearer realm=\"MCP SSE Server\"",
			expectedDetect: true,
			description:    "SSE with auth required should still be detected as SSE",
		},
		{
			name:           "SSE with 403 Forbidden",
			statusCode:     http.StatusForbidden,
			contentType:    "text/event-stream",
			expectedDetect: false,
			description:    "403 should not be accepted (only 200 and 401)",
		},
		{
			name:           "Wrong content-type with 200",
			statusCode:     http.StatusOK,
			contentType:    "application/json",
			expectedDetect: false,
			description:    "JSON content-type should not be detected as SSE",
		},
		{
			name:           "Wrong content-type with 401",
			statusCode:     http.StatusUnauthorized,
			contentType:    "application/json",
			expectedDetect: false,
			description:    "Even with 401, wrong content-type means not SSE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server that returns specified response
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Only handle GET requests
				if r.Method != "GET" {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
					return
				}

				w.Header().Set("Content-Type", tt.contentType)
				if tt.wwwAuth != "" {
					w.Header().Set("WWW-Authenticate", tt.wwwAuth)
				}
				w.WriteHeader(tt.statusCode)

				// Write SSE-like content if successful
				if tt.statusCode == http.StatusOK {
					w.Write([]byte("event: test\ndata: test\n\n"))
				} else if tt.statusCode == http.StatusUnauthorized && tt.contentType == "text/event-stream" {
					w.Write([]byte("event: error\ndata: {\"error\": \"Unauthorized\"}\n\n"))
				}
			}))
			defer server.Close()

			// Create detector and test
			detector := NewTransportDetector(5 * time.Second)
			ctx := context.Background()

			detected := detector.trySSE(ctx, server.URL)

			if detected != tt.expectedDetect {
				t.Errorf("%s: trySSE() = %v, want %v", tt.description, detected, tt.expectedDetect)
			}
		})
	}
}

// TestStreamableHTTPDetection_WithAuth tests Streamable HTTP detection with auth
func TestStreamableHTTPDetection_WithAuth(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		contentType    string
		wwwAuth        string
		expectedDetect bool
		description    string
	}{
		{
			name:           "Streamable HTTP with 200 OK",
			statusCode:     http.StatusOK,
			contentType:    "application/json",
			expectedDetect: true,
			description:    "Standard Streamable HTTP response",
		},
		{
			name:           "Streamable HTTP with 401 Auth Required",
			statusCode:     http.StatusUnauthorized,
			contentType:    "application/json",
			wwwAuth:        "Bearer realm=\"MCP Server\"",
			expectedDetect: true,
			description:    "Streamable HTTP with auth should be detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
					return
				}

				w.Header().Set("Content-Type", tt.contentType)
				if tt.wwwAuth != "" {
					w.Header().Set("WWW-Authenticate", tt.wwwAuth)
				}
				w.WriteHeader(tt.statusCode)

				if tt.contentType == "application/json" {
					_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32001,"message":"Unauthorized"}}`))
				} else {
					// Write non-JSON content
					_, _ = w.Write([]byte(`<html><body>Not an MCP server</body></html>`))
				}
			}))
			defer server.Close()

			detector := NewTransportDetector(5 * time.Second)
			ctx := context.Background()

			detected := detector.tryStreamableHTTP(ctx, server.URL)

			if detected != tt.expectedDetect {
				t.Errorf("%s: tryStreamableHTTP() = %v, want %v", tt.description, detected, tt.expectedDetect)
			}
		})
	}
}

// TestTransportDetection_PreferStreamableHTTP tests that Streamable HTTP is preferred over SSE
func TestTransportDetection_PreferStreamableHTTP(t *testing.T) {
	// Create a server that supports both protocols
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			// Streamable HTTP
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
		case "GET":
			// SSE
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("event: test\ndata: test\n\n"))
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	detector := NewTransportDetector(5 * time.Second)
	ctx := context.Background()

	transportType, endpoint, err := detector.DetectTransport(ctx, server.URL, "")

	if err != nil {
		t.Fatalf("DetectTransport() returned error: %v", err)
	}

	// Should detect as Streamable HTTP (preferred over SSE)
	if transportType != TransportStreamableHTTP {
		t.Errorf("DetectTransport() detected %v, want %v (Streamable HTTP should be preferred)",
			transportType, TransportStreamableHTTP)
	}

	// Endpoint should have /mcp appended (default path for Streamable HTTP)
	expectedEndpoint := server.URL + "/mcp"
	if endpoint != expectedEndpoint {
		t.Errorf("DetectTransport() endpoint = %v, want %v", endpoint, expectedEndpoint)
	}
}

// TestTransportDetection_AuthRequired tests full detection flow with auth-required servers
func TestTransportDetection_AuthRequired(t *testing.T) {
	tests := []struct {
		name             string
		setupServer      func() *httptest.Server
		expectedTransport TransportType
		description      string
	}{
		{
			name: "SSE only server with auth",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.Method {
					case "POST":
						// Reject POST (SSE doesn't use POST)
						http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
					case "GET":
						// SSE with auth required
						w.Header().Set("Content-Type", "text/event-stream")
						w.Header().Set("WWW-Authenticate", "Bearer realm=\"MCP SSE Server\"")
						w.WriteHeader(http.StatusUnauthorized)
						_, _ = w.Write([]byte("event: error\ndata: {\"error\": \"Unauthorized\"}\n\n"))
					}
				}))
			},
			expectedTransport: TransportSSE,
			description:      "Should detect SSE when POST is rejected and GET returns SSE with 401",
		},
		{
			name: "Streamable HTTP only with auth",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != "POST" {
						http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
						return
					}
					// Streamable HTTP with auth
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("WWW-Authenticate", "Bearer realm=\"MCP Server\"")
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32001,"message":"Unauthorized"}}`))
				}))
			},
			expectedTransport: TransportStreamableHTTP,
			description:      "Should detect Streamable HTTP with 401 auth response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			detector := NewTransportDetector(5 * time.Second)
			ctx := context.Background()

			transportType, _, err := detector.DetectTransport(ctx, server.URL, "")

			if err != nil {
				t.Fatalf("%s: DetectTransport() returned error: %v", tt.description, err)
			}

			if transportType != tt.expectedTransport {
				t.Errorf("%s: DetectTransport() = %v, want %v",
					tt.description, transportType, tt.expectedTransport)
			}
		})
	}
}
