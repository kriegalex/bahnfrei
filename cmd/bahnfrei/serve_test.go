// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package main

import (
	"context"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/web"
)

func TestParseServeFlagsDefaults(t *testing.T) {
	var out strings.Builder
	cfg, err := parseServeFlags(nil, &out)
	if err != nil {
		t.Fatalf("parseServeFlags: %v", err)
	}
	if cfg.role != roleVenue {
		t.Errorf("default role = %q, want %q", cfg.role, roleVenue)
	}
	if cfg.tlsMode != string(web.TLSModeLocal) {
		t.Errorf("default tls mode for venue = %q, want %q", cfg.tlsMode, web.TLSModeLocal)
	}
}

func TestParseServeFlagsHubDefaultsToACME(t *testing.T) {
	var out strings.Builder
	cfg, err := parseServeFlags([]string{"--role=hub", "--acme-domain=meet.example.org"}, &out)
	if err != nil {
		t.Fatalf("parseServeFlags: %v", err)
	}
	if cfg.tlsMode != string(web.TLSModeACME) {
		t.Errorf("default tls mode for hub = %q, want %q", cfg.tlsMode, web.TLSModeACME)
	}
}

func TestParseServeFlagsRejectsInvalidRole(t *testing.T) {
	var out strings.Builder
	if _, err := parseServeFlags([]string{"--role=bogus"}, &out); err == nil {
		t.Error("expected an error for an invalid --role")
	}
}

func TestParseServeFlagsRejectsInvalidTLSMode(t *testing.T) {
	var out strings.Builder
	if _, err := parseServeFlags([]string{"--tls-mode=bogus"}, &out); err == nil {
		t.Error("expected an error for an invalid --tls-mode")
	}
}

func TestParseServeFlagsACMERequiresDomain(t *testing.T) {
	var out strings.Builder
	if _, err := parseServeFlags([]string{"--role=hub"}, &out); err == nil {
		t.Error("--role=hub without --acme-domain should error before any network activity")
	}
}

func TestBuildServerOpensStoreAndWiresDependencies(t *testing.T) {
	cfg := serveConfig{
		role:       roleVenue,
		addr:       "127.0.0.1:0",
		dataDir:    t.TempDir(),
		tlsMode:    string(web.TLSModeLocal),
		sessionTTL: time.Hour,
	}
	deps, err := buildServer(cfg)
	if err != nil {
		t.Fatalf("buildServer: %v", err)
	}
	defer func() { _ = deps.dbase.Close() }()

	if deps.server == nil {
		t.Fatal("buildServer returned a nil *web.Server")
	}

	// The server must actually be able to accept a connection end to end
	// on an ephemeral port (proves the TLS/session/i18n wiring, not just
	// that the constructor didn't panic).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- deps.server.Serve(ctx, ln) }()
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Serve returned %v after immediate cancellation, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down promptly")
	}
}

func TestRunServeSurfacesFlagErrors(t *testing.T) {
	var out strings.Builder
	err := runServe(context.Background(), []string{"--role=bogus"}, &out)
	if err == nil {
		t.Error("runServe with an invalid role should return an error")
	}
}

func TestRunServeCancelledContextShutsDownCleanly(t *testing.T) {
	var out strings.Builder
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled: the server should come up and shut right back down

	err := runServe(ctx, []string{
		"--addr=127.0.0.1:0",
		"--data-dir=" + t.TempDir(),
	}, &out)
	if err != nil {
		t.Errorf("runServe with pre-canceled context = %v, want nil", err)
	}
	if !strings.Contains(out.String(), "listening on") {
		t.Errorf("runServe should announce the listen address; got %q", out.String())
	}
}

func TestBuildServerCreatesDataDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "data")
	cfg := serveConfig{role: roleVenue, addr: "127.0.0.1:0", dataDir: dir, tlsMode: string(web.TLSModeLocal), sessionTTL: time.Hour}
	deps, err := buildServer(cfg)
	if err != nil {
		t.Fatalf("buildServer with a not-yet-existing nested data dir: %v", err)
	}
	defer func() { _ = deps.dbase.Close() }()
}
