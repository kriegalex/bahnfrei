// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// AuditEntry is one immutable row of the audit trail (SYS-046):
// who did what to which entity, with before/after state and a reason
// for overrides/corrections.
type AuditEntry struct {
	Seq        int64
	TS         time.Time
	Actor      string
	Action     string
	EntityType string
	EntityID   string
	// Before and After hold JSON snapshots; empty means not applicable
	// (e.g. no Before on create).
	Before string
	After  string
	Reason string
}

// AppendAudit appends one entry and returns its monotonic sequence number.
// It deliberately takes *sql.Tx, not DBTX: the audit row must share the
// transaction of the mutation it records so both are durable together
// (SYS-081, ADR-004 §3) — an autocommit append would create a second,
// independent durability point. The table's triggers make later
// UPDATE/DELETE impossible; corrections are new rows.
func AppendAudit(ctx context.Context, tx *sql.Tx, e AuditEntry) (int64, error) {
	if e.Actor == "" || e.Action == "" || e.EntityType == "" || e.EntityID == "" {
		return 0, fmt.Errorf("audit entry incomplete: actor, action, entity_type, entity_id are mandatory (SYS-046)")
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO audit_log
		(actor, action, entity_type, entity_id, before_json, after_json, reason)
		VALUES (?, ?, ?, ?, nullif(?,''), nullif(?,''), nullif(?,''))`,
		e.Actor, e.Action, e.EntityType, e.EntityID, e.Before, e.After, e.Reason)
	if err != nil {
		return 0, fmt.Errorf("append audit: %w", err)
	}
	return res.LastInsertId()
}

// AuditTrail returns the entries for one entity in sequence order.
func AuditTrail(ctx context.Context, db DBTX, entityType, entityID string) ([]AuditEntry, error) {
	rows, err := db.QueryContext(ctx, `SELECT seq, ts, actor, action, entity_type, entity_id,
		coalesce(before_json,''), coalesce(after_json,''), coalesce(reason,'')
		FROM audit_log WHERE entity_type = ? AND entity_id = ? ORDER BY seq`,
		entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var ts string
		if err := rows.Scan(&e.Seq, &ts, &e.Actor, &e.Action, &e.EntityType, &e.EntityID,
			&e.Before, &e.After, &e.Reason); err != nil {
			return nil, err
		}
		if e.TS, err = time.Parse(time.RFC3339Nano, ts); err != nil {
			return nil, fmt.Errorf("audit seq %d: bad timestamp %q: %w", e.Seq, ts, err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
