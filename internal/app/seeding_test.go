// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// seedingFixture is a published meet with one 400m event and a
// qualification round, ready for heat-seeding tests (UC-008).
type seedingFixture struct {
	meets   *MeetService
	results *ResultsService
	st      *store.Store
	meetID  string
	eventID string
	roundID string
}

func newSeedingFixture(t *testing.T, disciplineCode string) seedingFixture {
	t.Helper()
	meets, results, st := newTestResults(t)
	ctx := context.Background()
	meet, err := meets.CreateMeet(ctx, organizer, ucMeetRequest())
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	ev, err := meets.AddEvent(ctx, organizer, meet.ID, AddEventRequest{
		DisciplineCode: disciplineCode, CategoryCodes: []string{"U18 W"},
		Rounds: []domain.RoundKind{domain.RoundQualification, domain.RoundFinal},
	})
	if err != nil {
		t.Fatalf("AddEvent: %v", err)
	}
	rounds, err := store.ListRounds(ctx, st.DB(), ev.ID)
	if err != nil {
		t.Fatalf("ListRounds: %v", err)
	}
	return seedingFixture{meets: meets, results: results, st: st, meetID: meet.ID, eventID: ev.ID, roundID: rounds[0].ID}
}

// confirmedEntry creates one athlete with a confirmed entry and given seed
// mark, in club clubID (empty means no club).
func (f seedingFixture) confirmedEntry(t *testing.T, name, clubID, seed string) store.EntryRecord {
	t.Helper()
	ctx := context.Background()
	var clubIDs []string
	if clubID != "" {
		clubIDs = []string{clubID}
	}
	a, err := store.CreateAthlete(ctx, f.st.DB(), domain.Athlete{
		FirstName: name, LastName: "Muster", BirthYear: 2009, Sex: domain.SexFemale, ClubIDs: clubIDs,
	})
	if err != nil {
		t.Fatalf("CreateAthlete: %v", err)
	}
	rec, err := store.CreateEntry(ctx, f.st.DB(), domain.Entry{
		EventID: f.eventID, AthleteID: a.ID, SeedPerformance: seed, Status: domain.EntryConfirmed,
	})
	if err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	return rec
}

// confirmedRelayEntry creates a relay team of legNames athletes in clubID
// with a confirmed entry in the fixture's event, exercising the
// entryClubID/entryDisplayName relay branches (seeding.go) that an
// individual-only fixture never reaches.
func (f seedingFixture) confirmedRelayEntry(t *testing.T, clubID string, legNames ...string) store.EntryRecord {
	t.Helper()
	ctx := context.Background()
	var legIDs []string
	for _, name := range legNames {
		a, err := store.CreateAthlete(ctx, f.st.DB(), domain.Athlete{
			FirstName: name, LastName: "Runner", BirthYear: 2009, Sex: domain.SexFemale, ClubIDs: []string{clubID},
		})
		if err != nil {
			t.Fatalf("CreateAthlete: %v", err)
		}
		legIDs = append(legIDs, a.ID)
	}
	team, err := store.CreateRelayTeam(ctx, f.st.DB(), domain.RelayTeam{ClubID: clubID, Composition: legIDs})
	if err != nil {
		t.Fatalf("CreateRelayTeam: %v", err)
	}
	rec, err := store.CreateEntry(ctx, f.st.DB(), domain.Entry{
		EventID: f.eventID, RelayTeamID: team.ID, Status: domain.EntryConfirmed,
	})
	if err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	return rec
}

// TestGenerateHeatsWithRelayEntries covers the relay branches of
// entryClubID/entryDisplayName (seeding.go), which an individual-only
// fixture never reaches: a relay event's heat sheet resolves each row's
// display name to "<club> (relay)" and its club to the team's club.
func TestGenerateHeatsWithRelayEntries(t *testing.T) {
	f := newSeedingFixture(t, "4x100m")
	ctx := context.Background()
	club, err := store.CreateClub(ctx, f.st.DB(), domain.Club{Name: "LC Relay"})
	if err != nil {
		t.Fatalf("CreateClub: %v", err)
	}
	f.confirmedRelayEntry(t, club.ID, "Leg1", "Leg2", "Leg3", "Leg4")
	f.confirmedRelayEntry(t, club.ID, "Leg5", "Leg6", "Leg7", "Leg8")

	sheet, err := f.results.GenerateHeats(ctx, office, f.meetID, f.eventID, f.roundID,
		GenerateHeatsRequest{MaxHeatSize: 8})
	if err != nil {
		t.Fatalf("GenerateHeats: %v", err)
	}
	if len(sheet.Units) != 1 || len(sheet.Units[0].Rows) != 2 {
		t.Fatalf("expected a single heat of 2 relay teams, got %+v", sheet.Units)
	}
	for _, row := range sheet.Units[0].Rows {
		if row.ClubName != "LC Relay" {
			t.Errorf("row.ClubName = %q, want LC Relay", row.ClubName)
		}
		if row.AthleteName != "LC Relay (relay)" {
			t.Errorf("row.AthleteName = %q, want %q", row.AthleteName, "LC Relay (relay)")
		}
	}

	// HeatSheetFor (the no-regeneration read path) resolves the same relay
	// display data through buildHeatSheet.
	reread, err := f.results.HeatSheetFor(ctx, office, f.meetID, f.eventID, f.roundID)
	if err != nil {
		t.Fatalf("HeatSheetFor: %v", err)
	}
	if len(reread.Units) != 1 || len(reread.Units[0].Rows) != 2 {
		t.Fatalf("HeatSheetFor = %+v, want the same 2-row heat", reread.Units)
	}
}

// TestPublicHeatSheetsListsOnlySeededEvents covers UC-008's public
// start-list surface (SYS-070/074): an event whose round has been seeded
// appears with its heat sheet; an event that exists but has never been
// seeded is omitted entirely, not shown empty.
func TestPublicHeatSheetsListsOnlySeededEvents(t *testing.T) {
	f := newSeedingFixture(t, "100m")
	ctx := context.Background()
	// A second event that is never seeded.
	if _, err := f.meets.AddEvent(ctx, organizer, f.meetID, AddEventRequest{
		DisciplineCode: "200m", CategoryCodes: []string{"U18 W"},
	}); err != nil {
		t.Fatalf("AddEvent: %v", err)
	}

	for i := 0; i < 4; i++ {
		f.confirmedEntry(t, fmt.Sprintf("E%d", i), "", fmt.Sprintf("%d.%02d", 1100+i*5, 0))
	}
	if _, err := f.results.GenerateHeats(ctx, office, f.meetID, f.eventID, f.roundID,
		GenerateHeatsRequest{MaxHeatSize: 8}); err != nil {
		t.Fatalf("GenerateHeats: %v", err)
	}

	sheets, err := f.results.PublicHeatSheets(ctx, f.meetID)
	if err != nil {
		t.Fatalf("PublicHeatSheets: %v", err)
	}
	if len(sheets) != 1 {
		t.Fatalf("PublicHeatSheets = %+v, want exactly one seeded event", sheets)
	}
	if sheets[0].EventID != f.eventID {
		t.Errorf("seeded event = %q, want %q", sheets[0].EventID, f.eventID)
	}
	if len(sheets[0].Rounds) != 1 || len(sheets[0].Rounds[0].Units) != 1 {
		t.Errorf("Rounds = %+v, want a single seeded round/unit", sheets[0].Rounds)
	}
}

// TestGenerateHeatsRejectsUnknownRound and TestHeatSheetForRejectsUnknownRound
// cover ErrRoundNotFound: a round id that does not belong to the event is
// rejected rather than silently seeding/returning nothing.
func TestGenerateHeatsRejectsUnknownRound(t *testing.T) {
	f := newSeedingFixture(t, "100m")
	f.confirmedEntry(t, "Solo", "", "12.00")
	if _, err := f.results.GenerateHeats(context.Background(), office, f.meetID, f.eventID, "no-such-round",
		GenerateHeatsRequest{MaxHeatSize: 8}); !errors.Is(err, ErrRoundNotFound) {
		t.Errorf("GenerateHeats(unknown round) = %v, want ErrRoundNotFound", err)
	}
}

func TestHeatSheetForRejectsUnknownRound(t *testing.T) {
	f := newSeedingFixture(t, "100m")
	if _, err := f.results.HeatSheetFor(context.Background(), office, f.meetID, f.eventID, "no-such-round"); !errors.Is(err, ErrRoundNotFound) {
		t.Errorf("HeatSheetFor(unknown round) = %v, want ErrRoundNotFound", err)
	}
}

// TestOverrideAssignmentRequiresOfficeCapability covers the SYS-090
// least-privilege gate on manual heat/lane edits.
func TestOverrideAssignmentRequiresOfficeCapability(t *testing.T) {
	f := newSeedingFixture(t, "100m")
	entry := f.confirmedEntry(t, "Solo", "", "12.00")
	ctx := context.Background()
	if _, err := f.results.GenerateHeats(ctx, office, f.meetID, f.eventID, f.roundID,
		GenerateHeatsRequest{MaxHeatSize: 8}); err != nil {
		t.Fatalf("GenerateHeats: %v", err)
	}
	sheet, err := f.results.HeatSheetFor(ctx, office, f.meetID, f.eventID, f.roundID)
	if err != nil {
		t.Fatalf("HeatSheetFor: %v", err)
	}
	unauthorized := Session{AccountID: "01SUB", Role: RoleEntrySubmitter}
	var forbidden ErrForbidden
	if err := f.results.OverrideAssignment(ctx, unauthorized, f.meetID, f.eventID, f.roundID,
		entry.ID, sheet.Units[0].UnitID, 1, sheet.Units[0].Rows[0].Version); !errors.As(err, &forbidden) {
		t.Errorf("OverrideAssignment by an entry-submitter = %v, want ErrForbidden", err)
	}
}

// TestOverrideAssignmentVersionConflict covers the optimistic-concurrency
// failure path: overriding with a stale expectedVersion is rejected rather
// than silently clobbering a concurrent edit.
func TestOverrideAssignmentVersionConflict(t *testing.T) {
	f := newSeedingFixture(t, "400m")
	ctx := context.Background()
	var entries []store.EntryRecord
	for i := 0; i < 4; i++ {
		entries = append(entries, f.confirmedEntry(t, fmt.Sprintf("F%d", i), "", fmt.Sprintf("%d.%02d", 5000+i*20, 0)))
	}
	if _, err := f.results.GenerateHeats(ctx, office, f.meetID, f.eventID, f.roundID,
		GenerateHeatsRequest{MaxHeatSize: 8, TrackLanes: 8}); err != nil {
		t.Fatalf("GenerateHeats: %v", err)
	}
	sheet, err := f.results.HeatSheetFor(ctx, office, f.meetID, f.eventID, f.roundID)
	if err != nil {
		t.Fatalf("HeatSheetFor: %v", err)
	}
	target := sheet.Units[0].Rows[0]
	staleVersion := target.Version + 1000

	if err := f.results.OverrideAssignment(ctx, office, f.meetID, f.eventID, f.roundID,
		target.EntryID, sheet.Units[0].UnitID, 2, staleVersion); !errors.Is(err, store.ErrVersionConflict) {
		t.Errorf("OverrideAssignment with a stale version = %v, want store.ErrVersionConflict", err)
	}
}

// TestOverrideAssignmentFirstManualPlacement covers OverrideAssignment's
// insert branch: an entry that is confirmed for the round but has never
// been seeded (GenerateHeats was never run) gets a brand-new manual
// assignment row rather than updating a nonexistent one.
func TestOverrideAssignmentFirstManualPlacement(t *testing.T) {
	f := newSeedingFixture(t, "100m")
	entry := f.confirmedEntry(t, "Solo", "", "12.00")
	ctx := context.Background()

	units, err := store.EnsureRoundUnitCount(ctx, f.st.DB(), f.roundID, 1)
	if err != nil {
		t.Fatalf("EnsureRoundUnitCount: %v", err)
	}

	if err := f.results.OverrideAssignment(ctx, office, f.meetID, f.eventID, f.roundID,
		entry.ID, units[0].ID, 3, 0); err != nil {
		t.Fatalf("OverrideAssignment (first placement): %v", err)
	}
	sheet, err := f.results.HeatSheetFor(ctx, office, f.meetID, f.eventID, f.roundID)
	if err != nil {
		t.Fatalf("HeatSheetFor: %v", err)
	}
	if len(sheet.Units) != 1 || len(sheet.Units[0].Rows) != 1 {
		t.Fatalf("sheet = %+v, want exactly the one manually-placed entry", sheet.Units)
	}
	row := sheet.Units[0].Rows[0]
	if row.Lane != 3 || !row.ManualOverride {
		t.Errorf("row = %+v, want lane 3 with ManualOverride true", row)
	}
}

// TestOverrideAssignmentSwapsOccupiedLane covers UC-008 #5's "the operator
// swaps two athletes": requesting a lane already held by a different entry
// in the target unit swaps the two lanes instead of failing the unique-lane
// constraint.
func TestOverrideAssignmentSwapsOccupiedLane(t *testing.T) {
	f := newSeedingFixture(t, "400m")
	ctx := context.Background()
	var entries []store.EntryRecord
	for i := 0; i < 4; i++ {
		entries = append(entries, f.confirmedEntry(t, fmt.Sprintf("G%d", i), "", fmt.Sprintf("%d.%02d", 5000+i*20, 0)))
	}
	if _, err := f.results.GenerateHeats(ctx, office, f.meetID, f.eventID, f.roundID,
		GenerateHeatsRequest{MaxHeatSize: 8, TrackLanes: 8}); err != nil {
		t.Fatalf("GenerateHeats: %v", err)
	}
	sheet, err := f.results.HeatSheetFor(ctx, office, f.meetID, f.eventID, f.roundID)
	if err != nil {
		t.Fatalf("HeatSheetFor: %v", err)
	}
	unitID := sheet.Units[0].UnitID
	rows := sheet.Units[0].Rows
	mover, occupant := rows[0], rows[1]

	if err := f.results.OverrideAssignment(ctx, office, f.meetID, f.eventID, f.roundID,
		mover.EntryID, unitID, occupant.Lane, mover.Version); err != nil {
		t.Fatalf("OverrideAssignment (swap): %v", err)
	}

	after, err := f.results.HeatSheetFor(ctx, office, f.meetID, f.eventID, f.roundID)
	if err != nil {
		t.Fatalf("HeatSheetFor: %v", err)
	}
	lanes := map[string]int{}
	for _, row := range after.Units[0].Rows {
		lanes[row.EntryID] = row.Lane
	}
	if lanes[mover.EntryID] != occupant.Lane {
		t.Errorf("mover's lane = %d, want the occupant's original lane %d", lanes[mover.EntryID], occupant.Lane)
	}
	if lanes[occupant.EntryID] != mover.Lane {
		t.Errorf("occupant's lane = %d, want the mover's original lane %d", lanes[occupant.EntryID], mover.Lane)
	}
}

// TestGenerateHeatsSYS026UC008_1 reproduces UC-008 #1: 21 confirmed 100m
// entries with seed times and 8 lanes generate 3 heats of 7, seeded
// serpentine, no two of the top-3 seeds sharing a heat.
func TestGenerateHeatsSYS026UC008_1(t *testing.T) {
	f := newSeedingFixture(t, "100m")
	for i := 0; i < 21; i++ {
		centi := 1100 + i*5
		f.confirmedEntry(t, fmt.Sprintf("A%d", i), "", fmt.Sprintf("%d.%02d", centi/100, centi%100))
	}
	sheet, err := f.results.GenerateHeats(context.Background(), office, f.meetID, f.eventID, f.roundID,
		GenerateHeatsRequest{MaxHeatSize: 7, TrackLanes: 8})
	if err != nil {
		t.Fatalf("GenerateHeats: %v", err)
	}
	if len(sheet.Units) != 3 {
		t.Fatalf("expected 3 heats, got %d", len(sheet.Units))
	}
	for _, u := range sheet.Units {
		if len(u.Rows) != 7 {
			t.Errorf("heat %s: expected 7 rows, got %d", u.UnitID, len(u.Rows))
		}
	}
	if sheet.RulesID != domain.SeedingRulesTR20 {
		t.Errorf("expected the applied rule set to be visible (D2.2), got %q", sheet.RulesID)
	}
}

// TestGenerateHeatsLaneDrawSYS027UC008_3 reproduces UC-008 #3 end to end
// through the app service: an 8-athlete 400m field with 8 track lanes
// draws every lane, none repeated.
func TestGenerateHeatsLaneDrawSYS027UC008_3(t *testing.T) {
	f := newSeedingFixture(t, "400m")
	for i := 0; i < 8; i++ {
		centi := 5000 + i*20
		f.confirmedEntry(t, fmt.Sprintf("B%d", i), "", fmt.Sprintf("%d.%02d", centi/100, centi%100))
	}
	sheet, err := f.results.GenerateHeats(context.Background(), office, f.meetID, f.eventID, f.roundID,
		GenerateHeatsRequest{MaxHeatSize: 8, TrackLanes: 8})
	if err != nil {
		t.Fatalf("GenerateHeats: %v", err)
	}
	if len(sheet.Units) != 1 {
		t.Fatalf("expected a single 8-athlete heat, got %d", len(sheet.Units))
	}
	lanes := map[int]bool{}
	for _, row := range sheet.Units[0].Rows {
		if row.Lane < 1 || row.Lane > 8 {
			t.Errorf("entry %s: lane %d out of range", row.EntryID, row.Lane)
		}
		if lanes[row.Lane] {
			t.Errorf("lane %d assigned twice", row.Lane)
		}
		lanes[row.Lane] = true
	}
}

// TestGenerateHeatsManualOverrideSurvivesRegenerationSYS028UC008_5
// reproduces UC-008 #5: a generated heat sheet, the operator swaps an
// athlete manually, then a scratch triggers regeneration — the manual swap
// survives.
func TestGenerateHeatsManualOverrideSurvivesRegenerationSYS028UC008_5(t *testing.T) {
	f := newSeedingFixture(t, "400m")
	var entries []store.EntryRecord
	for i := 0; i < 6; i++ {
		centi := 5000 + i*20
		entries = append(entries, f.confirmedEntry(t, fmt.Sprintf("C%d", i), "", fmt.Sprintf("%d.%02d", centi/100, centi%100)))
	}
	ctx := context.Background()
	if _, err := f.results.GenerateHeats(ctx, office, f.meetID, f.eventID, f.roundID,
		GenerateHeatsRequest{MaxHeatSize: 8, TrackLanes: 8}); err != nil {
		t.Fatalf("GenerateHeats (initial): %v", err)
	}

	sheet, err := f.results.HeatSheetFor(ctx, office, f.meetID, f.eventID, f.roundID)
	if err != nil {
		t.Fatalf("HeatSheetFor: %v", err)
	}
	targetUnit := sheet.Units[0].UnitID
	targetEntry := entries[len(entries)-1].ID // slowest seed, normally not in unit 0's front lanes
	var targetVersion int64
	for _, u := range sheet.Units {
		for _, row := range u.Rows {
			if row.EntryID == targetEntry {
				targetVersion = row.Version
			}
		}
	}
	if err := f.results.OverrideAssignment(ctx, office, f.meetID, f.eventID, f.roundID, targetEntry, targetUnit, 1, targetVersion); err != nil {
		t.Fatalf("OverrideAssignment: %v", err)
	}

	// Scratch one entry (not the overridden one) to force a meaningful
	// regeneration.
	scratched := entries[0]
	if _, err := store.UpdateEntryStatus(ctx, f.st.DB(), scratched.ID, scratched.Version, domain.EntryScratched); err != nil {
		t.Fatalf("scratch entry: %v", err)
	}

	sheet2, err := f.results.GenerateHeats(ctx, office, f.meetID, f.eventID, f.roundID,
		GenerateHeatsRequest{MaxHeatSize: 8, TrackLanes: 8})
	if err != nil {
		t.Fatalf("GenerateHeats (regeneration): %v", err)
	}
	foundOverride := false
	for _, u := range sheet2.Units {
		for _, row := range u.Rows {
			if row.EntryID == targetEntry {
				foundOverride = true
				if u.UnitID != targetUnit || row.Lane != 1 || !row.ManualOverride {
					t.Errorf("expected the manual override (unit %s, lane 1) to survive regeneration, got unit %s lane %d manual=%v",
						targetUnit, u.UnitID, row.Lane, row.ManualOverride)
				}
			}
			if row.EntryID == scratched.ID {
				t.Errorf("expected the scratched entry to be dropped from the regenerated heat sheet")
			}
		}
	}
	if !foundOverride {
		t.Fatal("expected the manually overridden entry to still appear in the regenerated heat sheet")
	}
}

// TestGenerateHeatsRejectsEmptyPool is a denial/edge-path test: no
// confirmed entries means there is nothing to seed.
func TestGenerateHeatsRejectsEmptyPool(t *testing.T) {
	f := newSeedingFixture(t, "100m")
	if _, err := f.results.GenerateHeats(context.Background(), office, f.meetID, f.eventID, f.roundID,
		GenerateHeatsRequest{MaxHeatSize: 8, TrackLanes: 8}); err == nil {
		t.Error("expected an error when there are no confirmed entries to seed")
	}
}

// TestGenerateHeatsOddFieldSize is a denial/edge-path test: an odd field
// size (not evenly divisible by heat capacity) still balances heats without
// dropping anyone.
func TestGenerateHeatsOddFieldSize(t *testing.T) {
	f := newSeedingFixture(t, "100m")
	for i := 0; i < 17; i++ {
		centi := 1100 + i*3
		f.confirmedEntry(t, fmt.Sprintf("D%d", i), "", fmt.Sprintf("%d.%02d", centi/100, centi%100))
	}
	sheet, err := f.results.GenerateHeats(context.Background(), office, f.meetID, f.eventID, f.roundID,
		GenerateHeatsRequest{MaxHeatSize: 8, TrackLanes: 8})
	if err != nil {
		t.Fatalf("GenerateHeats: %v", err)
	}
	total := 0
	for _, u := range sheet.Units {
		if len(u.Rows) > 8 {
			t.Errorf("heat %s exceeds the max heat size: %d", u.UnitID, len(u.Rows))
		}
		total += len(u.Rows)
	}
	if total != 17 {
		t.Errorf("expected all 17 entries seeded, got %d", total)
	}
}

// TestGenerateHeatsRequiresOfficeCapability is a denial/edge-path test.
func TestGenerateHeatsRequiresOfficeCapability(t *testing.T) {
	f := newSeedingFixture(t, "100m")
	f.confirmedEntry(t, "Solo", "", "12.00")
	unauthorized := Session{AccountID: "01SUB", Role: RoleEntrySubmitter}
	if _, err := f.results.GenerateHeats(context.Background(), unauthorized, f.meetID, f.eventID, f.roundID,
		GenerateHeatsRequest{MaxHeatSize: 8, TrackLanes: 8}); err == nil {
		t.Error("expected an authorization error for an entry-submitter generating heats")
	}
}
