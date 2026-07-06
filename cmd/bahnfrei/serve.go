// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/store"
	"github.com/kriegalex/bahnfrei/internal/web"
	"github.com/kriegalex/bahnfrei/internal/web/i18n"
)

// serveConfig holds the parsed "serve" subcommand flags: role selection
// (architecture.md §2: "role selected at startup (venue default, hub)")
// plus the shell's network/TLS/session settings (SYS-091, SYS-093).
type serveConfig struct {
	role       string
	addr       string
	dataDir    string
	tlsMode    string
	acmeDomain string
	acmeEmail  string
	sessionTTL time.Duration
}

const (
	roleVenue = "venue"
	roleHub   = "hub"
)

// parseServeFlags parses args into a serveConfig, applying role-dependent
// defaults (venue: local self-signed TLS, no internet-dependent CA per
// SYS-093; hub: ACME) unless overridden explicitly.
func parseServeFlags(args []string, out io.Writer) (serveConfig, error) {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(out)

	role := fs.String("role", roleVenue, "instance role: venue|hub")
	addr := fs.String("addr", ":8443", "listen address")
	dataDir := fs.String("data-dir", ".", "directory for the database and TLS cache")
	tlsMode := fs.String("tls-mode", "", "TLS certificate mode: local|acme (default: local for venue, acme for hub)")
	acmeDomain := fs.String("acme-domain", "", "comma-separated domain(s) to obtain an ACME certificate for (hub/acme mode)")
	acmeEmail := fs.String("acme-email", "", "ACME account contact email (hub/acme mode)")
	sessionTTL := fs.Duration("session-ttl", web.SessionTTLDefault, "how long a login session stays valid (SYS-091)")

	if err := fs.Parse(args); err != nil {
		return serveConfig{}, err
	}

	if *role != roleVenue && *role != roleHub {
		return serveConfig{}, fmt.Errorf("invalid --role %q: must be %q or %q", *role, roleVenue, roleHub)
	}
	mode := *tlsMode
	if mode == "" {
		if *role == roleHub {
			mode = string(web.TLSModeACME)
		} else {
			mode = string(web.TLSModeLocal)
		}
	}
	if mode != string(web.TLSModeLocal) && mode != string(web.TLSModeACME) {
		return serveConfig{}, fmt.Errorf("invalid --tls-mode %q: must be %q or %q", mode, web.TLSModeLocal, web.TLSModeACME)
	}
	if mode == string(web.TLSModeACME) && *acmeDomain == "" {
		return serveConfig{}, fmt.Errorf("--tls-mode=acme requires --acme-domain")
	}

	return serveConfig{
		role:       *role,
		addr:       *addr,
		dataDir:    *dataDir,
		tlsMode:    mode,
		acmeDomain: *acmeDomain,
		acmeEmail:  *acmeEmail,
		sessionTTL: *sessionTTL,
	}, nil
}

// serveDeps bundles everything runServe needs to shut down cleanly
// alongside the web.Server it returns.
type serveDeps struct {
	server *web.Server
	dbase  *store.Store
}

// buildServer wires storage, the app-layer services (session manager,
// auth/RBAC), the i18n catalogs and the SSE bus into a web.Server, per
// architecture.md §3 (web goes through app, never store directly).
func buildServer(cfg serveConfig) (serveDeps, error) {
	if err := os.MkdirAll(cfg.dataDir, 0o755); err != nil {
		return serveDeps{}, fmt.Errorf("create data dir %s: %w", cfg.dataDir, err)
	}

	st, err := store.Open(context.Background(), filepath.Join(cfg.dataDir, "bahnfrei.db"))
	if err != nil {
		return serveDeps{}, fmt.Errorf("open store: %w", err)
	}

	sessions := app.NewSessionManager(cfg.sessionTTL)
	auth := app.NewAuthService(st.DB(), sessions, app.DefaultPasswordParams)

	cats, err := i18n.Load()
	if err != nil {
		_ = st.Close()
		return serveDeps{}, fmt.Errorf("load i18n catalogs: %w", err)
	}

	bus := web.NewBus()

	var domains []string
	if cfg.acmeDomain != "" {
		domains = strings.Split(cfg.acmeDomain, ",")
	}
	webCfg := web.Config{
		Addr: cfg.addr,
		TLS: web.TLSConfig{
			Mode:     web.TLSMode(cfg.tlsMode),
			Domains:  domains,
			CacheDir: filepath.Join(cfg.dataDir, "tls"),
			Email:    cfg.acmeEmail,
		},
	}

	srv := web.New(webCfg, auth, sessions, cats, bus)
	return serveDeps{server: srv, dbase: st}, nil
}

// runServe parses flags, wires the server, and blocks serving until ctx is
// canceled (by SIGINT/SIGTERM in production, or directly by a test).
func runServe(ctx context.Context, args []string, out io.Writer) error {
	cfg, err := parseServeFlags(args, out)
	if err != nil {
		return err
	}
	deps, err := buildServer(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = deps.dbase.Close() }()

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Fprintf(out, "bahnfrei: listening on %s (role=%s, tls=%s)\n", cfg.addr, cfg.role, cfg.tlsMode)
	return deps.server.ListenAndServe(ctx)
}
