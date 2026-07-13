// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"testing"
)

func f64(v float64) *float64 { return &v }

// TestUC016_1_MeetingRecordFlagsLegalFATBetter covers UC-016 #1: a legal,
// FAT-timed mark that betters a loaded meeting record flags MR.
func TestUC016_1_MeetingRecordFlagsLegalFATBetter(t *testing.T) {
	eval := EvaluateRecord(RecordEvaluationInput{
		Family: FamilyTrack, DisciplineCode: "100m", Mark: "11.85",
		Timing: TimingElectronic, Wind: f64(1.1), WindRelevant: true,
		References: []RecordReference{{Type: RecordMeeting, CategoryCode: "U18 W", DisciplineCode: "100m", Mark: "11.90"}},
	})
	flags := eval.Flags()
	if len(flags) != 1 || flags[0] != "MR" {
		t.Fatalf("flags = %v, want [MR]", flags)
	}
	if !eval.Outcomes[0].Flagged || eval.Outcomes[0].Reason != "" {
		t.Errorf("outcome = %+v, want flagged with no reason", eval.Outcomes[0])
	}
}

// TestUC016_2_WindAssistedNeverFlags covers UC-016 #2: the same mark with
// wind +2.4 sets no record flag (still inspectable as wind_assisted).
func TestUC016_2_WindAssistedNeverFlags(t *testing.T) {
	eval := EvaluateRecord(RecordEvaluationInput{
		Family: FamilyTrack, DisciplineCode: "100m", Mark: "11.85",
		Timing: TimingElectronic, Wind: f64(2.4), WindRelevant: true,
		References: []RecordReference{{Type: RecordMeeting, CategoryCode: "U18 W", DisciplineCode: "100m", Mark: "11.90"}},
	})
	if flags := eval.Flags(); len(flags) != 0 {
		t.Fatalf("flags = %v, want none (wind-assisted)", flags)
	}
	if eval.Outcomes[0].Flagged || eval.Outcomes[0].Reason != ReasonWindAssisted {
		t.Errorf("outcome = %+v, want unflagged with reason %q", eval.Outcomes[0], ReasonWindAssisted)
	}
}

// TestUC016_3_HandTimedRequiresFATForShortRaces covers UC-016 #3: a
// hand-timed mark better than a record requiring FAT (races <=800m) sets
// no flag, with an inspectable reason.
func TestUC016_3_HandTimedRequiresFATForShortRaces(t *testing.T) {
	eval := EvaluateRecord(RecordEvaluationInput{
		Family: FamilyTrack, DisciplineCode: "100m", Mark: "11.80",
		Timing: TimingManual, WindRelevant: true,
		References: []RecordReference{{Type: RecordMeeting, CategoryCode: "U18 W", DisciplineCode: "100m", Mark: "11.90"}},
	})
	if flags := eval.Flags(); len(flags) != 0 {
		t.Fatalf("flags = %v, want none (hand-timed, FAT required)", flags)
	}
	if eval.Outcomes[0].Flagged || eval.Outcomes[0].Reason != ReasonRequiresFAT {
		t.Errorf("outcome = %+v, want unflagged with reason %q", eval.Outcomes[0], ReasonRequiresFAT)
	}
}

// TestRecordRequiresFAT_OnlyShortTrackRaces: the D6.2 FAT requirement
// applies to stadium races <=800m only — not field events, not longer
// track races.
func TestRecordRequiresFAT_OnlyShortTrackRaces(t *testing.T) {
	cases := []struct {
		family DisciplineFamily
		code   string
		want   bool
	}{
		{FamilyTrack, "100m", true},
		{FamilyTrack, "800m", true},
		{FamilyTrack, "1500m", false},
		{FamilyFieldHorizontal, "LJ", false},
		{FamilyRelay, "4x100m", false},
	}
	for _, c := range cases {
		if got := RecordRequiresFAT(c.family, c.code); got != c.want {
			t.Errorf("RecordRequiresFAT(%s, %s) = %v, want %v", c.family, c.code, got, c.want)
		}
	}
}

// TestUC016_5_PersonalBestFlagsWhenBettered covers UC-016 #5: an athlete
// with in-system history PB 12.02 running 11.95 legal flags PB; SB logic is
// analogous (season-scoped history).
func TestUC016_5_PersonalBestFlagsWhenBettered(t *testing.T) {
	eval := EvaluateRecord(RecordEvaluationInput{
		Family: FamilyTrack, DisciplineCode: "100m", Mark: "11.95",
		Timing: TimingElectronic, Wind: f64(0.5), WindRelevant: true,
		HasPriorBest: true, PriorBest: "12.02",
	})
	flags := eval.Flags()
	if len(flags) != 1 || flags[0] != FlagPersonalBest {
		t.Fatalf("flags = %v, want [PB]", flags)
	}
}

func TestSeasonBestFlagsAnalogousToPersonalBest(t *testing.T) {
	eval := EvaluateRecord(RecordEvaluationInput{
		Family: FamilyTrack, DisciplineCode: "100m", Mark: "12.50",
		Timing: TimingElectronic, WindRelevant: true,
		HasPriorSeasonBest: true, PriorSeasonBest: "12.60",
	})
	flags := eval.Flags()
	if len(flags) != 1 || flags[0] != FlagSeasonBest {
		t.Fatalf("flags = %v, want [SB]", flags)
	}
}

func TestNoHistoryMeansNoPBSBCandidate(t *testing.T) {
	eval := EvaluateRecord(RecordEvaluationInput{
		Family: FamilyTrack, DisciplineCode: "100m", Mark: "11.95", Timing: TimingElectronic,
	})
	if len(eval.Outcomes) != 0 {
		t.Errorf("outcomes = %+v, want none (no reference, no history)", eval.Outcomes)
	}
}

func TestStatusOnlyResultNeverFlags(t *testing.T) {
	eval := EvaluateRecord(RecordEvaluationInput{
		Family: FamilyTrack, DisciplineCode: "100m", Mark: "",
		References:   []RecordReference{{Type: RecordNational, CategoryCode: "U18 W", DisciplineCode: "100m", Mark: "11.90"}},
		HasPriorBest: true, PriorBest: "99.99",
	})
	if flags := eval.Flags(); flags != nil {
		t.Errorf("flags = %v, want none for a status-only result", flags)
	}
}

func TestEvaluateRecordDoesNotBetterReference(t *testing.T) {
	eval := EvaluateRecord(RecordEvaluationInput{
		Family: FamilyTrack, DisciplineCode: "100m", Mark: "12.10",
		Timing:     TimingElectronic,
		References: []RecordReference{{Type: RecordNational, CategoryCode: "U18 W", DisciplineCode: "100m", Mark: "11.90"}},
	})
	if eval.Outcomes[0].Flagged || eval.Outcomes[0].Reason != ReasonDoesNotBetter {
		t.Errorf("outcome = %+v, want unflagged with reason %q", eval.Outcomes[0], ReasonDoesNotBetter)
	}
}

// TestEvaluateRecordFieldHigherIsBetter: field marks compare the opposite
// direction from track (higher wins).
func TestEvaluateRecordFieldHigherIsBetter(t *testing.T) {
	eval := EvaluateRecord(RecordEvaluationInput{
		Family: FamilyFieldHorizontal, DisciplineCode: "LJ", Mark: "6.50",
		References: []RecordReference{{Type: RecordMeeting, CategoryCode: "U18 W", DisciplineCode: "LJ", Mark: "6.40"}},
	})
	if flags := eval.Flags(); len(flags) != 1 || flags[0] != "MR" {
		t.Fatalf("flags = %v, want [MR] (6.50 > 6.40)", flags)
	}
}

// TestRecordTypeFlagCode: RecordMeeting's "meeting" file value prints as
// "MR"; the other types already are their own flag code.
func TestRecordTypeFlagCode(t *testing.T) {
	cases := map[RecordType]string{
		RecordWorld: "WR", RecordArea: "AR", RecordNational: "NR", RecordMeeting: "MR",
	}
	for typ, want := range cases {
		if got := typ.FlagCode(); got != want {
			t.Errorf("%s.FlagCode() = %q, want %q", typ, got, want)
		}
	}
}

func TestWindAssisted(t *testing.T) {
	cases := []struct {
		reading *float64
		want    bool
	}{
		{nil, false},
		{f64(2.0), false}, // exactly the limit is legal (D5.3 "> +2.0")
		{f64(2.01), true},
		{f64(-1.0), false},
	}
	for _, c := range cases {
		if got := WindAssisted(c.reading); got != c.want {
			t.Errorf("WindAssisted(%v) = %v, want %v", c.reading, got, c.want)
		}
	}
}

func TestMarkAtLeastAsGood(t *testing.T) {
	if ok, err := MarkAtLeastAsGood(FamilyTrack, "11.90", "11.90"); err != nil || !ok {
		t.Errorf("equal track marks: ok=%v err=%v, want true/nil (equal counts, SYS-049)", ok, err)
	}
	if ok, err := MarkAtLeastAsGood(FamilyTrack, "11.91", "11.90"); err != nil || ok {
		t.Errorf("slower track mark: ok=%v err=%v, want false/nil", ok, err)
	}
	if ok, err := MarkAtLeastAsGood(FamilyFieldHorizontal, "6.40", "6.40"); err != nil || !ok {
		t.Errorf("equal field marks: ok=%v err=%v, want true/nil", ok, err)
	}
	if _, err := MarkAtLeastAsGood(FamilyTrack, "not-a-mark", "11.90"); err == nil {
		t.Error("invalid mark: want error")
	}
}

// TestBestMarks covers the PB/SB history reduction: wind-assisted marks
// never count, season is year-scoped, and the better-direction follows
// family.
func TestBestMarks(t *testing.T) {
	history := []MarkHistory{
		{Mark: "12.10", Year: 2025},
		{Mark: "11.80", Wind: f64(3.0), WindRelevant: true, Year: 2026}, // wind-assisted: excluded
		{Mark: "12.02", Year: 2026},
		{Mark: "12.05", Wind: f64(1.0), WindRelevant: true, Year: 2026},
	}
	best, hasBest, seasonBest, hasSeasonBest := BestMarks(FamilyTrack, history, 2026)
	if !hasBest || best != "12.02" {
		t.Errorf("best = %q (hasBest=%v), want 12.02", best, hasBest)
	}
	if !hasSeasonBest || seasonBest != "12.02" {
		t.Errorf("seasonBest = %q (hasSeasonBest=%v), want 12.02 (2026 only)", seasonBest, hasSeasonBest)
	}
}

func TestBestMarksNoHistory(t *testing.T) {
	_, hasBest, _, hasSeasonBest := BestMarks(FamilyTrack, nil, 2026)
	if hasBest || hasSeasonBest {
		t.Errorf("hasBest=%v hasSeasonBest=%v, want both false for no history", hasBest, hasSeasonBest)
	}
}

// TestUC016_4_ChecklistCoversRekordprotokollFieldsWithGaps covers UC-016
// #4: the generated checklist contains the in-system Rekordprotokoll data
// (timing class, wind, competitor count) with the fields this system never
// captures (zero-test, photo-finish image reference) marked as explicit
// gaps.
func TestUC016_4_ChecklistCoversRekordprotokollFieldsWithGaps(t *testing.T) {
	items := BuildRecordChecklist(TimingElectronic, true, f64(1.1), 6)
	byKey := make(map[string]RecordChecklistItem, len(items))
	for _, it := range items {
		byKey[it.Key] = it
	}
	if it := byKey[ChecklistTimingClass]; it.Value != "FAT" || it.Gap {
		t.Errorf("timing class = %+v, want FAT/not-a-gap", it)
	}
	if it := byKey[ChecklistWind]; it.Value != "+1.1" || it.Gap {
		t.Errorf("wind = %+v, want +1.1/not-a-gap", it)
	}
	if it := byKey[ChecklistCompetitors]; it.Value != "6" || it.Gap {
		t.Errorf("competitors = %+v, want 6/not-a-gap", it)
	}
	if it := byKey[ChecklistZeroTest]; !it.Gap || it.Value != "" {
		t.Errorf("zero-test = %+v, want an explicit gap (never captured in-system)", it)
	}
	if it := byKey[ChecklistPhotoFinishImage]; !it.Gap || it.Value != "" {
		t.Errorf("photo-finish image = %+v, want an explicit gap (never captured in-system)", it)
	}
}

func TestChecklistWindGapWhenNotYetRead(t *testing.T) {
	items := BuildRecordChecklist(TimingManual, true, nil, 4)
	for _, it := range items {
		if it.Key == ChecklistWind {
			if !it.Gap {
				t.Errorf("wind item = %+v, want a gap when no reading is on file yet", it)
			}
			return
		}
	}
	t.Fatal("wind item missing for a wind-relevant discipline")
}

func TestChecklistNoWindItemWhenNotWindRelevant(t *testing.T) {
	items := BuildRecordChecklist(TimingNone, false, nil, 4)
	for _, it := range items {
		if it.Key == ChecklistWind {
			t.Fatalf("wind item present for a non-wind-relevant discipline: %+v", it)
		}
	}
}

func TestParseRecordListValidation(t *testing.T) {
	valid := `{"id":"x","version":"1","records":[
		{"id":"r1","type":"meeting","categoryCode":"U18 W","disciplineCode":"100m","mark":"11.90","meetId":"m1"}
	]}`
	l, err := ParseRecordList([]byte(valid))
	if err != nil {
		t.Fatalf("ParseRecordList: %v", err)
	}
	if l.ID != "x" || len(l.Records) != 1 {
		t.Errorf("parsed list = %+v", l)
	}

	cases := map[string]string{
		`{"version":"1","records":[]}`: "id",
		`{"id":"x","records":[]}`:      "version",
		`{"id":"x","version":"1","records":[{"id":"","type":"meeting","categoryCode":"c","disciplineCode":"d","mark":"1.0","meetId":"m"}]}`:                                                                                                "empty id",
		`{"id":"x","version":"1","records":[{"id":"r","type":"bogus","categoryCode":"c","disciplineCode":"d","mark":"1.0"}]}`:                                                                                                              "unknown type",
		`{"id":"x","version":"1","records":[{"id":"r","type":"meeting","disciplineCode":"d","mark":"1.0","meetId":"m"}]}`:                                                                                                                  "category",
		`{"id":"x","version":"1","records":[{"id":"r","type":"meeting","categoryCode":"c","mark":"1.0","meetId":"m"}]}`:                                                                                                                    "discipline",
		`{"id":"x","version":"1","records":[{"id":"r","type":"meeting","categoryCode":"c","disciplineCode":"d","meetId":"m"}]}`:                                                                                                            "mark",
		`{"id":"x","version":"1","records":[{"id":"r","type":"meeting","categoryCode":"c","disciplineCode":"d","mark":"not-a-mark","meetId":"m"}]}`:                                                                                        "mark parse",
		`{"id":"x","version":"1","records":[{"id":"r","type":"meeting","categoryCode":"c","disciplineCode":"d","mark":"1.0"}]}`:                                                                                                            "meeting without meet id",
		`{"id":"x","version":"1","records":[{"id":"r","type":"meeting","categoryCode":"c","disciplineCode":"d","mark":"1.0","meetId":"m"},{"id":"r","type":"meeting","categoryCode":"c","disciplineCode":"d","mark":"1.0","meetId":"m"}]}`: "duplicate",
	}
	for input, desc := range cases {
		if _, err := ParseRecordList([]byte(input)); err == nil {
			t.Errorf("%s: want error", desc)
		}
	}
}

func TestBuiltinRecordLists(t *testing.T) {
	lists, err := BuiltinRecordLists()
	if err != nil {
		t.Fatalf("BuiltinRecordLists: %v", err)
	}
	if _, ok := lists[RecordListExample]; !ok {
		t.Errorf("built-in lists = %v, want %q present", lists, RecordListExample)
	}
}
