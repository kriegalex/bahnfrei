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

// Storage encodings for the meet tables (0003_meets.sql): competition days
// are date-only, instants carry full RFC 3339 precision (matching audit_log).
const (
	dayFormat     = "2006-01-02"
	instantFormat = time.RFC3339Nano
)

// MeetRecord is a persisted domain.Meet plus its optimistic-concurrency
// version (a persistence concern, deliberately kept off the domain entity).
type MeetRecord struct {
	domain.Meet
	Version int64
}

// EventRecord is a persisted domain.Event plus its version.
type EventRecord struct {
	domain.Event
	Version int64
}

// UnitRecord is a persisted domain.Unit plus its version.
type UnitRecord struct {
	domain.Unit
	Version int64
}

// CreateMeet inserts a new meet in status draft (SYS-001) with a fresh ID.
// ResultsPositioning defaults to federation_official (SYS-076) when unset —
// the conservative default: a freshly created meet never silently implies
// it is an authoritative publication.
func CreateMeet(ctx context.Context, db DBTX, m domain.Meet) (MeetRecord, error) {
	m.ID = NewID()
	if m.Status == "" {
		m.Status = domain.MeetDraft
	}
	if m.ResultsPositioning == "" {
		m.ResultsPositioning = domain.ResultsPositioningFederationOfficial
	}
	if err := m.Validate(); err != nil {
		return MeetRecord{}, err
	}
	_, err := db.ExecContext(ctx, `INSERT INTO meets
		(id, name, venue, homologation_ref, start_date, end_date, organizer, tier, status, category_scheme, template_id, scoring_table, results_positioning, official_source_name, official_source_url, entry_fee_cents, relay_fee_cents, version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		m.ID, m.Name, m.Venue, m.HomologationRef,
		m.StartDate.Format(dayFormat), m.EndDate.Format(dayFormat),
		m.Organizer, string(m.Tier), string(m.Status), m.CategorySchemeID,
		m.TemplateID, m.ScoringTableID, string(m.ResultsPositioning), m.OfficialSourceName, m.OfficialSourceURL,
		m.EntryFeeCents, m.RelayFeeCents)
	if err != nil {
		return MeetRecord{}, fmt.Errorf("create meet %q: %w", m.Name, err)
	}
	return MeetRecord{Meet: m, Version: 1}, nil
}

// GetMeet looks up one meet by ID.
func GetMeet(ctx context.Context, db DBTX, id string) (MeetRecord, error) {
	return scanMeet(db.QueryRowContext(ctx, `SELECT id, name, venue, homologation_ref,
		start_date, end_date, organizer, tier, status, category_scheme, template_id, scoring_table,
		results_positioning, official_source_name, official_source_url, entry_fee_cents, relay_fee_cents, version
		FROM meets WHERE id = ?`, id))
}

// ListMeets returns all meets, newest first.
func ListMeets(ctx context.Context, db DBTX) ([]MeetRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, name, venue, homologation_ref,
		start_date, end_date, organizer, tier, status, category_scheme, template_id, scoring_table,
		results_positioning, official_source_name, official_source_url, entry_fee_cents, relay_fee_cents, version
		FROM meets ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MeetRecord
	for rows.Next() {
		m, err := scanMeetRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// UpdateMeet rewrites the organizer-editable meet attributes (SYS-001:
// "create, edit, and archive"; SYS-076 positioning) under optimistic
// concurrency.
func UpdateMeet(ctx context.Context, db DBTX, id string, expectedVersion int64, m domain.Meet) (int64, error) {
	m.ID = id
	if m.Status == "" {
		m.Status = domain.MeetDraft // Validate needs a status; status itself is not updated here
	}
	if m.ResultsPositioning == "" {
		m.ResultsPositioning = domain.ResultsPositioningFederationOfficial
	}
	if err := m.Validate(); err != nil {
		return 0, err
	}
	return OptimisticUpdate(ctx, db, "meets", id, expectedVersion,
		Set{Column: "name", Value: m.Name},
		Set{Column: "venue", Value: m.Venue},
		Set{Column: "homologation_ref", Value: m.HomologationRef},
		Set{Column: "start_date", Value: m.StartDate.Format(dayFormat)},
		Set{Column: "end_date", Value: m.EndDate.Format(dayFormat)},
		Set{Column: "tier", Value: string(m.Tier)},
		Set{Column: "results_positioning", Value: string(m.ResultsPositioning)},
		Set{Column: "official_source_name", Value: m.OfficialSourceName},
		Set{Column: "official_source_url", Value: m.OfficialSourceURL})
}

// SetMeetStatus moves a meet through its lifecycle (draft → … → archived).
func SetMeetStatus(ctx context.Context, db DBTX, id string, expectedVersion int64, status domain.MeetStatus) (int64, error) {
	return OptimisticUpdate(ctx, db, "meets", id, expectedVersion,
		Set{Column: "status", Value: string(status)})
}

// UpdateMeetFeeSchedule sets the SYS-017 configurable fee schedule (a flat
// per-individual-entry fee and a flat per-relay-team-entry fee, in
// Rappen/cents) under optimistic concurrency.
func UpdateMeetFeeSchedule(ctx context.Context, db DBTX, id string, expectedVersion int64, entryFeeCents, relayFeeCents int64) (int64, error) {
	return OptimisticUpdate(ctx, db, "meets", id, expectedVersion,
		Set{Column: "entry_fee_cents", Value: entryFeeCents},
		Set{Column: "relay_fee_cents", Value: relayFeeCents})
}

func scanMeet(row *sql.Row) (MeetRecord, error) {
	var m MeetRecord
	var start, end, tier, status, positioning string
	err := row.Scan(&m.ID, &m.Name, &m.Venue, &m.HomologationRef,
		&start, &end, &m.Organizer, &tier, &status, &m.CategorySchemeID,
		&m.TemplateID, &m.ScoringTableID, &positioning, &m.OfficialSourceName, &m.OfficialSourceURL,
		&m.EntryFeeCents, &m.RelayFeeCents, &m.Version)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return MeetRecord{}, ErrNotFound
	case err != nil:
		return MeetRecord{}, err
	}
	return decodeMeet(m, start, end, tier, status, positioning)
}

func scanMeetRow(rows *sql.Rows) (MeetRecord, error) {
	var m MeetRecord
	var start, end, tier, status, positioning string
	if err := rows.Scan(&m.ID, &m.Name, &m.Venue, &m.HomologationRef,
		&start, &end, &m.Organizer, &tier, &status, &m.CategorySchemeID,
		&m.TemplateID, &m.ScoringTableID, &positioning, &m.OfficialSourceName, &m.OfficialSourceURL,
		&m.EntryFeeCents, &m.RelayFeeCents, &m.Version); err != nil {
		return MeetRecord{}, err
	}
	return decodeMeet(m, start, end, tier, status, positioning)
}

func decodeMeet(m MeetRecord, start, end, tier, status, positioning string) (MeetRecord, error) {
	var err error
	if m.StartDate, err = time.Parse(dayFormat, start); err != nil {
		return MeetRecord{}, fmt.Errorf("meet %s: bad start date %q: %w", m.ID, start, err)
	}
	if m.EndDate, err = time.Parse(dayFormat, end); err != nil {
		return MeetRecord{}, fmt.Errorf("meet %s: bad end date %q: %w", m.ID, end, err)
	}
	m.Tier = domain.MeetTier(tier)
	m.Status = domain.MeetStatus(status)
	m.ResultsPositioning = domain.ResultsPositioning(positioning)
	return m, nil
}

// CreateSession inserts one competition session (SYS-001: sessions per day).
func CreateSession(ctx context.Context, db DBTX, s domain.Session) (domain.Session, error) {
	s.ID = NewID()
	if err := s.Validate(); err != nil {
		return domain.Session{}, err
	}
	_, err := db.ExecContext(ctx, `INSERT INTO meet_sessions (id, meet_id, day, label)
		VALUES (?, ?, ?, ?)`, s.ID, s.MeetID, s.Day.Format(dayFormat), s.Label)
	if err != nil {
		return domain.Session{}, fmt.Errorf("create session for meet %s: %w", s.MeetID, err)
	}
	return s, nil
}

// ListSessions returns a meet's sessions in day order.
func ListSessions(ctx context.Context, db DBTX, meetID string) ([]domain.Session, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, meet_id, day, label
		FROM meet_sessions WHERE meet_id = ? ORDER BY day, label, id`, meetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Session
	for rows.Next() {
		var s domain.Session
		var day string
		if err := rows.Scan(&s.ID, &s.MeetID, &day, &s.Label); err != nil {
			return nil, err
		}
		if s.Day, err = time.Parse(dayFormat, day); err != nil {
			return nil, fmt.Errorf("session %s: bad day %q: %w", s.ID, day, err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// DeleteSessions removes all of a meet's sessions (used when an edit
// replaces the session plan wholesale).
func DeleteSessions(ctx context.Context, db DBTX, meetID string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM meet_sessions WHERE meet_id = ?`, meetID)
	return err
}

// CreateEvent inserts one programme event (SYS-002: discipline × category).
func CreateEvent(ctx context.Context, db DBTX, e domain.Event) (EventRecord, error) {
	e.ID = NewID()
	if e.Status == "" {
		e.Status = domain.EventDraft
	}
	if e.MeetID == "" || e.DisciplineCode == "" || len(e.CategoryCodes) == 0 {
		return EventRecord{}, errors.New("event: meet id, discipline code and at least one category are required (SYS-002)")
	}
	cats, err := json.Marshal(e.CategoryCodes)
	if err != nil {
		return EventRecord{}, fmt.Errorf("encode category codes: %w", err)
	}
	var deadline any
	if e.EntryDeadline != nil {
		deadline = e.EntryDeadline.UTC().Format(instantFormat)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO events
		(id, meet_id, discipline_code, category_codes, entry_standard, entry_deadline, status, entry_limit)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.MeetID, e.DisciplineCode, string(cats), e.EntryStandard, deadline, string(e.Status), e.EntryLimit)
	if err != nil {
		return EventRecord{}, fmt.Errorf("create event %s for meet %s: %w", e.DisciplineCode, e.MeetID, err)
	}
	return EventRecord{Event: e, Version: 1}, nil
}

const eventColumns = `id, meet_id, discipline_code, category_codes,
	entry_standard, entry_deadline, status, entry_limit, version`

// GetEvent looks up one programme event by ID (TASK-016 entry submission:
// resolving the event an entry targets).
func GetEvent(ctx context.Context, db DBTX, id string) (EventRecord, error) {
	return scanEvent(db.QueryRowContext(ctx, `SELECT `+eventColumns+` FROM events WHERE id = ?`, id))
}

// ListEvents returns a meet's programme in creation order.
func ListEvents(ctx context.Context, db DBTX, meetID string) ([]EventRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+eventColumns+`
		FROM events WHERE meet_id = ? ORDER BY id`, meetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EventRecord
	for rows.Next() {
		e, err := scanEventRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanEvent(row *sql.Row) (EventRecord, error) {
	var e EventRecord
	var cats, status string
	var deadline sql.NullString
	err := row.Scan(&e.ID, &e.MeetID, &e.DisciplineCode, &cats,
		&e.EntryStandard, &deadline, &status, &e.EntryLimit, &e.Version)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return EventRecord{}, ErrNotFound
	case err != nil:
		return EventRecord{}, err
	}
	return decodeEvent(e, cats, status, deadline)
}

func scanEventRow(rows *sql.Rows) (EventRecord, error) {
	var e EventRecord
	var cats, status string
	var deadline sql.NullString
	if err := rows.Scan(&e.ID, &e.MeetID, &e.DisciplineCode, &cats,
		&e.EntryStandard, &deadline, &status, &e.EntryLimit, &e.Version); err != nil {
		return EventRecord{}, err
	}
	return decodeEvent(e, cats, status, deadline)
}

func decodeEvent(e EventRecord, cats, status string, deadline sql.NullString) (EventRecord, error) {
	if err := json.Unmarshal([]byte(cats), &e.CategoryCodes); err != nil {
		return EventRecord{}, fmt.Errorf("event %s: bad category codes %q: %w", e.ID, cats, err)
	}
	if deadline.Valid {
		t, err := time.Parse(instantFormat, deadline.String)
		if err != nil {
			return EventRecord{}, fmt.Errorf("event %s: bad entry deadline %q: %w", e.ID, deadline.String, err)
		}
		e.EntryDeadline = &t
	}
	e.Status = domain.EventStatus(status)
	return e, nil
}

// CreateRound inserts one round of an event's progression, at position seq.
func CreateRound(ctx context.Context, db DBTX, r domain.Round, seq int) (domain.Round, error) {
	r.ID = NewID()
	if r.EventID == "" || r.Kind == "" {
		return domain.Round{}, errors.New("round: event id and kind are required")
	}
	_, err := db.ExecContext(ctx, `INSERT INTO rounds (id, event_id, kind, seq)
		VALUES (?, ?, ?, ?)`, r.ID, r.EventID, string(r.Kind), seq)
	if err != nil {
		return domain.Round{}, fmt.Errorf("create round for event %s: %w", r.EventID, err)
	}
	return r, nil
}

// ListRounds returns an event's rounds in progression order.
func ListRounds(ctx context.Context, db DBTX, eventID string) ([]domain.Round, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, event_id, kind
		FROM rounds WHERE event_id = ? ORDER BY seq`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Round
	for rows.Next() {
		var r domain.Round
		var kind string
		if err := rows.Scan(&r.ID, &r.EventID, &kind); err != nil {
			return nil, err
		}
		r.Kind = domain.RoundKind(kind)
		out = append(out, r)
	}
	return out, rows.Err()
}

// CreateUnit inserts one schedulable unit of a round. A zero ScheduledAt is
// stored as NULL (not yet scheduled).
func CreateUnit(ctx context.Context, db DBTX, u domain.Unit) (UnitRecord, error) {
	u.ID = NewID()
	if u.RoundID == "" {
		return UnitRecord{}, errors.New("unit: round id is required")
	}
	var sched any
	if !u.ScheduledAt.IsZero() {
		sched = u.ScheduledAt.UTC().Format(instantFormat)
	}
	_, err := db.ExecContext(ctx, `INSERT INTO units (id, round_id, scheduled_at, location)
		VALUES (?, ?, ?, ?)`, u.ID, u.RoundID, sched, u.Location)
	if err != nil {
		return UnitRecord{}, fmt.Errorf("create unit for round %s: %w", u.RoundID, err)
	}
	return UnitRecord{Unit: u, Version: 1}, nil
}

// UpdateUnitSchedule sets a unit's scheduled time and location under
// optimistic concurrency (the timetable-amendment write path, SYS-004).
func UpdateUnitSchedule(ctx context.Context, db DBTX, id string, expectedVersion int64, scheduledAt time.Time, location string) (int64, error) {
	if scheduledAt.IsZero() {
		return 0, errors.New("unit: scheduled time is required")
	}
	return OptimisticUpdate(ctx, db, "units", id, expectedVersion,
		Set{Column: "scheduled_at", Value: scheduledAt.UTC().Format(instantFormat)},
		Set{Column: "location", Value: location})
}

// TimetableEntry is one unit's line in a meet timetable: the unit joined to
// its round and event so a snapshot is self-describing (SYS-004 — a retained
// historical version must stay readable even after the programme changes).
type TimetableEntry struct {
	UnitID         string     `json:"unitId"`
	EventID        string     `json:"eventId"`
	DisciplineCode string     `json:"disciplineCode"`
	CategoryCodes  []string   `json:"categoryCodes"`
	RoundKind      string     `json:"roundKind"`
	ScheduledAt    *time.Time `json:"scheduledAt"`
	Location       string     `json:"location,omitempty"`
	UnitVersion    int64      `json:"-"` // live-view concern, not part of snapshots
}

// ListMeetUnits returns the live (unsnapshotted) timetable entries for a
// meet: every unit joined through rounds and events, scheduled first in
// time order, unscheduled last.
func ListMeetUnits(ctx context.Context, db DBTX, meetID string) ([]TimetableEntry, error) {
	rows, err := db.QueryContext(ctx, `SELECT u.id, e.id, e.discipline_code,
		e.category_codes, r.kind, u.scheduled_at, u.location, u.version
		FROM units u
		JOIN rounds r ON r.id = u.round_id
		JOIN events e ON e.id = r.event_id
		WHERE e.meet_id = ?
		ORDER BY u.scheduled_at IS NULL, u.scheduled_at, e.id, r.seq`, meetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TimetableEntry
	for rows.Next() {
		var t TimetableEntry
		var cats string
		var sched sql.NullString
		if err := rows.Scan(&t.UnitID, &t.EventID, &t.DisciplineCode, &cats,
			&t.RoundKind, &sched, &t.Location, &t.UnitVersion); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(cats), &t.CategoryCodes); err != nil {
			return nil, fmt.Errorf("unit %s: bad category codes %q: %w", t.UnitID, cats, err)
		}
		if sched.Valid {
			at, err := time.Parse(instantFormat, sched.String)
			if err != nil {
				return nil, fmt.Errorf("unit %s: bad scheduled time %q: %w", t.UnitID, sched.String, err)
			}
			t.ScheduledAt = &at
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TimetableVersion is one published timetable state: an immutable,
// timestamped snapshot (SYS-004, UC-001 #4). The highest version is the
// meet's current public timetable.
type TimetableVersion struct {
	ID          string
	MeetID      string
	Version     int
	PublishedAt time.Time
	Entries     []TimetableEntry
}

// AppendTimetableVersion publishes entries as the meet's next timetable
// version. Prior versions are never modified — amendments append.
func AppendTimetableVersion(ctx context.Context, db DBTX, meetID string, entries []TimetableEntry) (TimetableVersion, error) {
	body, err := json.Marshal(entries)
	if err != nil {
		return TimetableVersion{}, fmt.Errorf("encode timetable entries: %w", err)
	}
	v := TimetableVersion{ID: NewID(), MeetID: meetID, Entries: entries}
	// max+1 without a race: the store is single-writer by construction
	// (ADR-004 §2, SetMaxOpenConns(1)), and (meet_id, version) is UNIQUE as
	// a backstop.
	err = db.QueryRowContext(ctx, `SELECT coalesce(max(version), 0) + 1
		FROM timetable_versions WHERE meet_id = ?`, meetID).Scan(&v.Version)
	if err != nil {
		return TimetableVersion{}, err
	}
	err = db.QueryRowContext(ctx, `INSERT INTO timetable_versions
		(id, meet_id, version, entries_json) VALUES (?, ?, ?, ?)
		RETURNING published_at`, v.ID, meetID, v.Version, string(body)).
		Scan(&timetableTimeScanner{&v.PublishedAt})
	if err != nil {
		return TimetableVersion{}, fmt.Errorf("publish timetable v%d for meet %s: %w", v.Version, meetID, err)
	}
	return v, nil
}

// timetableTimeScanner parses the TEXT timestamp column into a time.Time.
type timetableTimeScanner struct{ t *time.Time }

func (s *timetableTimeScanner) Scan(src any) error {
	raw, ok := src.(string)
	if !ok {
		return fmt.Errorf("timestamp column: unexpected type %T", src)
	}
	t, err := time.Parse(instantFormat, raw)
	if err != nil {
		return err
	}
	*s.t = t
	return nil
}

// LatestTimetable returns the meet's current published timetable, or
// ErrNotFound if nothing has been published yet.
func LatestTimetable(ctx context.Context, db DBTX, meetID string) (TimetableVersion, error) {
	vs, err := listTimetableVersions(ctx, db, meetID, true)
	if err != nil {
		return TimetableVersion{}, err
	}
	if len(vs) == 0 {
		return TimetableVersion{}, ErrNotFound
	}
	return vs[0], nil
}

// ListTimetableVersions returns every published version, newest first, each
// with its publication timestamp (SYS-004: amendments retained).
func ListTimetableVersions(ctx context.Context, db DBTX, meetID string) ([]TimetableVersion, error) {
	return listTimetableVersions(ctx, db, meetID, false)
}

func listTimetableVersions(ctx context.Context, db DBTX, meetID string, latestOnly bool) ([]TimetableVersion, error) {
	q := `SELECT id, meet_id, version, published_at, entries_json
		FROM timetable_versions WHERE meet_id = ? ORDER BY version DESC`
	if latestOnly {
		q += ` LIMIT 1`
	}
	rows, err := db.QueryContext(ctx, q, meetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TimetableVersion
	for rows.Next() {
		var v TimetableVersion
		var body string
		if err := rows.Scan(&v.ID, &v.MeetID, &v.Version,
			&timetableTimeScanner{&v.PublishedAt}, &body); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(body), &v.Entries); err != nil {
			return nil, fmt.Errorf("timetable %s v%d: bad entries: %w", v.MeetID, v.Version, err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
