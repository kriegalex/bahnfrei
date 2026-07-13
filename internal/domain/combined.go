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
}

// RankCombined computes totals, orders rows and assigns competition ranks
// (1, 2, 2, 4) per the UBS Kids Cup Reglement §3:
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
	out := make([]CombinedStanding, len(rows))
	copy(out, rows)
	for i := range out {
		out[i].Total = 0
		out[i].Complete = len(out[i].Performances) > 0
		for _, p := range out[i].Performances {
			if p.Points == nil {
				out[i].Complete = false
				continue
			}
			out[i].Total += *p.Points
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if c := compareCombined(out[i], out[j]); c != 0 {
			return c > 0
		}
		return out[i].AthleteID < out[j].AthleteID
	})
	for i := range out {
		if i > 0 && compareCombined(out[i-1], out[i]) == 0 {
			out[i].Rank = out[i-1].Rank
			continue
		}
		out[i].Rank = i + 1
	}
	return out
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
