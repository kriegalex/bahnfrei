// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

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
}

// CaptureUnits lists a meet's capturable units — track and horizontal
// field disciplines; vertical jumps and relays are later slices
// (TASK-021/016).
func (s *ResultsService) CaptureUnits(ctx context.Context, meetID string) ([]CaptureUnit, error) {
	units, err := store.ListMeetUnits(ctx, s.db, meetID)
	if err != nil {
		return nil, err
	}
	var out []CaptureUnit
	for _, u := range units {
		disc, ok := s.catalog.ByCode(u.DisciplineCode)
		if !ok {
			continue
		}
		if disc.Family != domain.FamilyTrack && disc.Family != domain.FamilyFieldHorizontal {
			continue
		}
		out = append(out, CaptureUnit{
			UnitID:         u.UnitID,
			DisciplineCode: disc.Code,
			DisciplineName: disc.Name,
			Family:         disc.Family,
		})
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

	for _, p := range participants {
		row := CaptureRow{
			AthleteID: p.AthleteID,
			Bib:       p.Bib,
			FirstName: p.Athlete.FirstName,
			LastName:  p.Athlete.LastName,
			BirthYear: p.Athlete.BirthYear,
			Attempts:  make([]*store.AttemptRecord, v.Config.Attempts),
			Result:    byAthleteResult[p.AthleteID],
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
	return v, nil
}

// fieldStandings ranks the field by best mark with next-best tie-breaking
// (SYS-042) and, once every athlete on the start list has taken the
// pre-cut trials, the continuation order for the remaining rounds.
func fieldStandings(rows []CaptureRow, results map[string]*store.ResultRecord, cfg CaptureConfig) ([]UnitStandingRow, []string) {
	var series []domain.FieldSeries
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
	}

	var out []UnitStandingRow
	for _, st := range domain.RankFieldSeries(series) {
		row := UnitStandingRow{
			Rank:      st.Rank,
			AthleteID: st.AthleteID,
			Mark:      st.Best,
			Status:    st.Status,
		}
		if r := results[st.AthleteID]; r != nil {
			row.Points = r.Points
			row.StatusDetail = r.StatusDetail
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
	if err := Authorize(actor.Role, CapCaptureResults); err != nil {
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
		return store.AttemptRecord{}, fmt.Errorf("trial %d exceeds the %d-trial series (SYS-042)", attempt.Seq, cfg.Attempts)
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
		if result.Points, err = s.scorePoints(uc.meet, uc.disc.Code, domain.TimingNone, p.Athlete.Sex, best); err != nil {
			return store.AttemptRecord{}, err
		}
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
}

// SaveTrackResult captures one track result on a unit (UC-010 subset for
// the UKC 60 m): manual times are rounded up to the next 0.1 s and marked
// hand-timed (SYS-041), electronic times keep 0.01 s resolution (SYS-040),
// statuses are restricted to the CR 25 capture vocabulary and a DQ
// requires its rule reference (SYS-045).
func (s *ResultsService) SaveTrackResult(ctx context.Context, actor Session, meetID, unitID string, in TrackResultInput) (store.ResultRecord, error) {
	if err := Authorize(actor.Role, CapCaptureResults); err != nil {
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

	result := domain.Result{
		UnitID:       unitID,
		AthleteID:    in.AthleteID,
		Status:       in.Status,
		StatusDetail: in.StatusDetail,
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
		if result.Points, err = s.scorePoints(uc.meet, uc.disc.Code, timing, p.Athlete.Sex, result.Mark); err != nil {
			return store.ResultRecord{}, err
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.ResultRecord{}, fmt.Errorf("save track result: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rec, err := store.SaveResult(ctx, tx, result, timing)
	if err != nil {
		return store.ResultRecord{}, err
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

// scorePoints scores a mark against the meet's scoring table, if it has
// one (UC-033 #2); nil means the meet does not score points.
func (s *ResultsService) scorePoints(meet store.MeetRecord, disciplineCode string, timing domain.Timing, sex domain.Sex, mark string) (*int, error) {
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
