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
	"strings"
	"testing"
)

func TestNewIssueCatalog(t *testing.T) {
	catalog := NewIssueCatalog()

	if catalog == nil {
		t.Fatal("NewIssueCatalog returned nil")
	}

	if len(catalog.issues) == 0 {
		t.Error("Expected catalog to have default issues registered")
	}
}

func TestIssueCatalog_Enhance(t *testing.T) {
	catalog := NewIssueCatalog()

	issue := ValidationIssue{
		Level:   LevelError,
		Code:    "TRANSPORT_DETECTION_FAILED",
		Message: "Failed to detect transport",
	}

	enhanced := catalog.Enhance(issue)

	if enhanced.Code != issue.Code {
		t.Errorf("Expected code %s, got %s", issue.Code, enhanced.Code)
	}

	if len(enhanced.Suggestions) == 0 {
		t.Error("Expected enhanced issue to have suggestions")
	}

	if enhanced.DocumentationURL == "" {
		t.Error("Expected enhanced issue to have documentation URL")
	}

	// Verify suggestions are meaningful
	foundServerCheck := false
	for _, suggestion := range enhanced.Suggestions {
		if strings.Contains(strings.ToLower(suggestion), "server") {
			foundServerCheck = true
			break
		}
	}
	if !foundServerCheck {
		t.Error("Expected suggestions to mention server checks")
	}
}

func TestIssueCatalog_EnhanceUnknownIssue(t *testing.T) {
	catalog := NewIssueCatalog()

	issue := ValidationIssue{
		Level:   LevelError,
		Code:    "UNKNOWN_CODE",
		Message: "Unknown error",
	}

	enhanced := catalog.Enhance(issue)

	// Should still create enhanced issue even if not in catalog
	if enhanced.Code != issue.Code {
		t.Errorf("Expected code %s, got %s", issue.Code, enhanced.Code)
	}

	// But won't have suggestions
	if len(enhanced.Suggestions) != 0 {
		t.Error("Expected unknown issue to have no suggestions")
	}
}

func TestIssueCatalog_EnhanceAll(t *testing.T) {
	catalog := NewIssueCatalog()

	issues := []ValidationIssue{
		{
			Level:   LevelError,
			Code:    "TRANSPORT_DETECTION_FAILED",
			Message: "Transport detection failed",
		},
		{
			Level:   LevelError,
			Code:    CodeInitializeFailed,
			Message: "Initialize failed",
		},
	}

	enhanced := catalog.EnhanceAll(issues)

	if len(enhanced) != len(issues) {
		t.Errorf("Expected %d enhanced issues, got %d", len(issues), len(enhanced))
	}

	for i, e := range enhanced {
		if e.Code != issues[i].Code {
			t.Errorf("Enhanced issue %d has wrong code: expected %s, got %s",
				i, issues[i].Code, e.Code)
		}
		if len(e.Suggestions) == 0 {
			t.Errorf("Enhanced issue %d has no suggestions", i)
		}
	}
}

func TestIssueCatalog_GetTemplate(t *testing.T) {
	catalog := NewIssueCatalog()

	tests := []struct {
		code        string
		shouldExist bool
	}{
		{"TRANSPORT_DETECTION_FAILED", true},
		{CodeInitializeFailed, true},
		{CodeInvalidProtocolVersion, true},
		{"NONEXISTENT_CODE", false},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			template, ok := catalog.GetTemplate(tt.code)

			if ok != tt.shouldExist {
				t.Errorf("GetTemplate(%s) existence = %v, want %v", tt.code, ok, tt.shouldExist)
			}

			if tt.shouldExist {
				if template.Code != tt.code {
					t.Errorf("Template code = %s, want %s", template.Code, tt.code)
				}
				if len(template.Suggestions) == 0 {
					t.Error("Expected template to have suggestions")
				}
			}
		})
	}
}

func TestIssueCatalog_RegisterIssue(t *testing.T) {
	catalog := NewIssueCatalog()

	customTemplate := IssueTemplate{
		Code:        "CUSTOM_ISSUE",
		Title:       "Custom Issue",
		Description: "A custom validation issue",
		Suggestions: []string{"Do this", "Try that"},
	}

	catalog.RegisterIssue(customTemplate)

	template, ok := catalog.GetTemplate("CUSTOM_ISSUE")
	if !ok {
		t.Fatal("Failed to register custom issue")
	}

	if template.Code != customTemplate.Code {
		t.Errorf("Expected code %s, got %s", customTemplate.Code, template.Code)
	}

	if len(template.Suggestions) != len(customTemplate.Suggestions) {
		t.Errorf("Expected %d suggestions, got %d",
			len(customTemplate.Suggestions), len(template.Suggestions))
	}
}

func TestEnhancedValidationIssue_FormatSuggestions(t *testing.T) {
	enhanced := EnhancedValidationIssue{
		ValidationIssue: ValidationIssue{
			Level:   LevelError,
			Code:    "TEST_CODE",
			Message: "Test message",
		},
		Suggestions: []string{
			"First suggestion",
			"Second suggestion",
		},
		DocumentationURL: "https://example.com/docs",
	}

	formatted := enhanced.FormatSuggestions()

	if !strings.Contains(formatted, "Suggestions:") {
		t.Error("Formatted output should contain 'Suggestions:' header")
	}

	if !strings.Contains(formatted, "1. First suggestion") {
		t.Error("Formatted output should contain numbered first suggestion")
	}

	if !strings.Contains(formatted, "2. Second suggestion") {
		t.Error("Formatted output should contain numbered second suggestion")
	}

	if !strings.Contains(formatted, "https://example.com/docs") {
		t.Error("Formatted output should contain documentation URL")
	}
}

func TestEnhancedValidationIssue_String(t *testing.T) {
	enhanced := EnhancedValidationIssue{
		ValidationIssue: ValidationIssue{
			Level:   LevelError,
			Code:    "TEST_CODE",
			Message: "Test error message",
		},
		Suggestions: []string{
			"Try this",
		},
		DocumentationURL: "https://example.com",
		RelatedIssues:    []string{"RELATED_1", "RELATED_2"},
	}

	str := enhanced.String()

	if !strings.Contains(str, "ERROR") {
		t.Error("String output should contain severity level")
	}

	if !strings.Contains(str, "TEST_CODE") {
		t.Error("String output should contain issue code")
	}

	if !strings.Contains(str, "Test error message") {
		t.Error("String output should contain message")
	}

	if !strings.Contains(str, "Try this") {
		t.Error("String output should contain suggestions")
	}

	if !strings.Contains(str, "RELATED_1") {
		t.Error("String output should contain related issues")
	}
}

func TestDefaultIssueCatalog(t *testing.T) {
	// Verify the default catalog is initialized
	if DefaultIssueCatalog == nil {
		t.Fatal("DefaultIssueCatalog should not be nil")
	}

	// Test that it has common issues
	_, ok := DefaultIssueCatalog.GetTemplate("TRANSPORT_DETECTION_FAILED")
	if !ok {
		t.Error("DefaultIssueCatalog should have TRANSPORT_DETECTION_FAILED")
	}
}

func TestEnhanceIssue_ConvenienceFunction(t *testing.T) {
	issue := ValidationIssue{
		Level:   LevelError,
		Code:    CodeInitializeFailed,
		Message: "Init failed",
	}

	enhanced := EnhanceIssue(issue)

	if enhanced.Code != issue.Code {
		t.Errorf("Expected code %s, got %s", issue.Code, enhanced.Code)
	}

	if len(enhanced.Suggestions) == 0 {
		t.Error("Expected enhanced issue to have suggestions")
	}
}

func TestEnhanceIssues_ConvenienceFunction(t *testing.T) {
	issues := []ValidationIssue{
		{
			Level:   LevelError,
			Code:    "TRANSPORT_DETECTION_FAILED",
			Message: "Detection failed",
		},
		{
			Level:   LevelWarning,
			Code:    CodeNoCapabilities,
			Message: "No capabilities",
		},
	}

	enhanced := EnhanceIssues(issues)

	if len(enhanced) != len(issues) {
		t.Errorf("Expected %d enhanced issues, got %d", len(issues), len(enhanced))
	}

	for i, e := range enhanced {
		if e.Code != issues[i].Code {
			t.Errorf("Issue %d: expected code %s, got %s", i, issues[i].Code, e.Code)
		}
	}
}

func TestAllKnownIssues_HaveSuggestions(t *testing.T) {
	catalog := NewIssueCatalog()

	// List of all issue codes that should be registered
	knownCodes := []string{
		"TRANSPORT_DETECTION_FAILED",
		"TRANSPORT_CREATION_FAILED",
		"SSE_CONNECTION_FAILED",
		CodeInitializeFailed,
		CodeInvalidProtocolVersion,
		CodeMissingServerInfo,
		CodeNoCapabilities,
		CodeMissingCapability,
		CodeToolsListFailed,
		CodeResourcesListFailed,
		CodePromptsListFailed,
		"RETRIES_EXHAUSTED",
	}

	for _, code := range knownCodes {
		t.Run(code, func(t *testing.T) {
			template, ok := catalog.GetTemplate(code)
			if !ok {
				t.Errorf("Issue code %s is not registered in catalog", code)
				return
			}

			if template.Code != code {
				t.Errorf("Template code mismatch: expected %s, got %s", code, template.Code)
			}

			if template.Title == "" {
				t.Error("Template should have a title")
			}

			if template.Description == "" {
				t.Error("Template should have a description")
			}

			if len(template.Suggestions) == 0 {
				t.Error("Template should have at least one suggestion")
			}

			if template.DocumentationURL == "" {
				t.Error("Template should have a documentation URL")
			}

			// Verify suggestions are actionable (contain verbs)
			foundActionable := false
			actionVerbs := []string{"verify", "check", "ensure", "update", "try", "increase", "review"}
			for _, suggestion := range template.Suggestions {
				lowerSuggestion := strings.ToLower(suggestion)
				for _, verb := range actionVerbs {
					if strings.Contains(lowerSuggestion, verb) {
						foundActionable = true
						break
					}
				}
				if foundActionable {
					break
				}
			}

			if !foundActionable {
				t.Errorf("Template suggestions should be actionable (contain action verbs): %v",
					template.Suggestions)
			}
		})
	}
}

func TestValidationResult_EnhanceIssues(t *testing.T) {
	// Note: Issues are now pre-enhanced during validation using newErrorIssue/newWarningIssue helpers.
	// This test verifies that EnhanceIssues() correctly converts pre-enhanced issues to EnhancedValidationIssue.

	result := &ValidationResult{
		Success: false,
		Issues: []ValidationIssue{
			{
				Level:            LevelError,
				Code:             "TRANSPORT_DETECTION_FAILED",
				Message:          "Could not detect transport",
				Suggestions:      []string{"Check server is running", "Verify endpoint URL"},
				DocumentationURL: "https://github.com/vitorbari/mcp-operator",
				RelatedIssues:    []string{"TRANSPORT_CREATION_FAILED"},
			},
			{
				Level:            LevelError,
				Code:             CodeInitializeFailed,
				Message:          "Initialize request failed",
				Suggestions:      []string{"Check server logs", "Verify protocol version"},
				DocumentationURL: "https://github.com/vitorbari/mcp-operator",
			},
		},
	}

	enhanced := result.EnhanceIssues()

	if len(enhanced) != len(result.Issues) {
		t.Errorf("Expected %d enhanced issues, got %d", len(result.Issues), len(enhanced))
	}

	for i, e := range enhanced {
		if e.Code != result.Issues[i].Code {
			t.Errorf("Issue %d: code mismatch", i)
		}

		if len(e.Suggestions) == 0 {
			t.Errorf("Issue %d: expected suggestions", i)
		}

		if e.DocumentationURL == "" {
			t.Errorf("Issue %d: expected documentation URL", i)
		}
	}
}

func TestValidationResult_EnhanceIssuesWithCatalog(t *testing.T) {
	// Note: EnhanceIssuesWithCatalog now ignores the custom catalog since issues
	// are pre-enhanced during validation. This test verifies backward compatibility.

	customCatalog := NewIssueCatalog()
	customCatalog.RegisterIssue(IssueTemplate{
		Code:        "CUSTOM_ERROR",
		Title:       "Custom Error",
		Description: "A custom error for testing",
		Suggestions: []string{"Custom suggestion 1", "Custom suggestion 2"},
	})

	// Create a result with pre-enhanced issues
	result := &ValidationResult{
		Success: false,
		Issues: []ValidationIssue{
			{
				Level:            LevelError,
				Code:             "TRANSPORT_DETECTION_FAILED",
				Message:          "Failed to detect transport",
				Suggestions:      []string{"Check server is running", "Verify endpoint URL"},
				DocumentationURL: "https://github.com/vitorbari/mcp-operator",
			},
		},
	}

	enhanced := result.EnhanceIssuesWithCatalog(customCatalog)

	if len(enhanced) != 1 {
		t.Fatalf("Expected 1 enhanced issue, got %d", len(enhanced))
	}

	// Verify that the method still works even though it ignores the custom catalog
	if enhanced[0].Code != "TRANSPORT_DETECTION_FAILED" {
		t.Errorf("Expected code TRANSPORT_DETECTION_FAILED, got %s", enhanced[0].Code)
	}

	if len(enhanced[0].Suggestions) == 0 {
		t.Error("Expected enhanced issue to have suggestions")
	}
}

func TestIssueCatalog_RelatedIssues(t *testing.T) {
	catalog := NewIssueCatalog()

	// Test that related issues are set correctly
	template, ok := catalog.GetTemplate("TRANSPORT_DETECTION_FAILED")
	if !ok {
		t.Fatal("Expected TRANSPORT_DETECTION_FAILED to exist")
	}

	if len(template.RelatedIssues) == 0 {
		t.Error("Expected TRANSPORT_DETECTION_FAILED to have related issues")
	}

	// Verify related issues reference other known issues
	for _, relatedCode := range template.RelatedIssues {
		_, exists := catalog.GetTemplate(relatedCode)
		if !exists {
			t.Errorf("Related issue %s is not in catalog", relatedCode)
		}
	}
}
