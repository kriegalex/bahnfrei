// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"sync"
	"testing"
	"time"
)

func TestSessionCreateAndLookup(t *testing.T) {
	m := NewSessionManager(time.Hour)
	s, err := m.Create("acct-1", "alice", RoleMeetOrganizer)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if s.Token == "" {
		t.Fatal("session token must not be empty")
	}
	got, err := m.Lookup(s.Token)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got.AccountID != "acct-1" || got.Username != "alice" || got.Role != RoleMeetOrganizer {
		t.Errorf("Lookup = %+v, want AccountID=acct-1 Username=alice Role=meet_organizer", got)
	}
}

func TestSessionTokensAreUnique(t *testing.T) {
	m := NewSessionManager(time.Hour)
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		s, err := m.Create("acct", "u", RolePublic)
		if err != nil {
			t.Fatal(err)
		}
		if seen[s.Token] {
			t.Fatalf("duplicate session token generated: %q", s.Token)
		}
		seen[s.Token] = true
	}
}

func TestSessionLookupUnknownToken(t *testing.T) {
	m := NewSessionManager(time.Hour)
	if _, err := m.Lookup("does-not-exist"); err != ErrSessionNotFound {
		t.Errorf("Lookup(unknown) = %v, want ErrSessionNotFound", err)
	}
}

func TestSessionExpiry(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewSessionManager(time.Minute).WithClock(func() time.Time { return now })

	s, err := m.Create("acct", "u", RolePublic)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Lookup(s.Token); err != nil {
		t.Fatalf("Lookup before expiry: %v", err)
	}

	now = now.Add(2 * time.Minute)
	if _, err := m.Lookup(s.Token); err != ErrSessionNotFound {
		t.Errorf("Lookup after expiry = %v, want ErrSessionNotFound", err)
	}
	// Expired lookup evicts the entry.
	if got := m.Count(); got != 0 {
		t.Errorf("Count after expired lookup = %d, want 0 (evicted)", got)
	}
}

func TestSessionRevoke(t *testing.T) {
	m := NewSessionManager(time.Hour)
	s, err := m.Create("acct", "u", RolePublic)
	if err != nil {
		t.Fatal(err)
	}
	m.Revoke(s.Token)
	if _, err := m.Lookup(s.Token); err != ErrSessionNotFound {
		t.Errorf("Lookup after Revoke = %v, want ErrSessionNotFound", err)
	}
	// Revoking an unknown token is a no-op, not an error.
	m.Revoke("never-existed")
}

func TestSessionSweep(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewSessionManager(time.Minute).WithClock(func() time.Time { return now })

	if _, err := m.Create("a", "u1", RolePublic); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := m.Create("b", "u2", RolePublic); err != nil {
		t.Fatal(err)
	}
	if got := m.Count(); got != 2 {
		t.Fatalf("Count before sweep = %d, want 2", got)
	}
	n := m.Sweep()
	if n != 1 {
		t.Errorf("Sweep() = %d, want 1 (only the first session has expired)", n)
	}
	if got := m.Count(); got != 1 {
		t.Errorf("Count after sweep = %d, want 1", got)
	}
}

func TestSessionManagerConcurrentAccess(t *testing.T) {
	m := NewSessionManager(time.Hour)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := m.Create("acct", "u", RolePublic)
			if err != nil {
				t.Error(err)
				return
			}
			if _, err := m.Lookup(s.Token); err != nil {
				t.Error(err)
			}
			m.Revoke(s.Token)
		}()
	}
	wg.Wait()
}
