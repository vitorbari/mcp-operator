// Package proxy provides the core reverse proxy logic for the MCP metrics sidecar.
package proxy

import (
	"context"
	"log/slog"
	"net/url"
)

// Proxy is the MCP reverse proxy that intercepts traffic for metrics collection.
type Proxy struct {
	// listenAddr is the address the proxy listens on.
	listenAddr string

	// target is the URL of the MCP server to proxy requests to.
	target *url.URL

	// logger is the structured logger for the proxy.
	logger *slog.Logger
}

// New creates a new Proxy instance.
func New(listenAddr, targetAddr string, logger *slog.Logger) (*Proxy, error) {
	target, err := url.Parse(targetAddr)
	if err != nil {
		return nil, err
	}

	// If no scheme is provided, assume http
	if target.Scheme == "" {
		target.Scheme = "http"
		target, err = url.Parse(target.Scheme + "://" + targetAddr)
		if err != nil {
			return nil, err
		}
	}

	return &Proxy{
		listenAddr: listenAddr,
		target:     target,
		logger:     logger,
	}, nil
}

// Start starts the proxy server and blocks until the context is cancelled.
// This is a stub implementation that will be expanded in Task 2.
func (p *Proxy) Start(ctx context.Context) error {
	p.logger.Info("proxy start requested",
		slog.String("listen_addr", p.listenAddr),
		slog.String("target", p.target.String()),
	)

	// TODO: Implement reverse proxy in Task 2
	// This stub just waits for context cancellation
	<-ctx.Done()
	return ctx.Err()
}

// ListenAddr returns the address the proxy is configured to listen on.
func (p *Proxy) ListenAddr() string {
	return p.listenAddr
}

// TargetURL returns the target URL the proxy forwards requests to.
func (p *Proxy) TargetURL() *url.URL {
	return p.target
}
