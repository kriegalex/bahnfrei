// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"path/filepath"
	"sort"
	"testing"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// SYS-081: the durability pragmas must actually be in effect.
func TestOpenDurabilityPragmas(t *testing.T) {
	s := openTest(t)
	var journalMode string
	if err := s.DB().QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want wal (ADR-004 §1)", journalMode)
	}
	var synchronous int
	if err := s.DB().QueryRow("PRAGMA synchronous").Scan(&synchronous); err != nil {
		t.Fatal(err)
	}
	if synchronous != 2 { // 2 == FULL
		t.Errorf("synchronous = %d, want 2/FULL (SYS-081)", synchronous)
	}
	var fk int
	if err := s.DB().QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")
	s1, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if s1.Path() != path {
		t.Errorf("Path() = %q, want %q", s1.Path(), path)
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(ctx, path) // reopen: migrations no-op, check green
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })

	report, err := s2.Check(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK() {
		t.Errorf("reopened store not green: %s", report.Summary())
	}
}

func TestNewIDSortableUnique(t *testing.T) {
	const n = 1000
	ids := make([]string, n)
	seen := make(map[string]bool, n)
	for i := range ids {
		ids[i] = NewID()
		if len(ids[i]) != 26 {
			t.Fatalf("ULID length = %d, want 26", len(ids[i]))
		}
		if seen[ids[i]] {
			t.Fatalf("duplicate ULID %s", ids[i])
		}
		seen[ids[i]] = true
	}
	if !sort.StringsAreSorted(ids) {
		t.Error("ULIDs generated in sequence are not lexicographically sorted")
	}
}
