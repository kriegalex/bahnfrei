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
	cats    i18n.Catalogs
	bus     *Bus
	httpSrv *http.Server
}

// New builds a Server. cats is normally the result of i18n.Load(); bus is
// shared with whoever publishes live updates (the meet handlers publish
// timetable events on it, UC-001 #4/SYS-071).
func New(cfg Config, auth *app.AuthService, sess *app.SessionManager, meets *app.MeetService, cats i18n.Catalogs, bus *Bus) *Server {
	s := &Server{cfg: cfg, auth: auth, sess: sess, meets: meets, cats: cats, bus: bus}
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
	s.httpSrv.TLSConfig = tlsCfg
	tlsLn := tls.NewListener(ln, tlsCfg)

	errCh := make(chan error, 1)
	go func() { errCh <- s.httpSrv.Serve(tlsLn) }()

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
