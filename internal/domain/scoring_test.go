// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"strings"
	"testing"
)

// TestUKCScoringOfficialFixtures verifies the shipped UBS Kids Cup scoring
// table against rows transcribed independently from the official PDF
// (UBS_Kids_Cup_Wertungstabelle_Disziplinen.pdf, "Swiss Athletics
// Wertungstabelle 2025 — Version UBS Kids Cup") — UC-033 #2 "reference
// fixtures from the published table". Between-row cases exercise the
// Reglement §3 rule that a result between two listed marks earns the
// next-lower points ("nächsttiefere Punktzahl").
func TestUKCScoringOfficialFixtures(t *testing.T) {
	table, err := BuiltinScoringTable(ScoringTableUBSKidsCup)
	if err != nil {
		t.Fatalf("BuiltinScoringTable: %v", err)
	}

	cases := []struct {
		name       string
		discipline string
		timing     Timing
		sex        Sex
		mark       string
		want       int
	}{
		// Exact published rows (top, bottom, and mid-table).
		{"male LJ table ceiling", "ZoneLJ", TimingNone, SexMale, "7.99", 1100},
		{"male manual 60m best listed", "60m", TimingManual, SexMale, "6.48", 1099},
		{"male manual 60m next row", "60m", TimingManual, SexMale, "6.49", 1096},
		{"male electronic 60m best listed", "60m", TimingElectronic, SexMale, "6.72", 1099},
		{"female manual 60m ceiling", "60m", TimingManual, SexFemale, "7.00", 1100},
		{"female ball ceiling", "BallThrow200g", TimingNone, SexFemale, "73.09", 1100},
		{"male manual 60m last row", "60m", TimingManual, SexMale, "13.88", 1},
		{"female LJ last row", "ZoneLJ", TimingNone, SexFemale, "1.26", 1},
		{"male electronic 60m mid-table", "60m", TimingElectronic, SexMale, "8.42", 599},
		{"female electronic 60m mid-table", "60m", TimingElectronic, SexFemale, "8.42", 710},
		{"male LJ mid-table", "ZoneLJ", TimingNone, SexMale, "4.12", 425},
		{"female LJ mid-table", "ZoneLJ", TimingNone, SexFemale, "4.12", 548},

		// Between two listed marks: next-lower points (Reglement §3).
		{"male ball between 38.48 and 38.56", "BallThrow200g", TimingNone, SexMale, "38.50", 440},
		{"female ball between 38.44 and 38.51", "BallThrow200g", TimingNone, SexFemale, "38.50", 580},
		{"male ball between 95.79 and 95.88", "BallThrow200g", TimingNone, SexMale, "95.80", 1099},
		{"male manual 60m between 13.72 and 13.88", "60m", TimingManual, SexMale, "13.80", 1},

		// Better than the first row: the published ceiling applies.
		{"male LJ beyond ceiling", "ZoneLJ", TimingNone, SexMale, "8.40", 1100},
		{"male manual 60m beyond ceiling", "60m", TimingManual, SexMale, "6.30", 1099},

		// Worse than the last row: zero points.
		{"male manual 60m below table", "60m", TimingManual, SexMale, "14.50", 0},
		{"female LJ below table", "ZoneLJ", TimingNone, SexFemale, "1.20", 0},
		{"female ball below table", "BallThrow200g", TimingNone, SexFemale, "4.90", 0},

		// Comma decimal separator (Swiss result-list convention, C7.3).
		{"comma decimals accepted", "60m", TimingElectronic, SexFemale, "8,42", 710},
	}
	for _, tc := range cases {
		got, err := table.Points(tc.discipline, tc.timing, tc.sex, tc.mark)
		if err != nil {
			t.Errorf("%s: Points(%s/%s/%s, %q): %v", tc.name, tc.discipline, tc.timing, tc.sex, tc.mark, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: Points(%s/%s/%s, %q) = %d, want %d", tc.name, tc.discipline, tc.timing, tc.sex, tc.mark, got, tc.want)
		}
	}
}

func TestUKCScoringUnknownColumn(t *testing.T) {
	table, err := BuiltinScoringTable(ScoringTableUBSKidsCup)
	if err != nil {
		t.Fatalf("BuiltinScoringTable: %v", err)
	}
	// The UKC table has no wind-legal 100m column, and 60m field marks
	// always name a timing method.
	if _, err := table.Points("100m", TimingElectronic, SexMale, "12.00"); err == nil {
		t.Error("Points for unknown discipline column: want error, got nil")
	}
	if _, err := table.Points("60m", TimingNone, SexMale, "8.00"); err == nil {
		t.Error("Points for 60m without timing method: want error, got nil")
	}
	if _, err := table.Points("60m", TimingElectronic, SexMale, "abc"); err == nil {
		t.Error("Points for malformed mark: want error, got nil")
	}
}

// TestScoringTableIsData proves UC-033 #5: a revised series table is a
// data-file swap interpreted by the same code path — no code change. The
// same mark scores differently under a revised table document.
func TestScoringTableIsData(t *testing.T) {
	revised := `{
	  "id": "ubs-kids-cup",
	  "version": "2027-test",
	  "name": "revised table",
	  "rounding": "next-lower-points",
	  "columns": [
	    {"disciplineCode": "ZoneLJ", "timing": "", "sex": "M", "betterDirection": "higher",
	     "marks": [[500, "4.50"], [400, "4.00"], [300, "3.50"]]}
	  ]
	}`
	table, err := ParseScoringTable([]byte(revised))
	if err != nil {
		t.Fatalf("ParseScoringTable(revised): %v", err)
	}
	got, err := table.Points("ZoneLJ", TimingNone, SexMale, "4.12")
	if err != nil {
		t.Fatalf("Points: %v", err)
	}
	if got != 400 {
		t.Errorf("revised table Points(ZoneLJ, 4.12) = %d, want 400", got)
	}
	builtin, err := BuiltinScoringTable(ScoringTableUBSKidsCup)
	if err != nil {
		t.Fatalf("BuiltinScoringTable: %v", err)
	}
	official, err := builtin.Points("ZoneLJ", TimingNone, SexMale, "4.12")
	if err != nil {
		t.Fatalf("Points(builtin): %v", err)
	}
	if official == got {
		t.Errorf("revised and built-in tables agree (%d) — data swap proved nothing", got)
	}
}

func TestParseScoringTableRejectsBadData(t *testing.T) {
	cases := []struct {
		name string
		doc  string
	}{
		{"missing version", `{"id": "x", "rounding": "next-lower-points",
			"columns": [{"disciplineCode": "60m", "timing": "manual", "sex": "M", "betterDirection": "lower", "marks": [[10, "8.00"]]}]}`},
		{"unknown rounding rule", `{"id": "x", "version": "1", "rounding": "nearest",
			"columns": [{"disciplineCode": "60m", "timing": "manual", "sex": "M", "betterDirection": "lower", "marks": [[10, "8.00"]]}]}`},
		{"no columns", `{"id": "x", "version": "1", "rounding": "next-lower-points", "columns": []}`},
		{"duplicate column", `{"id": "x", "version": "1", "rounding": "next-lower-points", "columns": [
			{"disciplineCode": "60m", "timing": "manual", "sex": "M", "betterDirection": "lower", "marks": [[10, "8.00"]]},
			{"disciplineCode": "60m", "timing": "manual", "sex": "M", "betterDirection": "lower", "marks": [[10, "8.00"]]}]}`},
		{"bad direction", `{"id": "x", "version": "1", "rounding": "next-lower-points", "columns": [
			{"disciplineCode": "60m", "timing": "manual", "sex": "M", "betterDirection": "sideways", "marks": [[10, "8.00"]]}]}`},
		{"non-monotonic points", `{"id": "x", "version": "1", "rounding": "next-lower-points", "columns": [
			{"disciplineCode": "60m", "timing": "manual", "sex": "M", "betterDirection": "lower", "marks": [[10, "8.00"], [10, "8.10"]]}]}`},
		{"non-monotonic marks", `{"id": "x", "version": "1", "rounding": "next-lower-points", "columns": [
			{"disciplineCode": "ZoneLJ", "timing": "", "sex": "M", "betterDirection": "higher", "marks": [[10, "4.00"], [9, "4.50"]]}]}`},
		{"malformed mark", `{"id": "x", "version": "1", "rounding": "next-lower-points", "columns": [
			{"disciplineCode": "60m", "timing": "manual", "sex": "M", "betterDirection": "lower", "marks": [[10, "8.0.0"]]}]}`},
		{"not json", `{`},
	}
	for _, tc := range cases {
		if _, err := ParseScoringTable([]byte(tc.doc)); err == nil {
			t.Errorf("%s: want error, got nil", tc.name)
		}
	}
}

func TestParseCentiMark(t *testing.T) {
	good := map[string]int64{
		"8.42": 842, "38.5": 3850, "38": 3800, "95.88": 9588,
		"7,67": 767, "0.01": 1, " 6.48 ": 648,
	}
	for in, want := range good {
		got, err := ParseCentiMark(in)
		if err != nil {
			t.Errorf("ParseCentiMark(%q): %v", in, err)
		} else if got != want {
			t.Errorf("ParseCentiMark(%q) = %d, want %d", in, got, want)
		}
	}
	for _, in := range []string{"", "abc", "-1.00", "8.423", "8.", "1e3"} {
		if _, err := ParseCentiMark(in); err == nil {
			t.Errorf("ParseCentiMark(%q): want error, got nil", in)
		}
	}
}

// TestBuiltinScoringTableShape sanity-checks the generated data file: all
// eight official columns present with plausible extents.
func TestBuiltinScoringTableShape(t *testing.T) {
	table, err := BuiltinScoringTable(ScoringTableUBSKidsCup)
	if err != nil {
		t.Fatalf("BuiltinScoringTable: %v", err)
	}
	if !strings.Contains(table.Source, "ubs-kidscup.ch") {
		t.Error("scoring table must cite its official source")
	}
	for _, sex := range []Sex{SexMale, SexFemale} {
		for _, col := range []struct {
			disc   string
			timing Timing
		}{{"60m", TimingManual}, {"60m", TimingElectronic}, {"ZoneLJ", TimingNone}, {"BallThrow200g", TimingNone}} {
			c, ok := table.Column(col.disc, col.timing, sex)
			if !ok {
				t.Errorf("missing column %s/%s/%s", col.disc, col.timing, sex)
				continue
			}
			if c.Marks[0].Points != 1100 && c.Marks[0].Points != 1099 && c.Marks[0].Points != 1098 {
				t.Errorf("column %s/%s/%s starts at %d points, want near 1100", col.disc, col.timing, sex, c.Marks[0].Points)
			}
			if last := c.Marks[len(c.Marks)-1].Points; last != 1 {
				t.Errorf("column %s/%s/%s ends at %d points, want 1", col.disc, col.timing, sex, last)
			}
		}
	}
	if _, err := BuiltinScoringTables(); err != nil {
		t.Errorf("BuiltinScoringTables: %v", err)
	}
}
