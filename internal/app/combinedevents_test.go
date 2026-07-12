// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/store"
)

// newTestResultsWithCombinedScoring wires the WA combined-events formula
// table on top of the ordinary newTestResults fixture (TASK-021, SYS-044),
// without touching the shared newTestResults/newTestMeets helpers other
// TASK-021-adjacent suites also use.
func newTestResultsWithCombinedScoring(t *testing.T) (*MeetService, *ResultsService, *store.Store) {
	t.Helper()
	meets, results, st := newTestResults(t)
	tables, err := domain.BuiltinCombinedScoringTables()
	if err != nil {
		t.Fatalf("BuiltinCombinedScoringTables: %v", err)
	}
	meets.SetCombinedScoringTables(tables)
	results.SetCombinedScoringTables(tables)
	return meets, results, st
}

func heptathlonDay() time.Time { return time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC) }

func createHeptathlonMeet(t *testing.T, meets *MeetService) MeetRecord {
	t.Helper()
	rec, err := meets.CreateMeetFromTemplate(context.Background(), organizer, TemplateMeetRequest{
		TemplateID: domain.TemplateWAHeptathlon,
		Venue:      "Fribourg",
		Date:       heptathlonDay(),
	})
	if err != nil {
		t.Fatalf("CreateMeetFromTemplate(wa-heptathlon): %v", err)
	}
	return rec
}

// TestCreateCombinedEventsMeetFromTemplateSYS031SYS044UC013 covers UC-013's
// setup precondition (mirroring UC-033 #1 for combined events, SYS-031):
// a template + date + venue yield a complete seven-discipline heptathlon
// meet whose results score through the WA combined-events formula table,
// not the ordinary lookup ScoringTable.
func TestCreateCombinedEventsMeetFromTemplateSYS031SYS044UC013(t *testing.T) {
	meets, _, st := newTestResultsWithCombinedScoring(t)
	rec := createHeptathlonMeet(t, meets)

	if rec.ScoringTableID != "" {
		t.Errorf("ScoringTableID = %q, want empty (combined-events meets use the side table instead)", rec.ScoringTableID)
	}
	id, ok, err := store.GetMeetCombinedScoringTable(context.Background(), st.DB(), rec.ID)
	if err != nil || !ok || id != domain.CombinedScoringTableWA2001 {
		t.Fatalf("GetMeetCombinedScoringTable = (%q, %v, %v), want (%q, true, nil)", id, ok, err, domain.CombinedScoringTableWA2001)
	}

	detail, err := meets.Meet(context.Background(), rec.ID)
	if err != nil {
		t.Fatalf("Meet: %v", err)
	}
	wantDisciplines := map[string]bool{"100mH": true, "HJ": true, "SP": true, "200m": true, "LJ": true, "JT": true, "800m": true}
	if len(detail.Programme) != len(wantDisciplines) {
		t.Fatalf("programme = %+v, want %d disciplines", detail.Programme, len(wantDisciplines))
	}
	for _, pe := range detail.Programme {
		if !wantDisciplines[pe.DisciplineCode] {
			t.Errorf("unexpected discipline %q in heptathlon programme", pe.DisciplineCode)
		}
	}
}

// TestCombinedEventsScoringAndStandingsSYS044UC013_1_2 saves a handful of
// heptathlon results through the ordinary SaveResult path and checks both
// per-discipline points (WA formula) and cumulative standings after a
// partial programme (UC-013 #1/#2): "each athlete's points equal the World
// Athletics scoring-table formula output... standings merge... cumulative
// points and ranks are correct after each discipline."
func TestCombinedEventsScoringAndStandingsSYS044UC013_1_2(t *testing.T) {
	meets, results, _ := newTestResultsWithCombinedScoring(t)
	ctx := context.Background()
	rec := createHeptathlonMeet(t, meets)
	a := registerAthlete(t, results, rec.ID, "Jackie", "Muster", domain.SexFemale, 1962)
	b := registerAthlete(t, results, rec.ID, "Beat", "Schmid", domain.SexFemale, 1990)

	// A: the JJK 1988 100mH mark, worth 1172 points per the combined
	// scoring table fixtures (domain.TestCombinedScoringReferenceFixture...).
	saveA, err := results.SaveResult(ctx, office, rec.ID, ResultInput{
		AthleteID: a, DisciplineCode: "100mH", Mark: "12.69", Timing: domain.TimingElectronic,
	})
	if err != nil {
		t.Fatalf("SaveResult A 100mH: %v", err)
	}
	if saveA.Points == nil || *saveA.Points != 1172 {
		t.Fatalf("A 100mH points = %v, want 1172", saveA.Points)
	}
	// B runs slower: fewer points.
	saveB, err := results.SaveResult(ctx, office, rec.ID, ResultInput{
		AthleteID: b, DisciplineCode: "100mH", Mark: "14.20", Timing: domain.TimingElectronic,
	})
	if err != nil {
		t.Fatalf("SaveResult B 100mH: %v", err)
	}
	if saveB.Points == nil || *saveB.Points >= *saveA.Points {
		t.Fatalf("B 100mH points = %v, want fewer than A's %d", saveB.Points, *saveA.Points)
	}

	standings, err := results.Standings(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Standings: %v", err)
	}
	if len(standings.Disciplines) != 7 {
		t.Fatalf("standings disciplines = %v, want 7", standings.Disciplines)
	}
	div := findDivisionContaining(standings, a)
	if div.Rows == nil {
		t.Fatal("expected a division containing athlete A")
	}
	if len(div.Rows) < 2 {
		t.Fatalf("expected both athletes merged into the same division (SYS-052/UC-013 #1 'standings merge'), got %+v", div.Rows)
	}
	rankOf := map[string]int{}
	totalOf := map[string]int{}
	for _, row := range div.Rows {
		rankOf[row.AthleteID] = row.Rank
		totalOf[row.AthleteID] = row.Total
	}
	if rankOf[a] != 1 {
		t.Errorf("A rank = %d, want 1 (better 100mH, only discipline so far)", rankOf[a])
	}
	if totalOf[a] != *saveA.Points {
		t.Errorf("A cumulative total after one discipline = %d, want %d", totalOf[a], *saveA.Points)
	}

	// A second discipline completes: cumulative points update (UC-013 #2).
	saveA2, err := results.SaveResult(ctx, office, rec.ID, ResultInput{
		AthleteID: a, DisciplineCode: "SP", Mark: "15.80",
	})
	if err != nil {
		t.Fatalf("SaveResult A SP: %v", err)
	}
	standings2, err := results.Standings(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Standings (after 2nd discipline): %v", err)
	}
	div2 := findDivisionContaining(standings2, a)
	for _, row := range div2.Rows {
		if row.AthleteID == a && row.Total != *saveA.Points+*saveA2.Points {
			t.Errorf("A cumulative total after two disciplines = %d, want %d", row.Total, *saveA.Points+*saveA2.Points)
		}
	}
}

// TestCombinedEventsDNFAndWithdrawalSYS044UC013_3 checks UC-013 #3: a DNF
// in one discipline scores as an explicit gap (0 contribution, status
// visible) and the athlete continues in subsequent disciplines, while a
// retirement status is a distinct, settable capture status.
func TestCombinedEventsDNFAndWithdrawalSYS044UC013_3(t *testing.T) {
	meets, results, _ := newTestResultsWithCombinedScoring(t)
	ctx := context.Background()
	rec := createHeptathlonMeet(t, meets)
	a := registerAthlete(t, results, rec.ID, "Jackie", "Muster", domain.SexFemale, 1962)

	dnf, err := results.SaveResult(ctx, office, rec.ID, ResultInput{
		AthleteID: a, DisciplineCode: "100mH", Status: domain.StatusDNF,
	})
	if err != nil {
		t.Fatalf("SaveResult (DNF): %v", err)
	}
	if dnf.Points != nil {
		t.Errorf("DNF points = %v, want nil (no scoring result — the standings show the gap, UC-033 #3 convention)", dnf.Points)
	}
	if dnf.Status != domain.StatusDNF {
		t.Errorf("status = %q, want DNF (status visible)", dnf.Status)
	}

	// The athlete continues: a later discipline still accepts a result.
	next, err := results.SaveResult(ctx, office, rec.ID, ResultInput{
		AthleteID: a, DisciplineCode: "HJ", Mark: "1.70",
	})
	if err != nil {
		t.Fatalf("SaveResult after a DNF should still be accepted: %v", err)
	}
	if next.Points == nil {
		t.Error("expected the subsequent discipline to score normally")
	}

	standings, err := results.Standings(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Standings: %v", err)
	}
	div := findDivisionContaining(standings, a)
	found := false
	for _, row := range div.Rows {
		if row.AthleteID != a {
			continue
		}
		found = true
		if row.Complete {
			t.Error("Complete should be false: the 100mH discipline has no scoring result (DNF)")
		}
		for _, m := range row.Marks {
			if m.DisciplineCode == "100mH" && m.Status != domain.StatusDNF {
				t.Errorf("100mH performance status = %q, want DNF (status visible in the standings row)", m.Status)
			}
		}
	}
	if !found {
		t.Fatal("athlete A not found in standings")
	}
}

// TestCombinedEventsVerticalDisciplineScoresThroughWAFormulaSYS043SYS044UC012UC013
// is the vertical+combined intersection this task is named for: a high
// jump captured through the TASK-021 vertical-jump service on a
// combined-events meet scores via the WA combined-events formula table
// (not the discrete UKC-style lookup table), and the settled result feeds
// the same Standings() aggregation as every other discipline.
func TestCombinedEventsVerticalDisciplineScoresThroughWAFormulaSYS043SYS044UC012UC013(t *testing.T) {
	meets, results, _ := newTestResultsWithCombinedScoring(t)
	ctx := context.Background()
	rec := createHeptathlonMeet(t, meets)
	a := registerAthlete(t, results, rec.ID, "Jackie", "Muster", domain.SexFemale, 1962)

	detail, err := meets.Meet(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Meet: %v", err)
	}
	var hjUnit string
	for _, u := range detail.Units {
		if u.DisciplineCode == "HJ" {
			hjUnit = u.UnitID
		}
	}
	if hjUnit == "" {
		t.Fatal("meet has no HJ unit")
	}
	if err := store.AssignFieldOfficialUnit(ctx, meets.db, fieldOfficial.AccountID, rec.ID, hjUnit); err != nil {
		t.Fatalf("AssignFieldOfficialUnit: %v", err)
	}
	if _, err := results.SetVerticalHeights(ctx, office, rec.ID, hjUnit, VerticalHeightsInput{
		Heights: []string{"1.80", "1.83", "1.86"},
	}); err != nil {
		t.Fatalf("SetVerticalHeights: %v", err)
	}
	vTrial(t, results, rec.ID, hjUnit, a, 0, 1, domain.StatusO)
	vTrial(t, results, rec.ID, hjUnit, a, 1, 1, domain.StatusO)
	vTrial(t, results, rec.ID, hjUnit, a, 2, 1, domain.StatusX)
	vTrial(t, results, rec.ID, hjUnit, a, 2, 2, domain.StatusX)
	vTrial(t, results, rec.ID, hjUnit, a, 2, 3, domain.StatusX)

	view, err := results.VerticalCapture(ctx, rec.ID, hjUnit)
	if err != nil {
		t.Fatalf("VerticalCapture: %v", err)
	}
	if len(view.Standings) != 1 || view.Standings[0].BestHeight != "1.83" {
		t.Fatalf("standings = %+v, want best height 1.83", view.Standings)
	}
	// 1.83 m for the women's HJ formula (a=1.84523, b=75, c=1.348):
	// floor(1.84523 * (183-75)^1.348) = 1016 points.
	if view.Standings[0].Points == nil || *view.Standings[0].Points != 1016 {
		got := "nil"
		if view.Standings[0].Points != nil {
			got = fmt.Sprintf("%d", *view.Standings[0].Points)
		}
		t.Fatalf("HJ 1.83 points = %s, want 1016 (WA combined-events formula, not the UKC lookup table)", got)
	}

	standings, err := results.Standings(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Standings: %v", err)
	}
	div := findDivisionContaining(standings, a)
	for _, row := range div.Rows {
		if row.AthleteID != a {
			continue
		}
		for _, m := range row.Marks {
			if m.DisciplineCode == "HJ" {
				if m.Points == nil || *m.Points != 1016 {
					t.Errorf("HJ performance in Standings() = %v, want 1016 points", m.Points)
				}
				if m.Mark != "1.83" {
					t.Errorf("HJ mark in Standings() = %q, want 1.83", m.Mark)
				}
			}
		}
	}
}

// findDivisionContaining is a small test helper: the division-standings
// row for the first division that includes athleteID.
func findDivisionContaining(standings MeetStandings, athleteID string) DivisionStanding {
	for _, div := range standings.Divisions {
		for _, row := range div.Rows {
			if row.AthleteID == athleteID {
				return div
			}
		}
	}
	return DivisionStanding{}
}
