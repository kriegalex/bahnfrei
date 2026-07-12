// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import "testing"

func mustCombinedTable(t *testing.T) *CombinedScoringTable {
	t.Helper()
	tbl, err := BuiltinCombinedScoringTable(CombinedScoringTableWA2001)
	if err != nil {
		t.Fatalf("load built-in combined scoring table: %v", err)
	}
	return tbl
}

// TestCombinedScoringReferenceFixtureDecathlonSYS044UC013_1 replays Kevin
// Mayer's men's decathlon world record (9126 points, Decastar, Talence,
// 16 Sep 2018) event by event through the WA formula table and checks both
// the individual per-discipline points (published, cross-checked — see the
// data file's "source" note for the one corrected transcription) and the
// summed total against the well-known, independently published 9126
// record total (UC-013 #1: "points equal the World Athletics scoring-table
// formula output... reference fixtures, e.g. published table values").
func TestCombinedScoringReferenceFixtureDecathlonSYS044UC013_1(t *testing.T) {
	tbl := mustCombinedTable(t)
	marks := []struct {
		discipline string
		mark       string
		timing     Timing
		want       int
	}{
		{"100m", "10.55", TimingElectronic, 963},
		{"LJ", "7.80", TimingNone, 1010},
		{"SP", "16.00", TimingNone, 851},
		{"HJ", "2.05", TimingNone, 850},
		{"400m", "48.42", TimingElectronic, 889},
		// Published as 1101 on the source page, which is inconsistent with
		// the page's own stated 9126 total; 1007 is what the formula gives
		// and what actually sums to 9126 (see data file provenance note).
		{"110mH", "13.75", TimingElectronic, 1007},
		{"DT", "50.54", TimingNone, 882},
		{"PV", "5.45", TimingNone, 1051},
		{"JT", "71.90", TimingNone, 918},
		{"1500m", "276.11", TimingElectronic, 705}, // 4:36.11 expressed in seconds (the domain mark form)
	}
	total := 0
	for _, m := range marks {
		got, err := tbl.Points(m.discipline, SexMale, m.timing, m.mark)
		if err != nil {
			t.Fatalf("%s: %v", m.discipline, err)
		}
		if got != m.want {
			t.Errorf("%s %s -> %d points, want %d", m.discipline, m.mark, got, m.want)
		}
		total += got
	}
	if total != 9126 {
		t.Fatalf("decathlon total = %d, want 9126 (Kevin Mayer, Talence 2018-09-16 WR)", total)
	}
}

// TestCombinedScoringReferenceFixtureHeptathlonSYS044UC013_1 replays
// Jackie Joyner-Kersee's women's heptathlon world record (7291 points,
// Seoul Olympics, 24 Sep 1988) event by event and checks the summed total
// against the published record (UC-013 #1).
func TestCombinedScoringReferenceFixtureHeptathlonSYS044UC013_1(t *testing.T) {
	tbl := mustCombinedTable(t)
	marks := []struct {
		discipline string
		mark       string
		timing     Timing
	}{
		{"100mH", "12.69", TimingElectronic},
		{"HJ", "1.86", TimingNone},
		{"SP", "15.80", TimingNone},
		{"200m", "22.56", TimingElectronic},
		{"LJ", "7.27", TimingNone},
		{"JT", "45.66", TimingNone},
		{"800m", "128.51", TimingElectronic}, // 2:08.51
	}
	total := 0
	for _, m := range marks {
		got, err := tbl.Points(m.discipline, SexFemale, m.timing, m.mark)
		if err != nil {
			t.Fatalf("%s: %v", m.discipline, err)
		}
		total += got
	}
	if total != 7291 {
		t.Fatalf("heptathlon total = %d, want 7291 (Jackie Joyner-Kersee, Seoul 1988-09-24 WR)", total)
	}
	// The field-event points (HJ/SP/LJ/JT) additionally match independently
	// recalled published per-discipline splits exactly, cross-validating
	// the jump/throw formula branches specifically.
	fieldWant := map[string]int{"HJ": 1054, "SP": 915, "LJ": 1264, "JT": 776}
	for disc, want := range fieldWant {
		var mark string
		for _, m := range marks {
			if m.discipline == disc {
				mark = m.mark
			}
		}
		got, err := tbl.Points(disc, SexFemale, TimingNone, mark)
		if err != nil {
			t.Fatalf("%s: %v", disc, err)
		}
		if got != want {
			t.Errorf("%s %s -> %d points, want %d", disc, mark, got, want)
		}
	}
}

// TestCombinedScoringManualTimingAdjustmentSYS044 checks the WA manual-
// timing adjustment: a hand-timed mark scores as if it were the FAT mark
// (electronic time) plus the event's adjustment (0.24s for sprints/
// hurdles up to 400m... except 400m itself uses 0.14s, 0 beyond 400m).
func TestCombinedScoringManualTimingAdjustmentSYS044(t *testing.T) {
	tbl := mustCombinedTable(t)
	// A hand time of 11.00 for the men's 100m should score identically to
	// an electronic time of 11.24 (11.00 + 0.24 manual adjustment).
	manual, err := tbl.Points("100m", SexMale, TimingManual, "11.00")
	if err != nil {
		t.Fatal(err)
	}
	electronicEquivalent, err := tbl.Points("100m", SexMale, TimingElectronic, "11.24")
	if err != nil {
		t.Fatal(err)
	}
	if manual != electronicEquivalent {
		t.Errorf("manual 11.00 -> %d, electronic-equivalent 11.24 -> %d; want equal", manual, electronicEquivalent)
	}
	// An electronic 11.00 must score strictly better than the manual 11.00
	// (the manual mark is handicapped by the adjustment).
	electronic, err := tbl.Points("100m", SexMale, TimingElectronic, "11.00")
	if err != nil {
		t.Fatal(err)
	}
	if electronic <= manual {
		t.Errorf("electronic 11.00 (%d pts) should score more than manual 11.00 (%d pts)", electronic, manual)
	}
	// 400m uses the 0.14s adjustment, not 0.24s.
	manual400, err := tbl.Points("400m", SexMale, TimingManual, "50.00")
	if err != nil {
		t.Fatal(err)
	}
	electronic400Equivalent, err := tbl.Points("400m", SexMale, TimingElectronic, "50.14")
	if err != nil {
		t.Fatal(err)
	}
	if manual400 != electronic400Equivalent {
		t.Errorf("manual 400m 50.00 -> %d, electronic-equivalent 50.14 -> %d; want equal", manual400, electronic400Equivalent)
	}
	// 1500m/800m are beyond 400m: no manual adjustment applies.
	manual1500, err := tbl.Points("1500m", SexMale, TimingManual, "240.00")
	if err != nil {
		t.Fatal(err)
	}
	electronic1500, err := tbl.Points("1500m", SexMale, TimingElectronic, "240.00")
	if err != nil {
		t.Fatal(err)
	}
	if manual1500 != electronic1500 {
		t.Errorf("1500m manual (%d) and electronic (%d) at the same mark must be equal (no adjustment beyond 400m)", manual1500, electronic1500)
	}
}

// TestCombinedScoringZeroFloorSYS044 checks the boundary case SYS-044
// implies but the WA formula tables never spell out: a mark on the wrong
// side of the formula's zero-crossing (B) scores 0, never a negative or a
// math error (e.g. a fractional power of a negative base).
func TestCombinedScoringZeroFloorSYS044(t *testing.T) {
	tbl := mustCombinedTable(t)
	cases := []struct {
		name       string
		discipline string
		sex        Sex
		mark       string
	}{
		{"track: slower than the formula's B", "100m", SexMale, "18.00"},
		{"track: exactly at B", "100m", SexMale, "18.00"},
		{"jump: shorter than the formula's B", "LJ", SexMale, "2.00"},
		{"throw: shorter than the formula's B", "JT", SexMale, "5.00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tbl.Points(tc.discipline, tc.sex, TimingElectronic, tc.mark)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != 0 {
				t.Errorf("%s %s -> %d points, want 0", tc.discipline, tc.mark, got)
			}
		})
	}
}

// TestCombinedScoringFloorRoundingSYS044 checks the WA rule that a
// fractional result is always rounded down, never to the nearest integer
// (SYS-044: "reproducible from stored raw marks" requires this exact,
// documented rounding — a small change to a mark must never jump points
// by rounding up).
func TestCombinedScoringFloorRoundingSYS044(t *testing.T) {
	tbl := mustCombinedTable(t)
	// 100m 10.49 -> a×(18-10.49)^1.81 = 25.4347 × 7.51^1.81 ≈ 977.99;
	// floor must give 977, never round up to 978.
	got, err := tbl.Points("100m", SexMale, TimingElectronic, "10.49")
	if err != nil {
		t.Fatal(err)
	}
	if got != 977 {
		t.Errorf("100m 10.49 -> %d points, want 977 (floor, not round)", got)
	}
}

// TestCombinedScoringUnknownColumnSYS044 checks the interpreter rejects a
// discipline/sex the table has no formula for, rather than silently
// scoring 0 (a silent 0 would be indistinguishable from a legitimately
// unreachable mark).
func TestCombinedScoringUnknownColumnSYS044(t *testing.T) {
	tbl := mustCombinedTable(t)
	if _, err := tbl.Points("60m", SexMale, TimingElectronic, "7.00"); err == nil {
		t.Fatal("expected an error for a discipline/sex the table has no column for")
	}
}

// TestParseCombinedScoringTableValidationSYS044 exercises the data-file
// interpreter's structural validation (ADR-005 §4: a malformed data file
// must fail loudly at load time, not silently misscore at capture time).
func TestParseCombinedScoringTableValidationSYS044(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{"missing id", `{"version":"2001","columns":[{"disciplineCode":"100m","sex":"M","kind":"track","a":1,"b":1,"c":1}]}`},
		{"missing version", `{"id":"x","columns":[{"disciplineCode":"100m","sex":"M","kind":"track","a":1,"b":1,"c":1}]}`},
		{"no columns", `{"id":"x","version":"1","columns":[]}`},
		{"bad kind", `{"id":"x","version":"1","columns":[{"disciplineCode":"100m","sex":"M","kind":"bogus","a":1,"b":1,"c":1}]}`},
		{"bad sex", `{"id":"x","version":"1","columns":[{"disciplineCode":"100m","sex":"X","kind":"track","a":1,"b":1,"c":1}]}`},
		{"zero a", `{"id":"x","version":"1","columns":[{"disciplineCode":"100m","sex":"M","kind":"track","a":0,"b":1,"c":1}]}`},
		{"negative c", `{"id":"x","version":"1","columns":[{"disciplineCode":"100m","sex":"M","kind":"track","a":1,"b":1,"c":-1}]}`},
		{"manual adjustment on a throw", `{"id":"x","version":"1","columns":[{"disciplineCode":"JT","sex":"M","kind":"throw","a":1,"b":1,"c":1,"manualAdjustmentSec":0.24}]}`},
		{"negative manual adjustment", `{"id":"x","version":"1","columns":[{"disciplineCode":"100m","sex":"M","kind":"track","a":1,"b":1,"c":1,"manualAdjustmentSec":-0.1}]}`},
		{"duplicate column", `{"id":"x","version":"1","columns":[
			{"disciplineCode":"100m","sex":"M","kind":"track","a":1,"b":1,"c":1},
			{"disciplineCode":"100m","sex":"M","kind":"track","a":2,"b":2,"c":2}
		]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseCombinedScoringTable([]byte(tc.json)); err == nil {
				t.Fatal("expected a validation error")
			}
		})
	}
}

// TestAverageWindSYS044UC013_4 checks the D5.3/D5.5 combined-events wind-
// legality figure: the arithmetic mean across the wind-relevant
// disciplines' readings, with an explicit "no data" signal.
func TestAverageWindSYS044UC013_4(t *testing.T) {
	if _, ok := AverageWind(nil); ok {
		t.Error("AverageWind(nil) ok = true, want false (no wind-relevant reading yet)")
	}
	// Heptathlon: 100mH +1.2, LJ +2.4 -> average +1.8 (legal, <= 2.0).
	avg, ok := AverageWind([]float64{1.2, 2.4})
	if !ok || avg <= 1.799 || avg >= 1.801 {
		t.Errorf("AverageWind([1.2, 2.4]) = (%v, %v), want (~1.8, true)", avg, ok)
	}
	// Decathlon: 100m +2.4, LJ +2.4, 110mH +1.2 -> average +2.0 exactly.
	avg, ok = AverageWind([]float64{2.4, 2.4, 1.2})
	if !ok || avg <= 1.999 || avg >= 2.001 {
		t.Errorf("AverageWind([2.4, 2.4, 1.2]) = (%v, %v), want (~2.0, true)", avg, ok)
	}
}

// TestBuiltinCombinedScoringTablesSYS044 checks the built-in-data loader
// wiring (mirrors TestBuiltinScoringTables for the UKC table).
func TestBuiltinCombinedScoringTablesSYS044(t *testing.T) {
	tables, err := BuiltinCombinedScoringTables()
	if err != nil {
		t.Fatalf("load built-in combined scoring tables: %v", err)
	}
	tbl, ok := tables[CombinedScoringTableWA2001]
	if !ok {
		t.Fatalf("expected table %q among built-ins, got %v", CombinedScoringTableWA2001, tables)
	}
	if len(tbl.Columns) != 17 {
		t.Errorf("expected 17 columns (10 men's decathlon + 7 women's heptathlon disciplines), got %d", len(tbl.Columns))
	}
}
