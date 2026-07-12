// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import "fmt"

// EvaluateEntryStandard reports whether seedPerformance fails an event's
// configured entry standard (SYS-015, UC-003 #5): for track/relay
// disciplines a lower mark is better, so the seed fails when it exceeds the
// standard (e.g. 100 m: standard "12.20", seed "12.85" fails); for
// field/combined disciplines a higher mark is better, so the seed fails when
// it falls short of the standard. An empty standard means no condition is
// configured — never fails. A seed performance is required to evaluate a
// configured standard (SYS-015 "required seed-performance information").
func EvaluateEntryStandard(seedPerformance, standard string, family DisciplineFamily) (bool, error) {
	if standard == "" {
		return false, nil
	}
	if seedPerformance == "" {
		return false, fmt.Errorf("entry: seed performance is required to evaluate the entry standard (SYS-015)")
	}
	seed, err := ParseCentiMark(seedPerformance)
	if err != nil {
		return false, fmt.Errorf("entry: invalid seed performance %q: %w", seedPerformance, err)
	}
	std, err := ParseCentiMark(standard)
	if err != nil {
		return false, fmt.Errorf("entry: invalid entry standard %q: %w", standard, err)
	}
	if lowerMarkIsBetter(family) {
		return seed > std, nil
	}
	return seed < std, nil
}

// lowerMarkIsBetter reports whether a numerically lower mark is the better
// performance for family — true for time-based disciplines (track, relay),
// false for distance/height/points disciplines (field, combined).
func lowerMarkIsBetter(family DisciplineFamily) bool {
	switch family {
	case FamilyTrack, FamilyRelay:
		return true
	default:
		return false
	}
}
