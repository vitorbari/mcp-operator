// Package proxy provides the core reverse proxy logic for the MCP metrics sidecar.
package proxy

import (
	"bytes"
	"context"
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

		// Wrap the response writer to capture the status code and response body
		rc := newResponseCapture(w, DefaultMaxBodyCapture)

		// Process the request
		next.ServeHTTP(rc, req)

		// Calculate duration
		duration := time.Since(start)

		// Parse request and response for MCP metrics
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

		// Parse response if it's JSON
		respBody := rc.Body()
		if len(respBody) > 0 && isJSONContentType(rc.Header().Get("Content-Type")) {
			var err error
			parsedResp, err = mcp.ParseResponse(respBody)
			if err != nil {
				p.logger.Debug("failed to parse MCP response",
					slog.String("error", err.Error()),
				)
			}
		}

		// Log the request
		p.logger.Info("request",
			slog.String("http_method", req.Method),
			slog.String("path", req.URL.Path),
			slog.String("query", req.URL.RawQuery),
			slog.String("mcp_method", mcpMethod),
			slog.Int("status", rc.StatusCode()),
			slog.Duration("duration", duration),
			slog.String("client_ip", getClientIP(req)),
			slog.Int64("request_bytes", reqSize),
			slog.Int64("response_bytes", rc.BytesWritten()),
		)

		// Record metrics
		if p.recorder != nil {
			p.recorder.RecordRequest(ctx, mcpMethod, rc.StatusCode(), duration, reqSize, rc.BytesWritten())

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

			// Record errors
			if parsedResp != nil && parsedResp.IsError {
				p.recorder.RecordError(ctx, mcpMethod, parsedResp.ErrorCode)
			}
		}
	})
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
	p.server = &http.Server{
		Addr:         p.listenAddr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
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

// ListenAddr returns the address the proxy is configured to listen on.
func (p *Proxy) ListenAddr() string {
	return p.listenAddr
}

// TargetURL returns the target URL the proxy forwards requests to.
func (p *Proxy) TargetURL() *url.URL {
	return p.target
}
