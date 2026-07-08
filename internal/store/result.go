// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// ResultRecord is a persisted domain.Result plus the timing method its
// mark was taken with (scoring tables differ per timing method, SYS-053)
// and its version.
type ResultRecord struct {
	domain.Result
	Timing  domain.Timing
	Version int64
}

// MeetResult is a result joined up its unit → round → event chain, so
// consumers know which discipline and event a mark belongs to without
// N+1 lookups.
type MeetResult struct {
	ResultRecord
	EventID        string
	DisciplineCode string
}

// SaveResult inserts or replaces the settled result for (unit, athlete) —
// capture flows re-submit as marks are corrected; every save bumps the
// version and the caller audits the change (SYS-046). The attempt-level
// sequence is TASK-008's; this row is the settled per-unit outcome.
func SaveResult(ctx context.Context, db DBTX, r domain.Result, timing domain.Timing) (ResultRecord, error) {
	if r.UnitID == "" || r.AthleteID == "" {
		return ResultRecord{}, fmt.Errorf("result: unit id and athlete id are required")
	}
	flags, err := json.Marshal(orEmptySlice(r.RecordFlags))
	if err != nil {
		return ResultRecord{}, fmt.Errorf("save result: %w", err)
	}
	r.ID = NewID()
	_, err = db.ExecContext(ctx, `INSERT INTO results
		(id, unit_id, athlete_id, mark, timing, status, points, wind, lane, placing, record_flags, version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT (unit_id, athlete_id) DO UPDATE SET
		mark = excluded.mark, timing = excluded.timing, status = excluded.status,
		points = excluded.points, wind = excluded.wind, lane = excluded.lane,
		placing = excluded.placing, record_flags = excluded.record_flags,
		version = results.version + 1`,
		r.ID, r.UnitID, r.AthleteID, r.Mark, string(timing), string(r.Status),
		r.Points, r.Wind, r.Lane, r.Placing, string(flags))
	if err != nil {
		return ResultRecord{}, fmt.Errorf("save result unit %s athlete %s: %w", r.UnitID, r.AthleteID, err)
	}
	return GetResult(ctx, db, r.UnitID, r.AthleteID)
}

// GetResult returns the settled result for (unit, athlete).
func GetResult(ctx context.Context, db DBTX, unitID, athleteID string) (ResultRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, unit_id, athlete_id, mark, timing,
		status, points, wind, lane, placing, record_flags, version
		FROM results WHERE unit_id = ? AND athlete_id = ?`, unitID, athleteID)
	if err != nil {
		return ResultRecord{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return ResultRecord{}, err
		}
		return ResultRecord{}, ErrNotFound
	}
	return scanResult(rows)
}

// ListMeetResults returns every settled result of a meet with its event
// and discipline, the standings computation's input (UC-033 #2).
func ListMeetResults(ctx context.Context, db DBTX, meetID string) ([]MeetResult, error) {
	rows, err := db.QueryContext(ctx, `SELECT r.id, r.unit_id, r.athlete_id, r.mark,
		r.timing, r.status, r.points, r.wind, r.lane, r.placing, r.record_flags, r.version,
		e.id, e.discipline_code
		FROM results r
		JOIN units u ON u.id = r.unit_id
		JOIN rounds rd ON rd.id = u.round_id
		JOIN events e ON e.id = rd.event_id
		WHERE e.meet_id = ?
		ORDER BY e.id, r.athlete_id`, meetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MeetResult
	for rows.Next() {
		var m MeetResult
		var timing, status, flags string
		if err := rows.Scan(&m.ID, &m.UnitID, &m.AthleteID, &m.Mark, &timing, &status,
			&m.Points, &m.Wind, &m.Lane, &m.Placing, &flags, &m.Version,
			&m.EventID, &m.DisciplineCode); err != nil {
			return nil, err
		}
		m.Timing = domain.Timing(timing)
		m.Status = domain.QualificationStatus(status)
		if err := json.Unmarshal([]byte(flags), &m.RecordFlags); err != nil {
			return nil, fmt.Errorf("result %s: bad record flags: %w", m.ID, err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func scanResult(rows interface {
	Scan(dest ...any) error
}) (ResultRecord, error) {
	var r ResultRecord
	var timing, status, flags string
	if err := rows.Scan(&r.ID, &r.UnitID, &r.AthleteID, &r.Mark, &timing, &status,
		&r.Points, &r.Wind, &r.Lane, &r.Placing, &flags, &r.Version); err != nil {
		return ResultRecord{}, err
	}
	r.Timing = domain.Timing(timing)
	r.Status = domain.QualificationStatus(status)
	if err := json.Unmarshal([]byte(flags), &r.RecordFlags); err != nil {
		return ResultRecord{}, fmt.Errorf("result %s: bad record flags: %w", r.ID, err)
	}
	return r, nil
}
