// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Timing distinguishes the scoring column for hand-timed vs electronically
// timed track marks: the UBS Kids Cup Reglement (§3 Sprint) prescribes
// separate points tables per timing method. Field disciplines use
// TimingNone.
type Timing string

const (
	TimingNone       Timing = ""
	TimingManual     Timing = "manual"
	TimingElectronic Timing = "electronic"
)

// RoundingNextLowerPoints is the only scoring-table rounding rule this
// interpreter implements: a result falling between two listed marks earns
// the next-lower points row (UBS Kids Cup Reglement §3 Punktewertung:
// "Bei einem zwischen zwei Punkten liegenden Resultat muss die
// nächsttiefere Punktzahl berücksichtigt werden"). Data files must name
// their rule so a future table with different semantics fails loudly
// instead of being silently misscored.
const RoundingNextLowerPoints = "next-lower-points"

// ScoringMark is one row of a scoring column: the points awarded for
// achieving Mark (and, per the rounding rule, any result better than Mark
// but short of the next row).
type ScoringMark struct {
	Points int
	Mark   string // decimal string exactly as published (e.g. "6.48")
	centi  int64  // Mark in centi-units (centiseconds / centimetres)
}

// UnmarshalJSON decodes the compact [points, "mark"] pair used by the data
// files.
func (m *ScoringMark) UnmarshalJSON(data []byte) error {
	var pair [2]json.RawMessage
	if err := json.Unmarshal(data, &pair); err != nil {
		return fmt.Errorf("scoring mark must be a [points, mark] pair: %w", err)
	}
	if err := json.Unmarshal(pair[0], &m.Points); err != nil {
		return fmt.Errorf("scoring mark points: %w", err)
	}
	if err := json.Unmarshal(pair[1], &m.Mark); err != nil {
		return fmt.Errorf("scoring mark value: %w", err)
	}
	return nil
}

// ScoringColumn is one (discipline, timing, sex) column of a scoring table:
// the ordered mark→points thresholds for that combination.
type ScoringColumn struct {
	DisciplineCode  string        `json:"disciplineCode"`
	Timing          Timing        `json:"timing"`
	Sex             Sex           `json:"sex"`
	BetterDirection string        `json:"betterDirection"` // "lower" (times) or "higher" (distances)
	Marks           []ScoringMark `json:"marks"`           // points descending
}

// ScoringTable is a versioned, data-defined points table (SYS-053 /
// CON-01: scoring tables are data, not code). It is interpreted generically
// — a revised series table is a data-file swap, never a code change
// (UC-033 #5).
type ScoringTable struct {
	ID       string          `json:"id"`
	Version  string          `json:"version"`
	Name     string          `json:"name"`
	Source   string          `json:"source"`
	Rounding string          `json:"rounding"`
	Columns  []ScoringColumn `json:"columns"`

	index map[scoringKey]*ScoringColumn
}

type scoringKey struct {
	discipline string
	timing     Timing
	sex        Sex
}

// ParseScoringTable decodes and validates a scoring-table data file (the
// ADR-005 "data interpreter" entry point — built-in and operator-supplied
// files load identically).
func ParseScoringTable(data []byte) (*ScoringTable, error) {
	var t ScoringTable
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse scoring table: %w", err)
	}
	if err := t.validate(); err != nil {
		return nil, fmt.Errorf("parse scoring table: %w", err)
	}
	return &t, nil
}

func (t *ScoringTable) validate() error {
	if t.ID == "" {
		return fmt.Errorf("scoring table: id is required")
	}
	if t.Version == "" {
		return fmt.Errorf("scoring table %q: version is required (must carry a version identity)", t.ID)
	}
	if t.Rounding != RoundingNextLowerPoints {
		return fmt.Errorf("scoring table %q: unsupported rounding rule %q (interpreter implements %q)", t.ID, t.Rounding, RoundingNextLowerPoints)
	}
	if len(t.Columns) == 0 {
		return fmt.Errorf("scoring table %q: at least one column is required", t.ID)
	}
	t.index = make(map[scoringKey]*ScoringColumn, len(t.Columns))
	for i := range t.Columns {
		c := &t.Columns[i]
		key := scoringKey{c.DisciplineCode, c.Timing, c.Sex}
		if c.DisciplineCode == "" {
			return fmt.Errorf("scoring table %q: column %d has no discipline code", t.ID, i)
		}
		if c.BetterDirection != "lower" && c.BetterDirection != "higher" {
			return fmt.Errorf("scoring table %q: column %s has invalid betterDirection %q", t.ID, c.DisciplineCode, c.BetterDirection)
		}
		if _, dup := t.index[key]; dup {
			return fmt.Errorf("scoring table %q: duplicate column %s/%s/%s", t.ID, c.DisciplineCode, c.Timing, c.Sex)
		}
		if len(c.Marks) == 0 {
			return fmt.Errorf("scoring table %q: column %s/%s/%s has no marks", t.ID, c.DisciplineCode, c.Timing, c.Sex)
		}
		for j := range c.Marks {
			m := &c.Marks[j]
			centi, err := ParseCentiMark(m.Mark)
			if err != nil {
				return fmt.Errorf("scoring table %q: column %s/%s/%s row %d: %w", t.ID, c.DisciplineCode, c.Timing, c.Sex, j, err)
			}
			m.centi = centi
			if j == 0 {
				continue
			}
			prev := c.Marks[j-1]
			marksWorsen := m.centi > prev.centi // times grow as points fall
			if c.BetterDirection == "higher" {
				marksWorsen = m.centi < prev.centi // distances shrink as points fall
			}
			if m.Points >= prev.Points || !marksWorsen {
				return fmt.Errorf("scoring table %q: column %s/%s/%s not monotonic at row %d (%d:%s after %d:%s)",
					t.ID, c.DisciplineCode, c.Timing, c.Sex, j, m.Points, m.Mark, prev.Points, prev.Mark)
			}
		}
		t.index[key] = c
	}
	return nil
}

// Column returns the (discipline, timing, sex) column, if the table has one.
func (t *ScoringTable) Column(disciplineCode string, timing Timing, sex Sex) (*ScoringColumn, bool) {
	c, ok := t.index[scoringKey{disciplineCode, timing, sex}]
	return c, ok
}

// Points awards the points for a mark per the table's rounding rule: the
// highest points row whose listed mark the result equals or beats. A result
// worse than the last row scores 0 points; a result better than the first
// row earns that row's points (the table's published ceiling). Marks are
// decimal strings at the discipline's measurement resolution ("8.42",
// "38.5"); a comma decimal separator is accepted (Swiss convention).
func (t *ScoringTable) Points(disciplineCode string, timing Timing, sex Sex, mark string) (int, error) {
	c, ok := t.Column(disciplineCode, timing, sex)
	if !ok {
		return 0, fmt.Errorf("scoring table %q: no column for %s/%s/%s", t.ID, disciplineCode, timing, sex)
	}
	centi, err := ParseCentiMark(mark)
	if err != nil {
		return 0, err
	}
	// Marks are ordered points-descending, i.e. from best to worst
	// performance. Find the first row the result equals or beats.
	i := sort.Search(len(c.Marks), func(i int) bool {
		if c.BetterDirection == "lower" {
			return c.Marks[i].centi >= centi
		}
		return c.Marks[i].centi <= centi
	})
	if i == len(c.Marks) {
		return 0, nil
	}
	return c.Marks[i].Points, nil
}

// ParseCentiMark parses a non-negative decimal mark string with at most two
// decimal places into centi-units (centiseconds for times, centimetres for
// distances). Both "." and "," are accepted as the decimal separator —
// Swiss result lists print comma decimals (research C7.3).
func ParseCentiMark(mark string) (int64, error) {
	s := strings.ReplaceAll(strings.TrimSpace(mark), ",", ".")
	if s == "" {
		return 0, fmt.Errorf("empty mark")
	}
	whole, frac, hasFrac := strings.Cut(s, ".")
	if hasFrac && (len(frac) == 0 || len(frac) > 2) {
		return 0, fmt.Errorf("mark %q: expected at most two decimal places", mark)
	}
	for len(frac) < 2 {
		frac += "0"
	}
	var out int64
	for _, part := range []string{whole, frac} {
		for _, r := range part {
			if r < '0' || r > '9' {
				return 0, fmt.Errorf("mark %q: not a non-negative decimal number", mark)
			}
			out = out*10 + int64(r-'0')
			if out > 1<<40 {
				return 0, fmt.Errorf("mark %q: out of range", mark)
			}
		}
	}
	return out, nil
}
