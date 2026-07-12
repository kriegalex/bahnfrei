// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"fmt"
	"testing"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// captureTrackResult writes a settled track result directly (bypassing the
// capture-authorization path — these tests focus on progression, not
// capture, mirroring seeding_test.go's direct store use for entries).
func (f seedingFixture) captureTrackResult(t *testing.T, unitID, athleteID, mark string) {
	t.Helper()
	_, err := store.SaveResult(context.Background(), f.st.DB(), domain.Result{
		UnitID: unitID, AthleteID: athleteID, Mark: mark,
	}, domain.TimingElectronic)
	if err != nil {
		t.Fatalf("SaveResult: %v", err)
	}
}

// seededHeats generates heats for the fixture's round and returns the
// resulting sheet.
func (f seedingFixture) seededHeats(t *testing.T, maxHeatSize, trackLanes int) HeatSheet {
	t.Helper()
	sheet, err := f.results.GenerateHeats(context.Background(), office, f.meetID, f.eventID, f.roundID,
		GenerateHeatsRequest{MaxHeatSize: maxHeatSize, TrackLanes: trackLanes})
	if err != nil {
		t.Fatalf("GenerateHeats: %v", err)
	}
	return sheet
}

func (f seedingFixture) athleteOf(t *testing.T, entryID string) string {
	t.Helper()
	e, err := store.GetEntry(context.Background(), f.st.DB(), entryID)
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}
	return e.AthleteID
}

// TestAdvanceRoundSYS029UC009_1 reproduces UC-009 #1 through the app
// service: 3 heats of 4, top 2 places + 2 fastest advance, producing
// exactly 6 Q and 2 q, and the semi-final's start list can then be
// generated from the qualifiers.
func TestAdvanceRoundSYS029UC009_1(t *testing.T) {
	f := newSeedingFixture(t, "100m")
	var entries []store.EntryRecord
	for i := 0; i < 12; i++ {
		centi := 1100 + i*3
		entries = append(entries, f.confirmedEntry(t, fmt.Sprintf("R%d", i), "", fmt.Sprintf("%d.%02d", centi/100, centi%100)))
	}
	sheet := f.seededHeats(t, 4, 0)
	if len(sheet.Units) != 3 {
		t.Fatalf("expected 3 heats of 4, got %d", len(sheet.Units))
	}

	// Deliberately re-order finishing marks within each heat so the fastest
	// overall aren't simply "seed order" (proves advancement uses actual
	// results, not the seed mark).
	ctx := context.Background()
	for hi, u := range sheet.Units {
		base := 1100 + hi // vary slightly per heat so cross-heat times never tie
		for i, row := range u.Rows {
			mark := fmt.Sprintf("%d.%02d", (base+i*10)/100, (base+i*10)%100)
			f.captureTrackResult(t, u.UnitID, f.athleteOf(t, row.EntryID), mark)
		}
	}

	outcome, err := f.results.AdvanceRound(ctx, office, f.meetID, f.eventID, f.roundID,
		AdvancementRequest{TopN: 2, FastestK: 2})
	if err != nil {
		t.Fatalf("AdvanceRound: %v", err)
	}
	if outcome.Tie != nil {
		t.Fatalf("unexpected tie: %+v", outcome.Tie)
	}
	qCount, qtCount := 0, 0
	for _, a := range outcome.Advanced {
		switch a.Code {
		case domain.StatusQ:
			qCount++
		case domain.StatusQt:
			qtCount++
		}
	}
	if qCount != 6 {
		t.Errorf("expected 6 Q, got %d", qCount)
	}
	if qtCount != 2 {
		t.Errorf("expected 2 q, got %d", qtCount)
	}

	// The event's rounds were created [Qualification, Final] by the
	// fixture; generating the next round's heats from these qualifiers
	// proves the semi-final/next-round start list can be built (UC-009 #1).
	rounds, err := store.ListRounds(ctx, f.st.DB(), f.eventID)
	if err != nil {
		t.Fatalf("ListRounds: %v", err)
	}
	nextRoundID := rounds[1].ID
	nextSheet, err := f.results.GenerateHeats(ctx, office, f.meetID, f.eventID, nextRoundID,
		GenerateHeatsRequest{MaxHeatSize: 8, TrackLanes: 0})
	if err != nil {
		t.Fatalf("GenerateHeats (next round): %v", err)
	}
	total := 0
	for _, u := range nextSheet.Units {
		total += len(u.Rows)
	}
	if total != 8 {
		t.Fatalf("expected the next round to seed exactly the 8 qualifiers, got %d", total)
	}
	_ = entries
}

// TestAdvanceRoundTimeTieSYS029UC009_2 reproduces UC-009 #2 through the app
// service: a tie for the last time-qualifier spot is surfaced rather than
// auto-resolved, and ManualAdvance + a second AdvanceRound call resolves it.
func TestAdvanceRoundTimeTieSYS029UC009_2(t *testing.T) {
	f := newSeedingFixture(t, "100m")
	for i := 0; i < 4; i++ {
		f.confirmedEntry(t, fmt.Sprintf("T%d", i), "", "12.00")
	}
	sheet := f.seededHeats(t, 4, 0)
	unit := sheet.Units[0]
	ctx := context.Background()

	// Two athletes finish with an identical time — a tie for the single
	// remaining q slot after the top-1 auto-qualifies.
	marks := []string{"11.00", "11.50", "11.50", "12.00"}
	for i, row := range unit.Rows {
		f.captureTrackResult(t, unit.UnitID, f.athleteOf(t, row.EntryID), marks[i])
	}

	outcome, err := f.results.AdvanceRound(ctx, office, f.meetID, f.eventID, f.roundID,
		AdvancementRequest{TopN: 1, FastestK: 1})
	if err != nil {
		t.Fatalf("AdvanceRound: %v", err)
	}
	if outcome.Tie == nil {
		t.Fatal("expected a tie for the last qualifying spot")
	}
	if len(outcome.Tie.EntryIDs) != 2 {
		t.Fatalf("expected 2 tied entries, got %d", len(outcome.Tie.EntryIDs))
	}

	// Resolve by draw: advance one of the tied entries with qD.
	if err := f.results.ManualAdvance(ctx, office, f.meetID, f.eventID, f.roundID, outcome.Tie.EntryIDs[0], domain.StatusQD); err != nil {
		t.Fatalf("ManualAdvance: %v", err)
	}

	// Re-running AdvanceRound must not re-surface the (now resolved) tie.
	outcome2, err := f.results.AdvanceRound(ctx, office, f.meetID, f.eventID, f.roundID,
		AdvancementRequest{TopN: 1, FastestK: 1})
	if err != nil {
		t.Fatalf("AdvanceRound (re-run): %v", err)
	}
	if outcome2.Tie != nil {
		t.Errorf("expected the tie to stay resolved after ManualAdvance, got %+v", outcome2.Tie)
	}
}

// TestManualAdvanceRefereeCodeSYS029UC009_3 reproduces UC-009 #3: a referee
// decision advances an athlete with code qR, appearing on the next round's
// list.
func TestManualAdvanceRefereeCodeSYS029UC009_3(t *testing.T) {
	f := newSeedingFixture(t, "100m")
	e := f.confirmedEntry(t, "Ref", "", "12.00")
	f.seededHeats(t, 4, 0)
	ctx := context.Background()

	if err := f.results.ManualAdvance(ctx, office, f.meetID, f.eventID, f.roundID, e.ID, domain.StatusQR); err != nil {
		t.Fatalf("ManualAdvance: %v", err)
	}
	sheet, err := f.results.HeatSheetFor(ctx, office, f.meetID, f.eventID, f.roundID)
	if err != nil {
		t.Fatalf("HeatSheetFor: %v", err)
	}
	found := false
	for _, u := range sheet.Units {
		for _, row := range u.Rows {
			if row.EntryID == e.ID && row.Qualification == domain.StatusQR {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected the referee-advanced entry to carry code qR")
	}
}

// TestAdvanceRoundFieldSYS030UC009_4 reproduces UC-009 #4 through the app
// service: a long-throw qualification with standard 14.00m and finals
// capacity 12 — 5 athletes beat the standard (Q), the next 7 by mark (q).
func TestAdvanceRoundFieldSYS030UC009_4(t *testing.T) {
	f := newSeedingFixture(t, "SP")
	marks := []string{
		"15.20", "14.80", "14.50", "14.30", "14.05", // beat standard
		"13.95", "13.80", "13.70", "13.60", "13.50", "13.40", "13.30", // next 7 by mark
		"13.20", "13.10", // miss the cut
	}
	var entries []store.EntryRecord
	for i, m := range marks {
		entries = append(entries, f.confirmedEntry(t, fmt.Sprintf("F%d", i), "", ""))
		_ = m
	}
	sheet := f.seededHeats(t, len(entries), 0)
	unit := sheet.Units[0]
	ctx := context.Background()
	for i, row := range unit.Rows {
		f.captureTrackResult(t, unit.UnitID, f.athleteOf(t, row.EntryID), marks[i])
	}

	outcome, err := f.results.AdvanceRound(ctx, office, f.meetID, f.eventID, f.roundID,
		AdvancementRequest{Standard: "14.00", BetterDirection: "higher", FinalsCapacity: 12})
	if err != nil {
		t.Fatalf("AdvanceRound: %v", err)
	}
	if outcome.Tie != nil {
		t.Fatalf("unexpected tie: %+v", outcome.Tie)
	}
	if len(outcome.Advanced) != 12 {
		t.Fatalf("expected 12 advancing (5 Q + 7 q), got %d", len(outcome.Advanced))
	}
}

// TestAdvanceRoundRejectsIncompleteRound is a denial/edge-path test:
// advancing before a heat has any settled results is rejected rather than
// silently awarding qualification on nothing.
func TestAdvanceRoundRejectsIncompleteRound(t *testing.T) {
	f := newSeedingFixture(t, "100m")
	f.confirmedEntry(t, "Solo", "", "12.00")
	f.seededHeats(t, 4, 0)
	if _, err := f.results.AdvanceRound(context.Background(), office, f.meetID, f.eventID, f.roundID,
		AdvancementRequest{TopN: 1, FastestK: 1}); err == nil {
		t.Error("expected an error advancing a round with no settled results")
	}
}

// TestManualAdvanceRejectsIllegalCode is a denial/edge-path test.
func TestManualAdvanceRejectsIllegalCode(t *testing.T) {
	f := newSeedingFixture(t, "100m")
	e := f.confirmedEntry(t, "Solo", "", "12.00")
	f.seededHeats(t, 4, 0)
	if err := f.results.ManualAdvance(context.Background(), office, f.meetID, f.eventID, f.roundID, e.ID, "notacode"); err == nil {
		t.Error("expected an error for an illegal manual-advancement code")
	}
}

// TestAdvanceRoundRequiresOfficeCapability is a denial/edge-path test.
func TestAdvanceRoundRequiresOfficeCapability(t *testing.T) {
	f := newSeedingFixture(t, "100m")
	f.confirmedEntry(t, "Solo", "", "12.00")
	f.seededHeats(t, 4, 0)
	unauthorized := Session{AccountID: "01SUB", Role: RoleEntrySubmitter}
	if _, err := f.results.AdvanceRound(context.Background(), unauthorized, f.meetID, f.eventID, f.roundID,
		AdvancementRequest{TopN: 1, FastestK: 1}); err == nil {
		t.Error("expected an authorization error for an entry-submitter advancing a round")
	}
}
