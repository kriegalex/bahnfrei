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
