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
	"fmt"

	"github.com/vitorbari/mcp-operator/pkg/validator"
)

// ExampleEnhancedValidationIssue demonstrates how to use enhanced error messages
func ExampleEnhancedValidationIssue() {
	// Create a validator
	v := validator.NewValidator("http://localhost:8080")

	// Wrap with retry logic
	retryable := validator.NewRetryableValidatorWithDefaults(v)

	// Validate (this will fail since there's no server)
	result, _ := retryable.Validate(context.Background(), validator.ValidationOptions{})

	// Get enhanced issues with suggestions
	if !result.Success {
		enhanced := result.EnhanceIssues()

		for _, issue := range enhanced {
			fmt.Printf("Issue: %s\n", issue.Code)
			if len(issue.Suggestions) > 0 {
				fmt.Println("Suggestions:")
				for i, suggestion := range issue.Suggestions {
					if i < 2 { // Show first 2 suggestions for brevity
						fmt.Printf("  - %s\n", suggestion)
					}
				}
			}
		}
	}

	// Output will vary based on the error, but shows the pattern
	// Output:
}

// ExampleIssueCatalog_Enhance demonstrates using the issue catalog directly
func ExampleIssueCatalog_Enhance() {
	catalog := validator.NewIssueCatalog()

	issue := validator.ValidationIssue{
		Level:   validator.LevelError,
		Code:    "TRANSPORT_DETECTION_FAILED",
		Message: "Failed to detect transport type",
	}

	enhanced := catalog.Enhance(issue)

	fmt.Printf("Code: %s\n", enhanced.Code)
	fmt.Printf("Has suggestions: %v\n", len(enhanced.Suggestions) > 0)
	fmt.Printf("Has docs: %v\n", enhanced.DocumentationURL != "")

	// Output:
	// Code: TRANSPORT_DETECTION_FAILED
	// Has suggestions: true
	// Has docs: true
}

// ExampleEnhancedValidationIssue_String shows formatted error output
func ExampleEnhancedValidationIssue_String() {
	issue := validator.ValidationIssue{
		Level:   validator.LevelError,
		Code:    validator.CodeInitializeFailed,
		Message: "Initialize request failed",
	}

	enhanced := validator.EnhanceIssue(issue)

	// The String() method provides a comprehensive formatted output
	output := enhanced.String()

	// Output includes:
	// - Error level and code
	// - Original message
	// - Numbered suggestions
	// - Documentation URL
	// - Related issue codes
	fmt.Printf("Output length > 500: %v\n", len(output) > 500)
	fmt.Printf("Contains suggestions: %v\n", len(enhanced.Suggestions) > 0)

	// Output:
	// Output length > 500: true
	// Contains suggestions: true
}

// Example_retryableValidatorWithEnhancedErrors shows combining retry and enhanced errors
func Example_retryableValidatorWithEnhancedErrors() {
	// Create validator with custom retry config
	v := validator.NewValidator("http://localhost:8080")

	retryConfig := validator.RetryConfig{
		MaxAttempts:  2,
		InitialDelay: 100,
		MaxDelay:     500,
		Multiplier:   2.0,
	}

	retryable := validator.NewRetryableValidator(v, retryConfig)

	// Attempt validation
	result, _ := retryable.Validate(context.Background(), validator.ValidationOptions{})

	// Check for enhanced error information
	if !result.Success {
		enhanced := result.EnhanceIssues()

		fmt.Printf("Validation failed with %d issues\n", len(enhanced))

		// Each issue has actionable suggestions
		for _, issue := range enhanced {
			if issue.Code == "RETRIES_EXHAUSTED" {
				fmt.Println("Retries were exhausted")
				fmt.Printf("Suggestions available: %d\n", len(issue.Suggestions))
			}
		}
	}

	// Output will vary, shows the pattern
	// Output:
}
