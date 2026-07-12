// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// --- Entry eligibility (TASK-017, UC-005, SYS-014): category/licence/
// youth-protection validation surfaced as a per-entry eligible/warning/
// blocked outcome, with an authorized-operator override path. Builds on
// TASK-016's Entry/EntryDetail shapes (internal/app/entry.go) rather than
// introducing a parallel entry representation. ---

// EligibilityView is one entry's eligibility evaluation for display (UC-005
// #1–#4): the domain result plus any recorded override. A zero-value
// EligibilityView (no stored evaluation) is eligible/no-flags — see
// store.GetEntryEligibility's doc comment.
type EligibilityView struct {
	Outcome        domain.EligibilityOutcome
	Flags          []domain.EligibilityFlag
	Overridden     bool
	OverriddenBy   string
	OverrideReason string
	OverriddenAt   *time.Time
	// Version is the entry_eligibility row's optimistic-concurrency token,
	// required to submit an override.
	Version int64
}

// EffectiveOutcome mirrors store.EntryEligibilityRecord.EffectiveOutcome:
// an override always reads as eligible for start-list-gating purposes.
func (v EligibilityView) EffectiveOutcome() domain.EligibilityOutcome {
	if v.Overridden {
		return domain.EligibilityEligible
	}
	if v.Outcome == "" {
		return domain.EligibilityEligible
	}
	return v.Outcome
}

func eligibilityViewFrom(rec store.EntryEligibilityRecord) EligibilityView {
	return EligibilityView{
		Outcome: rec.Outcome, Flags: rec.Flags, Overridden: rec.Overridden(),
		OverriddenBy: rec.OverriddenBy, OverrideReason: rec.OverrideReason,
		OverriddenAt: rec.OverriddenAt, Version: rec.Version,
	}
}

// pickEventCategory resolves which of an event's (possibly several,
// SYS-002 combined-category) category codes is the target for eligibility
// evaluation: the single category if there is only one, else the first one
// matching the athlete's sex, else the first listed.
func pickEventCategory(scheme *domain.CategoryScheme, categoryCodes []string, sex domain.Sex) string {
	if len(categoryCodes) == 0 {
		return ""
	}
	if len(categoryCodes) == 1 || scheme == nil {
		return categoryCodes[0]
	}
	for _, code := range categoryCodes {
		if cat, ok := scheme.CategoryByCode(code); ok && cat.MatchesSex(sex) {
			return code
		}
	}
	return categoryCodes[0]
}

// evaluateAndStoreEligibility resolves the meet's category scheme, computes
// domain.EvaluateEligibility for one individual entry, and persists the
// result (SYS-014). It is a no-op (returns a zero EligibilityResult) when
// the meet's category scheme is unknown or the event has no category codes
// — entry creation itself already validates both via validateEntryEvent/
// AddEvent, so this should not normally happen; eligibility is a secondary
// check layered on top and a missing scheme/category must not block an
// entry submission that otherwise already succeeded.
func (s *ResultsService) evaluateAndStoreEligibility(ctx context.Context, tx store.DBTX, meet store.MeetRecord, event store.EventRecord, athlete store.AthleteRecord, entryID string) (domain.EligibilityResult, error) {
	scheme, ok := s.schemes[meet.CategorySchemeID]
	targetCategoryCode := pickEventCategory(scheme, event.CategoryCodes, athlete.Sex)
	if !ok || targetCategoryCode == "" {
		return domain.EligibilityResult{}, nil
	}
	_, hasLicence := athlete.ExternalIDs.Get(domain.NamespaceSwissAthleticsLicence)

	others, err := otherActiveRaceDistances(ctx, tx, meet.ID, athlete.ID, entryID)
	if err != nil {
		return domain.EligibilityResult{}, err
	}

	result := domain.EvaluateEligibility(scheme, s.catalog, domain.EligibilityInput{
		BirthYear: athlete.BirthYear, Sex: athlete.Sex,
		TargetCategoryCode: targetCategoryCode, DisciplineCode: event.DisciplineCode,
		HasLicence: hasLicence, MeetTier: meet.Tier, AsOf: meet.StartDate,
		OtherRaceDistancesM: others,
	})
	if err := store.UpsertEntryEligibility(ctx, tx, entryID, result); err != nil {
		return domain.EligibilityResult{}, err
	}
	return result, nil
}

// otherActiveRaceDistances gathers the track-race distances of athleteID's
// other active (non-scratched) entries at meetID, excluding excludeEntryID —
// the UC-005 #3 "max one race ≥600m per day" input. Approximated at meet
// scope rather than competition-day scope: events carry no day/session link
// yet (that wiring lands with round/unit seeding, TASK-018) — see OQ-031.
func otherActiveRaceDistances(ctx context.Context, db store.DBTX, meetID, athleteID, excludeEntryID string) ([]int, error) {
	recs, err := store.ListEntriesByMeet(ctx, db, meetID)
	if err != nil {
		return nil, err
	}
	var out []int
	events := map[string]store.EventRecord{}
	for _, r := range recs {
		if r.AthleteID != athleteID || r.ID == excludeEntryID || r.Status == domain.EntryScratched {
			continue
		}
		ev, ok := events[r.EventID]
		if !ok {
			var err error
			if ev, err = store.GetEvent(ctx, db, r.EventID); err != nil {
				return nil, err
			}
			events[r.EventID] = ev
		}
		if dist, ok := domain.TrackDistanceMeters(ev.DisciplineCode); ok {
			out = append(out, dist)
		}
	}
	return out, nil
}

// EntryEligibility returns one entry's current eligibility view (UC-005):
// available to the entry's own submitter and office level and above (mirrors
// entries-page visibility — a submitter needs to see why their own entry is
// flagged).
func (s *ResultsService) EntryEligibility(ctx context.Context, actor Session, entryID string) (EligibilityView, error) {
	if err := Authorize(actor.Role, CapSubmitEntries); err != nil {
		return EligibilityView{}, err
	}
	rec, err := store.GetEntryEligibility(ctx, s.db, entryID)
	if errors.Is(err, store.ErrNotFound) {
		return EligibilityView{}, nil
	}
	if err != nil {
		return EligibilityView{}, err
	}
	return eligibilityViewFrom(rec), nil
}

// FlaggedEntryDetail is one entry whose eligibility evaluation is not clean
// (warning or blocked), for the office eligibility-exceptions view.
type FlaggedEntryDetail struct {
	EntryDetail
	Eligibility EligibilityView
}

// EligibilityExceptions lists every meet entry currently flagged (warning or
// blocked, not overridden to eligible) — the office's UC-005 worklist,
// mirroring EntryExceptions' (SYS-015) precedent for the SYS-014 concern.
func (s *ResultsService) EligibilityExceptions(ctx context.Context, actor Session, meetID string) ([]FlaggedEntryDetail, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return nil, err
	}
	if _, err := store.GetMeet(ctx, s.db, meetID); err != nil {
		return nil, err
	}
	byEntry, err := store.ListEntryEligibilityByMeet(ctx, s.db, meetID)
	if err != nil {
		return nil, err
	}
	if len(byEntry) == 0 {
		return nil, nil
	}
	recs, err := store.ListEntriesByMeet(ctx, s.db, meetID)
	if err != nil {
		return nil, err
	}
	var flagged []store.EntryRecord
	var views []EligibilityView
	for _, r := range recs {
		elig, ok := byEntry[r.ID]
		if !ok {
			continue
		}
		v := eligibilityViewFrom(elig)
		// A clean, never-flagged entry is skipped; an overridden entry
		// stays visible so the office can see the resolved history
		// (UC-005 #4) even though its EffectiveOutcome is now eligible.
		if v.EffectiveOutcome() == domain.EligibilityEligible && !v.Overridden {
			continue
		}
		flagged = append(flagged, r)
		views = append(views, v)
	}
	details, err := s.enrichEntries(ctx, flagged)
	if err != nil {
		return nil, err
	}
	out := make([]FlaggedEntryDetail, len(details))
	for i, d := range details {
		out[i] = FlaggedEntryDetail{EntryDetail: d, Eligibility: views[i]}
	}
	return out, nil
}

// OverrideEligibility is the office action that clears a flagged entry to
// proceed (UC-005 #4): actor, reason and timestamp are recorded, and the
// original evaluation stays visible for audit.
func (s *ResultsService) OverrideEligibility(ctx context.Context, actor Session, meetID, entryID string, expectedVersion int64, reason string) (EligibilityView, error) {
	if err := Authorize(actor.Role, CapOfficeActions); err != nil {
		return EligibilityView{}, err
	}
	entry, err := store.GetEntry(ctx, s.db, entryID)
	if err != nil {
		return EligibilityView{}, err
	}
	event, err := store.GetEvent(ctx, s.db, entry.EventID)
	if err != nil {
		return EligibilityView{}, err
	}
	if event.MeetID != meetID {
		return EligibilityView{}, store.ErrNotFound
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return EligibilityView{}, fmt.Errorf("override eligibility: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	if _, err := store.OverrideEntryEligibility(ctx, tx, entryID, expectedVersion, actor.AccountID, reason, now); err != nil {
		return EligibilityView{}, err
	}
	after, _ := json.Marshal(map[string]string{"entry": entryID, "reason": reason})
	if _, err := store.AppendAudit(ctx, tx, store.AuditEntry{
		Actor: actor.AccountID, Action: "entry.eligibility_override",
		EntityType: "entry", EntityID: entryID, After: string(after), Reason: reason,
	}); err != nil {
		return EligibilityView{}, fmt.Errorf("audit entry.eligibility_override: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return EligibilityView{}, fmt.Errorf("override eligibility: %w", err)
	}

	rec, err := store.GetEntryEligibility(ctx, s.db, entryID)
	if err != nil {
		return EligibilityView{}, err
	}
	return eligibilityViewFrom(rec), nil
}
