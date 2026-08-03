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

// UnitAssignmentRecord is a persisted domain.UnitAssignment plus its version
// (TASK-018, SYS-026-030).
type UnitAssignmentRecord struct {
	domain.UnitAssignment
	Version int64
}

const unitAssignmentColumns = `id, unit_id, entry_id, seed_rank, lane, qualification, manual_override, version`

// SaveUnitAssignment inserts or replaces one entry's placement in a unit
// (unit_id, entry_id is unique): heat-seeding regeneration re-submits as
// heats are recomputed. Every save bumps the version.
func SaveUnitAssignment(ctx context.Context, db DBTX, a domain.UnitAssignment) (UnitAssignmentRecord, error) {
	if a.ID == "" {
		a.ID = NewID()
	}
	if err := a.Validate(); err != nil {
		return UnitAssignmentRecord{}, err
	}
	_, err := db.ExecContext(ctx, `INSERT INTO unit_entries
		(id, unit_id, entry_id, seed_rank, lane, qualification, manual_override, version)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT (unit_id, entry_id) DO UPDATE SET
		seed_rank = excluded.seed_rank, lane = excluded.lane,
		qualification = excluded.qualification, manual_override = excluded.manual_override,
		version = unit_entries.version + 1`,
		a.ID, a.UnitID, a.EntryID, a.SeedRank, a.Lane, string(a.Qualification), boolToInt(a.ManualOverride))
	if err != nil {
		return UnitAssignmentRecord{}, fmt.Errorf("save unit assignment (unit %s, entry %s): %w", a.UnitID, a.EntryID, err)
	}
	return GetUnitAssignment(ctx, db, a.UnitID, a.EntryID)
}

// GetUnitAssignment returns one entry's placement in a unit.
func GetUnitAssignment(ctx context.Context, db DBTX, unitID, entryID string) (UnitAssignmentRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+unitAssignmentColumns+`
		FROM unit_entries WHERE unit_id = ? AND entry_id = ?`, unitID, entryID)
	if err != nil {
		return UnitAssignmentRecord{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return UnitAssignmentRecord{}, err
		}
		return UnitAssignmentRecord{}, ErrNotFound
	}
	return scanUnitAssignment(rows)
}

// ListUnitAssignments returns every entry placed in one unit, seed-rank order.
func ListUnitAssignments(ctx context.Context, db DBTX, unitID string) ([]UnitAssignmentRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+unitAssignmentColumns+`
		FROM unit_entries WHERE unit_id = ? ORDER BY seed_rank, id`, unitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UnitAssignmentRecord
	for rows.Next() {
		a, err := scanUnitAssignment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListRoundAssignments returns every entry placed anywhere in a round
// (across all its units), unit then seed-rank order — round-progression's
// input (SYS-029).
func ListRoundAssignments(ctx context.Context, db DBTX, roundID string) ([]UnitAssignmentRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT ue.id, ue.unit_id, ue.entry_id, ue.seed_rank, ue.lane, ue.qualification, ue.manual_override, ue.version
		FROM unit_entries ue JOIN units u ON u.id = ue.unit_id
		WHERE u.round_id = ? ORDER BY ue.unit_id, ue.seed_rank, ue.id`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UnitAssignmentRecord
	for rows.Next() {
		a, err := scanUnitAssignment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteUnitAssignment removes one entry's placement (used when
// regeneration drops an entry that scratched/went DNS since the last
// generation — never removes a manual-override row; callers check that
// first).
func DeleteUnitAssignment(ctx context.Context, db DBTX, unitID, entryID string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM unit_entries WHERE unit_id = ? AND entry_id = ?`, unitID, entryID)
	return err
}

// SetUnitAssignmentQualification records a round-progression outcome
// (SYS-029: Q/q) or a manual advancement (qR/qJ/qD) on one entry's
// placement, under optimistic concurrency.
func SetUnitAssignmentQualification(ctx context.Context, db DBTX, id string, expectedVersion int64, code domain.QualificationStatus) (int64, error) {
	return OptimisticUpdate(ctx, db, "unit_entries", id, expectedVersion,
		Set{Column: "qualification", Value: string(code)})
}

// UpdateUnitAssignmentManual applies an operator's manual heat/lane override
// (SYS-028): the row is marked manual_override so a later regeneration
// leaves it untouched. lane <= 0 leaves the lane unassigned (a manual heat
// swap with no lane change).
func UpdateUnitAssignmentManual(ctx context.Context, db DBTX, id string, expectedVersion int64, unitID string, lane int) (int64, error) {
	return OptimisticUpdate(ctx, db, "unit_entries", id, expectedVersion,
		Set{Column: "unit_id", Value: unitID},
		Set{Column: "lane", Value: lane},
		Set{Column: "manual_override", Value: 1})
}

// ReleaseManualOverride clears the manual-override flag (SYS-028: "unless
// explicitly released") so the next regeneration is free to recompute this
// entry's heat/lane.
func ReleaseManualOverride(ctx context.Context, db DBTX, id string, expectedVersion int64) (int64, error) {
	return OptimisticUpdate(ctx, db, "unit_entries", id, expectedVersion,
		Set{Column: "manual_override", Value: 0})
}

func scanUnitAssignment(rows *sql.Rows) (UnitAssignmentRecord, error) {
	var a UnitAssignmentRecord
	var qualification string
	var manualOverride int
	if err := rows.Scan(&a.ID, &a.UnitID, &a.EntryID, &a.SeedRank, &a.Lane, &qualification, &manualOverride, &a.Version); err != nil {
		return UnitAssignmentRecord{}, err
	}
	a.Qualification = domain.QualificationStatus(qualification)
	a.ManualOverride = manualOverride != 0
	return a, nil
}

// EnsureRoundUnitCount grows a round's unit set to at least count units,
// creating additional units as needed (heat splitting is seeding's job,
// TASK-018 — every round starts with exactly one placeholder unit,
// AddEvent's comment). It never removes units — shrinking could discard
// scheduled/captured data — so a round that already has more units than
// count keeps them all; extra units simply receive no assignments on the
// next regeneration. Returns the round's units in stable (creation) order.
func EnsureRoundUnitCount(ctx context.Context, db DBTX, roundID string, count int) ([]UnitRecord, error) {
	if count <= 0 {
		return nil, errors.New("ensure round unit count: a positive count is required")
	}
	rows, err := db.QueryContext(ctx, `SELECT id, round_id, scheduled_at, location, version
		FROM units WHERE round_id = ? ORDER BY id`, roundID)
	if err != nil {
		return nil, err
	}
	var existing []UnitRecord
	for rows.Next() {
		var u UnitRecord
		var sched sql.NullString
		if err := rows.Scan(&u.ID, &u.RoundID, &sched, &u.Location, &u.Version); err != nil {
			rows.Close() // #nosec G104 -- Close on a read-only result set has no actionable failure mode; the repo's errcheck policy (.golangci.yml) exempts (*sql.Rows).Close for this reason
			return nil, err
		}
		existing = append(existing, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close() // #nosec G104 -- Close on a read-only result set has no actionable failure mode; rows.Err() was already checked above, per the repo's errcheck policy (.golangci.yml)

	for len(existing) < count {
		u, err := CreateUnit(ctx, db, domain.Unit{RoundID: roundID})
		if err != nil {
			return nil, fmt.Errorf("ensure round unit count for round %s: %w", roundID, err)
		}
		existing = append(existing, u)
	}
	return existing, nil
}
