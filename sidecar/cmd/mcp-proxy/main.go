// Package main is the entry point for the MCP metrics sidecar proxy.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vitorbari/mcp-operator/sidecar/pkg/config"
	"github.com/vitorbari/mcp-operator/sidecar/pkg/health"
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
		slog.Duration("health_check_interval", cfg.HealthCheckInterval),
		slog.Bool("tls_enabled", cfg.TLSEnabled),
	)

	// Validate and load TLS configuration if enabled
	var tlsConfig *tls.Config
	if cfg.TLSEnabled {
		// Validate TLS files exist
		if err := proxy.ValidateTLSFiles(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil {
			logger.Error("TLS configuration error", slog.String("error", err.Error()))
			os.Exit(1)
		}

		// Check certificate expiry
		expiry, err := proxy.ValidateCertExpiry(cfg.TLSCertFile)
		if err != nil {
			logger.Error("failed to check certificate expiry", slog.String("error", err.Error()))
			os.Exit(1)
		}

		if proxy.IsCertExpiringSoon(expiry) {
			logger.Warn("TLS certificate is expiring soon",
				slog.Time("expiry", expiry),
				slog.Int("days_until_expiry", proxy.DaysUntilExpiry(expiry)),
			)
		} else {
			logger.Info("TLS certificate validated",
				slog.Time("expiry", expiry),
				slog.Int("days_until_expiry", proxy.DaysUntilExpiry(expiry)),
			)
		}

		// Load TLS configuration
		tlsConfig, err = proxy.LoadTLSConfig(cfg.TLSCertFile, cfg.TLSKeyFile, cfg.TLSMinVersion)
		if err != nil {
			logger.Error("failed to load TLS configuration", slog.String("error", err.Error()))
			os.Exit(1)
		}

		logger.Info("TLS configuration loaded",
			slog.String("cert_file", cfg.TLSCertFile),
			slog.String("key_file", cfg.TLSKeyFile),
			slog.String("min_version", cfg.TLSMinVersion),
		)
	}

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

	// Create the health checker for target connectivity
	healthChecker := health.NewHealthChecker(cfg.TargetAddr, cfg.HealthCheckInterval)

	// Setup context with signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the health checker
	healthChecker.Start(ctx)

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logger.Info("received shutdown signal", slog.String("signal", sig.String()))
		cancel()
	}()

	// Start the metrics server with health endpoints
	metricsServer := startMetricsServer(cfg.MetricsAddr, recorder, healthChecker, logger)

	// Start the proxy (blocking)
	logger.Info("proxy configured",
		slog.String("listen_addr", p.ListenAddr()),
		slog.String("target", p.TargetURL().String()),
		slog.String("metrics_addr", cfg.MetricsAddr),
		slog.Bool("tls_enabled", cfg.TLSEnabled),
	)

	var proxyErr error
	if cfg.TLSEnabled {
		proxyErr = p.StartWithTLS(ctx, tlsConfig, cfg.TLSCertFile, cfg.TLSKeyFile)
	} else {
		proxyErr = p.Start(ctx)
	}

	if proxyErr != nil && proxyErr != context.Canceled {
		logger.Error("proxy error", slog.String("error", proxyErr.Error()))
		os.Exit(1)
	}

	// Shutdown health checker
	healthChecker.Stop()

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

// startMetricsServer starts the Prometheus metrics HTTP server with health endpoints.
func startMetricsServer(addr string, recorder *metrics.Recorder, healthChecker *health.HealthChecker, logger *slog.Logger) *http.Server {
	mux := http.NewServeMux()

	// Use the recorder's handler which serves Prometheus format metrics
	mux.Handle("/metrics", recorder.Handler())

	// Health check endpoints using the health checker
	mux.HandleFunc("/healthz", healthChecker.LivenessHandler())
	mux.HandleFunc("/readyz", healthChecker.ReadinessHandler())

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
