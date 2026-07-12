// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// SetMeetCombinedScoringTable names which WA combined-events formula table
// (SYS-044) scores meetID's results, replacing any previous choice. A meet
// with no row here scores (if at all) through the ordinary
// meets.scoring_table lookup-table path instead (0014_combined_scoring.sql
// explains why this lives in its own table).
func SetMeetCombinedScoringTable(ctx context.Context, db DBTX, meetID, scoringTableID string) error {
	if meetID == "" || scoringTableID == "" {
		return errors.New("meet combined scoring: meet id and scoring table id are required")
	}
	_, err := db.ExecContext(ctx, `INSERT INTO meet_combined_scoring (meet_id, scoring_table_id) VALUES (?, ?)
		ON CONFLICT(meet_id) DO UPDATE SET scoring_table_id = excluded.scoring_table_id`,
		meetID, scoringTableID)
	if err != nil {
		return fmt.Errorf("set meet combined scoring table for %s: %w", meetID, err)
	}
	return nil
}

// GetMeetCombinedScoringTable returns the combined-events scoring table ID
// configured for meetID, and whether one is configured at all (a meet
// without one is not a combined-events meet).
func GetMeetCombinedScoringTable(ctx context.Context, db DBTX, meetID string) (string, bool, error) {
	var id string
	err := db.QueryRowContext(ctx, `SELECT scoring_table_id FROM meet_combined_scoring WHERE meet_id = ?`, meetID).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", false, nil
	case err != nil:
		return "", false, err
	}
	return id, true, nil
}
