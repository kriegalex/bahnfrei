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
	"strconv"
	"strings"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// --- Online entries (TASK-016, UC-003/UC-006, SYS-011/012/015/017/018):
// individual, club-bulk and relay entry submission against the event
// programme, bib assignment over the existing meet-wide participants table
// (TASK-007), and the per-club fee summary. Methods live on *ResultsService
// (mirrors assignment.go's precedent) rather than growing a new service, so
// existing wiring (Server, apptest.Fixture) needs no change. ---

// Sentinel errors for entry submission (SYS-011/012/015).
var (
	// ErrEntriesClosed means the meet is not in a status that accepts
	// entries (published/live), or the event itself is closed.
	ErrEntriesClosed = errors.New("entries are not open for this meet or event")
	// ErrEntryDeadlinePassed means the event's configured entry deadline has
	// passed (SYS-011: "Deadline enforcement SHALL be server-side" —
	// UC-003 #3, including "direct request forgery").
	ErrEntryDeadlinePassed = errors.New("entry deadline has passed")
	// ErrEntryLimitReached means the event's configured entry limit
	// (SYS-015) has been reached.
	ErrEntryLimitReached = errors.New("event entry limit reached")
	// ErrSeedPerformanceRequired means an entry was submitted without a seed
	// performance (SYS-015: "required seed-performance information").
	ErrSeedPerformanceRequired = errors.New("seed performance is required")
	// ErrDuplicateEntry aliases the store sentinel for web handlers.
	ErrDuplicateEntry = store.ErrDuplicateEntry
	// ErrBibRequired means a bib assignment was attempted with an empty bib.
	ErrBibRequired = errors.New("a bib value is required")
	// ErrNotRelayEntry means a relay-composition edit targeted an entry that
	// is not a relay entry.
	ErrNotRelayEntry = errors.New("entry is not a relay entry")
)

// EntryDetail is one entry enriched with its athlete/relay-team, event and
// club data for display (UC-003 #1/#2, UC-006 #3).
type EntryDetail struct {
	store.EntryRecord
	Event       store.EventRecord
	AthleteName string // individual entries only
	ClubName    string
	RelayTeam   *RelayTeamDetail // relay entries only
	// FeeCents is this entry's configured fee (the meet's per-individual or
	// per-relay fee, SYS-017) at read time — not stored on the entry itself,
	// so a later fee-schedule change is reflected immediately.
	FeeCents int64
	// Eligibility is the entry's most recent SYS-014 evaluation (TASK-017,
	// UC-005); a zero value (Outcome "") reads as eligible/no-flags — see
	// EligibilityView.EffectiveOutcome. Relay entries are not evaluated
	// (OQ-032): always zero-value.
	Eligibility EligibilityView
}

// RelayTeamDetail is a relay team enriched with its athletes' display names,
// in leg/reserve order.
type RelayTeamDetail struct {
	store.RelayTeamRecord
	Composition []string
	Reserves    []string
}

// EntryEventOption is one event open for online entry, for the submission
// form's event picker.
type EntryEventOption struct {
	EventID  string
	Label    string
	Deadline *time.Time
	IsRelay  bool
}

// IndividualEntryInput is one athlete's online entry (UC-003 #1).
// PublicationWithdrawn collects the SYS-103 publication-consent choice at
// entry-submission time (TASK-023, UC-023) — false (the default, an
// unchecked form checkbox) means results are publicly listed as usual;
// true suppresses the athlete's identity on public surfaces from the
// start (see internal/domain/privacy.go for the opt-out rationale and
// enforcement).
type IndividualEntryInput struct {
	EventID              string
	FirstName            string
	LastName             string
	BirthYear            int
	Sex                  domain.Sex
	Club                 string
	SeedPerformance      string
	PublicationWithdrawn bool
}

// BulkEntryLine is one line of a club submitter's bulk entry operation
// (UC-003 #2). PublicationWithdrawn is per line — consent is per person
// (SYS-103), never per submission batch.
type BulkEntryLine struct {
	FirstName            string
	LastName             string
	BirthYear            int
	Sex                  domain.Sex
	EventID              string
	SeedPerformance      string
	PublicationWithdrawn bool
}

// BulkEntryInput is a club submitter's bulk entry operation: every line
// enters an athlete (new or reused within the operation) for one event,
// under a single submitting club.
type BulkEntryInput struct {
	Club  string
	Lines []BulkEntryLine
}

// RelayLegInput is one relay team member: a leg athlete or a reserve.
// PublicationWithdrawn is per leg athlete — a relay's public display name
// is the club/team, but each leg member is a natural person whose own
// SYS-103 consent must be captured at creation time (TASK-023, UC-023),
// not defaulted silently.
type RelayLegInput struct {
	FirstName            string
	LastName             string
	BirthYear            int
	Sex                  domain.Sex
	PublicationWithdrawn bool
}

// RelayEntryInput is a club's relay-team entry (UC-003 #4): an ordered leg
// composition plus reserves.
type RelayEntryInput struct {
	EventID     string
	Club        string
	Composition []RelayLegInput
	Reserves    []RelayLegInput
}

// OpenEntryEvents lists the meet's events currently open for online entry —
// meet published/live, event not closed, deadline not passed — for the
// submission form's event picker. Returns an empty list (not an error) for a
// meet that is not yet open for entry.
func (s *ResultsService) OpenEntryEvents(ctx context.Context, meetID string) ([]EntryEventOption, error) {
	meet, err := store.GetMeet(ctx, s.db, meetID)
	if err != nil {
		return nil, err
	}
	if meet.Status != domain.MeetPublished && meet.Status != domain.MeetLive {
		return nil, nil
	}
	events, err := store.ListEvents(ctx, s.db, meetID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var out []EntryEventOption
	for _, e := range events {
		if e.Status == domain.EventClosed {
			continue
		}
		if e.EntryDeadline != nil && !now.Before(*e.EntryDeadline) {
			continue
		}
		disc, _ := s.catalog.ByCode(e.DisciplineCode)
		name := disc.Name
		if name == "" {
			name = e.DisciplineCode
		}
		out = append(out, EntryEventOption{
			EventID:  e.ID,
			Label:    name + " (" + strings.Join(e.CategoryCodes, ", ") + ")",
			Deadline: e.EntryDeadline,
			IsRelay:  disc.Family == domain.FamilyRelay,
		})
	}
	return out, nil
}

// validateEntryEvent resolves eventID within meet and checks it is open for
// entry (SYS-011 "Deadline enforcement SHALL be server-side" — enforced here
// regardless of what the submission form offered, closing the UC-003 #3
// request-forgery gap).
func validateEntryEvent(ctx context.Context, db store.DBTX, meet store.MeetRecord, eventID string, now time.Time) (store.EventRecord, error) {
	event, err := store.GetEvent(ctx, db, eventID)
	if err != nil {
		return store.EventRecord{}, err
	}
	if event.MeetID != meet.ID {
		return store.EventRecord{}, store.ErrNotFound
	}
	if meet.Status != domain.MeetPublished && meet.Status != domain.MeetLive {
		return store.EventRecord{}, ErrEntriesClosed
	}
	if event.Status == domain.EventClosed {
		return store.EventRecord{}, ErrEntriesClosed
	}
	if event.EntryDeadline != nil && !now.Before(*event.EntryDeadline) {
		return store.EventRecord{}, ErrEntryDeadlinePassed
	}
	return event, nil
}

// checkEntryLimit rejects a new entry once event's configured cap (SYS-015)
// is reached; zero means unlimited.
func checkEntryLimit(ctx context.Context, db store.DBTX, event store.EventRecord) error {
	if event.EntryLimit <= 0 {
		return nil
	}
	n, err := store.CountActiveEntriesForEvent(ctx, db, event.ID)
	if err != nil {
		return err
	}
	if n >= event.EntryLimit {
		return ErrEntryLimitReached
	}
	return nil
}

// ensureClub resolves clubName to a club, creating it on first sight
// (mirrors ParticipantInput/RegisterParticipant's precedent).
func ensureClub(ctx context.Context, db store.DBTX, clubName string) (store.ClubRecord, error) {
	club, err := store.GetClubByName(ctx, db, clubName)
	if errors.Is(err, store.ErrNotFound) {
		club, err = store.CreateClub(ctx, db, domain.Club{Name: clubName})
	}
	return club, err
}

// createAthlete validates and persists one athlete's person data (SYS-010),
// affiliated with clubID if given, with their SYS-103 publication-consent
// state as collected by the submitting flow (TASK-023, UC-023 — every
// athlete-creating entry path records consent explicitly, never silently).
func createAthlete(ctx context.Context, db store.DBTX, clubID, firstName, lastName string, birthYear int, sex domain.Sex, consent domain.PublicationConsent) (store.AthleteRecord, error) {
	if strings.TrimSpace(firstName) == "" || strings.TrimSpace(lastName) == "" {
		return store.AthleteRecord{}, errors.New("entry: athlete first and last name are required")
	}
	if birthYear <= 0 {
		return store.AthleteRecord{}, errors.New("entry: athlete birth year is required")
	}
	if sex != domain.SexMale && sex != domain.SexFemale {
		return store.AthleteRecord{}, fmt.Errorf("entry: invalid sex %q", sex)
	}
	var clubIDs []string
	if clubID != "" {
		clubIDs = []string{clubID}
	}
	return store.CreateAthlete(ctx, db, domain.Athlete{
		FirstName: firstName, LastName: lastName, BirthYear: birthYear, Sex: sex, ClubIDs: clubIDs,
		Consent: consent,
	})
}

// entryConsent builds the PublicationConsent an entry flow records for a
// newly created athlete: the submitter's withdrawal choice plus the
// who/when audit context (SYS-103).
func (s *ResultsService) entryConsent(actor Session, withdrawn bool) domain.PublicationConsent {
	return domain.PublicationConsent{
		ResultsPublicationWithdrawn: withdrawn,
		RecordedAt:                  s.now(),
		RecordedBy:                  actor.AccountID,
	}
}

// evaluateStandard checks a seed performance against event's entry standard
// (SYS-015, UC-003 #5); seedPerformance is required whenever a standard is
// configured.
func (s *ResultsService) evaluateStandard(event store.EventRecord, seedPerformance string) (bool, error) {
	if strings.TrimSpace(seedPerformance) == "" {
		return false, ErrSeedPerformanceRequired
	}
	disc, _ := s.catalog.ByCode(event.DisciplineCode)
	return domain.EvaluateEntryStandard(seedPerformance, event.EntryStandard, disc.Family)
}

// SubmitIndividualEntry submits one athlete's online entry (UC-003 #1,
// SYS-011): resolves/creates the athlete and their club, checks the event is
// open (deadline, limit) and evaluates the entry standard (SYS-015), then
// ensures the athlete holds a meet-wide participant slot for later bib
// assignment (SYS-018).
func (s *ResultsService) SubmitIndividualEntry(ctx context.Context, actor Session, meetID string, in IndividualEntryInput) (EntryDetail, error) {
	if err := Authorize(actor.Role, CapSubmitEntries); err != nil {
		return EntryDetail{}, err
	}
	meet, err := store.GetMeet(ctx, s.db, meetID)
	if err != nil {
		return EntryDetail{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return EntryDetail{}, fmt.Errorf("submit entry: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	event, err := validateEntryEvent(ctx, tx, meet, in.EventID, time.Now())
	if err != nil {
		return EntryDetail{}, err
	}
	fails, err := s.evaluateStandard(event, in.SeedPerformance)
	if err != nil {
		return EntryDetail{}, err
	}
	if err := checkEntryLimit(ctx, tx, event); err != nil {
		return EntryDetail{}, err
	}

	var club store.ClubRecord
	if strings.TrimSpace(in.Club) != "" {
		if club, err = ensureClub(ctx, tx, in.Club); err != nil {
			return EntryDetail{}, err
		}
	}
	athlete, err := createAthlete(ctx, tx, club.ID, in.FirstName, in.LastName, in.BirthYear, in.Sex,
		s.entryConsent(actor, in.PublicationWithdrawn))
	if err != nil {
		return EntryDetail{}, err
	}
	if _, err := store.EnsureParticipant(ctx, tx, meetID, athlete.ID); err != nil {
		return EntryDetail{}, err
	}

	rec, err := store.CreateEntry(ctx, tx, domain.Entry{
		EventID: event.ID, AthleteID: athlete.ID, SeedPerformance: in.SeedPerformance,
		Source: domain.EntrySourceOnline, FailsStandard: fails, SubmittedBy: actor.AccountID,
	})
	if err != nil {
		return EntryDetail{}, err
	}
	eligResult, err := s.evaluateAndStoreEligibility(ctx, tx, meet, event, athlete, rec.ID)
	if err != nil {
		return EntryDetail{}, err
	}

	if err := auditEntrySubmit(ctx, tx, actor, rec.ID, athlete.FirstName+" "+athlete.LastName, "entry.submit"); err != nil {
		return EntryDetail{}, err
	}
	if err := tx.Commit(); err != nil {
		return EntryDetail{}, fmt.Errorf("submit entry: %w", err)
	}
	return EntryDetail{
		EntryRecord: rec, Event: event, AthleteName: athlete.FirstName + " " + athlete.LastName,
		ClubName: club.Name, FeeCents: meet.EntryFeeCents,
		Eligibility: EligibilityView{Outcome: eligResult.Outcome, Flags: eligResult.Flags, Version: 1},
	}, nil
}

// SubmitClubBulkEntries submits a club submitter's bulk entry operation
// (UC-003 #2: "enter 15 athletes across 6 events in one bulk operation");
// every line is validated and applied atomically — if any line is invalid,
// none are created, so "all N entries exist" always holds when this
// succeeds.
func (s *ResultsService) SubmitClubBulkEntries(ctx context.Context, actor Session, meetID string, in BulkEntryInput) ([]EntryDetail, error) {
	if err := Authorize(actor.Role, CapSubmitEntries); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Club) == "" {
		return nil, errors.New("entry: club is required for a bulk submission")
	}
	if len(in.Lines) == 0 {
		return nil, errors.New("entry: at least one entry line is required")
	}
	meet, err := store.GetMeet(ctx, s.db, meetID)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("submit bulk entries: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	club, err := ensureClub(ctx, tx, in.Club)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	out := make([]EntryDetail, 0, len(in.Lines))
	for i, line := range in.Lines {
		event, err := validateEntryEvent(ctx, tx, meet, line.EventID, now)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		fails, err := s.evaluateStandard(event, line.SeedPerformance)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		if err := checkEntryLimit(ctx, tx, event); err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		athlete, err := createAthlete(ctx, tx, club.ID, line.FirstName, line.LastName, line.BirthYear, line.Sex,
			s.entryConsent(actor, line.PublicationWithdrawn))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		if _, err := store.EnsureParticipant(ctx, tx, meetID, athlete.ID); err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		rec, err := store.CreateEntry(ctx, tx, domain.Entry{
			EventID: event.ID, AthleteID: athlete.ID, SeedPerformance: line.SeedPerformance,
			Source: domain.EntrySourceOnline, FailsStandard: fails, SubmittedBy: actor.AccountID,
		})
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		eligResult, err := s.evaluateAndStoreEligibility(ctx, tx, meet, event, athlete, rec.ID)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		out = append(out, EntryDetail{
			EntryRecord: rec, Event: event, AthleteName: athlete.FirstName + " " + athlete.LastName,
			ClubName: club.Name, FeeCents: meet.EntryFeeCents,
			Eligibility: EligibilityView{Outcome: eligResult.Outcome, Flags: eligResult.Flags, Version: 1},
		})
	}

	after, _ := json.Marshal(map[string]any{"club": club.Name, "count": len(out)})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "entry.bulk_submit",
		EntityType: "meet", EntityID: meetID, After: string(after),
	}); err != nil {
		return nil, fmt.Errorf("audit entry.bulk_submit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("submit bulk entries: %w", err)
	}
	return out, nil
}

// legAthletes creates one athlete per relay leg input, affiliated with
// clubID, ensuring each holds a meet-wide participant slot. Each leg's
// SYS-103 consent choice is recorded per person (TASK-023, UC-023) with
// the submitting actor as the recorder.
func (s *ResultsService) legAthletes(ctx context.Context, tx store.DBTX, actor Session, meetID, clubID string, legs []RelayLegInput) ([]string, []string, error) {
	ids := make([]string, 0, len(legs))
	names := make([]string, 0, len(legs))
	for i, leg := range legs {
		athlete, err := createAthlete(ctx, tx, clubID, leg.FirstName, leg.LastName, leg.BirthYear, leg.Sex,
			s.entryConsent(actor, leg.PublicationWithdrawn))
		if err != nil {
			return nil, nil, fmt.Errorf("leg %d: %w", i+1, err)
		}
		if _, err := store.EnsureParticipant(ctx, tx, meetID, athlete.ID); err != nil {
			return nil, nil, fmt.Errorf("leg %d: %w", i+1, err)
		}
		ids = append(ids, athlete.ID)
		names = append(names, athlete.FirstName+" "+athlete.LastName)
	}
	return ids, names, nil
}

// SubmitRelayEntry submits a club's relay-team entry (UC-003 #4: "a team of
// 4 named athletes in order plus 2 reserves"): the composition size is not
// hardcoded here — the web form offers the standard 4+2 shape, but this
// service accepts any non-empty composition.
func (s *ResultsService) SubmitRelayEntry(ctx context.Context, actor Session, meetID string, in RelayEntryInput) (EntryDetail, error) {
	if err := Authorize(actor.Role, CapSubmitEntries); err != nil {
		return EntryDetail{}, err
	}
	if strings.TrimSpace(in.Club) == "" {
		return EntryDetail{}, errors.New("entry: club is required for a relay entry")
	}
	if len(in.Composition) == 0 {
		return EntryDetail{}, errors.New("entry: relay composition requires at least one athlete")
	}
	meet, err := store.GetMeet(ctx, s.db, meetID)
	if err != nil {
		return EntryDetail{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return EntryDetail{}, fmt.Errorf("submit relay entry: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	event, err := validateEntryEvent(ctx, tx, meet, in.EventID, time.Now())
	if err != nil {
		return EntryDetail{}, err
	}
	if err := checkEntryLimit(ctx, tx, event); err != nil {
		return EntryDetail{}, err
	}
	club, err := ensureClub(ctx, tx, in.Club)
	if err != nil {
		return EntryDetail{}, err
	}
	composition, compNames, err := s.legAthletes(ctx, tx, actor, meetID, club.ID, in.Composition)
	if err != nil {
		return EntryDetail{}, err
	}
	reserves, resNames, err := s.legAthletes(ctx, tx, actor, meetID, club.ID, in.Reserves)
	if err != nil {
		return EntryDetail{}, err
	}
	team, err := store.CreateRelayTeam(ctx, tx, domain.RelayTeam{
		ClubID: club.ID, Composition: composition, Reserves: reserves,
	})
	if err != nil {
		return EntryDetail{}, err
	}
	rec, err := store.CreateEntry(ctx, tx, domain.Entry{
		EventID: event.ID, RelayTeamID: team.ID, Source: domain.EntrySourceOnline, SubmittedBy: actor.AccountID,
	})
	if err != nil {
		return EntryDetail{}, err
	}
	if err := auditEntrySubmit(ctx, tx, actor, rec.ID, club.Name, "entry.submit_relay"); err != nil {
		return EntryDetail{}, err
	}
	if err := tx.Commit(); err != nil {
		return EntryDetail{}, fmt.Errorf("submit relay entry: %w", err)
	}
	return EntryDetail{
		EntryRecord: rec, Event: event, ClubName: club.Name, FeeCents: meet.RelayFeeCents,
		RelayTeam: &RelayTeamDetail{RelayTeamRecord: team, Composition: compNames, Reserves: resNames},
	}, nil
}

// UpdateRelayComposition revises a relay entry's leg composition/reserves
// (UC-003 #4: "permits changes until the configured deadline, rejecting them
// after it") — deadline enforcement mirrors submission (SYS-011).
func (s *ResultsService) UpdateRelayComposition(ctx context.Context, actor Session, meetID, entryID string, expectedVersion int64, composition, reserves []RelayLegInput) (EntryDetail, error) {
	if err := Authorize(actor.Role, CapSubmitEntries); err != nil {
		return EntryDetail{}, err
	}
	if len(composition) == 0 {
		return EntryDetail{}, errors.New("entry: relay composition requires at least one athlete")
	}
	entry, err := store.GetEntry(ctx, s.db, entryID)
	if err != nil {
		return EntryDetail{}, err
	}
	if entry.RelayTeamID == "" {
		return EntryDetail{}, ErrNotRelayEntry
	}
	event, err := store.GetEvent(ctx, s.db, entry.EventID)
	if err != nil {
		return EntryDetail{}, err
	}
	if event.MeetID != meetID {
		return EntryDetail{}, store.ErrNotFound
	}
	if event.EntryDeadline != nil && !time.Now().Before(*event.EntryDeadline) {
		return EntryDetail{}, ErrEntryDeadlinePassed
	}
	team, err := store.GetRelayTeam(ctx, s.db, entry.RelayTeamID)
	if err != nil {
		return EntryDetail{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return EntryDetail{}, fmt.Errorf("update relay composition: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	compIDs, compNames, err := s.legAthletes(ctx, tx, actor, meetID, team.ClubID, composition)
	if err != nil {
		return EntryDetail{}, err
	}
	resIDs, resNames, err := s.legAthletes(ctx, tx, actor, meetID, team.ClubID, reserves)
	if err != nil {
		return EntryDetail{}, err
	}
	newVersion, err := store.UpdateRelayTeamComposition(ctx, tx, team.ID, expectedVersion, compIDs, resIDs)
	if err != nil {
		return EntryDetail{}, err
	}
	after, _ := json.Marshal(map[string]any{"composition": compNames, "reserves": resNames})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "entry.relay_composition_update",
		EntityType: "relay_team", EntityID: team.ID, After: string(after),
	}); err != nil {
		return EntryDetail{}, fmt.Errorf("audit entry.relay_composition_update: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return EntryDetail{}, fmt.Errorf("update relay composition: %w", err)
	}
	team.Composition, team.Reserves, team.Version = compIDs, resIDs, newVersion
	clubName := ""
	if names, err := store.ClubNames(ctx, s.db, []string{team.ClubID}); err == nil {
		clubName = names[team.ClubID]
	}
	return EntryDetail{
		EntryRecord: entry, Event: event, ClubName: clubName,
		RelayTeam: &RelayTeamDetail{RelayTeamRecord: team, Composition: compNames, Reserves: resNames},
	}, nil
}

// auditEntrySubmit records one entry-submission audit row.
func auditEntrySubmit(ctx context.Context, tx *sql.Tx, actor Session, entryID, subject, action string) error {
	after, _ := json.Marshal(map[string]string{"subject": subject})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: action,
		EntityType: "entry", EntityID: entryID, After: string(after),
	}); err != nil {
		return fmt.Errorf("audit %s: %w", action, err)
	}
	return nil
}

// MyEntries lists the entries actor submitted at meetID, newest first
// (UC-003 #1/#2: "the entry is visible to them with status entered").
func (s *ResultsService) MyEntries(ctx context.Context, actor Session, meetID string) ([]EntryDetail, error) {
	if err := Authorize(actor.Role, CapSubmitEntries); err != nil {
		return nil, err
	}
	if _, err := store.GetMeet(ctx, s.db, meetID); err != nil {
		return nil, err
	}
	recs, err := store.ListEntriesBySubmitter(ctx, s.db, meetID, actor.AccountID)
	if err != nil {
		return nil, err
	}
	return s.enrichEntries(ctx, recs)
}

// EntryExceptions lists the meet's entries flagged as failing their event's
// entry standard (SYS-015 "reportable"; UC-003 #5's organizer exception
// report).
func (s *ResultsService) EntryExceptions(ctx context.Context, actor Session, meetID string) ([]EntryDetail, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return nil, err
	}
	if _, err := store.GetMeet(ctx, s.db, meetID); err != nil {
		return nil, err
	}
	recs, err := store.ListEntriesByMeet(ctx, s.db, meetID)
	if err != nil {
		return nil, err
	}
	var failing []store.EntryRecord
	for _, r := range recs {
		if r.FailsStandard {
			failing = append(failing, r)
		}
	}
	return s.enrichEntries(ctx, failing)
}

// enrichEntries resolves each entry's event, athlete/relay-team and club
// data for display.
func (s *ResultsService) enrichEntries(ctx context.Context, recs []store.EntryRecord) ([]EntryDetail, error) {
	out := make([]EntryDetail, 0, len(recs))
	events := map[string]store.EventRecord{}
	meets := map[string]store.MeetRecord{}
	for _, rec := range recs {
		event, ok := events[rec.EventID]
		if !ok {
			var err error
			if event, err = store.GetEvent(ctx, s.db, rec.EventID); err != nil {
				return nil, err
			}
			events[rec.EventID] = event
		}
		meet, ok := meets[event.MeetID]
		if !ok {
			var err error
			if meet, err = store.GetMeet(ctx, s.db, event.MeetID); err != nil {
				return nil, err
			}
			meets[event.MeetID] = meet
		}
		detail := EntryDetail{EntryRecord: rec, Event: event}
		switch {
		case rec.AthleteID != "":
			athlete, err := store.GetAthlete(ctx, s.db, rec.AthleteID)
			if err != nil {
				return nil, err
			}
			detail.AthleteName = athlete.FirstName + " " + athlete.LastName
			if len(athlete.ClubIDs) > 0 {
				if names, err := store.ClubNames(ctx, s.db, athlete.ClubIDs); err == nil {
					detail.ClubName = names[athlete.ClubIDs[0]]
				}
			}
			detail.FeeCents = meet.EntryFeeCents
			if eligRec, err := store.GetEntryEligibility(ctx, s.db, rec.ID); err == nil {
				detail.Eligibility = eligibilityViewFrom(eligRec)
			}
		case rec.RelayTeamID != "":
			team, err := store.GetRelayTeam(ctx, s.db, rec.RelayTeamID)
			if err != nil {
				return nil, err
			}
			rtd := RelayTeamDetail{RelayTeamRecord: team}
			for _, id := range team.Composition {
				if a, err := store.GetAthlete(ctx, s.db, id); err == nil {
					rtd.Composition = append(rtd.Composition, a.FirstName+" "+a.LastName)
				}
			}
			for _, id := range team.Reserves {
				if a, err := store.GetAthlete(ctx, s.db, id); err == nil {
					rtd.Reserves = append(rtd.Reserves, a.FirstName+" "+a.LastName)
				}
			}
			detail.RelayTeam = &rtd
			if names, err := store.ClubNames(ctx, s.db, []string{team.ClubID}); err == nil {
				detail.ClubName = names[team.ClubID]
			}
			detail.FeeCents = meet.RelayFeeCents
		}
		out = append(out, detail)
	}
	return out, nil
}

// --- Bib assignment (TASK-016, UC-006 #1/#2, SYS-018) — reuses the
// meet-wide participants table (TASK-007). ---

// AssignBib assigns or edits one participant's bib under optimistic
// concurrency; a bib already taken within the meet is rejected
// (ErrDuplicateParticipant, UC-006 #2).
func (s *ResultsService) AssignBib(ctx context.Context, actor Session, meetID, participantID string, expectedVersion int64, bib string) error {
	if err := Authorize(actor.Role, CapOrganizeMeet); err != nil {
		return err
	}
	bib = strings.TrimSpace(bib)
	if bib == "" {
		return ErrBibRequired
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("assign bib: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := store.UpdateParticipantBib(ctx, tx, participantID, expectedVersion, bib); err != nil {
		return err
	}
	after, _ := json.Marshal(map[string]string{"bib": bib, "meet": meetID})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "participant.bib_assign",
		EntityType: "participant", EntityID: participantID, After: string(after),
	}); err != nil {
		return fmt.Errorf("audit participant.bib_assign: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("assign bib: %w", err)
	}
	return nil
}

// BulkAssignBibsByClub assigns sequential bib numbers, starting at start, to
// every currently-unbibbed participant affiliated with clubID at meetID, in
// last/first-name order (UC-006 #1: "assigns bibs from ranges per club").
// Returns the number of bibs assigned.
func (s *ResultsService) BulkAssignBibsByClub(ctx context.Context, actor Session, meetID, clubID string, start int) (int, error) {
	if err := Authorize(actor.Role, CapOrganizeMeet); err != nil {
		return 0, err
	}
	if start <= 0 {
		return 0, errors.New("bib: starting number must be positive")
	}
	rows, err := store.ListParticipants(ctx, s.db, meetID)
	if err != nil {
		return 0, err
	}
	var targets []store.ParticipantRow
	for _, r := range rows {
		if r.Bib != "" {
			continue
		}
		for _, id := range r.Athlete.ClubIDs {
			if id == clubID {
				targets = append(targets, r)
				break
			}
		}
	}
	if len(targets) == 0 {
		return 0, nil
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Athlete.LastName != targets[j].Athlete.LastName {
			return targets[i].Athlete.LastName < targets[j].Athlete.LastName
		}
		return targets[i].Athlete.FirstName < targets[j].Athlete.FirstName
	})

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("bulk assign bibs: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	n := start
	for _, t := range targets {
		if _, err := store.UpdateParticipantBib(ctx, tx, t.ID, t.Version, strconv.Itoa(n)); err != nil {
			return 0, err
		}
		n++
	}
	after, _ := json.Marshal(map[string]any{"club": clubID, "start": start, "count": len(targets)})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "participant.bib_bulk_assign",
		EntityType: "meet", EntityID: meetID, After: string(after),
	}); err != nil {
		return 0, fmt.Errorf("audit participant.bib_bulk_assign: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("bulk assign bibs: %w", err)
	}
	return len(targets), nil
}

// --- Fee summary (TASK-016, UC-006 #3, SYS-017) ---

// ClubFeeSummary is one club's entry counts and fee total.
type ClubFeeSummary struct {
	ClubID            string
	ClubName          string
	IndividualEntries int
	RelayEntries      int
	TotalCents        int64
}

// FeeSummary is the meet's configured fee schedule plus the per-club
// breakdown and grand total (UC-006 #3: "per-club totals equal the entries ×
// schedule arithmetic").
type FeeSummary struct {
	EntryFeeCents int64
	RelayFeeCents int64
	Clubs         []ClubFeeSummary
	TotalCents    int64
}

// FeeSummary computes the meet's fee summary from its active entries and
// configured fee schedule (scratched entries are excluded — they owe
// nothing).
func (s *ResultsService) FeeSummary(ctx context.Context, actor Session, meetID string) (FeeSummary, error) {
	if err := Authorize(actor.Role, CapOrganizeMeet); err != nil {
		return FeeSummary{}, err
	}
	meet, err := store.GetMeet(ctx, s.db, meetID)
	if err != nil {
		return FeeSummary{}, err
	}
	recs, err := store.ListEntriesByMeet(ctx, s.db, meetID)
	if err != nil {
		return FeeSummary{}, err
	}

	byClub := map[string]*ClubFeeSummary{}
	var order []string
	add := func(clubID, clubName string, individual bool) {
		cs, ok := byClub[clubID]
		if !ok {
			cs = &ClubFeeSummary{ClubID: clubID, ClubName: clubName}
			byClub[clubID] = cs
			order = append(order, clubID)
		}
		if individual {
			cs.IndividualEntries++
		} else {
			cs.RelayEntries++
		}
	}

	for _, r := range recs {
		if r.Status == domain.EntryScratched {
			continue
		}
		switch {
		case r.AthleteID != "":
			athlete, err := store.GetAthlete(ctx, s.db, r.AthleteID)
			if err != nil {
				return FeeSummary{}, err
			}
			clubID, clubName := "", ""
			if len(athlete.ClubIDs) > 0 {
				clubID = athlete.ClubIDs[0]
				if names, err := store.ClubNames(ctx, s.db, []string{clubID}); err == nil {
					clubName = names[clubID]
				}
			}
			add(clubID, clubName, true)
		case r.RelayTeamID != "":
			team, err := store.GetRelayTeam(ctx, s.db, r.RelayTeamID)
			if err != nil {
				return FeeSummary{}, err
			}
			clubName := ""
			if names, err := store.ClubNames(ctx, s.db, []string{team.ClubID}); err == nil {
				clubName = names[team.ClubID]
			}
			add(team.ClubID, clubName, false)
		}
	}

	sum := FeeSummary{EntryFeeCents: meet.EntryFeeCents, RelayFeeCents: meet.RelayFeeCents}
	for _, id := range order {
		cs := byClub[id]
		cs.TotalCents = int64(cs.IndividualEntries)*meet.EntryFeeCents + int64(cs.RelayEntries)*meet.RelayFeeCents
		sum.TotalCents += cs.TotalCents
		sum.Clubs = append(sum.Clubs, *cs)
	}
	sort.Slice(sum.Clubs, func(i, j int) bool { return sum.Clubs[i].ClubName < sum.Clubs[j].ClubName })
	return sum, nil
}
