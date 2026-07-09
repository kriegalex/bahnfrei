// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/caddyserver/certmagic"
)

// TLSMode selects how the server obtains its TLS certificate (SYS-093:
// "All access over non-local networks SHALL be encrypted in transit ...
// the venue-local mode SHALL NOT require internet-dependent certificate
// infrastructure to function").
type TLSMode string

const (
	// TLSModeLocal issues a locally-generated, self-signed certificate,
	// cached on disk. No internet access or external CA is ever contacted
	// — this is what SYS-093's venue-local clause requires.
	TLSModeLocal TLSMode = "local"
	// TLSModeACME manages a publicly-trusted certificate via certmagic/ACME
	// (ADR-003), for the internet-reachable hub role (ADR-002).
	TLSModeACME TLSMode = "acme"
	// TLSModeOff serves plaintext HTTP with no TLS. It exists for local
	// development and the browser end-to-end suite only (loopback is a
	// secure context, so service workers register over http://localhost —
	// UC-034 #3): SYS-093 still requires TLS on any non-local network, so
	// this mode MUST NOT be used for real venue/hub deployments and is never
	// a default.
	TLSModeOff TLSMode = "off"
)

// TLSConfig configures certificate acquisition for the HTTP server.
type TLSConfig struct {
	Mode TLSMode
	// Domains are the SANs to cover: DNS names the hub is reachable at
	// (ACME mode), or hostnames/IPs the venue-local cert should present
	// (local mode; defaults to localhost + loopback if empty).
	Domains []string
	// CacheDir stores certificates/keys (both modes) and ACME account
	// state (ACME mode only) across restarts.
	CacheDir string
	// Email is the ACME account contact (required by most CAs); ACME mode
	// only.
	Email string
	// CA overrides the ACME directory URL; empty means certmagic's
	// built-in default (Let's Encrypt production). Tests use this to point
	// at a non-existent endpoint so no real network call can succeed.
	CA string
}

// NewTLSConfig builds a *tls.Config for cfg.Mode. ACME mode performs real
// network operations (ACME account registration, certificate issuance) via
// certmagic.Config.ManageSync and should only be called with a real,
// internet-reachable domain; local mode never touches the network.
func NewTLSConfig(ctx context.Context, cfg TLSConfig) (*tls.Config, error) {
	switch cfg.Mode {
	case TLSModeOff:
		return nil, nil // plaintext HTTP; Serve skips the TLS listener wrapper
	case TLSModeLocal:
		return localTLSConfig(cfg)
	case TLSModeACME:
		return acmeTLSConfig(ctx, cfg)
	default:
		return nil, fmt.Errorf("unknown TLS mode %q", cfg.Mode)
	}
}

// --- ACME (hub role) ---

// buildACMEConfig wires a certmagic.Config from cfg without touching the
// network — this is the unit-testable half of ACME mode. acmeTLSConfig
// additionally calls ManageSync, which does real ACME issuance and cannot
// run in this repo's offline test suite (constraint: TLS/ACME paths that
// can't run in tests are isolated behind this seam).
func buildACMEConfig(cfg TLSConfig) *certmagic.Config {
	cacheDir := cfg.CacheDir
	if cacheDir == "" {
		cacheDir = "."
	}
	storage := &certmagic.FileStorage{Path: filepath.Join(cacheDir, "acme")}

	var magic *certmagic.Config
	cache := certmagic.NewCache(certmagic.CacheOptions{
		GetConfigForCert: func(certmagic.Certificate) (*certmagic.Config, error) {
			return magic, nil
		},
	})
	magic = certmagic.New(cache, certmagic.Config{Storage: storage})
	issuer := certmagic.NewACMEIssuer(magic, certmagic.ACMEIssuer{
		CA:     cfg.CA,
		Email:  cfg.Email,
		Agreed: true,
	})
	magic.Issuers = []certmagic.Issuer{issuer}
	return magic
}

func acmeTLSConfig(ctx context.Context, cfg TLSConfig) (*tls.Config, error) {
	if len(cfg.Domains) == 0 {
		return nil, fmt.Errorf("acme TLS mode requires at least one domain")
	}
	magic := buildACMEConfig(cfg)
	if err := magic.ManageSync(ctx, cfg.Domains); err != nil {
		return nil, fmt.Errorf("acme manage %v: %w", cfg.Domains, err)
	}
	return magic.TLSConfig(), nil
}

// --- Local self-signed (venue-local / offline-friendly) ---

const selfSignedValidity = 365 * 24 * time.Hour

// renewBefore regenerates the cached local certificate once it has fewer
// than this much validity remaining, so a long-running venue instance
// never serves an expired cert without operator intervention.
const renewBefore = 30 * 24 * time.Hour

func localTLSConfig(cfg TLSConfig) (*tls.Config, error) {
	domains := cfg.Domains
	if len(domains) == 0 {
		domains = []string{"localhost"}
	}
	cacheDir := cfg.CacheDir
	if cacheDir == "" {
		cacheDir = "."
	}
	certPath := filepath.Join(cacheDir, "local-cert.pem")
	keyPath := filepath.Join(cacheDir, "local-key.pem")

	cert, err := loadOrGenerateSelfSigned(certPath, keyPath, domains)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// loadOrGenerateSelfSigned loads a cached cert/key pair if present and
// still valid for at least renewBefore longer; otherwise it generates a
// fresh self-signed certificate, persists it, and returns it.
func loadOrGenerateSelfSigned(certPath, keyPath string, domains []string) (tls.Certificate, error) {
	if cert, ok := loadValidCert(certPath, keyPath); ok {
		return cert, nil
	}

	certPEM, keyPEM, err := generateSelfSigned(domains, time.Now())
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate self-signed cert: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(certPath), 0o755); err != nil {
		return tls.Certificate{}, fmt.Errorf("create TLS cache dir: %w", err)
	}
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return tls.Certificate{}, fmt.Errorf("write local cert: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return tls.Certificate{}, fmt.Errorf("write local key: %w", err)
	}
	return tls.X509KeyPair(certPEM, keyPEM)
}

func loadValidCert(certPath, keyPath string) (tls.Certificate, bool) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, false
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return tls.Certificate{}, false
	}
	if time.Until(leaf.NotAfter) < renewBefore {
		return tls.Certificate{}, false
	}
	return cert, true
}

// generateSelfSigned produces a fresh ECDSA P-256 self-signed certificate
// (PEM-encoded cert and key) covering domains, valid from now for
// selfSignedValidity. Hostnames go to DNSNames; anything parseable as an
// IP goes to IPAddresses instead, so "localhost" and "192.168.1.10" both
// work as SANs.
func generateSelfSigned(domains []string, now time.Time) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}

	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "bahnfrei venue-local"},
		NotBefore:             now.Add(-5 * time.Minute), // clock-skew tolerance
		NotAfter:              now.Add(selfSignedValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true, // self-signed leaf acts as its own trust anchor
		BasicConstraintsValid: true,
	}
	for _, d := range domains {
		if ip := net.ParseIP(d); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, d)
		}
	}
	template.IPAddresses = append(template.IPAddresses, net.ParseIP("127.0.0.1"), net.ParseIP("::1"))

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}
