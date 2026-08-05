// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// --- Protest clock, announcement & corrections (TASK-019, UC-015;
// SYS-046/047): once a unit's result list is announced, further edits to
// its results are corrections — audited with actor/reason, gated by the
// protest window (D8.3) — not ordinary capture. Wind (SYS-040, UC-010 #4)
// is a per-race reading, not per-athlete: SetUnitWind is the one write path
// and SaveTrackResult (capture.go) reads it back onto every mark it saves. ---

// ErrCorrectionRequired means the unit's results have already been
// announced (SYS-047): further edits must go through CorrectResult, which
// records a reason and (once the protest window has lapsed) an escalation
// reference, rather than silently overwriting a posted result list.
var ErrCorrectionRequired = errors.New("this unit's results have been announced; use the correction flow (SYS-046/047)")

// ErrCorrectionReasonRequired means a correction was submitted with no
// reason (SYS-046, UC-015 #3: "any correction... reason").
var ErrCorrectionReasonRequired = errors.New("a correction requires a reason (SYS-046)")

// ErrEscalationRequired means a correction targets a result whose protest
// window has already elapsed (it is "official"): TASK-019's scope note —
// "corrections after window require explicit escalation/flag" — requires an
// explicit escalation reference (e.g. a jury/referee decision) before an
// already-official result is touched.
var ErrEscalationRequired = errors.New("correcting an official result (protest window elapsed) requires an explicit escalation reference (SYS-047, D8.3)")

// protestState resolves a unit's current SYS-047 protest-clock state from
// its announcement history.
func (s *ResultsService) protestState(ctx context.Context, unitID string) (domain.ProtestState, error) {
	at, found, err := store.LatestAnnouncement(ctx, s.db, unitID)
	if err != nil {
		return domain.ProtestState{}, err
	}
	return domain.ComputeProtestState(at, found, time.Now().UTC()), nil
}

// UnitProtestState is the read-only form CheckUnitAccess-style callers (the
// capture view) use to render the protest-clock banner (UC-015 #1/#4).
func (s *ResultsService) UnitProtestState(ctx context.Context, meetID, unitID string) (domain.ProtestState, error) {
	if _, err := s.unitContext(ctx, meetID, unitID); err != nil {
		return domain.ProtestState{}, err
	}
	return s.protestState(ctx, unitID)
}

// UnitWind returns a unit's current per-race wind reading (SYS-040), nil if
// none has been recorded yet — the capture view's read side for the wind
// form's current-value display.
func (s *ResultsService) UnitWind(ctx context.Context, meetID, unitID string) (*float64, error) {
	if _, err := s.unitContext(ctx, meetID, unitID); err != nil {
		return nil, err
	}
	return store.GetUnitWind(ctx, s.db, unitID)
}

// requireNotAnnounced is the capture-write guard SaveTrackResult and
// SaveFieldAttempt call before writing: once a unit is announced, plain
// capture saves are rejected in favor of the audited correction flow.
func (s *ResultsService) requireNotAnnounced(ctx context.Context, unitID string) error {
	state, err := s.protestState(ctx, unitID)
	if err != nil {
		return err
	}
	if state.Announced {
		return ErrCorrectionRequired
	}
	return nil
}

// AnnounceUnitResults posts a unit's current result list (UC-015 #1):
// office action, records the announcement timestamp that starts the
// 30-minute protest window (SYS-047) and is itself audited.
func (s *ResultsService) AnnounceUnitResults(ctx context.Context, actor Session, meetID, unitID string) (domain.ProtestState, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return domain.ProtestState{}, err
	}
	if _, err := s.unitContext(ctx, meetID, unitID); err != nil {
		return domain.ProtestState{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.ProtestState{}, fmt.Errorf("announce unit results: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	seq, announcedAt, err := store.AnnounceUnit(ctx, tx, unitID, actor.AccountID)
	if err != nil {
		return domain.ProtestState{}, err
	}
	after, _ := json.Marshal(map[string]any{"unit": unitID, "seq": seq, "announcedAt": announcedAt})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "result.announce",
		EntityType: "unit", EntityID: unitID, After: string(after),
	}); err != nil {
		return domain.ProtestState{}, fmt.Errorf("audit result.announce: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.ProtestState{}, fmt.Errorf("announce unit results: %w", err)
	}
	s.notifyChanged(meetID)
	return domain.ComputeProtestState(announcedAt, true, time.Now().UTC()), nil
}

// SetUnitWind records the single per-race wind reading (SYS-040: "per-race
// wind reading where the discipline is wind-relevant") and applies it to
// every already-settled result of the unit (UC-010 #4). Capture access is
// the same per-event scoping as ordinary track/field capture (SYS-090); a
// non-wind-relevant discipline rejects the call outright (UC-010's wind
// legality rules never apply outside straight sprints/hurdles/horizontal
// jumps and throws). Track units only: horizontal field events already
// capture wind per attempt (TASK-008, domain.Attempt.Wind) — a settled,
// unit-level wind override there would be silently clobbered the next time
// SaveFieldAttempt recomputes the settled result from the attempt series,
// so this settled-result-level reading is scoped to track (OQ-036).
func (s *ResultsService) SetUnitWind(ctx context.Context, actor Session, meetID, unitID string, wind float64) error {
	if err := s.authorizeCaptureAccess(ctx, actor, meetID, unitID); err != nil {
		return err
	}
	uc, err := s.unitContext(ctx, meetID, unitID)
	if err != nil {
		return err
	}
	if uc.disc.Family != domain.FamilyTrack {
		return fmt.Errorf("unit-level wind capture is for track units (field events record wind per attempt, TASK-008); unit %s is %s", unitID, uc.disc.Family)
	}
	if !uc.disc.WindRelevant {
		return fmt.Errorf("discipline %q is not wind-relevant, no wind reading expected (SYS-040)", uc.disc.Code)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set unit wind: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	before, err := store.GetUnitWind(ctx, tx, unitID)
	if err != nil {
		return err
	}
	if err := store.SetUnitWind(ctx, tx, unitID, wind); err != nil {
		return err
	}
	if err := store.UpdateResultsWind(ctx, tx, unitID, &wind); err != nil {
		return err
	}
	beforeJSON, _ := json.Marshal(map[string]any{"wind": before})
	afterJSON, _ := json.Marshal(map[string]any{"wind": wind})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "unit.wind",
		EntityType: "unit", EntityID: unitID, Before: string(beforeJSON), After: string(afterJSON),
	}); err != nil {
		return fmt.Errorf("audit unit.wind: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("set unit wind: %w", err)
	}
	s.notifyChanged(meetID)
	return nil
}

// CorrectionInput is one office-issued correction to a settled result
// (UC-015 #2/#3): the amended mark/status plus the mandatory reason, and —
// only once the protest window has elapsed — the escalation reference the
// TASK-019 scope note requires.
type CorrectionInput struct {
	Mark         string
	Timing       domain.Timing // track units only; ignored for field
	Status       domain.QualificationStatus
	StatusDetail string
	Reason       string
	Escalation   string
}

// CorrectResult amends a unit's settled result for one athlete after its
// result list has been announced (UC-015 #2): office-only, requires a
// reason (SYS-046), requires an escalation reference once the protest
// window has elapsed (SYS-047), audits actor/timestamp/before/after/reason
// (immutable, SYS-046), and re-announces the unit — opening a fresh appeal
// window (UC-015 #2's "the amended list gets a new announcement
// timestamp"). Track, horizontal-field and vertical-field units are all
// supported at the settled-result granularity (DEC-024/TASK-040 widened the
// original track+horizontal scope to vertical); a field correction does not
// retroactively rewrite the attempt/trial series it was derived from
// (OQ-036).
func (s *ResultsService) CorrectResult(ctx context.Context, actor Session, meetID, unitID, athleteID string, in CorrectionInput) (store.ResultRecord, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return store.ResultRecord{}, err
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		return store.ResultRecord{}, ErrCorrectionReasonRequired
	}
	uc, err := s.unitContext(ctx, meetID, unitID)
	if err != nil {
		return store.ResultRecord{}, err
	}
	if uc.disc.Family != domain.FamilyTrack && uc.disc.Family != domain.FamilyFieldHorizontal && uc.disc.Family != domain.FamilyFieldVertical {
		return store.ResultRecord{}, fmt.Errorf("result correction is not defined for discipline family %q", uc.disc.Family)
	}
	p, err := s.participant(ctx, meetID, athleteID)
	if err != nil {
		return store.ResultRecord{}, err
	}

	state, err := s.protestState(ctx, unitID)
	if err != nil {
		return store.ResultRecord{}, err
	}
	if state.Official && strings.TrimSpace(in.Escalation) == "" {
		return store.ResultRecord{}, ErrEscalationRequired
	}

	existing, err := store.GetResult(ctx, s.db, unitID, athleteID)
	hadExisting := true
	if errors.Is(err, store.ErrNotFound) {
		hadExisting = false
	} else if err != nil {
		return store.ResultRecord{}, err
	}

	result := domain.Result{
		UnitID: unitID, AthleteID: athleteID,
		Status: in.Status, StatusDetail: in.StatusDetail,
	}
	if hadExisting {
		result.Wind = existing.Wind
		result.Lane = existing.Lane
	}
	timing := domain.TimingNone
	if in.Status != domain.StatusNone {
		if err := domain.ValidateCaptureStatus(in.Status, in.StatusDetail); err != nil {
			return store.ResultRecord{}, err
		}
	} else if uc.disc.Family == domain.FamilyTrack {
		switch in.Timing {
		case domain.TimingManual:
			if result.Mark, err = domain.RoundUpHandTime(in.Mark); err != nil {
				return store.ResultRecord{}, err
			}
		case domain.TimingElectronic:
			if result.Mark, err = domain.ValidateFATTime(in.Mark); err != nil {
				return store.ResultRecord{}, err
			}
		default:
			return store.ResultRecord{}, fmt.Errorf("a timing method is required for a time correction (SYS-041)")
		}
		timing = in.Timing
	} else {
		centi, err := domain.ParseCentiMark(in.Mark)
		if err != nil {
			return store.ResultRecord{}, err
		}
		if centi == 0 {
			return store.ResultRecord{}, errors.New("a corrected mark must be > 0")
		}
		result.Mark = domain.FormatCentiMark(centi, 2)
	}
	// Scoring and record/best re-evaluation only apply to a mark-bearing
	// correction (in.Status == StatusNone): a DNS/DNF/DQ/NM correction has
	// no mark to score or compare against a record list — calling either
	// unconditionally here (as this function did before) fails outright
	// (domain.ParseCentiMark("") on the empty mark) the moment a status-only
	// correction is attempted, which no test exercised until TASK-040 added
	// a reachable status field to the field/vertical correction UI and
	// caught it; SaveTrackResult/SaveFieldAttempt already gate the
	// equivalent capture-time scoring call the same way (SYS-041/042).
	if in.Status == domain.StatusNone {
		if result.Points, err = s.scorePoints(ctx, s.db, uc.meet, uc.disc.Code, timing, p.Athlete.Sex, result.Mark); err != nil {
			return store.ResultRecord{}, err
		}
		// Record/best flagging (SYS-049/050) must be re-evaluated against the
		// corrected mark, not silently dropped: domain.Result{} above starts
		// with nil RecordFlags, and store.SaveResult persists exactly what it
		// is given — a correction that skipped this would erase a prior "MR"/
		// "PB" flag even when the corrected mark still earns it.
		eval, err := s.evaluateRecord(ctx, s.db, uc.meet, uc.disc, p.Athlete, result.Mark, timing, result.Wind, unitID)
		if err != nil {
			return store.ResultRecord{}, err
		}
		result.RecordFlags = eval.Flags()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.ResultRecord{}, fmt.Errorf("correct result: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var beforeJSON []byte
	if hadExisting {
		beforeJSON, _ = json.Marshal(existing)
	}
	rec, err := store.SaveResult(ctx, tx, result, timing)
	if err != nil {
		return store.ResultRecord{}, err
	}
	afterJSON, _ := json.Marshal(map[string]any{
		"mark": rec.Mark, "timing": timing, "status": in.Status,
		"statusDetail": in.StatusDetail, "points": rec.Points, "escalation": strings.TrimSpace(in.Escalation),
	})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "result.correct",
		EntityType: "result", EntityID: rec.ID,
		Before: string(beforeJSON), After: string(afterJSON), Reason: reason,
	}); err != nil {
		return store.ResultRecord{}, fmt.Errorf("audit result.correct: %w", err)
	}
	// UC-015 #2: the amended list gets a new announcement timestamp,
	// reopening the appeal window.
	seq, announcedAt, err := store.AnnounceUnit(ctx, tx, unitID, actor.AccountID)
	if err != nil {
		return store.ResultRecord{}, err
	}
	reannounceAfter, _ := json.Marshal(map[string]any{"unit": unitID, "seq": seq, "announcedAt": announcedAt, "cause": "correction"})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "result.announce",
		EntityType: "unit", EntityID: unitID, After: string(reannounceAfter),
	}); err != nil {
		return store.ResultRecord{}, fmt.Errorf("audit result.announce (re-announce): %w", err)
	}
	if err := tx.Commit(); err != nil {
		return store.ResultRecord{}, fmt.Errorf("correct result: %w", err)
	}
	s.notifyChanged(meetID)
	return rec, nil
}
