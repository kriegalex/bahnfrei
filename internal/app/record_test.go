// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// recordMeetDay is the fixture meet start date every test in this file
// uses: chosen so a 2010 birth year resolves to "U18 W"/"U18 M" and a 2012
// birth year resolves to "U16 M" in the swiss-athletics scheme (age =
// asOf.Year() - birthYear).
func recordMeetDay() time.Time { return time.Date(2027, 6, 10, 0, 0, 0, 0, time.UTC) }

// plainMeet creates a bare (non-template) swiss-athletics-scheme meet —
// the shape UC-016/UC-028's plain-meet scenarios need, as opposed to
// createUKCMeet's fixed 3-discipline template.
func plainMeet(t *testing.T, meets *MeetService) MeetRecord {
	t.Helper()
	rec, err := meets.CreateMeet(context.Background(), organizer, MeetRequest{
		Name: "Abendmeeting Uster", Venue: "Stadion Buchholz", HomologationRef: "CH-ZH-042",
		StartDate: recordMeetDay(), EndDate: recordMeetDay(),
		Tier: "C-Meeting", CategorySchemeID: domain.SchemeSwissAthletics,
	})
	if err != nil {
		t.Fatalf("CreateMeet: %v", err)
	}
	return rec
}

// addTrackEvent adds one discipline/category event (a single final round,
// one unit) and returns that unit's ID.
func addTrackEvent(t *testing.T, meets *MeetService, meetID, disciplineCode string, categoryCodes []string) string {
	t.Helper()
	ctx := context.Background()
	if _, err := meets.AddEvent(ctx, organizer, meetID, AddEventRequest{
		DisciplineCode: disciplineCode, CategoryCodes: categoryCodes,
	}); err != nil {
		t.Fatalf("AddEvent(%s, %v): %v", disciplineCode, categoryCodes, err)
	}
	detail, err := meets.Meet(ctx, meetID)
	if err != nil {
		t.Fatalf("Meet: %v", err)
	}
	// The most recently added unit is last in the (event-ordered) list.
	return detail.Units[len(detail.Units)-1].UnitID
}

// grantCapture assigns the shared fieldOfficial test account capture
// access to unitID (SYS-090 per-event scoping), mirroring unitOf's grant
// without requiring the discipline-by-name lookup unitOf does.
func grantCapture(t *testing.T, results *ResultsService, meetID, unitID string) {
	t.Helper()
	if err := store.AssignFieldOfficialUnit(context.Background(), results.db, fieldOfficial.AccountID, meetID, unitID); err != nil {
		t.Fatalf("AssignFieldOfficialUnit: %v", err)
	}
}

func exampleU18WMeetingRecordList(meetID string) map[string]*domain.RecordList {
	return map[string]*domain.RecordList{
		"test-mr": {
			ID: "test-mr", Version: "1", Name: "Test meeting records",
			Records: []domain.RecordReference{
				{ID: "r-100m-u18w", Type: domain.RecordMeeting, CategoryCode: "U18 W", DisciplineCode: "100m", Mark: "11.90", MeetID: meetID},
			},
		},
	}
}

func containsFlag(flags []string, want string) bool {
	for _, f := range flags {
		if f == want {
			return true
		}
	}
	return false
}

func wind(t *testing.T, results *ResultsService, meetID, unitID string, reading float64) {
	t.Helper()
	if err := results.SetUnitWind(context.Background(), fieldOfficial, meetID, unitID, reading); err != nil {
		t.Fatalf("SetUnitWind: %v", err)
	}
}

// TestUC016_1_MeetingRecordFlaggedLegalFAT covers UC-016 #1: a loaded
// meeting-record list with 100m U18 W = 11.90; a legal (+1.1), FAT 11.85
// flags MR in the settled result, the operator capture standings, and the
// public/exports-facing Standings() (SYS-049).
func TestUC016_1_MeetingRecordFlaggedLegalFAT(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := plainMeet(t, meets)
	unitID := addTrackEvent(t, meets, rec.ID, "100m", []string{"U18 W"})
	grantCapture(t, results, rec.ID, unitID)
	results.SetRecordLists(exampleU18WMeetingRecordList(rec.ID))
	if err := results.SetMeetRecordLists(ctx, organizer, rec.ID, []string{"test-mr"}); err != nil {
		t.Fatalf("SetMeetRecordLists: %v", err)
	}
	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2010, Sex: domain.SexFemale, Bib: "1",
	})
	wind(t, results, rec.ID, unitID, 1.1)

	saved, err := results.SaveTrackResult(ctx, fieldOfficial, rec.ID, unitID, TrackResultInput{
		AthleteID: anna.AthleteID, Time: "11.85", Timing: domain.TimingElectronic,
	})
	if err != nil {
		t.Fatalf("SaveTrackResult: %v", err)
	}
	if len(saved.RecordFlags) != 1 || saved.RecordFlags[0] != "MR" {
		t.Fatalf("saved result flags = %v, want [MR]", saved.RecordFlags)
	}

	// Operator view (capture standings).
	uc, err := results.UnitCapture(ctx, rec.ID, unitID)
	if err != nil {
		t.Fatalf("UnitCapture: %v", err)
	}
	if len(uc.Standings) != 1 || len(uc.Standings[0].RecordFlags) != 1 || uc.Standings[0].RecordFlags[0] != "MR" {
		t.Fatalf("operator standings = %+v, want one row flagged MR", uc.Standings)
	}

	// Public/exports-facing standings (Standings() feeds public results and
	// the printed result list, SYS-049 "public results, and exports").
	standings, err := results.Standings(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Standings: %v", err)
	}
	found := false
	for _, div := range standings.Divisions {
		for _, row := range div.Rows {
			for _, m := range row.Marks {
				if m.DisciplineCode == "100m" {
					if len(m.RecordFlags) != 1 || m.RecordFlags[0] != "MR" {
						t.Errorf("standings mark = %+v, want flagged MR", m)
					}
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("100m performance not found in Standings()")
	}
}

// TestUC016_2_WindAssistedNoFlagButPlacingStands covers UC-016 #2: the same
// mark with wind +2.4 sets no record flag, but the result still ranks
// (placing stands).
func TestUC016_2_WindAssistedNoFlagButPlacingStands(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := plainMeet(t, meets)
	unitID := addTrackEvent(t, meets, rec.ID, "100m", []string{"U18 W"})
	grantCapture(t, results, rec.ID, unitID)
	results.SetRecordLists(exampleU18WMeetingRecordList(rec.ID))
	if err := results.SetMeetRecordLists(ctx, organizer, rec.ID, []string{"test-mr"}); err != nil {
		t.Fatalf("SetMeetRecordLists: %v", err)
	}
	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2010, Sex: domain.SexFemale, Bib: "1",
	})
	bea := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Bea", LastName: "Beispiel", BirthYear: 2010, Sex: domain.SexFemale, Bib: "2",
	})
	wind(t, results, rec.ID, unitID, 2.4)

	saved, err := results.SaveTrackResult(ctx, fieldOfficial, rec.ID, unitID, TrackResultInput{
		AthleteID: anna.AthleteID, Time: "11.85", Timing: domain.TimingElectronic,
	})
	if err != nil {
		t.Fatalf("SaveTrackResult: %v", err)
	}
	if len(saved.RecordFlags) != 0 {
		t.Fatalf("flags = %v, want none (wind-assisted, +2.4)", saved.RecordFlags)
	}
	if _, err := results.SaveTrackResult(ctx, fieldOfficial, rec.ID, unitID, TrackResultInput{
		AthleteID: bea.AthleteID, Time: "12.50", Timing: domain.TimingElectronic,
	}); err != nil {
		t.Fatalf("SaveTrackResult (bea): %v", err)
	}

	uc, err := results.UnitCapture(ctx, rec.ID, unitID)
	if err != nil {
		t.Fatalf("UnitCapture: %v", err)
	}
	if len(uc.Standings) != 2 || uc.Standings[0].AthleteID != anna.AthleteID || uc.Standings[0].Rank != 1 {
		t.Fatalf("standings = %+v, want Anna ranked first despite the unflagged wind-assisted mark", uc.Standings)
	}
	if len(uc.Standings[0].RecordFlags) != 0 {
		t.Errorf("Anna's flags = %v, want none", uc.Standings[0].RecordFlags)
	}
}

// TestUC016_3_HandTimedNoFlagReasonInspectable covers UC-016 #3: a
// hand-timed mark better than a record requiring FAT (100m, <=800m) sets no
// flag, and the reason is inspectable (domain.EvaluateRecord's own outcome,
// exercised end to end through the capture save path here).
func TestUC016_3_HandTimedNoFlagReasonInspectable(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := plainMeet(t, meets)
	unitID := addTrackEvent(t, meets, rec.ID, "100m", []string{"U18 W"})
	grantCapture(t, results, rec.ID, unitID)
	results.SetRecordLists(exampleU18WMeetingRecordList(rec.ID))
	if err := results.SetMeetRecordLists(ctx, organizer, rec.ID, []string{"test-mr"}); err != nil {
		t.Fatalf("SetMeetRecordLists: %v", err)
	}
	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2010, Sex: domain.SexFemale, Bib: "1",
	})
	wind(t, results, rec.ID, unitID, 1.0) // legal, isolates the FAT reason

	saved, err := results.SaveTrackResult(ctx, fieldOfficial, rec.ID, unitID, TrackResultInput{
		AthleteID: anna.AthleteID, Time: "11.80", Timing: domain.TimingManual,
	})
	if err != nil {
		t.Fatalf("SaveTrackResult: %v", err)
	}
	if len(saved.RecordFlags) != 0 {
		t.Fatalf("flags = %v, want none (hand-timed, FAT required for <=800m)", saved.RecordFlags)
	}

	// The reason is inspectable: re-running the same evaluation domain-side
	// with the mark/timing this save used reports why (UC-016 #3).
	eval := domain.EvaluateRecord(domain.RecordEvaluationInput{
		Family: domain.FamilyTrack, DisciplineCode: "100m", Mark: saved.Mark,
		Timing: saved.Timing, WindRelevant: true,
		References: []domain.RecordReference{{Type: domain.RecordMeeting, CategoryCode: "U18 W", DisciplineCode: "100m", Mark: "11.90"}},
	})
	if len(eval.Outcomes) != 1 || eval.Outcomes[0].Flagged || eval.Outcomes[0].Reason != domain.ReasonRequiresFAT {
		t.Fatalf("outcome = %+v, want an unflagged requires_fat_timing reason", eval.Outcomes)
	}
}

// TestUC016_4_RecordChecklistGeneratedWithGaps covers UC-016 #4: the
// generated checklist for a flagged record contains the Rekordprotokoll
// data available in-system, with gaps explicitly marked for manual
// completion.
func TestUC016_4_RecordChecklistGeneratedWithGaps(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := plainMeet(t, meets)
	unitID := addTrackEvent(t, meets, rec.ID, "100m", []string{"U18 W"})
	grantCapture(t, results, rec.ID, unitID)
	results.SetRecordLists(exampleU18WMeetingRecordList(rec.ID))
	if err := results.SetMeetRecordLists(ctx, organizer, rec.ID, []string{"test-mr"}); err != nil {
		t.Fatalf("SetMeetRecordLists: %v", err)
	}
	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2010, Sex: domain.SexFemale, Bib: "1",
	})
	bea := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Bea", LastName: "Beispiel", BirthYear: 2010, Sex: domain.SexFemale, Bib: "2",
	})
	wind(t, results, rec.ID, unitID, 1.1)
	if _, err := results.SaveTrackResult(ctx, fieldOfficial, rec.ID, unitID, TrackResultInput{
		AthleteID: anna.AthleteID, Time: "11.85", Timing: domain.TimingElectronic,
	}); err != nil {
		t.Fatalf("SaveTrackResult (anna): %v", err)
	}
	if _, err := results.SaveTrackResult(ctx, fieldOfficial, rec.ID, unitID, TrackResultInput{
		AthleteID: bea.AthleteID, Time: "12.50", Timing: domain.TimingElectronic,
	}); err != nil {
		t.Fatalf("SaveTrackResult (bea): %v", err)
	}

	view, err := results.RecordChecklist(ctx, rec.ID, unitID, anna.AthleteID)
	if err != nil {
		t.Fatalf("RecordChecklist: %v", err)
	}
	if len(view.RecordFlags) != 1 || view.RecordFlags[0] != "MR" {
		t.Fatalf("checklist flags = %v, want [MR]", view.RecordFlags)
	}
	byKey := map[string]domain.RecordChecklistItem{}
	for _, it := range view.Items {
		byKey[it.Key] = it
	}
	if it := byKey[domain.ChecklistTimingClass]; it.Value != "FAT" {
		t.Errorf("timing class = %+v, want FAT", it)
	}
	if it := byKey[domain.ChecklistWind]; it.Value != "+1.1" || it.Gap {
		t.Errorf("wind = %+v, want +1.1/not-a-gap", it)
	}
	if it := byKey[domain.ChecklistCompetitors]; it.Value != "2" {
		t.Errorf("competitors = %+v, want 2", it)
	}
	if it := byKey[domain.ChecklistZeroTest]; !it.Gap {
		t.Errorf("zero-test = %+v, want an explicit gap", it)
	}
	if it := byKey[domain.ChecklistPhotoFinishImage]; !it.Gap {
		t.Errorf("photo-finish image = %+v, want an explicit gap", it)
	}
}

// TestUC016_5_PersonalBestFlaggedFromHistory covers UC-016 #5: an athlete
// with in-system history PB 12.02 running 11.95 legal flags PB.
func TestUC016_5_PersonalBestFlaggedFromHistory(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := plainMeet(t, meets)
	oldUnit := addTrackEvent(t, meets, rec.ID, "100m", []string{"U18 W"})
	newUnit := addTrackEvent(t, meets, rec.ID, "100m", []string{"U18 W"})
	grantCapture(t, results, rec.ID, oldUnit)
	grantCapture(t, results, rec.ID, newUnit)
	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2010, Sex: domain.SexFemale, Bib: "1",
	})
	wind(t, results, rec.ID, oldUnit, 0.0)
	wind(t, results, rec.ID, newUnit, 0.5)

	if _, err := results.SaveTrackResult(ctx, fieldOfficial, rec.ID, oldUnit, TrackResultInput{
		AthleteID: anna.AthleteID, Time: "12.02", Timing: domain.TimingElectronic,
	}); err != nil {
		t.Fatalf("SaveTrackResult (old PB): %v", err)
	}

	saved, err := results.SaveTrackResult(ctx, fieldOfficial, rec.ID, newUnit, TrackResultInput{
		AthleteID: anna.AthleteID, Time: "11.95", Timing: domain.TimingElectronic,
	})
	if err != nil {
		t.Fatalf("SaveTrackResult (new PB): %v", err)
	}
	// Both units fall in the same competition year, so the improved mark is
	// simultaneously a season best — PB and SB flag independently
	// (SYS-049's "SB logic analogous"), both legitimately present here.
	if !containsFlag(saved.RecordFlags, domain.FlagPersonalBest) {
		t.Fatalf("flags = %v, want PB present", saved.RecordFlags)
	}
	if !containsFlag(saved.RecordFlags, domain.FlagSeasonBest) {
		t.Fatalf("flags = %v, want SB present (same season as the prior mark)", saved.RecordFlags)
	}

	// A mark that does NOT better the prior best earns no PB flag.
	worseUnit := addTrackEvent(t, meets, rec.ID, "100m", []string{"U18 W"})
	grantCapture(t, results, rec.ID, worseUnit)
	wind(t, results, rec.ID, worseUnit, 0.5)
	worse, err := results.SaveTrackResult(ctx, fieldOfficial, rec.ID, worseUnit, TrackResultInput{
		AthleteID: anna.AthleteID, Time: "12.10", Timing: domain.TimingElectronic,
	})
	if err != nil {
		t.Fatalf("SaveTrackResult (worse): %v", err)
	}
	if len(worse.RecordFlags) != 0 {
		t.Fatalf("flags = %v, want none (does not better 11.95)", worse.RecordFlags)
	}
}

// TestSetMeetRecordListsAuthorizationAndValidation covers the meet-setup
// configuration path: organizer-only, and every listed ID must be wired.
func TestSetMeetRecordListsAuthorizationAndValidation(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := plainMeet(t, meets)
	results.SetRecordLists(exampleU18WMeetingRecordList(rec.ID))

	var forbidden ErrForbidden
	if err := results.SetMeetRecordLists(ctx, fieldOfficial, rec.ID, []string{"test-mr"}); !errors.As(err, &forbidden) {
		t.Errorf("as field official = %v, want ErrForbidden", err)
	}
	if err := results.SetMeetRecordLists(ctx, organizer, rec.ID, []string{"no-such-list"}); err == nil {
		t.Error("unknown record list: want error")
	}
	if err := results.SetMeetRecordLists(ctx, organizer, rec.ID, []string{"test-mr"}); err != nil {
		t.Fatalf("SetMeetRecordLists: %v", err)
	}
	ids, err := results.MeetRecordListIDs(ctx, rec.ID)
	if err != nil || len(ids) != 1 || ids[0] != "test-mr" {
		t.Errorf("MeetRecordListIDs = %v, %v, want [test-mr]", ids, err)
	}
	opts := results.RecordListOptions()
	if len(opts) != 1 || opts[0].ID != "test-mr" {
		t.Errorf("RecordListOptions = %+v, want [test-mr]", opts)
	}
}

// TestUC028_1_MixedCategoryUnitCombinedAndSplit covers UC-028 #1: a 100m
// unit combining U16 M and U18 M entrants (for lack of entries) produces
// both the combined race result and per-category re-ranked presentations.
func TestUC028_1_MixedCategoryUnitCombinedAndSplit(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := plainMeet(t, meets)
	unitID := addTrackEvent(t, meets, rec.ID, "100m", []string{"U16 M", "U18 M"})
	grantCapture(t, results, rec.ID, unitID)

	// U16 M (birth year 2012): Cyril 12.50, Dan 12.80.
	// U18 M (birth year 2010): Ali 11.20, Bob 11.50.
	type entrant struct {
		first, last string
		birthYear   int
		bib, time   string
	}
	entrants := []entrant{
		{"Ali", "Amrein", 2010, "3", "11.20"},
		{"Bob", "Blatter", 2010, "4", "11.50"},
		{"Cyril", "Curty", 2012, "1", "12.50"},
		{"Dan", "Diethelm", 2012, "2", "12.80"},
	}
	byName := map[string]store.ParticipantRow{}
	for _, e := range entrants {
		p := register(t, results, rec.ID, ParticipantInput{
			FirstName: e.first, LastName: e.last, BirthYear: e.birthYear, Sex: domain.SexMale, Bib: e.bib,
		})
		byName[e.first] = p
		if _, err := results.SaveTrackResult(ctx, fieldOfficial, rec.ID, unitID, TrackResultInput{
			AthleteID: p.AthleteID, Time: e.time, Timing: domain.TimingElectronic,
		}); err != nil {
			t.Fatalf("SaveTrackResult(%s): %v", e.first, err)
		}
	}

	uc, err := results.UnitCapture(ctx, rec.ID, unitID)
	if err != nil {
		t.Fatalf("UnitCapture: %v", err)
	}

	// The combined race result: all four ranked together by raw time.
	if len(uc.Standings) != 4 {
		t.Fatalf("combined standings = %+v, want 4 rows", uc.Standings)
	}
	if uc.Standings[0].AthleteID != byName["Ali"].AthleteID || uc.Standings[0].Rank != 1 {
		t.Errorf("combined rank 1 = %+v, want Ali (11.20, fastest overall)", uc.Standings[0])
	}
	if uc.Standings[3].AthleteID != byName["Dan"].AthleteID || uc.Standings[3].Rank != 4 {
		t.Errorf("combined rank 4 = %+v, want Dan (12.80, slowest overall)", uc.Standings[3])
	}

	// Per-category re-ranked presentations.
	if len(uc.CategorySplits) != 2 {
		t.Fatalf("category splits = %+v, want 2 groups (U16 M, U18 M)", uc.CategorySplits)
	}
	byCat := map[string][]UnitStandingRow{}
	for _, split := range uc.CategorySplits {
		byCat[split.CategoryCode] = split.Standings
	}
	u18 := byCat["U18 M"]
	if len(u18) != 2 || u18[0].AthleteID != byName["Ali"].AthleteID || u18[0].Rank != 1 ||
		u18[1].AthleteID != byName["Bob"].AthleteID || u18[1].Rank != 2 {
		t.Fatalf("U18 M split = %+v, want Ali 1st, Bob 2nd (re-ranked within category)", u18)
	}
	u16 := byCat["U16 M"]
	if len(u16) != 2 || u16[0].AthleteID != byName["Cyril"].AthleteID || u16[0].Rank != 1 ||
		u16[1].AthleteID != byName["Dan"].AthleteID || u16[1].Rank != 2 {
		t.Fatalf("U16 M split = %+v, want Cyril 1st, Dan 2nd (re-ranked within category)", u16)
	}
}

// TestUC028_2_UKCPerBirthYearDivisionSplitInOneHeat covers UC-028 #2: a
// UKC-style meet's per-birth-year divisions sharing one heat get the same
// split-presentation mechanism (reusing the age-category resolution
// TASK-007's meet-wide Standings() already uses, just applied per-unit).
func TestUC028_2_UKCPerBirthYearDivisionSplitInOneHeat(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)
	unitID := unitOf(t, results, meets, rec.ID, "60m")

	m12a := register(t, results, rec.ID, ParticipantInput{FirstName: "M12a", LastName: "X", BirthYear: 2014, Sex: domain.SexMale, Bib: "1"})
	m12b := register(t, results, rec.ID, ParticipantInput{FirstName: "M12b", LastName: "X", BirthYear: 2014, Sex: domain.SexMale, Bib: "2"})
	m13 := register(t, results, rec.ID, ParticipantInput{FirstName: "M13", LastName: "X", BirthYear: 2013, Sex: domain.SexMale, Bib: "3"})

	for athleteID, tm := range map[string]string{m12a.AthleteID: "9.5", m12b.AthleteID: "9.2", m13.AthleteID: "9.0"} {
		if _, err := results.SaveTrackResult(ctx, fieldOfficial, rec.ID, unitID, TrackResultInput{
			AthleteID: athleteID, Time: tm, Timing: domain.TimingManual,
		}); err != nil {
			t.Fatalf("SaveTrackResult: %v", err)
		}
	}

	uc, err := results.UnitCapture(ctx, rec.ID, unitID)
	if err != nil {
		t.Fatalf("UnitCapture: %v", err)
	}
	if len(uc.CategorySplits) < 2 {
		t.Fatalf("category splits = %+v, want at least the M12/M13 divisions present in this heat", uc.CategorySplits)
	}
	byCat := map[string][]UnitStandingRow{}
	for _, split := range uc.CategorySplits {
		byCat[split.CategoryCode] = split.Standings
	}
	m12 := byCat["M12"]
	if len(m12) != 2 || m12[0].AthleteID != m12b.AthleteID {
		t.Fatalf("M12 split = %+v, want M12b (9.2, faster) ranked ahead of M12a within the M12 division", m12)
	}
	m13split := byCat["M13"]
	if len(m13split) != 1 || m13split[0].AthleteID != m13.AthleteID || m13split[0].Rank != 1 {
		t.Fatalf("M13 split = %+v, want the lone M13 entrant ranked 1st in their own division", m13split)
	}
}
