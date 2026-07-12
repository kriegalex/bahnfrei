// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"errors"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// newTieredEntryFixture is entryFixture with a configurable meet tier and
// one open event of the given discipline/category — the eligibility tests
// need tiers/categories the default C-Meeting/U16 W entryFixture does not
// exercise (B-Meeting licence requirement, U12 youth-protection bands).
func newTieredEntryFixture(t *testing.T, tier, disciplineCode, categoryCode string) entryFixture {
	t.Helper()
	meets, results, st := newTestResults(t)
	ctx := context.Background()
	req := ucMeetRequest()
	req.Tier = tier
	rec, err := meets.CreateMeet(ctx, organizer, req)
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	ev, err := meets.AddEvent(ctx, organizer, rec.ID, AddEventRequest{
		DisciplineCode: disciplineCode, CategoryCodes: []string{categoryCode},
	})
	if err != nil {
		t.Fatalf("AddEvent: %v", err)
	}
	if err := meets.PublishMeet(ctx, organizer, rec.ID, rec.Version); err != nil {
		t.Fatalf("PublishMeet: %v", err)
	}
	return entryFixture{meets: meets, results: results, st: st, meetID: rec.ID, eventID: ev.ID}
}

// TestOnlineEntryEligibilityLicenceMissingBlockedSYS014UC005_1 covers UC-005
// #1: an entry submitted online (which collects no licence number) at a
// licence-required tier (Swiss B-Meeting) is flagged licence-missing/blocked
// before start-list generation.
func TestOnlineEntryEligibilityLicenceMissingBlockedSYS014UC005_1(t *testing.T) {
	f := newTieredEntryFixture(t, "B-Meeting", "100m", "Women")
	ctx := context.Background()

	detail, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, IndividualEntryInput{
		EventID: f.eventID, FirstName: "Anna", LastName: "Muster", BirthYear: 1995,
		Sex: domain.SexFemale, SeedPerformance: "12.85",
	})
	if err != nil {
		t.Fatalf("SubmitIndividualEntry: %v", err)
	}
	if detail.Eligibility.EffectiveOutcome() != domain.EligibilityBlocked {
		t.Fatalf("EffectiveOutcome = %q, want blocked (flags %+v)", detail.Eligibility.EffectiveOutcome(), detail.Eligibility.Flags)
	}

	view, err := f.results.EntryEligibility(ctx, entrySubmitter, detail.ID)
	if err != nil {
		t.Fatalf("EntryEligibility: %v", err)
	}
	found := false
	for _, fl := range view.Flags {
		if fl.Code == domain.FlagLicenceMissing {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected %s flag, got %+v", domain.FlagLicenceMissing, view.Flags)
	}
}

// TestOnlineEntryEligibilityLicenceMissingWarningAtCMeetingSYS014UC005 covers
// the C-Meeting counterpart: unlicensed entries are a warning, not a block
// (D1.2 — results simply excluded from ranking lists).
func TestOnlineEntryEligibilityLicenceMissingWarningAtCMeetingSYS014UC005(t *testing.T) {
	f := newEntryFixture(t) // default fixture is C-Meeting, U16 W/100m
	ctx := context.Background()

	detail, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, IndividualEntryInput{
		EventID: f.eventID, FirstName: "Nora", LastName: "Weber", BirthYear: 2012,
		Sex: domain.SexFemale, SeedPerformance: "13.00",
	})
	if err != nil {
		t.Fatalf("SubmitIndividualEntry: %v", err)
	}
	if detail.Eligibility.EffectiveOutcome() != domain.EligibilityWarning {
		t.Fatalf("EffectiveOutcome = %q, want warning (flags %+v)", detail.Eligibility.EffectiveOutcome(), detail.Eligibility.Flags)
	}
}

// TestOnlineEntryEligibilityYouthMaxDistanceBlockedSYS014UC005_2 covers
// UC-005 #2: a U12 athlete entered for 3000m (max 2000m per D4.3) is
// flagged with the youth-protection rule reference.
func TestOnlineEntryEligibilityYouthMaxDistanceBlockedSYS014UC005_2(t *testing.T) {
	f := newTieredEntryFixture(t, "C-Meeting", "3000m", "U12 M")
	ctx := context.Background()

	// meetDay(0) is 2027-06-10; birth year 2016 -> age 11 in 2027, U12 M
	// (age band 10-11).
	detail, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, IndividualEntryInput{
		EventID: f.eventID, FirstName: "Timo", LastName: "Frei", BirthYear: 2016,
		Sex: domain.SexMale, SeedPerformance: "15:00.00",
	})
	if err != nil {
		t.Fatalf("SubmitIndividualEntry: %v", err)
	}
	if detail.Eligibility.EffectiveOutcome() != domain.EligibilityBlocked {
		t.Fatalf("EffectiveOutcome = %q, want blocked (flags %+v)", detail.Eligibility.EffectiveOutcome(), detail.Eligibility.Flags)
	}
	found := false
	for _, fl := range detail.Eligibility.Flags {
		if fl.Code == domain.FlagYouthMaxDistance {
			found = true
			if fl.Citation == "" {
				t.Error("youth-protection flag must carry a rule citation")
			}
		}
	}
	if !found {
		t.Fatalf("expected %s flag, got %+v", domain.FlagYouthMaxDistance, detail.Eligibility.Flags)
	}
}

// TestOnlineEntryEligibilityOneRacePerDayBlockedSYS014UC005_3 covers UC-005
// #3: a U12 athlete's first ≥600m entry is not flagged for the one-race cap;
// a second ≥600m entry at the same meet, for the *same athlete*, is. Online
// individual submission creates a fresh athlete row per call (no person-
// record reuse — that is the CSV import path's job, UC-004 #2), so this
// exercises the same-athlete scenario directly against the store, the way
// the import path (which does reuse an existing athlete across rows) will
// drive it.
func TestOnlineEntryEligibilityOneRacePerDayBlockedSYS014UC005_3(t *testing.T) {
	f := newTieredEntryFixture(t, "C-Meeting", "800m", "U12 W")
	ctx := context.Background()
	secondEventID := f.addEvent(t, AddEventRequest{DisciplineCode: "1500m", CategoryCodes: []string{"U12 W"}})

	athlete, err := store.CreateAthlete(ctx, f.st.DB(), domain.Athlete{
		FirstName: "Elin", LastName: "Suter", BirthYear: 2016, Sex: domain.SexFemale,
	})
	if err != nil {
		t.Fatalf("CreateAthlete: %v", err)
	}
	meet, err := store.GetMeet(ctx, f.st.DB(), f.meetID)
	if err != nil {
		t.Fatalf("GetMeet: %v", err)
	}
	firstEvent, err := store.GetEvent(ctx, f.st.DB(), f.eventID)
	if err != nil {
		t.Fatalf("GetEvent (first): %v", err)
	}
	secondEvent, err := store.GetEvent(ctx, f.st.DB(), secondEventID)
	if err != nil {
		t.Fatalf("GetEvent (second): %v", err)
	}

	firstEntry, err := store.CreateEntry(ctx, f.st.DB(), domain.Entry{EventID: firstEvent.ID, AthleteID: athlete.ID, SeedPerformance: "2:40.00"})
	if err != nil {
		t.Fatalf("CreateEntry (first): %v", err)
	}
	firstResult, err := f.results.evaluateAndStoreEligibility(ctx, f.st.DB(), meet, firstEvent, athlete, firstEntry.ID)
	if err != nil {
		t.Fatalf("evaluateAndStoreEligibility (first): %v", err)
	}
	for _, fl := range firstResult.Flags {
		if fl.Code == domain.FlagYouthOneRacePerDay {
			t.Fatalf("first ≥600m race must not trip the one-race-per-day cap, got %+v", firstResult.Flags)
		}
	}

	secondEntry, err := store.CreateEntry(ctx, f.st.DB(), domain.Entry{EventID: secondEvent.ID, AthleteID: athlete.ID, SeedPerformance: "5:40.00"})
	if err != nil {
		t.Fatalf("CreateEntry (second): %v", err)
	}
	secondResult, err := f.results.evaluateAndStoreEligibility(ctx, f.st.DB(), meet, secondEvent, athlete, secondEntry.ID)
	if err != nil {
		t.Fatalf("evaluateAndStoreEligibility (second): %v", err)
	}
	if secondResult.Outcome != domain.EligibilityBlocked {
		t.Fatalf("Outcome = %q, want blocked (flags %+v)", secondResult.Outcome, secondResult.Flags)
	}
	found := false
	for _, fl := range secondResult.Flags {
		if fl.Code == domain.FlagYouthOneRacePerDay {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected %s flag on the second ≥600m entry, got %+v", domain.FlagYouthOneRacePerDay, secondResult.Flags)
	}
}

// TestOverrideEligibilitySYS014UC005_4 covers UC-005 #4: an authorized
// operator overrides a flagged entry with a reason; the entry's effective
// outcome clears to eligible, the override (actor, reason, timestamp) is
// recorded, and it lands in the audit trail (SYS-046).
func TestOverrideEligibilitySYS014UC005_4(t *testing.T) {
	f := newTieredEntryFixture(t, "B-Meeting", "100m", "Women")
	ctx := context.Background()
	detail, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, IndividualEntryInput{
		EventID: f.eventID, FirstName: "Anna", LastName: "Muster", BirthYear: 1995,
		Sex: domain.SexFemale, SeedPerformance: "12.85",
	})
	if err != nil {
		t.Fatalf("SubmitIndividualEntry: %v", err)
	}
	if detail.Eligibility.EffectiveOutcome() != domain.EligibilityBlocked {
		t.Fatalf("precondition: expected blocked, got %q", detail.Eligibility.EffectiveOutcome())
	}

	view, err := f.results.OverrideEligibility(ctx, office, f.meetID, detail.ID, detail.Eligibility.Version, "licence confirmed by phone with Swiss Athletics office")
	if err != nil {
		t.Fatalf("OverrideEligibility: %v", err)
	}
	if view.EffectiveOutcome() != domain.EligibilityEligible {
		t.Fatalf("EffectiveOutcome after override = %q, want eligible", view.EffectiveOutcome())
	}
	if !view.Overridden || view.OverriddenBy != office.AccountID {
		t.Fatalf("override actor not recorded: %+v", view)
	}
	if view.OverriddenAt == nil {
		t.Fatal("override timestamp not recorded")
	}
	// The original evaluation must remain visible (audit).
	if view.Outcome != domain.EligibilityBlocked {
		t.Fatalf("override must preserve the original outcome, got %q", view.Outcome)
	}

	entries, err := store.ListAudit(ctx, f.st.DB(), 100)
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	auditFound := false
	for _, e := range entries {
		if e.Action == "entry.eligibility_override" && e.EntityID == detail.ID && e.Actor == office.AccountID {
			auditFound = true
			if e.Reason == "" {
				t.Error("override audit row must carry the reason")
			}
		}
	}
	if !auditFound {
		t.Fatal("expected an entry.eligibility_override audit row")
	}
}

// TestOverrideEligibilityRequiresOfficeRoleSYS090 covers the "authorized
// operator" gate (UC-005 #4): an entry submitter cannot override their own
// entry's eligibility flags.
func TestOverrideEligibilityRequiresOfficeRoleSYS090(t *testing.T) {
	f := newTieredEntryFixture(t, "B-Meeting", "100m", "Women")
	ctx := context.Background()
	detail, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, IndividualEntryInput{
		EventID: f.eventID, FirstName: "Anna", LastName: "Muster", BirthYear: 1995,
		Sex: domain.SexFemale, SeedPerformance: "12.85",
	})
	if err != nil {
		t.Fatalf("SubmitIndividualEntry: %v", err)
	}
	var forbidden ErrForbidden
	if _, err := f.results.OverrideEligibility(ctx, entrySubmitter, f.meetID, detail.ID, detail.Eligibility.Version, "reason"); !errors.As(err, &forbidden) {
		t.Errorf("entry-submitter OverrideEligibility = %v, want ErrForbidden", err)
	}
}

// TestEligibilityExceptionsSYS014 covers the office worklist: flagged
// entries (any severity) are listed; entries with no eligibility evaluation
// at all (relay entries — OQ-032) are excluded.
func TestEligibilityExceptionsSYS014(t *testing.T) {
	f := newTieredEntryFixture(t, "B-Meeting", "100m", "Women")
	ctx := context.Background()
	relayEventID := f.addEvent(t, AddEventRequest{DisciplineCode: "4x100m", CategoryCodes: []string{"Women"}})

	blocked, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, IndividualEntryInput{
		EventID: f.eventID, FirstName: "Anna", LastName: "Muster", BirthYear: 1995,
		Sex: domain.SexFemale, SeedPerformance: "12.85",
	})
	if err != nil {
		t.Fatalf("SubmitIndividualEntry: %v", err)
	}
	leg := func(name string) RelayLegInput {
		return RelayLegInput{FirstName: name, LastName: "Relay", BirthYear: 1996, Sex: domain.SexFemale}
	}
	if _, err := f.results.SubmitRelayEntry(ctx, entrySubmitter, f.meetID, RelayEntryInput{
		EventID: relayEventID, Club: "LC Relay",
		Composition: []RelayLegInput{leg("A"), leg("B"), leg("C"), leg("D")},
	}); err != nil {
		t.Fatalf("SubmitRelayEntry: %v", err)
	}

	exceptions, err := f.results.EligibilityExceptions(ctx, office, f.meetID)
	if err != nil {
		t.Fatalf("EligibilityExceptions: %v", err)
	}
	if len(exceptions) != 1 || exceptions[0].ID != blocked.ID {
		t.Fatalf("EligibilityExceptions = %+v, want exactly the blocked individual entry (relay excluded)", exceptions)
	}
}

// TestEntryEligibilityRequiresRoleSYS090 covers the read-access floor:
// unauthenticated/public sessions cannot read eligibility detail.
func TestEntryEligibilityRequiresRoleSYS090(t *testing.T) {
	f := newTieredEntryFixture(t, "B-Meeting", "100m", "Women")
	ctx := context.Background()
	detail, err := f.results.SubmitIndividualEntry(ctx, entrySubmitter, f.meetID, IndividualEntryInput{
		EventID: f.eventID, FirstName: "Anna", LastName: "Muster", BirthYear: 1995,
		Sex: domain.SexFemale, SeedPerformance: "12.85",
	})
	if err != nil {
		t.Fatalf("SubmitIndividualEntry: %v", err)
	}
	public := Session{Role: RolePublic}
	var forbidden ErrForbidden
	if _, err := f.results.EntryEligibility(ctx, public, detail.ID); !errors.As(err, &forbidden) {
		t.Errorf("public EntryEligibility = %v, want ErrForbidden", err)
	}
	if _, err := f.results.EligibilityExceptions(ctx, entrySubmitter, f.meetID); !errors.As(err, &forbidden) {
		t.Errorf("entry-submitter EligibilityExceptions = %v, want ErrForbidden", err)
	}
}
