// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGenerateSelfSignedCovers(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	certPEM, keyPEM, err := generateSelfSigned([]string{"example.local", "192.168.1.10"}, now)
	if err != nil {
		t.Fatalf("generateSelfSigned: %v", err)
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if !contains(leaf.DNSNames, "example.local") {
		t.Errorf("DNSNames = %v, want to include example.local", leaf.DNSNames)
	}
	foundIP := false
	for _, ip := range leaf.IPAddresses {
		if ip.String() == "192.168.1.10" {
			foundIP = true
		}
	}
	if !foundIP {
		t.Errorf("IPAddresses = %v, want to include 192.168.1.10", leaf.IPAddresses)
	}
	if leaf.NotAfter.Sub(now) < selfSignedValidity-time.Hour {
		t.Errorf("NotAfter = %v, want at least ~%v after now", leaf.NotAfter, selfSignedValidity)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestLoadOrGenerateSelfSignedPersistsAndReuses(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	cert1, err := loadOrGenerateSelfSigned(certPath, keyPath, []string{"localhost"})
	if err != nil {
		t.Fatalf("first loadOrGenerateSelfSigned: %v", err)
	}
	if _, err := os.Stat(certPath); err != nil {
		t.Errorf("cert file not written: %v", err)
	}
	if _, err := os.Stat(keyPath); err != nil {
		t.Errorf("key file not written: %v", err)
	}

	cert2, err := loadOrGenerateSelfSigned(certPath, keyPath, []string{"localhost"})
	if err != nil {
		t.Fatalf("second loadOrGenerateSelfSigned: %v", err)
	}
	if string(cert1.Certificate[0]) != string(cert2.Certificate[0]) {
		t.Error("second call should reuse the cached certificate, not regenerate")
	}
}

func TestLoadOrGenerateSelfSignedRegeneratesWhenNearExpiry(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	// Write a cert that expires in 1 day — inside renewBefore's 30-day
	// window, so it must not be reused.
	almostExpiredPEM, keyPEM, err := generateSelfSigned([]string{"localhost"}, time.Now().Add(-selfSignedValidity+24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certPath, almostExpiredPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}

	fresh, err := loadOrGenerateSelfSigned(certPath, keyPath, []string{"localhost"})
	if err != nil {
		t.Fatalf("loadOrGenerateSelfSigned: %v", err)
	}
	leaf, err := x509.ParseCertificate(fresh.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if time.Until(leaf.NotAfter) < renewBefore {
		t.Errorf("expected a freshly regenerated certificate, got one expiring at %v", leaf.NotAfter)
	}
}

func TestLocalTLSConfigDefaultsToLocalhost(t *testing.T) {
	cfg := TLSConfig{Mode: TLSModeLocal, CacheDir: t.TempDir()}
	tlsCfg, err := NewTLSConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewTLSConfig: %v", err)
	}
	if len(tlsCfg.Certificates) != 1 {
		t.Fatalf("Certificates = %d, want 1", len(tlsCfg.Certificates))
	}
	leaf, err := x509.ParseCertificate(tlsCfg.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if !contains(leaf.DNSNames, "localhost") {
		t.Errorf("default local TLS cert DNSNames = %v, want to include localhost", leaf.DNSNames)
	}
}

func TestNewTLSConfigRejectsUnknownMode(t *testing.T) {
	if _, err := NewTLSConfig(context.Background(), TLSConfig{Mode: "bogus"}); err == nil {
		t.Error("NewTLSConfig with an unknown mode should error")
	}
}

func TestBuildACMEConfigWiresIssuer(t *testing.T) {
	// buildACMEConfig must never touch the network — it only assembles the
	// certmagic.Config/Issuer graph (ACME issuance itself, via ManageSync,
	// is outside this repo's offline test suite; see acmeTLSConfig's doc
	// comment).
	cfg := TLSConfig{
		Mode:     TLSModeACME,
		Domains:  []string{"meet.example.org"},
		CacheDir: t.TempDir(),
		Email:    "ops@example.org",
		CA:       "https://acme.invalid/directory",
	}
	magic := buildACMEConfig(cfg)
	if magic == nil {
		t.Fatal("buildACMEConfig returned nil")
	}
	if len(magic.Issuers) != 1 {
		t.Fatalf("Issuers = %d, want 1", len(magic.Issuers))
	}
}

func TestAcmeTLSConfigRequiresDomain(t *testing.T) {
	_, err := acmeTLSConfig(context.Background(), TLSConfig{Mode: TLSModeACME, CacheDir: t.TempDir()})
	if err == nil {
		t.Error("acmeTLSConfig with no domains should error before attempting any network call")
	}
}

// TestServerServeAndShutdown exercises the full listen/TLS/shutdown
// lifecycle end to end using the local (offline) TLS mode, on an
// ephemeral loopback port.
func TestServerServeAndShutdown(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal, CacheDir: t.TempDir()})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- deps.server.Serve(ctx, ln) }()

	client := &tls.Config{InsecureSkipVerify: true} //nolint:gosec // test client trusts the self-signed cert deliberately
	conn, err := tls.Dial("tcp", ln.Addr().String(), client)
	if err != nil {
		t.Fatalf("TLS dial to running server: %v", err)
	}
	_ = conn.Close()

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Serve returned %v after shutdown, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not shut down within 5s of context cancellation")
	}
}

// TestServerListenAndServe exercises the ListenAndServe convenience path
// (its own net.Listen call, not an externally supplied listener) end to
// end.
func TestServerListenAndServe(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal, CacheDir: t.TempDir()})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- deps.server.ListenAndServe(ctx) }()
	// Give the listener a moment to bind before canceling.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("ListenAndServe returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ListenAndServe did not shut down within 5s")
	}
}

func TestServerListenAndServeInvalidAddr(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal, CacheDir: t.TempDir()})
	badSrv := deps.withConfig(Config{Addr: "this-is-not-a-valid-address"})
	if err := badSrv.ListenAndServe(context.Background()); err == nil {
		t.Error("ListenAndServe with an invalid address should return an error")
	}
}
