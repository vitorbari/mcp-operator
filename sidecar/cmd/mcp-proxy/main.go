// Package main is the entry point for the MCP metrics sidecar proxy.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/vitorbari/mcp-operator/sidecar/pkg/config"
	"github.com/vitorbari/mcp-operator/sidecar/pkg/proxy"
)

func main() {
	// Parse configuration from flags
	cfg := config.ParseFlags()

	// Setup structured logging
	logger := cfg.SetupLogger()

	// Log startup configuration
	logger.Info("starting MCP metrics sidecar proxy",
		slog.String("listen_addr", cfg.ListenAddr),
		slog.String("target_addr", cfg.TargetAddr),
		slog.String("metrics_addr", cfg.MetricsAddr),
		slog.String("log_level", cfg.LogLevel),
	)

	// Create the proxy
	p, err := proxy.New(cfg.ListenAddr, cfg.TargetAddr, logger)
	if err != nil {
		logger.Error("failed to create proxy", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Setup context with signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logger.Info("received shutdown signal", slog.String("signal", sig.String()))
		cancel()
	}()

	// Start the proxy (blocking)
	logger.Info("proxy configured",
		slog.String("listen_addr", p.ListenAddr()),
		slog.String("target", p.TargetURL().String()),
	)

	if err := p.Start(ctx); err != nil && err != context.Canceled {
		logger.Error("proxy error", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("proxy shutdown complete")
}
