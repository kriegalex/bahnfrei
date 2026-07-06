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
	tb.Helper()
	st, err := store.Open(context.Background(), filepath.Join(tb.TempDir(), "test.db"))
	if err != nil {
		tb.Fatalf("apptest: store.Open: %v", err)
	}
	tb.Cleanup(func() { _ = st.Close() })

	sessions := app.NewSessionManager(sessionTTL)
	auth := app.NewAuthService(st.DB(), sessions, FastPasswordParams)
	return auth, sessions
}
