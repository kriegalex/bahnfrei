// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

// Package web is the HTTP shell (TASK-005, ADR-003): server lifecycle,
// SSR templates (templ) progressively enhanced with HTMX, the SSE live-
// update bus, session/RBAC middleware, i18n rendering, and TLS. Per
// architecture.md §3, web never touches internal/store directly — it
// calls through internal/app.
package web

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/web/i18n"
)

const (
	readHeaderTimeout = 10 * time.Second
	shutdownTimeout   = 15 * time.Second
)

// Server wires the HTTP handler tree to its dependencies and owns the
// listener lifecycle.
type Server struct {
	cfg     Config
	auth    *app.AuthService
	sess    *app.SessionManager
	meets   *app.MeetService
	results *app.ResultsService
	backup  *app.BackupService
	privacy *app.PrivacyService
	cats    i18n.Catalogs
	bus     *Bus
	httpSrv *http.Server
	// loginLimiter throttles brute-force login attempts (SYS-092, ASVS L2
	// V2.2.1). See internal/web/ratelimit.go.
	loginLimiter *loginRateLimiter
	// publicResults is the per-meet public-results render cache (ADR-004
	// read-path amendment, TASK-035/OQ-066). See publiccache.go.
	publicResults *publicResultsCache
}

// SetPrivacy wires the TASK-023 data-subject-rights/retention service
// (SYS-101/SYS-102, UC-024). Additive, post-construction (mirrors
// ResultsService.SetSeriesUploadTemplates) rather than a New() parameter,
// so it never breaks New's existing call sites/tests. Privacy routes 404
// if never wired (see handlePrivacyList et al. in privacy.go), matching
// how an unconfigured series-upload template renders "not available".
func (s *Server) SetPrivacy(p *app.PrivacyService) *Server {
	s.privacy = p
	return s
}

// New builds a Server. cats is normally the result of i18n.Load(); bus is
// shared with whoever publishes live updates (the meet handlers publish
// timetable events on it, UC-001 #4/SYS-071). backup wires the one-action
// instance backup (SYS-084, UC-020 #3).
func New(cfg Config, auth *app.AuthService, sess *app.SessionManager, meets *app.MeetService, results *app.ResultsService, backup *app.BackupService, cats i18n.Catalogs, bus *Bus) *Server {
	s := &Server{cfg: cfg, auth: auth, sess: sess, meets: meets, results: results, backup: backup, cats: cats, bus: bus,
		loginLimiter:  newLoginRateLimiter(loginFailLimit, loginFailWindow),
		publicResults: newPublicResultsCache()}
	// Every committed capture write (and consent change, SYS-103) fans out
	// to the meet's SSE topic — the capture and public live pages refresh
	// from it (UC-011 #4, SYS-071) — and invalidates that meet's public-
	// results render cache (ADR-004 read-path amendment, TASK-035/OQ-066),
	// so the very next request rebuilds instead of serving a stale render.
	results.OnResultsChanged(func(meetID string) {
		s.publicResults.invalidate(meetID)
		bus.Publish("meet-"+meetID, Event{Name: "results", Data: `{"meet":"` + meetID + `"}`})
	})
	s.httpSrv = &http.Server{
		Addr:              cfg.Addr,
		Handler:           s.routes(),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	return s
}

// Serve accepts plain TCP connections on ln, wraps them in TLS per
// cfg.TLS (SYS-093 — every surface this shell serves is TLS, whether via
// ACME for the hub role or the self-signed venue-local certificate), and
// blocks until ctx is canceled or the listener errors. On ctx cancellation
// it performs a graceful shutdown.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	tlsCfg, err := NewTLSConfig(ctx, s.cfg.TLS)
	if err != nil {
		return fmt.Errorf("build TLS config: %w", err)
	}
	// TLSModeOff (dev/E2E only) yields a nil config: serve plaintext HTTP on
	// the bare listener. Every production mode wraps the listener in TLS.
	srvLn := ln
	if tlsCfg != nil {
		s.httpSrv.TLSConfig = tlsCfg
		srvLn = tls.NewListener(ln, tlsCfg)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- s.httpSrv.Serve(srvLn) }()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := s.httpSrv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		return nil
	}
}

// ListenAndServe is the convenience entry point for cmd/bahnfrei: it opens
// cfg.Addr and calls Serve.
func (s *Server) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.cfg.Addr, err)
	}
	return s.Serve(ctx, ln)
}
