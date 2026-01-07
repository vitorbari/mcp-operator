// Package config provides configuration for the MCP metrics sidecar proxy.
package config

import (
	"flag"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Config holds the configuration for the MCP proxy sidecar.
type Config struct {
	// ListenAddr is the address the proxy listens on for incoming requests.
	ListenAddr string

	// TargetAddr is the address of the MCP server to proxy requests to.
	TargetAddr string

	// MetricsAddr is the address to expose Prometheus metrics on.
	MetricsAddr string

	// LogLevel controls the logging verbosity (debug, info, warn, error).
	LogLevel string

	// HealthCheckInterval is the interval between health checks of the target.
	HealthCheckInterval time.Duration
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() *Config {
	return &Config{
		ListenAddr:          ":8080",
		TargetAddr:          "localhost:3001",
		MetricsAddr:         ":9090",
		LogLevel:            "info",
		HealthCheckInterval: 10 * time.Second,
	}
}

// ParseFlags parses command-line flags and returns a Config.
func ParseFlags() *Config {
	cfg := DefaultConfig()

	flag.StringVar(&cfg.ListenAddr, "listen-addr", cfg.ListenAddr, "Address to listen on for incoming requests")
	flag.StringVar(&cfg.TargetAddr, "target-addr", cfg.TargetAddr, "Address of the MCP server to proxy to")
	flag.StringVar(&cfg.MetricsAddr, "metrics-addr", cfg.MetricsAddr, "Address to expose Prometheus metrics on")
	flag.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "Log level (debug, info, warn, error)")
	flag.DurationVar(&cfg.HealthCheckInterval, "health-check-interval", cfg.HealthCheckInterval, "Interval between health checks of the target")

	flag.Parse()

	return cfg
}

// LogLevel returns the slog.Level corresponding to the configured log level string.
func (c *Config) GetLogLevel() slog.Level {
	switch strings.ToLower(c.LogLevel) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// SetupLogger configures the default slog logger based on the config.
func (c *Config) SetupLogger() *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: c.GetLogLevel(),
	}
	handler := slog.NewJSONHandler(os.Stdout, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}
