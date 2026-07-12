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

// verticalMeet builds a standalone-HJ meet (no scoring table — vertical
// jump capture must work whether or not a meet scores combined points) with
// a single-final HJ event/unit, for UC-012 service-level tests.
func verticalMeet(t *testing.T, meets *MeetService) (MeetRecord, string) {
	t.Helper()
	ctx := context.Background()
	rec, err := meets.CreateMeet(ctx, organizer, MeetRequest{
		Name: "HJ Test Meet", Venue: "Fribourg",
		StartDate: ukcDay(), EndDate: ukcDay(),
		CategorySchemeID: domain.SchemeSwissAthletics,
	})
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	if _, err := meets.AddEvent(ctx, organizer, rec.ID, AddEventRequest{
		DisciplineCode: "HJ", CategoryCodes: []string{"Men", "Women"},
	}); err != nil {
		t.Fatalf("AddEvent: %v", err)
	}
	detail, err := meets.Meet(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Meet: %v", err)
	}
	for _, u := range detail.Units {
		if u.DisciplineCode == "HJ" {
			if err := store.AssignFieldOfficialUnit(ctx, meets.db, fieldOfficial.AccountID, rec.ID, u.UnitID); err != nil {
				t.Fatalf("AssignFieldOfficialUnit: %v", err)
			}
			return rec, u.UnitID
		}
	}
	t.Fatal("meet has no HJ unit")
	return MeetRecord{}, ""
}

func registerAthlete(t *testing.T, results *ResultsService, meetID, first, last string, sex domain.Sex, birthYear int) string {
	t.Helper()
	p, err := results.RegisterParticipant(context.Background(), office, meetID, ParticipantInput{
		FirstName: first, LastName: last, Sex: sex, BirthYear: birthYear, Bib: first + "-" + last,
	})
	if err != nil {
		t.Fatalf("RegisterParticipant: %v", err)
	}
	return p.AthleteID
}

func vTrial(t *testing.T, results *ResultsService, meetID, unitID, athleteID string, heightIdx, seq int, kind domain.QualificationStatus) store.VerticalTrialRecord {
	t.Helper()
	rec, err := results.SaveVerticalTrial(context.Background(), fieldOfficial, meetID, unitID, VerticalTrialInput{
		AthleteID: athleteID, HeightIdx: heightIdx, Seq: seq, Kind: kind,
	})
	if err != nil {
		t.Fatalf("SaveVerticalTrial(height %d seq %d %s): %v", heightIdx, seq, kind, err)
	}
	return rec
}

// TestVerticalCaptureRequiresHeightsSYS043UC012 checks that capturing a
// trial before the office configures the progression fails clearly
// (SYS-043: "bar heights sequence... office-configurable").
func TestVerticalCaptureRequiresHeightsSYS043UC012(t *testing.T) {
	meets, results, _ := newTestResults(t)
	rec, unitID := verticalMeet(t, meets)
	athleteID := registerAthlete(t, results, rec.ID, "Anna", "Muster", domain.SexFemale, 1998)

	_, err := results.SaveVerticalTrial(context.Background(), fieldOfficial, rec.ID, unitID, VerticalTrialInput{
		AthleteID: athleteID, HeightIdx: 0, Seq: 1, Kind: domain.StatusO,
	})
	if !errors.Is(err, ErrVerticalHeightsNotConfigured) {
		t.Fatalf("SaveVerticalTrial before configuration = %v, want ErrVerticalHeightsNotConfigured", err)
	}
}

// TestSetVerticalHeightsSYS043 checks the office-configuration flow: only
// office-level accounts may configure heights, an existing progression may
// only be extended (not rewritten), and extension supports the UC-012 #3
// jump-off flow.
func TestSetVerticalHeightsSYS043(t *testing.T) {
	meets, results, _ := newTestResults(t)
	rec, unitID := verticalMeet(t, meets)
	ctx := context.Background()

	_, err := results.SetVerticalHeights(ctx, fieldOfficial, rec.ID, unitID, VerticalHeightsInput{
		Heights: []string{"1.60"},
	})
	if _, ok := err.(ErrForbidden); !ok {
		t.Errorf("field official configuring heights = %v, want ErrForbidden", err)
	}

	cfg, err := results.SetVerticalHeights(ctx, office, rec.ID, unitID, VerticalHeightsInput{
		Heights: []string{"1.60", "1.65", "1.70"},
	})
	if err != nil {
		t.Fatalf("SetVerticalHeights: %v", err)
	}
	if cfg.Version != 1 || len(cfg.Heights) != 3 {
		t.Fatalf("initial config = %+v", cfg)
	}

	// Rewriting an already-configured height is rejected.
	if _, err := results.SetVerticalHeights(ctx, office, rec.ID, unitID, VerticalHeightsInput{
		Heights: []string{"1.61", "1.65", "1.70"}, ExpectedVersion: cfg.Version,
	}); err == nil {
		t.Error("rewriting an existing height: want error")
	}

	// Extending (jump-off) succeeds.
	extended, err := results.SetVerticalHeights(ctx, office, rec.ID, unitID, VerticalHeightsInput{
		Heights: []string{"1.60", "1.65", "1.70", "1.72"}, ExpectedVersion: cfg.Version,
	})
	if err != nil {
		t.Fatalf("SetVerticalHeights (extend): %v", err)
	}
	if len(extended.Heights) != 4 {
		t.Errorf("extended heights = %v", extended.Heights)
	}
}

// TestVerticalCaptureUC012_1 replays the UC-012 #1 worked example through
// the full service (SaveVerticalTrial + VerticalCapture): heights
// 1.60/1.65/1.70/1.75 with O, XO, XXO, XXX — best 1.70, eliminated at 1.75.
func TestVerticalCaptureUC012_1(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec, unitID := verticalMeet(t, meets)
	athleteID := registerAthlete(t, results, rec.ID, "Anna", "Muster", domain.SexFemale, 1998)
	if _, err := results.SetVerticalHeights(ctx, office, rec.ID, unitID, VerticalHeightsInput{
		Heights: []string{"1.60", "1.65", "1.70", "1.75"},
	}); err != nil {
		t.Fatalf("SetVerticalHeights: %v", err)
	}

	vTrial(t, results, rec.ID, unitID, athleteID, 0, 1, domain.StatusO)
	vTrial(t, results, rec.ID, unitID, athleteID, 1, 1, domain.StatusX)
	vTrial(t, results, rec.ID, unitID, athleteID, 1, 2, domain.StatusO)
	vTrial(t, results, rec.ID, unitID, athleteID, 2, 1, domain.StatusX)
	vTrial(t, results, rec.ID, unitID, athleteID, 2, 2, domain.StatusX)
	vTrial(t, results, rec.ID, unitID, athleteID, 2, 3, domain.StatusO)
	vTrial(t, results, rec.ID, unitID, athleteID, 3, 1, domain.StatusX)
	vTrial(t, results, rec.ID, unitID, athleteID, 3, 2, domain.StatusX)
	last := vTrial(t, results, rec.ID, unitID, athleteID, 3, 3, domain.StatusX)
	if last.Version != 1 {
		t.Errorf("last trial version = %d, want 1 (a new cell)", last.Version)
	}

	view, err := results.VerticalCapture(ctx, rec.ID, unitID)
	if err != nil {
		t.Fatalf("VerticalCapture: %v", err)
	}
	if len(view.Standings) != 1 {
		t.Fatalf("standings = %+v, want exactly one row", view.Standings)
	}
	st := view.Standings[0]
	if st.BestHeight != "1.70" {
		t.Errorf("best height = %q, want 1.70", st.BestHeight)
	}
	if !st.Eliminated {
		t.Error("expected Eliminated true")
	}
	if len(view.Rows) != 1 || view.Rows[0].Result == nil || view.Rows[0].Result.Mark != "1.70" {
		t.Fatalf("settled result = %+v", view.Rows)
	}
	if view.Rows[0].Cells[2][2].Kind != domain.StatusO {
		t.Errorf("cell (height 2, trial 3) = %q, want O", view.Rows[0].Cells[2][2].Kind)
	}
}

// TestVerticalCaptureCountbackAndTieSYS043UC012_3 exercises the countback
// ranking and the first-place tie flag through the full capture service
// (two athletes on the same unit).
func TestVerticalCaptureCountbackAndTieSYS043UC012_3(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec, unitID := verticalMeet(t, meets)
	a := registerAthlete(t, results, rec.ID, "Anna", "Muster", domain.SexFemale, 1998)
	b := registerAthlete(t, results, rec.ID, "Beat", "Schmid", domain.SexMale, 1997)
	if _, err := results.SetVerticalHeights(ctx, office, rec.ID, unitID, VerticalHeightsInput{
		Heights: []string{"1.80", "1.83"},
	}); err != nil {
		t.Fatalf("SetVerticalHeights: %v", err)
	}

	// A clears both heights first try; B needs a second try at 1.83.
	vTrial(t, results, rec.ID, unitID, a, 0, 1, domain.StatusO)
	vTrial(t, results, rec.ID, unitID, a, 1, 1, domain.StatusO)
	vTrial(t, results, rec.ID, unitID, b, 0, 1, domain.StatusO)
	vTrial(t, results, rec.ID, unitID, b, 1, 1, domain.StatusX)
	vTrial(t, results, rec.ID, unitID, b, 1, 2, domain.StatusO)

	view, err := results.VerticalCapture(ctx, rec.ID, unitID)
	if err != nil {
		t.Fatalf("VerticalCapture: %v", err)
	}
	if len(view.Standings) != 2 {
		t.Fatalf("standings = %+v", view.Standings)
	}
	if view.Standings[0].AthleteID != a || view.Standings[0].Rank != 1 {
		t.Fatalf("expected A ranked first (fewer attempts at 1.83), got %+v", view.Standings)
	}
	if view.Standings[0].TieForFirst {
		t.Error("no tie expected here")
	}
	if view.Standings[1].AthleteID != b || view.Standings[1].Rank != 2 {
		t.Fatalf("expected B ranked second, got %+v", view.Standings)
	}
}

// TestVerticalCaptureVersionConflictSYS086UC021 mirrors the horizontal
// field-attempt conflict test at the vertical-capture service level
// (UC-021 #2).
func TestVerticalCaptureVersionConflictSYS086UC021(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec, unitID := verticalMeet(t, meets)
	athleteID := registerAthlete(t, results, rec.ID, "Anna", "Muster", domain.SexFemale, 1998)
	if _, err := results.SetVerticalHeights(ctx, office, rec.ID, unitID, VerticalHeightsInput{
		Heights: []string{"1.60"},
	}); err != nil {
		t.Fatalf("SetVerticalHeights: %v", err)
	}
	vTrial(t, results, rec.ID, unitID, athleteID, 0, 1, domain.StatusO)

	_, err := results.SaveVerticalTrial(ctx, fieldOfficial, rec.ID, unitID, VerticalTrialInput{
		AthleteID: athleteID, HeightIdx: 0, Seq: 1, Kind: domain.StatusX,
	})
	var conflict *VerticalTrialConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("double insert = %v, want *VerticalTrialConflictError", err)
	}
	if conflict.Current.Kind != domain.StatusO {
		t.Errorf("conflict must carry the stored trial, got %+v", conflict.Current)
	}
}

// TestVerticalCaptureRespectsAnnouncementGuardUC015 checks that vertical
// capture is gated by the same SYS-047 announce/correction guard as
// track/horizontal capture — TASK-021 must not destabilize it.
func TestVerticalCaptureRespectsAnnouncementGuardUC015(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec, unitID := verticalMeet(t, meets)
	athleteID := registerAthlete(t, results, rec.ID, "Anna", "Muster", domain.SexFemale, 1998)
	if _, err := results.SetVerticalHeights(ctx, office, rec.ID, unitID, VerticalHeightsInput{
		Heights: []string{"1.60"},
	}); err != nil {
		t.Fatalf("SetVerticalHeights: %v", err)
	}
	vTrial(t, results, rec.ID, unitID, athleteID, 0, 1, domain.StatusO)

	if _, err := results.AnnounceUnitResults(ctx, office, rec.ID, unitID); err != nil {
		t.Fatalf("AnnounceUnitResults: %v", err)
	}
	_, err := results.SaveVerticalTrial(ctx, fieldOfficial, rec.ID, unitID, VerticalTrialInput{
		AthleteID: athleteID, HeightIdx: 0, Seq: 2, Kind: domain.StatusX,
	})
	if !errors.Is(err, ErrCorrectionRequired) {
		t.Fatalf("capture after announcement = %v, want ErrCorrectionRequired", err)
	}
}

// TestCaptureUnitsIncludesVerticalSYS043 checks the capture index now
// lists vertical units alongside track/horizontal ones.
func TestCaptureUnitsIncludesVerticalSYS043(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec, unitID := verticalMeet(t, meets)

	units, err := results.CaptureUnits(ctx, office, rec.ID)
	if err != nil {
		t.Fatalf("CaptureUnits: %v", err)
	}
	found := false
	for _, u := range units {
		if u.UnitID == unitID {
			found = true
			if u.Family != domain.FamilyFieldVertical {
				t.Errorf("unit family = %q, want field-vertical", u.Family)
			}
		}
	}
	if !found {
		t.Fatalf("vertical unit %s not in capture index: %+v", unitID, units)
	}
}
