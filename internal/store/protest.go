// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SetUnitWind upserts the single per-race wind reading for a unit (SYS-040,
// TASK-019): every athlete's result in the race carries the same reading,
// applied uniformly by UpdateResultsWind in the same transaction the caller
// opens.
func SetUnitWind(ctx context.Context, db DBTX, unitID string, wind float64) error {
	_, err := db.ExecContext(ctx, `INSERT INTO unit_wind (unit_id, wind) VALUES (?, ?)
		ON CONFLICT (unit_id) DO UPDATE SET wind = excluded.wind`, unitID, wind)
	if err != nil {
		return fmt.Errorf("set unit wind for unit %s: %w", unitID, err)
	}
	return nil
}

// GetUnitWind returns the unit's current wind reading, or nil if none has
// been recorded yet.
func GetUnitWind(ctx context.Context, db DBTX, unitID string) (*float64, error) {
	var wind float64
	err := db.QueryRowContext(ctx, `SELECT wind FROM unit_wind WHERE unit_id = ?`, unitID).Scan(&wind)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get unit wind for unit %s: %w", unitID, err)
	}
	return &wind, nil
}

// UpdateResultsWind writes wind onto every already-settled result of a unit
// (SYS-040, UC-010 #4: "every mark in that race is flagged wind-assisted"),
// version-bumped for optimistic-concurrency consumers.
func UpdateResultsWind(ctx context.Context, db DBTX, unitID string, wind *float64) error {
	_, err := db.ExecContext(ctx, `UPDATE results SET wind = ?, version = version + 1 WHERE unit_id = ?`, wind, unitID)
	if err != nil {
		return fmt.Errorf("update results wind for unit %s: %w", unitID, err)
	}
	return nil
}

// AnnounceUnit records a new result-list announcement for a unit
// (SYS-047, UC-015 #1/#2): the next sequence number after the unit's
// current latest, timestamped now. The first announcement opens the
// protest window; a correction's re-announcement (TASK-019's
// app.CorrectResult) reopens the appeal window.
func AnnounceUnit(ctx context.Context, db DBTX, unitID, actor string) (seq int, announcedAt time.Time, err error) {
	row := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) FROM result_announcements WHERE unit_id = ?`, unitID)
	var lastSeq int
	if err := row.Scan(&lastSeq); err != nil {
		return 0, time.Time{}, fmt.Errorf("announce unit %s: %w", unitID, err)
	}
	seq = lastSeq + 1
	announcedAt = time.Now().UTC()
	_, err = db.ExecContext(ctx, `INSERT INTO result_announcements (id, unit_id, seq, announced_at, actor)
		VALUES (?, ?, ?, ?, ?)`, NewID(), unitID, seq, announcedAt.Format(instantFormat), actor)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("announce unit %s: %w", unitID, err)
	}
	return seq, announcedAt, nil
}

// LatestAnnouncement returns a unit's most recent announcement timestamp,
// and whether one exists at all (SYS-047).
func LatestAnnouncement(ctx context.Context, db DBTX, unitID string) (announcedAt time.Time, found bool, err error) {
	var raw string
	row := db.QueryRowContext(ctx, `SELECT announced_at FROM result_announcements
		WHERE unit_id = ? ORDER BY seq DESC LIMIT 1`, unitID)
	if err := row.Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, fmt.Errorf("latest announcement for unit %s: %w", unitID, err)
	}
	at, err := time.Parse(instantFormat, raw)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("latest announcement for unit %s: bad timestamp %q: %w", unitID, raw, err)
	}
	return at, true, nil
}
