// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import "context"

// FieldOfficialUnit is one per-meet capability grant: accountID may open
// unitID for capture (SYS-090's "assignable per meet"; TASK-013, UC-022).
type FieldOfficialUnit struct {
	AccountID string
	MeetID    string
	UnitID    string
}

// AssignFieldOfficialUnit grants accountID capture access to unitID within
// meetID. Idempotent: assigning an already-granted (account, unit) pair is a
// no-op, not a duplicate-key error.
func AssignFieldOfficialUnit(ctx context.Context, db DBTX, accountID, meetID, unitID string) error {
	_, err := db.ExecContext(ctx, `INSERT INTO field_official_units
		(id, account_id, meet_id, unit_id) VALUES (?, ?, ?, ?)
		ON CONFLICT (account_id, unit_id) DO NOTHING`,
		NewID(), accountID, meetID, unitID)
	return err
}

// UnassignFieldOfficialUnit revokes accountID's capture access to unitID.
// Revoking a grant that does not exist is a no-op.
func UnassignFieldOfficialUnit(ctx context.Context, db DBTX, accountID, unitID string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM field_official_units
		WHERE account_id = ? AND unit_id = ?`, accountID, unitID)
	return err
}

// IsFieldOfficialAssigned reports whether accountID may capture unitID
// (SYS-090 per-event scoping, enforced server-side — UC-022 #1).
func IsFieldOfficialAssigned(ctx context.Context, db DBTX, accountID, unitID string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT count(*) FROM field_official_units
		WHERE account_id = ? AND unit_id = ?`, accountID, unitID).Scan(&n)
	return n > 0, err
}

// AssignedUnitIDs returns the unit IDs of meetID that accountID is assigned
// to, for filtering a field official's capture unit list.
func AssignedUnitIDs(ctx context.Context, db DBTX, accountID, meetID string) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT unit_id FROM field_official_units
		WHERE account_id = ? AND meet_id = ?`, accountID, meetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]bool{}
	for rows.Next() {
		var unitID string
		if err := rows.Scan(&unitID); err != nil {
			return nil, err
		}
		out[unitID] = true
	}
	return out, rows.Err()
}

// ListFieldOfficialAssignments returns every (account, unit) grant recorded
// for meetID, for building the office's assignment matrix (TASK-013).
func ListFieldOfficialAssignments(ctx context.Context, db DBTX, meetID string) ([]FieldOfficialUnit, error) {
	rows, err := db.QueryContext(ctx, `SELECT account_id, meet_id, unit_id
		FROM field_official_units WHERE meet_id = ?`, meetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FieldOfficialUnit
	for rows.Next() {
		var f FieldOfficialUnit
		if err := rows.Scan(&f.AccountID, &f.MeetID, &f.UnitID); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// AssignedMeetIDs returns the distinct meet IDs accountID holds at least
// one current capture assignment in, most-recently-assigned meet first
// (TASK-042, DEC-025 "my assignments" dashboard): the field-official panel
// needs to know which meets to list without scanning every meet in the
// instance.
func AssignedMeetIDs(ctx context.Context, db DBTX, accountID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT meet_id FROM field_official_units
		WHERE account_id = ? GROUP BY meet_id ORDER BY max(created_at) DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var meetID string
		if err := rows.Scan(&meetID); err != nil {
			return nil, err
		}
		out = append(out, meetID)
	}
	return out, rows.Err()
}
