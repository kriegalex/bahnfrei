// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"encoding/json"
	"fmt"
	"math"
)

// CombinedScoringKind selects which of the three WA combined-events
// formulas (SYS-044) a column applies: a running/hurdles time (lower is
// better), a vertical jump height or a horizontal jump distance (higher is
// better, both keyed in centimetres), or a throw distance (higher is
// better, keyed in metres — the one family whose unit differs from a plain
// mark's centi-units).
type CombinedScoringKind string

const (
	CombinedScoringTrack CombinedScoringKind = "track"
	CombinedScoringJump  CombinedScoringKind = "jump"
	CombinedScoringThrow CombinedScoringKind = "throw"
)

// CombinedScoringColumn is one (discipline, sex) formula column: the WA
// combined-events points formula is P = floor(A × |mark − B|^C), the sign
// and unit of "mark" set by Kind (SYS-044). ManualAdjustmentSec, when
// non-zero, is added to a hand-timed mark before scoring (WA combined
// events rule: sprints/hurdles ≤400 m add 0.24 s, 400 m adds 0.14 s, to
// approximate FAT reaction-time parity) — zero for every other event.
type CombinedScoringColumn struct {
	DisciplineCode      string              `json:"disciplineCode"`
	Sex                 Sex                 `json:"sex"`
	Kind                CombinedScoringKind `json:"kind"`
	A                   float64             `json:"a"`
	B                   float64             `json:"b"`
	C                   float64             `json:"c"`
	ManualAdjustmentSec float64             `json:"manualAdjustmentSec,omitempty"`
}

// CombinedScoringTable is a versioned, data-defined WA combined-events
// points formula table (SYS-044, ADR-005 §4: rule-shaped data, not code):
// the 2001 IAAF/WA A/B/C coefficients per (discipline, sex) ship as data
// and are interpreted generically here — a revised edition is a data-file
// swap, never a code change.
type CombinedScoringTable struct {
	ID      string                  `json:"id"`
	Version string                  `json:"version"`
	Name    string                  `json:"name"`
	Source  string                  `json:"source"`
	Columns []CombinedScoringColumn `json:"columns"`

	index map[combinedScoringKey]*CombinedScoringColumn
}

type combinedScoringKey struct {
	discipline string
	sex        Sex
}

// ParseCombinedScoringTable decodes and validates a combined-events
// scoring-table data file (the ADR-005 "data interpreter" entry point —
// built-in and operator-supplied files load identically).
func ParseCombinedScoringTable(data []byte) (*CombinedScoringTable, error) {
	var t CombinedScoringTable
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse combined scoring table: %w", err)
	}
	if err := t.validate(); err != nil {
		return nil, fmt.Errorf("parse combined scoring table: %w", err)
	}
	return &t, nil
}

func (t *CombinedScoringTable) validate() error {
	if t.ID == "" {
		return fmt.Errorf("combined scoring table: id is required")
	}
	if t.Version == "" {
		return fmt.Errorf("combined scoring table %q: version is required (must carry a version identity)", t.ID)
	}
	if len(t.Columns) == 0 {
		return fmt.Errorf("combined scoring table %q: at least one column is required", t.ID)
	}
	t.index = make(map[combinedScoringKey]*CombinedScoringColumn, len(t.Columns))
	for i := range t.Columns {
		c := &t.Columns[i]
		if c.DisciplineCode == "" {
			return fmt.Errorf("combined scoring table %q: column %d has no discipline code", t.ID, i)
		}
		switch c.Kind {
		case CombinedScoringTrack, CombinedScoringJump, CombinedScoringThrow:
		default:
			return fmt.Errorf("combined scoring table %q: column %s has invalid kind %q", t.ID, c.DisciplineCode, c.Kind)
		}
		if c.Sex != SexMale && c.Sex != SexFemale {
			return fmt.Errorf("combined scoring table %q: column %s has invalid sex %q", t.ID, c.DisciplineCode, c.Sex)
		}
		if c.A <= 0 || c.C <= 0 {
			return fmt.Errorf("combined scoring table %q: column %s/%s must have positive a and c coefficients", t.ID, c.DisciplineCode, c.Sex)
		}
		if c.ManualAdjustmentSec < 0 {
			return fmt.Errorf("combined scoring table %q: column %s/%s has a negative manual timing adjustment", t.ID, c.DisciplineCode, c.Sex)
		}
		if c.ManualAdjustmentSec != 0 && c.Kind != CombinedScoringTrack {
			return fmt.Errorf("combined scoring table %q: column %s/%s: a manual timing adjustment only applies to track events", t.ID, c.DisciplineCode, c.Sex)
		}
		key := combinedScoringKey{c.DisciplineCode, c.Sex}
		if _, dup := t.index[key]; dup {
			return fmt.Errorf("combined scoring table %q: duplicate column %s/%s", t.ID, c.DisciplineCode, c.Sex)
		}
		t.index[key] = c
	}
	return nil
}

// Column returns the (discipline, sex) formula column, if the table has
// one.
func (t *CombinedScoringTable) Column(disciplineCode string, sex Sex) (*CombinedScoringColumn, bool) {
	c, ok := t.index[combinedScoringKey{disciplineCode, sex}]
	return c, ok
}

// Points scores mark against the (discipline, sex) formula column
// (SYS-044): P = floor(A × (B − T)^C) for a track time T in seconds (manual
// times get ManualAdjustmentSec added first), P = floor(A × (M − B)^C) for
// a jump height/distance M in centimetres, P = floor(A × (D − B)^C) for a
// throw distance D in metres. A mark that does not clear the formula's zero
// crossing (B) scores 0, never negative points — the WA rule's implicit
// floor (a performance worse than the "zero" boundary the formula
// describes simply earns nothing, it is never described in the tables).
func (t *CombinedScoringTable) Points(disciplineCode string, sex Sex, timing Timing, mark string) (int, error) {
	c, ok := t.Column(disciplineCode, sex)
	if !ok {
		return 0, fmt.Errorf("combined scoring table %q: no column for %s/%s", t.ID, disciplineCode, sex)
	}
	centi, err := ParseCentiMark(mark)
	if err != nil {
		return 0, err
	}
	value := float64(centi) / 100 // centi-units -> the formula's native unit (seconds, or metres for throws)

	var diff float64
	switch c.Kind {
	case CombinedScoringTrack:
		if timing == TimingManual {
			value += c.ManualAdjustmentSec
		}
		diff = c.B - value
	case CombinedScoringJump:
		diff = float64(centi) - c.B // jump formulas are keyed in centimetres, not the metres `value` holds
	case CombinedScoringThrow:
		diff = value - c.B
	}
	if diff <= 0 {
		return 0, nil
	}
	return int(math.Floor(c.A * math.Pow(diff, c.C))), nil
}

// AverageWind computes the WA combined-events wind-legality figure
// (D5.3/D5.5, UC-013 #4): "wind legality uses the average across
// wind-relevant disciplines" — the arithmetic mean of the wind readings
// from every wind-relevant discipline in the combined-events programme
// (e.g. 100m/LJ/110mH for the decathlon, 100mH/LJ for the heptathlon; the
// caller selects which readings to include — this function only
// averages). The second return is false for an empty input (no
// wind-relevant reading recorded yet), so callers never divide by zero or
// mistake "no data" for a legal 0.0 m/s average. Flagging a record
// candidate against the resulting figure (SYS-049/050/051) is TASK-022
// scope; this is the shared computation UC-013 #4 names.
func AverageWind(winds []float64) (float64, bool) {
	if len(winds) == 0 {
		return 0, false
	}
	var sum float64
	for _, w := range winds {
		sum += w
	}
	return sum / float64(len(winds)), true
}
