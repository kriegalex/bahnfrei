// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"errors"
	"fmt"
	"sort"
)

// AttemptKind is what happened on one trial of a horizontal field event
// (SYS-042): a measured mark, a foul (X), a pass (–), or a retirement (r).
// The D5.2 result-list symbols are presentation (Attempt.Display); the kind
// is the stored fact.
type AttemptKind string

const (
	AttemptValid  AttemptKind = "valid"
	AttemptFoul   AttemptKind = "foul"
	AttemptPass   AttemptKind = "pass"
	AttemptRetire AttemptKind = "retire"
)

// Attempt is one trial in a horizontal field event series: mark in metres at
// 0.01 m resolution for valid attempts, per-attempt wind where the
// discipline is wind-relevant (SYS-042).
type Attempt struct {
	Seq  int // 1-based trial number
	Kind AttemptKind
	Mark string   // canonical decimal metres, only for AttemptValid
	Wind *float64 // m/s, only where wind-relevant
}

// Validate checks one attempt against the SYS-042 capture rules and
// normalizes a valid mark to its canonical two-decimal form ("6.1" → "6.10").
func (a *Attempt) Validate(windRelevant bool) error {
	if a.Seq < 1 {
		return errors.New("attempt: trial number must be ≥ 1")
	}
	switch a.Kind {
	case AttemptValid:
		centi, err := ParseCentiMark(a.Mark)
		if err != nil {
			return fmt.Errorf("attempt: %w", err)
		}
		if centi == 0 {
			return errors.New("attempt: a valid attempt needs a mark > 0")
		}
		a.Mark = FormatCentiMark(centi, 2)
	case AttemptFoul, AttemptPass, AttemptRetire:
		if a.Mark != "" {
			return fmt.Errorf("attempt: a %s carries no mark", a.Kind)
		}
	default:
		return fmt.Errorf("attempt: unknown kind %q", a.Kind)
	}
	if a.Wind != nil && !windRelevant {
		return errors.New("attempt: discipline is not wind-relevant, no wind reading expected")
	}
	return nil
}

// Display renders the attempt cell per the D5.2 result-list convention.
func (a Attempt) Display() string {
	switch a.Kind {
	case AttemptValid:
		return a.Mark
	case AttemptFoul:
		return string(StatusX)
	case AttemptPass:
		return string(StatusPass)
	case AttemptRetire:
		return string(StatusR)
	}
	return ""
}

// FieldSeries is one athlete's attempt sequence in a horizontal field event
// unit, seq-ordered.
type FieldSeries struct {
	AthleteID string
	Attempts  []Attempt
}

// Retired reports whether the athlete has retired from the competition (r).
func (s FieldSeries) Retired() bool {
	for _, a := range s.Attempts {
		if a.Kind == AttemptRetire {
			return true
		}
	}
	return false
}

// marksDesc returns the series' valid marks in centi-metres, best first —
// the comparison vector the D5.6 tie-break walks.
func (s FieldSeries) marksDesc() []int64 {
	var out []int64
	for _, a := range s.Attempts {
		if a.Kind != AttemptValid {
			continue
		}
		if centi, err := ParseCentiMark(a.Mark); err == nil {
			out = append(out, centi)
		}
	}
	sort.Sort(sort.Reverse(int64Slice(out)))
	return out
}

// Best returns the best valid mark (canonical form) and whether one exists.
func (s FieldSeries) Best() (string, bool) {
	marks := s.marksDesc()
	if len(marks) == 0 {
		return "", false
	}
	return FormatCentiMark(marks[0], 2), true
}

// Status derives the series' D5.2 result status: r once the athlete retired
// (their best mark still ranks, UC-011 #3), NM when trials were taken but no
// valid mark exists, none while the series is open or has a mark.
func (s FieldSeries) Status() QualificationStatus {
	if s.Retired() {
		return StatusR
	}
	if _, ok := s.Best(); !ok && len(s.Attempts) > 0 {
		return StatusNM
	}
	return StatusNone
}

// FieldStanding is one athlete's line in a horizontal field event ranking.
type FieldStanding struct {
	AthleteID string
	Best      string // canonical best mark, "" when none
	Status    QualificationStatus
	Rank      int // 0 for unrankable rows (no valid mark)
}

// RankFieldSeries ranks a horizontal field event by best mark with
// next-best-mark tie-breaking (SYS-042, D5.6): equal bests are separated by
// the better second-best mark and so on through the series; athletes whose
// full mark series coincide rank equal (UC-011 #2). A retiree's best mark
// still ranks (UC-011 #3). Athletes without a valid mark are listed after
// all ranked athletes, unranked, in athlete-ID order for determinism.
func RankFieldSeries(series []FieldSeries) []FieldStanding {
	type row struct {
		standing FieldStanding
		marks    []int64
	}
	rows := make([]row, len(series))
	for i, s := range series {
		best, _ := s.Best()
		rows[i] = row{
			standing: FieldStanding{AthleteID: s.AthleteID, Best: best, Status: s.Status()},
			marks:    s.marksDesc(),
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if c := compareMarks(rows[i].marks, rows[j].marks); c != 0 {
			return c > 0
		}
		return rows[i].standing.AthleteID < rows[j].standing.AthleteID
	})
	out := make([]FieldStanding, len(rows))
	for i, r := range rows {
		out[i] = r.standing
		if len(r.marks) == 0 {
			continue // unrankable: keep Rank 0
		}
		if i > 0 && compareMarks(rows[i-1].marks, r.marks) == 0 {
			out[i].Rank = out[i-1].Rank
			continue
		}
		out[i].Rank = i + 1
	}
	return out
}

// compareMarks compares two best-first mark vectors: >0 when a ranks ahead,
// 0 for an official tie (identical series). A missing next-best loses to
// any existing mark (an athlete with one 6.42 ranks behind one with 6.42
// and a 6.10 — the next-best rule, D5.6).
func compareMarks(a, b []int64) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		ma, mb := int64(-1), int64(-1)
		if i < len(a) {
			ma = a[i]
		}
		if i < len(b) {
			mb = b[i]
		}
		if ma != mb {
			return int(ma - mb)
		}
	}
	return 0
}

// FieldContinuation applies the round-3 field cut (SYS-042): given every
// athlete's series after cutAfter completed rounds, exactly the top topN by
// the current ranking continue, returned in rule-correct competing order for
// the remaining trials — reverse of the ranking, best last (WA TR 25.6.4;
// UC-011 #1). Ties for the last qualifying place that survive the full
// series tie-break all advance. A retiree does not continue even when their
// mark ranks in the top topN.
func FieldContinuation(series []FieldSeries, topN int) []string {
	ranked := RankFieldSeries(series)
	retired := make(map[string]bool, len(series))
	for _, s := range series {
		retired[s.AthleteID] = s.Retired()
	}
	var qualified []FieldStanding
	for _, st := range ranked {
		if st.Rank == 0 || retired[st.AthleteID] {
			continue
		}
		// st.Rank counts ties once (1,2,2,4): everyone ranked within topN
		// advances, including all athletes tied at the boundary.
		if st.Rank <= topN {
			qualified = append(qualified, st)
		}
	}
	out := make([]string, len(qualified))
	for i, st := range qualified {
		out[len(qualified)-1-i] = st.AthleteID
	}
	return out
}

// int64Slice implements sort.Interface (sort.Slice would re-allocate the
// closure per call on this hot ranking path).
type int64Slice []int64

func (s int64Slice) Len() int           { return len(s) }
func (s int64Slice) Less(i, j int) bool { return s[i] < s[j] }
func (s int64Slice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// FormatCentiMark renders centi-units as a decimal string with the given
// number of decimals (2 for distances/FAT times, 1 for hand times).
func FormatCentiMark(centi int64, decimals int) string {
	switch decimals {
	case 1:
		return fmt.Sprintf("%d.%d", centi/100, (centi%100)/10)
	default:
		return fmt.Sprintf("%d.%02d", centi/100, centi%100)
	}
}
