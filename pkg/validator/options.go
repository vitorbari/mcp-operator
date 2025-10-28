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
	"time"
)

// TimeoutConfig provides granular timeout controls for validation
type TimeoutConfig struct {
	// Overall timeout for the entire validation operation
	// If zero, uses validator's default timeout
	Overall time.Duration

	// Detection timeout for transport auto-detection
	// If zero, uses Overall timeout or validator default
	Detection time.Duration

	// Connection timeout for TCP connection establishment
	// If zero, uses 10 seconds
	Connection time.Duration

	// Request timeout for individual HTTP requests (initialize, list operations)
	// If zero, uses Overall timeout or validator default
	Request time.Duration

	// TLS handshake timeout for HTTPS connections
	// If zero, uses 10 seconds
	TLSHandshake time.Duration
}

// DefaultTimeoutConfig returns sensible default timeouts
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		Overall:      30 * time.Second,
		Detection:    10 * time.Second,
		Connection:   10 * time.Second,
		Request:      30 * time.Second,
		TLSHandshake: 10 * time.Second,
	}
}

// FastTimeoutConfig returns aggressive timeouts for fast-failing scenarios
func FastTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		Overall:      10 * time.Second,
		Detection:    3 * time.Second,
		Connection:   3 * time.Second,
		Request:      5 * time.Second,
		TLSHandshake: 3 * time.Second,
	}
}

// SlowTimeoutConfig returns generous timeouts for slow networks or startup delays
func SlowTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		Overall:      120 * time.Second,
		Detection:    30 * time.Second,
		Connection:   20 * time.Second,
		Request:      60 * time.Second,
		TLSHandshake: 20 * time.Second,
	}
}

// HTTPClientConfig configures HTTP client behavior
type HTTPClientConfig struct {
	// Transport is a custom http.RoundTripper
	// If nil, a default transport with connection pooling is created
	Transport http.RoundTripper

	// MaxIdleConns controls the maximum number of idle (keep-alive) connections across all hosts
	// Default: 100
	MaxIdleConns int

	// MaxIdleConnsPerHost controls the maximum idle connections per host
	// Default: 10
	MaxIdleConnsPerHost int

	// MaxConnsPerHost limits the total number of connections per host (including active)
	// Default: 0 (unlimited)
	MaxConnsPerHost int

	// IdleConnTimeout is the maximum time an idle connection stays in the pool
	// Default: 90 seconds
	IdleConnTimeout time.Duration

	// DisableKeepAlives, if true, disables HTTP keep-alives and will only use connections once
	// Default: false (keep-alives enabled for connection reuse)
	DisableKeepAlives bool

	// DisableCompression, if true, prevents the Transport from requesting compression
	// Default: false
	DisableCompression bool
}

// DefaultHTTPClientConfig returns HTTP client configuration optimized for connection reuse
func DefaultHTTPClientConfig() HTTPClientConfig {
	return HTTPClientConfig{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     0, // unlimited
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
		DisableCompression:  false,
	}
}

// HighVolumeHTTPClientConfig returns configuration optimized for high-volume validation
// Suitable when validating many servers concurrently
func HighVolumeHTTPClientConfig() HTTPClientConfig {
	return HTTPClientConfig{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 20,
		MaxConnsPerHost:     0,
		IdleConnTimeout:     120 * time.Second,
		DisableKeepAlives:   false,
		DisableCompression:  false,
	}
}

// ValidatorConfig provides comprehensive validator configuration
type ValidatorConfig struct {
	// BaseURL is the server base URL
	BaseURL string

	// Timeouts configures timeout behavior
	Timeouts TimeoutConfig

	// HTTPClient configures HTTP client behavior
	HTTPClient HTTPClientConfig

	// MetricsRecorder for custom metrics collection
	// If nil, uses default Prometheus recorder
	MetricsRecorder MetricsRecorder

	// TransportFactory for custom transport implementations
	// If nil, uses default factory
	TransportFactory TransportFactory
}

// DefaultValidatorConfig returns a validator configuration with sensible defaults
func DefaultValidatorConfig(baseURL string) ValidatorConfig {
	return ValidatorConfig{
		BaseURL:          baseURL,
		Timeouts:         DefaultTimeoutConfig(),
		HTTPClient:       DefaultHTTPClientConfig(),
		MetricsRecorder:  nil, // Will use default
		TransportFactory: nil, // Will use default
	}
}

// NewValidatorWithConfig creates a validator from comprehensive configuration
// Note: Consider using NewValidator with functional options for better composability
func NewValidatorWithConfig(config ValidatorConfig) *Validator {
	opts := []Option{}

	if config.Timeouts.Overall > 0 {
		opts = append(opts, WithTimeout(config.Timeouts.Overall))
	}

	if config.TransportFactory != nil {
		opts = append(opts, WithFactory(config.TransportFactory))
	}

	if config.MetricsRecorder != nil {
		opts = append(opts, WithMetricsRecorder(config.MetricsRecorder))
	}

	// For HTTPClient config, create a custom client if needed
	if config.HTTPClient.MaxIdleConns > 0 || config.HTTPClient.Transport != nil {
		httpClient := createHTTPClient(config.Timeouts, config.HTTPClient)
		opts = append(opts, WithHTTPClient(httpClient))
	}

	return NewValidator(config.BaseURL, opts...)
}

// createHTTPClient creates an HTTP client with optimized settings
func createHTTPClient(timeouts TimeoutConfig, clientConfig HTTPClientConfig) *http.Client {
	// Use custom transport if provided
	if clientConfig.Transport != nil {
		return &http.Client{
			Transport: clientConfig.Transport,
			Timeout:   timeouts.Request,
		}
	}

	// Create transport with connection pooling
	transport := &http.Transport{
		MaxIdleConns:        clientConfig.MaxIdleConns,
		MaxIdleConnsPerHost: clientConfig.MaxIdleConnsPerHost,
		MaxConnsPerHost:     clientConfig.MaxConnsPerHost,
		IdleConnTimeout:     clientConfig.IdleConnTimeout,
		DisableKeepAlives:   clientConfig.DisableKeepAlives,
		DisableCompression:  clientConfig.DisableCompression,

		// Timeout settings
		DialContext: (&TimeoutDialer{
			Timeout: timeouts.Connection,
		}).DialContext,
		TLSHandshakeTimeout: timeouts.TLSHandshake,

		// Response header timeout
		ResponseHeaderTimeout: timeouts.Request,

		// Expect continue timeout
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeouts.Request,
	}
}

// ApplyTimeouts applies timeout configuration to validation options
func (opts *ValidationOptions) ApplyTimeouts(timeouts TimeoutConfig) {
	if timeouts.Overall > 0 {
		opts.Timeout = timeouts.Overall
	}
}

// WithTimeouts returns a copy of ValidationOptions with timeouts applied
func (opts ValidationOptions) WithTimeouts(timeouts TimeoutConfig) ValidationOptions {
	if timeouts.Overall > 0 {
		opts.Timeout = timeouts.Overall
	}
	return opts
}

// WithStrictMode returns a copy of ValidationOptions with strict mode enabled
func (opts ValidationOptions) WithStrictMode() ValidationOptions {
	opts.StrictMode = true
	return opts
}

// WithRequiredCapabilities returns a copy with required capabilities set
func (opts ValidationOptions) WithRequiredCapabilities(capabilities ...string) ValidationOptions {
	opts.RequiredCapabilities = capabilities
	return opts
}

// WithTransport returns a copy with explicit transport specified
func (opts ValidationOptions) WithTransport(transport TransportType) ValidationOptions {
	opts.Transport = transport
	return opts
}

// WithPath returns a copy with custom endpoint path
func (opts ValidationOptions) WithPath(path string) ValidationOptions {
	opts.ConfiguredPath = path
	return opts
}
