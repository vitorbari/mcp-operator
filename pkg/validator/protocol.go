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
)

// Protocol version constants based on MCP specification
// These represent the evolution of the Model Context Protocol
const (
	// ProtocolVersion20241105 is the initial stable version with HTTP+SSE transports
	ProtocolVersion20241105 = "2024-11-05"

	// ProtocolVersion20250326 introduced Streamable HTTP as the preferred transport
	ProtocolVersion20250326 = "2025-03-26"

	// ProtocolVersion20250618 is the latest version with OAuth 2.1 support
	ProtocolVersion20250618 = "2025-06-18"
)

// SupportedProtocolVersions lists all protocol versions we support, in preference order (newest first)
var SupportedProtocolVersions = []string{
	ProtocolVersion20250618,
	ProtocolVersion20250326,
	ProtocolVersion20241105,
}

// ProtocolVersionDetector handles protocol version detection and negotiation
type ProtocolVersionDetector struct {
	preferredVersion  string
	supportedVersions []string
}

// NewProtocolVersionDetector creates a new protocol version detector
func NewProtocolVersionDetector() *ProtocolVersionDetector {
	return &ProtocolVersionDetector{
		preferredVersion:  ProtocolVersion20250618, // Latest version
		supportedVersions: SupportedProtocolVersions,
	}
}

// NewProtocolVersionDetectorWithPreferred creates a detector with a specific preferred version
func NewProtocolVersionDetectorWithPreferred(preferredVersion string) *ProtocolVersionDetector {
	return &ProtocolVersionDetector{
		preferredVersion:  preferredVersion,
		supportedVersions: SupportedProtocolVersions,
	}
}

// GetPreferredVersion returns the preferred protocol version for initialization
func (d *ProtocolVersionDetector) GetPreferredVersion() string {
	return d.preferredVersion
}

// NegotiateVersion determines the best protocol version to use
// It selects the latest version supported by both client and server
func (d *ProtocolVersionDetector) NegotiateVersion(serverVersions []string) string {
	// If server doesn't advertise versions, fall back to oldest for compatibility
	if len(serverVersions) == 0 {
		return ProtocolVersion20241105
	}

	// Try to use the latest version supported by both client and server
	for _, clientVersion := range d.supportedVersions {
		for _, serverVersion := range serverVersions {
			if clientVersion == serverVersion {
				return clientVersion
			}
		}
	}

	// No common version found, fall back to oldest for maximum compatibility
	return ProtocolVersion20241105
}

// DetectVersionFromResponse extracts protocol version from server response
// This checks multiple sources: headers, response body, and capability hints
func (d *ProtocolVersionDetector) DetectVersionFromResponse(headers http.Header, body map[string]interface{}) string {
	// Method 1: Check MCP-Protocol-Version header (introduced in 2025-03-26)
	if version := headers.Get("MCP-Protocol-Version"); version != "" {
		// Headers take precedence - if server advertises a version, use it
		// even if we need to fall back for unsupported versions
		if d.IsSupported(version) {
			return version
		}
		// If header exists but version is unsupported, still try other methods
	}

	// Method 2: Check protocolVersion in response body
	if result, ok := body["result"].(map[string]interface{}); ok {
		if version, ok := result["protocolVersion"].(string); ok {
			if d.IsSupported(version) {
				return version
			}
		}
	}

	// Method 3: Infer from capabilities structure
	version := d.inferVersionFromCapabilities(body)
	if version != "" && d.IsSupported(version) {
		return version
	}

	// Method 4: Default to oldest supported version for compatibility
	return ProtocolVersion20241105
}

// inferVersionFromCapabilities attempts to infer protocol version from capability structure
func (d *ProtocolVersionDetector) inferVersionFromCapabilities(body map[string]interface{}) string {
	result, ok := body["result"].(map[string]interface{})
	if !ok {
		return ""
	}

	caps, ok := result["capabilities"].(map[string]interface{})
	if !ok {
		return ""
	}

	// Version 2025-06-18 introduced "roots" capability
	if _, hasRoots := caps["roots"]; hasRoots {
		return ProtocolVersion20250618
	}

	// Version 2025-03-26 has specific capability structure differences
	// Check for presence of modern capabilities
	if _, hasTools := caps["tools"]; hasTools {
		if toolsCap, ok := caps["tools"].(map[string]interface{}); ok {
			// 2025-03-26+ has listChanged field in capabilities
			if _, hasListChanged := toolsCap["listChanged"]; hasListChanged {
				return ProtocolVersion20250326
			}
		}
	}

	// Default to base version
	return ProtocolVersion20241105
}

// IsSupported checks if a protocol version is supported
func (d *ProtocolVersionDetector) IsSupported(version string) bool {
	for _, supported := range d.supportedVersions {
		if version == supported {
			return true
		}
	}
	return false
}

// IsCompatible checks if two versions are compatible
// Generally, servers should be backward compatible within major version changes
func (d *ProtocolVersionDetector) IsCompatible(version1, version2 string) bool {
	// For now, we consider all 2024-2025 versions to be compatible
	// This is because the protocol maintains backward compatibility
	supportedVersionsSet := make(map[string]bool)
	for _, v := range d.supportedVersions {
		supportedVersionsSet[v] = true
	}

	return supportedVersionsSet[version1] && supportedVersionsSet[version2]
}

// GetVersionFeatures returns a description of features available in a version
func (d *ProtocolVersionDetector) GetVersionFeatures(version string) VersionFeatures {
	switch version {
	case ProtocolVersion20250618:
		return VersionFeatures{
			SupportsStreamableHTTP:    true,
			SupportsSSE:               true,
			SupportsSessionManagement: true,
			SupportsRootsCapability:   true,
			SupportsOAuth:             true,
		}
	case ProtocolVersion20250326:
		return VersionFeatures{
			SupportsStreamableHTTP:    true,
			SupportsSSE:               true,
			SupportsSessionManagement: true,
			SupportsRootsCapability:   false,
			SupportsOAuth:             false,
		}
	case ProtocolVersion20241105:
		return VersionFeatures{
			SupportsStreamableHTTP:    false,
			SupportsSSE:               true,
			SupportsSessionManagement: false,
			SupportsRootsCapability:   false,
			SupportsOAuth:             false,
		}
	default:
		// Unknown version, return conservative feature set
		return VersionFeatures{
			SupportsStreamableHTTP:    false,
			SupportsSSE:               true,
			SupportsSessionManagement: false,
			SupportsRootsCapability:   false,
			SupportsOAuth:             false,
		}
	}
}

// VersionFeatures describes the features available in a protocol version
type VersionFeatures struct {
	SupportsStreamableHTTP    bool
	SupportsSSE               bool
	SupportsSessionManagement bool
	SupportsRootsCapability   bool
	SupportsOAuth             bool
}

// CompareVersions compares two protocol versions
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func CompareVersions(v1, v2 string) int {
	// Simple string comparison works for our date-based versions
	if v1 < v2 {
		return -1
	}
	if v1 > v2 {
		return 1
	}
	return 0
}

// IsVersionNewer checks if version1 is newer than version2
func IsVersionNewer(version1, version2 string) bool {
	return CompareVersions(version1, version2) > 0
}

// GetLatestSupportedVersion returns the latest version from a list
func GetLatestSupportedVersion(versions []string) string {
	if len(versions) == 0 {
		return ProtocolVersion20241105
	}

	latest := versions[0]
	for _, v := range versions[1:] {
		if IsVersionNewer(v, latest) {
			latest = v
		}
	}

	return latest
}
