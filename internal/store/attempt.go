// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// AttemptRecord is a persisted domain.Attempt within its unit, plus its
// optimistic-concurrency version.
type AttemptRecord struct {
	domain.Attempt
	ID        string
	UnitID    string
	AthleteID string
	Version   int64
}

// SaveAttempt writes one trial of a horizontal field event (SYS-042).
// expectedVersion 0 inserts a new attempt; a positive value re-captures an
// existing one under optimistic concurrency. A stale expectation — including
// inserting a trial another device already captured — returns
// ErrVersionConflict together with the currently stored attempt, so the
// caller can surface both versions instead of silently overwriting
// (UC-021 #2, SYS-086's "never silently discarded").
func SaveAttempt(ctx context.Context, db DBTX, unitID, athleteID string, a domain.Attempt, expectedVersion int64) (AttemptRecord, error) {
	if unitID == "" || athleteID == "" {
		return AttemptRecord{}, errors.New("attempt: unit id and athlete id are required")
	}
	if expectedVersion > 0 {
		newVersion, err := OptimisticUpdate(ctx, db, "attempts", attemptID(ctx, db, unitID, athleteID, a.Seq), expectedVersion,
			Set{Column: "kind", Value: string(a.Kind)},
			Set{Column: "mark", Value: a.Mark},
			Set{Column: "wind", Value: a.Wind})
		if err != nil {
			return attemptConflict(ctx, db, unitID, athleteID, a.Seq, err)
		}
		rec, err := GetAttempt(ctx, db, unitID, athleteID, a.Seq)
		if err != nil {
			return AttemptRecord{}, err
		}
		rec.Version = newVersion
		return rec, nil
	}

	rec := AttemptRecord{Attempt: a, ID: NewID(), UnitID: unitID, AthleteID: athleteID, Version: 1}
	_, err := db.ExecContext(ctx, `INSERT INTO attempts
		(id, unit_id, athlete_id, seq, kind, mark, wind, version)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1)`,
		rec.ID, unitID, athleteID, a.Seq, string(a.Kind), a.Mark, a.Wind)
	if err != nil {
		if isUniqueViolation(err) {
			return attemptConflict(ctx, db, unitID, athleteID, a.Seq, ErrVersionConflict)
		}
		return AttemptRecord{}, fmt.Errorf("save attempt unit %s athlete %s trial %d: %w", unitID, athleteID, a.Seq, err)
	}
	return rec, nil
}

// attemptID resolves the row id of (unit, athlete, seq); "" when absent —
// OptimisticUpdate then reports ErrNotFound.
func attemptID(ctx context.Context, db DBTX, unitID, athleteID string, seq int) string {
	var id string
	_ = db.QueryRowContext(ctx, `SELECT id FROM attempts
		WHERE unit_id = ? AND athlete_id = ? AND seq = ?`, unitID, athleteID, seq).Scan(&id)
	return id
}

// attemptConflict decorates a version-conflict error with the attempt
// currently stored, so callers can show both versions (UC-021 #2).
func attemptConflict(ctx context.Context, db DBTX, unitID, athleteID string, seq int, cause error) (AttemptRecord, error) {
	current, err := GetAttempt(ctx, db, unitID, athleteID, seq)
	if err != nil {
		return AttemptRecord{}, cause
	}
	return current, fmt.Errorf("attempt unit %s athlete %s trial %d: %w", unitID, athleteID, seq, cause)
}

// GetAttempt returns the stored trial (unit, athlete, seq).
func GetAttempt(ctx context.Context, db DBTX, unitID, athleteID string, seq int) (AttemptRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, unit_id, athlete_id, seq, kind, mark, wind, version
		FROM attempts WHERE unit_id = ? AND athlete_id = ? AND seq = ?`, unitID, athleteID, seq)
	if err != nil {
		return AttemptRecord{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return AttemptRecord{}, err
		}
		return AttemptRecord{}, ErrNotFound
	}
	return scanAttempt(rows)
}

// ListUnitAttempts returns every trial captured for a unit, per athlete in
// trial order — the capture grid's and series recomputation's input.
func ListUnitAttempts(ctx context.Context, db DBTX, unitID string) ([]AttemptRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, unit_id, athlete_id, seq, kind, mark, wind, version
		FROM attempts WHERE unit_id = ? ORDER BY athlete_id, seq`, unitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AttemptRecord
	for rows.Next() {
		rec, err := scanAttempt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func scanAttempt(rows *sql.Rows) (AttemptRecord, error) {
	var rec AttemptRecord
	var kind string
	if err := rows.Scan(&rec.ID, &rec.UnitID, &rec.AthleteID, &rec.Seq,
		&kind, &rec.Mark, &rec.Wind, &rec.Version); err != nil {
		return AttemptRecord{}, err
	}
	rec.Kind = domain.AttemptKind(kind)
	return rec, nil
}
