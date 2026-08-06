// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// AttemptRecord aliases the store type for the web layer (architecture.md
// §3: web never imports internal/store).
type AttemptRecord = store.AttemptRecord

// ResultRecord aliases the store type for the web layer.
type ResultRecord = store.ResultRecord

// AttemptConflictError reports a concurrent capture of the same trial: the
// caller's write did not apply and Current is what another session stored —
// both versions must be shown, never silently merged (UC-021 #2, SYS-086).
type AttemptConflictError struct {
	Current store.AttemptRecord
}

func (e *AttemptConflictError) Error() string {
	return fmt.Sprintf("trial %d was captured concurrently (stored: %s, version %d)",
		e.Current.Seq, e.Current.Display(), e.Current.Version)
}

func (e *AttemptConflictError) Unwrap() error { return ErrConflict }

// ResultConflictError reports a concurrent settled-result edit (UC-021
// #2): the caller's write did not apply and Current is what another
// session stored — both versions must be shown, never silently merged
// (SYS-083). Surfaced by SaveTrackResult only when the caller opts into
// optimistic checking via TrackResultInput.ExpectedVersion.
type ResultConflictError struct {
	Current store.ResultRecord
}

func (e *ResultConflictError) Error() string {
	return fmt.Sprintf("result was edited concurrently (stored: mark=%q status=%q, version %d)",
		e.Current.Mark, e.Current.Status, e.Current.Version)
}

func (e *ResultConflictError) Unwrap() error { return ErrConflict }

// CaptureConfig is a horizontal field event's series shape (SYS-042:
// "the configured trial count and field-cut after round 3").
type CaptureConfig struct {
	Attempts int // trials per athlete
	CutAfter int // completed rounds before the cut; 0 = no cut
	CutTo    int // field size continuing after the cut
}

// captureConfig derives a discipline's series shape: template meets carry
// it as rule data (UBS Kids Cup: 3 trials, everyone keeps jumping); other
// meets get the WA default series of 3+3 trials with a cut to the top 8
// after round 3 (D5.2; per-event overrides are TASK-018+ scope).
func (s *ResultsService) captureConfig(meet store.MeetRecord, disciplineCode string) CaptureConfig {
	if tpl, ok := s.templates[meet.TemplateID]; ok {
		for _, ev := range tpl.Events {
			if ev.DisciplineCode == disciplineCode {
				return CaptureConfig{Attempts: ev.Attempts}
			}
		}
	}
	return CaptureConfig{Attempts: 6, CutAfter: 3, CutTo: 8}
}

// unitContext is one capturable unit resolved against its meet and the
// discipline catalog — every capture entry point starts here.
type unitContext struct {
	meet  store.MeetRecord
	entry store.TimetableEntry
	disc  domain.Discipline
}

func (s *ResultsService) unitContext(ctx context.Context, meetID, unitID string) (unitContext, error) {
	meet, err := store.GetMeet(ctx, s.db, meetID)
	if err != nil {
		return unitContext{}, err
	}
	units, err := store.ListMeetUnits(ctx, s.db, meetID)
	if err != nil {
		return unitContext{}, err
	}
	for _, u := range units {
		if u.UnitID != unitID {
			continue
		}
		disc, ok := s.catalog.ByCode(u.DisciplineCode)
		if !ok {
			return unitContext{}, fmt.Errorf("unit %s: discipline %q is not in the catalog", unitID, u.DisciplineCode)
		}
		return unitContext{meet: meet, entry: u, disc: disc}, nil
	}
	return unitContext{}, fmt.Errorf("unit %s: %w", unitID, store.ErrNotFound)
}

// ErrUnitNotAssigned means the acting field official is not scoped to this
// event unit (SYS-090's "field/event official (scoped to assigned
// events)"; UC-022 #1).
var ErrUnitNotAssigned = errors.New("field official is not assigned to this event unit")

// authorizeCaptureAccess enforces SYS-090's per-event scoping on top of the
// coarse CapCaptureResults role check: an account with exactly
// RoleFieldOfficial may only act on a unit explicitly assigned to it for
// this meet (server-side, not just hidden in the UI — UC-022 #1);
// competition office and above are not scoped by unit (they already carry
// broader capabilities SYS-090 does not limit per event). A denied attempt
// is itself recorded in the audit trail (UC-022 #1: "access is denied and
// the attempt logged").
func (s *ResultsService) authorizeCaptureAccess(ctx context.Context, actor Session, meetID, unitID string) error {
	if err := Authorize(actor.Role, CapCaptureResults); err != nil {
		return err
	}
	if actor.Role != RoleFieldOfficial {
		return nil
	}
	ok, err := store.IsFieldOfficialAssigned(ctx, s.db, actor.AccountID, unitID)
	if err != nil {
		return err
	}
	if !ok {
		s.auditDenied(ctx, actor, "capture.access_denied", "unit", unitID,
			fmt.Sprintf("field official %s is not assigned to unit %s (meet %s)", actor.AccountID, unitID, meetID))
		return ErrUnitNotAssigned
	}
	return nil
}

// CheckUnitAccess is the exported form of authorizeCaptureAccess for the web
// layer to gate a unit's read-only capture views before rendering them
// (UC-022 #1 covers both read and write access to an unassigned unit).
func (s *ResultsService) CheckUnitAccess(ctx context.Context, actor Session, meetID, unitID string) error {
	return s.authorizeCaptureAccess(ctx, actor, meetID, unitID)
}

// auditDenied best-effort records a denied privileged/scoped action
// (UC-022 #1). It never fails the caller's deny path: the primary
// ErrUnitNotAssigned/ErrForbidden return already communicates the denial,
// so a failure writing this secondary log entry must not mask it.
func (s *ResultsService) auditDenied(ctx context.Context, actor Session, action, entityType, entityID, reason string) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: action, EntityType: entityType, EntityID: entityID, Reason: reason,
	}); err != nil {
		return
	}
	_ = tx.Commit()
}

// participant returns the meet participant for athleteID — capture only
// accepts marks for registered athletes.
func (s *ResultsService) participant(ctx context.Context, meetID, athleteID string) (store.ParticipantRow, error) {
	rows, err := store.ListParticipants(ctx, s.db, meetID)
	if err != nil {
		return store.ParticipantRow{}, err
	}
	for _, p := range rows {
		if p.AthleteID == athleteID {
			return p, nil
		}
	}
	return store.ParticipantRow{}, fmt.Errorf("athlete %s is not registered for meet %s: %w", athleteID, meetID, store.ErrNotFound)
}

// CaptureUnit is one line of the capture index: a unit an official can
// open for result entry.
type CaptureUnit struct {
	UnitID         string
	DisciplineCode string
	DisciplineName string
	Family         domain.DisciplineFamily
	// ScheduledAt and Location carry the unit's timetable placement
	// (TASK-004/SYS-004 scheduling) through to the field-official "my
	// assignments" panel (TASK-046, SYS-151/UC-041 #2) so a volunteer's
	// first two questions — when, where — are answered without a second
	// page. Nil/"" when the unit has not been scheduled yet.
	ScheduledAt *time.Time
	Location    string
}

// CaptureUnits lists a meet's capturable units — track, horizontal and
// vertical field disciplines (TASK-021); relays are a later slice
// (TASK-016). For an account with exactly RoleFieldOfficial the list is
// filtered to units it is assigned to for this meet (SYS-090 per-event
// scoping, UC-022 #1); every other authorized role sees every capturable
// unit, matching authorizeCaptureAccess's scoping rule.
func (s *ResultsService) CaptureUnits(ctx context.Context, actor Session, meetID string) ([]CaptureUnit, error) {
	units, err := store.ListMeetUnits(ctx, s.db, meetID)
	if err != nil {
		return nil, err
	}
	var assigned map[string]bool
	if actor.Role == RoleFieldOfficial {
		assigned, err = store.AssignedUnitIDs(ctx, s.db, actor.AccountID, meetID)
		if err != nil {
			return nil, err
		}
	}
	var out []CaptureUnit
	for _, u := range units {
		disc, ok := s.catalog.ByCode(u.DisciplineCode)
		if !ok {
			continue
		}
		if disc.Family != domain.FamilyTrack && disc.Family != domain.FamilyFieldHorizontal && disc.Family != domain.FamilyFieldVertical {
			continue
		}
		if actor.Role == RoleFieldOfficial && !assigned[u.UnitID] {
			continue
		}
		out = append(out, CaptureUnit{
			UnitID:         u.UnitID,
			DisciplineCode: disc.Code,
			DisciplineName: disc.Name,
			Family:         disc.Family,
			ScheduledAt:    u.ScheduledAt,
			Location:       u.Location,
		})
	}
	return out, nil
}

// AssignedMeet is one meet a field-official actor holds capture
// assignments in, with only the units it is scoped to there (TASK-042,
// DEC-025 "my assignments" dashboard).
type AssignedMeet struct {
	MeetID   string
	MeetName string
	Units    []CaptureUnit
}

// AssignedMeets lists the meets actor (a field official) holds a current
// capture assignment in, each with only the units it is scoped to there
// (TASK-042, DEC-025) — the field-official panel of the "my assignments"
// dashboard, replacing the out-of-band link an organizer used to share
// (OQ-089). Restricted to exactly RoleFieldOfficial: SYS-090 per-event
// scoping applies only to that role, so any other authorized caller gets
// an empty list here (it has its own dashboard panel instead — OQ-110).
func (s *ResultsService) AssignedMeets(ctx context.Context, actor Session) ([]AssignedMeet, error) {
	if err := Authorize(actor.Role, CapCaptureResults); err != nil {
		return nil, err
	}
	if actor.Role != RoleFieldOfficial {
		return nil, nil
	}
	meetIDs, err := store.AssignedMeetIDs(ctx, s.readConn(), actor.AccountID)
	if err != nil {
		return nil, err
	}
	out := make([]AssignedMeet, 0, len(meetIDs))
	for _, meetID := range meetIDs {
		meet, err := store.GetMeet(ctx, s.readConn(), meetID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			return nil, err
		}
		units, err := s.CaptureUnits(ctx, actor, meetID)
		if err != nil {
			return nil, err
		}
		if len(units) == 0 {
			continue
		}
		out = append(out, AssignedMeet{MeetID: meet.ID, MeetName: meet.Name, Units: units})
	}
	return out, nil
}

// CaptureRow is one athlete's line in the capture grid: identity columns,
// the attempt series (index i = trial i+1; nil = not captured) and the
// settled result.
type CaptureRow struct {
	AthleteID string
	Bib       string
	FirstName string
	LastName  string
	ClubName  string
	BirthYear int
	Attempts  []*store.AttemptRecord
	Result    *store.ResultRecord
	// Lane is the athlete's drawn lane for this unit, if heat seeding
	// (TASK-018, SYS-026/027) assigned one; 0 = no lane assigned (a
	// non-laned event, or the unit has not been seeded yet). Read-only
	// display context — never written back here, so it carries no bearing
	// on capture-attempt versioning or the offline sync protocol
	// (internal/sync/doc.go).
	Lane int
}

// UnitStandingRow is one line of a unit's current ranking (UC-011 #4).
type UnitStandingRow struct {
	Rank         int // 0 = unranked (no valid mark yet)
	AthleteID    string
	Mark         string
	Status       domain.QualificationStatus
	StatusDetail string
	Timing       domain.Timing
	Points       *int
	// RecordFlags carries the SYS-049 record/best flags (e.g. "MR", "PB")
	// through to every operator-view renderer built on UnitStandingRow.
	RecordFlags []string
}

// UnitCaptureView is everything the capture page for one unit renders.
type UnitCaptureView struct {
	Meet           store.MeetRecord
	UnitID         string
	DisciplineCode string
	DisciplineName string
	Family         domain.DisciplineFamily
	WindRelevant   bool
	Config         CaptureConfig
	Rows           []CaptureRow
	Standings      []UnitStandingRow
	// Continuation is the rule-correct competing order for the trials after
	// the field cut, once every athlete has CutAfter attempts (SYS-042);
	// empty when the unit has no cut or the cut round is still open.
	Continuation []string
	// CategorySplits re-ranks this same unit's entrants per category (SYS-052,
	// UC-028 #1/#2: "combining categories/divisions for lack of entries"): one
	// group per category present among the unit's entrants, each ranked
	// exactly like Standings but restricted to that group. A unit whose
	// entrants are all one category still gets one (degenerate) group here —
	// callers only need to render a "per category" section when
	// len(CategorySplits) > 1.
	CategorySplits []CategorySplitStanding
}

// CategorySplitStanding is one category group's re-ranked view of a unit
// whose field combines more than one age category/division (SYS-052).
type CategorySplitStanding struct {
	CategoryCode string
	Standings    []UnitStandingRow
}

// UnitCapture assembles the capture view for one unit: the participant
// grid with every captured attempt, the settled results, and the current
// standings (UC-011 #4).
func (s *ResultsService) UnitCapture(ctx context.Context, meetID, unitID string) (UnitCaptureView, error) {
	uc, err := s.unitContext(ctx, meetID, unitID)
	if err != nil {
		return UnitCaptureView{}, err
	}
	participants, err := store.ListParticipants(ctx, s.db, meetID)
	if err != nil {
		return UnitCaptureView{}, err
	}
	clubs, err := s.ClubNamesFor(ctx, participants)
	if err != nil {
		return UnitCaptureView{}, err
	}
	attempts, err := store.ListUnitAttempts(ctx, s.db, unitID)
	if err != nil {
		return UnitCaptureView{}, err
	}
	results, err := store.ListUnitResults(ctx, s.db, unitID)
	if err != nil {
		return UnitCaptureView{}, err
	}
	lanes, err := s.laneByAthlete(ctx, unitID)
	if err != nil {
		return UnitCaptureView{}, err
	}

	v := UnitCaptureView{
		Meet:           uc.meet,
		UnitID:         unitID,
		DisciplineCode: uc.disc.Code,
		DisciplineName: uc.disc.Name,
		Family:         uc.disc.Family,
		WindRelevant:   uc.disc.WindRelevant,
		Config:         s.captureConfig(uc.meet, uc.disc.Code),
	}

	byAthleteAttempts := map[string][]*store.AttemptRecord{}
	for i := range attempts {
		a := &attempts[i]
		byAthleteAttempts[a.AthleteID] = append(byAthleteAttempts[a.AthleteID], a)
	}
	byAthleteResult := map[string]*store.ResultRecord{}
	for i := range results {
		byAthleteResult[results[i].AthleteID] = &results[i]
	}
	// catByAthlete resolves each participant's default category (SYS-052,
	// UC-028) up front, alongside the grid build below, so the split-by-
	// category standings need no second participant query. A meet
	// referencing an unknown scheme (should not happen once created) simply
	// gets no split — the combined Standings below is unaffected.
	scheme, hasScheme := s.schemes[uc.meet.CategorySchemeID]
	catByAthlete := make(map[string]string, len(participants))
	if hasScheme {
		for _, p := range participants {
			if cat, err := scheme.ResolveDefaultCategory(p.Athlete.BirthYear, p.Athlete.Sex, uc.meet.StartDate); err == nil {
				catByAthlete[p.AthleteID] = cat.Code
			}
		}
	}

	for _, p := range participants {
		row := CaptureRow{
			AthleteID: p.AthleteID,
			Bib:       p.Bib,
			FirstName: p.Athlete.FirstName,
			LastName:  p.Athlete.LastName,
			BirthYear: p.Athlete.BirthYear,
			Attempts:  make([]*store.AttemptRecord, v.Config.Attempts),
			Result:    byAthleteResult[p.AthleteID],
			Lane:      lanes[p.AthleteID],
		}
		if len(p.Athlete.ClubIDs) > 0 {
			row.ClubName = clubs[p.Athlete.ClubIDs[0]]
		}
		for _, a := range byAthleteAttempts[p.AthleteID] {
			if a.Seq >= 1 && a.Seq <= v.Config.Attempts {
				row.Attempts[a.Seq-1] = a
			}
		}
		v.Rows = append(v.Rows, row)
	}

	if uc.disc.Family == domain.FamilyFieldHorizontal {
		v.Standings, v.Continuation = fieldStandings(v.Rows, byAthleteResult, v.Config)
	} else {
		v.Standings = trackStandings(results)
	}
	if hasScheme {
		v.CategorySplits = splitStandingsByCategory(scheme, catByAthlete, v.Rows, byAthleteResult, uc.disc.Family, v.Config)
	}
	return v, nil
}

// splitStandingsByCategory re-ranks rows per category (SYS-052, UC-028
// #1/#2): the same fieldStandings/trackStandings ranking functions the
// combined-unit standings use, each re-run on the subset of rows one
// category's entrants make up, in the meet's category-scheme order (so
// output order is deterministic and matches the scheme's own age-band
// ordering, not row-arrival order).
func splitStandingsByCategory(scheme *domain.CategoryScheme, catByAthlete map[string]string, rows []CaptureRow,
	byAthleteResult map[string]*store.ResultRecord, family domain.DisciplineFamily, cfg CaptureConfig) []CategorySplitStanding {
	byCategory := map[string][]CaptureRow{}
	for _, row := range rows {
		code, ok := catByAthlete[row.AthleteID]
		if !ok {
			continue // no resolvable category for this athlete: excluded from every split group
		}
		byCategory[code] = append(byCategory[code], row)
	}

	var out []CategorySplitStanding
	for _, cat := range scheme.Categories {
		group, ok := byCategory[cat.Code]
		if !ok {
			continue
		}
		var standings []UnitStandingRow
		if family == domain.FamilyFieldHorizontal {
			standings, _ = fieldStandings(group, byAthleteResult, cfg)
		} else {
			recs := make([]store.ResultRecord, 0, len(group))
			for _, row := range group {
				if row.Result != nil {
					recs = append(recs, *row.Result)
				}
			}
			standings = trackStandings(recs)
		}
		out = append(out, CategorySplitStanding{CategoryCode: cat.Code, Standings: standings})
	}
	return out
}

// laneByAthlete resolves a unit's drawn lanes (TASK-018, SYS-026/027),
// keyed by athlete ID, for the capture grid's read-only lane column — a
// unit that has not been seeded yet (or a non-laned event) yields an empty
// map, so every CaptureRow's Lane simply stays 0.
func (s *ResultsService) laneByAthlete(ctx context.Context, unitID string) (map[string]int, error) {
	assignments, err := store.ListUnitAssignments(ctx, s.db, unitID)
	if err != nil {
		return nil, err
	}
	if len(assignments) == 0 {
		return nil, nil
	}
	lanes := make(map[string]int, len(assignments))
	for _, a := range assignments {
		if a.Lane == 0 {
			continue
		}
		entry, err := store.GetEntry(ctx, s.db, a.EntryID)
		if err != nil {
			return nil, err
		}
		if entry.AthleteID != "" {
			lanes[entry.AthleteID] = a.Lane
		}
	}
	return lanes, nil
}

// fieldStandings ranks the field by best mark with next-best tie-breaking
// (SYS-042) and, once every athlete on the start list has taken the
// pre-cut trials, the continuation order for the remaining rounds.
//
// Ranking and continuation both start from the raw attempt series, but only
// continuation stays on it: a settled-level correction (UC-015 #2,
// TASK-040/DEC-024) amends the `results` row directly without rewriting the
// attempt series it was derived from (OQ-036), so ranking off attempts
// alone would keep showing the pre-correction mark/status forever. The
// ranking basis (rankSeries) is reconciled with the settled result whenever
// it has moved past what the raw attempts alone would produce — a mark
// correction becomes the athlete's sole ranking input, a status correction
// (DNS/DNF/DQ/NM) drops them to unranked with the corrected status — so
// UC-015 #2's "placings ... reflect the change" holds for the field grid's
// own live standings, not just the meet-wide CurrentStandings (which
// already reads `results` directly and needs no such reconciliation). The
// cut/continuation computation deliberately keeps reading the untouched raw
// series: SYS-042's cut is a mid-competition, pre-announcement concept, and
// OQ-036 already scopes corrections to already-announced (capture-closed)
// units.
func fieldStandings(rows []CaptureRow, results map[string]*store.ResultRecord, cfg CaptureConfig) ([]UnitStandingRow, []string) {
	var series, rankSeries []domain.FieldSeries
	overrideStatus := map[string]domain.QualificationStatus{}
	cutRoundComplete := cfg.CutAfter > 0 && len(rows) > 0
	for _, row := range rows {
		fs := domain.FieldSeries{AthleteID: row.AthleteID}
		taken := 0
		for _, a := range row.Attempts {
			if a == nil {
				continue
			}
			taken++
			fs.Attempts = append(fs.Attempts, a.Attempt)
		}
		if cfg.CutAfter > 0 && taken < cfg.CutAfter {
			cutRoundComplete = false
		}
		series = append(series, fs)

		rfs := fs
		if r := results[row.AthleteID]; r != nil {
			switch {
			case r.Status != domain.StatusNone && r.Status != fs.Status():
				rfs.Attempts = nil
				overrideStatus[row.AthleteID] = r.Status
			case r.Mark != "":
				if best, ok := fs.Best(); !ok || best != r.Mark {
					rfs.Attempts = []domain.Attempt{{Seq: 1, Kind: domain.AttemptValid, Mark: r.Mark}}
				}
			}
		}
		rankSeries = append(rankSeries, rfs)
	}

	var out []UnitStandingRow
	for _, st := range domain.RankFieldSeries(rankSeries) {
		status := st.Status
		if s, ok := overrideStatus[st.AthleteID]; ok {
			status = s
		}
		row := UnitStandingRow{
			Rank:      st.Rank,
			AthleteID: st.AthleteID,
			Mark:      st.Best,
			Status:    status,
		}
		if r := results[st.AthleteID]; r != nil {
			row.Points = r.Points
			row.StatusDetail = r.StatusDetail
			row.RecordFlags = r.RecordFlags
		}
		out = append(out, row)
	}
	if !cutRoundComplete {
		return out, nil
	}
	return out, domain.FieldContinuation(series, cfg.CutTo)
}

// trackStandings orders a track unit's settled results: ranked marks first
// (lower time is better, ties share a rank), status-only rows after.
func trackStandings(results []store.ResultRecord) []UnitStandingRow {
	type row struct {
		standing UnitStandingRow
		centi    int64
	}
	var ranked, unranked []row
	for _, r := range results {
		st := row{standing: UnitStandingRow{
			AthleteID:    r.AthleteID,
			Mark:         r.Mark,
			Status:       r.Status,
			StatusDetail: r.StatusDetail,
			Timing:       r.Timing,
			Points:       r.Points,
			RecordFlags:  r.RecordFlags,
		}}
		if r.Mark == "" || r.Status != domain.StatusNone {
			unranked = append(unranked, st)
			continue
		}
		centi, err := domain.ParseCentiMark(r.Mark)
		if err != nil {
			unranked = append(unranked, st)
			continue
		}
		st.centi = centi
		ranked = append(ranked, st)
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].centi != ranked[j].centi {
			return ranked[i].centi < ranked[j].centi
		}
		return ranked[i].standing.AthleteID < ranked[j].standing.AthleteID
	})
	sort.SliceStable(unranked, func(i, j int) bool {
		return unranked[i].standing.AthleteID < unranked[j].standing.AthleteID
	})

	var out []UnitStandingRow
	for i, r := range ranked {
		r.standing.Rank = i + 1
		if i > 0 && ranked[i-1].centi == r.centi {
			r.standing.Rank = out[i-1].Rank
		}
		out = append(out, r.standing)
	}
	for _, r := range unranked {
		out = append(out, r.standing)
	}
	return out
}

// FieldAttemptInput is one trial captured in the horizontal-attempt grid.
type FieldAttemptInput struct {
	AthleteID string
	Seq       int
	Kind      domain.AttemptKind
	Mark      string
	Wind      *float64
	// ExpectedVersion is the attempt version the capture form was rendered
	// from; 0 records a new trial. A mismatch is an AttemptConflictError.
	ExpectedVersion int64
}

// SaveFieldAttempt captures one trial of a horizontal field event
// (UC-011, SYS-042): the attempt is validated and stored, and the
// athlete's settled unit result (best mark, series status, points where
// the meet scores) is recomputed from the full series in the same
// transaction, so standings are never stale relative to attempts.
func (s *ResultsService) SaveFieldAttempt(ctx context.Context, actor Session, meetID, unitID string, in FieldAttemptInput) (store.AttemptRecord, error) {
	if err := s.authorizeCaptureAccess(ctx, actor, meetID, unitID); err != nil {
		return store.AttemptRecord{}, err
	}
	uc, err := s.unitContext(ctx, meetID, unitID)
	if err != nil {
		return store.AttemptRecord{}, err
	}
	if uc.disc.Family != domain.FamilyFieldHorizontal {
		return store.AttemptRecord{}, fmt.Errorf("unit %s is %s, not a horizontal field event", unitID, uc.disc.Family)
	}
	p, err := s.participant(ctx, meetID, in.AthleteID)
	if err != nil {
		return store.AttemptRecord{}, err
	}
	cfg := s.captureConfig(uc.meet, uc.disc.Code)
	attempt := domain.Attempt{Seq: in.Seq, Kind: in.Kind, Mark: in.Mark, Wind: in.Wind}
	if err := attempt.Validate(uc.disc.WindRelevant); err != nil {
		return store.AttemptRecord{}, err
	}
	if attempt.Seq > cfg.Attempts {
		return store.AttemptRecord{}, fmt.Errorf("%w: trial %d exceeds the %d-trial series (SYS-042)", domain.ErrInvalidMark, attempt.Seq, cfg.Attempts)
	}
	// Once the unit's results are announced (SYS-047), further edits are
	// corrections (UC-015 #2/#3), not plain capture — see CorrectResult.
	if err := s.requireNotAnnounced(ctx, unitID); err != nil {
		return store.AttemptRecord{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.AttemptRecord{}, fmt.Errorf("save attempt: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var beforeJSON []byte
	if prev, err := store.GetAttempt(ctx, tx, unitID, in.AthleteID, attempt.Seq); err == nil {
		beforeJSON, _ = json.Marshal(prev)
	} else if !errors.Is(err, store.ErrNotFound) {
		return store.AttemptRecord{}, err
	}

	rec, err := store.SaveAttempt(ctx, tx, unitID, in.AthleteID, attempt, in.ExpectedVersion)
	if err != nil {
		if errors.Is(err, store.ErrVersionConflict) {
			return rec, &AttemptConflictError{Current: rec}
		}
		return store.AttemptRecord{}, err
	}

	// Settle the athlete's unit result from the full series.
	all, err := store.ListUnitAttempts(ctx, tx, unitID)
	if err != nil {
		return store.AttemptRecord{}, err
	}
	series := domain.FieldSeries{AthleteID: in.AthleteID}
	for _, a := range all {
		if a.AthleteID == in.AthleteID {
			series.Attempts = append(series.Attempts, a.Attempt)
		}
	}
	result := domain.Result{UnitID: unitID, AthleteID: in.AthleteID, Status: series.Status()}
	if best, ok := series.Best(); ok {
		result.Mark = best
		if result.Points, err = s.scorePoints(ctx, tx, uc.meet, uc.disc.Code, domain.TimingNone, p.Athlete.Sex, best); err != nil {
			return store.AttemptRecord{}, err
		}
		// Record/best flagging (SYS-049/050) evaluates against the specific
		// trial that produced the settled best mark — its own wind reading,
		// not any other trial's (a wind-assisted best cannot borrow a legal
		// reading from a different attempt).
		var bestWind *float64
		for _, a := range series.Attempts {
			if a.Kind == domain.AttemptValid && a.Mark == best {
				bestWind = a.Wind
				break
			}
		}
		eval, err := s.evaluateRecord(ctx, tx, uc.meet, uc.disc, p.Athlete, best, domain.TimingNone, bestWind, unitID)
		if err != nil {
			return store.AttemptRecord{}, err
		}
		result.RecordFlags = eval.Flags()
	}
	settled, err := store.SaveResult(ctx, tx, result, domain.TimingNone)
	if err != nil {
		return store.AttemptRecord{}, err
	}

	afterJSON, _ := json.Marshal(rec)
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "attempt.save",
		EntityType: "attempt", EntityID: rec.ID,
		Before: string(beforeJSON), After: string(afterJSON),
	}); err != nil {
		return store.AttemptRecord{}, fmt.Errorf("audit attempt save: %w", err)
	}
	settledJSON, _ := json.Marshal(map[string]any{
		"mark": settled.Mark, "status": settled.Status, "points": settled.Points,
	})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "result.settle",
		EntityType: "result", EntityID: settled.ID, After: string(settledJSON),
	}); err != nil {
		return store.AttemptRecord{}, fmt.Errorf("audit result settle: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return store.AttemptRecord{}, fmt.Errorf("save attempt: %w", err)
	}
	s.notifyChanged(meetID)
	return rec, nil
}

// TrackResultInput is one athlete's captured track outcome: a time (with
// its timing method — the provenance SYS-041 keeps distinguishable) or a
// CR 25 status.
type TrackResultInput struct {
	AthleteID    string
	Time         string
	Timing       domain.Timing
	Status       domain.QualificationStatus
	StatusDetail string
	// ExpectedVersion enables optimistic-concurrency checking (SYS-083,
	// UC-021 #2) instead of the default blind upsert: nil preserves the
	// original "capture flows re-submit as marks are corrected" semantics
	// every pre-existing call site relies on (repeated in-session
	// corrections before announcement never conflict with themselves); a
	// non-nil value requires the stored result — if any — to still be at
	// that version, returning *ResultConflictError with both the caller's
	// attempted values and the currently stored row otherwise. Two
	// concurrent office sessions racing to save the same athlete's same
	// result is the scenario this exists for.
	ExpectedVersion *int64
}

// SaveTrackResult captures one track result on a unit (UC-010 subset for
// the UKC 60 m): manual times are rounded up to the next 0.1 s and marked
// hand-timed (SYS-041), electronic times keep 0.01 s resolution (SYS-040),
// statuses are restricted to the CR 25 capture vocabulary and a DQ
// requires its rule reference (SYS-045).
func (s *ResultsService) SaveTrackResult(ctx context.Context, actor Session, meetID, unitID string, in TrackResultInput) (store.ResultRecord, error) {
	if err := s.authorizeCaptureAccess(ctx, actor, meetID, unitID); err != nil {
		return store.ResultRecord{}, err
	}
	uc, err := s.unitContext(ctx, meetID, unitID)
	if err != nil {
		return store.ResultRecord{}, err
	}
	if uc.disc.Family != domain.FamilyTrack {
		return store.ResultRecord{}, fmt.Errorf("unit %s is %s, not a track event", unitID, uc.disc.Family)
	}
	p, err := s.participant(ctx, meetID, in.AthleteID)
	if err != nil {
		return store.ResultRecord{}, err
	}
	// Once the unit's results are announced (SYS-047), further edits are
	// corrections (UC-015 #2/#3), not plain capture — see CorrectResult.
	if err := s.requireNotAnnounced(ctx, unitID); err != nil {
		return store.ResultRecord{}, err
	}

	result := domain.Result{
		UnitID:       unitID,
		AthleteID:    in.AthleteID,
		Status:       in.Status,
		StatusDetail: in.StatusDetail,
	}
	// Wind is a per-race reading (SYS-040), never entered per athlete: every
	// mark captured on a wind-relevant unit carries the unit's current
	// reading (UC-010 #4), set via SetUnitWind — nil until the office/field
	// official records it.
	if uc.disc.WindRelevant {
		if result.Wind, err = store.GetUnitWind(ctx, s.db, unitID); err != nil {
			return store.ResultRecord{}, err
		}
	}
	if lanes, err := s.laneByAthlete(ctx, unitID); err != nil {
		return store.ResultRecord{}, err
	} else if lane, ok := lanes[in.AthleteID]; ok {
		result.Lane = lane
	}
	timing := domain.TimingNone
	if in.Status != domain.StatusNone {
		if err := domain.ValidateCaptureStatus(in.Status, in.StatusDetail); err != nil {
			return store.ResultRecord{}, err
		}
	} else {
		switch in.Timing {
		case domain.TimingManual:
			if result.Mark, err = domain.RoundUpHandTime(in.Time); err != nil {
				return store.ResultRecord{}, err
			}
		case domain.TimingElectronic:
			if result.Mark, err = domain.ValidateFATTime(in.Time); err != nil {
				return store.ResultRecord{}, err
			}
		default:
			return store.ResultRecord{}, fmt.Errorf("a timing method is required for a time (SYS-041)")
		}
		timing = in.Timing
		if result.Points, err = s.scorePoints(ctx, s.db, uc.meet, uc.disc.Code, timing, p.Athlete.Sex, result.Mark); err != nil {
			return store.ResultRecord{}, err
		}
		// Record/best flagging (SYS-049/050/051, UC-016): a status-only
		// result (DNS/DNF/DQ/NM, the other branch) never flags — no mark to
		// evaluate.
		eval, err := s.evaluateRecord(ctx, s.db, uc.meet, uc.disc, p.Athlete, result.Mark, timing, result.Wind, unitID)
		if err != nil {
			return store.ResultRecord{}, err
		}
		result.RecordFlags = eval.Flags()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.ResultRecord{}, fmt.Errorf("save track result: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var rec store.ResultRecord
	if in.ExpectedVersion != nil {
		rec, err = store.SaveResultOptimistic(ctx, tx, result, timing, "manual", *in.ExpectedVersion)
		if err != nil {
			if errors.Is(err, store.ErrVersionConflict) {
				return rec, &ResultConflictError{Current: rec}
			}
			return store.ResultRecord{}, err
		}
	} else {
		rec, err = store.SaveResult(ctx, tx, result, timing)
		if err != nil {
			return store.ResultRecord{}, err
		}
	}
	after, _ := json.Marshal(map[string]any{
		"mark": rec.Mark, "timing": timing, "status": in.Status,
		"statusDetail": in.StatusDetail, "points": rec.Points,
	})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "result.capture",
		EntityType: "result", EntityID: rec.ID, After: string(after),
	}); err != nil {
		return store.ResultRecord{}, fmt.Errorf("audit track capture: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return store.ResultRecord{}, fmt.Errorf("save track result: %w", err)
	}
	s.notifyChanged(meetID)
	return rec, nil
}

// --- Bulk "mark remaining as DNS" (TASK-041, DEC-025/OQ-070): the second
// real SYS-114 bulk operation, after check-in's close-check-in (checkin.go).
// Scoped to still-open TRACK units (UC-010's trackForm — the gap OQ-070
// named): every entrant with no captured result yet becomes DNS in one
// audited action, office-gated like AnnounceUnitResults, going through the
// TASK-034 confirm sub-page (internal/web/confirm.go) rather than a bare
// POST. Deliberately NOT wired into the TASK-009 offline capture queue
// (internal/sync/doc.go): that protocol only ever carries attempt-level ops
// for horizontal/vertical field units (capture-offline.js is only loaded
// for v.IsField in capture.templ) — track results are always a synchronous,
// online, server-rendered save, so a bulk server-side result write here has
// no capture-island optimistic-version state to race with. ---

// ErrBulkDNSTrackOnly means the bulk "mark remaining as DNS" action was
// invoked on a non-track unit: it is scoped to UC-010 track result entry
// (OQ-070) — horizontal/vertical field events are per-attempt series, not a
// single per-athlete status, so "no captured result yet" has no equivalent
// bulk-safe meaning there.
var ErrBulkDNSTrackOnly = errors.New("bulk 'mark remaining as DNS' is scoped to track units (SYS-114, UC-010)")

// requireTrackUnit is the family guard BulkDNSCandidateCount and
// BulkMarkRemainingDNS share.
func requireTrackUnit(uc unitContext) error {
	if uc.disc.Family != domain.FamilyTrack {
		return ErrBulkDNSTrackOnly
	}
	return nil
}

// unresultedTrackEntries resolves the athlete IDs of a track unit's
// entrants (the same population UnitCapture's Rows renders — every meet
// participant, TASK-008's PoC-scale roster model) that have no settled
// result yet, in roster order, alongside their drawn lanes (SYS-026/027)
// for the result rows BulkMarkRemainingDNS writes.
func (s *ResultsService) unresultedTrackEntries(ctx context.Context, meetID, unitID string) ([]string, map[string]int, error) {
	participants, err := store.ListParticipants(ctx, s.db, meetID)
	if err != nil {
		return nil, nil, err
	}
	results, err := store.ListUnitResults(ctx, s.db, unitID)
	if err != nil {
		return nil, nil, err
	}
	resulted := make(map[string]bool, len(results))
	for _, r := range results {
		resulted[r.AthleteID] = true
	}
	lanes, err := s.laneByAthlete(ctx, unitID)
	if err != nil {
		return nil, nil, err
	}
	var ids []string
	for _, p := range participants {
		if !resulted[p.AthleteID] {
			ids = append(ids, p.AthleteID)
		}
	}
	return ids, lanes, nil
}

// BulkDNSCandidateCount reports how many of a still-open track unit's
// entrants have no captured result yet — the TASK-034 confirm sub-page's
// preview count, read-only (no write, no audit).
func (s *ResultsService) BulkDNSCandidateCount(ctx context.Context, actor Session, meetID, unitID string) (int, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return 0, err
	}
	uc, err := s.unitContext(ctx, meetID, unitID)
	if err != nil {
		return 0, err
	}
	if err := requireTrackUnit(uc); err != nil {
		return 0, err
	}
	if err := s.requireNotAnnounced(ctx, unitID); err != nil {
		return 0, err
	}
	ids, _, err := s.unresultedTrackEntries(ctx, meetID, unitID)
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}

// BulkMarkRemainingDNS marks every entrant of a still-open track unit that
// has no captured result yet as DNS, in one audited office action
// (TASK-041, DEC-025/OQ-070, SYS-114/SYS-046): the second real SYS-114 bulk
// operation, mirroring CloseCheckIn's per-entry audit shape. Entries that
// already carry a result (any status, not just a valid mark) are left
// untouched. Returns the number of entries marked DNS.
func (s *ResultsService) BulkMarkRemainingDNS(ctx context.Context, actor Session, meetID, unitID string) (int, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return 0, err
	}
	uc, err := s.unitContext(ctx, meetID, unitID)
	if err != nil {
		return 0, err
	}
	if err := requireTrackUnit(uc); err != nil {
		return 0, err
	}
	if err := s.requireNotAnnounced(ctx, unitID); err != nil {
		return 0, err
	}
	ids, lanes, err := s.unresultedTrackEntries(ctx, meetID, unitID)
	if err != nil {
		return 0, err
	}

	n := 0
	for _, athleteID := range ids {
		applied, err := s.bulkDNSOne(ctx, actor, unitID, athleteID, lanes[athleteID])
		if err != nil {
			return n, err
		}
		if applied {
			n++
		}
	}
	if n > 0 {
		s.notifyChanged(meetID)
	}
	return n, nil
}

// bulkDNSOne inserts one DNS result for an entrant with no result yet.
// Insert-only (SaveResultOptimistic's expectedVersion 0): a result captured
// concurrently between BulkMarkRemainingDNS's snapshot and this write —
// another operator saving that same athlete's real time/status at the same
// moment — wins the race; this write reports applied=false (not an error)
// and the entry is correctly left with its real result, matching "entries
// with results untouched" rather than clobbering a concurrent capture.
func (s *ResultsService) bulkDNSOne(ctx context.Context, actor Session, unitID, athleteID string, lane int) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("capture.bulk_dns: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result := domain.Result{UnitID: unitID, AthleteID: athleteID, Status: domain.StatusDNS, Lane: lane}
	rec, err := store.SaveResultOptimistic(ctx, tx, result, domain.TimingNone, "manual", 0)
	if err != nil {
		if errors.Is(err, store.ErrVersionConflict) {
			return false, nil
		}
		return false, err
	}
	after, _ := json.Marshal(map[string]string{"status": string(domain.StatusDNS)})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "capture.bulk_dns",
		EntityType: "result", EntityID: rec.ID, After: string(after),
	}); err != nil {
		return false, fmt.Errorf("audit capture.bulk_dns: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("capture.bulk_dns: %w", err)
	}
	return true, nil
}

// scorePoints scores a mark against the meet's scoring table, if it has
// one (UC-033 #2); nil means the meet does not score points. A meet
// configured with a WA combined-events formula table (SYS-044, TASK-021:
// store.GetMeetCombinedScoringTable, set at creation from a
// wa-decathlon/wa-heptathlon-style template) scores through that table
// instead — the two are mutually exclusive per meet (0014_combined_scoring
// .sql explains why). db must be the same handle (s.db, or the caller's
// open tx) the caller is already using: several call sites invoke this
// from inside a transaction, and this method's own store reads must run
// on that transaction's connection, not a fresh one from the pool — a
// single-connection SQLite pool deadlocks otherwise (the open tx never
// releases its connection while waiting on a query that itself is waiting
// for a connection).
func (s *ResultsService) scorePoints(ctx context.Context, db store.DBTX, meet store.MeetRecord, disciplineCode string, timing domain.Timing, sex domain.Sex, mark string) (*int, error) {
	if combinedID, ok, err := store.GetMeetCombinedScoringTable(ctx, db, meet.ID); err != nil {
		return nil, err
	} else if ok {
		table, ok := s.combinedTables[combinedID]
		if !ok {
			return nil, fmt.Errorf("meet %s references unknown combined scoring table %q", meet.ID, combinedID)
		}
		pts, err := table.Points(disciplineCode, sex, timing, mark)
		if err != nil {
			return nil, err
		}
		return &pts, nil
	}
	if meet.ScoringTableID == "" {
		return nil, nil
	}
	table, ok := s.tables[meet.ScoringTableID]
	if !ok {
		return nil, fmt.Errorf("meet %s references unknown scoring table %q", meet.ID, meet.ScoringTableID)
	}
	pts, err := table.Points(disciplineCode, timing, sex, mark)
	if err != nil {
		return nil, err
	}
	return &pts, nil
}
