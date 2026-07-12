// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"encoding/json"
	"fmt"
)

// DistributionSerpentine is the only heat-distribution method this
// interpreter implements: ranked candidates are dealt to heats in a
// boustrophedon ("snake") order (SYS-026, D2.2). A rules file naming a
// different method fails loudly rather than being silently misinterpreted
// (mirrors ScoringTable's RoundingNextLowerPoints precedent).
const DistributionSerpentine = "serpentine"

// LaneGroup is one TR20.4 rank-group → lane-set mapping (D2.3): the
// athletes/teams ranked RankFrom..RankTo (1-based, inclusive) draw randomly
// among Lanes.
type LaneGroup struct {
	RankFrom int   `json:"rankFrom"`
	RankTo   int   `json:"rankTo"`
	Lanes    []int `json:"lanes"`
}

// SeedingRules is a versioned, data-defined heat-seeding and lane-draw rule
// set (SYS-026/027, D2.2-D2.4): the distribution method, whether same-club
// athletes are separated where mathematically possible, which disciplines
// race fully in lanes (and therefore use the grouped TR20.4 draw rather than
// a by-lot draw), and the TR20.4 rank-group → lane-set table per lane count.
// Interpreted generically by internal/domain/heatseeding.go — a revised rule
// (e.g. a 6-lane group table) is a data-file swap, never a code change
// (ADR-005 §4).
type SeedingRules struct {
	ID                  string                 `json:"id"`
	Version             string                 `json:"version"`
	Name                string                 `json:"name"`
	Source              string                 `json:"source"`
	Distribution        string                 `json:"distribution"`
	ClubSeparation      bool                   `json:"clubSeparation"`
	LaneRaceDisciplines []string               `json:"laneRaceDisciplines"`
	LaneGroups          map[string][]LaneGroup `json:"laneGroups"` // keyed by lane count, as a string (JSON object keys)

	laneRaceSet map[string]bool
}

// ParseSeedingRules decodes and validates a seeding-rules data file (the
// ADR-005 "data interpreter" entry point — built-in and operator-supplied
// files load identically).
func ParseSeedingRules(data []byte) (*SeedingRules, error) {
	var r SeedingRules
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse seeding rules: %w", err)
	}
	if err := r.validate(); err != nil {
		return nil, fmt.Errorf("parse seeding rules: %w", err)
	}
	return &r, nil
}

func (r *SeedingRules) validate() error {
	if r.ID == "" {
		return fmt.Errorf("seeding rules: id is required")
	}
	if r.Version == "" {
		return fmt.Errorf("seeding rules %q: version is required (must carry a version identity)", r.ID)
	}
	if r.Distribution != DistributionSerpentine {
		return fmt.Errorf("seeding rules %q: unsupported distribution method %q (interpreter implements %q)", r.ID, r.Distribution, DistributionSerpentine)
	}
	for laneCount, groups := range r.LaneGroups {
		if len(groups) == 0 {
			return fmt.Errorf("seeding rules %q: lane group table for %q lanes is empty", r.ID, laneCount)
		}
		seen := map[int]bool{}
		for _, g := range groups {
			if g.RankFrom <= 0 || g.RankTo < g.RankFrom {
				return fmt.Errorf("seeding rules %q: lane group %d-%d for %q lanes has an invalid rank range", r.ID, g.RankFrom, g.RankTo, laneCount)
			}
			if len(g.Lanes) != g.RankTo-g.RankFrom+1 {
				return fmt.Errorf("seeding rules %q: lane group %d-%d for %q lanes has %d lanes for %d ranks", r.ID, g.RankFrom, g.RankTo, laneCount, len(g.Lanes), g.RankTo-g.RankFrom+1)
			}
			for _, lane := range g.Lanes {
				if lane <= 0 {
					return fmt.Errorf("seeding rules %q: lane group for %q lanes has a non-positive lane number", r.ID, laneCount)
				}
				if seen[lane] {
					return fmt.Errorf("seeding rules %q: lane %d used by more than one group for %q lanes", r.ID, lane, laneCount)
				}
				seen[lane] = true
			}
		}
	}
	r.laneRaceSet = make(map[string]bool, len(r.LaneRaceDisciplines))
	for _, code := range r.LaneRaceDisciplines {
		r.laneRaceSet[code] = true
	}
	return nil
}

// IsLaneRace reports whether disciplineCode races fully in lanes for its
// entire distance (D2.3: 400m/800m-class races use the grouped TR20.4 draw);
// anything not listed uses the by-lot draw (D2.3's "events longer than
// 800m... drawn by lot").
func (r *SeedingRules) IsLaneRace(disciplineCode string) bool {
	return r.laneRaceSet[disciplineCode]
}

// LaneGroupsFor returns the TR20.4 rank-group → lane-set table for a race
// with exactly laneCount lanes in use, if the rules define one.
func (r *SeedingRules) LaneGroupsFor(laneCount int) ([]LaneGroup, bool) {
	groups, ok := r.LaneGroups[fmt.Sprintf("%d", laneCount)]
	return groups, ok
}
