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

// --- Round progression (TASK-018, UC-009, SYS-029/030): compute and record
// Q/q/qR/qJ/qD on a round's unit assignments (D2.4, D5.2). Building the
// next round's start list is GenerateHeats acting on the next round —
// candidatePool already reads qualifiers from the previous round's
// assignments, so no separate "build next round" entry point is needed
// (UC-009 #1's "the semi-final start list is generated"). ---

// AdvancementRequest configures one round-progression computation.
// TopN/FastestK apply to track rounds (SYS-029); Standard/FinalsCapacity/
// BetterDirection apply to field rounds (SYS-030, D2.4). Zero values
// disable the corresponding channel.
type AdvancementRequest struct {
	TopN            int
	FastestK        int
	Standard        string
	FinalsCapacity  int
	BetterDirection string
}

// AdvancementOutcome is one AdvanceRound call's result: the entries newly
// written with a qualification code, and an outstanding tie (if any) that
// still needs an operator decision before the round is fully resolved
// (UC-009 #2).
type AdvancementOutcome struct {
	Advanced []domain.Advancement
	Tie      *domain.TimeTie
}

// ErrRoundNotComplete means a round-progression run found a unit with no
// settled results yet — advancing before the round has finished would
// silently award qualification on incomplete data.
var ErrRoundNotComplete = errors.New("round has units with no settled results yet")

// ErrRoundNotSeeded means AdvanceRound was called before GenerateHeats ever
// ran for this round (no unit assignments exist yet) — there is nothing to
// progress (TASK-055/SYS-117: distinct from ErrRoundNotComplete, which means
// heats exist but haven't all finished).
var ErrRoundNotSeeded = errors.New("round has no seeded entries yet")

// ErrInvalidQualificationCode means ManualAdvance received a code outside
// the legal manual-advancement alphabet (D2.4/D5.2: Q/q/qR/qJ/qD) — the
// seeding page's mini-form only offers the legal codes, so this only
// happens from a malformed direct request.
var ErrInvalidQualificationCode = errors.New("not a legal advancement code (D2.4/D5.2)")

// ErrEntryNotInRound means ManualAdvance's entryID has no assignment in
// roundID — the round changed (regeneration, scratch) since the seeding
// page that offered this entry was rendered.
var ErrEntryNotInRound = errors.New("entry is not seeded in this round")

// AdvanceRound computes SYS-029/030 progression for one completed round and
// writes the resulting Q/q qualification codes onto its unit assignments.
// Entries a previous call (or a manual override) already resolved are left
// untouched — calling this again after ManualAdvance resolves an
// outstanding tie is the normal way to finish a round whose fastest-loser
// cutoff was contested (UC-009 #2).
func (s *ResultsService) AdvanceRound(ctx context.Context, actor Session, meetID, eventID, roundID string, req AdvancementRequest) (AdvancementOutcome, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return AdvancementOutcome{}, err
	}
	event, err := eventOfMeet(ctx, s.db, meetID, eventID)
	if err != nil {
		return AdvancementOutcome{}, err
	}
	disc, ok := s.catalog.ByCode(event.DisciplineCode)
	if !ok {
		return AdvancementOutcome{}, fmt.Errorf("advance round: unknown discipline %q", event.DisciplineCode)
	}
	assignments, err := store.ListRoundAssignments(ctx, s.db, roundID)
	if err != nil {
		return AdvancementOutcome{}, err
	}
	if len(assignments) == 0 {
		return AdvancementOutcome{}, fmt.Errorf("advance round: %w", ErrRoundNotSeeded)
	}

	alreadyResolved := map[string]domain.QualificationStatus{}
	for _, a := range assignments {
		if a.Qualification != domain.StatusNone {
			alreadyResolved[a.EntryID] = a.Qualification
		}
	}

	var advanced []domain.Advancement
	var tie *domain.TimeTie
	switch disc.Family {
	case domain.FamilyTrack:
		heats, err := s.trackHeatFinishes(ctx, assignments)
		if err != nil {
			return AdvancementOutcome{}, err
		}
		advanced, tie = domain.ComputeTrackAdvancement(heats, domain.AdvancementRule{TopN: req.TopN, FastestK: req.FastestK})
	case domain.FamilyFieldHorizontal, domain.FamilyFieldVertical:
		finishes, err := s.fieldFinishes(ctx, assignments)
		if err != nil {
			return AdvancementOutcome{}, err
		}
		advanced, tie = domain.ComputeFieldAdvancement(finishes, req.Standard, req.BetterDirection, req.FinalsCapacity)
	default:
		return AdvancementOutcome{}, fmt.Errorf("advance round: progression is not defined for discipline family %q", disc.Family)
	}

	if tie != nil {
		// Once the operator has manually decided at least one member of a
		// tied group (a draw picks a single winner; the others simply do
		// not advance — D2.4/D5.2 has no "definitively excluded" code), the
		// group is resolved and must not keep re-surfacing on every later
		// AdvanceRound call.
		for _, id := range tie.EntryIDs {
			if _, ok := alreadyResolved[id]; ok {
				tie = nil
				break
			}
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AdvancementOutcome{}, fmt.Errorf("advance round: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var written []domain.Advancement
	byUnitEntry := map[string]store.UnitAssignmentRecord{}
	for _, a := range assignments {
		byUnitEntry[a.EntryID] = a
	}
	for _, adv := range advanced {
		if _, resolved := alreadyResolved[adv.EntryID]; resolved {
			continue // idempotent re-invocation: never clobber a resolved entry
		}
		rec, ok := byUnitEntry[adv.EntryID]
		if !ok {
			continue
		}
		if _, err := store.SetUnitAssignmentQualification(ctx, tx, rec.ID, rec.Version, adv.Code); err != nil {
			return AdvancementOutcome{}, err
		}
		written = append(written, adv)
	}

	after, _ := json.Marshal(map[string]any{"round": roundID, "advanced": len(written), "tie": tie != nil})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "progression.advance",
		EntityType: "round", EntityID: roundID, After: string(after),
	}); err != nil {
		return AdvancementOutcome{}, fmt.Errorf("audit progression.advance: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return AdvancementOutcome{}, fmt.Errorf("advance round: %w", err)
	}
	s.notifyChanged(meetID)
	return AdvancementOutcome{Advanced: written, Tie: tie}, nil
}

// ManualAdvance records a referee/jury/draw decision (qR/qJ/qD) — or a
// direct Q/q correction — on one entry's current-round assignment
// (UC-009 #2/#3). Audited.
func (s *ResultsService) ManualAdvance(ctx context.Context, actor Session, meetID, eventID, roundID, entryID string, code domain.QualificationStatus) error {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return err
	}
	if _, err := eventOfMeet(ctx, s.db, meetID, eventID); err != nil {
		return err
	}
	switch code {
	case domain.StatusQ, domain.StatusQt, domain.StatusQR, domain.StatusQJ, domain.StatusQD:
	default:
		return fmt.Errorf("manual advance: %q: %w", code, ErrInvalidQualificationCode)
	}
	assignments, err := store.ListRoundAssignments(ctx, s.db, roundID)
	if err != nil {
		return err
	}
	var target *store.UnitAssignmentRecord
	for i := range assignments {
		if assignments[i].EntryID == entryID {
			target = &assignments[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("manual advance: entry %s is not seeded in round %s: %w: %w", entryID, roundID, ErrEntryNotInRound, store.ErrNotFound)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("manual advance: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := store.SetUnitAssignmentQualification(ctx, tx, target.ID, target.Version, code); err != nil {
		return err
	}
	after, _ := json.Marshal(map[string]string{"entry": entryID, "code": string(code)})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "progression.manual_advance",
		EntityType: "unit_entry", EntityID: target.ID, After: string(after),
	}); err != nil {
		return fmt.Errorf("audit progression.manual_advance: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("manual advance: %w", err)
	}
	s.notifyChanged(meetID)
	return nil
}

// trackHeatFinishes assembles per-heat finish data from a round's settled
// track results, ranked by trackStandings (the same ranking the capture
// view already renders, so progression and the office's live standings can
// never disagree).
func (s *ResultsService) trackHeatFinishes(ctx context.Context, assignments []store.UnitAssignmentRecord) ([][]domain.HeatFinish, error) {
	byUnit := map[string][]store.UnitAssignmentRecord{}
	var unitOrder []string
	seen := map[string]bool{}
	for _, a := range assignments {
		byUnit[a.UnitID] = append(byUnit[a.UnitID], a)
		if !seen[a.UnitID] {
			seen[a.UnitID] = true
			unitOrder = append(unitOrder, a.UnitID)
		}
	}

	var heats [][]domain.HeatFinish
	for _, unitID := range unitOrder {
		entryByAthlete := map[string]string{} // athleteID -> entryID
		for _, a := range byUnit[unitID] {
			entry, err := store.GetEntry(ctx, s.db, a.EntryID)
			if err != nil {
				return nil, err
			}
			if entry.AthleteID != "" {
				entryByAthlete[entry.AthleteID] = a.EntryID
			}
		}
		results, err := store.ListUnitResults(ctx, s.db, unitID)
		if err != nil {
			return nil, err
		}
		if len(results) == 0 {
			return nil, fmt.Errorf("advance round: unit %s has no settled results yet: %w", unitID, ErrRoundNotComplete)
		}
		var finish []domain.HeatFinish
		for _, st := range trackStandings(results) {
			entryID, ok := entryByAthlete[st.AthleteID]
			if !ok {
				continue
			}
			place := 0
			if st.Status == domain.StatusNone {
				place = st.Rank
			}
			finish = append(finish, domain.HeatFinish{
				EntryID: entryID, Place: place, Mark: st.Mark, Status: st.Status,
			})
		}
		heats = append(heats, finish)
	}
	return heats, nil
}

// fieldFinishes assembles settled field-event marks for a round's entries
// (SYS-030).
func (s *ResultsService) fieldFinishes(ctx context.Context, assignments []store.UnitAssignmentRecord) ([]domain.FieldFinish, error) {
	var out []domain.FieldFinish
	for _, a := range assignments {
		entry, err := store.GetEntry(ctx, s.db, a.EntryID)
		if err != nil {
			return nil, err
		}
		if entry.AthleteID == "" {
			continue
		}
		res, err := store.GetResult(ctx, s.db, a.UnitID, entry.AthleteID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, domain.FieldFinish{EntryID: a.EntryID, Mark: res.Mark})
	}
	return out, nil
}
