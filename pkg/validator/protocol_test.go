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
	"net/http"
	"testing"
)

func TestNewProtocolVersionDetector(t *testing.T) {
	detector := NewProtocolVersionDetector()

	if detector == nil {
		t.Fatal("NewProtocolVersionDetector returned nil")
	}

	if detector.preferredVersion != ProtocolVersion20250618 {
		t.Errorf("Expected preferred version %s, got %s", ProtocolVersion20250618, detector.preferredVersion)
	}

	if len(detector.supportedVersions) != 3 {
		t.Errorf("Expected 3 supported versions, got %d", len(detector.supportedVersions))
	}
}

func TestProtocolVersionDetector_GetPreferredVersion(t *testing.T) {
	detector := NewProtocolVersionDetector()
	preferred := detector.GetPreferredVersion()

	if preferred != ProtocolVersion20250618 {
		t.Errorf("GetPreferredVersion() = %s, want %s", preferred, ProtocolVersion20250618)
	}
}

func TestProtocolVersionDetector_IsSupported(t *testing.T) {
	detector := NewProtocolVersionDetector()

	tests := []struct {
		version   string
		supported bool
	}{
		{ProtocolVersion20241105, true},
		{ProtocolVersion20250326, true},
		{ProtocolVersion20250618, true},
		{"1.0.0", false},
		{"2026-01-01", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			result := detector.IsSupported(tt.version)
			if result != tt.supported {
				t.Errorf("IsSupported(%q) = %v, want %v", tt.version, result, tt.supported)
			}
		})
	}
}

func TestProtocolVersionDetector_NegotiateVersion(t *testing.T) {
	detector := NewProtocolVersionDetector()

	tests := []struct {
		name           string
		serverVersions []string
		expected       string
	}{
		{
			name:           "ServerSupportsLatest",
			serverVersions: []string{ProtocolVersion20250618, ProtocolVersion20250326},
			expected:       ProtocolVersion20250618,
		},
		{
			name:           "ServerSupportsMiddle",
			serverVersions: []string{ProtocolVersion20250326, ProtocolVersion20241105},
			expected:       ProtocolVersion20250326,
		},
		{
			name:           "ServerSupportsOldest",
			serverVersions: []string{ProtocolVersion20241105},
			expected:       ProtocolVersion20241105,
		},
		{
			name:           "NoCommonVersion",
			serverVersions: []string{"1.0.0", "2026-01-01"},
			expected:       ProtocolVersion20241105, // Fallback to oldest
		},
		{
			name:           "EmptyServerVersions",
			serverVersions: []string{},
			expected:       ProtocolVersion20241105, // Fallback to oldest
		},
		{
			name:           "ServerHasNewerVersion",
			serverVersions: []string{"2026-01-01", ProtocolVersion20250326},
			expected:       ProtocolVersion20250326, // Use latest common version
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.NegotiateVersion(tt.serverVersions)
			if result != tt.expected {
				t.Errorf("NegotiateVersion(%v) = %s, want %s", tt.serverVersions, result, tt.expected)
			}
		})
	}
}

func TestProtocolVersionDetector_DetectVersionFromResponse(t *testing.T) {
	detector := NewProtocolVersionDetector()

	tests := []struct {
		name     string
		headers  http.Header
		body     map[string]interface{}
		expected string
	}{
		{
			name: "VersionInHeader",
			headers: func() http.Header {
				h := http.Header{}
				h.Set("MCP-Protocol-Version", ProtocolVersion20250618)
				return h
			}(),
			body:     map[string]interface{}{},
			expected: ProtocolVersion20250618,
		},
		{
			name:    "VersionInBody",
			headers: http.Header{},
			body: map[string]interface{}{
				"result": map[string]interface{}{
					"protocolVersion": ProtocolVersion20250326,
				},
			},
			expected: ProtocolVersion20250326,
		},
		{
			name:    "InferFromRootsCapability",
			headers: http.Header{},
			body: map[string]interface{}{
				"result": map[string]interface{}{
					"capabilities": map[string]interface{}{
						"roots": map[string]interface{}{},
					},
				},
			},
			expected: ProtocolVersion20250618,
		},
		{
			name:    "InferFromToolsCapability",
			headers: http.Header{},
			body: map[string]interface{}{
				"result": map[string]interface{}{
					"capabilities": map[string]interface{}{
						"tools": map[string]interface{}{
							"listChanged": true,
						},
					},
				},
			},
			expected: ProtocolVersion20250326,
		},
		{
			name:     "NoVersionInfo",
			headers:  http.Header{},
			body:     map[string]interface{}{},
			expected: ProtocolVersion20241105, // Fallback
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectVersionFromResponse(tt.headers, tt.body)
			if result != tt.expected {
				t.Errorf("DetectVersionFromResponse() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestProtocolVersionDetector_IsCompatible(t *testing.T) {
	detector := NewProtocolVersionDetector()

	tests := []struct {
		version1   string
		version2   string
		compatible bool
	}{
		{ProtocolVersion20241105, ProtocolVersion20250326, true},
		{ProtocolVersion20250326, ProtocolVersion20250618, true},
		{ProtocolVersion20241105, ProtocolVersion20250618, true},
		{ProtocolVersion20241105, "1.0.0", false},
		{"1.0.0", "2.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.version1+"_"+tt.version2, func(t *testing.T) {
			result := detector.IsCompatible(tt.version1, tt.version2)
			if result != tt.compatible {
				t.Errorf("IsCompatible(%s, %s) = %v, want %v", tt.version1, tt.version2, result, tt.compatible)
			}
		})
	}
}

func TestProtocolVersionDetector_GetVersionFeatures(t *testing.T) {
	detector := NewProtocolVersionDetector()

	tests := []struct {
		version  string
		features VersionFeatures
	}{
		{
			version: ProtocolVersion20250618,
			features: VersionFeatures{
				SupportsStreamableHTTP:    true,
				SupportsSSE:               true,
				SupportsSessionManagement: true,
				SupportsRootsCapability:   true,
				SupportsOAuth:             true,
			},
		},
		{
			version: ProtocolVersion20250326,
			features: VersionFeatures{
				SupportsStreamableHTTP:    true,
				SupportsSSE:               true,
				SupportsSessionManagement: true,
				SupportsRootsCapability:   false,
				SupportsOAuth:             false,
			},
		},
		{
			version: ProtocolVersion20241105,
			features: VersionFeatures{
				SupportsStreamableHTTP:    false,
				SupportsSSE:               true,
				SupportsSessionManagement: false,
				SupportsRootsCapability:   false,
				SupportsOAuth:             false,
			},
		},
		{
			version: "unknown",
			features: VersionFeatures{
				SupportsStreamableHTTP:    false,
				SupportsSSE:               true,
				SupportsSessionManagement: false,
				SupportsRootsCapability:   false,
				SupportsOAuth:             false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			features := detector.GetVersionFeatures(tt.version)
			if features != tt.features {
				t.Errorf("GetVersionFeatures(%s) = %+v, want %+v", tt.version, features, tt.features)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{ProtocolVersion20241105, ProtocolVersion20250326, -1},
		{ProtocolVersion20250326, ProtocolVersion20241105, 1},
		{ProtocolVersion20250326, ProtocolVersion20250326, 0},
		{ProtocolVersion20250618, ProtocolVersion20250326, 1},
		{ProtocolVersion20241105, ProtocolVersion20250618, -1},
	}

	for _, tt := range tests {
		t.Run(tt.v1+"_vs_"+tt.v2, func(t *testing.T) {
			result := CompareVersions(tt.v1, tt.v2)
			if result != tt.expected {
				t.Errorf("CompareVersions(%s, %s) = %d, want %d", tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

func TestIsVersionNewer(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected bool
	}{
		{ProtocolVersion20250618, ProtocolVersion20250326, true},
		{ProtocolVersion20250326, ProtocolVersion20241105, true},
		{ProtocolVersion20241105, ProtocolVersion20250326, false},
		{ProtocolVersion20250326, ProtocolVersion20250326, false},
	}

	for _, tt := range tests {
		t.Run(tt.v1+"_vs_"+tt.v2, func(t *testing.T) {
			result := IsVersionNewer(tt.v1, tt.v2)
			if result != tt.expected {
				t.Errorf("IsVersionNewer(%s, %s) = %v, want %v", tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

func TestGetLatestSupportedVersion(t *testing.T) {
	tests := []struct {
		name     string
		versions []string
		expected string
	}{
		{
			name:     "MultipleVersions",
			versions: []string{ProtocolVersion20241105, ProtocolVersion20250618, ProtocolVersion20250326},
			expected: ProtocolVersion20250618,
		},
		{
			name:     "SingleVersion",
			versions: []string{ProtocolVersion20241105},
			expected: ProtocolVersion20241105,
		},
		{
			name:     "EmptyList",
			versions: []string{},
			expected: ProtocolVersion20241105, // Fallback
		},
		{
			name:     "OutOfOrder",
			versions: []string{ProtocolVersion20250326, ProtocolVersion20241105, ProtocolVersion20250618},
			expected: ProtocolVersion20250618,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetLatestSupportedVersion(tt.versions)
			if result != tt.expected {
				t.Errorf("GetLatestSupportedVersion(%v) = %s, want %s", tt.versions, result, tt.expected)
			}
		})
	}
}

func TestNewProtocolVersionDetectorWithPreferred(t *testing.T) {
	preferredVersion := ProtocolVersion20250326
	detector := NewProtocolVersionDetectorWithPreferred(preferredVersion)

	if detector.GetPreferredVersion() != preferredVersion {
		t.Errorf("Expected preferred version %s, got %s", preferredVersion, detector.GetPreferredVersion())
	}
}

func TestSupportedProtocolVersions(t *testing.T) {
	// Verify the constant is properly defined
	if len(SupportedProtocolVersions) == 0 {
		t.Error("SupportedProtocolVersions should not be empty")
	}

	// Verify versions are in order (newest first)
	for i := 0; i < len(SupportedProtocolVersions)-1; i++ {
		if !IsVersionNewer(SupportedProtocolVersions[i], SupportedProtocolVersions[i+1]) {
			t.Errorf("SupportedProtocolVersions not in correct order: %s should be newer than %s",
				SupportedProtocolVersions[i], SupportedProtocolVersions[i+1])
		}
	}

	// Verify all expected versions are present
	expectedVersions := map[string]bool{
		ProtocolVersion20241105: true,
		ProtocolVersion20250326: true,
		ProtocolVersion20250618: true,
	}

	for _, version := range SupportedProtocolVersions {
		if !expectedVersions[version] {
			t.Errorf("Unexpected version in SupportedProtocolVersions: %s", version)
		}
		delete(expectedVersions, version)
	}

	if len(expectedVersions) > 0 {
		t.Errorf("Missing versions in SupportedProtocolVersions: %v", expectedVersions)
	}
}
