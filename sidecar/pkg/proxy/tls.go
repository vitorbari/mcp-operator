// Package proxy provides TLS configuration for the MCP metrics sidecar.
package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"time"
)

// CertExpiryWarningThreshold is the duration before expiry to start warning.
const CertExpiryWarningThreshold = 30 * 24 * time.Hour // 30 days

// LoadTLSConfig loads and configures TLS settings from certificate files.
// It returns a tls.Config with secure defaults.
func LoadTLSConfig(certFile, keyFile, minVersion string) (*tls.Config, error) {
	// Load certificate and key
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS certificate: %w", err)
	}

	// Determine minimum TLS version
	var minVer uint16
	switch strings.TrimPrefix(strings.ToLower(minVersion), "tls") {
	case "1.3", "13":
		minVer = tls.VersionTLS13
	case "1.2", "12", "":
		minVer = tls.VersionTLS12
	default:
		return nil, fmt.Errorf("unsupported TLS version: %s (supported: 1.2, 1.3)", minVersion)
	}

	// Configure TLS with secure defaults
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   minVer,
		// Secure cipher suites (TLS 1.2)
		// TLS 1.3 cipher suites are not configurable - Go uses secure defaults
		CipherSuites: []uint16{
			// TLS 1.3 cipher suites (always enabled when TLS 1.3 is supported)
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			// TLS 1.2 cipher suites (ECDHE only, no RSA key exchange)
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		},
		// Prefer server cipher suites
		PreferServerCipherSuites: true,
	}

	return tlsConfig, nil
}

// ValidateCertExpiry parses the certificate file and returns the expiry date.
// Returns an error if the certificate cannot be parsed.
func ValidateCertExpiry(certFile string) (time.Time, error) {
	// Read the certificate file
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to read certificate file: %w", err)
	}

	// Decode the PEM block
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return time.Time{}, fmt.Errorf("failed to decode PEM block from certificate file")
	}

	// Parse the certificate
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return cert.NotAfter, nil
}

// IsCertExpiringSoon checks if the certificate is expiring within the warning threshold.
func IsCertExpiringSoon(expiry time.Time) bool {
	return time.Until(expiry) < CertExpiryWarningThreshold
}

// DaysUntilExpiry returns the number of days until the certificate expires.
func DaysUntilExpiry(expiry time.Time) int {
	return int(time.Until(expiry).Hours() / 24)
}

// ValidateTLSFiles checks that the TLS certificate and key files exist and are readable.
func ValidateTLSFiles(certFile, keyFile string) error {
	// Check certificate file
	if certFile == "" {
		return fmt.Errorf("TLS certificate file path is required when TLS is enabled")
	}
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		return fmt.Errorf("TLS certificate file not found: %s", certFile)
	} else if err != nil {
		return fmt.Errorf("cannot access TLS certificate file: %w", err)
	}

	// Check key file
	if keyFile == "" {
		return fmt.Errorf("TLS key file path is required when TLS is enabled")
	}
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		return fmt.Errorf("TLS key file not found: %s", keyFile)
	} else if err != nil {
		return fmt.Errorf("cannot access TLS key file: %w", err)
	}

	return nil
}
