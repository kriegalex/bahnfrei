// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// EntryEligibilityRecord is the persisted eligibility evaluation for one
// entry (TASK-017, SYS-014): the most recent domain.EligibilityResult plus
// an optional operator override (UC-005 #4).
type EntryEligibilityRecord struct {
	EntryID        string
	Outcome        domain.EligibilityOutcome
	Flags          []domain.EligibilityFlag
	OverriddenBy   string
	OverrideReason string
	OverriddenAt   *time.Time
	Version        int64
}

// Overridden reports whether an authorized operator has recorded an
// eligibility override for this entry (UC-005 #4).
func (r EntryEligibilityRecord) Overridden() bool { return r.OverriddenBy != "" }

// EffectiveOutcome is the outcome that gates start-list generation (SYS-014
// "flagged before start-list generation"): a recorded override always clears
// a blocked/warning entry to eligible, while preserving the original
// Outcome/Flags for audit/display.
func (r EntryEligibilityRecord) EffectiveOutcome() domain.EligibilityOutcome {
	if r.Overridden() {
		return domain.EligibilityEligible
	}
	return r.Outcome
}

// UpsertEntryEligibility records the outcome of (re-)evaluating one entry's
// eligibility (SYS-014). Any prior override is cleared — a re-evaluation
// (e.g. a repeat import) reconsiders from scratch, and a still-applicable
// override must be re-recorded by an authorized operator with a fresh
// reason; the original override event remains in the append-only audit log
// regardless (SYS-046).
func UpsertEntryEligibility(ctx context.Context, db DBTX, entryID string, result domain.EligibilityResult) error {
	flags, err := json.Marshal(result.Flags)
	if err != nil {
		return fmt.Errorf("upsert entry eligibility: %w", err)
	}
	outcome := result.Outcome
	if outcome == "" {
		outcome = domain.EligibilityEligible
	}
	_, err = db.ExecContext(ctx, `INSERT INTO entry_eligibility (id, outcome, flags_json, version)
		VALUES (?, ?, ?, 1)
		ON CONFLICT(id) DO UPDATE SET
			outcome = excluded.outcome, flags_json = excluded.flags_json,
			overridden_by = '', override_reason = '', overridden_at = '',
			version = entry_eligibility.version + 1`,
		entryID, string(outcome), string(flags))
	if err != nil {
		return fmt.Errorf("upsert entry eligibility for entry %s: %w", entryID, err)
	}
	return nil
}

// GetEntryEligibility looks up one entry's eligibility record. Returns
// ErrNotFound if the entry has never been evaluated (callers should treat
// that as eligible/no-flags — every entry pathway that creates flags also
// calls UpsertEntryEligibility, so an absent row means no rule applied,
// not that evaluation was skipped).
func GetEntryEligibility(ctx context.Context, db DBTX, entryID string) (EntryEligibilityRecord, error) {
	return scanEligibility(db.QueryRowContext(ctx, `SELECT id, outcome, flags_json,
		overridden_by, override_reason, overridden_at, version
		FROM entry_eligibility WHERE id = ?`, entryID))
}

// ListEntryEligibilityByMeet returns every recorded eligibility evaluation
// for entries at meetID, keyed by entry ID (the office eligibility-exceptions
// view, UC-005).
func ListEntryEligibilityByMeet(ctx context.Context, db DBTX, meetID string) (map[string]EntryEligibilityRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT ee.id, ee.outcome, ee.flags_json,
		ee.overridden_by, ee.override_reason, ee.overridden_at, ee.version
		FROM entry_eligibility ee
		JOIN entries en ON en.id = ee.id
		JOIN events ev ON ev.id = en.event_id
		WHERE ev.meet_id = ?`, meetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]EntryEligibilityRecord{}
	for rows.Next() {
		rec, err := scanEligibilityRow(rows)
		if err != nil {
			return nil, err
		}
		out[rec.EntryID] = rec
	}
	return out, rows.Err()
}

// OverrideEntryEligibility records an authorized operator's override of a
// blocked/warning entry (UC-005 #4: "an authorized operator overrides with a
// reason, then the entry proceeds and the override ... is in the audit
// trail") under optimistic concurrency. Callers also append the
// audit_log row (internal/app, SYS-046) — this call only updates the
// current-state cache the entries/eligibility views read.
func OverrideEntryEligibility(ctx context.Context, db DBTX, entryID string, expectedVersion int64, actor, reason string, at time.Time) (int64, error) {
	if actor == "" {
		return 0, errors.New("eligibility override: actor is required")
	}
	if reason == "" {
		return 0, errors.New("eligibility override: reason is required")
	}
	return OptimisticUpdate(ctx, db, "entry_eligibility", entryID, expectedVersion,
		Set{Column: "overridden_by", Value: actor},
		Set{Column: "override_reason", Value: reason},
		Set{Column: "overridden_at", Value: at.UTC().Format(time.RFC3339)},
	)
}

func scanEligibility(row *sql.Row) (EntryEligibilityRecord, error) {
	var rec EntryEligibilityRecord
	var outcome, flags, overriddenAt sql.NullString
	err := row.Scan(&rec.EntryID, &outcome, &flags, &rec.OverriddenBy, &rec.OverrideReason, &overriddenAt, &rec.Version)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return EntryEligibilityRecord{}, ErrNotFound
	case err != nil:
		return EntryEligibilityRecord{}, err
	}
	return decodeEligibility(rec, outcome, flags, overriddenAt)
}

func scanEligibilityRow(rows *sql.Rows) (EntryEligibilityRecord, error) {
	var rec EntryEligibilityRecord
	var outcome, flags, overriddenAt sql.NullString
	if err := rows.Scan(&rec.EntryID, &outcome, &flags, &rec.OverriddenBy, &rec.OverrideReason, &overriddenAt, &rec.Version); err != nil {
		return EntryEligibilityRecord{}, err
	}
	return decodeEligibility(rec, outcome, flags, overriddenAt)
}

func decodeEligibility(rec EntryEligibilityRecord, outcome, flags, overriddenAt sql.NullString) (EntryEligibilityRecord, error) {
	rec.Outcome = domain.EligibilityOutcome(outcome.String)
	if flags.Valid && flags.String != "" {
		if err := json.Unmarshal([]byte(flags.String), &rec.Flags); err != nil {
			return EntryEligibilityRecord{}, fmt.Errorf("entry eligibility %s: bad flags: %w", rec.EntryID, err)
		}
	}
	if overriddenAt.Valid && overriddenAt.String != "" {
		t, err := time.Parse(time.RFC3339, overriddenAt.String)
		if err != nil {
			return EntryEligibilityRecord{}, fmt.Errorf("entry eligibility %s: bad overridden_at: %w", rec.EntryID, err)
		}
		rec.OverriddenAt = &t
	}
	return rec, nil
}
