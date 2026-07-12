// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// TestEntryEligibilityUpsertAndGetSYS014 covers the storage round trip for a
// blocked eligibility evaluation (SYS-014): flags persist with their
// citation, and Overridden()/EffectiveOutcome() report unresolved-block
// state until an override is recorded.
func TestEntryEligibilityUpsertAndGetSYS014(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	ev := entryFixtureEvent(t, s)
	athlete := entryFixtureAthlete(t, s, "Lea")
	entry, err := CreateEntry(ctx, s.DB(), domain.Entry{EventID: ev.ID, AthleteID: athlete.ID, SeedPerformance: "13.50"})
	if err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}

	result := domain.EligibilityResult{
		Outcome: domain.EligibilityBlocked,
		Flags: []domain.EligibilityFlag{
			{Code: domain.FlagLicenceMissing, Outcome: domain.EligibilityBlocked, Reason: "no licence", Citation: "D3.1"},
		},
	}
	if err := UpsertEntryEligibility(ctx, s.DB(), entry.ID, result); err != nil {
		t.Fatalf("UpsertEntryEligibility: %v", err)
	}

	got, err := GetEntryEligibility(ctx, s.DB(), entry.ID)
	if err != nil {
		t.Fatalf("GetEntryEligibility: %v", err)
	}
	if got.Outcome != domain.EligibilityBlocked || len(got.Flags) != 1 || got.Flags[0].Code != domain.FlagLicenceMissing {
		t.Fatalf("got %+v, want blocked with one licence_missing flag", got)
	}
	if got.Overridden() {
		t.Fatal("fresh evaluation must not report Overridden()")
	}
	if got.EffectiveOutcome() != domain.EligibilityBlocked {
		t.Fatalf("EffectiveOutcome() = %q, want blocked (no override yet)", got.EffectiveOutcome())
	}
}

// TestEntryEligibilityOverrideClearsEffectiveOutcomeSYS014UC005_4 covers
// UC-005 #4: an authorized operator overrides a flagged entry with a reason,
// the entry proceeds (EffectiveOutcome eligible), and the override is
// recorded (actor, reason, timestamp) while the original evaluation stays
// visible.
func TestEntryEligibilityOverrideClearsEffectiveOutcomeSYS014UC005_4(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	ev := entryFixtureEvent(t, s)
	athlete := entryFixtureAthlete(t, s, "Nils")
	entry, err := CreateEntry(ctx, s.DB(), domain.Entry{EventID: ev.ID, AthleteID: athlete.ID, SeedPerformance: "13.50"})
	if err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	blocked := domain.EligibilityResult{
		Outcome: domain.EligibilityBlocked,
		Flags:   []domain.EligibilityFlag{{Code: domain.FlagYouthMaxDistance, Outcome: domain.EligibilityBlocked, Reason: "over the cap"}},
	}
	if err := UpsertEntryEligibility(ctx, s.DB(), entry.ID, blocked); err != nil {
		t.Fatalf("UpsertEntryEligibility: %v", err)
	}
	rec, err := GetEntryEligibility(ctx, s.DB(), entry.ID)
	if err != nil {
		t.Fatalf("GetEntryEligibility: %v", err)
	}

	at := time.Date(2027, time.June, 1, 9, 0, 0, 0, time.UTC)
	newVersion, err := OverrideEntryEligibility(ctx, s.DB(), entry.ID, rec.Version, "office-1", "medical clearance on file", at)
	if err != nil {
		t.Fatalf("OverrideEntryEligibility: %v", err)
	}
	if newVersion != rec.Version+1 {
		t.Fatalf("newVersion = %d, want %d", newVersion, rec.Version+1)
	}

	got, err := GetEntryEligibility(ctx, s.DB(), entry.ID)
	if err != nil {
		t.Fatalf("GetEntryEligibility after override: %v", err)
	}
	if !got.Overridden() {
		t.Fatal("expected Overridden() true after override")
	}
	if got.EffectiveOutcome() != domain.EligibilityEligible {
		t.Fatalf("EffectiveOutcome() = %q, want eligible after override", got.EffectiveOutcome())
	}
	if got.OverriddenBy != "office-1" || got.OverrideReason != "medical clearance on file" {
		t.Fatalf("override actor/reason not recorded: %+v", got)
	}
	if got.OverriddenAt == nil || !got.OverriddenAt.Equal(at) {
		t.Fatalf("override timestamp not recorded: %+v", got.OverriddenAt)
	}
	// The original evaluation (outcome+flags) must remain visible for audit.
	if got.Outcome != domain.EligibilityBlocked || len(got.Flags) != 1 {
		t.Fatalf("override must not erase the original evaluation, got %+v", got)
	}
}

// TestEntryEligibilityOverrideConflictSYS083 covers optimistic-concurrency:
// overriding with a stale version is rejected.
func TestEntryEligibilityOverrideConflictSYS083(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	ev := entryFixtureEvent(t, s)
	athlete := entryFixtureAthlete(t, s, "Timo")
	entry, err := CreateEntry(ctx, s.DB(), domain.Entry{EventID: ev.ID, AthleteID: athlete.ID, SeedPerformance: "13.50"})
	if err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	if err := UpsertEntryEligibility(ctx, s.DB(), entry.ID, domain.EligibilityResult{Outcome: domain.EligibilityBlocked}); err != nil {
		t.Fatalf("UpsertEntryEligibility: %v", err)
	}
	if _, err := OverrideEntryEligibility(ctx, s.DB(), entry.ID, 99, "office-1", "reason", time.Now()); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("expected ErrVersionConflict, got %v", err)
	}
}

// TestListEntryEligibilityByMeetSYS014 covers the office eligibility view's
// storage query: only entries at the given meet are returned, keyed by
// entry ID.
func TestListEntryEligibilityByMeetSYS014(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	ev := entryFixtureEvent(t, s)
	athlete := entryFixtureAthlete(t, s, "Rea")
	entry, err := CreateEntry(ctx, s.DB(), domain.Entry{EventID: ev.ID, AthleteID: athlete.ID, SeedPerformance: "13.50"})
	if err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	if err := UpsertEntryEligibility(ctx, s.DB(), entry.ID, domain.EligibilityResult{Outcome: domain.EligibilityWarning}); err != nil {
		t.Fatalf("UpsertEntryEligibility: %v", err)
	}

	meetID := mustEventMeetID(t, s, ev.ID)
	byEntry, err := ListEntryEligibilityByMeet(ctx, s.DB(), meetID)
	if err != nil {
		t.Fatalf("ListEntryEligibilityByMeet: %v", err)
	}
	rec, ok := byEntry[entry.ID]
	if !ok {
		t.Fatalf("expected entry %s in meet eligibility list, got %v", entry.ID, byEntry)
	}
	if rec.Outcome != domain.EligibilityWarning {
		t.Fatalf("Outcome = %q, want warning", rec.Outcome)
	}
}

// TestGetEntryEligibilityNotFound covers the "never evaluated" case: absent
// row reports ErrNotFound, which callers treat as eligible/no-flags.
func TestGetEntryEligibilityNotFound(t *testing.T) {
	s := openTest(t)
	if _, err := GetEntryEligibility(context.Background(), s.DB(), "does-not-exist"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func mustEventMeetID(t *testing.T, s *Store, eventID string) string {
	t.Helper()
	ev, err := GetEvent(context.Background(), s.DB(), eventID)
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	return ev.MeetID
}
