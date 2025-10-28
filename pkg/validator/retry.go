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
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// RetryConfig configures retry behavior for validation operations
type RetryConfig struct {
	// MaxAttempts is the maximum number of validation attempts
	MaxAttempts int

	// InitialDelay is the delay before the first retry
	InitialDelay time.Duration

	// MaxDelay is the maximum delay between retries
	MaxDelay time.Duration

	// Multiplier is the factor by which the delay increases after each retry
	Multiplier float64

	// RetryableErrors are error message patterns that should trigger a retry
	RetryableErrors []string
}

// DefaultRetryConfig returns sensible defaults for retry behavior
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		RetryableErrors: []string{
			"connection refused",
			"connection reset",
			"timeout",
			"temporary failure",
			"dial tcp",
			"i/o timeout",
			"EOF",
			"transport detection failed",
			"failed to connect",
			"no such host",
		},
	}
}

// NoRetryConfig returns a config that disables retries
func NoRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:     1,
		InitialDelay:    0,
		MaxDelay:        0,
		Multiplier:      1.0,
		RetryableErrors: []string{},
	}
}

// RetryableValidator wraps a validator with retry logic
// This makes validation robust against temporary network issues and server startup delays
type RetryableValidator struct {
	validator       *Validator
	config          RetryConfig
	metricsRecorder MetricsRecorder
}

// NewRetryableValidator creates a validator with retry logic
func NewRetryableValidator(validator *Validator, config RetryConfig) *RetryableValidator {
	return &RetryableValidator{
		validator:       validator,
		config:          config,
		metricsRecorder: NewMetricsRecorder(),
	}
}

// NewRetryableValidatorWithDefaults creates a validator with default retry configuration
func NewRetryableValidatorWithDefaults(validator *Validator) *RetryableValidator {
	return NewRetryableValidator(validator, DefaultRetryConfig())
}

// Validate attempts validation with retry logic
// This method handles transient failures gracefully using exponential backoff
func (r *RetryableValidator) Validate(ctx context.Context, opts ValidationOptions) (*ValidationResult, error) {
	var result *ValidationResult
	var lastErr error

	// If retries are disabled, just call the validator once
	if r.config.MaxAttempts <= 1 {
		return r.validator.Validate(ctx, opts)
	}

	logger := log.FromContext(ctx)

	backoff := wait.Backoff{
		Duration: r.config.InitialDelay,
		Factor:   r.config.Multiplier,
		Steps:    r.config.MaxAttempts,
		Cap:      r.config.MaxDelay,
	}

	attempt := 0
	retryCount := 0

	err := wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		attempt++

		logger.V(1).Info("Attempting validation",
			"attempt", attempt,
			"maxAttempts", r.config.MaxAttempts,
		)

		result, lastErr = r.validator.Validate(ctx, opts)

		// Success - validation passed
		if lastErr == nil && result != nil && result.Success {
			if retryCount > 0 {
				logger.Info("Validation succeeded after retries",
					"attempts", attempt,
					"retries", retryCount,
				)
			}
			return true, nil
		}

		// Check if we should retry
		shouldRetry := r.isRetryable(lastErr, result)

		if shouldRetry && attempt < r.config.MaxAttempts {
			retryCount++
			nextDelay := r.calculateNextDelay(retryCount)

			logger.Info("Validation failed, retrying...",
				"attempt", attempt,
				"maxAttempts", r.config.MaxAttempts,
				"error", r.formatError(lastErr, result),
				"nextDelay", nextDelay,
			)

			// Return false to trigger retry
			return false, nil
		}

		// Non-retryable error or max attempts reached
		if attempt >= r.config.MaxAttempts {
			logger.Info("Max retry attempts reached",
				"attempts", attempt,
				"lastError", r.formatError(lastErr, result),
			)
		} else {
			logger.V(1).Info("Non-retryable error encountered",
				"error", r.formatError(lastErr, result),
			)
		}

		// Return true to stop retrying (but we still have an error)
		return true, nil
	})

	// If context was cancelled or deadline exceeded during backoff
	if err != nil {
		return nil, err
	}

	// Return the last validation result (might be failed)
	if result != nil && !result.Success && retryCount > 0 {
		// Add a note about retries to the result
		result.Issues = append(result.Issues, ValidationIssue{
			Level:   LevelInfo,
			Code:    "RETRIES_EXHAUSTED",
			Message: "Validation failed after multiple retry attempts",
		})
	}

	// Record retry metrics if retries occurred
	if retryCount > 0 {
		transportName := string(result.DetectedTransport)
		if transportName == "" {
			transportName = "unknown"
		}
		r.metricsRecorder.RecordRetries(transportName, retryCount)
	}

	return result, lastErr
}

// SetMetricsRecorder allows replacing the metrics recorder (useful for testing)
func (r *RetryableValidator) SetMetricsRecorder(recorder MetricsRecorder) {
	r.metricsRecorder = recorder
}

// isRetryable determines if an error or validation failure should trigger a retry
func (r *RetryableValidator) isRetryable(err error, result *ValidationResult) bool {
	// Check error messages
	if err != nil {
		errStr := err.Error()
		for _, retryableErr := range r.config.RetryableErrors {
			if strings.Contains(strings.ToLower(errStr), strings.ToLower(retryableErr)) {
				return true
			}
		}
	}

	// Check validation result issues
	if result != nil && len(result.Issues) > 0 {
		for _, issue := range result.Issues {
			// Retry on transport detection failures
			if issue.Code == "TRANSPORT_DETECTION_FAILED" ||
				issue.Code == "TRANSPORT_CREATION_FAILED" ||
				issue.Code == "SSE_CONNECTION_FAILED" {
				return true
			}

			// Check if issue message contains retryable patterns
			for _, retryableErr := range r.config.RetryableErrors {
				if strings.Contains(strings.ToLower(issue.Message), strings.ToLower(retryableErr)) {
					return true
				}
			}
		}
	}

	return false
}

// calculateNextDelay calculates the next retry delay based on the retry count
func (r *RetryableValidator) calculateNextDelay(retryCount int) time.Duration {
	delay := time.Duration(float64(r.config.InitialDelay) * float64(retryCount) * r.config.Multiplier)
	if delay > r.config.MaxDelay {
		delay = r.config.MaxDelay
	}
	return delay
}

// formatError formats an error and validation result for logging
func (r *RetryableValidator) formatError(err error, result *ValidationResult) string {
	if err != nil {
		return err.Error()
	}

	if result != nil && len(result.Issues) > 0 {
		// Return the first error message
		for _, issue := range result.Issues {
			if issue.Level == LevelError {
				return issue.Message
			}
		}
		// If no errors, return first issue
		return result.Issues[0].Message
	}

	return "unknown error"
}

// GetConfig returns the retry configuration
func (r *RetryableValidator) GetConfig() RetryConfig {
	return r.config
}

// GetValidator returns the underlying validator
func (r *RetryableValidator) GetValidator() *Validator {
	return r.validator
}
