// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"errors"
	"testing"
)

func widgetStore(t *testing.T) *Store {
	t.Helper()
	s := openTest(t)
	if _, err := s.DB().Exec(`CREATE TABLE widgets (
		id      TEXT PRIMARY KEY,
		name    TEXT NOT NULL,
		version INTEGER NOT NULL DEFAULT 1
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`INSERT INTO widgets (id, name) VALUES ('w1', 'discus')`); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestOptimisticUpdateHappyPath(t *testing.T) {
	ctx := context.Background()
	s := widgetStore(t)

	v, err := OptimisticUpdate(ctx, s.DB(), "widgets", "w1", 1, Set{"name", "javelin"})
	if err != nil {
		t.Fatalf("OptimisticUpdate: %v", err)
	}
	if v != 2 {
		t.Errorf("new version = %d, want 2", v)
	}
	var name string
	var version int64
	if err := s.DB().QueryRow(`SELECT name, version FROM widgets WHERE id = 'w1'`).
		Scan(&name, &version); err != nil {
		t.Fatal(err)
	}
	if name != "javelin" || version != 2 {
		t.Errorf("row = (%s, v%d), want (javelin, v2)", name, version)
	}
}

// SYS-083: a concurrent edit to the same entity is surfaced, never
// silently last-write-wins.
func TestOptimisticUpdateConflictSurfaced(t *testing.T) {
	ctx := context.Background()
	s := widgetStore(t)

	// Two operators both read version 1. The first update wins…
	if _, err := OptimisticUpdate(ctx, s.DB(), "widgets", "w1", 1, Set{"name", "shot put"}); err != nil {
		t.Fatal(err)
	}
	// …the second must get a conflict carrying the current version.
	cur, err := OptimisticUpdate(ctx, s.DB(), "widgets", "w1", 1, Set{"name", "hammer"})
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("err = %v, want ErrVersionConflict", err)
	}
	if cur != 2 {
		t.Errorf("reported current version = %d, want 2", cur)
	}
	// The stale write must not have been applied.
	var name string
	if err := s.DB().QueryRow(`SELECT name FROM widgets WHERE id = 'w1'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "shot put" {
		t.Errorf("stale write applied: name = %s", name)
	}
}

func TestOptimisticUpdateNotFound(t *testing.T) {
	s := widgetStore(t)
	_, err := OptimisticUpdate(context.Background(), s.DB(), "widgets", "ghost", 1, Set{"name", "x"})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestOptimisticUpdateRejectsBadIdentifiers(t *testing.T) {
	ctx := context.Background()
	s := widgetStore(t)
	cases := []struct {
		table string
		sets  []Set
	}{
		{"widgets; DROP TABLE widgets", []Set{{"name", "x"}}},
		{"widgets", []Set{{"name = 'x' WHERE 1=1; --", "x"}}},
		{"widgets", []Set{{"version", int64(99)}}}, // version is helper-managed
		{"widgets", []Set{{"id", "w2"}}},           // identity is immutable here
		{"widgets", nil},                           // no assignments
	}
	for _, c := range cases {
		if _, err := OptimisticUpdate(ctx, s.DB(), c.table, "w1", 1, c.sets...); err == nil {
			t.Errorf("accepted table=%q sets=%v", c.table, c.sets)
		}
	}
}
