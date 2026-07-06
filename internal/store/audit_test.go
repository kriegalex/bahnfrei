// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

// appendAudit wraps one AppendAudit in its own committed transaction, as
// production callers do around their paired mutation.
func appendAudit(t *testing.T, s *Store, e AuditEntry) (int64, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := s.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	seq, err := AppendAudit(ctx, tx, e)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return seq, nil
}

func TestAuditAppendAndTrail(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	seq1, err := appendAudit(t, s, AuditEntry{
		Actor: "office-1", Action: "result.confirm", EntityType: "result", EntityID: "r1",
		After: `{"mark":"11.24"}`,
	})
	if err != nil {
		t.Fatalf("AppendAudit: %v", err)
	}
	seq2, err := appendAudit(t, s, AuditEntry{
		Actor: "office-1", Action: "result.correct", EntityType: "result", EntityID: "r1",
		Before: `{"mark":"11.24"}`, After: `{"mark":"11.42"}`, Reason: "transposed digits",
	})
	if err != nil {
		t.Fatal(err)
	}
	if seq2 != seq1+1 {
		t.Errorf("sequence not monotonic: %d then %d", seq1, seq2)
	}

	trail, err := AuditTrail(ctx, s.DB(), "result", "r1")
	if err != nil {
		t.Fatal(err)
	}
	if len(trail) != 2 {
		t.Fatalf("trail length = %d, want 2", len(trail))
	}
	if trail[1].Reason != "transposed digits" || trail[1].Before != `{"mark":"11.24"}` {
		t.Errorf("correction row mangled: %+v", trail[1])
	}
	if time.Since(trail[0].TS) > time.Minute {
		t.Errorf("timestamp implausible: %v", trail[0].TS)
	}
}

func TestAuditMandatoryFields(t *testing.T) {
	s := openTest(t)
	_, err := appendAudit(t, s, AuditEntry{Actor: "x", Action: "y"})
	if err == nil || !strings.Contains(err.Error(), "mandatory") {
		t.Errorf("incomplete entry accepted, err = %v", err)
	}
}

// SYS-046 / ADR-004 §3: the audit log is append-only, enforced in the
// database itself, not by application discipline.
func TestAuditImmutableByTrigger(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	seq, err := appendAudit(t, s, AuditEntry{
		Actor: "a", Action: "b", EntityType: "c", EntityID: "d",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.DB().ExecContext(ctx,
		`UPDATE audit_log SET actor = 'tampered' WHERE seq = ?`, seq); err == nil {
		t.Error("UPDATE on audit_log succeeded — trigger missing")
	} else if !strings.Contains(err.Error(), "append-only") {
		t.Errorf("unexpected UPDATE error: %v", err)
	}

	if _, err := s.DB().ExecContext(ctx,
		`DELETE FROM audit_log WHERE seq = ?`, seq); err == nil {
		t.Error("DELETE on audit_log succeeded — trigger missing")
	} else if !strings.Contains(err.Error(), "append-only") {
		t.Errorf("unexpected DELETE error: %v", err)
	}

	// And the row is untouched.
	trail, err := AuditTrail(ctx, s.DB(), "c", "d")
	if err != nil {
		t.Fatal(err)
	}
	if len(trail) != 1 || trail[0].Actor != "a" {
		t.Errorf("audit row altered despite triggers: %+v", trail)
	}
}
