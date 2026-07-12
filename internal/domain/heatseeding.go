// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"fmt"
	"math/rand"
	"sort"
)

// SeedCandidate is one entry ranked for heat seeding or lane draw: its seed
// mark (the SYS-026 "ranked seed performances" — original entry seed for a
// first round, or the previous round's settled mark for a later round) and
// club affiliation (D2.2 club-separation). Mark == "" means unseeded — the
// candidate is still assigned a heat/lane, just last in rank order.
type SeedCandidate struct {
	EntryID string
	ClubID  string
	Mark    string
}

// RankCandidates orders candidates by seed mark, best first (lower time is
// better — every laned/timed discipline this ranks; field-event seeding by
// mark is Later scope per D2.5). Unseeded or unparsable marks sort after
// every ranked candidate, in their given (stable) order — never dropped
// (SYS-026 seeds "confirmed entries", not just the ones with a known mark).
func RankCandidates(candidates []SeedCandidate) []SeedCandidate {
	type ranked struct {
		c     SeedCandidate
		centi int64
		ok    bool
		idx   int
	}
	rs := make([]ranked, len(candidates))
	for i, c := range candidates {
		centi, err := ParseCentiMark(c.Mark)
		rs[i] = ranked{c: c, centi: centi, ok: err == nil, idx: i}
	}
	sort.SliceStable(rs, func(i, j int) bool {
		if rs[i].ok != rs[j].ok {
			return rs[i].ok // ranked candidates sort before unranked ones
		}
		if !rs[i].ok {
			return false // preserve original order among unranked (stable sort handles it)
		}
		return rs[i].centi < rs[j].centi
	})
	out := make([]SeedCandidate, len(rs))
	for i, r := range rs {
		out[i] = r.c
	}
	return out
}

// ChooseHeatCount computes the number of heats needed so no heat exceeds
// maxHeatSize (SYS-026), as balanced as possible (the minimal heat count
// satisfying the cap). n or maxHeatSize <= 0 is a caller error.
func ChooseHeatCount(n, maxHeatSize int) (int, error) {
	if n <= 0 {
		return 0, fmt.Errorf("heat count: at least one candidate is required")
	}
	if maxHeatSize <= 0 {
		return 0, fmt.Errorf("heat count: a positive max heat size is required")
	}
	return (n + maxHeatSize - 1) / maxHeatSize, nil
}

// SeedHeats distributes ranked candidates into heatCount heats using
// standard serpentine ("boustrophedon") distribution (SYS-026, D2.2): rank 1
// to heat 0, rank 2 to heat 1, ..., then the direction reverses at the last
// heat and continues back down, guaranteeing the top heatCount seeds never
// share a heat. When clubSeparation is set, adjacent-rank same-club
// collisions the serpentine pass-boundary can create are resolved, where
// mathematically possible, by exchanging with the next-ranked candidate in
// a different heat (D2.2: "exchanges between heats should be made between
// athletes seeded in the same group of lanes").
func SeedHeats(ranked []SeedCandidate, heatCount int, clubSeparation bool) ([][]SeedCandidate, error) {
	if heatCount <= 0 {
		return nil, fmt.Errorf("seed heats: a positive heat count is required")
	}
	heatOf := make([]int, len(ranked))
	for i := range ranked {
		p := i / heatCount
		j := i % heatCount
		if p%2 == 0 {
			heatOf[i] = j
		} else {
			heatOf[i] = heatCount - 1 - j
		}
	}
	if clubSeparation {
		separateClubs(ranked, heatOf)
	}
	heats := make([][]SeedCandidate, heatCount)
	for i, c := range ranked {
		heats[heatOf[i]] = append(heats[heatOf[i]], c)
	}
	return heats, nil
}

// separateClubs mutates heatOf in place: for every adjacent-rank pair placed
// in the same heat by the serpentine pass boundary, if they share a
// non-empty club, it tries swapping the later-ranked candidate's heat with
// the next candidate's heat — the minimal-disruption exchange D2.2
// describes — provided the swap introduces no new same-club collision.
// Best-effort: a collision that cannot be resolved without breaking the
// separation rule elsewhere is left in place (D2.2's own qualifier,
// "whenever possible").
func separateClubs(ranked []SeedCandidate, heatOf []int) {
	for i := 0; i < len(ranked)-1; i++ {
		if heatOf[i] != heatOf[i+1] {
			continue
		}
		if ranked[i].ClubID == "" || ranked[i].ClubID != ranked[i+1].ClubID {
			continue
		}
		if i+2 >= len(ranked) {
			continue // no later candidate to exchange with
		}
		if heatOf[i+2] == heatOf[i+1] {
			continue // swapping would not change anything
		}
		newHeatForI1 := heatOf[i+2]
		newHeatForI2 := heatOf[i+1]
		if wouldCollide(ranked, heatOf, i+1, newHeatForI1) || wouldCollide(ranked, heatOf, i+2, newHeatForI2) {
			continue
		}
		heatOf[i+1], heatOf[i+2] = newHeatForI1, newHeatForI2
	}
}

// wouldCollide reports whether placing ranked[idx] into heat would put it
// alongside another candidate (excluding idx itself) from the same
// non-empty club.
func wouldCollide(ranked []SeedCandidate, heatOf []int, idx, heat int) bool {
	club := ranked[idx].ClubID
	if club == "" {
		return false
	}
	for i, h := range heatOf {
		if i == idx || h != heat {
			continue
		}
		if ranked[i].ClubID == club {
			return true
		}
	}
	return false
}

// DrawLanesGrouped assigns lanes to a fully-ranked field per the TR20.4
// grouped method (D2.3): rankedIDs[rankFrom-1:rankTo] within each group
// draw randomly among that group's lane set. rankedIDs must be ordered best
// (rank 1) first and its length must match the table's coverage exactly —
// callers choose the lane-group table keyed by the field size (SeedingRules
// .LaneGroupsFor). rng is caller-supplied so production draws use fresh
// entropy (repeated draws differ, UC-008 #3) while tests can inject a fixed
// seed for reproducibility.
func DrawLanesGrouped(rankedIDs []string, groups []LaneGroup, rng *rand.Rand) (map[string]int, error) {
	lanes := make(map[string]int, len(rankedIDs))
	for _, g := range groups {
		if g.RankTo > len(rankedIDs) {
			return nil, fmt.Errorf("lane draw: group ranks %d-%d exceed field size %d", g.RankFrom, g.RankTo, len(rankedIDs))
		}
		ids := rankedIDs[g.RankFrom-1 : g.RankTo]
		choices := append([]int(nil), g.Lanes...)
		rng.Shuffle(len(choices), func(i, j int) { choices[i], choices[j] = choices[j], choices[i] })
		for i, id := range ids {
			lanes[id] = choices[i]
		}
	}
	return lanes, nil
}

// DrawLanesByLot assigns availableLanes to ids by uniform random permutation
// (D2.3: "drawn by lot" for events longer than 800m, relays longer than
// 4x400m, or a field this table has no grouped rule for). len(availableLanes)
// must be >= len(ids).
func DrawLanesByLot(ids []string, availableLanes []int, rng *rand.Rand) (map[string]int, error) {
	if len(availableLanes) < len(ids) {
		return nil, fmt.Errorf("lane draw by lot: %d lanes available for %d entries", len(availableLanes), len(ids))
	}
	choices := append([]int(nil), availableLanes...)
	rng.Shuffle(len(choices), func(i, j int) { choices[i], choices[j] = choices[j], choices[i] })
	lanes := make(map[string]int, len(ids))
	for i, id := range ids {
		lanes[id] = choices[i]
	}
	return lanes, nil
}

// ShiftForUnusedLanes re-maps lane numbers drawn against an "effective"
// lane count onto a track's physical numbering when inside lanes are left
// unused (D2.3: "on a 9-lane track with 8 athletes, lane 2 is treated as
// lane 1 for Rule 20.4 purposes" — i.e. physical = effective + unused).
// unused <= 0 returns lanes unchanged.
func ShiftForUnusedLanes(lanes map[string]int, unused int) map[string]int {
	if unused <= 0 {
		return lanes
	}
	out := make(map[string]int, len(lanes))
	for id, l := range lanes {
		out[id] = l + unused
	}
	return out
}

// SequentialLanes returns the physical lane numbers 1..count, for building
// an availableLanes slice for DrawLanesByLot.
func SequentialLanes(count int) []int {
	out := make([]int, count)
	for i := range out {
		out[i] = i + 1
	}
	return out
}
