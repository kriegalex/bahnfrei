// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// --- Vertical jump capture (TASK-021, UC-012; SYS-043) ---

// VerticalTrialRecord aliases the store type for the web layer
// (architecture.md §3: web never imports internal/store).
type VerticalTrialRecord = store.VerticalTrialRecord

// UnitDiscipline resolves a unit's discipline — the web layer's cheap
// pre-check for family-specific capture rendering (vertical vs
// horizontal/track, TASK-021), without pulling in a whole UnitCaptureView.
func (s *ResultsService) UnitDiscipline(ctx context.Context, meetID, unitID string) (domain.Discipline, error) {
	uc, err := s.unitContext(ctx, meetID, unitID)
	if err != nil {
		return domain.Discipline{}, err
	}
	return uc.disc, nil
}

// VerticalTrialConflictError reports a concurrent capture of the same
// height/trial: the caller's write did not apply and Current is what
// another session stored (UC-021 #2, SYS-086), mirroring
// AttemptConflictError for horizontal events.
type VerticalTrialConflictError struct {
	Current store.VerticalTrialRecord
}

func (e *VerticalTrialConflictError) Error() string {
	return fmt.Sprintf("height %d trial %d was captured concurrently (stored: %s, version %d)",
		e.Current.HeightIdx, e.Current.Seq, e.Current.Display(), e.Current.Version)
}

func (e *VerticalTrialConflictError) Unwrap() error { return ErrConflict }

// ErrVerticalHeightsNotConfigured means the office has not yet set the
// unit's bar-height progression (SYS-043) — vertical capture cannot record
// a trial without knowing what heights exist.
var ErrVerticalHeightsNotConfigured = errors.New("this unit's bar-height progression has not been configured yet (SYS-043)")

// VerticalHeightsInput is the office's configured (or extended) bar-height
// progression for a unit (SYS-043).
type VerticalHeightsInput struct {
	Heights         []string
	ExpectedVersion int64 // 0 = first-time configuration
}

// SetVerticalHeights configures (or extends, e.g. with a jump-off height)
// a vertical-jump unit's bar-height progression — office action (SYS-043:
// "office-configurable"). Existing trials are untouched: extending the
// progression is always safe (RankVertical/domain.VerticalSeries index by
// position, and existing indices never move because heights may only be
// appended, never reordered or removed — enforced below).
func (s *ResultsService) SetVerticalHeights(ctx context.Context, actor Session, meetID, unitID string, in VerticalHeightsInput) (store.VerticalUnitConfig, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return store.VerticalUnitConfig{}, err
	}
	uc, err := s.unitContext(ctx, meetID, unitID)
	if err != nil {
		return store.VerticalUnitConfig{}, err
	}
	if uc.disc.Family != domain.FamilyFieldVertical {
		return store.VerticalUnitConfig{}, fmt.Errorf("unit %s is %s, not a vertical field event", unitID, uc.disc.Family)
	}
	if len(in.Heights) == 0 {
		return store.VerticalUnitConfig{}, errors.New("at least one height is required (SYS-043)")
	}
	if in.ExpectedVersion > 0 {
		existing, err := store.GetVerticalUnitConfig(ctx, s.db, unitID)
		if err != nil {
			return store.VerticalUnitConfig{}, err
		}
		for i, h := range existing.Heights {
			if i >= len(in.Heights) || in.Heights[i] != h {
				return store.VerticalUnitConfig{}, fmt.Errorf("the existing progression may only be extended, not changed (height %d was %q)", i, h)
			}
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.VerticalUnitConfig{}, fmt.Errorf("set vertical heights: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	cfg, err := store.SetVerticalUnitConfig(ctx, tx, unitID, in.Heights, in.ExpectedVersion)
	if err != nil {
		return store.VerticalUnitConfig{}, err
	}
	after, _ := json.Marshal(map[string]any{"heights": in.Heights})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "vertical.heights.set",
		EntityType: "unit", EntityID: unitID, After: string(after),
	}); err != nil {
		return store.VerticalUnitConfig{}, fmt.Errorf("audit vertical.heights.set: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return store.VerticalUnitConfig{}, fmt.Errorf("set vertical heights: %w", err)
	}
	s.notifyChanged(meetID)
	return cfg, nil
}

// VerticalTrialInput is one trial captured in the height-progression grid.
type VerticalTrialInput struct {
	AthleteID       string
	HeightIdx       int
	Seq             int
	Kind            domain.QualificationStatus // StatusO, StatusX, StatusPass or StatusR
	ExpectedVersion int64
}

// SaveVerticalTrial captures one trial of a vertical-jump event (UC-012,
// SYS-043): the trial is validated and stored, and the athlete's settled
// unit result (best cleared height, D5.2 series status, points where the
// meet scores) is recomputed from the full series in the same
// transaction — mirroring SaveFieldAttempt's settle-in-transaction shape
// for horizontal events.
func (s *ResultsService) SaveVerticalTrial(ctx context.Context, actor Session, meetID, unitID string, in VerticalTrialInput) (store.VerticalTrialRecord, error) {
	if err := s.authorizeCaptureAccess(ctx, actor, meetID, unitID); err != nil {
		return store.VerticalTrialRecord{}, err
	}
	if err := s.requireNotAnnounced(ctx, unitID); err != nil {
		return store.VerticalTrialRecord{}, err
	}
	uc, err := s.unitContext(ctx, meetID, unitID)
	if err != nil {
		return store.VerticalTrialRecord{}, err
	}
	if uc.disc.Family != domain.FamilyFieldVertical {
		return store.VerticalTrialRecord{}, fmt.Errorf("unit %s is %s, not a vertical field event", unitID, uc.disc.Family)
	}
	p, err := s.participant(ctx, meetID, in.AthleteID)
	if err != nil {
		return store.VerticalTrialRecord{}, err
	}
	cfg, err := store.GetVerticalUnitConfig(ctx, s.db, unitID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return store.VerticalTrialRecord{}, ErrVerticalHeightsNotConfigured
		}
		return store.VerticalTrialRecord{}, err
	}
	if in.HeightIdx < 0 || in.HeightIdx >= len(cfg.Heights) {
		return store.VerticalTrialRecord{}, fmt.Errorf("height index %d is outside the configured %d-height progression (SYS-043)", in.HeightIdx, len(cfg.Heights))
	}
	trial := domain.VerticalTrial{HeightIdx: in.HeightIdx, Seq: in.Seq, Kind: in.Kind}
	if err := trial.Validate(); err != nil {
		return store.VerticalTrialRecord{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.VerticalTrialRecord{}, fmt.Errorf("save vertical trial: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var beforeJSON []byte
	if prev, err := store.GetVerticalTrial(ctx, tx, unitID, in.AthleteID, trial.HeightIdx, trial.Seq); err == nil {
		beforeJSON, _ = json.Marshal(prev)
	} else if !errors.Is(err, store.ErrNotFound) {
		return store.VerticalTrialRecord{}, err
	}

	rec, err := store.SaveVerticalTrial(ctx, tx, unitID, in.AthleteID, trial, in.ExpectedVersion)
	if err != nil {
		if errors.Is(err, store.ErrVersionConflict) {
			return rec, &VerticalTrialConflictError{Current: rec}
		}
		return store.VerticalTrialRecord{}, err
	}

	// Settle the athlete's unit result from the full series.
	all, err := store.ListUnitVerticalTrials(ctx, tx, unitID)
	if err != nil {
		return store.VerticalTrialRecord{}, err
	}
	series := domain.VerticalSeries{AthleteID: in.AthleteID}
	for _, t := range all {
		if t.AthleteID == in.AthleteID {
			series.Trials = append(series.Trials, t.VerticalTrial)
		}
	}
	result := domain.Result{UnitID: unitID, AthleteID: in.AthleteID, Status: series.Status()}
	if best, ok := series.BestHeightIdx(); ok {
		result.Mark = cfg.Heights[best]
		if result.Points, err = s.scorePoints(ctx, tx, uc.meet, uc.disc.Code, domain.TimingNone, p.Athlete.Sex, result.Mark); err != nil {
			return store.VerticalTrialRecord{}, err
		}
	}
	settled, err := store.SaveResult(ctx, tx, result, domain.TimingNone)
	if err != nil {
		return store.VerticalTrialRecord{}, err
	}

	afterJSON, _ := json.Marshal(rec)
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "vertical.trial.save",
		EntityType: "vertical_trial", EntityID: rec.ID,
		Before: string(beforeJSON), After: string(afterJSON),
	}); err != nil {
		return store.VerticalTrialRecord{}, fmt.Errorf("audit vertical.trial.save: %w", err)
	}
	settledJSON, _ := json.Marshal(map[string]any{
		"mark": settled.Mark, "status": settled.Status, "points": settled.Points,
	})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "result.settle",
		EntityType: "result", EntityID: settled.ID, After: string(settledJSON),
	}); err != nil {
		return store.VerticalTrialRecord{}, fmt.Errorf("audit result.settle: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return store.VerticalTrialRecord{}, fmt.Errorf("save vertical trial: %w", err)
	}
	s.notifyChanged(meetID)
	return rec, nil
}

// VerticalTrialCell is one (height, trial) cell of the capture grid.
type VerticalTrialCell struct {
	Kind    domain.QualificationStatus // "" = not yet captured
	Version int64
}

// VerticalCaptureRow is one athlete's line in the height-progression grid:
// identity columns, the per-height trial cells (Cells[heightIdx][seq-1])
// and the settled result.
type VerticalCaptureRow struct {
	AthleteID string
	Bib       string
	FirstName string
	LastName  string
	ClubName  string
	BirthYear int
	Cells     [][3]VerticalTrialCell
	Result    *store.ResultRecord
}

// VerticalStandingRow is one line of a vertical-jump unit's current
// ranking (UC-012 #3/#4).
type VerticalStandingRow struct {
	Rank           int
	AthleteID      string
	BestHeight     string // "" when no height was cleared
	AttemptsAtBest int
	TotalFailures  int
	Eliminated     bool
	Retired        bool
	TieForFirst    bool
	Points         *int
}

// VerticalCaptureView is everything the vertical-jump capture page for one
// unit renders.
type VerticalCaptureView struct {
	Meet           store.MeetRecord
	UnitID         string
	DisciplineCode string
	DisciplineName string
	// Heights is the unit's configured bar-height progression, ascending;
	// empty means the office has not configured it yet (SYS-043).
	Heights   []string
	Rows      []VerticalCaptureRow
	Standings []VerticalStandingRow
}

// VerticalCapture assembles the capture view for one vertical-jump unit:
// the height-progression grid with every captured trial, the settled
// results, and the current SYS-043 countback standings (UC-012 #3/#4).
func (s *ResultsService) VerticalCapture(ctx context.Context, meetID, unitID string) (VerticalCaptureView, error) {
	uc, err := s.unitContext(ctx, meetID, unitID)
	if err != nil {
		return VerticalCaptureView{}, err
	}
	if uc.disc.Family != domain.FamilyFieldVertical {
		return VerticalCaptureView{}, fmt.Errorf("unit %s is %s, not a vertical field event", unitID, uc.disc.Family)
	}
	participants, err := store.ListParticipants(ctx, s.db, meetID)
	if err != nil {
		return VerticalCaptureView{}, err
	}
	clubs, err := s.ClubNamesFor(ctx, participants)
	if err != nil {
		return VerticalCaptureView{}, err
	}
	trials, err := store.ListUnitVerticalTrials(ctx, s.db, unitID)
	if err != nil {
		return VerticalCaptureView{}, err
	}
	results, err := store.ListUnitResults(ctx, s.db, unitID)
	if err != nil {
		return VerticalCaptureView{}, err
	}

	v := VerticalCaptureView{
		Meet:           uc.meet,
		UnitID:         unitID,
		DisciplineCode: uc.disc.Code,
		DisciplineName: uc.disc.Name,
	}
	if cfg, err := store.GetVerticalUnitConfig(ctx, s.db, unitID); err == nil {
		v.Heights = cfg.Heights
	} else if !errors.Is(err, store.ErrNotFound) {
		return VerticalCaptureView{}, err
	}

	byAthleteTrials := map[string][]store.VerticalTrialRecord{}
	for _, t := range trials {
		byAthleteTrials[t.AthleteID] = append(byAthleteTrials[t.AthleteID], t)
	}
	byAthleteResult := map[string]*store.ResultRecord{}
	for i := range results {
		byAthleteResult[results[i].AthleteID] = &results[i]
	}

	var series []domain.VerticalSeries
	for _, p := range participants {
		row := VerticalCaptureRow{
			AthleteID: p.AthleteID,
			Bib:       p.Bib,
			FirstName: p.Athlete.FirstName,
			LastName:  p.Athlete.LastName,
			BirthYear: p.Athlete.BirthYear,
			Cells:     make([][3]VerticalTrialCell, len(v.Heights)),
			Result:    byAthleteResult[p.AthleteID],
		}
		if len(p.Athlete.ClubIDs) > 0 {
			row.ClubName = clubs[p.Athlete.ClubIDs[0]]
		}
		vs := domain.VerticalSeries{AthleteID: p.AthleteID}
		for _, t := range byAthleteTrials[p.AthleteID] {
			vs.Trials = append(vs.Trials, t.VerticalTrial)
			if t.HeightIdx >= 0 && t.HeightIdx < len(row.Cells) && t.Seq >= 1 && t.Seq <= 3 {
				row.Cells[t.HeightIdx][t.Seq-1] = VerticalTrialCell{Kind: t.Kind, Version: t.Version}
			}
		}
		series = append(series, vs)
		v.Rows = append(v.Rows, row)
	}

	for _, st := range domain.RankVertical(series) {
		row := VerticalStandingRow{
			Rank:           st.Rank,
			AthleteID:      st.AthleteID,
			AttemptsAtBest: st.AttemptsAtBest,
			TotalFailures:  st.TotalFailures,
			Eliminated:     st.Eliminated,
			Retired:        st.Retired,
			TieForFirst:    st.TieForFirst,
		}
		if st.BestHeightIdx >= 0 && st.BestHeightIdx < len(v.Heights) {
			row.BestHeight = v.Heights[st.BestHeightIdx]
		}
		if r := byAthleteResult[st.AthleteID]; r != nil {
			row.Points = r.Points
		}
		v.Standings = append(v.Standings, row)
	}
	return v, nil
}
