// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import "testing"

func perf(points ...int) []CombinedPerformance {
	out := make([]CombinedPerformance, len(points))
	for i, p := range points {
		v := p
		out[i] = CombinedPerformance{Points: &v}
	}
	return out
}

// TestRankCombinedReglementRules exercises the UBS Kids Cup Reglement §3
// Rangierung fixtures: majority rule on equal totals, highest single
// discipline as the second tie-break, and equal ranks only for identical
// points multisets.
func TestRankCombinedReglementRules(t *testing.T) {
	rows := []CombinedStanding{
		// A and B tie on 1000; B is better in two of three disciplines,
		// which beats A's single highest mark (majority rule first).
		{AthleteID: "A", Performances: perf(500, 300, 200)},
		{AthleteID: "B", Performances: perf(400, 350, 250)},
		// C beats D on the majority rule despite D holding the single
		// highest mark of the pair.
		{AthleteID: "C", Performances: perf(300, 300, 300)},
		{AthleteID: "D", Performances: perf(400, 250, 250)},
		// E simply has more points than everyone.
		{AthleteID: "E", Performances: perf(400, 400, 400)},
	}
	got := RankCombined(rows)
	order := []string{"E", "B", "A", "C", "D"}
	for i, want := range order {
		if got[i].AthleteID != want {
			t.Fatalf("position %d = %s, want %s (full order: %v)", i, got[i].AthleteID, want, ids(got))
		}
	}
	wantRanks := []int{1, 2, 3, 4, 5}
	for i, want := range wantRanks {
		if got[i].Rank != want {
			t.Errorf("%s rank = %d, want %d", got[i].AthleteID, got[i].Rank, want)
		}
	}
	if got[0].Total != 1200 || got[1].Total != 1000 {
		t.Errorf("totals = %d, %d; want 1200, 1000", got[0].Total, got[1].Total)
	}
}

// TestRankCombinedHighestSingleTieBreak covers the second Reglement rule:
// equal totals, majority undecided (1–1 with one discipline tied) → the
// highest single-discipline points decides.
func TestRankCombinedHighestSingleTieBreak(t *testing.T) {
	rows := []CombinedStanding{
		{AthleteID: "low", Performances: perf(400, 400, 200)},
		{AthleteID: "high", Performances: perf(500, 300, 200)},
	}
	got := RankCombined(rows)
	if got[0].AthleteID != "high" || got[0].Rank != 1 || got[1].Rank != 2 {
		t.Errorf("order = %v ranks %d,%d; want high first, ranks 1,2", ids(got), got[0].Rank, got[1].Rank)
	}

	// Same highest mark: the comparison extends to the next-best points
	// so that only identical multisets share a rank.
	rows = []CombinedStanding{
		{AthleteID: "flat", Performances: perf(500, 250, 250)},
		{AthleteID: "spiky", Performances: perf(500, 300, 200)},
	}
	got = RankCombined(rows)
	if got[0].AthleteID != "spiky" || got[1].Rank != 2 {
		t.Errorf("order = %v; want spiky first by second-highest points", ids(got))
	}
}

// TestRankCombinedTrueTie: identical points in all three disciplines share
// the rank and the next rank is skipped (competition ranking).
func TestRankCombinedTrueTie(t *testing.T) {
	rows := []CombinedStanding{
		{AthleteID: "y", Performances: perf(400, 300, 200)},
		{AthleteID: "x", Performances: perf(400, 300, 200)},
		{AthleteID: "z", Performances: perf(400, 300, 100)},
	}
	got := RankCombined(rows)
	if got[0].Rank != 1 || got[1].Rank != 1 || got[2].Rank != 3 {
		t.Errorf("ranks = %d,%d,%d; want 1,1,3", got[0].Rank, got[1].Rank, got[2].Rank)
	}
	// Deterministic presentation order inside a tie: athlete ID.
	if got[0].AthleteID != "x" || got[1].AthleteID != "y" {
		t.Errorf("tie order = %v; want x before y", ids(got))
	}
	// Same multiset across different disciplines is still an official tie
	// (no Reglement rule distinguishes it).
	rows = []CombinedStanding{
		{AthleteID: "p", Performances: perf(500, 300, 200)},
		{AthleteID: "q", Performances: perf(300, 500, 200)},
	}
	got = RankCombined(rows)
	if got[0].Rank != 1 || got[1].Rank != 1 {
		t.Errorf("permuted multiset ranks = %d,%d; want 1,1", got[0].Rank, got[1].Rank)
	}
}

// TestRankCombinedMissingDiscipline is the UC-033 #3 rule: a missing
// discipline contributes zero points, the athlete still ranks by total,
// and the row is flagged incomplete so standings can show the gap
// explicitly (assumption OQ-020: the Reglement is silent on this case).
func TestRankCombinedMissingDiscipline(t *testing.T) {
	rows := []CombinedStanding{
		{AthleteID: "full", Performances: perf(300, 300, 300)},
		{AthleteID: "gap", Performances: []CombinedPerformance{
			{Points: intp(500)}, {Points: intp(450)}, {Status: StatusDNS},
		}},
		{AthleteID: "small", Performances: perf(250, 250, 250)},
	}
	got := RankCombined(rows)
	if got[0].AthleteID != "gap" {
		t.Fatalf("order = %v; the two-discipline athlete must rank first by total 950", ids(got))
	}
	for _, r := range got {
		switch r.AthleteID {
		case "gap":
			if r.Total != 950 || r.Complete || r.Rank != 1 {
				t.Errorf("gap: total=%d complete=%v rank=%d; want 950,false,1", r.Total, r.Complete, r.Rank)
			}
		case "full":
			if !r.Complete || r.Rank != 2 {
				t.Errorf("full: complete=%v rank=%d; want true,2", r.Complete, r.Rank)
			}
		case "small":
			if r.Rank != 3 {
				t.Errorf("small: rank=%d; want 3", r.Rank)
			}
		}
	}
}

func TestRankCombinedEmpty(t *testing.T) {
	if got := RankCombined(nil); len(got) != 0 {
		t.Errorf("RankCombined(nil) = %v, want empty", got)
	}
}

func ids(rows []CombinedStanding) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.AthleteID
	}
	return out
}

func intp(v int) *int { return &v }

// TestRankCombinedTiesStandTR39 pins the OQ-045 closure (SYS-044, UC-013):
// per the primary WA CR&TR 2026 edition, TR 39 (Combined Events
// Competitions) final placing provision — "If two or more athletes achieve
// an equal number of points for any place in the competition, it shall be
// determined as a tie" — equal totals share the place with NO further
// comparison. The pre-2020 majority/highest-event tie-break survives only
// as the UBS Kids Cup Reglement §3 policy (RankCombined's default).
func TestRankCombinedTiesStandTR39(t *testing.T) {
	rows := []CombinedStanding{
		// a: wins discipline 1, loses discipline 2; same 1623 total as b.
		{AthleteID: "a", Performances: []CombinedPerformance{{Points: intp(1094)}, {Points: intp(529)}}},
		{AthleteID: "b", Performances: []CombinedPerformance{{Points: intp(978)}, {Points: intp(645)}}},
		{AthleteID: "c", Performances: []CombinedPerformance{{Points: intp(900)}, {Points: intp(600)}}},
	}

	stand := RankCombinedWithTieBreak(rows, TieBreakTiesStand)
	rankOf := map[string]int{}
	for _, r := range stand {
		rankOf[r.AthleteID] = r.Rank
	}
	if rankOf["a"] != 1 || rankOf["b"] != 1 {
		t.Errorf("ties-stand: ranks a=%d b=%d, want both 1 (WA TR 39: equal points is a tie)", rankOf["a"], rankOf["b"])
	}
	if rankOf["c"] != 3 {
		t.Errorf("ties-stand: rank c=%d, want 3 (two athletes share first)", rankOf["c"])
	}

	// Control: the UKC majority/highest policy breaks the same tie — a's
	// highest single-discipline points (1094) beat b's (978).
	ukc := RankCombined(rows)
	ukcRank := map[string]int{}
	for _, r := range ukc {
		ukcRank[r.AthleteID] = r.Rank
	}
	if ukcRank["a"] != 1 || ukcRank["b"] != 2 {
		t.Errorf("UKC policy control: ranks a=%d b=%d, want 1 and 2", ukcRank["a"], ukcRank["b"])
	}
}

// TestCombinedScoringTableTieBreakPolicy pins the data plumbing: the
// shipped WA table declares ties-stand, an absent tieBreak defaults to
// ties-stand (every WA-formula table is governed by TR 39), and an unknown
// policy fails parsing loudly.
func TestCombinedScoringTableTieBreakPolicy(t *testing.T) {
	table, err := BuiltinCombinedScoringTable("wa-combined-events-2001")
	if err != nil {
		t.Fatalf("load built-in combined table: %v", err)
	}
	if table.TieBreak != TieBreakTiesStand || table.EffectiveTieBreak() != TieBreakTiesStand {
		t.Errorf("built-in WA table tieBreak = %q, want %q", table.TieBreak, TieBreakTiesStand)
	}
	if (&CombinedScoringTable{}).EffectiveTieBreak() != TieBreakTiesStand {
		t.Error("empty tieBreak must default to ties-stand (TR 39 governs WA-formula tables)")
	}
	if _, err := ParseCombinedScoringTable([]byte(`{"id":"x","version":"1","tieBreak":"coin-flip","columns":[{"disciplineCode":"100m","sex":"M","kind":"track","a":1,"b":1,"c":1}]}`)); err == nil {
		t.Error("unknown tieBreak policy must be rejected")
	}
}

// TestUC033FinalStandingsUnrankedIncomplete pins UC-033 #3/DEC-016/OQ-020
// against the LV Langenthal official Gesamtrangliste, 17.05.2025
// (https://lvl.ch/images/resultate/2025/Gesamtrangliste_UBSKidsCup_2025.pdf),
// UBS Kids Cup M14 division: "full" mirrors the last ranked row (Spack
// Sophie, rank 23, total 907) and "gap" mirrors the unranked row directly
// below it (Joao Daniella: 60m "n.a." — never attempted, ZoneLJ/BallThrow
// "ogV" — attempted, no valid mark, 1 pt each per the floor; total shown as
// "aufg.", no rank). FinalRankCombined must reproduce both: full stays
// ranked, gap drops to the unranked tail with no rank and the 1+1=2 partial
// total the evidence's points row shows.
func TestUC033FinalStandingsUnrankedIncomplete(t *testing.T) {
	one := 1
	rows := []CombinedStanding{
		{AthleteID: "full", Performances: perf(100, 396, 411)}, // Spack Sophie: 496 + 411 = 907
		{AthleteID: "gap", Performances: []CombinedPerformance{
			{Status: StatusDNS},              // 60m: never attempted ("n.a.")
			{Status: StatusNM, Points: &one}, // ZoneLJ: "ogV", floored to 1 (caller applies the floor)
			{Status: StatusNM, Points: &one}, // BallThrow200g: "ogV", floored to 1
		}},
	}
	got := FinalRankCombined(rows, TieBreakMajorityThenHighest)
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}
	fullRow, gapRow := got[0], got[1]
	if fullRow.AthleteID != "full" {
		fullRow, gapRow = got[1], got[0]
	}
	if fullRow.AthleteID != "full" || fullRow.Rank != 1 || fullRow.Total != 907 || !fullRow.Complete {
		t.Errorf("full: %+v, want rank 1, total 907, complete", fullRow)
	}
	if gapRow.AthleteID != "gap" || gapRow.Rank != 0 || gapRow.Total != 2 || gapRow.Complete {
		t.Errorf("gap: %+v, want rank 0 (unranked), total 2, incomplete — a missing discipline (DNS/\"n.a.\") "+
			"unranks the row even though the other two disciplines scored via the ogV floor", gapRow)
	}
}

// TestUC033FinalStandingsAttemptedNoValidResultStillRanks pins the
// evidence's other edge case (LV Langenthal Gesamtrangliste, W9 division,
// Geiser Lukas, rank 28): every discipline was attempted — one valid mark
// (60m, 210 pts) plus two "ogV" disciplines floored to 1 pt each — so the
// athlete still ranks normally (total 212), unlike a genuinely missing
// discipline (TestUC033FinalStandingsUnrankedIncomplete).
func TestUC033FinalStandingsAttemptedNoValidResultStillRanks(t *testing.T) {
	one := 1
	rows := []CombinedStanding{
		{AthleteID: "geiser", Performances: []CombinedPerformance{
			perf(210)[0],
			{Status: StatusNM, Points: &one},
			{Status: StatusNM, Points: &one},
		}},
	}
	got := FinalRankCombined(rows, TieBreakMajorityThenHighest)
	if len(got) != 1 || got[0].Rank != 1 || got[0].Total != 212 || !got[0].Complete {
		t.Errorf("geiser: %+v, want rank 1, total 212, complete (ogV disciplines count as present once floored)", got)
	}
}

// TestFinalRankCombinedOutOfCompetition pins the evidence's third case (LV
// Langenthal Gesamtrangliste, W12, Thome Lauriane): every discipline has a
// valid mark (total 1'603 — higher than the division's actual rank 1), yet
// the row is unranked ("n.a.") — the out-of-competition case
// (DEC-016/OQ-020 investigation, OQ-090/OQ-091). Both provisional
// (RankCombinedWithTieBreak) and final ranking must never assign it a
// numeric rank, even though it is otherwise Complete.
func TestFinalRankCombinedOutOfCompetition(t *testing.T) {
	rows := []CombinedStanding{
		{AthleteID: "ranked", Performances: perf(300, 300, 300)},
		{AthleteID: "ooc", Performances: perf(609, 391, 603), OutOfCompetition: true}, // 1'603 — would rank 1st
	}
	for _, mode := range []string{"provisional", "final"} {
		var got []CombinedStanding
		if mode == "provisional" {
			got = RankCombinedWithTieBreak(rows, TieBreakMajorityThenHighest)
		} else {
			got = FinalRankCombined(rows, TieBreakMajorityThenHighest)
		}
		var ranked, ooc CombinedStanding
		for _, r := range got {
			if r.AthleteID == "ooc" {
				ooc = r
			} else {
				ranked = r
			}
		}
		if ranked.Rank != 1 {
			t.Errorf("%s: ranked athlete's rank = %d, want 1 (unaffected by the OOC row)", mode, ranked.Rank)
		}
		if ooc.Rank != 0 {
			t.Errorf("%s: out-of-competition rank = %d, want 0 (never ranked, despite the higher total)", mode, ooc.Rank)
		}
		if ooc.Total != 1603 {
			t.Errorf("%s: out-of-competition total = %d, want 1603 (marks stay visible)", mode, ooc.Total)
		}
	}
}

// TestAttemptedNoValidResult pins the domain vocabulary DEC-016/OQ-020's
// floor and final-unranked rules key off: NM/NH/DNF/R count as "present,
// no valid attempt" (floor-eligible, not "missing"); DNS/none (never
// captured) and DQ (a rule violation, not evidenced either way) do not.
func TestAttemptedNoValidResult(t *testing.T) {
	for status, want := range map[QualificationStatus]bool{
		StatusNM: true, StatusNH: true, StatusDNF: true, StatusR: true,
		StatusDNS: false, StatusNone: false, StatusDQ: false,
	} {
		if got := AttemptedNoValidResult(status); got != want {
			t.Errorf("AttemptedNoValidResult(%q) = %v, want %v", status, got, want)
		}
	}
}
