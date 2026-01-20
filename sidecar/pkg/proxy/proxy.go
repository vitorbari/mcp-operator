// Package proxy provides the core reverse proxy logic for the MCP metrics sidecar.
package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/vitorbari/mcp-operator/sidecar/pkg/mcp"
	"github.com/vitorbari/mcp-operator/sidecar/pkg/metrics"
)

// Proxy is the MCP reverse proxy that intercepts traffic for metrics collection.
type Proxy struct {
	// listenAddr is the address the proxy listens on.
	listenAddr string

	// target is the URL of the MCP server to proxy requests to.
	target *url.URL

	// reverseProxy is the underlying HTTP reverse proxy.
	reverseProxy *httputil.ReverseProxy

	// server is the HTTP server.
	server *http.Server

	// logger is the structured logger for the proxy.
	logger *slog.Logger

	// recorder is the metrics recorder (optional, can be nil).
	recorder *metrics.Recorder
}

// New creates a new Proxy instance.
func New(listenAddr, targetAddr string, logger *slog.Logger) (*Proxy, error) {
	return NewWithRecorder(listenAddr, targetAddr, logger, nil)
}

// NewWithRecorder creates a new Proxy instance with an optional metrics recorder.
func NewWithRecorder(listenAddr, targetAddr string, logger *slog.Logger, recorder *metrics.Recorder) (*Proxy, error) {
	// If no scheme is provided, assume http
	// url.Parse treats "localhost:3001" as scheme "localhost" and opaque "3001"
	// so we need to check for a valid scheme before parsing
	if !strings.Contains(targetAddr, "://") {
		targetAddr = "http://" + targetAddr
	}

	target, err := url.Parse(targetAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse target address: %w", err)
	}

	p := &Proxy{
		listenAddr: listenAddr,
		target:     target,
		logger:     logger,
		recorder:   recorder,
	}

	// Create the reverse proxy
	p.reverseProxy = p.createReverseProxy()

	return p, nil
}

// createReverseProxy creates and configures the httputil.ReverseProxy.
func (p *Proxy) createReverseProxy() *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(p.target)

	// Customize the Director to handle headers properly
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		p.modifyRequest(req)
	}

	// Custom error handler for target unavailable
	proxy.ErrorHandler = p.errorHandler

	// Custom transport with reasonable timeouts
	proxy.Transport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Enable immediate flushing for SSE support
	proxy.FlushInterval = -1

	return proxy
}

// modifyRequest modifies the outgoing request to the target.
func (p *Proxy) modifyRequest(req *http.Request) {
	// Set the Host header to the target host
	req.Host = p.target.Host

	// Handle X-Forwarded-For header
	clientIP := getClientIP(req)
	if prior := req.Header.Get("X-Forwarded-For"); prior != "" {
		clientIP = prior + ", " + clientIP
	}
	req.Header.Set("X-Forwarded-For", clientIP)

	// Set X-Real-IP if not already set
	if req.Header.Get("X-Real-IP") == "" {
		req.Header.Set("X-Real-IP", getClientIP(req))
	}

	// Set X-Forwarded-Proto
	scheme := "http"
	if req.TLS != nil {
		scheme = "https"
	}
	req.Header.Set("X-Forwarded-Proto", scheme)

	// Set X-Forwarded-Host
	if req.Header.Get("X-Forwarded-Host") == "" {
		req.Header.Set("X-Forwarded-Host", req.Host)
	}
}

// errorHandler handles errors when proxying to the target.
func (p *Proxy) errorHandler(w http.ResponseWriter, req *http.Request, err error) {
	p.logger.Error("proxy error",
		slog.String("method", req.Method),
		slog.String("path", req.URL.Path),
		slog.String("error", err.Error()),
	)

	// Return 502 Bad Gateway for target unavailable
	w.WriteHeader(http.StatusBadGateway)
	fmt.Fprintf(w, "proxy error: %v", err)
}

// getClientIP extracts the client IP from the request.
func getClientIP(req *http.Request) string {
	// Try to get IP from X-Real-IP header first
	if ip := req.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}

	// Then try X-Forwarded-For
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}
	return ip
}

// metricsMiddleware wraps a handler and records metrics for each request.
func (p *Proxy) metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		ctx := req.Context()

		// Track active connections
		if p.recorder != nil {
			p.recorder.IncrementConnections(ctx)
			defer p.recorder.DecrementConnections(ctx)
		}

		// Capture request body for JSON parsing
		var reqBody []byte
		var reqSize int64
		if req.Body != nil && isJSONContentType(req.Header.Get("Content-Type")) {
			bodyBytes, err := io.ReadAll(req.Body)
			if err == nil {
				reqBody = bodyBytes
				reqSize = int64(len(bodyBytes))
				// Restore the body for the reverse proxy
				req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			}
		} else {
			// Use Content-Length for non-JSON requests
			reqSize = req.ContentLength
			if reqSize < 0 {
				reqSize = 0
			}
		}

		// Use SSE-aware response writer that can detect and handle SSE responses
		sw := newSSEAwareWriter(w, p.recorder, req.Method)

		// Defer SSE connection close handling.
		// IMPORTANT: This must be in a defer because for SSE connections,
		// ServeHTTP blocks until the client disconnects. When the client disconnects,
		// the http.Server may not return normally from ServeHTTP, but defers still run.
		// This ensures SSE connection close metrics are always recorded.
		defer func() {
			if sw.isSSE {
				sw.recordSSEClose()
			}
		}()

		// Process the request
		next.ServeHTTP(sw, req)

		// Calculate duration (for non-SSE requests that reach this point)
		duration := time.Since(start)

		// Parse request for MCP method (always, regardless of SSE)
		mcpMethod := "unknown"
		var parsedReq *mcp.ParsedRequest
		var parsedResp *mcp.ParsedResponse

		if len(reqBody) > 0 {
			var err error
			parsedReq, err = mcp.ParseRequest(reqBody)
			if err != nil {
				p.logger.Debug("failed to parse MCP request",
					slog.String("error", err.Error()),
				)
			} else {
				mcpMethod = parsedReq.Method
			}
		}

		// Parse response if it's JSON (skip for SSE as body is streamed)
		if !sw.isSSE {
			respBody := sw.Body()
			if len(respBody) > 0 && isJSONContentType(sw.Header().Get("Content-Type")) {
				var err error
				parsedResp, err = mcp.ParseResponse(respBody)
				if err != nil {
					p.logger.Debug("failed to parse MCP response",
						slog.String("error", err.Error()),
					)
				}
			}
		}

		// Log the request
		p.logger.Info("request",
			slog.String("http_method", req.Method),
			slog.String("path", req.URL.Path),
			slog.String("query", req.URL.RawQuery),
			slog.String("mcp_method", mcpMethod),
			slog.Int("status", sw.statusCode),
			slog.Duration("duration", duration),
			slog.String("client_ip", getClientIP(req)),
			slog.Int64("request_bytes", reqSize),
			slog.Int64("response_bytes", sw.bytesWritten),
			slog.Bool("sse", sw.isSSE),
		)

		// Record metrics
		if p.recorder != nil {
			p.recorder.RecordRequest(ctx, mcpMethod, sw.statusCode, duration, reqSize, sw.bytesWritten)

			// Record MCP-specific metrics
			if parsedReq != nil {
				// Record tool calls
				if parsedReq.Method == mcp.MethodToolsCall && parsedReq.ToolName != "" {
					p.recorder.RecordToolCall(ctx, parsedReq.ToolName)
				}

				// Record resource reads
				if parsedReq.Method == mcp.MethodResourcesRead && parsedReq.ResourceURI != "" {
					p.recorder.RecordResourceRead(ctx, parsedReq.ResourceURI)
				}
			}

			// Record errors (from JSON response only, SSE errors are tracked separately)
			if parsedResp != nil && parsedResp.IsError {
				p.recorder.RecordError(ctx, mcpMethod, parsedResp.ErrorCode)
			}
		}
	})
}

// sseAwareWriter is a response writer that detects SSE responses and handles them appropriately.
type sseAwareWriter struct {
	http.ResponseWriter
	recorder     *metrics.Recorder
	httpMethod   string // HTTP method (GET, POST, etc.) - used to distinguish SSE streams from Streamable HTTP
	statusCode   int
	bytesWritten int64
	isSSE        bool
	body         bytes.Buffer
	maxCapture   int
	startTime    time.Time
	headersSent  bool
	// SSE event accumulator (per-connection)
	sseEventType string
	sseEventData strings.Builder
}

// newSSEAwareWriter creates a new SSE-aware response writer.
// The httpMethod is used to distinguish long-lived SSE streams (GET) from
// Streamable HTTP request-responses (POST) that also use text/event-stream content type.
func newSSEAwareWriter(w http.ResponseWriter, recorder *metrics.Recorder, httpMethod string) *sseAwareWriter {
	return &sseAwareWriter{
		ResponseWriter: w,
		recorder:       recorder,
		httpMethod:     httpMethod,
		statusCode:     http.StatusOK,
		maxCapture:     DefaultMaxBodyCapture,
		startTime:      time.Now(),
	}
}

// WriteHeader captures the status code and detects SSE responses.
func (sw *sseAwareWriter) WriteHeader(code int) {
	if sw.headersSent {
		return
	}
	sw.statusCode = code
	sw.headersSent = true

	// Check if this is an SSE response
	contentType := sw.Header().Get("Content-Type")
	sw.isSSE = IsSSEContentType(contentType)

	// Only record SSE connection metrics for GET requests (true long-lived SSE streams).
	// POST requests with text/event-stream content type are Streamable HTTP request-responses,
	// not long-lived SSE connections, and should not be counted as SSE connections.
	if sw.isSSE && sw.recorder != nil && sw.httpMethod == http.MethodGet {
		sw.recorder.SSEConnectionOpened(context.Background())
	}

	sw.ResponseWriter.WriteHeader(code)
}

// Write captures the response and handles SSE event parsing.
func (sw *sseAwareWriter) Write(b []byte) (int, error) {
	if !sw.headersSent {
		sw.WriteHeader(http.StatusOK)
	}

	n, err := sw.ResponseWriter.Write(b)
	sw.bytesWritten += int64(n)

	if sw.isSSE && sw.httpMethod == http.MethodGet {
		// For true SSE streams (GET requests only), parse events from the data being written.
		// POST requests with SSE content type are Streamable HTTP request-responses,
		// not long-lived SSE streams, so we don't track SSE event metrics for them.
		sw.parseSSEData(b)
	} else if !sw.isSSE {
		// For non-SSE, capture body for later parsing
		if sw.body.Len() < sw.maxCapture {
			remaining := sw.maxCapture - sw.body.Len()
			if len(b) <= remaining {
				sw.body.Write(b)
			} else {
				sw.body.Write(b[:remaining])
			}
		}
	}

	return n, err
}

// Flush implements http.Flusher for SSE streaming support.
func (sw *sseAwareWriter) Flush() {
	if flusher, ok := sw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Body returns the captured response body (for non-SSE responses).
func (sw *sseAwareWriter) Body() []byte {
	return sw.body.Bytes()
}

// parseSSEData parses SSE events from the written data and records metrics.
func (sw *sseAwareWriter) parseSSEData(data []byte) {
	if sw.recorder == nil {
		return
	}

	// Parse line by line
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")

		if strings.HasPrefix(line, "event:") {
			sw.sseEventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			dataContent := strings.TrimPrefix(line, "data:")
			if sw.sseEventData.Len() > 0 {
				sw.sseEventData.WriteString("\n")
			}
			sw.sseEventData.WriteString(dataContent)
		} else if line == "" && (sw.sseEventType != "" || sw.sseEventData.Len() > 0) {
			// Empty line marks end of event
			eventType := sw.sseEventType
			if eventType == "" {
				eventType = "message"
			}

			ctx := context.Background()
			sw.recorder.SSEEventReceived(ctx, eventType)

			// Try to parse MCP data
			if sw.sseEventData.Len() > 0 {
				dataStr := strings.TrimSpace(sw.sseEventData.String())
				sw.parseMCPFromSSE(ctx, dataStr)
			}

			// Reset accumulator
			sw.sseEventType = ""
			sw.sseEventData.Reset()
		}
	}
}

// parseMCPFromSSE attempts to parse MCP JSON-RPC from SSE data.
func (sw *sseAwareWriter) parseMCPFromSSE(ctx context.Context, data string) {
	if sw.recorder == nil {
		return
	}

	// Try parsing as request
	if req, err := mcp.ParseRequest([]byte(data)); err == nil {
		if req.Method == mcp.MethodToolsCall && req.ToolName != "" {
			sw.recorder.RecordToolCall(ctx, req.ToolName)
		}
		if req.Method == mcp.MethodResourcesRead && req.ResourceURI != "" {
			sw.recorder.RecordResourceRead(ctx, req.ResourceURI)
		}
		return
	}

	// Try parsing as response
	if resp, err := mcp.ParseResponse([]byte(data)); err == nil {
		if resp.IsError {
			sw.recorder.RecordError(ctx, "sse", resp.ErrorCode)
		}
	}
}

// recordSSEClose is called when the SSE connection is closed to record metrics.
// Only records for GET requests (true long-lived SSE streams), matching the logic in WriteHeader.
func (sw *sseAwareWriter) recordSSEClose() {
	if sw.isSSE && sw.recorder != nil && sw.httpMethod == http.MethodGet {
		sw.recorder.SSEConnectionClosed(context.Background(), time.Since(sw.startTime))
	}
}

// isJSONContentType checks if the content type indicates JSON.
func isJSONContentType(contentType string) bool {
	return strings.Contains(contentType, "application/json")
}

// loggingResponseWriter wraps http.ResponseWriter to capture status code and bytes written.
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

// WriteHeader captures the status code.
func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

// Write captures the number of bytes written.
func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	n, err := lrw.ResponseWriter.Write(b)
	lrw.bytesWritten += int64(n)
	return n, err
}

// Flush implements http.Flusher for streaming support.
func (lrw *loggingResponseWriter) Flush() {
	if flusher, ok := lrw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Start starts the proxy server and blocks until the context is cancelled.
func (p *Proxy) Start(ctx context.Context) error {
	// Create the HTTP handler with metrics middleware
	handler := p.metricsMiddleware(p.reverseProxy)

	// Create the HTTP server
	// Note: WriteTimeout is set to 0 to support long-lived SSE connections.
	// SSE connections can last indefinitely and would be killed by a write timeout.
	p.server = &http.Server{
		Addr:        p.listenAddr,
		Handler:     handler,
		ReadTimeout: 30 * time.Second,
		IdleTimeout: 120 * time.Second,
	}

	// Channel to capture server errors
	errChan := make(chan error, 1)

	// Start the server in a goroutine
	go func() {
		p.logger.Info("starting HTTP server",
			slog.String("listen_addr", p.listenAddr),
			slog.String("target", p.target.String()),
		)
		if err := p.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		}
		close(errChan)
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		p.logger.Info("shutting down proxy server")
		// Create a shutdown context with timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := p.server.Shutdown(shutdownCtx); err != nil {
			p.logger.Error("error during server shutdown", slog.String("error", err.Error()))
			return err
		}
		p.logger.Info("proxy server stopped gracefully")
		return ctx.Err()

	case err := <-errChan:
		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	}
}

// StartWithTLS starts the proxy server with TLS and blocks until the context is cancelled.
func (p *Proxy) StartWithTLS(ctx context.Context, tlsConfig *tls.Config, certFile, keyFile string) error {
	// Create the HTTP handler with metrics middleware
	handler := p.metricsMiddleware(p.reverseProxy)

	// Create the HTTPS server
	// Note: WriteTimeout is set to 0 to support long-lived SSE connections.
	// SSE connections can last indefinitely and would be killed by a write timeout.
	p.server = &http.Server{
		Addr:        p.listenAddr,
		Handler:     handler,
		TLSConfig:   tlsConfig,
		ReadTimeout: 30 * time.Second,
		IdleTimeout: 120 * time.Second,
	}

	// Channel to capture server errors
	errChan := make(chan error, 1)

	// Start the server in a goroutine
	go func() {
		p.logger.Info("starting HTTPS server",
			slog.String("listen_addr", p.listenAddr),
			slog.String("target", p.target.String()),
		)
		if err := p.server.ListenAndServeTLS(certFile, keyFile); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		}
		close(errChan)
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		p.logger.Info("shutting down proxy server")
		// Create a shutdown context with timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := p.server.Shutdown(shutdownCtx); err != nil {
			p.logger.Error("error during server shutdown", slog.String("error", err.Error()))
			return err
		}
		p.logger.Info("proxy server stopped gracefully")
		return ctx.Err()

	case err := <-errChan:
		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	}
}

// ListenAddr returns the address the proxy is configured to listen on.
func (p *Proxy) ListenAddr() string {
	return p.listenAddr
}

// TargetURL returns the target URL the proxy forwards requests to.
func (p *Proxy) TargetURL() *url.URL {
	return p.target
}
