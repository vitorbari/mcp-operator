// Package main is the entry point for the MCP metrics sidecar proxy.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vitorbari/mcp-operator/sidecar/pkg/config"
	"github.com/vitorbari/mcp-operator/sidecar/pkg/metrics"
	"github.com/vitorbari/mcp-operator/sidecar/pkg/proxy"
)

// Version is set at build time via -ldflags.
var Version = "dev"

func main() {
	// Parse configuration from flags
	cfg := config.ParseFlags()

	// Setup structured logging
	logger := cfg.SetupLogger()

	// Log startup configuration
	logger.Info("starting MCP metrics sidecar proxy",
		slog.String("version", Version),
		slog.String("listen_addr", cfg.ListenAddr),
		slog.String("target_addr", cfg.TargetAddr),
		slog.String("metrics_addr", cfg.MetricsAddr),
		slog.String("log_level", cfg.LogLevel),
	)

	// Create the metrics recorder with OpenTelemetry
	recorder, err := metrics.NewRecorder(Version, cfg.TargetAddr)
	if err != nil {
		logger.Error("failed to create metrics recorder", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Create the proxy with metrics recorder
	p, err := proxy.NewWithRecorder(cfg.ListenAddr, cfg.TargetAddr, logger, recorder)
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

	// Start the metrics server
	metricsServer := startMetricsServer(cfg.MetricsAddr, recorder, logger)

	// Start the proxy (blocking)
	logger.Info("proxy configured",
		slog.String("listen_addr", p.ListenAddr()),
		slog.String("target", p.TargetURL().String()),
		slog.String("metrics_addr", cfg.MetricsAddr),
	)

	if err := p.Start(ctx); err != nil && err != context.Canceled {
		logger.Error("proxy error", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Shutdown metrics server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("metrics server shutdown error", slog.String("error", err.Error()))
	}

	// Shutdown OpenTelemetry meter provider
	if err := recorder.Shutdown(shutdownCtx); err != nil {
		logger.Error("metrics recorder shutdown error", slog.String("error", err.Error()))
	}

	logger.Info("proxy shutdown complete")
}

// startMetricsServer starts the Prometheus metrics HTTP server.
func startMetricsServer(addr string, recorder *metrics.Recorder, logger *slog.Logger) *http.Server {
	mux := http.NewServeMux()

	// Use the recorder's handler which serves Prometheus format metrics
	mux.Handle("/metrics", recorder.Handler())

	// Add a simple health check endpoint on the metrics server
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("starting metrics server", slog.String("addr", addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("metrics server error", slog.String("error", err.Error()))
		}
	}()

	return server
}
