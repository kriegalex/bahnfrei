// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"fmt"
	"reflect"
	"testing"
)

func wind(v float64) *float64 { return &v }

func valid(seq int, mark string) Attempt { return Attempt{Seq: seq, Kind: AttemptValid, Mark: mark} }

func series(id string, attempts ...Attempt) FieldSeries {
	return FieldSeries{AthleteID: id, Attempts: attempts}
}

// TestAttemptValidate covers the SYS-042 capture vocabulary: mark in metres
// at 0.01 m for valid attempts, no mark on X/–/r, wind only where relevant.
func TestAttemptValidate(t *testing.T) {
	cases := []struct {
		name         string
		attempt      Attempt
		windRelevant bool
		wantErr      bool
		wantMark     string
	}{
		{"valid mark normalized", valid(1, "6.1"), true, false, "6.10"},
		{"comma decimal accepted", valid(1, "6,12"), false, false, "6.12"},
		{"valid with wind", Attempt{Seq: 1, Kind: AttemptValid, Mark: "6.12", Wind: wind(1.4)}, true, false, "6.12"},
		{"wind on non-wind-relevant", Attempt{Seq: 1, Kind: AttemptValid, Mark: "6.12", Wind: wind(1.4)}, false, true, ""},
		{"foul carries no mark", Attempt{Seq: 2, Kind: AttemptFoul}, true, false, ""},
		{"foul with mark rejected", Attempt{Seq: 2, Kind: AttemptFoul, Mark: "6.12"}, true, true, ""},
		{"pass ok", Attempt{Seq: 3, Kind: AttemptPass}, true, false, ""},
		{"retire ok", Attempt{Seq: 4, Kind: AttemptRetire}, true, false, ""},
		{"zero mark rejected", valid(1, "0"), true, true, ""},
		{"garbage mark rejected", valid(1, "abc"), true, true, ""},
		{"three decimals rejected", valid(1, "6.123"), true, true, ""},
		{"seq zero rejected", valid(0, "6.12"), true, true, ""},
		{"unknown kind rejected", Attempt{Seq: 1, Kind: "levitate"}, true, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.attempt.Validate(tc.windRelevant)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && tc.attempt.Kind == AttemptValid && tc.attempt.Mark != tc.wantMark {
				t.Fatalf("normalized mark = %q, want %q", tc.attempt.Mark, tc.wantMark)
			}
		})
	}
}

func TestAttemptDisplay(t *testing.T) {
	cases := map[string]Attempt{
		"6.12": {Kind: AttemptValid, Mark: "6.12"},
		"X":    {Kind: AttemptFoul},
		"-":    {Kind: AttemptPass},
		"r":    {Kind: AttemptRetire},
	}
	for want, a := range cases {
		if got := a.Display(); got != want {
			t.Errorf("Display(%s) = %q, want %q", a.Kind, got, want)
		}
	}
}

// TestRankFieldSeries_NextBestTieBreak is UC-011 #2: two athletes with best
// 6.42 are separated by the better second-best mark; identical full series
// rank equal.
func TestRankFieldSeries_NextBestTieBreak(t *testing.T) {
	ranked := RankFieldSeries([]FieldSeries{
		series("a", valid(1, "6.42"), Attempt{Seq: 2, Kind: AttemptFoul}, valid(3, "6.10")),
		series("b", valid(1, "6.20"), valid(2, "6.42"), valid(3, "6.30")),
		series("c", valid(1, "5.90")),
	})
	want := []FieldStanding{
		{AthleteID: "b", Best: "6.42", Rank: 1},
		{AthleteID: "a", Best: "6.42", Rank: 2},
		{AthleteID: "c", Best: "5.90", Rank: 3},
	}
	if !reflect.DeepEqual(ranked, want) {
		t.Fatalf("ranked = %+v, want %+v", ranked, want)
	}
}

func TestRankFieldSeries_IdenticalSeriesShareRank(t *testing.T) {
	ranked := RankFieldSeries([]FieldSeries{
		series("a", valid(1, "6.42"), valid(2, "6.10")),
		series("b", valid(1, "6.10"), valid(2, "6.42")),
		series("c", valid(1, "6.00")),
	})
	if ranked[0].Rank != 1 || ranked[1].Rank != 1 {
		t.Fatalf("equal full series must share rank 1: %+v", ranked)
	}
	if ranked[2].AthleteID != "c" || ranked[2].Rank != 3 {
		t.Fatalf("competition ranking must skip shared ranks (1,1,3): %+v", ranked[2])
	}
}

// TestRankFieldSeries_RetireeStillRanks is UC-011 #3: an athlete retiring
// after a valid mark still ranks by that mark, with status r.
func TestRankFieldSeries_RetireeStillRanks(t *testing.T) {
	ranked := RankFieldSeries([]FieldSeries{
		series("quitter", valid(1, "6.50"), Attempt{Seq: 2, Kind: AttemptRetire}),
		series("stayer", valid(1, "6.40")),
	})
	if ranked[0].AthleteID != "quitter" || ranked[0].Rank != 1 {
		t.Fatalf("retiree's best mark must still rank first: %+v", ranked)
	}
	if ranked[0].Status != StatusR {
		t.Fatalf("retiree status = %q, want %q", ranked[0].Status, StatusR)
	}
}

func TestRankFieldSeries_NoMarkIsNM(t *testing.T) {
	ranked := RankFieldSeries([]FieldSeries{
		series("marked", valid(1, "5.00")),
		series("fouler", Attempt{Seq: 1, Kind: AttemptFoul}, Attempt{Seq: 2, Kind: AttemptFoul}),
		series("open"),
	})
	byID := map[string]FieldStanding{}
	for _, st := range ranked {
		byID[st.AthleteID] = st
	}
	if st := byID["fouler"]; st.Status != StatusNM || st.Rank != 0 {
		t.Fatalf("all-foul series: status %q rank %d, want NM unranked", st.Status, st.Rank)
	}
	if st := byID["open"]; st.Status != StatusNone {
		t.Fatalf("empty series must not be NM yet, got %q", st.Status)
	}
	if ranked[0].AthleteID != "marked" {
		t.Fatalf("ranked athletes must precede NM rows: %+v", ranked)
	}
}

// TestFieldContinuation_CutTop8 is UC-011 #1: a flight of 12 with 3+3
// attempts and a cut to top 8 — after round 3 exactly the top 8 by best
// mark continue, in rule-correct (reverse-ranking, best last) order.
func TestFieldContinuation_CutTop8(t *testing.T) {
	var flight []FieldSeries
	// Athletes j01..j12 with bests 6.01..6.12: j12 leads the ranking.
	for i := 1; i <= 12; i++ {
		flight = append(flight, series(fmt.Sprintf("j%02d", i),
			valid(1, fmt.Sprintf("6.%02d", i)),
			Attempt{Seq: 2, Kind: AttemptFoul},
			Attempt{Seq: 3, Kind: AttemptPass},
		))
	}
	got := FieldContinuation(flight, 8)
	// Top 8 are j05..j12; reverse ranking order means worst of them first.
	want := []string{"j05", "j06", "j07", "j08", "j09", "j10", "j11", "j12"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("continuation order = %v, want %v", got, want)
	}
}

func TestFieldContinuation_BoundaryTieAllAdvance(t *testing.T) {
	flight := []FieldSeries{
		series("a", valid(1, "6.40")),
		series("b", valid(1, "6.30")),
		series("tie1", valid(1, "6.20")),
		series("tie2", valid(1, "6.20")),
	}
	got := FieldContinuation(flight, 3)
	if len(got) != 4 {
		t.Fatalf("identical series tied for the last place must all advance, got %v", got)
	}
}

func TestFieldContinuation_RetireeAndNMExcluded(t *testing.T) {
	flight := []FieldSeries{
		series("lead", valid(1, "6.50"), Attempt{Seq: 2, Kind: AttemptRetire}),
		series("nm", Attempt{Seq: 1, Kind: AttemptFoul}),
		series("stay", valid(1, "6.00")),
	}
	got := FieldContinuation(flight, 8)
	if !reflect.DeepEqual(got, []string{"stay"}) {
		t.Fatalf("retiree/NM must not continue, got %v", got)
	}
}

func TestFormatCentiMark(t *testing.T) {
	if got := FormatCentiMark(612, 2); got != "6.12" {
		t.Errorf("FormatCentiMark(612, 2) = %q", got)
	}
	if got := FormatCentiMark(1140, 1); got != "11.4" {
		t.Errorf("FormatCentiMark(1140, 1) = %q", got)
	}
	if got := FormatCentiMark(1005, 2); got != "10.05" {
		t.Errorf("FormatCentiMark(1005, 2) = %q", got)
	}
}
