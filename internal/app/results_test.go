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

var office = Session{AccountID: "01OFF", Username: "office", Role: RoleCompetitionOffice}

// newTestResults wires a MeetService and a ResultsService over one
// temporary store (the UC-033 flows span both).
func newTestResults(t *testing.T) (*MeetService, *ResultsService, *store.Store) {
	t.Helper()
	meets, st := newTestMeets(t)
	catalog, err := domain.BuiltinDisciplineCatalog()
	if err != nil {
		t.Fatalf("BuiltinDisciplineCatalog: %v", err)
	}
	schemes, err := domain.BuiltinCategorySchemes()
	if err != nil {
		t.Fatalf("BuiltinCategorySchemes: %v", err)
	}
	tables, err := domain.BuiltinScoringTables()
	if err != nil {
		t.Fatalf("BuiltinScoringTables: %v", err)
	}
	templates, err := domain.BuiltinMeetTemplates()
	if err != nil {
		t.Fatalf("BuiltinMeetTemplates: %v", err)
	}
	results := NewResultsService(st.DB(), catalog, schemes, tables, templates)
	seriesUploads, err := domain.BuiltinSeriesUploadTemplates()
	if err != nil {
		t.Fatalf("BuiltinSeriesUploadTemplates: %v", err)
	}
	results.SetSeriesUploadTemplates(seriesUploads)
	importProfiles, err := domain.BuiltinImportMappingProfiles()
	if err != nil {
		t.Fatalf("BuiltinImportMappingProfiles: %v", err)
	}
	results.SetImportMappingProfiles(importProfiles)
	recordLists, err := domain.BuiltinRecordLists()
	if err != nil {
		t.Fatalf("BuiltinRecordLists: %v", err)
	}
	results.SetRecordLists(recordLists)
	return meets, results, st
}

func ukcDay() time.Time { return time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC) }

func createUKCMeet(t *testing.T, meets *MeetService) MeetRecord {
	t.Helper()
	rec, err := meets.CreateMeetFromTemplate(context.Background(), organizer, TemplateMeetRequest{
		TemplateID: domain.TemplateUBSKidsCup,
		Venue:      "Le Mouret",
		Date:       ukcDay(),
	})
	if err != nil {
		t.Fatalf("CreateMeetFromTemplate: %v", err)
	}
	return rec
}

// TestUC033_1_CreateUKCMeetFromTemplate covers UC-033 #1: template + date
// + venue yield a complete UKC meet — the 3 disciplines, the per-birth-year
// divisions M/W 7–15 and the UKC scoring — with no further configuration.
func TestUC033_1_CreateUKCMeetFromTemplate(t *testing.T) {
	meets, _, st := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)

	if rec.Status != domain.MeetDraft {
		t.Errorf("status = %q, want draft", rec.Status)
	}
	if rec.Name != "UBS Kids Cup Le Mouret 2026" {
		t.Errorf("derived name = %q", rec.Name)
	}
	if rec.CategorySchemeID != domain.SchemeUBSKidsCup {
		t.Errorf("scheme = %q, want %q", rec.CategorySchemeID, domain.SchemeUBSKidsCup)
	}
	if rec.ScoringTableID != domain.ScoringTableUBSKidsCup {
		t.Errorf("scoring table = %q, want %q (UKC combined scoring)", rec.ScoringTableID, domain.ScoringTableUBSKidsCup)
	}
	if rec.TemplateID != domain.TemplateUBSKidsCup {
		t.Errorf("template = %q", rec.TemplateID)
	}

	d, err := meets.Meet(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Meet: %v", err)
	}
	if len(d.Sessions) != 1 {
		t.Errorf("got %d sessions, want 1", len(d.Sessions))
	}
	wantDisciplines := map[string]bool{"60m": true, "ZoneLJ": true, "BallThrow200g": true}
	if len(d.Programme) != 3 {
		t.Fatalf("programme has %d events, want the 3 UKC disciplines", len(d.Programme))
	}
	for _, ev := range d.Programme {
		if !wantDisciplines[ev.DisciplineCode] {
			t.Errorf("unexpected discipline %q", ev.DisciplineCode)
		}
		if len(ev.CategoryCodes) != 18 {
			t.Errorf("%s spans %d divisions, want 18 (M/W 7-15)", ev.DisciplineCode, len(ev.CategoryCodes))
		}
		if len(ev.Rounds) != 1 {
			t.Errorf("%s has %d rounds, want a single final", ev.DisciplineCode, len(ev.Rounds))
		}
	}
	if len(d.Units) != 3 {
		t.Errorf("got %d schedulable units, want 3", len(d.Units))
	}

	trail, err := store.AuditTrail(ctx, st.DB(), "meet", rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(trail) != 1 || trail[0].Action != "meet.create-from-template" {
		t.Errorf("audit trail = %+v, want one meet.create-from-template", trail)
	}
}

func TestCreateFromTemplateAuthorizationAndValidation(t *testing.T) {
	meets, _, _ := newTestResults(t)
	ctx := context.Background()

	official := Session{AccountID: "01FLD", Username: "field", Role: RoleFieldOfficial}
	var forbidden ErrForbidden
	if _, err := meets.CreateMeetFromTemplate(ctx, official, TemplateMeetRequest{
		TemplateID: domain.TemplateUBSKidsCup, Venue: "X", Date: ukcDay(),
	}); !errors.As(err, &forbidden) {
		t.Errorf("as field official = %v, want ErrForbidden (SYS-090)", err)
	}
	if _, err := meets.CreateMeetFromTemplate(ctx, organizer, TemplateMeetRequest{
		TemplateID: "no-such-template", Venue: "X", Date: ukcDay(),
	}); err == nil {
		t.Error("unknown template: want error")
	}
	if _, err := meets.CreateMeetFromTemplate(ctx, organizer, TemplateMeetRequest{
		TemplateID: domain.TemplateUBSKidsCup, Venue: "", Date: ukcDay(),
	}); err == nil {
		t.Error("missing venue: want error")
	}
	if _, err := meets.CreateMeetFromTemplate(ctx, organizer, TemplateMeetRequest{
		TemplateID: domain.TemplateUBSKidsCup, Venue: "X",
	}); err == nil {
		t.Error("missing date: want error")
	}
}

func register(t *testing.T, results *ResultsService, meetID string, in ParticipantInput) store.ParticipantRow {
	t.Helper()
	p, err := results.RegisterParticipant(context.Background(), office, meetID, in)
	if err != nil {
		t.Fatalf("RegisterParticipant(%s %s): %v", in.FirstName, in.LastName, err)
	}
	return p
}

func save(t *testing.T, results *ResultsService, meetID string, in ResultInput) store.ResultRecord {
	t.Helper()
	rec, err := results.SaveResult(context.Background(), office, meetID, in)
	if err != nil {
		t.Fatalf("SaveResult(%s %s): %v", in.DisciplineCode, in.Mark, err)
	}
	return rec
}

// TestUC033_2_ScoringAndStandings covers UC-033 #2 and #4: saved UKC
// results score exactly per the official points table (fixture values
// verified against the published PDF), division standings update, and
// ranking rows carry the federation presentation structure (rank, bib,
// name, club, birth year, per-discipline marks, points, total — C7.3).
// The M12 boy proves the SYS-052 split: one field, per-division lists.
func TestUC033_2_ScoringAndStandings(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)

	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014,
		Sex: domain.SexFemale, Club: "LC Test", Bib: "101",
	})
	bea := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Bea", LastName: "Beispiel", BirthYear: 2014,
		Sex: domain.SexFemale, Club: "LC Test", Bib: "102",
	})
	max := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Max", LastName: "Model", BirthYear: 2014,
		Sex: domain.SexMale, Bib: "201",
	})

	// Official-table fixtures (W column): 8.42 elec → 710, 4.12 → 548,
	// 38.50 → 580 (between 38.44 and 38.51 → next-lower row).
	r := save(t, results, rec.ID, ResultInput{
		AthleteID: anna.AthleteID, DisciplineCode: "60m",
		Mark: "8.42", Timing: domain.TimingElectronic,
	})
	if r.Points == nil || *r.Points != 710 {
		t.Fatalf("60m 8.42 points = %v, want 710 (official table)", r.Points)
	}

	// Standings update immediately after the first save (UC-033 #2).
	st, err := results.Standings(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Standings: %v", err)
	}
	if got := findRow(t, st, "W12", "101").Total; got != 710 {
		t.Errorf("W12 bib 101 total after first save = %d, want 710", got)
	}

	save(t, results, rec.ID, ResultInput{AthleteID: anna.AthleteID, DisciplineCode: "ZoneLJ", Mark: "4.12"})
	save(t, results, rec.ID, ResultInput{AthleteID: anna.AthleteID, DisciplineCode: "BallThrow200g", Mark: "38.50"})

	// Bea: slower/shorter everywhere → ranks second.
	save(t, results, rec.ID, ResultInput{AthleteID: bea.AthleteID, DisciplineCode: "60m", Mark: "9.50", Timing: domain.TimingElectronic})
	save(t, results, rec.ID, ResultInput{AthleteID: bea.AthleteID, DisciplineCode: "ZoneLJ", Mark: "3.50"})
	save(t, results, rec.ID, ResultInput{AthleteID: bea.AthleteID, DisciplineCode: "BallThrow200g", Mark: "25.00"})

	// Max scores in his own division (M12) off the male columns: the same
	// mark yields different points than Anna's female column (599 ≠ 710).
	rm := save(t, results, rec.ID, ResultInput{AthleteID: max.AthleteID, DisciplineCode: "60m", Mark: "8.42", Timing: domain.TimingElectronic})
	if rm.Points == nil || *rm.Points != 599 {
		t.Fatalf("male 60m 8.42 points = %v, want 599 (official table)", rm.Points)
	}

	st, err = results.Standings(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Standings: %v", err)
	}
	if len(st.Disciplines) != 3 || st.Disciplines[0] != "60m" {
		t.Errorf("discipline columns = %v", st.Disciplines)
	}
	if len(st.Divisions) != 2 {
		t.Fatalf("got %d divisions with rows, want W12 and M12 (SYS-052 split)", len(st.Divisions))
	}

	annaRow := findRow(t, st, "W12", "101")
	if annaRow.Rank != 1 || annaRow.Total != 710+548+580 {
		t.Errorf("Anna rank/total = %d/%d, want 1/%d", annaRow.Rank, annaRow.Total, 710+548+580)
	}
	if !annaRow.Complete {
		t.Error("Anna has all three results; Complete = false")
	}
	// UC-033 #4 presentation structure: bib, name, club, birth year,
	// per-discipline marks and points, total.
	if annaRow.LastName != "Muster" || annaRow.ClubName != "LC Test" || annaRow.BirthYear != 2014 {
		t.Errorf("row identity = %+v", annaRow)
	}
	if len(annaRow.Marks) != 3 || annaRow.Marks[0].Mark != "8.42" || *annaRow.Marks[0].Points != 710 ||
		annaRow.Marks[1].Mark != "4.12" || annaRow.Marks[2].Mark != "38.50" {
		t.Errorf("per-discipline marks = %+v", annaRow.Marks)
	}
	if beaRow := findRow(t, st, "W12", "102"); beaRow.Rank != 2 {
		t.Errorf("Bea rank = %d, want 2", beaRow.Rank)
	}
	if maxRow := findRow(t, st, "M12", "201"); maxRow.Rank != 1 || maxRow.Total != 599 {
		t.Errorf("Max rank/total = %d/%d, want 1/599 in M12", maxRow.Rank, maxRow.Total)
	}
}

// TestUC033_3_MissingDiscipline: an athlete missing one discipline still
// ranks by total in PROVISIONAL (live/in-progress) standings — the
// live/in-progress half of UC-033 #3/DEC-016/OQ-020, unchanged by
// TASK-036 — and the standings mark the gap explicitly. See
// TestUC033FinalStandingsUnrankedIncomplete for the FINAL-standings
// counterpart, where the same gap instead unranks the row.
func TestUC033_3_MissingDiscipline(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)

	gap := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Gina", LastName: "Gap", BirthYear: 2015, Sex: domain.SexFemale, Bib: "1",
	})
	full := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Fiona", LastName: "Full", BirthYear: 2015, Sex: domain.SexFemale, Bib: "2",
	})

	// Gina: two strong results, no ball throw. Fiona: three modest ones.
	save(t, results, rec.ID, ResultInput{AthleteID: gap.AthleteID, DisciplineCode: "60m", Mark: "8.42", Timing: domain.TimingElectronic})   // 710
	save(t, results, rec.ID, ResultInput{AthleteID: gap.AthleteID, DisciplineCode: "ZoneLJ", Mark: "4.12"})                                 // 548
	save(t, results, rec.ID, ResultInput{AthleteID: full.AthleteID, DisciplineCode: "60m", Mark: "10.00", Timing: domain.TimingElectronic}) // < 710
	save(t, results, rec.ID, ResultInput{AthleteID: full.AthleteID, DisciplineCode: "ZoneLJ", Mark: "3.00"})
	save(t, results, rec.ID, ResultInput{AthleteID: full.AthleteID, DisciplineCode: "BallThrow200g", Mark: "20.00"})

	st, err := results.Standings(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Standings: %v", err)
	}
	gapRow := findRow(t, st, "W11", "1")
	if gapRow.Total != 710+548 {
		t.Errorf("gap total = %d, want %d (missing discipline contributes 0)", gapRow.Total, 710+548)
	}
	if gapRow.Complete {
		t.Error("gap row reports Complete despite a missing discipline")
	}
	if gapRow.Marks[2].Points != nil || gapRow.Marks[2].Mark != "" {
		t.Errorf("missing ball throw must be an explicit empty slot, got %+v", gapRow.Marks[2])
	}
	fullRow := findRow(t, st, "W11", "2")
	if gapRow.Total > fullRow.Total && (gapRow.Rank != 1 || fullRow.Rank != 2) {
		t.Errorf("ranking does not follow totals: gap %d/%d, full %d/%d",
			gapRow.Rank, gapRow.Total, fullRow.Rank, fullRow.Total)
	}
	if fullRow.Total > gapRow.Total && (fullRow.Rank != 1 || gapRow.Rank != 2) {
		t.Errorf("ranking does not follow totals: gap %d/%d, full %d/%d",
			gapRow.Rank, gapRow.Total, fullRow.Rank, fullRow.Total)
	}
}

// announceAllUKCUnits closes out a UKC meet's three combined-scoring units
// (SYS-047) so SeriesComplete/CurrentStandings switch to FINAL semantics —
// the "division's series is complete" trigger UC-033 #3/DEC-016 gates on.
func announceAllUKCUnits(t *testing.T, results *ResultsService, meets *MeetService, meetID string) {
	t.Helper()
	for _, disc := range []string{"60m", "ZoneLJ", "BallThrow200g"} {
		unitID := unitOf(t, results, meets, meetID, disc)
		if _, err := results.AnnounceUnitResults(context.Background(), office, meetID, unitID); err != nil {
			t.Fatalf("AnnounceUnitResults(%s): %v", disc, err)
		}
	}
}

// TestUC033FinalStandingsUnrankedIncomplete pins UC-033 #3/DEC-016/OQ-020
// end to end (RegisterParticipant → SaveResult → AnnounceUnitResults →
// FinalStandings/CurrentStandings). The domain-level fixture tests
// (TestUC033FinalStandingsUnrankedIncomplete and
// TestUC033FinalStandingsAttemptedNoValidResultStillRanks in
// internal/domain/combined_test.go) reproduce the LV Langenthal official
// Gesamtrangliste's exact evidenced marks/totals; this test exercises the
// same "gap" shape (one DNS discipline plus two NM/"ogV" ones) through the
// real capture/announce/standings service pipeline with simple round
// marks, since the points-table-accurate values are already covered by
// TestUKCScoringOfficialFixtures. "full" completes normally and stays
// ranked; "gap" mirrors the evidence's unranked "aufg." row (Joao
// Daniella): 60m DNS — never attempted, ZoneLJ/BallThrow200g NM —
// attempted, no valid mark, 1 pt each per the table's NoValidAttemptFloor;
// unranked, no rank, in the FINAL list only.
func TestUC033FinalStandingsUnrankedIncomplete(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)

	full := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Fiona", LastName: "Full", BirthYear: 2015, Sex: domain.SexFemale, Bib: "1",
	})
	gap := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Gina", LastName: "Gap", BirthYear: 2015, Sex: domain.SexFemale, Bib: "2",
	})

	save(t, results, rec.ID, ResultInput{AthleteID: full.AthleteID, DisciplineCode: "60m", Mark: "10.00", Timing: domain.TimingElectronic})
	save(t, results, rec.ID, ResultInput{AthleteID: full.AthleteID, DisciplineCode: "ZoneLJ", Mark: "3.00"})
	save(t, results, rec.ID, ResultInput{AthleteID: full.AthleteID, DisciplineCode: "BallThrow200g", Mark: "20.00"})

	save(t, results, rec.ID, ResultInput{AthleteID: gap.AthleteID, DisciplineCode: "60m", Status: domain.StatusDNS})
	save(t, results, rec.ID, ResultInput{AthleteID: gap.AthleteID, DisciplineCode: "ZoneLJ", Status: domain.StatusNM})
	save(t, results, rec.ID, ResultInput{AthleteID: gap.AthleteID, DisciplineCode: "BallThrow200g", Status: domain.StatusNM})

	// Before the series is complete: provisional semantics apply, and both
	// rows still rank (unchanged pre-TASK-036 behaviour — mirroring
	// TestUC033_3_MissingDiscipline).
	if complete, err := results.SeriesComplete(ctx, rec.ID); err != nil || complete {
		t.Fatalf("SeriesComplete before announcement = %v, %v; want false, nil", complete, err)
	}
	live, err := results.CurrentStandings(ctx, rec.ID)
	if err != nil {
		t.Fatalf("CurrentStandings (live): %v", err)
	}
	if live.Final {
		t.Error("CurrentStandings before announcement reports Final = true")
	}
	if findRow(t, live, "W11", "2").Rank == 0 {
		t.Error("provisional standings must still rank an incomplete athlete (unchanged pre-TASK-036 behaviour)")
	}

	announceAllUKCUnits(t, results, meets, rec.ID)

	if complete, err := results.SeriesComplete(ctx, rec.ID); err != nil || !complete {
		t.Fatalf("SeriesComplete after announcement = %v, %v; want true, nil", complete, err)
	}
	final, err := results.CurrentStandings(ctx, rec.ID)
	if err != nil {
		t.Fatalf("CurrentStandings (final): %v", err)
	}
	if !final.Final {
		t.Fatal("CurrentStandings after every unit is announced reports Final = false")
	}

	fullRow := findRow(t, final, "W11", "1")
	if fullRow.Rank != 1 || !fullRow.Complete {
		t.Errorf("full: rank=%d complete=%v, want 1/true", fullRow.Rank, fullRow.Complete)
	}

	gapRow := findRow(t, final, "W11", "2")
	if gapRow.Rank != 0 {
		t.Errorf("gap: rank=%d, want 0 (unranked — a missing 60m result, not merely ogV, per DEC-016/OQ-020)", gapRow.Rank)
	}
	if gapRow.Complete {
		t.Error("gap row reports Complete despite the DNS 60m")
	}
	if gapRow.Total != 2 {
		t.Errorf("gap total = %d, want 2 (1 pt floor each for the two NM/ogV disciplines, DEC-016/OQ-020)", gapRow.Total)
	}
	if gapRow.Marks[1].Points == nil || *gapRow.Marks[1].Points != 1 {
		t.Errorf("gap ZoneLJ (NM/ogV) points = %v, want 1 (the table's NoValidAttemptFloor)", gapRow.Marks[1].Points)
	}
	if gapRow.Marks[0].Points != nil {
		t.Errorf("gap 60m (DNS) points = %v, want nil (DNS never floors)", gapRow.Marks[0].Points)
	}

	// The live floor fix is not final-only: a provisional read taken now
	// (the series is complete, but Standings ignores that) shows the same
	// floored total, just still ranked (partial-total policy).
	stillLive, err := results.Standings(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Standings (provisional, post-announcement): %v", err)
	}
	if row := findRow(t, stillLive, "W11", "2"); row.Total != 2 || row.Rank == 0 {
		t.Errorf("provisional gap row = %+v, want total 2 and still ranked (floor applies to both modes)", row)
	}

	// Sweep: the series-upload export renders the same FINAL unranked
	// marker in place of a numeric rank/total (TASK-036 surface sweep).
	f, _ := openSeriesUpload(t, results, rec.ID)
	rows, err := f.GetRows(f.GetSheetList()[0])
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	var gapExportRow []string
	for _, row := range rows[1:] {
		if len(row) > 2 && row[2] == "2" { // StNr column
			gapExportRow = row
		}
	}
	if gapExportRow == nil {
		t.Fatalf("gap participant (StNr 2) missing from series-upload export")
	}
	if gapExportRow[1] != "aufg." { // Rang column
		t.Errorf("gap export rank cell = %q, want \"aufg.\" (final unranked-missing marker)", gapExportRow[1])
	}
	if last := gapExportRow[len(gapExportRow)-1]; last != "aufg." { // Total column
		t.Errorf("gap export total cell = %q, want \"aufg.\"", last)
	}
}

// TestSetOutOfCompetition covers TASK-036's DEC-016/OQ-020 investigation
// flag end to end: office-role authorization, the audit trail, and the
// standings effect — an out-of-competition athlete's marks stay visible
// and Complete, but they never hold a numeric Rank, in provisional or
// final standings alike (mirroring the LV Langenthal Gesamtrangliste's
// Thome Lauriane row: every mark present, still unranked "n.a.").
func TestSetOutOfCompetition(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)

	ooc := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Thome", LastName: "Lauriane", BirthYear: 2015, Sex: domain.SexFemale, Bib: "1",
	})
	rival := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Other", LastName: "Athlete", BirthYear: 2015, Sex: domain.SexFemale, Bib: "2",
	})
	save(t, results, rec.ID, ResultInput{AthleteID: ooc.AthleteID, DisciplineCode: "60m", Mark: "8.79", Timing: domain.TimingElectronic})
	save(t, results, rec.ID, ResultInput{AthleteID: ooc.AthleteID, DisciplineCode: "ZoneLJ", Mark: "4.38"})
	save(t, results, rec.ID, ResultInput{AthleteID: ooc.AthleteID, DisciplineCode: "BallThrow200g", Mark: "26.60"})
	save(t, results, rec.ID, ResultInput{AthleteID: rival.AthleteID, DisciplineCode: "60m", Mark: "10.00", Timing: domain.TimingElectronic})
	save(t, results, rec.ID, ResultInput{AthleteID: rival.AthleteID, DisciplineCode: "ZoneLJ", Mark: "3.00"})
	save(t, results, rec.ID, ResultInput{AthleteID: rival.AthleteID, DisciplineCode: "BallThrow200g", Mark: "20.00"})

	if err := results.SetOutOfCompetition(ctx, fieldOfficial, rec.ID, ooc.AthleteID, true); err == nil {
		t.Error("SetOutOfCompetition as a field official: want authorization error (office capability required)")
	}
	if err := results.SetOutOfCompetition(ctx, office, rec.ID, ooc.AthleteID, true); err != nil {
		t.Fatalf("SetOutOfCompetition: %v", err)
	}

	st, err := results.Standings(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Standings: %v", err)
	}
	oocRow := findRow(t, st, "W11", "1")
	if oocRow.Rank != 0 {
		t.Errorf("provisional out-of-competition rank = %d, want 0 (never ranked, even mid-meet)", oocRow.Rank)
	}
	if !oocRow.OutOfCompetition || !oocRow.Complete {
		t.Errorf("out-of-competition row = %+v, want OutOfCompetition and Complete both true (marks stay visible)", oocRow)
	}
	if rivalRow := findRow(t, st, "W11", "2"); rivalRow.Rank != 1 {
		t.Errorf("rival rank = %d, want 1 (the sole ranked athlete)", rivalRow.Rank)
	}

	announceAllUKCUnits(t, results, meets, rec.ID)
	final, err := results.FinalStandings(ctx, rec.ID)
	if err != nil {
		t.Fatalf("FinalStandings: %v", err)
	}
	if row := findRow(t, final, "W11", "1"); row.Rank != 0 {
		t.Errorf("final out-of-competition rank = %d, want 0 (still never ranked)", row.Rank)
	}
	if row := findRow(t, final, "W11", "2"); row.Rank != 1 {
		t.Errorf("final rival rank = %d, want 1", row.Rank)
	}
}

// TestUC033_5_ScoringTableSwapAtServiceLevel: the meet references its
// scoring table by ID; a revised data file under the same ID scores
// subsequent saves differently — no code change (CON-01).
func TestUC033_5_ScoringTableSwapAtServiceLevel(t *testing.T) {
	meets, results, st := newTestResults(t)
	rec := createUKCMeet(t, meets)
	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "1",
	})

	revised, err := domain.ParseScoringTable([]byte(`{
	  "id": "ubs-kids-cup", "version": "2027-revised", "name": "revised",
	  "rounding": "next-lower-points",
	  "columns": [
	    {"disciplineCode": "ZoneLJ", "timing": "", "sex": "W", "betterDirection": "higher",
	     "marks": [[999, "4.00"], [500, "3.00"]]}
	  ]
	}`))
	if err != nil {
		t.Fatalf("ParseScoringTable: %v", err)
	}
	schemes, err := domain.BuiltinCategorySchemes()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := domain.BuiltinDisciplineCatalog()
	if err != nil {
		t.Fatal(err)
	}
	swapped := NewResultsService(st.DB(), catalog, schemes, map[string]*domain.ScoringTable{
		domain.ScoringTableUBSKidsCup: revised,
	}, nil)
	r, err := swapped.SaveResult(context.Background(), office, rec.ID, ResultInput{
		AthleteID: anna.AthleteID, DisciplineCode: "ZoneLJ", Mark: "4.12",
	})
	if err != nil {
		t.Fatalf("SaveResult: %v", err)
	}
	if r.Points == nil || *r.Points != 999 {
		t.Errorf("revised-table points = %v, want 999 (data swap, no code change)", r.Points)
	}
}

func TestResultsAuthorizationAndValidation(t *testing.T) {
	meets, results, _ := newTestResults(t)
	ctx := context.Background()
	rec := createUKCMeet(t, meets)
	submitter := Session{AccountID: "01SUB", Username: "club", Role: RoleEntrySubmitter}
	var forbidden ErrForbidden

	if _, err := results.RegisterParticipant(ctx, Session{Role: RoleFieldOfficial}, rec.ID, ParticipantInput{}); !errors.As(err, &forbidden) {
		t.Errorf("RegisterParticipant as field official = %v, want ErrForbidden", err)
	}
	if _, err := results.SaveResult(ctx, submitter, rec.ID, ResultInput{}); !errors.As(err, &forbidden) {
		t.Errorf("SaveResult as entry submitter = %v, want ErrForbidden", err)
	}

	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "7",
	})
	// Duplicate athlete registration and duplicate bib both fail loudly.
	if _, err := results.RegisterParticipant(ctx, office, rec.ID, ParticipantInput{
		FirstName: "Other", LastName: "Kid", BirthYear: 2013, Sex: domain.SexMale, Bib: "7",
	}); !errors.Is(err, ErrDuplicateParticipant) {
		t.Errorf("duplicate bib = %v, want ErrDuplicateParticipant", err)
	}

	// Unknown discipline, missing mark, and non-scoring status.
	if _, err := results.SaveResult(ctx, office, rec.ID, ResultInput{
		AthleteID: anna.AthleteID, DisciplineCode: "HighJump", Mark: "1.50",
	}); err == nil {
		t.Error("unknown discipline: want error")
	}
	if _, err := results.SaveResult(ctx, office, rec.ID, ResultInput{
		AthleteID: anna.AthleteID, DisciplineCode: "60m", Timing: domain.TimingElectronic,
	}); err == nil {
		t.Error("no mark and no status: want error")
	}
	dns, err := results.SaveResult(ctx, office, rec.ID, ResultInput{
		AthleteID: anna.AthleteID, DisciplineCode: "60m", Status: domain.StatusDNS,
	})
	if err != nil {
		t.Fatalf("SaveResult(DNS): %v", err)
	}
	if dns.Points != nil {
		t.Errorf("DNS carries points %v, want none", *dns.Points)
	}

	// Corrections replace the settled mark and bump the version (SYS-046).
	first := save(t, results, rec.ID, ResultInput{AthleteID: anna.AthleteID, DisciplineCode: "ZoneLJ", Mark: "4.12"})
	second := save(t, results, rec.ID, ResultInput{AthleteID: anna.AthleteID, DisciplineCode: "ZoneLJ", Mark: "4.20"})
	if second.Version <= first.Version {
		t.Errorf("correction version = %d, want > %d", second.Version, first.Version)
	}
	if second.Mark != "4.20" {
		t.Errorf("corrected mark = %q", second.Mark)
	}
}

// findRow locates a division standings row by division code and bib.
func findRow(t *testing.T, st MeetStandings, division, bib string) StandingRow {
	t.Helper()
	for _, div := range st.Divisions {
		if div.CategoryCode != division {
			continue
		}
		for _, row := range div.Rows {
			if row.Bib == bib {
				return row
			}
		}
	}
	t.Fatalf("no row bib %q in division %q (have %+v)", bib, division, st.Divisions)
	return StandingRow{}
}
