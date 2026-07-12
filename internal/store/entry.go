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

// EntryRecord is a persisted domain.Entry plus its version.
type EntryRecord struct {
	domain.Entry
	Version int64
}

// ErrDuplicateEntry means the athlete or relay team is already entered for
// the event (the event-scoped UNIQUE indexes in 0009_online_entries.sql,
// UC-003 #1).
var ErrDuplicateEntry = errors.New("athlete or relay team already entered for this event")

// entryColumns is the plain (unqualified) SELECT list shared by every entry
// query that reads from the entries table alone.
const entryColumns = `id, event_id, athlete_id, relay_team_id, seed_performance,
	status, source, started_up, started_down, fails_standard, submitted_by, version`

// entryColumnsQualified is the same columns, "en."-qualified for the queries
// that join entries to events.
const entryColumnsQualified = `en.id, en.event_id, en.athlete_id, en.relay_team_id, en.seed_performance,
	en.status, en.source, en.started_up, en.started_down, en.fails_standard, en.submitted_by, en.version`

// CreateEntry inserts a new online/import/manual entry (SYS-011/012).
func CreateEntry(ctx context.Context, db DBTX, e domain.Entry) (EntryRecord, error) {
	e.ID = NewID()
	if e.Status == "" {
		e.Status = domain.EntryEntered
	}
	if e.Source == "" {
		e.Source = domain.EntrySourceOnline
	}
	if err := e.Validate(); err != nil {
		return EntryRecord{}, err
	}
	_, err := db.ExecContext(ctx, `INSERT INTO entries
		(id, event_id, athlete_id, relay_team_id, seed_performance, status, source, started_up, started_down, fails_standard, submitted_by, version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		e.ID, e.EventID, e.AthleteID, e.RelayTeamID, e.SeedPerformance, string(e.Status), string(e.Source),
		boolToInt(e.StartedUp), boolToInt(e.StartedDown), boolToInt(e.FailsStandard), e.SubmittedBy)
	if err != nil {
		if isUniqueViolation(err) {
			return EntryRecord{}, ErrDuplicateEntry
		}
		return EntryRecord{}, fmt.Errorf("create entry for event %s: %w", e.EventID, err)
	}
	return EntryRecord{Entry: e, Version: 1}, nil
}

// GetEntry looks up one entry by ID.
func GetEntry(ctx context.Context, db DBTX, id string) (EntryRecord, error) {
	return scanEntry(db.QueryRowContext(ctx, `SELECT `+entryColumns+` FROM entries WHERE id = ?`, id))
}

// ListEntriesByEvent returns every entry for one event (organizer/office
// views, entry-limit counting).
func ListEntriesByEvent(ctx context.Context, db DBTX, eventID string) ([]EntryRecord, error) {
	return queryEntries(ctx, db, `SELECT `+entryColumns+` FROM entries WHERE event_id = ? ORDER BY id`, eventID)
}

// GetEntryByEventAthlete looks up the (at most one, per the event/athlete
// unique index) entry for one athlete at one event — the CSV/Alabus import
// path's idempotency lookup (SYS-013, UC-004 #2): re-importing the same row
// finds the entry a prior import already created instead of colliding on
// ErrDuplicateEntry with no way back to that entry's ID. Returns ErrNotFound
// if the athlete has no entry at eventID.
func GetEntryByEventAthlete(ctx context.Context, db DBTX, eventID, athleteID string) (EntryRecord, error) {
	return scanEntry(db.QueryRowContext(ctx, `SELECT `+entryColumns+`
		FROM entries WHERE event_id = ? AND athlete_id = ?`, eventID, athleteID))
}

// ListEntriesByMeet returns every entry across a meet's programme (UC-006
// exception report and fee summary).
func ListEntriesByMeet(ctx context.Context, db DBTX, meetID string) ([]EntryRecord, error) {
	return queryEntries(ctx, db, `SELECT `+entryColumnsQualified+`
		FROM entries en JOIN events ev ON ev.id = en.event_id
		WHERE ev.meet_id = ? ORDER BY en.id`, meetID)
}

// ListEntriesBySubmitter returns the entries a given account submitted at
// one meet (UC-003 #1/#2: "the entry is visible to them with status
// entered").
func ListEntriesBySubmitter(ctx context.Context, db DBTX, meetID, submittedBy string) ([]EntryRecord, error) {
	return queryEntries(ctx, db, `SELECT `+entryColumnsQualified+`
		FROM entries en JOIN events ev ON ev.id = en.event_id
		WHERE ev.meet_id = ? AND en.submitted_by = ? ORDER BY en.id`, meetID, submittedBy)
}

// CountActiveEntriesForEvent counts an event's non-scratched entries (SYS-015
// entry-limit enforcement, UC-003).
func CountActiveEntriesForEvent(ctx context.Context, db DBTX, eventID string) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT count(*) FROM entries
		WHERE event_id = ? AND status <> ?`, eventID, string(domain.EntryScratched)).Scan(&n)
	return n, err
}

// UpdateEntryStatus moves an entry through its lifecycle (SYS-016) under
// optimistic concurrency.
func UpdateEntryStatus(ctx context.Context, db DBTX, id string, expectedVersion int64, status domain.EntryStatus) (int64, error) {
	return OptimisticUpdate(ctx, db, "entries", id, expectedVersion,
		Set{Column: "status", Value: string(status)})
}

func queryEntries(ctx context.Context, db DBTX, query string, args ...any) ([]EntryRecord, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EntryRecord
	for rows.Next() {
		e, err := scanEntryRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanEntry(row *sql.Row) (EntryRecord, error) {
	var e EntryRecord
	var status, source string
	var startedUp, startedDown, failsStandard int
	err := row.Scan(&e.ID, &e.EventID, &e.AthleteID, &e.RelayTeamID, &e.SeedPerformance,
		&status, &source, &startedUp, &startedDown, &failsStandard, &e.SubmittedBy, &e.Version)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return EntryRecord{}, ErrNotFound
	case err != nil:
		return EntryRecord{}, err
	}
	return decodeEntry(e, status, source, startedUp, startedDown, failsStandard), nil
}

func scanEntryRow(rows *sql.Rows) (EntryRecord, error) {
	var e EntryRecord
	var status, source string
	var startedUp, startedDown, failsStandard int
	if err := rows.Scan(&e.ID, &e.EventID, &e.AthleteID, &e.RelayTeamID, &e.SeedPerformance,
		&status, &source, &startedUp, &startedDown, &failsStandard, &e.SubmittedBy, &e.Version); err != nil {
		return EntryRecord{}, err
	}
	return decodeEntry(e, status, source, startedUp, startedDown, failsStandard), nil
}

func decodeEntry(e EntryRecord, status, source string, startedUp, startedDown, failsStandard int) EntryRecord {
	e.Status = domain.EntryStatus(status)
	e.Source = domain.EntrySource(source)
	e.StartedUp = startedUp != 0
	e.StartedDown = startedDown != 0
	e.FailsStandard = failsStandard != 0
	return e
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
