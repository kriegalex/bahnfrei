// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// ErrDuplicateParticipant aliases the store sentinel for web handlers.
var ErrDuplicateParticipant = store.ErrDuplicateParticipant

// ResultsService hosts participation, scored result capture and combined
// standings (UC-033 #2–#4; SYS-053/052), plus the attempt-level field and
// track capture flows on top (TASK-008, UC-010/UC-011).
type ResultsService struct {
	db        *sql.DB
	catalog   *domain.DisciplineCatalog
	schemes   map[string]*domain.CategoryScheme
	tables    map[string]*domain.ScoringTable
	templates map[string]*domain.MeetTemplate
	onChange  func(meetID string)
}

// NewResultsService wires a ResultsService; catalog, schemes, tables and
// templates are normally the built-in data.
func NewResultsService(db *sql.DB, catalog *domain.DisciplineCatalog, schemes map[string]*domain.CategoryScheme,
	tables map[string]*domain.ScoringTable, templates map[string]*domain.MeetTemplate) *ResultsService {
	return &ResultsService{db: db, catalog: catalog, schemes: schemes, tables: tables, templates: templates}
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
// plus their start number at this meet.
type ParticipantInput struct {
	FirstName string
	LastName  string
	BirthYear int
	Sex       domain.Sex
	Club      string
	Bib       string
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

// Participants lists a meet's registered athletes.
func (s *ResultsService) Participants(ctx context.Context, meetID string) ([]store.ParticipantRow, error) {
	return store.ListParticipants(ctx, s.db, meetID)
}

// ClubNamesFor resolves the club names of the given participants' club
// IDs (presentation joins for roster and start lists).
func (s *ResultsService) ClubNamesFor(ctx context.Context, rows []store.ParticipantRow) (map[string]string, error) {
	var ids []string
	for _, p := range rows {
		ids = append(ids, p.Athlete.ClubIDs...)
	}
	return store.ClubNames(ctx, s.db, ids)
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
		if result.Points, err = s.scorePoints(meet, in.DisciplineCode, in.Timing, athlete.Sex, in.Mark); err != nil {
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
// explicit gap UC-033 #3 requires.
type StandingRow struct {
	Rank      int
	Bib       string
	AthleteID string
	FirstName string
	LastName  string
	ClubName  string
	BirthYear int
	Marks     []domain.CombinedPerformance // aligned with DivisionStandings.Disciplines
	Total     int
	Complete  bool
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
}

// Standings computes the meet's per-division combined standings (UC-033
// #2–#4; SYS-052 split presentation of a mixed-category field): every
// participant is resolved to their division by the meet's category scheme,
// scored results align to the meet's disciplines, and each division ranks
// per the series rules (domain.RankCombined).
func (s *ResultsService) Standings(ctx context.Context, meetID string) (MeetStandings, error) {
	meet, err := store.GetMeet(ctx, s.db, meetID)
	if err != nil {
		return MeetStandings{}, err
	}
	scheme, ok := s.schemes[meet.CategorySchemeID]
	if !ok {
		return MeetStandings{}, fmt.Errorf("meet %s references unknown category scheme %q", meetID, meet.CategorySchemeID)
	}
	events, err := store.ListEvents(ctx, s.db, meetID)
	if err != nil {
		return MeetStandings{}, err
	}
	participants, err := store.ListParticipants(ctx, s.db, meetID)
	if err != nil {
		return MeetStandings{}, err
	}
	results, err := store.ListMeetResults(ctx, s.db, meetID)
	if err != nil {
		return MeetStandings{}, err
	}

	out := MeetStandings{}
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
		vec[i] = domain.CombinedPerformance{
			DisciplineCode: r.DisciplineCode,
			Mark:           r.Mark,
			Status:         r.Status,
			Points:         r.Points,
		}
	}

	// Resolve club names once.
	var clubIDs []string
	for _, p := range participants {
		clubIDs = append(clubIDs, p.Athlete.ClubIDs...)
	}
	clubNames, err := store.ClubNames(ctx, s.db, clubIDs)
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
			Bib:       p.Bib,
			AthleteID: p.AthleteID,
			FirstName: p.Athlete.FirstName,
			LastName:  p.Athlete.LastName,
			ClubName:  club,
			BirthYear: p.Athlete.BirthYear,
			Marks:     perAthlete[p.AthleteID],
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
			ranked[i] = domain.CombinedStanding{AthleteID: r.AthleteID, Performances: r.Marks}
			rowByAthlete[r.AthleteID] = r
		}
		div := DivisionStanding{CategoryCode: cat.Code}
		for _, st := range domain.RankCombined(ranked) {
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
