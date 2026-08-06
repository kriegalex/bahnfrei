// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// ErrDuplicateParticipant aliases the store sentinel for web handlers.
var ErrDuplicateParticipant = store.ErrDuplicateParticipant

// ResultsService hosts participation, scored result capture and combined
// standings (UC-033 #2–#4; SYS-053/052), plus the attempt-level field and
// track capture flows on top (TASK-008, UC-010/UC-011).
type ResultsService struct {
	db *sql.DB
	// readDB is the pooled WAL read-connection set (ADR-004 read-path
	// amendment, TASK-035/OQ-066), wired via SetReadDB — normally
	// store.Store.ReadDB(). Only the read-only query paths behind the
	// public results/start-list surfaces use it (readConn); every write
	// still goes through db, preserving the ADR-004 §2 single-writer
	// invariant untouched. Falls back to db when unset, matching every
	// other SetXxx wiring hook's "additive, never breaks an existing
	// caller" convention (see SetPrivacy in internal/web/server.go).
	readDB        *sql.DB
	catalog       *domain.DisciplineCatalog
	schemes       map[string]*domain.CategoryScheme
	tables        map[string]*domain.ScoringTable
	templates     map[string]*domain.MeetTemplate
	seriesUploads map[string]*domain.SeriesUploadTemplate
	// importProfiles are the SYS-013 entry-import mapping profiles
	// (TASK-017), wired via SetImportMappingProfiles — normally the built-in
	// system-native/alabus profiles (see internal/domain/schemes.go).
	importProfiles map[string]*domain.ImportMappingProfile
	// combinedTables are the SYS-044 WA combined-events formula tables
	// (TASK-021), wired via SetCombinedScoringTables — normally the
	// built-in domain.BuiltinCombinedScoringTables().
	combinedTables map[string]*domain.CombinedScoringTable
	// recordLists are the SYS-049 loadable record/best reference lists
	// (TASK-022), wired via SetRecordLists — normally
	// domain.BuiltinRecordLists() plus whatever organizer-authored lists a
	// deployment adds. A meet references the subset that applies to it via
	// store.AddMeetRecordList/ListMeetRecordListIDs (internal/app/record.go).
	recordLists map[string]*domain.RecordList
	onChange    func(meetID string)
	now         Clock
}

// SetImportMappingProfiles wires the SYS-013 entry-import mapping profiles
// (TASK-017), mirroring SetSeriesUploadTemplates' precedent — normally the
// built-in profiles from domain.BuiltinImportMappingProfiles.
func (s *ResultsService) SetImportMappingProfiles(profiles map[string]*domain.ImportMappingProfile) {
	s.importProfiles = profiles
}

// SetCombinedScoringTables wires the WA combined-events formula tables
// (SYS-044, TASK-021) scorePoints consults for a meet configured with one
// (store.GetMeetCombinedScoringTable, set at meet creation from a
// wa-decathlon/wa-heptathlon-style template) — normally the built-in
// domain.BuiltinCombinedScoringTables(). Optional: without this, such a
// meet's capture saves fail with a clear "unknown scoring table" error
// rather than silently scoring 0.
func (s *ResultsService) SetCombinedScoringTables(tables map[string]*domain.CombinedScoringTable) {
	s.combinedTables = tables
}

// SetRecordLists wires the SYS-049 loadable record/best reference lists
// (TASK-022) — normally domain.BuiltinRecordLists(). A meet with no
// configured record list (SetMeetRecordLists/store.AddMeetRecordList) still
// gets PB/SB flagging from in-system athlete history; it just has no
// WR/AR/NR/MR reference to flag against.
func (s *ResultsService) SetRecordLists(lists map[string]*domain.RecordList) {
	s.recordLists = lists
}

// SetReadDB wires the pooled WAL read-connection set (ADR-004 read-path
// amendment, TASK-035/OQ-066) — normally store.Store.ReadDB(). Optional:
// without it, readConn falls back to the writer connection (correct, just
// not pooled — every existing caller and test keeps working unchanged).
func (s *ResultsService) SetReadDB(db *sql.DB) { s.readDB = db }

// readConn returns the pooled read connection for the read-only
// public-surface queries that use it, falling back to the writer
// connection when no read pool has been wired.
func (s *ResultsService) readConn() *sql.DB {
	if s.readDB != nil {
		return s.readDB
	}
	return s.db
}

// NewResultsService wires a ResultsService; catalog, schemes, tables and
// templates are normally the built-in data.
func NewResultsService(db *sql.DB, catalog *domain.DisciplineCatalog, schemes map[string]*domain.CategoryScheme,
	tables map[string]*domain.ScoringTable, templates map[string]*domain.MeetTemplate) *ResultsService {
	return &ResultsService{db: db, catalog: catalog, schemes: schemes, tables: tables, templates: templates, now: time.Now}
}

// WithClock overrides the time source (tests only) — used by the SYS-103
// consent-timestamp and TestConsent* tests for deterministic assertions,
// mirroring SessionManager.WithClock.
func (s *ResultsService) WithClock(now Clock) *ResultsService {
	s.now = now
	return s
}

// OnResultsChanged registers the live-update hook: fn runs after every
// committed capture write with the affected meet's ID (UC-011 #4 — the web
// layer publishes it on the SSE bus, SYS-071). Set once at wiring time.
func (s *ResultsService) OnResultsChanged(fn func(meetID string)) { s.onChange = fn }

// notifyChanged fires the registered live-update hook, if any.
func (s *ResultsService) notifyChanged(meetID string) {
	if s.onChange != nil {
		s.onChange(meetID)
	}
}

// ParticipantInput is one athlete registration: person data (SYS-010)
// plus their start number at this meet. PublicationWithdrawn collects the
// SYS-103 publication-consent choice at entry time (UC-023: "entry flows
// collect consent where the use-cases say so") — false (the default, an
// unchecked form checkbox) means the athlete's results are publicly listed
// as usual; true suppresses their identity on public surfaces from the
// start (see internal/domain/privacy.go for the enforcement).
type ParticipantInput struct {
	FirstName            string
	LastName             string
	BirthYear            int
	Sex                  domain.Sex
	Club                 string
	Bib                  string
	PublicationWithdrawn bool
}

// RegisterParticipant registers an athlete for a meet, creating the
// athlete (and their club on first sight) — the minimal roster flow the
// UKC PoC needs; entry workflows proper are TASK-016.
func (s *ResultsService) RegisterParticipant(ctx context.Context, actor Session, meetID string, in ParticipantInput) (store.ParticipantRow, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return store.ParticipantRow{}, err
	}
	if _, err := store.GetMeet(ctx, s.db, meetID); err != nil {
		return store.ParticipantRow{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.ParticipantRow{}, fmt.Errorf("register participant: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var clubIDs []string
	if in.Club != "" {
		club, err := store.GetClubByName(ctx, tx, in.Club)
		if errors.Is(err, store.ErrNotFound) {
			club, err = store.CreateClub(ctx, tx, domain.Club{Name: in.Club})
		}
		if err != nil {
			return store.ParticipantRow{}, err
		}
		clubIDs = []string{club.ID}
	}
	athlete, err := store.CreateAthlete(ctx, tx, domain.Athlete{
		FirstName: in.FirstName,
		LastName:  in.LastName,
		BirthYear: in.BirthYear,
		Sex:       in.Sex,
		ClubIDs:   clubIDs,
		Consent: domain.PublicationConsent{
			ResultsPublicationWithdrawn: in.PublicationWithdrawn,
			RecordedAt:                  s.now(),
			RecordedBy:                  actor.AccountID,
		},
	})
	if err != nil {
		return store.ParticipantRow{}, err
	}
	p, err := store.RegisterParticipant(ctx, tx, meetID, athlete.ID, in.Bib)
	if err != nil {
		return store.ParticipantRow{}, err
	}

	after, _ := json.Marshal(map[string]string{
		"athlete": athlete.ID, "bib": in.Bib, "name": in.FirstName + " " + in.LastName,
	})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "participant.register",
		EntityType: "participant", EntityID: p.ID, After: string(after),
	}); err != nil {
		return store.ParticipantRow{}, fmt.Errorf("audit participant register: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return store.ParticipantRow{}, fmt.Errorf("register participant: %w", err)
	}
	return store.ParticipantRow{Participant: p, Athlete: athlete.Athlete}, nil
}

// Participants lists a meet's registered athletes. Read-only: uses the
// pooled read connection (ADR-004 read-path amendment, TASK-035) since
// this backs the public start-list page as well as operator rosters.
func (s *ResultsService) Participants(ctx context.Context, meetID string) ([]store.ParticipantRow, error) {
	return store.ListParticipants(ctx, s.readConn(), meetID)
}

// MatchesParticipantSearch reports whether a participant's name, bib or
// club contains query, case-insensitively (DEC-021/OQ-067, TASK-038:
// "server-side query-param filter on the operator roster and entries
// lists — name/bib/club at minimum"). An empty query always matches, so
// this doubles as the no-filter predicate the roster/bib-assignment
// pages fall back to when no search has been entered. Exported so both
// internal/web (the roster/bibs handlers) and the SYS-120 performance
// suite (internal/app, perf-tagged) share one definition of "matches".
func MatchesParticipantSearch(query, firstName, lastName, bib, club string) bool {
	q := strings.TrimSpace(strings.ToLower(query))
	if q == "" {
		return true
	}
	name := strings.ToLower(firstName + " " + lastName)
	return strings.Contains(name, q) ||
		strings.Contains(strings.ToLower(bib), q) ||
		strings.Contains(strings.ToLower(club), q)
}

// SetConsent updates an athlete's SYS-103 publication-consent flags
// (UC-023 #3: "consent flags changed mid-meet" must reach public surfaces
// within one publication cycle). meetID drives the same live-update hook
// every capture write fires (notifyChanged): the web layer's per-meet
// public-results render cache (ADR-004 read-path amendment, TASK-035)
// invalidates on it, so this write's very next public request already
// reflects it — the same promise this held before that cache existed,
// restored explicitly now that "every public read recomputes standings
// live" is no longer true by default. Office level and above
// (CapPrivacyActions); audited with the new flag values only, never the
// athlete's name (the audit row already carries the athlete's ID as
// EntityID, which is enough to look the change up without duplicating
// identity into the log body).
func (s *ResultsService) SetConsent(ctx context.Context, actor Session, meetID, athleteID string, withdrawn bool) error {
	if err := Authorize(actor.Role, CapPrivacyActions); err != nil {
		return err
	}
	athlete, err := store.GetAthlete(ctx, s.db, athleteID)
	if err != nil {
		return err
	}
	if athlete.Anonymized {
		return ErrAthleteAnonymized
	}
	consent := domain.PublicationConsent{
		ResultsPublicationWithdrawn: withdrawn,
		PhotoConsentGiven:           athlete.Consent.PhotoConsentGiven,
		ExtendedDataConsentGiven:    athlete.Consent.ExtendedDataConsentGiven,
		RecordedAt:                  s.now(),
		RecordedBy:                  actor.AccountID,
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set consent: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := store.UpdateAthleteConsent(ctx, tx, athleteID, athlete.Version, consent); err != nil {
		return err
	}
	after, _ := json.Marshal(map[string]bool{"results_publication_withdrawn": withdrawn})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "athlete.consent.update",
		EntityType: "athlete", EntityID: athleteID, After: string(after),
	}); err != nil {
		return fmt.Errorf("audit consent update: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("set consent: %w", err)
	}
	s.notifyChanged(meetID)
	return nil
}

// SetOutOfCompetition flags or clears a participant's ausser
// Konkurrenz/hors concours status (TASK-036, DEC-016/OQ-020 investigation
// of the LV Langenthal Gesamtrangliste's "n.a." row with every discipline
// mark present, Thome Lauriane W12): the athlete's marks stay captured and
// visible, but Standings/FinalStandings never assign them a numeric rank
// while the flag is set. Office level and above (CapOfficeActions,
// matching RegisterParticipant/bib assignment); audited with the new flag
// value. There is deliberately no dedicated operator-UI toggle yet
// (OQ-091) — this is the callable capability the flag needed to exist and
// be correctly interpreted by standings; wiring a roster-page control is
// left to a follow-up.
func (s *ResultsService) SetOutOfCompetition(ctx context.Context, actor Session, meetID, athleteID string, outOfCompetition bool) error {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return err
	}
	p, err := store.GetParticipantByAthlete(ctx, s.db, meetID, athleteID)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set out of competition: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := store.UpdateParticipantOutOfCompetition(ctx, tx, p.ID, p.Version, outOfCompetition); err != nil {
		return err
	}
	after, _ := json.Marshal(map[string]bool{"out_of_competition": outOfCompetition})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "participant.out_of_competition.update",
		EntityType: "participant", EntityID: p.ID, After: string(after),
	}); err != nil {
		return fmt.Errorf("audit out-of-competition update: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("set out of competition: %w", err)
	}
	s.notifyChanged(meetID)
	return nil
}

// --- participant identity correction (TASK-049, SYS-150/UC-043) ---

// bibFormat is the same shape RegisterParticipant/AssignBib have always
// accepted in practice (digits, matching the roster's
// "CAST(p.bib AS INTEGER)" ordering — store.ListParticipants) — an empty
// value clears the bib, exactly like the roster-add and bib-assignment
// forms already allow.
var bibFormat = regexp.MustCompile(`^[0-9]+$`)

// ValidBibFormat reports whether bib is acceptable input for a participant
// correction: empty (no bib yet) or digits-only. Exported so the web
// layer's field-level validation (OQ-075/UC-038 #4) shares this exact rule
// with the app-layer defense-in-depth check in UpdateParticipantIdentity.
func ValidBibFormat(bib string) bool {
	return bib == "" || bibFormat.MatchString(bib)
}

// ParticipantIdentityBirthYearBounds is the plausible-birth-year range
// shared by every registration/correction form (mirrors
// internal/web.individualEntryBirthYearBounds's rationale: no athlete
// competing today was born before 1900, and a future birth year is never
// valid). Takes the clock the service was wired with so tests using
// WithClock stay deterministic.
func (s *ResultsService) ParticipantIdentityBirthYearBounds() (min, max int) {
	return 1900, s.now().Year()
}

// ErrParticipantIdentityInvalid means an UpdateParticipantIdentity call
// failed structural validation (required name, birth-year bounds, sex,
// bib format) — a defense-in-depth backstop behind the web layer's own
// field-level validation, mirroring SubmitIndividualEntry's precedent.
var ErrParticipantIdentityInvalid = errors.New("participant identity: invalid field values")

// ParticipantIdentityInput is one office-issued correction to a
// participant's identity data (SYS-150: name, birth year, sex, club, bib).
// Reason is optional — unlike CorrectionInput's result-correction reason
// (SYS-046), SYS-150 does not require one; it is carried into the audit
// row when given.
type ParticipantIdentityInput struct {
	FirstName string
	LastName  string
	BirthYear int
	Sex       domain.Sex
	Club      string
	Bib       string
	Reason    string
}

// UpdateParticipantIdentity corrects a participant's identity data after
// registration (SYS-150, UC-043 F4): office-only, optimistic-version-
// guarded on the participant row (a concurrent edit — including a second
// submission of this same form — yields ErrConflict), rejects a bib
// already taken by someone else in the meet (ErrDuplicateParticipant,
// reusing AssignBib's store-level uniqueness check) and an already-erased
// athlete (ErrAthleteAnonymized — an erased participant is never
// editable). The correction never touches the athletes.birth_date,
// external_ids, para_classes or consent columns, and never writes to
// results/attempts: UC-043 #3 requires captured marks to survive a
// correction byte-identical, which holds trivially here since this
// function's only writes are to the participants and athletes rows.
//
// Category re-derivation (UC-043 #1) needs no extra step: every
// standings/capture-grouping call resolves an athlete's division from
// their *current* birth year and sex on every read
// (ResultsService.standings, capture.go's catByAthlete — see
// domain.CategoryScheme.ResolveDefaultCategory) rather than from a stored
// snapshot, so the very next read after this commits already reflects the
// athlete's new division.
func (s *ResultsService) UpdateParticipantIdentity(ctx context.Context, actor Session, meetID, participantID string, expectedVersion int64, in ParticipantIdentityInput) (store.ParticipantRow, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return store.ParticipantRow{}, err
	}
	firstName := strings.TrimSpace(in.FirstName)
	lastName := strings.TrimSpace(in.LastName)
	club := strings.TrimSpace(in.Club)
	bib := strings.TrimSpace(in.Bib)
	minYear, maxYear := s.ParticipantIdentityBirthYearBounds()
	if lastName == "" || in.BirthYear < minYear || in.BirthYear > maxYear ||
		(in.Sex != domain.SexMale && in.Sex != domain.SexFemale) || !ValidBibFormat(bib) {
		return store.ParticipantRow{}, ErrParticipantIdentityInvalid
	}

	p, err := store.GetParticipant(ctx, s.db, participantID)
	if err != nil {
		return store.ParticipantRow{}, err
	}
	if p.MeetID != meetID {
		return store.ParticipantRow{}, store.ErrNotFound
	}
	before, err := store.GetAthlete(ctx, s.db, p.AthleteID)
	if err != nil {
		return store.ParticipantRow{}, err
	}
	if before.Anonymized {
		return store.ParticipantRow{}, ErrAthleteAnonymized
	}
	beforeClubs, err := store.ClubNames(ctx, s.db, before.ClubIDs)
	if err != nil {
		return store.ParticipantRow{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.ParticipantRow{}, fmt.Errorf("update participant identity: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// The participant-row optimistic update is the single concurrency gate
	// for this whole correction (bib included): it always runs, even when
	// bib is unchanged, so a stale expectedVersion is rejected regardless
	// of which fields the office actually changed.
	if _, err := store.UpdateParticipantBib(ctx, tx, participantID, expectedVersion, bib); err != nil {
		return store.ParticipantRow{}, err
	}

	var clubIDs []string
	if club != "" {
		c, err := store.GetClubByName(ctx, tx, club)
		if errors.Is(err, store.ErrNotFound) {
			c, err = store.CreateClub(ctx, tx, domain.Club{Name: club})
		}
		if err != nil {
			return store.ParticipantRow{}, err
		}
		clubIDs = []string{c.ID}
	}

	// The athlete row's version is re-read inside the write transaction
	// (fresh, not the caller-supplied expectedVersion): this function is
	// the only writer of these five columns, and SQLite's single-writer
	// serialization (ADR-004 §2) guarantees no other transaction can have
	// changed them between this read and the write below, so the
	// participant-row check above remains the sole caller-visible
	// concurrency gate.
	fresh, err := store.GetAthlete(ctx, tx, p.AthleteID)
	if err != nil {
		return store.ParticipantRow{}, err
	}
	if fresh.Anonymized {
		// Defense-in-depth against the narrow window between the
		// pre-transaction anonymized check above and this write: an
		// erasure that lands in between must still win over an in-flight
		// identity correction, never the other way round.
		return store.ParticipantRow{}, ErrAthleteAnonymized
	}
	if _, err := store.UpdateAthleteIdentity(ctx, tx, p.AthleteID, fresh.Version,
		firstName, lastName, in.BirthYear, in.Sex, clubIDs); err != nil {
		return store.ParticipantRow{}, err
	}

	beforeClub := ""
	if len(before.ClubIDs) > 0 {
		beforeClub = beforeClubs[before.ClubIDs[0]]
	}
	beforeJSON, _ := json.Marshal(map[string]string{
		"name":      before.FirstName + " " + before.LastName,
		"birthYear": fmt.Sprintf("%d", before.BirthYear), "sex": string(before.Sex),
		"club": beforeClub, "bib": p.Bib,
	})
	afterJSON, _ := json.Marshal(map[string]string{
		"name":      firstName + " " + lastName,
		"birthYear": fmt.Sprintf("%d", in.BirthYear), "sex": string(in.Sex),
		"club": club, "bib": bib,
	})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "participant.identity_correct",
		EntityType: "participant", EntityID: participantID,
		Before: string(beforeJSON), After: string(afterJSON), Reason: strings.TrimSpace(in.Reason),
	}); err != nil {
		return store.ParticipantRow{}, fmt.Errorf("audit participant.identity_correct: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return store.ParticipantRow{}, fmt.Errorf("update participant identity: %w", err)
	}
	s.notifyChanged(meetID)

	updated, err := store.GetParticipant(ctx, s.db, participantID)
	if err != nil {
		return store.ParticipantRow{}, err
	}
	updatedAthlete, err := store.GetAthlete(ctx, s.db, p.AthleteID)
	if err != nil {
		return store.ParticipantRow{}, err
	}
	return store.ParticipantRow{Participant: updated, Athlete: updatedAthlete.Athlete}, nil
}

// ClubNamesFor resolves the club names of the given participants' club
// IDs (presentation joins for roster and start lists). Read-only: pooled
// read connection (ADR-004 read-path amendment, TASK-035).
func (s *ResultsService) ClubNamesFor(ctx context.Context, rows []store.ParticipantRow) (map[string]string, error) {
	var ids []string
	for _, p := range rows {
		ids = append(ids, p.Athlete.ClubIDs...)
	}
	return store.ClubNames(ctx, s.readConn(), ids)
}

// ResultInput is one settled mark for an athlete in one of the meet's
// disciplines. For meets with a single unit per discipline (template
// meets) the discipline identifies the unit.
type ResultInput struct {
	AthleteID      string
	DisciplineCode string
	Mark           string
	Timing         domain.Timing
	Status         domain.QualificationStatus // non-empty for DNS/NM/DQ/…
	StatusDetail   string                     // DQ rule reference (SYS-045)
}

// SaveResult stores a settled result and scores it against the meet's
// scoring table (UC-033 #2): points follow from the table, the athlete's
// sex column and the timing method; non-scoring statuses carry no points.
// Saving again for the same athlete and discipline replaces the settled
// mark (version-bumped and audited, SYS-046).
func (s *ResultsService) SaveResult(ctx context.Context, actor Session, meetID string, in ResultInput) (store.ResultRecord, error) {
	if err := Authorize(actor.Role, CapCaptureResults); err != nil {
		return store.ResultRecord{}, err
	}
	meet, err := store.GetMeet(ctx, s.db, meetID)
	if err != nil {
		return store.ResultRecord{}, err
	}
	athlete, err := store.GetAthlete(ctx, s.db, in.AthleteID)
	if err != nil {
		return store.ResultRecord{}, err
	}
	unitID, err := s.disciplineUnit(ctx, meetID, in.DisciplineCode)
	if err != nil {
		return store.ResultRecord{}, err
	}

	result := domain.Result{
		UnitID:       unitID,
		AthleteID:    athlete.ID,
		Mark:         in.Mark,
		Status:       in.Status,
		StatusDetail: in.StatusDetail,
	}
	if in.Status != domain.StatusNone {
		if err := domain.ValidateCaptureStatus(in.Status, in.StatusDetail); err != nil {
			return store.ResultRecord{}, err
		}
	} else {
		if in.Mark == "" {
			return store.ResultRecord{}, fmt.Errorf("a mark or a status is required")
		}
		if result.Points, err = s.scorePoints(ctx, s.db, meet, in.DisciplineCode, in.Timing, athlete.Sex, in.Mark); err != nil {
			return store.ResultRecord{}, err
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.ResultRecord{}, fmt.Errorf("save result: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rec, err := store.SaveResult(ctx, tx, result, in.Timing)
	if err != nil {
		return store.ResultRecord{}, err
	}
	after, _ := json.Marshal(map[string]any{
		"discipline": in.DisciplineCode, "mark": in.Mark, "timing": in.Timing,
		"status": in.Status, "statusDetail": in.StatusDetail, "points": result.Points,
	})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "result.save",
		EntityType: "result", EntityID: rec.ID, After: string(after),
	}); err != nil {
		return store.ResultRecord{}, fmt.Errorf("audit result save: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return store.ResultRecord{}, fmt.Errorf("save result: %w", err)
	}
	s.notifyChanged(meetID)
	return rec, nil
}

// disciplineUnit resolves the single capturable unit of a meet's
// discipline. Meets with multiple units per discipline (heats — TASK-018
// seeding) need unit-level capture (TASK-008) instead.
func (s *ResultsService) disciplineUnit(ctx context.Context, meetID, disciplineCode string) (string, error) {
	units, err := store.ListMeetUnits(ctx, s.db, meetID)
	if err != nil {
		return "", err
	}
	var found []string
	for _, u := range units {
		if u.DisciplineCode == disciplineCode {
			found = append(found, u.UnitID)
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("meet has no %q event", disciplineCode)
	case 1:
		return found[0], nil
	default:
		return "", fmt.Errorf("discipline %q has %d units; capture per unit instead", disciplineCode, len(found))
	}
}

// StandingRow is one athlete's line in a division ranking list, matching
// the observed federation presentation (UC-033 #4, C7.3): rank, bib,
// name, club, birth year, per-discipline marks and points, total. A
// missing discipline appears as a Performance without points — the
// explicit gap UC-033 #3 requires. Consent carries the athlete's SYS-103
// publication-consent flags through to every renderer built on
// StandingRow (public results, printed result lists, series upload,
// office standings) so minimization/suppression (internal/domain/privacy.go
// PublicDisplayNameFor/PublicDisplayClubFor) has one shared source instead
// of a second athlete lookup per page.
type StandingRow struct {
	Rank      int
	Bib       string
	AthleteID string
	FirstName string
	LastName  string
	ClubName  string
	BirthYear int
	Sex       domain.Sex
	Marks     []domain.CombinedPerformance // aligned with DivisionStandings.Disciplines
	Total     int
	Complete  bool
	Consent   domain.PublicationConsent
	// OutOfCompetition mirrors store.Participant.OutOfCompetition
	// (TASK-036, DEC-016/OQ-020 investigation): true rows never hold a
	// numeric Rank (0), in provisional or final standings alike.
	OutOfCompetition bool
}

// DivisionStanding is one division's ranked list.
type DivisionStanding struct {
	CategoryCode string
	Rows         []StandingRow
}

// MeetStandings is every division's current standing plus the discipline
// columns the rows align to.
type MeetStandings struct {
	Disciplines []string // catalog codes, event order
	Divisions   []DivisionStanding
	// Final is true when this is a FINAL standings computation
	// (ResultsService.FinalStandings — UC-033 #3, DEC-016/OQ-020): a
	// division-complete rendering where a discipline missing entirely
	// leaves its athlete unranked at the bottom, rather than the
	// PROVISIONAL rank-by-partial-total behaviour Standings always
	// returns. Renderers use it to label the list accordingly.
	Final bool
}

// Standings computes the meet's PROVISIONAL per-division combined standings
// (UC-033 #2–#4; SYS-052 split presentation of a mixed-category field):
// every participant is resolved to their division by the meet's category
// scheme, scored results align to the meet's disciplines, and each division
// ranks per the series rules (domain.RankCombinedWithTieBreak) — a missing
// discipline still ranks by partial total, the live/in-progress behaviour
// UC-033 #3 keeps unchanged (DEC-016/OQ-020: the meet is ongoing, so every
// athlete is temporarily "incomplete" at some point). Renderers label this
// "provisional". See FinalStandings for the FINAL, division-complete
// rendering.
func (s *ResultsService) Standings(ctx context.Context, meetID string) (MeetStandings, error) {
	return s.standings(ctx, meetID, false)
}

// FinalStandings computes FINAL per-division combined standings (UC-033
// #3, DEC-016/OQ-020): the official TAF3 convention observed in the LV
// Langenthal Gesamtrangliste (17.05.2025) — an athlete missing a discipline
// entirely is listed unranked at the bottom instead of ranked by partial
// total, and a discipline attempted with no valid result scores the
// scoring table's NoValidAttemptFloor and still counts as present. Callers
// needing "final once the division's series is complete, else provisional"
// should call CurrentStandings instead, which applies SeriesComplete for
// them; call this directly only when final semantics are wanted
// unconditionally (e.g. an explicit "close out and finalize" action).
func (s *ResultsService) FinalStandings(ctx context.Context, meetID string) (MeetStandings, error) {
	return s.standings(ctx, meetID, true)
}

// CurrentStandings returns FINAL standings once every discipline feeding
// them has been announced (SeriesComplete) and PROVISIONAL standings
// otherwise — the single decision point every "final list" rendering
// surface (public results, PDF result lists, series-upload export) shares,
// so none of them re-implement the completeness check (TASK-036).
func (s *ResultsService) CurrentStandings(ctx context.Context, meetID string) (MeetStandings, error) {
	complete, err := s.SeriesComplete(ctx, meetID)
	if err != nil {
		return MeetStandings{}, err
	}
	if complete {
		return s.FinalStandings(ctx, meetID)
	}
	return s.Standings(ctx, meetID)
}

// SeriesComplete reports whether every one of meetID's events has had every
// round/unit's results announced (SYS-047) — the "division's series is
// complete" trigger UC-033 #3/DEC-016 gates FINAL standings on. Unlike
// disciplineUnit (the single-capturable-unit assumption template meets
// like the UKC's make), this walks every round and unit an event has, so a
// heats-based meet (TASK-018 seeding: multiple units per discipline) is
// handled correctly instead of erroring — it is complete only once every
// heat/final of every event is announced. A meet with no events, or an
// event with no rounds/units scheduled yet, is never complete. The
// event/round/unit walk is read-only and uses the pooled read connection
// (ADR-004 §9); the per-unit announcement lookup (protestState) stays on
// the writer connection with every other announcement read.
func (s *ResultsService) SeriesComplete(ctx context.Context, meetID string) (bool, error) {
	db := s.readConn()
	events, err := store.ListEvents(ctx, db, meetID)
	if err != nil {
		return false, err
	}
	if len(events) == 0 {
		return false, nil
	}
	for _, ev := range events {
		rounds, err := store.ListRounds(ctx, db, ev.ID)
		if err != nil {
			return false, err
		}
		if len(rounds) == 0 {
			return false, nil
		}
		for _, rd := range rounds {
			units, err := store.ListRoundUnits(ctx, db, rd.ID)
			if err != nil {
				return false, err
			}
			if len(units) == 0 {
				return false, nil
			}
			for _, u := range units {
				state, err := s.protestState(ctx, u.ID)
				if err != nil {
					return false, err
				}
				if !state.Announced {
					return false, nil
				}
			}
		}
	}
	return true, nil
}

// standings is the shared computation behind Standings/FinalStandings: the
// two only differ in which domain ranking rule closes out each division
// (RankCombinedWithTieBreak vs FinalRankCombined) and the floor/missing-
// discipline handling that rule implies. Applying the scoring table's
// NoValidAttemptFloor (DEC-016/OQ-020) to a present-but-no-valid-attempt
// discipline happens here for BOTH modes: it corrects the points table
// interpretation itself (a scoring fix, not a final-only presentation
// rule), so a live standings total already reflects the official "ogV"
// floor the moment that discipline's series settles. Read-only throughout:
// uses the pooled read connection (ADR-004 §9, TASK-035/OQ-066) — this is
// the query the public results page's per-meet render cache (internal/web)
// wraps, so most of its call volume is one render per result change rather
// than one per viewer, but every cache miss (and every operator/standings-
// page caller) still benefits from not convoying behind the single writer
// connection.
func (s *ResultsService) standings(ctx context.Context, meetID string, final bool) (MeetStandings, error) {
	db := s.readConn()
	meet, err := store.GetMeet(ctx, db, meetID)
	if err != nil {
		return MeetStandings{}, err
	}
	scheme, ok := s.schemes[meet.CategorySchemeID]
	if !ok {
		return MeetStandings{}, fmt.Errorf("meet %s references unknown category scheme %q", meetID, meet.CategorySchemeID)
	}
	events, err := store.ListEvents(ctx, db, meetID)
	if err != nil {
		return MeetStandings{}, err
	}
	participants, err := store.ListParticipants(ctx, db, meetID)
	if err != nil {
		return MeetStandings{}, err
	}
	results, err := store.ListMeetResults(ctx, db, meetID)
	if err != nil {
		return MeetStandings{}, err
	}

	// The equal-totals policy follows the meet's governing rule set: a
	// WA-formula combined-events meet ranks per its combined scoring
	// table's declared policy (WA TR 39: equal points is a tie — OQ-045),
	// everything else keeps the UBS Kids Cup Reglement §3 majority rule
	// RankCombined has always applied.
	tieBreak := domain.TieBreakMajorityThenHighest
	if combinedID, ok, err := store.GetMeetCombinedScoringTable(ctx, db, meetID); err != nil {
		return MeetStandings{}, err
	} else if ok {
		if table, known := s.combinedTables[combinedID]; known {
			tieBreak = table.EffectiveTieBreak()
		}
	}

	// The scoring table's NoValidAttemptFloor (DEC-016/OQ-020), if any —
	// only a plain (non-combined-events) meet's own table declares one;
	// meet.ScoringTableID is empty for a WA combined-events meet (which
	// scores through combinedID above instead), so the lookup naturally
	// resolves to 0 (no floor) for those.
	floorPoints := 0
	if table, ok := s.tables[meet.ScoringTableID]; ok {
		floorPoints = table.NoValidAttemptFloor
	}

	out := MeetStandings{Final: final}
	for _, ev := range events {
		out.Disciplines = append(out.Disciplines, ev.DisciplineCode)
	}
	discIndex := make(map[string]int, len(out.Disciplines))
	for i, d := range out.Disciplines {
		discIndex[d] = i
	}

	// One aligned performance vector per athlete.
	perAthlete := make(map[string][]domain.CombinedPerformance, len(participants))
	for _, p := range participants {
		perAthlete[p.AthleteID] = make([]domain.CombinedPerformance, len(out.Disciplines))
		for i, d := range out.Disciplines {
			perAthlete[p.AthleteID][i] = domain.CombinedPerformance{DisciplineCode: d}
		}
	}
	for _, r := range results {
		vec, ok := perAthlete[r.AthleteID]
		if !ok {
			continue // result for an unregistered athlete: not standings input
		}
		i, ok := discIndex[r.DisciplineCode]
		if !ok {
			continue
		}
		points := r.Points
		if points == nil && floorPoints > 0 && domain.AttemptedNoValidResult(r.Status) {
			// UC-033 #3, DEC-016/OQ-020: present but no valid attempt
			// ("ogV") scores the table's floor, not 0 — applied here so it
			// reaches both provisional and final totals identically.
			floor := floorPoints
			points = &floor
		}
		vec[i] = domain.CombinedPerformance{
			DisciplineCode: r.DisciplineCode,
			Mark:           r.Mark,
			Status:         r.Status,
			Points:         points,
			RecordFlags:    r.RecordFlags,
		}
	}

	// Resolve club names once.
	var clubIDs []string
	for _, p := range participants {
		clubIDs = append(clubIDs, p.Athlete.ClubIDs...)
	}
	clubNames, err := store.ClubNames(ctx, db, clubIDs)
	if err != nil {
		return MeetStandings{}, err
	}

	// Group participants into divisions per the meet's scheme (division
	// membership follows birth year and sex — UC-033 #1).
	byDivision := map[string][]StandingRow{}
	for _, p := range participants {
		cat, err := scheme.ResolveDefaultCategory(p.Athlete.BirthYear, p.Athlete.Sex, meet.StartDate)
		if err != nil {
			return MeetStandings{}, fmt.Errorf("participant %s: %w", p.ID, err)
		}
		club := ""
		if len(p.Athlete.ClubIDs) > 0 {
			club = clubNames[p.Athlete.ClubIDs[0]]
		}
		byDivision[cat.Code] = append(byDivision[cat.Code], StandingRow{
			Bib:              p.Bib,
			AthleteID:        p.AthleteID,
			FirstName:        p.Athlete.FirstName,
			LastName:         p.Athlete.LastName,
			ClubName:         club,
			BirthYear:        p.Athlete.BirthYear,
			Sex:              p.Athlete.Sex,
			Marks:            perAthlete[p.AthleteID],
			Consent:          p.Athlete.Consent,
			OutOfCompetition: p.OutOfCompetition,
		})
	}

	// Rank each division and emit them in scheme order.
	for _, cat := range scheme.Categories {
		rows, ok := byDivision[cat.Code]
		if !ok {
			continue
		}
		ranked := make([]domain.CombinedStanding, len(rows))
		rowByAthlete := make(map[string]StandingRow, len(rows))
		for i, r := range rows {
			ranked[i] = domain.CombinedStanding{
				AthleteID: r.AthleteID, Performances: r.Marks, OutOfCompetition: r.OutOfCompetition,
			}
			rowByAthlete[r.AthleteID] = r
		}
		rankFn := domain.RankCombinedWithTieBreak
		if final {
			rankFn = domain.FinalRankCombined
		}
		div := DivisionStanding{CategoryCode: cat.Code}
		for _, st := range rankFn(ranked, tieBreak) {
			row := rowByAthlete[st.AthleteID]
			row.Rank = st.Rank
			row.Total = st.Total
			row.Complete = st.Complete
			row.Marks = st.Performances
			div.Rows = append(div.Rows, row)
		}
		out.Divisions = append(out.Divisions, div)
	}
	return out, nil
}
