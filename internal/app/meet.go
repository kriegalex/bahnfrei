// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// Aliases so the web layer can name these result types without importing
// internal/store (architecture.md §3 / depguard "web-goes-through-app").
type (
	MeetRecord       = store.MeetRecord
	EventRecord      = store.EventRecord
	TimetableEntry   = store.TimetableEntry
	TimetableVersion = store.TimetableVersion
)

// ErrMeetNotFound is the app-level "no such entity" for meet-setup reads,
// aliasing the store sentinel so web handlers can match it.
var ErrMeetNotFound = store.ErrNotFound

// ErrConflict surfaces an optimistic-concurrency loss (SYS-083: the caller
// must be told, never silently overwritten).
var ErrConflict = store.ErrVersionConflict

// MeetService hosts the UC-001 meet-setup use-cases: create/edit/archive
// meets with sessions (SYS-001), compose the event programme (SYS-002),
// maintain the published timetable (SYS-004) and produce the sanctioning
// summary (SYS-006). Every mutation is authorized (CapOrganizeMeet) and
// audited.
type MeetService struct {
	db      *sql.DB
	catalog *domain.DisciplineCatalog
	schemes map[string]*domain.CategoryScheme
}

// NewMeetService wires a MeetService. catalog and schemes are normally the
// built-in data (domain.BuiltinDisciplineCatalog / BuiltinCategorySchemes).
func NewMeetService(db *sql.DB, catalog *domain.DisciplineCatalog, schemes map[string]*domain.CategoryScheme) *MeetService {
	return &MeetService{db: db, catalog: catalog, schemes: schemes}
}

// CategorySchemes lists the selectable scheme IDs, sorted (for the meet
// creation form).
func (s *MeetService) CategorySchemes() []string {
	ids := make([]string, 0, len(s.schemes))
	for id := range s.schemes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Scheme returns a loaded category scheme by ID.
func (s *MeetService) Scheme(id string) (*domain.CategoryScheme, bool) {
	sch, ok := s.schemes[id]
	return sch, ok
}

// Catalog returns the discipline catalog this instance runs with.
func (s *MeetService) Catalog() *domain.DisciplineCatalog { return s.catalog }

// SessionPlan is one requested session line in a meet create/edit.
type SessionPlan struct {
	Day   time.Time
	Label string
}

// MeetRequest carries the organizer-editable meet attributes (SYS-001).
type MeetRequest struct {
	Name             string
	Venue            string
	HomologationRef  string
	StartDate        time.Time
	EndDate          time.Time
	Tier             string
	CategorySchemeID string
	Sessions         []SessionPlan
}

func (s *MeetService) validateMeetRequest(req MeetRequest) error {
	if _, ok := s.schemes[req.CategorySchemeID]; !ok {
		return fmt.Errorf("unknown category scheme %q", req.CategorySchemeID)
	}
	for _, sp := range req.Sessions {
		if sp.Day.Before(req.StartDate) || sp.Day.After(req.EndDate) {
			return fmt.Errorf("session day %s is outside the meet dates", sp.Day.Format("2006-01-02"))
		}
	}
	return nil
}

// CreateMeet creates a meet in status draft with its sessions (UC-001 #2).
// The acting account is recorded as the organizer identity (SYS-001).
func (s *MeetService) CreateMeet(ctx context.Context, actor Session, req MeetRequest) (MeetRecord, error) {
	if err := Authorize(actor.Role, CapOrganizeMeet); err != nil {
		return MeetRecord{}, err
	}
	if err := s.validateMeetRequest(req); err != nil {
		return MeetRecord{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MeetRecord{}, fmt.Errorf("create meet: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rec, err := store.CreateMeet(ctx, tx, domain.Meet{
		Name:             req.Name,
		Venue:            req.Venue,
		HomologationRef:  req.HomologationRef,
		StartDate:        req.StartDate,
		EndDate:          req.EndDate,
		Organizer:        actor.Username,
		Tier:             domain.MeetTier(req.Tier),
		CategorySchemeID: req.CategorySchemeID,
	})
	if err != nil {
		return MeetRecord{}, err
	}
	for _, sp := range req.Sessions {
		if _, err := store.CreateSession(ctx, tx, domain.Session{
			MeetID: rec.ID, Day: sp.Day, Label: sp.Label,
		}); err != nil {
			return MeetRecord{}, err
		}
	}

	after, _ := json.Marshal(rec.Meet)
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "meet.create",
		EntityType: "meet", EntityID: rec.ID, After: string(after),
	}); err != nil {
		return MeetRecord{}, fmt.Errorf("audit meet create: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return MeetRecord{}, fmt.Errorf("create meet: %w", err)
	}
	return rec, nil
}

// UpdateMeet edits a meet's attributes and replaces its session plan under
// optimistic concurrency (SYS-001 "edit"; SYS-083 conflict semantics).
func (s *MeetService) UpdateMeet(ctx context.Context, actor Session, meetID string, expectedVersion int64, req MeetRequest) error {
	if err := Authorize(actor.Role, CapOrganizeMeet); err != nil {
		return err
	}
	if err := s.validateMeetRequest(req); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("update meet: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	before, err := store.GetMeet(ctx, tx, meetID)
	if err != nil {
		return err
	}
	if _, err := store.UpdateMeet(ctx, tx, meetID, expectedVersion, domain.Meet{
		Name:            req.Name,
		Venue:           req.Venue,
		HomologationRef: req.HomologationRef,
		StartDate:       req.StartDate,
		EndDate:         req.EndDate,
		Tier:            domain.MeetTier(req.Tier),
	}); err != nil {
		return err
	}
	if err := store.DeleteSessions(ctx, tx, meetID); err != nil {
		return fmt.Errorf("update meet sessions: %w", err)
	}
	for _, sp := range req.Sessions {
		if _, err := store.CreateSession(ctx, tx, domain.Session{
			MeetID: meetID, Day: sp.Day, Label: sp.Label,
		}); err != nil {
			return err
		}
	}

	beforeJSON, _ := json.Marshal(before.Meet)
	afterRec, err := store.GetMeet(ctx, tx, meetID)
	if err != nil {
		return err
	}
	afterJSON, _ := json.Marshal(afterRec.Meet)
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "meet.update",
		EntityType: "meet", EntityID: meetID,
		Before: string(beforeJSON), After: string(afterJSON),
	}); err != nil {
		return fmt.Errorf("audit meet update: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("update meet: %w", err)
	}
	return nil
}

// ArchiveMeet moves a meet to status archived (SYS-001 "archive").
func (s *MeetService) ArchiveMeet(ctx context.Context, actor Session, meetID string, expectedVersion int64) error {
	if err := Authorize(actor.Role, CapOrganizeMeet); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("archive meet: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := store.SetMeetStatus(ctx, tx, meetID, expectedVersion, domain.MeetArchived); err != nil {
		return err
	}
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "meet.archive",
		EntityType: "meet", EntityID: meetID,
		After: `{"status":"archived"}`,
	}); err != nil {
		return fmt.Errorf("audit meet archive: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("archive meet: %w", err)
	}
	return nil
}

// ListMeets returns all meets, newest first (operator overview).
func (s *MeetService) ListMeets(ctx context.Context) ([]MeetRecord, error) {
	return store.ListMeets(ctx, s.db)
}

// ProgrammeEvent is one programme line enriched with the discipline's
// catalog data — its family is the capture type UC-001 #3 requires the
// programme to show (track / horizontal field / relay / …).
type ProgrammeEvent struct {
	EventRecord
	DisciplineName string
	Family         domain.DisciplineFamily
	Rounds         []domain.Round
}

// MeetDetail aggregates everything the meet workspace shows: the meet, its
// sessions (SYS-001), programme (SYS-002) and live (unpublished) timetable
// entries (SYS-004).
type MeetDetail struct {
	MeetRecord
	Sessions  []domain.Session
	Programme []ProgrammeEvent
	Units     []TimetableEntry
}

// Meet loads a meet with its sessions, programme and units.
func (s *MeetService) Meet(ctx context.Context, meetID string) (MeetDetail, error) {
	rec, err := store.GetMeet(ctx, s.db, meetID)
	if err != nil {
		return MeetDetail{}, err
	}
	d := MeetDetail{MeetRecord: rec}
	if d.Sessions, err = store.ListSessions(ctx, s.db, meetID); err != nil {
		return MeetDetail{}, err
	}
	events, err := store.ListEvents(ctx, s.db, meetID)
	if err != nil {
		return MeetDetail{}, err
	}
	for _, ev := range events {
		pe := ProgrammeEvent{EventRecord: ev}
		if disc, ok := s.catalog.ByCode(ev.DisciplineCode); ok {
			pe.DisciplineName = disc.Name
			pe.Family = disc.Family
		}
		if pe.Rounds, err = store.ListRounds(ctx, s.db, ev.ID); err != nil {
			return MeetDetail{}, err
		}
		d.Programme = append(d.Programme, pe)
	}
	if d.Units, err = store.ListMeetUnits(ctx, s.db, meetID); err != nil {
		return MeetDetail{}, err
	}
	return d, nil
}

// AddEventRequest describes one programme event to add (SYS-002).
type AddEventRequest struct {
	DisciplineCode string
	CategoryCodes  []string
	Rounds         []domain.RoundKind // empty means a single final
	EntryStandard  string
	EntryDeadline  *time.Time
}

// AddEvent appends an event (discipline × category) with its round
// structure to a meet's programme (UC-001 #3). The discipline must exist in
// the catalog and every category in the meet's scheme; each round gets one
// schedulable unit (heat splitting is seeding's job, TASK-018).
func (s *MeetService) AddEvent(ctx context.Context, actor Session, meetID string, req AddEventRequest) (EventRecord, error) {
	if err := Authorize(actor.Role, CapOrganizeMeet); err != nil {
		return EventRecord{}, err
	}
	meet, err := store.GetMeet(ctx, s.db, meetID)
	if err != nil {
		return EventRecord{}, err
	}
	if _, ok := s.catalog.ByCode(req.DisciplineCode); !ok {
		return EventRecord{}, fmt.Errorf("unknown discipline %q", req.DisciplineCode)
	}
	scheme, ok := s.schemes[meet.CategorySchemeID]
	if !ok {
		return EventRecord{}, fmt.Errorf("meet %s references unknown category scheme %q", meetID, meet.CategorySchemeID)
	}
	if len(req.CategoryCodes) == 0 {
		return EventRecord{}, errors.New("at least one category is required (SYS-002)")
	}
	for _, code := range req.CategoryCodes {
		if _, ok := scheme.CategoryByCode(code); !ok {
			return EventRecord{}, fmt.Errorf("category %q is not defined by scheme %q", code, meet.CategorySchemeID)
		}
	}
	rounds := req.Rounds
	if len(rounds) == 0 {
		rounds = []domain.RoundKind{domain.RoundFinal}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return EventRecord{}, fmt.Errorf("add event: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rec, err := store.CreateEvent(ctx, tx, domain.Event{
		MeetID:         meetID,
		DisciplineCode: req.DisciplineCode,
		CategoryCodes:  req.CategoryCodes,
		EntryStandard:  req.EntryStandard,
		EntryDeadline:  req.EntryDeadline,
	})
	if err != nil {
		return EventRecord{}, err
	}
	for i, kind := range rounds {
		round, err := store.CreateRound(ctx, tx, domain.Round{EventID: rec.ID, Kind: kind}, i)
		if err != nil {
			return EventRecord{}, err
		}
		if _, err := store.CreateUnit(ctx, tx, domain.Unit{RoundID: round.ID}); err != nil {
			return EventRecord{}, err
		}
	}

	after, _ := json.Marshal(rec.Event)
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "event.create",
		EntityType: "event", EntityID: rec.ID, After: string(after),
	}); err != nil {
		return EventRecord{}, fmt.Errorf("audit event create: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return EventRecord{}, fmt.Errorf("add event: %w", err)
	}
	return rec, nil
}

// ScheduleUnit sets one unit's scheduled time and venue location (the
// timetable's draft state; publication is a separate action, SYS-004).
func (s *MeetService) ScheduleUnit(ctx context.Context, actor Session, unitID string, expectedVersion int64, at time.Time, location string) error {
	if err := Authorize(actor.Role, CapOrganizeMeet); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("schedule unit: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := store.UpdateUnitSchedule(ctx, tx, unitID, expectedVersion, at, location); err != nil {
		return err
	}
	after, _ := json.Marshal(map[string]string{
		"scheduledAt": at.UTC().Format(time.RFC3339), "location": location,
	})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "unit.schedule",
		EntityType: "unit", EntityID: unitID, After: string(after),
	}); err != nil {
		return fmt.Errorf("audit unit schedule: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("schedule unit: %w", err)
	}
	return nil
}

// PublishTimetable snapshots the meet's current unit schedule as the next
// published timetable version (SYS-004: every published amendment is
// timestamped and retained — republishing appends, never rewrites).
func (s *MeetService) PublishTimetable(ctx context.Context, actor Session, meetID string) (TimetableVersion, error) {
	if err := Authorize(actor.Role, CapOrganizeMeet); err != nil {
		return TimetableVersion{}, err
	}
	if _, err := store.GetMeet(ctx, s.db, meetID); err != nil {
		return TimetableVersion{}, err
	}
	entries, err := store.ListMeetUnits(ctx, s.db, meetID)
	if err != nil {
		return TimetableVersion{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TimetableVersion{}, fmt.Errorf("publish timetable: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	v, err := store.AppendTimetableVersion(ctx, tx, meetID, entries)
	if err != nil {
		return TimetableVersion{}, err
	}
	after, _ := json.Marshal(map[string]any{"version": v.Version, "units": len(entries)})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "timetable.publish",
		EntityType: "meet", EntityID: meetID, After: string(after),
	}); err != nil {
		return TimetableVersion{}, fmt.Errorf("audit timetable publish: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return TimetableVersion{}, fmt.Errorf("publish timetable: %w", err)
	}
	return v, nil
}

// PublicTimetable returns a meet's current published timetable — the public
// read (SYS-090 unauthenticated public read; delivery per SYS-071).
// ErrMeetNotFound doubles as "nothing published yet".
func (s *MeetService) PublicTimetable(ctx context.Context, meetID string) (MeetRecord, TimetableVersion, error) {
	rec, err := store.GetMeet(ctx, s.db, meetID)
	if err != nil {
		return MeetRecord{}, TimetableVersion{}, err
	}
	v, err := store.LatestTimetable(ctx, s.db, meetID)
	if err != nil {
		return MeetRecord{}, TimetableVersion{}, err
	}
	return rec, v, nil
}

// TimetableVersions returns every published version, newest first
// (SYS-004: retained amendments with timestamps).
func (s *MeetService) TimetableVersions(ctx context.Context, meetID string) ([]TimetableVersion, error) {
	return store.ListTimetableVersions(ctx, s.db, meetID)
}

// SanctioningSummary is the human-readable meet registration summary
// SYS-006 requires: tier, venue with homologation reference, dates,
// organizer, and the categories and disciplines on the programme.
type SanctioningSummary struct {
	Meet        MeetRecord
	Sessions    []domain.Session
	Categories  []string
	Disciplines []string // display names from the catalog
	GeneratedAt time.Time
}

// Complete reports whether every SYS-006-required field is present — the
// UC-001 #5 completeness check ("suitable for federation registration").
func (s SanctioningSummary) Complete() (bool, []string) {
	var missing []string
	if s.Meet.Name == "" {
		missing = append(missing, "name")
	}
	if s.Meet.Venue == "" {
		missing = append(missing, "venue")
	}
	if s.Meet.HomologationRef == "" {
		missing = append(missing, "homologation reference")
	}
	if s.Meet.Organizer == "" {
		missing = append(missing, "organizer")
	}
	if s.Meet.Tier == "" {
		missing = append(missing, "tier")
	}
	if len(s.Categories) == 0 {
		missing = append(missing, "categories")
	}
	if len(s.Disciplines) == 0 {
		missing = append(missing, "disciplines")
	}
	return len(missing) == 0, missing
}

// SanctioningSummary assembles the summary for one meet (UC-001 #5).
func (s *MeetService) SanctioningSummary(ctx context.Context, meetID string) (SanctioningSummary, error) {
	rec, err := store.GetMeet(ctx, s.db, meetID)
	if err != nil {
		return SanctioningSummary{}, err
	}
	sum := SanctioningSummary{Meet: rec, GeneratedAt: time.Now().UTC()}
	if sum.Sessions, err = store.ListSessions(ctx, s.db, meetID); err != nil {
		return SanctioningSummary{}, err
	}
	events, err := store.ListEvents(ctx, s.db, meetID)
	if err != nil {
		return SanctioningSummary{}, err
	}
	catSet := map[string]bool{}
	discSet := map[string]bool{}
	for _, ev := range events {
		for _, c := range ev.CategoryCodes {
			catSet[c] = true
		}
		name := ev.DisciplineCode
		if disc, ok := s.catalog.ByCode(ev.DisciplineCode); ok {
			name = disc.Name
		}
		discSet[name] = true
	}
	for c := range catSet {
		sum.Categories = append(sum.Categories, c)
	}
	for d := range discSet {
		sum.Disciplines = append(sum.Disciplines, d)
	}
	sort.Strings(sum.Categories)
	sort.Strings(sum.Disciplines)
	return sum, nil
}
