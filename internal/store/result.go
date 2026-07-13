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
	Timing domain.Timing
	// Source is the capture provenance (SYS-041, ADR-006 consequence #4):
	// "manual" (operator keyboard entry — SaveResult's default, every
	// pre-existing call site) or "import_lif"/"import_csv" (the
	// timing-exchange ingest pipeline, internal/app/exchange.go). The
	// timing-import conflict pipeline reads this to tell an existing
	// manual result (never silently overwritten, SYS-061) from a row a
	// previous import itself wrote (safe to refresh).
	Source  string
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
// sequence is TASK-008's; this row is the settled per-unit outcome. Every
// call through this entry point is operator capture provenance ("manual",
// SYS-041) — the timing-exchange ingest pipeline uses
// SaveResultWithSource instead so an import never gets misread as a
// manual entry by a later conflict check.
func SaveResult(ctx context.Context, db DBTX, r domain.Result, timing domain.Timing) (ResultRecord, error) {
	return SaveResultWithSource(ctx, db, r, timing, "manual")
}

// SaveResultWithSource is SaveResult with an explicit provenance tag
// (ADR-006 consequence #4). source must be one of the results.source CHECK
// constraint's values ("manual", "import_lif", "import_csv") — the
// migration enforces it as a defense-in-depth backstop.
func SaveResultWithSource(ctx context.Context, db DBTX, r domain.Result, timing domain.Timing, source string) (ResultRecord, error) {
	if r.UnitID == "" || r.AthleteID == "" {
		return ResultRecord{}, fmt.Errorf("result: unit id and athlete id are required")
	}
	flags, err := json.Marshal(orEmptySlice(r.RecordFlags))
	if err != nil {
		return ResultRecord{}, fmt.Errorf("save result: %w", err)
	}
	r.ID = NewID()
	_, err = db.ExecContext(ctx, `INSERT INTO results
		(id, unit_id, athlete_id, mark, timing, status, status_detail, points, wind, lane, placing, record_flags, source, version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT (unit_id, athlete_id) DO UPDATE SET
		mark = excluded.mark, timing = excluded.timing, status = excluded.status,
		status_detail = excluded.status_detail,
		points = excluded.points, wind = excluded.wind, lane = excluded.lane,
		placing = excluded.placing, record_flags = excluded.record_flags,
		source = excluded.source,
		version = results.version + 1`,
		r.ID, r.UnitID, r.AthleteID, r.Mark, string(timing), string(r.Status),
		r.StatusDetail, r.Points, r.Wind, r.Lane, r.Placing, string(flags), source)
	if err != nil {
		return ResultRecord{}, fmt.Errorf("save result unit %s athlete %s: %w", r.UnitID, r.AthleteID, err)
	}
	return GetResult(ctx, db, r.UnitID, r.AthleteID)
}

// SaveResultOptimistic is SaveResultWithSource under an explicit
// optimistic-concurrency check (SYS-083, UC-021 #2), mirroring
// SaveAttempt's pattern: expectedVersion 0 inserts a brand-new result for
// (unit, athlete) — a concurrent first save from another session still
// conflicts via the table's UNIQUE(unit_id, athlete_id) index rather than
// silently coalescing two independent captures; a positive expectedVersion
// re-saves an existing result only if it has not moved on since the
// caller read it. A stale expectation returns ErrVersionConflict together
// with the row currently stored, so the caller can surface both versions
// instead of last-write-wins. Unlike SaveResult/SaveResultWithSource
// (blind upsert — capture flows intentionally re-submit as marks are
// corrected before announcement, SYS-041), this is the entry point for
// callers that DO want the conflict surfaced: two office sessions racing
// to correct the same settled result.
func SaveResultOptimistic(ctx context.Context, db DBTX, r domain.Result, timing domain.Timing, source string, expectedVersion int64) (ResultRecord, error) {
	if r.UnitID == "" || r.AthleteID == "" {
		return ResultRecord{}, fmt.Errorf("result: unit id and athlete id are required")
	}
	flags, err := json.Marshal(orEmptySlice(r.RecordFlags))
	if err != nil {
		return ResultRecord{}, fmt.Errorf("save result: %w", err)
	}

	if expectedVersion > 0 {
		id := resultID(ctx, db, r.UnitID, r.AthleteID)
		newVersion, err := OptimisticUpdate(ctx, db, "results", id, expectedVersion,
			Set{Column: "mark", Value: r.Mark},
			Set{Column: "timing", Value: string(timing)},
			Set{Column: "status", Value: string(r.Status)},
			Set{Column: "status_detail", Value: r.StatusDetail},
			Set{Column: "points", Value: r.Points},
			Set{Column: "wind", Value: r.Wind},
			Set{Column: "lane", Value: r.Lane},
			Set{Column: "placing", Value: r.Placing},
			Set{Column: "record_flags", Value: string(flags)},
			Set{Column: "source", Value: source})
		if err != nil {
			return resultConflict(ctx, db, r.UnitID, r.AthleteID, err)
		}
		rec, err := GetResult(ctx, db, r.UnitID, r.AthleteID)
		if err != nil {
			return ResultRecord{}, err
		}
		rec.Version = newVersion
		return rec, nil
	}

	r.ID = NewID()
	_, err = db.ExecContext(ctx, `INSERT INTO results
		(id, unit_id, athlete_id, mark, timing, status, status_detail, points, wind, lane, placing, record_flags, source, version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		r.ID, r.UnitID, r.AthleteID, r.Mark, string(timing), string(r.Status),
		r.StatusDetail, r.Points, r.Wind, r.Lane, r.Placing, string(flags), source)
	if err != nil {
		if isUniqueViolation(err) {
			return resultConflict(ctx, db, r.UnitID, r.AthleteID, ErrVersionConflict)
		}
		return ResultRecord{}, fmt.Errorf("save result unit %s athlete %s: %w", r.UnitID, r.AthleteID, err)
	}
	return GetResult(ctx, db, r.UnitID, r.AthleteID)
}

// resultID resolves the row id of (unit, athlete); "" when absent —
// OptimisticUpdate then reports ErrNotFound.
func resultID(ctx context.Context, db DBTX, unitID, athleteID string) string {
	var id string
	_ = db.QueryRowContext(ctx, `SELECT id FROM results
		WHERE unit_id = ? AND athlete_id = ?`, unitID, athleteID).Scan(&id)
	return id
}

// resultConflict decorates a version-conflict error with the result
// currently stored, so callers can show both versions (UC-021 #2).
func resultConflict(ctx context.Context, db DBTX, unitID, athleteID string, cause error) (ResultRecord, error) {
	current, err := GetResult(ctx, db, unitID, athleteID)
	if err != nil {
		return ResultRecord{}, cause
	}
	return current, fmt.Errorf("result unit %s athlete %s: %w", unitID, athleteID, cause)
}

// GetResult returns the settled result for (unit, athlete).
func GetResult(ctx context.Context, db DBTX, unitID, athleteID string) (ResultRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, unit_id, athlete_id, mark, timing,
		status, status_detail, points, wind, lane, placing, record_flags, source, version
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
		r.timing, r.status, r.status_detail, r.points, r.wind, r.lane, r.placing, r.record_flags, r.source, r.version,
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
			&m.StatusDetail, &m.Points, &m.Wind, &m.Lane, &m.Placing, &flags, &m.Source, &m.Version,
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

// ListUnitResults returns every settled result of one unit — the capture
// view's and unit-ranking computation's input.
func ListUnitResults(ctx context.Context, db DBTX, unitID string) ([]ResultRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, unit_id, athlete_id, mark, timing,
		status, status_detail, points, wind, lane, placing, record_flags, source, version
		FROM results WHERE unit_id = ? ORDER BY athlete_id`, unitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ResultRecord
	for rows.Next() {
		r, err := scanResult(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanResult(rows interface {
	Scan(dest ...any) error
}) (ResultRecord, error) {
	var r ResultRecord
	var timing, status, flags string
	if err := rows.Scan(&r.ID, &r.UnitID, &r.AthleteID, &r.Mark, &timing, &status,
		&r.StatusDetail, &r.Points, &r.Wind, &r.Lane, &r.Placing, &flags, &r.Source, &r.Version); err != nil {
		return ResultRecord{}, err
	}
	r.Timing = domain.Timing(timing)
	r.Status = domain.QualificationStatus(status)
	if err := json.Unmarshal([]byte(flags), &r.RecordFlags); err != nil {
		return ResultRecord{}, fmt.Errorf("result %s: bad record flags: %w", r.ID, err)
	}
	return r, nil
}
