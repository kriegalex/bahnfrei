// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"fmt"
	"sort"
)

// VerticalTrial is one trial at one bar height in a vertical-jump series
// (SYS-043, UC-012): the D5.2 result-list symbols O (clear), X (fail), –
// (pass — the athlete takes no trial at this height at all) or r
// (retirement) recorded against a height in the unit's configured
// progression. Kind reuses the CR 25 QualificationStatus vocabulary
// (StatusO/X/Pass/R) rather than introducing a parallel one.
type VerticalTrial struct {
	HeightIdx int // index into the unit's configured height progression
	Seq       int // 1-based trial number within this height (1..3, D5.2/TR26.3)
	Kind      QualificationStatus
}

// Validate checks one vertical trial against the SYS-043 capture vocabulary:
// a non-negative height index, a trial number of 1-3 (WA TR26.3: at most
// three attempts per height) and one of the four legal symbols.
func (t VerticalTrial) Validate() error {
	if t.HeightIdx < 0 {
		return fmt.Errorf("vertical trial: height index must be >= 0")
	}
	if t.Seq < 1 || t.Seq > 3 {
		return fmt.Errorf("vertical trial: trial number must be 1-3 (D5.2/TR26.3: at most three attempts per height)")
	}
	switch t.Kind {
	case StatusO, StatusX, StatusPass, StatusR:
	default:
		return fmt.Errorf("vertical trial: kind %q is not one of O/X/–/r", t.Kind)
	}
	return nil
}

// Display renders the trial per the D5.2 result-list convention.
func (t VerticalTrial) Display() string { return string(t.Kind) }

// VerticalSeries is one athlete's full recorded trial history across a
// bar-height progression (SYS-043). Trials may be stored in any order —
// every derived query walks them in height-then-seq order.
type VerticalSeries struct {
	AthleteID string
	Trials    []VerticalTrial
}

// sortedTrials returns Trials ordered by height then trial seq — the order
// the WA TR26.4 elimination rule and the D5.2 display both walk.
func (s VerticalSeries) sortedTrials() []VerticalTrial {
	out := make([]VerticalTrial, len(s.Trials))
	copy(out, s.Trials)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].HeightIdx != out[j].HeightIdx {
			return out[i].HeightIdx < out[j].HeightIdx
		}
		return out[i].Seq < out[j].Seq
	})
	return out
}

// Retired reports whether the athlete explicitly retired (r): once
// retired, no further trials are legal (WA TR26.4/25.22).
func (s VerticalSeries) Retired() bool {
	for _, t := range s.Trials {
		if t.Kind == StatusR {
			return true
		}
	}
	return false
}

// Eliminated reports whether the series contains three consecutive
// failures, irrespective of the height(s) at which they occur (SYS-043,
// UC-012 #2). The running streak resets on a clear (O), is left unchanged
// by a pass (–: passing to the next height neither counts toward nor
// resets the streak — WA TR26.4/D5.2) and increments on a fail (X).
func (s VerticalSeries) Eliminated() bool {
	streak := 0
	for _, t := range s.sortedTrials() {
		switch t.Kind {
		case StatusO:
			streak = 0
		case StatusX:
			streak++
			if streak >= 3 {
				return true
			}
		case StatusPass:
			// no-op: neither resets nor counts (WA TR26.4).
		case StatusR:
			return false // an explicit retirement is not an elimination.
		}
	}
	return false
}

// BestHeightIdx returns the index of the highest height the athlete
// cleared (any O trial recorded there), and whether one exists.
func (s VerticalSeries) BestHeightIdx() (int, bool) {
	best := -1
	for _, t := range s.Trials {
		if t.Kind == StatusO && t.HeightIdx > best {
			best = t.HeightIdx
		}
	}
	if best < 0 {
		return 0, false
	}
	return best, true
}

// AttemptsAt returns the number of trials recorded at heightIdx — the
// SYS-043 countback rule's first tie-break ("fewest attempts at the height
// last cleared").
func (s VerticalSeries) AttemptsAt(heightIdx int) int {
	n := 0
	for _, t := range s.Trials {
		if t.HeightIdx == heightIdx {
			n++
		}
	}
	return n
}

// TotalFailures returns the total number of X trials across the entire
// series — the SYS-043 countback rule's second tie-break ("fewest total
// failures").
func (s VerticalSeries) TotalFailures() int {
	n := 0
	for _, t := range s.Trials {
		if t.Kind == StatusX {
			n++
		}
	}
	return n
}

// Status derives the series' D5.2 result status: r once the athlete
// retired (their best height still ranks, mirroring FieldSeries.Status's
// treatment of a horizontal-event retirement), NM when trials were taken
// but no height was ever cleared, none while the series is still open or
// already has a cleared height.
func (s VerticalSeries) Status() QualificationStatus {
	if s.Retired() {
		return StatusR
	}
	if _, ok := s.BestHeightIdx(); !ok && len(s.Trials) > 0 {
		return StatusNM
	}
	return StatusNone
}

// VerticalStanding is one athlete's line in a vertical-jump ranking
// (SYS-043, UC-012 #3).
type VerticalStanding struct {
	AthleteID      string
	BestHeightIdx  int // -1 when no height was cleared (unranked)
	AttemptsAtBest int
	TotalFailures  int
	Eliminated     bool
	Retired        bool
	Rank           int // 0 for unranked rows
	// TieForFirst flags a persisting tie at rank 1 after the full countback
	// (UC-012 #3): "flagged for jump-off/shared-first resolution per rule,
	// operator-selectable" — RankVertical only flags it; the office
	// resolves it either by accepting the shared rank or by recording
	// further trials at one or more additional (jump-off) heights appended
	// to the unit's progression, which this same ranking naturally
	// incorporates on recompute (a jump-off needs no separate mechanism:
	// an extra height is scored exactly like any other).
	TieForFirst bool
}

// RankVertical ranks a set of vertical-jump series by the SYS-043
// countback rule: higher best-height-cleared first; ties broken by fewer
// attempts at that height, then fewer total failures; a residual tie
// shares the rank. Athletes without a cleared height are listed after
// every ranked athlete, unranked (Rank 0), in athlete-ID order for
// determinism.
func RankVertical(series []VerticalSeries) []VerticalStanding {
	rows := make([]VerticalStanding, len(series))
	for i, s := range series {
		st := VerticalStanding{
			AthleteID:     s.AthleteID,
			Retired:       s.Retired(),
			Eliminated:    s.Eliminated(),
			TotalFailures: s.TotalFailures(),
			BestHeightIdx: -1,
		}
		if best, ok := s.BestHeightIdx(); ok {
			st.BestHeightIdx = best
			st.AttemptsAtBest = s.AttemptsAt(best)
		}
		rows[i] = st
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if c := compareVertical(rows[i], rows[j]); c != 0 {
			return c > 0
		}
		return rows[i].AthleteID < rows[j].AthleteID
	})
	for i := range rows {
		if rows[i].BestHeightIdx < 0 {
			continue // unranked: Rank stays 0
		}
		if i > 0 && rows[i-1].BestHeightIdx >= 0 && compareVertical(rows[i-1], rows[i]) == 0 {
			rows[i].Rank = rows[i-1].Rank
		} else {
			rows[i].Rank = i + 1
		}
	}
	if len(rows) > 1 && rows[0].Rank == 1 && rows[1].Rank == 1 {
		for i := range rows {
			if rows[i].Rank != 1 {
				break
			}
			rows[i].TieForFirst = true
		}
	}
	return rows
}

// compareVertical returns >0 when a ranks ahead of b, <0 the reverse, 0 for
// an official tie. Unrankable rows (no cleared height) always compare
// behind rankable ones.
func compareVertical(a, b VerticalStanding) int {
	ar, br := a.BestHeightIdx >= 0, b.BestHeightIdx >= 0
	switch {
	case ar && !br:
		return 1
	case !ar && br:
		return -1
	case !ar && !br:
		return 0
	}
	if a.BestHeightIdx != b.BestHeightIdx {
		return a.BestHeightIdx - b.BestHeightIdx
	}
	if a.AttemptsAtBest != b.AttemptsAtBest {
		return b.AttemptsAtBest - a.AttemptsAtBest // fewer attempts ranks ahead
	}
	if a.TotalFailures != b.TotalFailures {
		return b.TotalFailures - a.TotalFailures // fewer failures ranks ahead
	}
	return 0
}

// DefaultHeightIncrementCM is the WA-prescribed uniform bar increment per
// vertical-jump discipline code (source: World Athletics Combined Events
// rules, "In vertical jumps, the bar must be raised uniformly throughout
// the competition: increments of 3 cm in the high jump and 10 cm in the
// pole vault" — https://worldathleticsscores.com/resources/combined-events,
// fetched 2026-07-13; the same uniform-increment convention is the
// long-standing WA TR rule for standalone vertical jumps). SYS-043's
// "office-configurable, with defaults per spec" refers to this increment —
// the starting height is not spec-prescribed (it varies by category/
// ability) and is always office-supplied; see
// GenerateHeightProgression.
var DefaultHeightIncrementCM = map[string]int{
	"HJ": 3,
	"PV": 10,
}

// GenerateHeightProgression builds a bar-height progression (ascending,
// two-decimal metres) starting at startCM and rising by incrementCM per
// step, for stepCount steps — the helper an office configuring a unit uses
// to seed the default progression before adjusting it by hand.
func GenerateHeightProgression(startCM, incrementCM, stepCount int) []string {
	out := make([]string, stepCount)
	h := startCM
	for i := 0; i < stepCount; i++ {
		out[i] = FormatCentiMark(int64(h), 2)
		h += incrementCM
	}
	return out
}
