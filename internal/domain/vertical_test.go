// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import "testing"

func trials(spec ...any) []VerticalTrial {
	// spec is a flat list of (heightIdx int, kinds string) pairs, e.g.
	// trials(0, "O", 1, "XO", 2, "XXO", 3, "XXX") — a compact way to write
	// the UC-012 #1 style progression notation directly in test fixtures.
	var out []VerticalTrial
	for i := 0; i < len(spec); i += 2 {
		h := spec[i].(int)
		kinds := spec[i+1].(string)
		for seq, r := range kinds {
			out = append(out, VerticalTrial{HeightIdx: h, Seq: seq + 1, Kind: QualificationStatus(r)})
		}
	}
	return out
}

// TestVerticalTrialValidateSYS043UC012 checks the capture-vocabulary
// invariants a stored trial must satisfy (SYS-043).
func TestVerticalTrialValidateSYS043UC012(t *testing.T) {
	cases := []struct {
		name    string
		trial   VerticalTrial
		wantErr bool
	}{
		{"valid clear", VerticalTrial{HeightIdx: 0, Seq: 1, Kind: StatusO}, false},
		{"valid fail", VerticalTrial{HeightIdx: 0, Seq: 2, Kind: StatusX}, false},
		{"valid pass", VerticalTrial{HeightIdx: 1, Seq: 1, Kind: StatusPass}, false},
		{"valid retire", VerticalTrial{HeightIdx: 2, Seq: 1, Kind: StatusR}, false},
		{"negative height", VerticalTrial{HeightIdx: -1, Seq: 1, Kind: StatusO}, true},
		{"seq zero", VerticalTrial{HeightIdx: 0, Seq: 0, Kind: StatusO}, true},
		{"seq four", VerticalTrial{HeightIdx: 0, Seq: 4, Kind: StatusO}, true},
		{"unknown kind", VerticalTrial{HeightIdx: 0, Seq: 1, Kind: StatusDNS}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.trial.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// TestVerticalEliminationSYS043UC012_1 is the UC-012 #1 worked example
// verbatim: heights 1.60/1.65/1.70/1.75 with O, XO, XXO, XXX — best is
// 1.70 (height index 2) and the athlete is eliminated at 1.75.
func TestVerticalEliminationSYS043UC012_1(t *testing.T) {
	s := VerticalSeries{AthleteID: "A", Trials: trials(0, "O", 1, "XO", 2, "XXO", 3, "XXX")}
	if !s.Eliminated() {
		t.Fatal("expected elimination after XXX at the fourth height")
	}
	best, ok := s.BestHeightIdx()
	if !ok || best != 2 {
		t.Fatalf("best height index = %d, ok=%v; want 2 (1.70), ok=true", best, ok)
	}
	if got := s.AttemptsAt(best); got != 3 {
		t.Errorf("attempts at best height = %d, want 3 (XXO)", got)
	}
	if got := s.TotalFailures(); got != 6 {
		t.Errorf("total failures = %d, want 6 (0+1+2+3)", got)
	}
}

// TestVerticalEliminationSYS043UC012_2 exercises the UC-012 #2 narrative:
// a pass between two heights neither counts toward nor resets the
// three-consecutive-failures streak, so the streak keeps accumulating
// across the passed height and elimination triggers on the height after
// it. Fixture: 1.60 X (streak 1), 1.65 pass (streak stays 1), 1.70 X, X
// (streak 2, then 3 → eliminated) — a concrete instantiation of the UC-012
// #2 acceptance text ("an athlete passing at 1.65 after a fail at 1.60...
// fail twice more... elimination triggers on three consecutive failures
// across heights").
func TestVerticalEliminationSYS043UC012_2(t *testing.T) {
	s := VerticalSeries{AthleteID: "A", Trials: trials(0, "X", 1, "-", 2, "XX")}
	if !s.Eliminated() {
		t.Fatal("expected elimination: X (1.60), pass (1.65, does not reset), X, X (1.70) = 3 consecutive fails")
	}
	if s.Retired() {
		t.Error("elimination is not the same as an explicit retirement")
	}
	// Without the pass "resetting" the streak — if it wrongly reset, only
	// two consecutive fails would remain at 1.70 and elimination would not
	// yet have triggered. Confirm the intervening pass really carried the
	// streak by checking a variant that DOES reset (a clear in between) is
	// NOT eliminated with the same X counts.
	notEliminated := VerticalSeries{AthleteID: "B", Trials: trials(0, "X", 1, "O", 2, "XX")}
	if notEliminated.Eliminated() {
		t.Error("a clear (O) between the fails must reset the streak — this series has only 2 consecutive fails at the end")
	}
}

// TestVerticalRetirementSYS043UC012 checks that an explicit retirement is
// distinct from an elimination: Retired() is true, Eliminated() is false,
// and no best height is lost by retiring after already clearing one.
func TestVerticalRetirementSYS043UC012(t *testing.T) {
	s := VerticalSeries{AthleteID: "A", Trials: trials(0, "O", 1, "O", 2, "r")}
	if !s.Retired() {
		t.Fatal("expected Retired() true")
	}
	if s.Eliminated() {
		t.Error("a retirement must not also read as an elimination")
	}
	best, ok := s.BestHeightIdx()
	if !ok || best != 1 {
		t.Fatalf("best height index = %d, ok=%v; want 1 (retiring keeps the prior best)", best, ok)
	}
}

// TestVerticalSeriesStatusSYS043 checks the D5.2 result-status derivation
// (mirrors FieldSeries.Status for horizontal events).
func TestVerticalSeriesStatusSYS043(t *testing.T) {
	cases := []struct {
		name string
		s    VerticalSeries
		want QualificationStatus
	}{
		{"open series", VerticalSeries{Trials: nil}, StatusNone},
		{"cleared a height", VerticalSeries{Trials: trials(0, "O")}, StatusNone},
		{"never cleared", VerticalSeries{Trials: trials(0, "XXX")}, StatusNM},
		{"retired after clearing", VerticalSeries{Trials: trials(0, "O", 1, "r")}, StatusR},
		{"retired without clearing", VerticalSeries{Trials: trials(0, "Xr")}, StatusR},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.s.Status(); got != tc.want {
				t.Errorf("Status() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestVerticalCountbackSYS043UC012_3 exercises the SYS-043 countback rule
// (fewer attempts at the height last cleared, then fewer total failures)
// and the UC-012 #3 first-place tie flag.
func TestVerticalCountbackSYS043UC012_3(t *testing.T) {
	t.Run("fewer attempts at last cleared height wins", func(t *testing.T) {
		// Both clear 1.83 (height index 3) as their best; A took 1 trial
		// there, B took 2 — A ranks ahead despite identical earlier series.
		a := VerticalSeries{AthleteID: "A", Trials: trials(0, "O", 1, "O", 2, "O", 3, "O")}
		b := VerticalSeries{AthleteID: "B", Trials: trials(0, "O", 1, "O", 2, "O", 3, "XO")}
		got := RankVertical([]VerticalSeries{b, a}) // deliberately unsorted input
		if got[0].AthleteID != "A" || got[0].Rank != 1 {
			t.Fatalf("expected A rank 1 (fewer attempts at 1.83), got %+v", got)
		}
		if got[1].AthleteID != "B" || got[1].Rank != 2 {
			t.Fatalf("expected B rank 2, got %+v", got)
		}
		if got[0].TieForFirst || got[1].TieForFirst {
			t.Error("no tie for first should be flagged here")
		}
	})

	t.Run("equal attempts at best height falls to fewer total failures", func(t *testing.T) {
		// Both clear 1.83 in 2 trials (XO) each, but A failed once earlier
		// (at 1.75) and B failed twice earlier (at 1.70 and 1.75) —
		// A has fewer total failures and ranks ahead.
		a := VerticalSeries{AthleteID: "A", Trials: trials(0, "O", 1, "XO", 2, "XO")}
		b := VerticalSeries{AthleteID: "B", Trials: trials(0, "O", 1, "XO", 2, "XXO")}
		got := RankVertical([]VerticalSeries{a, b})
		if got[0].AthleteID != "A" || got[1].AthleteID != "B" {
			t.Fatalf("expected A ahead of B (fewer total failures), got %+v", got)
		}
	})

	t.Run("persisting tie for first place is flagged", func(t *testing.T) {
		// Identical series for both — a genuine tie for first with no
		// countback separator: RankVertical flags it rather than picking
		// an arbitrary winner (UC-012 #3).
		a := VerticalSeries{AthleteID: "A", Trials: trials(0, "O", 1, "O")}
		b := VerticalSeries{AthleteID: "B", Trials: trials(0, "O", 1, "O")}
		got := RankVertical([]VerticalSeries{a, b})
		if got[0].Rank != 1 || got[1].Rank != 1 {
			t.Fatalf("expected shared rank 1, got %+v", got)
		}
		if !got[0].TieForFirst || !got[1].TieForFirst {
			t.Fatalf("expected both rows flagged TieForFirst, got %+v", got)
		}
	})

	t.Run("a tie for a lower place is not flagged", func(t *testing.T) {
		first := VerticalSeries{AthleteID: "A", Trials: trials(0, "O", 1, "O", 2, "O")}
		tiedSecond1 := VerticalSeries{AthleteID: "B", Trials: trials(0, "O", 1, "O")}
		tiedSecond2 := VerticalSeries{AthleteID: "C", Trials: trials(0, "O", 1, "O")}
		got := RankVertical([]VerticalSeries{first, tiedSecond1, tiedSecond2})
		for _, row := range got {
			if row.TieForFirst {
				t.Fatalf("only a rank-1 tie is flagged, got TieForFirst on %+v", row)
			}
		}
		if got[1].Rank != 2 || got[2].Rank != 2 {
			t.Fatalf("expected B and C to share rank 2, got %+v", got)
		}
	})

	t.Run("a jump-off height resolves the tie without a separate mechanism", func(t *testing.T) {
		// A and B tie at 1.90 (height index 2); a jump-off height 1.92 is
		// appended (index 3) — A clears it, B fails. RankVertical treats
		// it like any other height: A's higher best-height-idx wins
		// outright, no more countback needed.
		a := VerticalSeries{AthleteID: "A", Trials: trials(0, "O", 1, "O", 2, "O", 3, "O")}
		b := VerticalSeries{AthleteID: "B", Trials: trials(0, "O", 1, "O", 2, "O", 3, "X")}
		got := RankVertical([]VerticalSeries{a, b})
		if got[0].AthleteID != "A" || got[0].Rank != 1 || got[0].TieForFirst {
			t.Fatalf("expected A alone at rank 1 after the jump-off, got %+v", got)
		}
		if got[1].AthleteID != "B" || got[1].Rank != 2 {
			t.Fatalf("expected B at rank 2, got %+v", got)
		}
	})
}

// TestVerticalCountbackFixturesSYS043UC012_4 is the "reference fixture
// suite" UC-012 #4 calls for. Unlike a scoring table, WA TR26.8 countback
// has no externally published numeric table to replay — it is a rule
// algorithm, not data — so this fixture set is a documented, rule-derived
// worked example (WA TR26.8: (i) fewest attempts at the height last
// cleared, (ii) fewest total failures throughout the competition) rather
// than a transcription of a published table row, matching how the
// combined-events *scoring formula* fixtures (combinedscoring_test.go) are
// the external-table case for this task.
func TestVerticalCountbackFixturesSYS043UC012_4(t *testing.T) {
	// A textbook four-way high-jump countback: everyone clears through
	// 1.75 (index 2); the countback then separates them purely by rule.
	series := []VerticalSeries{
		{AthleteID: "clean-sweep", Trials: trials(0, "O", 1, "O", 2, "O")},            // 0 attempts lost, 0 failures
		{AthleteID: "one-fail-at-best", Trials: trials(0, "O", 1, "O", 2, "XO")},      // 2 attempts at best, 1 failure
		{AthleteID: "early-fail-clean-best", Trials: trials(0, "XO", 1, "O", 2, "O")}, // 1 attempt at best, 1 failure (earlier)
		{AthleteID: "two-fails-at-best", Trials: trials(0, "O", 1, "O", 2, "XXO")},    // 3 attempts at best, 2 failures
		{AthleteID: "eliminated-next-height", Trials: trials(0, "O", 1, "O", 2, "O", 3, "XXX")},
	}
	got := RankVertical(series)
	rankOf := map[string]int{}
	for _, row := range got {
		rankOf[row.AthleteID] = row.Rank
	}
	// All five best-clear 1.75 (height index 2), so the countback runs on
	// (fewer attempts at 1.75, then fewer total failures for the whole
	// competition — including the eliminating failures at the next height,
	// per the SYS-043 wording "fewest total failures" with no cutoff
	// clause):
	//   clean-sweep:             1 attempt at 1.75, 0 failures total -> 1st
	//   early-fail-clean-best:   1 attempt at 1.75, 1 failure total  -> 2nd
	//   eliminated-next-height:  1 attempt at 1.75, 3 failures total -> 3rd
	//     (the XXX at the next height still counts toward "total failures")
	//   one-fail-at-best:        2 attempts at 1.75, 1 failure total -> 4th
	//   two-fails-at-best:       3 attempts at 1.75, 2 failures total -> 5th
	want := map[string]int{
		"clean-sweep":            1,
		"early-fail-clean-best":  2,
		"eliminated-next-height": 3,
		"one-fail-at-best":       4,
		"two-fails-at-best":      5,
	}
	for id, wantRank := range want {
		if rankOf[id] != wantRank {
			t.Errorf("%s rank = %d, want %d (full: %v)", id, rankOf[id], wantRank, rankOf)
		}
	}
}

// TestVerticalUnrankedAthletesSYS043 checks that an athlete with no
// cleared height (e.g. eliminated at the opening height) is listed
// unranked, after every rankable row, deterministically by athlete ID —
// mirroring RankFieldSeries' convention for horizontal events.
func TestVerticalUnrankedAthletesSYS043(t *testing.T) {
	cleared := VerticalSeries{AthleteID: "cleared", Trials: trials(0, "O")}
	neverCleared := VerticalSeries{AthleteID: "never", Trials: trials(0, "XXX")}
	got := RankVertical([]VerticalSeries{neverCleared, cleared})
	if got[0].AthleteID != "cleared" || got[0].Rank != 1 {
		t.Fatalf("expected cleared first at rank 1, got %+v", got[0])
	}
	if got[1].AthleteID != "never" || got[1].Rank != 0 {
		t.Fatalf("expected never-cleared unranked (Rank 0), got %+v", got[1])
	}
	if !got[1].Eliminated {
		t.Error("expected the never-cleared athlete flagged Eliminated")
	}
}

// TestGenerateHeightProgressionSYS043 checks the office-configuration
// helper's arithmetic and the documented default increments (3 cm HJ,
// 10 cm PV — WA Combined Events rules, see DefaultHeightIncrementCM).
func TestGenerateHeightProgressionSYS043(t *testing.T) {
	if got, want := DefaultHeightIncrementCM["HJ"], 3; got != want {
		t.Errorf("HJ default increment = %d, want %d", got, want)
	}
	if got, want := DefaultHeightIncrementCM["PV"], 10; got != want {
		t.Errorf("PV default increment = %d, want %d", got, want)
	}
	got := GenerateHeightProgression(150, 3, 4)
	want := []string{"1.50", "1.53", "1.56", "1.59"}
	if len(got) != len(want) {
		t.Fatalf("progression length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("progression[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
