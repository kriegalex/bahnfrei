// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import "sort"

// AdvancementRule is the configured round-progression rule (SYS-029, D2.4):
// the top TopN placers per heat auto-qualify (Q), and the next FastestK
// across every heat, ranked by time from a single timing source, qualify by
// time (q). TopN/FastestK of 0 disables that channel.
type AdvancementRule struct {
	TopN     int
	FastestK int
}

// HeatFinish is one entry's finish in a completed heat, the input to
// ComputeTrackAdvancement: its place (1-based; 0 = did not finish/place),
// mark and any CR 25 status. Only entries with Status == StatusNone and a
// positive Place are eligible for Q; only those with a parseable Mark are
// eligible for q (SYS-029: "single timing source for time-based
// qualification" — callers pass marks from one timing source only).
type HeatFinish struct {
	EntryID string
	Place   int
	Mark    string
	Status  QualificationStatus
}

// Advancement is one entry's computed round-progression outcome.
type Advancement struct {
	EntryID string
	Code    QualificationStatus // StatusQ or StatusQt
}

// TimeTie surfaces a tie at the qualifying-by-time cutoff for operator
// resolution (UC-009 #2, D2.4): the tied mark, the tied entries, and how
// many of them the round's remaining capacity can actually accommodate —
// "both advance if capacity, else draw qD" is an operator call this
// function does not make.
type TimeTie struct {
	Mark           string
	EntryIDs       []string
	RemainingSlots int
}

// ComputeTrackAdvancement computes SYS-029 round progression across a
// round's completed heats: top rule.TopN placers per heat qualify Q, then
// the next rule.FastestK fastest remaining finishers (pooled across every
// heat) qualify q. A tie at the time cutoff is not resolved here — the
// clear-cut q's are returned and the tied group is surfaced separately
// (UC-009 #2).
func ComputeTrackAdvancement(heats [][]HeatFinish, rule AdvancementRule) ([]Advancement, *TimeTie) {
	var advanced []Advancement
	qualified := map[string]bool{}

	if rule.TopN > 0 {
		for _, heat := range heats {
			var finishers []HeatFinish
			for _, f := range heat {
				if f.Status == StatusNone && f.Place > 0 {
					finishers = append(finishers, f)
				}
			}
			sort.SliceStable(finishers, func(i, j int) bool { return finishers[i].Place < finishers[j].Place })
			for i, f := range finishers {
				if i >= rule.TopN {
					break
				}
				advanced = append(advanced, Advancement{EntryID: f.EntryID, Code: StatusQ})
				qualified[f.EntryID] = true
			}
		}
	}

	if rule.FastestK <= 0 {
		return advanced, nil
	}

	type timed struct {
		id    string
		mark  string
		centi int64
	}
	var pool []timed
	for _, heat := range heats {
		for _, f := range heat {
			if qualified[f.EntryID] || f.Status != StatusNone {
				continue
			}
			centi, err := ParseCentiMark(f.Mark)
			if err != nil {
				continue
			}
			pool = append(pool, timed{id: f.EntryID, mark: f.Mark, centi: centi})
		}
	}
	sort.SliceStable(pool, func(i, j int) bool { return pool[i].centi < pool[j].centi })

	if len(pool) <= rule.FastestK {
		for _, t := range pool {
			advanced = append(advanced, Advancement{EntryID: t.id, Code: StatusQt})
		}
		return advanced, nil
	}

	cutoff := pool[rule.FastestK-1].centi
	var secured []timed
	var tiedAtCutoff []timed
	for _, t := range pool {
		switch {
		case t.centi < cutoff:
			secured = append(secured, t)
		case t.centi == cutoff:
			tiedAtCutoff = append(tiedAtCutoff, t)
		}
	}
	for _, t := range secured {
		advanced = append(advanced, Advancement{EntryID: t.id, Code: StatusQt})
	}
	remaining := rule.FastestK - len(secured)
	if len(tiedAtCutoff) <= remaining {
		// Not actually a tie forcing a choice: every tied mark fits.
		for _, t := range tiedAtCutoff {
			advanced = append(advanced, Advancement{EntryID: t.id, Code: StatusQt})
		}
		return advanced, nil
	}
	ids := make([]string, len(tiedAtCutoff))
	for i, t := range tiedAtCutoff {
		ids[i] = t.id
	}
	return advanced, &TimeTie{Mark: tiedAtCutoff[0].mark, EntryIDs: ids, RemainingSlots: remaining}
}

// FieldFinish is one entry's settled field-event mark, the input to
// ComputeFieldAdvancement.
type FieldFinish struct {
	EntryID string
	Mark    string
}

// ComputeFieldAdvancement computes SYS-030 field-event qualification to a
// final round of attempts (D2.4, UC-009 #4): entries meeting or beating
// standard qualify Q (on standard); the remaining entries, ranked by mark,
// fill the rest of capacity by performance (q). standard == "" means no
// qualifying standard is configured — everyone competes for capacity by
// mark alone. better is "higher" (throws/jumps for distance) or "lower"
// (never used for field marks today, kept for symmetry with ScoringColumn).
func ComputeFieldAdvancement(entries []FieldFinish, standard, better string, capacity int) ([]Advancement, *TimeTie) {
	type marked struct {
		id    string
		mark  string
		centi int64
	}
	var pool []marked
	for _, e := range entries {
		centi, err := ParseCentiMark(e.Mark)
		if err != nil {
			continue
		}
		pool = append(pool, marked{id: e.EntryID, mark: e.Mark, centi: centi})
	}
	better1 := better == "higher"
	sort.SliceStable(pool, func(i, j int) bool {
		if better1 {
			return pool[i].centi > pool[j].centi
		}
		return pool[i].centi < pool[j].centi
	})

	var advanced []Advancement
	qualified := map[string]bool{}
	if standard != "" {
		if stdCenti, err := ParseCentiMark(standard); err == nil {
			for _, m := range pool {
				beats := m.centi >= stdCenti
				if !better1 {
					beats = m.centi <= stdCenti
				}
				if beats {
					advanced = append(advanced, Advancement{EntryID: m.id, Code: StatusQ})
					qualified[m.id] = true
				}
			}
		}
	}

	var remainingPool []marked
	for _, m := range pool {
		if !qualified[m.id] {
			remainingPool = append(remainingPool, m)
		}
	}
	remaining := capacity - len(qualified)
	if remaining <= 0 {
		return advanced, nil
	}
	if len(remainingPool) <= remaining {
		for _, m := range remainingPool {
			advanced = append(advanced, Advancement{EntryID: m.id, Code: StatusQt})
		}
		return advanced, nil
	}
	cutoff := remainingPool[remaining-1].centi
	var secured, tied []marked
	for _, m := range remainingPool {
		switch {
		case m.centi == cutoff:
			tied = append(tied, m)
		case (better1 && m.centi > cutoff) || (!better1 && m.centi < cutoff):
			secured = append(secured, m) // strictly better than the cutoff mark
		}
		// strictly worse than cutoff: excluded from both secured and tied.
	}
	for _, m := range secured {
		advanced = append(advanced, Advancement{EntryID: m.id, Code: StatusQt})
	}
	slotsLeft := remaining - len(secured)
	if len(tied) <= slotsLeft {
		for _, m := range tied {
			advanced = append(advanced, Advancement{EntryID: m.id, Code: StatusQt})
		}
		return advanced, nil
	}
	ids := make([]string, len(tied))
	for i, m := range tied {
		ids[i] = m.id
	}
	return advanced, &TimeTie{Mark: tied[0].mark, EntryIDs: ids, RemainingSlots: slotsLeft}
}
