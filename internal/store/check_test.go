// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckGreenOnFreshStore(t *testing.T) {
	s := openTest(t)
	report, err := s.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK() {
		t.Errorf("fresh store red: %s", report.Summary())
	}
	if got := report.Summary(); got != "consistency check green" {
		t.Errorf("Summary() = %q", got)
	}
}

func TestCheckDetectsForeignKeyViolations(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	// Plant an orphan row with FK enforcement off for this connection
	// (MaxOpenConns(1) makes the pragma stick for the next statements).
	stmts := []string{
		`CREATE TABLE parents (id TEXT PRIMARY KEY)`,
		`CREATE TABLE children (id TEXT PRIMARY KEY,
			parent_id TEXT NOT NULL REFERENCES parents(id))`,
		`PRAGMA foreign_keys = OFF`,
		`INSERT INTO children (id, parent_id) VALUES ('c1', 'no-such-parent')`,
		`PRAGMA foreign_keys = ON`,
	}
	for _, q := range stmts {
		if _, err := s.DB().ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	report, err := s.Check(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK() {
		t.Fatal("orphan row not detected")
	}
	if report.FKViolations != 1 {
		t.Errorf("FKViolations = %d, want 1", report.FKViolations)
	}
	if !strings.Contains(report.Summary(), "foreign-key") {
		t.Errorf("Summary() = %q, want foreign-key mention", report.Summary())
	}
}

func TestCheckDetectsAuditSequenceGap(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	// Appending with an explicit far-future seq is INSERT (triggers allow it)
	// but breaks the gapless invariant — the checker must notice.
	if _, err := s.DB().ExecContext(ctx, `INSERT INTO audit_log
		(seq, actor, action, entity_type, entity_id)
		VALUES (999, 'x', 'y', 'z', 'w')`); err != nil {
		t.Fatal(err)
	}
	report, err := s.Check(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK() {
		t.Fatal("audit sequence gap not detected")
	}
	if !strings.Contains(report.Summary(), "audit sequence gap") {
		t.Errorf("Summary() = %q, want audit gap mention", report.Summary())
	}
}

// ADR-004 §1: a red check must block startup — Open fails rather than
// accepting writes on an inconsistent database.
func TestOpenRefusesRedStore(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "red.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().ExecContext(ctx, `INSERT INTO audit_log
		(seq, actor, action, entity_type, entity_id)
		VALUES (999, 'x', 'y', 'z', 'w')`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, path); err == nil ||
		!strings.Contains(err.Error(), "consistency check") {
		t.Errorf("Open on red store: err = %v, want consistency-check failure", err)
	}
}

func TestOpenFailsOnUnusablePath(t *testing.T) {
	_, err := Open(context.Background(),
		filepath.Join(t.TempDir(), "no-such-dir", "x.db"))
	if err == nil {
		t.Error("Open with missing parent directory succeeded")
	}
}

func TestAuditTrailRejectsBadTimestamp(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	if _, err := s.DB().ExecContext(ctx, `INSERT INTO audit_log
		(ts, actor, action, entity_type, entity_id)
		VALUES ('not-a-time', 'a', 'b', 'c', 'd')`); err != nil {
		t.Fatal(err)
	}
	if _, err := AuditTrail(ctx, s.DB(), "c", "d"); err == nil ||
		!strings.Contains(err.Error(), "bad timestamp") {
		t.Errorf("err = %v, want bad-timestamp failure", err)
	}
}
