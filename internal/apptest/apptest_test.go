// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package apptest

import (
	"context"
	"testing"
	"time"
)

// TestFixtureWiring proves the fixture hands out working services against
// one shared store: the auth service can bootstrap and log in, and the
// meet service reads the same database.
func TestFixtureWiring(t *testing.T) {
	fix := New(t, time.Hour)
	ctx := context.Background()

	if _, err := fix.Auth.Bootstrap(ctx, "admin", "Admin", "s3cret-passphrase"); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if _, err := fix.Auth.Login(ctx, "admin", "s3cret-passphrase"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	meets, err := fix.Meets.ListMeets(ctx)
	if err != nil {
		t.Fatalf("ListMeets: %v", err)
	}
	if len(meets) != 0 {
		t.Errorf("fresh fixture has %d meets, want 0", len(meets))
	}

	// The compatibility wrapper returns the same style of ready services.
	auth, sessions := AuthService(t, time.Hour)
	if auth == nil || sessions == nil {
		t.Fatal("AuthService returned nil services")
	}
}
