// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import "sort"

// CombinedPerformance is one discipline's contribution to a combined-event
// standing: the settled mark and the points it scored. Points nil means no
// scoring result exists (not yet competed, DNS, NM, DQ, …) — the standings
// must show that gap explicitly, never hide it (UC-033 #3).
type CombinedPerformance struct {
	DisciplineCode string
	Mark           string
	Status         QualificationStatus
	Points         *int
	// RecordFlags carries the SYS-049 record/best flags (e.g. "MR", "PB")
	// through to every standings renderer built on CombinedPerformance
	// (public results, the printed result list) — the same flags the
	// operator capture view shows (SYS-049 "flags SHALL appear in operator
	// views, public results, and exports").
	RecordFlags []string
}

// CombinedStanding is one athlete's row in a combined-event ranking.
// Performances are aligned by index across all rows (template discipline
// order), which is what the Reglement's per-discipline tie comparison
// operates on.
type CombinedStanding struct {
	AthleteID    string
	Performances []CombinedPerformance
	Total        int
	Rank         int
	Complete     bool // every discipline has a scoring result
	// OutOfCompetition marks an athlete competing ausser Konkurrenz/hors
	// concours (DEC-016/OQ-020 investigation of the LV Langenthal evidence's
	// "n.a." row with every mark present — Thome Lauriane, W12,
	// https://lvl.ch/images/resultate/2025/Gesamtrangliste_UBSKidsCup_2025.pdf):
	// marks are still captured and shown, but the athlete never holds a
	// numeric Rank, in provisional or final standings alike.
	OutOfCompetition bool
}

// CombinedTieBreak names the equal-totals ranking policy a combined-events
// standing applies. It is rule-shaped data (ADR-005 §4): the governing
// series/federation rule set decides it, not code.
type CombinedTieBreak string

const (
	// TieBreakMajorityThenHighest is the UBS Kids Cup Reglement §3 policy:
	// on equal totals, better points in the majority of disciplines wins,
	// then the highest (then next-highest, …) single-discipline points.
	TieBreakMajorityThenHighest CombinedTieBreak = "majority-then-highest"
	// TieBreakTiesStand is the World Athletics combined-events policy
	// (CR&TR 2026 edition, TR 39 Combined Events Competitions, final
	// placing provision, verified 2026-07-15 — OQ-045): "If two or more
	// athletes achieve an equal number of points for any place in the
	// competition, it shall be determined as a tie." WA abolished the
	// pre-2020 majority/highest-event tie-break; equal totals share the
	// place with no further comparison.
	TieBreakTiesStand CombinedTieBreak = "ties-stand"
)

// RankCombined computes totals, orders rows and assigns competition ranks
// (1, 2, 2, 4) per the UBS Kids Cup Reglement §3 (the
// TieBreakMajorityThenHighest policy):
//
//   - the total is the sum of the discipline points ("Dreikampfresultat");
//     a missing discipline contributes 0 points but still ranks by total
//     (series convention; the Reglement is silent — see OQ-020),
//   - on equal totals, whoever has the better points in the majority of
//     disciplines ("in zwei der drei Disziplinen") ranks first,
//   - failing that, whoever holds the highest single-discipline points
//     ranks first (extended to the next-highest points on repeated
//     equality, so that per the Reglement equal ranks occur only when all
//     discipline points coincide),
//   - rows with identical points multisets share a rank.
//
// Rows must have index-aligned Performances; ordering is deterministic
// (athlete ID breaks presentation order, not rank) so repeated computation
// is stable.
func RankCombined(rows []CombinedStanding) []CombinedStanding {
	return RankCombinedWithTieBreak(rows, TieBreakMajorityThenHighest)
}

// RankCombinedWithTieBreak is RankCombined under an explicit equal-totals
// policy: TieBreakMajorityThenHighest (UKC Reglement §3) or TieBreakTiesStand
// (WA TR 39: equal points for any place is a tie — SYS-044/UC-013 combined
// events). An unknown policy falls back to the UKC behaviour, matching
// RankCombined's long-standing default.
//
// This is the PROVISIONAL ranking rule (UC-033 #3, DEC-016): every row
// still ranks by its (possibly partial) total, missing disciplines
// contributing 0 — the meet is ongoing and every athlete is temporarily
// "incomplete" at some point, so nothing here can wait for completeness.
// A row with OutOfCompetition set never holds a numeric Rank in either
// mode; see FinalRankCombined for the additional FINAL-only rule (a
// discipline missing entirely, as opposed to attempted with no valid
// result, makes a row unranked once the division's series is complete).
func RankCombinedWithTieBreak(rows []CombinedStanding, tieBreak CombinedTieBreak) []CombinedStanding {
	cmp := compareCombined
	if tieBreak == TieBreakTiesStand {
		cmp = func(a, b CombinedStanding) int { return a.Total - b.Total }
	}
	var eligible, ooc []CombinedStanding
	for _, r := range rows {
		r.Total, r.Complete = combinedTotal(r.Performances)
		if r.OutOfCompetition {
			r.Rank = 0
			ooc = append(ooc, r)
			continue
		}
		eligible = append(eligible, r)
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		if c := cmp(eligible[i], eligible[j]); c != 0 {
			return c > 0
		}
		return eligible[i].AthleteID < eligible[j].AthleteID
	})
	for i := range eligible {
		if i > 0 && cmp(eligible[i-1], eligible[i]) == 0 {
			eligible[i].Rank = eligible[i-1].Rank
			continue
		}
		eligible[i].Rank = i + 1
	}
	sort.SliceStable(ooc, func(i, j int) bool { return ooc[i].AthleteID < ooc[j].AthleteID })
	return append(eligible, ooc...)
}

// FinalRankCombined computes FINAL combined-event standings (UC-033 #3,
// DEC-016/OQ-020): the official TAF3 convention observed in the LV
// Langenthal Gesamtrangliste, 17.05.2025 (https://lvl.ch/images/resultate/
// 2025/Gesamtrangliste_UBSKidsCup_2025.pdf). Unlike the provisional rule
// (RankCombinedWithTieBreak), a row missing at least one discipline
// entirely — no captured result at all, not even an invalid-attempt status
// — is excluded from ranking and appended after every ranked row (Rank 0,
// deterministic AthleteID order), matching the evidence's unranked
// "aufg." rows (e.g. Joao Daniella, M14). A discipline the athlete
// attempted but produced no valid result for (AttemptedNoValidResult) is
// NOT "missing" here — the evidence's "ogV" rows still rank normally once
// every other discipline is complete (e.g. Geiser Lukas, W9, rank 28 with
// two ogV disciplines) — callers apply the scoring table's
// NoValidAttemptFloor before calling this so such a discipline already
// carries Points. OutOfCompetition rows are always unranked too (the
// evidence's Thome Lauriane row: every mark present, still "n.a." —
// OQ-090/OQ-091 track the residual investigation).
func FinalRankCombined(rows []CombinedStanding, tieBreak CombinedTieBreak) []CombinedStanding {
	var rankable, unranked []CombinedStanding
	for _, r := range rows {
		total, complete := combinedTotal(r.Performances)
		if !r.OutOfCompetition && complete {
			rankable = append(rankable, r)
			continue
		}
		r.Total, r.Complete, r.Rank = total, complete, 0
		unranked = append(unranked, r)
	}
	ranked := RankCombinedWithTieBreak(rankable, tieBreak)
	sort.SliceStable(unranked, func(i, j int) bool { return unranked[i].AthleteID < unranked[j].AthleteID })
	return append(ranked, unranked...)
}

// combinedTotal sums a row's known discipline points and reports whether
// every discipline has one — the shared totals/completeness computation
// both ranking functions apply identically.
func combinedTotal(perf []CombinedPerformance) (total int, complete bool) {
	complete = len(perf) > 0
	for _, p := range perf {
		if p.Points == nil {
			complete = false
			continue
		}
		total += *p.Points
	}
	return total, complete
}

// AttemptedNoValidResult reports whether status marks a discipline the
// athlete was present for and attempted, but which produced no valid
// result: NM/NH (field — every trial failed) or DNF (track — started, no
// valid time), or R (retired without ever posting a valid mark). A scoring
// table's NoValidAttemptFloor (UC-033 #3, DEC-016/OQ-020 — the official
// "ogV" rule, 1 point in the evidenced UKC table) applies to these.
// StatusNone (never captured) and StatusDNS (did not start) do not — those
// are the "missing discipline" FinalRankCombined lists unranked at the
// bottom. StatusDQ is excluded too: a disqualification is a rule
// violation, not merely "no valid attempt", and the evidence has no DQ row
// to confirm either treatment.
func AttemptedNoValidResult(status QualificationStatus) bool {
	switch status {
	case StatusNM, StatusNH, StatusDNF, StatusR:
		return true
	default:
		return false
	}
}

// compareCombined returns >0 when a ranks ahead of b, <0 when b ranks ahead
// of a, and 0 for an official tie (identical points multisets).
func compareCombined(a, b CombinedStanding) int {
	if a.Total != b.Total {
		return a.Total - b.Total
	}
	// Majority rule: better points in two of the three disciplines.
	winsA, winsB := 0, 0
	n := len(a.Performances)
	for i := 0; i < n && i < len(b.Performances); i++ {
		pa, pb := points(a.Performances, i), points(b.Performances, i)
		switch {
		case pa > pb:
			winsA++
		case pb > pa:
			winsB++
		}
	}
	if winsA > n/2 || winsB > n/2 {
		return winsA - winsB
	}
	// Highest single points, then next-highest, and so on.
	sa, sb := sortedPoints(a.Performances), sortedPoints(b.Performances)
	for i := range sa {
		if i >= len(sb) {
			return 1
		}
		if sa[i] != sb[i] {
			return sa[i] - sb[i]
		}
	}
	if len(sb) > len(sa) {
		return -1
	}
	return 0
}

func points(ps []CombinedPerformance, i int) int {
	if i >= len(ps) || ps[i].Points == nil {
		return 0
	}
	return *ps[i].Points
}

func sortedPoints(ps []CombinedPerformance) []int {
	out := make([]int, len(ps))
	for i := range ps {
		out[i] = points(ps, i)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out
}
