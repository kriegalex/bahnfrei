// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// AddMeetRecordList associates recordListID with meetID (SYS-049): a meet
// may load more than one list (its own meeting-record list plus a shared
// national list, say), so this is additive — calling it again with a
// different ID adds a second association rather than replacing the first
// (0019_records.sql's many-to-many shape). Re-adding the same pair is a
// no-op.
func AddMeetRecordList(ctx context.Context, db DBTX, meetID, recordListID string) error {
	if meetID == "" || recordListID == "" {
		return errors.New("meet record list: meet id and record list id are required")
	}
	_, err := db.ExecContext(ctx, `INSERT INTO meet_record_lists (meet_id, record_list_id) VALUES (?, ?)
		ON CONFLICT(meet_id, record_list_id) DO NOTHING`, meetID, recordListID)
	if err != nil {
		return fmt.Errorf("add meet record list for %s: %w", meetID, err)
	}
	return nil
}

// RemoveMeetRecordList removes one meet/record-list association, if present.
func RemoveMeetRecordList(ctx context.Context, db DBTX, meetID, recordListID string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM meet_record_lists WHERE meet_id = ? AND record_list_id = ?`, meetID, recordListID)
	if err != nil {
		return fmt.Errorf("remove meet record list for %s: %w", meetID, err)
	}
	return nil
}

// ReplaceMeetRecordLists sets meetID's full set of configured record lists
// to exactly listIDs (SYS-049, UC-016 operator configuration), replacing
// whatever was configured before. Call within a transaction alongside the
// caller's audit write, like every other replace-semantics setter in this
// package.
func ReplaceMeetRecordLists(ctx context.Context, db DBTX, meetID string, listIDs []string) error {
	if meetID == "" {
		return errors.New("meet record lists: meet id is required")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM meet_record_lists WHERE meet_id = ?`, meetID); err != nil {
		return fmt.Errorf("replace meet record lists for %s: %w", meetID, err)
	}
	for _, id := range listIDs {
		if err := AddMeetRecordList(ctx, db, meetID, id); err != nil {
			return err
		}
	}
	return nil
}

// ListMeetRecordListIDs returns the record-list IDs configured for meetID,
// in no particular order — a meet with none returns an empty slice (not an
// error): it is simply not evaluated against any reference record.
func ListMeetRecordListIDs(ctx context.Context, db DBTX, meetID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT record_list_id FROM meet_record_lists WHERE meet_id = ?`, meetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// AthleteDisciplineResult is one prior settled result of an athlete in a
// discipline, across every meet (athletes are instance-global, SYS-010) —
// the input to the SYS-049 PB/SB history reduction
// (domain.BestMarks). Excludes status-only rows (DNS/DNF/NM/DQ/…, no mark)
// and empty marks: a personal/season best is always an actual measured
// performance.
type AthleteDisciplineResult struct {
	ResultID      string
	Mark          string
	Wind          *float64
	MeetStartDate time.Time
}

// ListAthleteResultsByDiscipline returns athleteID's settled marks in
// disciplineCode across every meet, for the PB/SB reduction. excludeUnitID
// (empty to include everything) excludes the one row this same (unit,
// athlete) pair might already hold — a capture flow re-saving the same mark
// before announcement (an operator correcting a typo, say) must never count
// its own prior save as history to beat (the results table's (unit_id,
// athlete_id) uniqueness means excluding by unit id is equivalent to, and
// simpler than, excluding by an as-yet-unknown result id).
func ListAthleteResultsByDiscipline(ctx context.Context, db DBTX, athleteID, disciplineCode, excludeUnitID string) ([]AthleteDisciplineResult, error) {
	rows, err := db.QueryContext(ctx, `SELECT r.id, r.mark, r.wind, m.start_date
		FROM results r
		JOIN units u ON u.id = r.unit_id
		JOIN rounds rd ON rd.id = u.round_id
		JOIN events e ON e.id = rd.event_id
		JOIN meets m ON m.id = e.meet_id
		WHERE r.athlete_id = ? AND e.discipline_code = ? AND r.mark <> '' AND r.status = '' AND r.unit_id <> ?`,
		athleteID, disciplineCode, excludeUnitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AthleteDisciplineResult
	for rows.Next() {
		var r AthleteDisciplineResult
		var start string
		if err := rows.Scan(&r.ResultID, &r.Mark, &r.Wind, &start); err != nil {
			return nil, err
		}
		if r.MeetStartDate, err = time.Parse(dayFormat, start); err != nil {
			return nil, fmt.Errorf("athlete %s discipline %s result %s: bad meet start date: %w", athleteID, disciplineCode, r.ResultID, err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
