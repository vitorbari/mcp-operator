package proxy

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// generateTestCert creates a self-signed certificate for testing.
// Returns paths to cert and key files, and a cleanup function.
func generateTestCert(t *testing.T, notAfter time.Time) (certFile, keyFile string, cleanup func()) {
	t.Helper()

	// Generate a private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	// Create a certificate template
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore:             time.Now(),
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
	}

	// Create the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	// Create temp directory for cert files
	tmpDir, err := os.MkdirTemp("", "tls-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	certFile = filepath.Join(tmpDir, "cert.pem")
	keyFile = filepath.Join(tmpDir, "key.pem")

	// Write certificate
	certOut, err := os.Create(certFile)
	if err != nil {
		t.Fatalf("Failed to create cert file: %v", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	certOut.Close()

	// Write private key
	keyOut, err := os.Create(keyFile)
	if err != nil {
		t.Fatalf("Failed to create key file: %v", err)
	}
	keyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyBytes}); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}
	keyOut.Close()

	cleanup = func() {
		os.RemoveAll(tmpDir)
	}

	return certFile, keyFile, cleanup
}

func TestLoadTLSConfig(t *testing.T) {
	// Generate valid cert for testing
	certFile, keyFile, cleanup := generateTestCert(t, time.Now().Add(365*24*time.Hour))
	defer cleanup()

	tests := []struct {
		name       string
		certFile   string
		keyFile    string
		minVersion string
		wantErr    bool
		wantMinVer uint16
	}{
		{
			name:       "valid cert with TLS 1.2",
			certFile:   certFile,
			keyFile:    keyFile,
			minVersion: "1.2",
			wantErr:    false,
			wantMinVer: tls.VersionTLS12,
		},
		{
			name:       "valid cert with TLS 1.3",
			certFile:   certFile,
			keyFile:    keyFile,
			minVersion: "1.3",
			wantErr:    false,
			wantMinVer: tls.VersionTLS13,
		},
		{
			name:       "valid cert with empty version defaults to 1.2",
			certFile:   certFile,
			keyFile:    keyFile,
			minVersion: "",
			wantErr:    false,
			wantMinVer: tls.VersionTLS12,
		},
		{
			name:       "invalid TLS version",
			certFile:   certFile,
			keyFile:    keyFile,
			minVersion: "1.1",
			wantErr:    true,
		},
		{
			name:       "non-existent cert file",
			certFile:   "/nonexistent/cert.pem",
			keyFile:    keyFile,
			minVersion: "1.2",
			wantErr:    true,
		},
		{
			name:       "non-existent key file",
			certFile:   certFile,
			keyFile:    "/nonexistent/key.pem",
			minVersion: "1.2",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadTLSConfig(tt.certFile, tt.keyFile, tt.minVersion)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadTLSConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && cfg.MinVersion != tt.wantMinVer {
				t.Errorf("LoadTLSConfig() MinVersion = %v, want %v", cfg.MinVersion, tt.wantMinVer)
			}
		})
	}
}

func TestValidateCertExpiry(t *testing.T) {
	// Generate cert expiring in 1 year
	certFile1Year, _, cleanup1 := generateTestCert(t, time.Now().Add(365*24*time.Hour))
	defer cleanup1()

	// Generate cert expiring in 10 days
	certFile10Days, _, cleanup2 := generateTestCert(t, time.Now().Add(10*24*time.Hour))
	defer cleanup2()

	tests := []struct {
		name          string
		certFile      string
		wantErr       bool
		expectingSoon bool
	}{
		{
			name:          "cert expiring in 1 year",
			certFile:      certFile1Year,
			wantErr:       false,
			expectingSoon: false,
		},
		{
			name:          "cert expiring in 10 days",
			certFile:      certFile10Days,
			wantErr:       false,
			expectingSoon: true,
		},
		{
			name:     "non-existent cert",
			certFile: "/nonexistent/cert.pem",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expiry, err := ValidateCertExpiry(tt.certFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCertExpiry() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				isSoon := IsCertExpiringSoon(expiry)
				if isSoon != tt.expectingSoon {
					t.Errorf("IsCertExpiringSoon() = %v, want %v", isSoon, tt.expectingSoon)
				}
			}
		})
	}
}

func TestValidateTLSFiles(t *testing.T) {
	// Generate valid cert
	certFile, keyFile, cleanup := generateTestCert(t, time.Now().Add(365*24*time.Hour))
	defer cleanup()

	tests := []struct {
		name     string
		certFile string
		keyFile  string
		wantErr  bool
	}{
		{
			name:     "valid cert and key",
			certFile: certFile,
			keyFile:  keyFile,
			wantErr:  false,
		},
		{
			name:     "empty cert path",
			certFile: "",
			keyFile:  keyFile,
			wantErr:  true,
		},
		{
			name:     "empty key path",
			certFile: certFile,
			keyFile:  "",
			wantErr:  true,
		},
		{
			name:     "non-existent cert",
			certFile: "/nonexistent/cert.pem",
			keyFile:  keyFile,
			wantErr:  true,
		},
		{
			name:     "non-existent key",
			certFile: certFile,
			keyFile:  "/nonexistent/key.pem",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTLSFiles(tt.certFile, tt.keyFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTLSFiles() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDaysUntilExpiry(t *testing.T) {
	tests := []struct {
		name     string
		expiry   time.Time
		wantDays int
	}{
		{
			name:     "30 days from now",
			expiry:   time.Now().Add(30 * 24 * time.Hour),
			wantDays: 30,
		},
		{
			name:     "1 day from now",
			expiry:   time.Now().Add(24 * time.Hour),
			wantDays: 1,
		},
		{
			name:     "already expired",
			expiry:   time.Now().Add(-24 * time.Hour),
			wantDays: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DaysUntilExpiry(tt.expiry)
			// Allow +/- 1 day tolerance due to time calculations
			if got < tt.wantDays-1 || got > tt.wantDays+1 {
				t.Errorf("DaysUntilExpiry() = %v, want %v (±1)", got, tt.wantDays)
			}
		})
	}
}

func TestLoadTLSConfig_CipherSuites(t *testing.T) {
	certFile, keyFile, cleanup := generateTestCert(t, time.Now().Add(365*24*time.Hour))
	defer cleanup()

	cfg, err := LoadTLSConfig(certFile, keyFile, "1.2")
	if err != nil {
		t.Fatalf("LoadTLSConfig() error = %v", err)
	}

	// Verify secure cipher suites are configured
	if len(cfg.CipherSuites) == 0 {
		t.Error("Expected cipher suites to be configured")
	}

	// Verify PreferServerCipherSuites is set
	if !cfg.PreferServerCipherSuites {
		t.Error("Expected PreferServerCipherSuites to be true")
	}
}

func TestIsCertExpiringSoon(t *testing.T) {
	tests := []struct {
		name     string
		expiry   time.Time
		expected bool
	}{
		{
			name:     "expires in 60 days",
			expiry:   time.Now().Add(60 * 24 * time.Hour),
			expected: false,
		},
		{
			name:     "expires in 29 days",
			expiry:   time.Now().Add(29 * 24 * time.Hour),
			expected: true,
		},
		{
			name:     "expires in 1 day",
			expiry:   time.Now().Add(24 * time.Hour),
			expected: true,
		},
		{
			name:     "already expired",
			expiry:   time.Now().Add(-24 * time.Hour),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCertExpiringSoon(tt.expiry)
			if got != tt.expected {
				t.Errorf("IsCertExpiringSoon() = %v, want %v", got, tt.expected)
			}
		})
	}
}
