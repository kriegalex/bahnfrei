// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"fmt"
	"math/rand"
	"testing"
)

func mustSeedingRules(t *testing.T) *SeedingRules {
	t.Helper()
	r, err := BuiltinSeedingRules(SeedingRulesTR20)
	if err != nil {
		t.Fatalf("load built-in TR20 seeding rules: %v", err)
	}
	return r
}

// TestSerpentineSeedingSYS026UC008_1 reproduces UC-008 #1 (D2.2): 21
// confirmed 100m entries with seed times and 8 lanes generate 3 heats of 7,
// seeded serpentine by seed mark, and no two of the top-3 seeds share a heat.
func TestSerpentineSeedingSYS026UC008_1(t *testing.T) {
	var candidates []SeedCandidate
	for i := 0; i < 21; i++ {
		// 10.50, 10.55, 10.60, ... strictly increasing centiseconds.
		centi := 1050 + i*5
		candidates = append(candidates, SeedCandidate{
			EntryID: fmt.Sprintf("e%02d", i+1),
			Mark:    fmt.Sprintf("%d.%02d", centi/100, centi%100),
		})
	}

	ranked := RankCandidates(candidates)
	heatCount, err := ChooseHeatCount(len(ranked), 7)
	if err != nil {
		t.Fatalf("choose heat count: %v", err)
	}
	if heatCount != 3 {
		t.Fatalf("expected 3 heats for 21 entries at max 7/heat, got %d", heatCount)
	}
	heats, err := SeedHeats(ranked, heatCount, true)
	if err != nil {
		t.Fatalf("seed heats: %v", err)
	}
	if len(heats) != 3 {
		t.Fatalf("expected 3 heats, got %d", len(heats))
	}
	for i, h := range heats {
		if len(h) != 7 {
			t.Errorf("heat %d: expected 7 athletes, got %d", i, len(h))
		}
	}

	// The top-3 seeds (e01, e02, e03) must land in three different heats.
	heatOfTop3 := map[string]int{}
	for hi, h := range heats {
		for _, c := range h {
			if c.EntryID == "e01" || c.EntryID == "e02" || c.EntryID == "e03" {
				heatOfTop3[c.EntryID] = hi
			}
		}
	}
	if len(heatOfTop3) != 3 {
		t.Fatalf("expected to find all 3 top seeds, found %d", len(heatOfTop3))
	}
	seen := map[int]bool{}
	for id, hi := range heatOfTop3 {
		if seen[hi] {
			t.Errorf("top-3 seed %s shares heat %d with another top-3 seed", id, hi)
		}
		seen[hi] = true
	}
}

// TestClubSeparationSYS026UC008_2 reproduces UC-008 #2 (D2.2): two athletes
// of the same club in adjacent seed positions land in different heats where
// mathematically possible.
func TestClubSeparationSYS026UC008_2(t *testing.T) {
	// 6 candidates, 2 heats of 3 (serpentine: heat pattern 0,1,0,1,0,1 for
	// ranks 1..6 is NOT what happens; with heatCount=2 the pattern is
	// 0,1,1,0,0,1 — ranks 2 and 3 collide in heat 1). Make ranks 2 and 3 the
	// same club so the pass-boundary collision is exercised.
	candidates := []SeedCandidate{
		{EntryID: "r1", ClubID: "clubA", Mark: "10.00"},
		{EntryID: "r2", ClubID: "clubX", Mark: "10.10"},
		{EntryID: "r3", ClubID: "clubX", Mark: "10.20"}, // same club as r2, adjacent rank
		{EntryID: "r4", ClubID: "clubB", Mark: "10.30"},
		{EntryID: "r5", ClubID: "clubC", Mark: "10.40"},
		{EntryID: "r6", ClubID: "clubD", Mark: "10.50"},
	}
	ranked := RankCandidates(candidates)
	heats, err := SeedHeats(ranked, 2, true)
	if err != nil {
		t.Fatalf("seed heats: %v", err)
	}
	heatOf := map[string]int{}
	for hi, h := range heats {
		for _, c := range h {
			heatOf[c.EntryID] = hi
		}
	}
	if heatOf["r2"] == heatOf["r3"] {
		t.Errorf("expected club-separation to place r2 and r3 (both clubX) in different heats, both landed in heat %d", heatOf["r2"])
	}
	// Without club separation the collision must reproduce (proves the test
	// actually exercises the pass-boundary case, not a no-op).
	heatsNoSep, err := SeedHeats(ranked, 2, false)
	if err != nil {
		t.Fatalf("seed heats (no separation): %v", err)
	}
	heatOfNoSep := map[string]int{}
	for hi, h := range heatsNoSep {
		for _, c := range h {
			heatOfNoSep[c.EntryID] = hi
		}
	}
	if heatOfNoSep["r2"] != heatOfNoSep["r3"] {
		t.Fatalf("test setup error: expected r2/r3 to collide without club separation")
	}
}

// TestLaneDrawGroupedSYS027UC008_3 reproduces UC-008 #3 (D2.3, TR20.4): a
// 400m final field ranked 1-8 with 8 lanes draws ranks 1-4 into lanes 4-7,
// ranks 5-6 into lanes 3/8, ranks 7-8 into lanes 1/2 — random within group,
// so repeated draws produce different in-group permutations.
func TestLaneDrawGroupedSYS027UC008_3(t *testing.T) {
	rules := mustSeedingRules(t)
	if !rules.IsLaneRace("400m") {
		t.Fatal("400m must be classified as a lane race (D2.3)")
	}
	groups, ok := rules.LaneGroupsFor(8)
	if !ok {
		t.Fatal("expected an 8-lane TR20.4 group table")
	}

	ranked := make([]string, 8)
	for i := range ranked {
		ranked[i] = fmt.Sprintf("rank%d", i+1)
	}

	wantGroup := map[string]map[int]bool{
		"rank1": {4: true, 5: true, 6: true, 7: true},
		"rank2": {4: true, 5: true, 6: true, 7: true},
		"rank3": {4: true, 5: true, 6: true, 7: true},
		"rank4": {4: true, 5: true, 6: true, 7: true},
		"rank5": {3: true, 8: true},
		"rank6": {3: true, 8: true},
		"rank7": {1: true, 2: true},
		"rank8": {1: true, 2: true},
	}

	seenPermutations := map[[8]int]bool{}
	for seed := int64(0); seed < 20; seed++ {
		lanes, err := DrawLanesGrouped(ranked, groups, rand.New(rand.NewSource(seed)))
		if err != nil {
			t.Fatalf("draw lanes: %v", err)
		}
		if len(lanes) != 8 {
			t.Fatalf("expected 8 lane assignments, got %d", len(lanes))
		}
		used := map[int]bool{}
		var perm [8]int
		for i, id := range ranked {
			lane, ok := lanes[id]
			if !ok {
				t.Fatalf("no lane assigned to %s", id)
			}
			if used[lane] {
				t.Fatalf("lane %d assigned twice", lane)
			}
			used[lane] = true
			if !wantGroup[id][lane] {
				t.Errorf("%s (%s): lane %d is not in its TR20.4 group %v", id, id, lane, wantGroup[id])
			}
			perm[i] = lane
		}
		seenPermutations[perm] = true
	}
	if len(seenPermutations) < 2 {
		t.Error("expected repeated draws to produce different in-group permutations (UC-008 #3 randomness)")
	}
}

// TestLaneDrawDeterministicWithSeed proves lane draws are reproducible given
// the same RNG seed (verification/regression tooling requirement).
func TestLaneDrawDeterministicWithSeed(t *testing.T) {
	rules := mustSeedingRules(t)
	groups, _ := rules.LaneGroupsFor(8)
	ranked := make([]string, 8)
	for i := range ranked {
		ranked[i] = fmt.Sprintf("rank%d", i+1)
	}
	a, err := DrawLanesGrouped(ranked, groups, rand.New(rand.NewSource(42)))
	if err != nil {
		t.Fatalf("draw a: %v", err)
	}
	b, err := DrawLanesGrouped(ranked, groups, rand.New(rand.NewSource(42)))
	if err != nil {
		t.Fatalf("draw b: %v", err)
	}
	for id, lane := range a {
		if b[id] != lane {
			t.Errorf("same seed produced different lanes for %s: %d vs %d", id, lane, b[id])
		}
	}
}

// TestUnusedInnerLanesSYS027UC008_4 reproduces UC-008 #4 (D2.3): a 9-lane
// track with 8 athletes and lane 1 configured unused shifts assignments per
// the rule's adapted numbering — lane 2 becomes "lane 1" for TR20.4
// purposes, so drawn effective lanes 1..8 map onto physical lanes 2..9.
func TestUnusedInnerLanesSYS027UC008_4(t *testing.T) {
	rules := mustSeedingRules(t)
	groups, _ := rules.LaneGroupsFor(8)
	ranked := make([]string, 8)
	for i := range ranked {
		ranked[i] = fmt.Sprintf("rank%d", i+1)
	}
	effective, err := DrawLanesGrouped(ranked, groups, rand.New(rand.NewSource(1)))
	if err != nil {
		t.Fatalf("draw lanes: %v", err)
	}
	trackLanes, fieldSize := 9, 8
	unused := trackLanes - fieldSize
	if unused != 1 {
		t.Fatalf("test setup error: expected exactly one unused inner lane")
	}
	physical := ShiftForUnusedLanes(effective, unused)
	for id, effLane := range effective {
		want := effLane + 1
		if physical[id] != want {
			t.Errorf("%s: effective lane %d should map to physical lane %d, got %d", id, effLane, want, physical[id])
		}
		if physical[id] < 2 || physical[id] > 9 {
			t.Errorf("%s: physical lane %d is outside the used range [2,9] (lane 1 must stay unused)", id, physical[id])
		}
	}
}

// TestLaneDrawByLotSYS027 reproduces D2.3's "events longer than 800m...
// drawn by lot" rule for a discipline not in the grouped table.
func TestLaneDrawByLotSYS027(t *testing.T) {
	rules := mustSeedingRules(t)
	if rules.IsLaneRace("1500m") {
		t.Fatal("1500m must not be classified as a TR20.4 grouped lane race")
	}
	ids := []string{"a", "b", "c", "d"}
	lanes, err := DrawLanesByLot(ids, SequentialLanes(6), rand.New(rand.NewSource(7)))
	if err != nil {
		t.Fatalf("draw by lot: %v", err)
	}
	used := map[int]bool{}
	for _, id := range ids {
		lane := lanes[id]
		if lane < 1 || lane > 6 {
			t.Errorf("%s: lane %d outside available range", id, lane)
		}
		if used[lane] {
			t.Errorf("lane %d assigned twice", lane)
		}
		used[lane] = true
	}
}

// TestDrawLanesGroupedRejectsUndersizedField is a denial/edge-path test:
// asking for a group table whose ranks exceed the actual field size fails
// loudly instead of silently assigning garbage lanes.
func TestDrawLanesGroupedRejectsUndersizedField(t *testing.T) {
	rules := mustSeedingRules(t)
	groups, _ := rules.LaneGroupsFor(8)
	shortField := []string{"a", "b", "c"} // fewer than the group table expects
	if _, err := DrawLanesGrouped(shortField, groups, rand.New(rand.NewSource(1))); err == nil {
		t.Fatal("expected an error when the field is smaller than the lane-group table's coverage")
	}
}

// TestChooseHeatCountOddFieldSize is a denial/edge-path test: an odd field
// size still balances heats without exceeding the per-heat cap.
func TestChooseHeatCountOddFieldSize(t *testing.T) {
	n, err := ChooseHeatCount(17, 8)
	if err != nil {
		t.Fatalf("choose heat count: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 heats for 17 at max 8/heat, got %d", n)
	}
}

// TestChooseHeatCountRejectsNonPositiveInputs is a denial/edge-path test.
func TestChooseHeatCountRejectsNonPositiveInputs(t *testing.T) {
	if _, err := ChooseHeatCount(0, 8); err == nil {
		t.Error("expected an error for zero candidates")
	}
	if _, err := ChooseHeatCount(8, 0); err == nil {
		t.Error("expected an error for a non-positive max heat size")
	}
}

// TestRankCandidatesKeepsUnseededAthletesAtEnd is a denial/edge-path test:
// withdrawn/unseeded athletes (no parseable mark) are still scheduled, just
// ranked after every seeded candidate.
func TestRankCandidatesKeepsUnseededAthletesAtEnd(t *testing.T) {
	candidates := []SeedCandidate{
		{EntryID: "unseeded1", Mark: ""},
		{EntryID: "seeded1", Mark: "12.00"},
		{EntryID: "unseeded2", Mark: "DNS"}, // not a parseable mark
		{EntryID: "seeded2", Mark: "11.00"},
	}
	ranked := RankCandidates(candidates)
	if ranked[0].EntryID != "seeded2" || ranked[1].EntryID != "seeded1" {
		t.Fatalf("expected seeded candidates first in mark order, got %v", ranked)
	}
	if ranked[2].EntryID != "unseeded1" || ranked[3].EntryID != "unseeded2" {
		t.Fatalf("expected unseeded candidates last, in original order, got %v", ranked)
	}
}

func TestSeedHeatsRejectsNonPositiveHeatCount(t *testing.T) {
	if _, err := SeedHeats([]SeedCandidate{{EntryID: "a"}}, 0, false); err == nil {
		t.Error("expected an error for a non-positive heat count")
	}
}

func TestDrawLanesByLotRejectsInsufficientLanes(t *testing.T) {
	if _, err := DrawLanesByLot([]string{"a", "b", "c"}, SequentialLanes(2), rand.New(rand.NewSource(1))); err == nil {
		t.Error("expected an error when there are fewer lanes than entries")
	}
}
