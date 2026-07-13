// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"errors"
	"strings"
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

// TestUnitDiscipline checks the web layer's cheap discipline pre-check
// (TASK-021): it resolves the unit's discipline on a real unit, and returns
// a not-found error for a unit id that does not belong to the meet.
func TestUnitDiscipline(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec, unitID := verticalMeet(t, meets)

	disc, err := results.UnitDiscipline(ctx, rec.ID, unitID)
	if err != nil {
		t.Fatalf("UnitDiscipline: %v", err)
	}
	if disc.Code != "HJ" {
		t.Errorf("discipline code = %q, want HJ", disc.Code)
	}

	if _, err := results.UnitDiscipline(ctx, rec.ID, "no-such-unit"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("UnitDiscipline(unknown unit) = %v, want store.ErrNotFound", err)
	}
}

// TestVerticalTrialConflictErrorMessageAndUnwrap covers
// VerticalTrialConflictError's Error() formatting and its Unwrap() to
// ErrConflict — exercised through errors.Is/errors.As the way a caller
// actually consumes it, plus a direct %v format check (D5.2 display, height
// and trial and version all present in the message).
func TestVerticalTrialConflictErrorMessageAndUnwrap(t *testing.T) {
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
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("SaveVerticalTrial conflict = %v, want it to unwrap to ErrConflict", err)
	}
	var conflict *VerticalTrialConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected *VerticalTrialConflictError, got %T", err)
	}
	msg := conflict.Error()
	if !strings.Contains(msg, "height 0") || !strings.Contains(msg, "trial 1") || !strings.Contains(msg, "version 1") {
		t.Errorf("Error() = %q, want it to mention height 0, trial 1 and version 1", msg)
	}
	if !strings.Contains(msg, conflict.Current.Display()) {
		t.Errorf("Error() = %q, want it to include the stored trial's display %q", msg, conflict.Current.Display())
	}
}

// TestSetVerticalHeightsRejectsNonVerticalUnit and
// TestSetVerticalHeightsRejectsEmptyHeights cover the two denial paths
// SetVerticalHeights checks before touching storage: a unit whose
// discipline isn't a vertical-jump family, and an empty heights list
// (SYS-043 requires at least one).
func TestSetVerticalHeightsRejectsNonVerticalUnit(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec, err := meets.CreateMeet(ctx, organizer, MeetRequest{
		Name: "Non-Vertical Meet", Venue: "Fribourg",
		StartDate: ukcDay(), EndDate: ukcDay(),
		CategorySchemeID: domain.SchemeSwissAthletics,
	})
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	if _, err := meets.AddEvent(ctx, organizer, rec.ID, AddEventRequest{
		DisciplineCode: "100m", CategoryCodes: []string{"Men", "Women"},
	}); err != nil {
		t.Fatalf("AddEvent: %v", err)
	}
	detail, err := meets.Meet(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Meet: %v", err)
	}
	var unitID string
	for _, u := range detail.Units {
		if u.DisciplineCode == "100m" {
			unitID = u.UnitID
		}
	}
	if unitID == "" {
		t.Fatal("meet has no 100m unit")
	}

	if _, err := results.SetVerticalHeights(ctx, office, rec.ID, unitID, VerticalHeightsInput{
		Heights: []string{"1.60"},
	}); err == nil {
		t.Error("expected an error configuring vertical heights on a non-vertical unit")
	}
}

func TestSetVerticalHeightsRejectsEmptyHeights(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec, unitID := verticalMeet(t, meets)

	if _, err := results.SetVerticalHeights(ctx, office, rec.ID, unitID, VerticalHeightsInput{
		Heights: nil,
	}); err == nil {
		t.Error("expected an error for an empty heights list (SYS-043)")
	}
}

// TestSaveVerticalTrialDenialPaths covers several of SaveVerticalTrial's
// input-validation denial paths in one pass: an athlete not registered as a
// meet participant, a height index outside the configured progression, and
// an invalid trial (bad sequence number / unrecognized kind) rejected by
// domain.VerticalTrial.Validate.
func TestSaveVerticalTrialDenialPaths(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec, unitID := verticalMeet(t, meets)
	athleteID := registerAthlete(t, results, rec.ID, "Anna", "Muster", domain.SexFemale, 1998)
	if _, err := results.SetVerticalHeights(ctx, office, rec.ID, unitID, VerticalHeightsInput{
		Heights: []string{"1.60", "1.65"},
	}); err != nil {
		t.Fatalf("SetVerticalHeights: %v", err)
	}

	t.Run("unregistered athlete", func(t *testing.T) {
		_, err := results.SaveVerticalTrial(ctx, fieldOfficial, rec.ID, unitID, VerticalTrialInput{
			AthleteID: "no-such-athlete", HeightIdx: 0, Seq: 1, Kind: domain.StatusO,
		})
		if !errors.Is(err, store.ErrNotFound) {
			t.Errorf("SaveVerticalTrial for an unregistered athlete = %v, want store.ErrNotFound", err)
		}
	})

	t.Run("height index outside the configured progression", func(t *testing.T) {
		_, err := results.SaveVerticalTrial(ctx, fieldOfficial, rec.ID, unitID, VerticalTrialInput{
			AthleteID: athleteID, HeightIdx: 5, Seq: 1, Kind: domain.StatusO,
		})
		if err == nil {
			t.Error("expected an error for a height index outside the 2-height progression")
		}
	})

	t.Run("invalid trial number", func(t *testing.T) {
		_, err := results.SaveVerticalTrial(ctx, fieldOfficial, rec.ID, unitID, VerticalTrialInput{
			AthleteID: athleteID, HeightIdx: 0, Seq: 4, Kind: domain.StatusO,
		})
		if err == nil {
			t.Error("expected an error for trial sequence 4 (only 1-3 are legal, TR26.3)")
		}
	})

	t.Run("invalid trial kind", func(t *testing.T) {
		_, err := results.SaveVerticalTrial(ctx, fieldOfficial, rec.ID, unitID, VerticalTrialInput{
			AthleteID: athleteID, HeightIdx: 0, Seq: 1, Kind: domain.QualificationStatus("Q"),
		})
		if err == nil {
			t.Error("expected an error for an unrecognized trial kind")
		}
	})
}

// TestSaveVerticalTrialRejectsNonVerticalUnit covers the family guard on
// the capture path itself (mirrors SetVerticalHeights' equivalent check).
func TestSaveVerticalTrialRejectsNonVerticalUnit(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec, err := meets.CreateMeet(ctx, organizer, MeetRequest{
		Name: "Non-Vertical Meet 2", Venue: "Fribourg",
		StartDate: ukcDay(), EndDate: ukcDay(),
		CategorySchemeID: domain.SchemeSwissAthletics,
	})
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	if _, err := meets.AddEvent(ctx, organizer, rec.ID, AddEventRequest{
		DisciplineCode: "100m", CategoryCodes: []string{"Men", "Women"},
	}); err != nil {
		t.Fatalf("AddEvent: %v", err)
	}
	detail, err := meets.Meet(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Meet: %v", err)
	}
	var unitID string
	for _, u := range detail.Units {
		if u.DisciplineCode == "100m" {
			unitID = u.UnitID
			if err := store.AssignFieldOfficialUnit(ctx, meets.db, fieldOfficial.AccountID, rec.ID, u.UnitID); err != nil {
				t.Fatalf("AssignFieldOfficialUnit: %v", err)
			}
		}
	}
	if unitID == "" {
		t.Fatal("meet has no 100m unit")
	}
	athleteID := registerAthlete(t, results, rec.ID, "Anna", "Muster", domain.SexFemale, 1998)

	if _, err := results.SaveVerticalTrial(ctx, fieldOfficial, rec.ID, unitID, VerticalTrialInput{
		AthleteID: athleteID, HeightIdx: 0, Seq: 1, Kind: domain.StatusO,
	}); err == nil {
		t.Error("expected an error capturing a vertical trial on a non-vertical unit")
	}
}

// TestVerticalCaptureRejectsNonVerticalUnit covers VerticalCapture's own
// family guard (the read-side view assembly).
func TestVerticalCaptureRejectsNonVerticalUnit(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec, err := meets.CreateMeet(ctx, organizer, MeetRequest{
		Name: "Non-Vertical Meet 3", Venue: "Fribourg",
		StartDate: ukcDay(), EndDate: ukcDay(),
		CategorySchemeID: domain.SchemeSwissAthletics,
	})
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	if _, err := meets.AddEvent(ctx, organizer, rec.ID, AddEventRequest{
		DisciplineCode: "100m", CategoryCodes: []string{"Men", "Women"},
	}); err != nil {
		t.Fatalf("AddEvent: %v", err)
	}
	detail, err := meets.Meet(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Meet: %v", err)
	}
	var unitID string
	for _, u := range detail.Units {
		if u.DisciplineCode == "100m" {
			unitID = u.UnitID
		}
	}
	if unitID == "" {
		t.Fatal("meet has no 100m unit")
	}

	if _, err := results.VerticalCapture(ctx, rec.ID, unitID); err == nil {
		t.Error("expected an error assembling a vertical capture view for a non-vertical unit")
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
