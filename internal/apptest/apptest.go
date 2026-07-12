// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

// Package apptest builds a temporary, fully wired internal/app fixture
// (a SQLite-backed *app.AuthService and its *app.SessionManager) for use
// by other packages' tests. It exists so that internal/web's tests can
// exercise real login/RBAC flows without importing internal/store
// directly — architecture.md §3's "web never touches store directly"
// rule (enforced by the golangci-lint depguard "web-goes-through-app"
// rule) applies to test files too, and this package is the seam that
// keeps that true while still letting web tests set up realistic
// fixtures.
package apptest

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// FastPasswordParams is a deliberately cheap argon2id cost so test suites
// that log in repeatedly stay fast. Production code must never use this —
// only app.DefaultPasswordParams ships.
var FastPasswordParams = app.PasswordParams{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32}

// AuthService opens a fresh SQLite store in a t.TempDir(), closing it via
// t.Cleanup, and returns a ready-to-use *app.AuthService and its
// *app.SessionManager.
func AuthService(tb testing.TB, sessionTTL time.Duration) (*app.AuthService, *app.SessionManager) {
	f := New(tb, sessionTTL)
	return f.Auth, f.Sessions
}

// Fixture bundles every wired app service backed by one temporary store.
type Fixture struct {
	Auth     *app.AuthService
	Sessions *app.SessionManager
	Meets    *app.MeetService
	Results  *app.ResultsService
	Backup   *app.BackupService
	// Privacy wires TASK-023's data-subject-rights/retention service
	// (SYS-101/SYS-102, UC-024).
	Privacy *app.PrivacyService
}

// New opens a fresh SQLite store in a t.TempDir(), closing it via
// t.Cleanup, and wires the full app service set against it (with the
// built-in discipline catalog and category schemes).
func New(tb testing.TB, sessionTTL time.Duration) Fixture {
	tb.Helper()
	st, err := store.Open(context.Background(), filepath.Join(tb.TempDir(), "test.db"))
	if err != nil {
		tb.Fatalf("apptest: store.Open: %v", err)
	}
	tb.Cleanup(func() { _ = st.Close() })

	catalog, err := domain.BuiltinDisciplineCatalog()
	if err != nil {
		tb.Fatalf("apptest: load discipline catalog: %v", err)
	}
	schemes, err := domain.BuiltinCategorySchemes()
	if err != nil {
		tb.Fatalf("apptest: load category schemes: %v", err)
	}
	tables, err := domain.BuiltinScoringTables()
	if err != nil {
		tb.Fatalf("apptest: load scoring tables: %v", err)
	}
	templates, err := domain.BuiltinMeetTemplates()
	if err != nil {
		tb.Fatalf("apptest: load meet templates: %v", err)
	}
	seriesUploads, err := domain.BuiltinSeriesUploadTemplates()
	if err != nil {
		tb.Fatalf("apptest: load series upload templates: %v", err)
	}
	importProfiles, err := domain.BuiltinImportMappingProfiles()
	if err != nil {
		tb.Fatalf("apptest: load import mapping profiles: %v", err)
	}

	sessions := app.NewSessionManager(sessionTTL)
	results := app.NewResultsService(st.DB(), catalog, schemes, tables, templates)
	results.SetSeriesUploadTemplates(seriesUploads)
	results.SetImportMappingProfiles(importProfiles)
	return Fixture{
		Auth:     app.NewAuthService(st.DB(), sessions, FastPasswordParams),
		Sessions: sessions,
		Meets:    app.NewMeetService(st.DB(), catalog, schemes, tables, templates),
		Results:  results,
		Backup:   app.NewBackupService(st),
		Privacy:  app.NewPrivacyService(st.DB()),
	}
}
