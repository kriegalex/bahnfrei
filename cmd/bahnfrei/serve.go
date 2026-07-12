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
	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
	"github.com/kriegalex/bahnfrei/internal/web"
	"github.com/kriegalex/bahnfrei/internal/web/i18n"
)

// serveConfig holds the parsed "serve" subcommand flags: role selection
// (architecture.md §2: "role selected at startup (venue default, hub)")
// plus the shell's network/TLS/session settings (SYS-091, SYS-093).
type serveConfig struct {
	role          string
	addr          string
	dataDir       string
	tlsMode       string
	acmeDomain    string
	acmeEmail     string
	sessionTTL    time.Duration
	retentionDays int
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
	tlsMode := fs.String("tls-mode", "", "TLS certificate mode: local|acme|off (default: local for venue, acme for hub; off = plaintext HTTP, dev/E2E only, never on a non-local network per SYS-093)")
	acmeDomain := fs.String("acme-domain", "", "comma-separated domain(s) to obtain an ACME certificate for (hub/acme mode)")
	acmeEmail := fs.String("acme-email", "", "ACME account contact email (hub/acme mode)")
	sessionTTL := fs.Duration("session-ttl", web.SessionTTLDefault, "how long a login session stays valid (SYS-091)")
	retentionDays := fs.Int("retention-days", app.DefaultRetentionDays, "SYS-102 retention: days post-meet before personal data (full birth dates, consent-recorder identity, audit PII) becomes purgeable")

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
	if mode != string(web.TLSModeLocal) && mode != string(web.TLSModeACME) && mode != string(web.TLSModeOff) {
		return serveConfig{}, fmt.Errorf("invalid --tls-mode %q: must be %q, %q or %q", mode, web.TLSModeLocal, web.TLSModeACME, web.TLSModeOff)
	}
	if mode == string(web.TLSModeACME) && *acmeDomain == "" {
		return serveConfig{}, fmt.Errorf("--tls-mode=acme requires --acme-domain")
	}
	if *retentionDays <= 0 {
		return serveConfig{}, fmt.Errorf("--retention-days must be positive, got %d", *retentionDays)
	}

	return serveConfig{
		role:          *role,
		addr:          *addr,
		dataDir:       *dataDir,
		tlsMode:       mode,
		acmeDomain:    *acmeDomain,
		acmeEmail:     *acmeEmail,
		sessionTTL:    *sessionTTL,
		retentionDays: *retentionDays,
	}, nil
}

// serveDeps bundles everything runServe needs to shut down cleanly
// alongside the web.Server it returns. privacy is exposed separately so
// runServe can trigger the SYS-102 startup retention sweep without
// web.Server needing to expose its internal app-layer wiring.
type serveDeps struct {
	server  *web.Server
	dbase   *store.Store
	privacy *app.PrivacyService
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

	catalog, err := domain.BuiltinDisciplineCatalog()
	if err != nil {
		_ = st.Close()
		return serveDeps{}, fmt.Errorf("load discipline catalog: %w", err)
	}
	schemes, err := domain.BuiltinCategorySchemes()
	if err != nil {
		_ = st.Close()
		return serveDeps{}, fmt.Errorf("load category schemes: %w", err)
	}
	tables, err := domain.BuiltinScoringTables()
	if err != nil {
		_ = st.Close()
		return serveDeps{}, fmt.Errorf("load scoring tables: %w", err)
	}
	templates, err := domain.BuiltinMeetTemplates()
	if err != nil {
		_ = st.Close()
		return serveDeps{}, fmt.Errorf("load meet templates: %w", err)
	}
	seriesUploads, err := domain.BuiltinSeriesUploadTemplates()
	if err != nil {
		_ = st.Close()
		return serveDeps{}, fmt.Errorf("load series upload templates: %w", err)
	}
	importProfiles, err := domain.BuiltinImportMappingProfiles()
	if err != nil {
		_ = st.Close()
		return serveDeps{}, fmt.Errorf("load import mapping profiles: %w", err)
	}
	meets := app.NewMeetService(st.DB(), catalog, schemes, tables, templates)
	results := app.NewResultsService(st.DB(), catalog, schemes, tables, templates)
	results.SetSeriesUploadTemplates(seriesUploads)
	results.SetImportMappingProfiles(importProfiles)
	backup := app.NewBackupService(st)
	privacy := app.NewPrivacyService(st.DB())

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
		AppVersion: version,
	}

	srv := web.New(webCfg, auth, sessions, meets, results, backup, cats, bus).SetPrivacy(privacy)
	return serveDeps{server: srv, dbase: st, privacy: privacy}, nil
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

	// SYS-102: retention purge SHALL be "automatically purgeable" — a
	// manual-only trigger (the /admin/privacy route, UC-024 #3) would not
	// satisfy that, so every process start also runs one sweep,
	// synchronously and before the listener opens: this system's storage
	// is a single-writer local SQLite database at PoC/club-meet scale
	// (ADR-004 §2), so one sweep is a bounded, fast, one-shot cost, and
	// running it synchronously — rather than in a background goroutine —
	// avoids a startup-vs-shutdown race with no correctness benefit here.
	// A purge failure is logged, never fatal: retention hygiene must never
	// block a venue from serving a meet.
	report, err := deps.privacy.PurgeExpiredAtStartup(ctx, cfg.retentionDays)
	switch {
	case err != nil:
		fmt.Fprintf(out, "bahnfrei: startup retention purge (SYS-102) failed: %v\n", err)
	case report.AthletesPurged > 0 || report.AuditRowsRedacted > 0:
		fmt.Fprintf(out, "bahnfrei: startup retention purge (SYS-102): %d athlete(s), %d audit row(s) redacted\n",
			report.AthletesPurged, report.AuditRowsRedacted)
	}

	fmt.Fprintf(out, "bahnfrei: listening on %s (role=%s, tls=%s)\n", cfg.addr, cfg.role, cfg.tlsMode)
	return deps.server.ListenAndServe(ctx)
}
