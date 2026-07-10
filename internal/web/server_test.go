// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/apptest"
	"github.com/kriegalex/bahnfrei/internal/web/i18n"
)

// testServerDeps bundles a fully wired Server with the underlying pieces
// tests need direct access to (e.g. to bootstrap an account, or to build a
// second Server against the same app-layer dependencies but a different
// Config).
type testServerDeps struct {
	server   *Server
	auth     *app.AuthService
	sessions *app.SessionManager
	meets    *app.MeetService
	results  *app.ResultsService
	backup   *app.BackupService
	cats     i18n.Catalogs
	bus      *Bus
}

// newTestServer builds a Server against a real (temporary) app.AuthService
// fixture from internal/apptest — web tests never import internal/store
// directly (architecture.md §3, enforced by depguard even for _test.go
// files).
func newTestServer(t *testing.T, tlsCfg TLSConfig) *testServerDeps {
	t.Helper()
	fix := apptest.New(t, time.Hour)
	cats, err := i18n.Load()
	if err != nil {
		t.Fatalf("i18n.Load: %v", err)
	}
	bus := NewBus()

	cfg := Config{Addr: "127.0.0.1:0", TLS: tlsCfg, AppVersion: "test"}
	deps := &testServerDeps{auth: fix.Auth, sessions: fix.Sessions, meets: fix.Meets, results: fix.Results, backup: fix.Backup, cats: cats, bus: bus}
	deps.server = New(cfg, fix.Auth, fix.Sessions, fix.Meets, fix.Results, fix.Backup, cats, bus)
	return deps
}

// withConfig rebuilds the Server against the same app-layer dependencies
// (store, sessions, catalogs, bus) but a different Config — used to test
// listener-address handling without standing up a whole new store.
func (d *testServerDeps) withConfig(cfg Config) *Server {
	return New(cfg, d.auth, d.sessions, d.meets, d.results, d.backup, d.cats, d.bus)
}
