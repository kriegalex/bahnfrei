// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"encoding/json"
	"fmt"
)

// RecordList is a versioned, data-defined set of RecordReference rows
// (SYS-049, D6.1) — "loadable national/area/world reference lists" plus an
// organizer's own meeting-record list, all interpreted through this one
// generic parser (ADR-005 §4: rule-shaped data is data, not code). A meet
// is wired to zero or more lists (internal/store's meet_record_lists,
// mirroring 0014_combined_scoring.sql's per-meet side-table precedent) —
// zero means no reference-record flagging beyond PB/SB history for that
// meet.
type RecordList struct {
	ID      string            `json:"id"`
	Version string            `json:"version"`
	Name    string            `json:"name"`
	Source  string            `json:"source"`
	Notes   string            `json:"notes"`
	Records []RecordReference `json:"records"`
}

// ParseRecordList decodes and validates a record-list data file (the
// ADR-005 "data interpreter" entry point: built-in and organizer-supplied
// files load identically).
func ParseRecordList(data []byte) (*RecordList, error) {
	var l RecordList
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("parse record list: %w", err)
	}
	if err := l.Validate(); err != nil {
		return nil, fmt.Errorf("parse record list: %w", err)
	}
	return &l, nil
}

// Validate checks the list's structural invariants: identity, version, and
// that every row carries the fields a comparison needs.
func (l *RecordList) Validate() error {
	if l.ID == "" {
		return fmt.Errorf("record list: id is required")
	}
	if l.Version == "" {
		return fmt.Errorf("record list %q: version is required (must carry a version identity)", l.ID)
	}
	seen := make(map[string]bool, len(l.Records))
	for _, r := range l.Records {
		if r.ID == "" {
			return fmt.Errorf("record list %q: record with empty id", l.ID)
		}
		if seen[r.ID] {
			return fmt.Errorf("record list %q: duplicate record id %q", l.ID, r.ID)
		}
		seen[r.ID] = true
		switch r.Type {
		case RecordWorld, RecordArea, RecordNational, RecordMeeting:
		default:
			return fmt.Errorf("record list %q: record %q has unknown type %q", l.ID, r.ID, r.Type)
		}
		if r.CategoryCode == "" {
			return fmt.Errorf("record list %q: record %q has no category code", l.ID, r.ID)
		}
		if r.DisciplineCode == "" {
			return fmt.Errorf("record list %q: record %q has no discipline code", l.ID, r.ID)
		}
		if r.Mark == "" {
			return fmt.Errorf("record list %q: record %q has no mark", l.ID, r.ID)
		}
		if _, err := ParseCentiMark(r.Mark); err != nil {
			return fmt.Errorf("record list %q: record %q: %w", l.ID, r.ID, err)
		}
		if r.Type == RecordMeeting && r.MeetID == "" {
			return fmt.Errorf("record list %q: meeting record %q needs a meet id", l.ID, r.ID)
		}
	}
	return nil
}

// --- SYS-050 wind legality & SYS-049/051 record-timing eligibility ---

// WindLegalLimit is the D5.3 wind-legality threshold: an average tailwind
// strictly above this is wind-assisted.
const WindLegalLimit = 2.0

// WindAssisted reports whether reading exceeds the D5.3 tailwind limit
// (SYS-050); a nil reading (not wind-relevant, or not yet recorded) is
// never wind-assisted.
func WindAssisted(reading *float64) bool {
	return reading != nil && *reading > WindLegalLimit
}

// RecordRequiresFAT reports whether a record claim for disciplineCode
// requires fully-automatic timing (D5.1/D6.2: stadium races at or under
// 800 m). Only track disciplines with a parseable leading distance are
// covered — relays and hurdles/steeplechase codes without a bare leading
// distance are out of this MVP's scope (see docs open questions).
func RecordRequiresFAT(family DisciplineFamily, disciplineCode string) bool {
	if family != FamilyTrack {
		return false
	}
	dist, ok := TrackDistanceMeters(disciplineCode)
	return ok && dist <= 800
}

// MarkAtLeastAsGood reports whether mark equals or betters reference for
// family (SYS-049 "equal/better"): lower-is-better for track/relay,
// higher-is-better otherwise.
func MarkAtLeastAsGood(family DisciplineFamily, mark, reference string) (bool, error) {
	a, err := ParseCentiMark(mark)
	if err != nil {
		return false, fmt.Errorf("mark: %w", err)
	}
	b, err := ParseCentiMark(reference)
	if err != nil {
		return false, fmt.Errorf("reference mark: %w", err)
	}
	if lowerMarkIsBetter(family) {
		return a <= b, nil
	}
	return a >= b, nil
}

// Record-evaluation reason codes (UC-016 #3: "the reason is inspectable") —
// stable machine strings, not user-facing text; the web layer maps them to
// localized copy.
const (
	ReasonDoesNotBetter = "does_not_better_reference"
	ReasonWindAssisted  = "wind_assisted"
	ReasonRequiresFAT   = "requires_fat_timing"
	FlagPersonalBest    = "PB"
	FlagSeasonBest      = "SB"
)

// RecordCandidateOutcome is one reference or best this result was evaluated
// against: whether it flagged, and — when it did not — the inspectable
// reason (UC-016 #3/#4).
type RecordCandidateOutcome struct {
	FlagCode      string // "WR"/"AR"/"NR"/"MR"/"PB"/"SB"
	ReferenceMark string // the mark compared against, for display
	Flagged       bool
	Reason        string // empty when Flagged
}

// RecordEvaluation is the full outcome of evaluating one captured result
// against every applicable reference and personal/season best (SYS-049).
type RecordEvaluation struct {
	Outcomes []RecordCandidateOutcome
}

// Flags returns the flag codes that ended up set, in evaluation order —
// what Result.RecordFlags is persisted as.
func (e RecordEvaluation) Flags() []string {
	var out []string
	for _, o := range e.Outcomes {
		if o.Flagged {
			out = append(out, o.FlagCode)
		}
	}
	return out
}

// RecordEvaluationInput is everything EvaluateRecord needs to decide which
// record/best flags a just-captured result earns (SYS-049/050).
type RecordEvaluationInput struct {
	Family         DisciplineFamily
	DisciplineCode string
	Mark           string // the new result's mark; empty (status-only result) never flags
	Timing         Timing
	Wind           *float64
	WindRelevant   bool
	// References are the WR/AR/NR/meeting reference rows already filtered
	// to this discipline+category (and, for meeting rows, this meet) by the
	// caller (internal/app/record.go).
	References []RecordReference
	// HasPriorBest/PriorBest and HasPriorSeasonBest/PriorSeasonBest are the
	// athlete's best-ever and best-this-season marks from in-system history,
	// before this result (SYS-049 "where the athlete's history exists in
	// the system") — false means no qualifying history exists, so no PB/SB
	// candidate is evaluated at all.
	HasPriorBest       bool
	PriorBest          string
	HasPriorSeasonBest bool
	PriorSeasonBest    string
}

// EvaluateRecord decides which record/best flags in.Mark earns (SYS-049,
// UC-016): a pure function over the caller-resolved references and history,
// so both the capture-save flagging path and an on-demand "why not"
// inspection (UC-016 #3) always agree. Wind-assisted marks (SYS-050) and,
// for reference records requiring FAT (D6.2), a non-electronic mark, never
// flag — the FAT requirement applies only to WR/AR/NR/MR reference claims,
// not to PB/SB (D6.2 governs record documentation, not personal-best
// tracking).
func EvaluateRecord(in RecordEvaluationInput) RecordEvaluation {
	var eval RecordEvaluation
	if in.Mark == "" {
		return eval
	}
	windAssisted := in.WindRelevant && WindAssisted(in.Wind)
	fatMissing := RecordRequiresFAT(in.Family, in.DisciplineCode) && in.Timing != TimingElectronic

	for _, ref := range in.References {
		outcome := RecordCandidateOutcome{FlagCode: ref.Type.FlagCode(), ReferenceMark: ref.Mark}
		better, err := MarkAtLeastAsGood(in.Family, in.Mark, ref.Mark)
		switch {
		case err != nil || !better:
			outcome.Reason = ReasonDoesNotBetter
		case windAssisted:
			outcome.Reason = ReasonWindAssisted
		case fatMissing:
			outcome.Reason = ReasonRequiresFAT
		default:
			outcome.Flagged = true
		}
		eval.Outcomes = append(eval.Outcomes, outcome)
	}

	if in.HasPriorBest {
		eval.Outcomes = append(eval.Outcomes, bestOutcome(FlagPersonalBest, in.Family, in.Mark, in.PriorBest, windAssisted))
	}
	if in.HasPriorSeasonBest {
		eval.Outcomes = append(eval.Outcomes, bestOutcome(FlagSeasonBest, in.Family, in.Mark, in.PriorSeasonBest, windAssisted))
	}
	return eval
}

// bestOutcome evaluates one PB/SB candidate: no FAT requirement (D6.2 is a
// record-claim rule, not a personal-best rule), but still wind-gated
// (SYS-050 applies to "record/best flagging" generally).
func bestOutcome(flagCode string, family DisciplineFamily, mark, prior string, windAssisted bool) RecordCandidateOutcome {
	outcome := RecordCandidateOutcome{FlagCode: flagCode, ReferenceMark: prior}
	better, err := MarkAtLeastAsGood(family, mark, prior)
	switch {
	case err != nil || !better:
		outcome.Reason = ReasonDoesNotBetter
	case windAssisted:
		outcome.Reason = ReasonWindAssisted
	default:
		outcome.Flagged = true
	}
	return outcome
}

// --- PB/SB history reduction ---

// MarkHistory is one prior result of an athlete in a discipline, the input
// BestMarks reduces to a prior best/season-best (internal/store resolves
// these across every meet the athlete has results in — athletes are
// instance-global, SYS-010).
type MarkHistory struct {
	Mark         string
	Wind         *float64
	WindRelevant bool
	Year         int // the meet's competition year this mark was set in
}

// BestMarks reduces an athlete's discipline history to their all-time best
// and best-in-season marks (season = the calendar year the new result is
// being captured in), skipping wind-assisted marks (SYS-050: they never
// count as a best) and marks this package cannot parse (defensive; capture
// always stores canonical marks). hasBest/hasSeasonBest are false when no
// qualifying mark exists — EvaluateRecord then evaluates no PB/SB candidate
// at all, matching SYS-049's "where the athlete's history exists".
func BestMarks(family DisciplineFamily, history []MarkHistory, season int) (best string, hasBest bool, seasonBest string, hasSeasonBest bool) {
	var bestCenti, seasonCenti int64
	for _, h := range history {
		if h.WindRelevant && WindAssisted(h.Wind) {
			continue
		}
		centi, err := ParseCentiMark(h.Mark)
		if err != nil {
			continue
		}
		if !hasBest || centiBetter(family, centi, bestCenti) {
			bestCenti, hasBest, best = centi, true, h.Mark
		}
		if h.Year == season && (!hasSeasonBest || centiBetter(family, centi, seasonCenti)) {
			seasonCenti, hasSeasonBest, seasonBest = centi, true, h.Mark
		}
	}
	return best, hasBest, seasonBest, hasSeasonBest
}

func centiBetter(family DisciplineFamily, a, b int64) bool {
	if lowerMarkIsBetter(family) {
		return a < b
	}
	return a > b
}

// --- SYS-051 record-documentation checklist (Swiss Rekordprotokoll, D6.2) ---

// Record-checklist item keys (i18n-mapped by the web layer).
const (
	ChecklistTimingClass      = "timingClass"
	ChecklistZeroTest         = "zeroTest"
	ChecklistWind             = "wind"
	ChecklistPhotoFinishImage = "photoFinishImage"
	ChecklistCompetitors      = "competitors"
)

// RecordChecklistItem is one Rekordprotokoll field: the in-system value
// when available, or an explicit gap for the operator to complete by hand
// (UC-016 #4: "gaps explicitly marked for manual completion").
type RecordChecklistItem struct {
	Key   string
	Value string
	Gap   bool
}

// BuildRecordChecklist assembles the SYS-051 record-documentation checklist
// for one flagged result: the fields already available in-system (timing
// homologation class, wind reading, competitor count) plus the fields this
// system never captures (the zero-test attestation, a photo-finish image
// reference) as explicit gaps — never silently omitted, since a missing
// Rekordprotokoll field is what risks a lost 30-day WA ratification window.
func BuildRecordChecklist(timing Timing, windRelevant bool, wind *float64, competitors int) []RecordChecklistItem {
	items := []RecordChecklistItem{
		// TimingNone is a legitimate value here (a field mark is measured,
		// never timed) — "n/a", not a gap.
		{Key: ChecklistTimingClass, Value: timingClassLabel(timing)},
		{Key: ChecklistZeroTest, Gap: true},
	}
	if windRelevant {
		if wind != nil {
			items = append(items, RecordChecklistItem{Key: ChecklistWind, Value: FormatWind(*wind)})
		} else {
			items = append(items, RecordChecklistItem{Key: ChecklistWind, Gap: true})
		}
	}
	items = append(items,
		RecordChecklistItem{Key: ChecklistPhotoFinishImage, Gap: true},
		RecordChecklistItem{Key: ChecklistCompetitors, Value: fmt.Sprintf("%d", competitors)},
	)
	return items
}

// timingClassLabel renders a result's timing method as the Rekordprotokoll
// homologation-class label. Field marks carry TimingNone (no timing method
// applies at all — measured, not timed): rendered as "n/a", not a gap.
func timingClassLabel(timing Timing) string {
	switch timing {
	case TimingElectronic:
		return "FAT"
	case TimingManual:
		return "hand"
	default:
		return "n/a"
	}
}

// FormatWind renders a wind reading to one decimal with an explicit sign,
// the D5.2 result-list convention ("+1.1", "-0.3").
func FormatWind(wind float64) string {
	return fmt.Sprintf("%+.1f", wind)
}
