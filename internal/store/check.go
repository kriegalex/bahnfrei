// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"fmt"
	"strings"
)

// CheckReport is the startup consistency check result (ADR-004 §1, UC-020 #2):
// it must be green before the application accepts writes.
type CheckReport struct {
	// Integrity holds PRAGMA integrity_check findings; empty means ok.
	Integrity []string
	// FKViolations counts PRAGMA foreign_key_check rows.
	FKViolations int
	// AuditRows and AuditMaxSeq must match: single-writer + rollback
	// semantics make the append-only sequence gapless (invariant query).
	// This is a proxy that assumes no legitimate gaps ever exist; anything
	// that would create one (e.g. the SYS-101 redaction routine, if it ever
	// deletes rather than nulls columns) must revisit this invariant.
	AuditRows   int64
	AuditMaxSeq int64
}

func (r *CheckReport) OK() bool {
	return len(r.Integrity) == 0 && r.FKViolations == 0 && r.AuditRows == r.AuditMaxSeq
}

func (r *CheckReport) Summary() string {
	if r.OK() {
		return "consistency check green"
	}
	var b strings.Builder
	b.WriteString("consistency check RED:")
	if len(r.Integrity) > 0 {
		fmt.Fprintf(&b, " integrity_check: %s;", strings.Join(r.Integrity, "; "))
	}
	if r.FKViolations > 0 {
		fmt.Fprintf(&b, " %d foreign-key violation(s);", r.FKViolations)
	}
	if r.AuditRows != r.AuditMaxSeq {
		fmt.Fprintf(&b, " audit sequence gap: %d rows, max seq %d;", r.AuditRows, r.AuditMaxSeq)
	}
	return strings.TrimSuffix(b.String(), ";")
}

// Check runs the consistency check. An error means the check itself could not
// run; a returned report may still be red — callers decide via OK().
func (s *Store) Check(ctx context.Context) (*CheckReport, error) {
	r := &CheckReport{}

	rows, err := s.db.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			rows.Close()
			return nil, err
		}
		if line != "ok" {
			r.Integrity = append(r.Integrity, line)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	fkRows, err := s.db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return nil, err
	}
	for fkRows.Next() {
		r.FKViolations++
	}
	fkRows.Close()
	if err := fkRows.Err(); err != nil {
		return nil, err
	}

	if err := s.db.QueryRowContext(ctx,
		"SELECT count(*), coalesce(max(seq), 0) FROM audit_log").
		Scan(&r.AuditRows, &r.AuditMaxSeq); err != nil {
		return nil, err
	}
	return r, nil
}
