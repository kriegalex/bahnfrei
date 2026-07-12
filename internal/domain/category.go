// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// Category is one row of a CategoryScheme: an age/sex competition class
// (SyRS §2 CategoryScheme/Category entity; ADR-005 §4 "rule-shaped data is
// data, not code"). Age bounds are nominal, calendar-year based (D4.2):
// an athlete's age-this-year is (competition year − birth year).
type Category struct {
	// Code is the exact, scheme-defined display code (e.g. "U16 M", "Men",
	// "M35"). Codes are unique within a scheme.
	Code string `json:"code"`
	// Sex this category is offered for. Empty means sex-agnostic (e.g. a
	// mixed/open youth-series division).
	Sex Sex `json:"sex"`
	// MinAge/MaxAge bound the nominal age window (inclusive). MaxAge nil
	// means open-ended (e.g. Swiss "Men"/"Women" 23+).
	MinAge int  `json:"minAge"`
	MaxAge *int `json:"maxAge"`
	// Rank orders categories for start-up/start-down comparison and default
	// resolution priority (ascending = younger/lower to older/higher).
	Rank int `json:"rank"`
	// Label is a human-readable descriptive name (docs/UI only).
	Label string `json:"label"`
	// StartUpAllowed: a younger athlete may enter this category ahead of
	// their default age band (WO §1.2 general note).
	StartUpAllowed bool `json:"startUpAllowed"`
	// StartDownAllowed: an older athlete may enter this category behind
	// their default age band (e.g. Masters starting down, WO §1.2).
	StartDownAllowed bool `json:"startDownAllowed"`
	// Elective categories are never returned by default resolution (e.g.
	// Masters bands); they are only reachable by explicit selection.
	Elective bool `json:"elective"`
}

// InAgeRange reports whether age falls within [MinAge, MaxAge] (MaxAge nil
// = unbounded above).
func (c Category) InAgeRange(age int) bool {
	if age < c.MinAge {
		return false
	}
	return c.MaxAge == nil || age <= *c.MaxAge
}

// MatchesSex reports whether the category applies to sex (categories with an
// empty Sex are sex-agnostic/mixed divisions).
func (c Category) MatchesSex(sex Sex) bool {
	return c.Sex == "" || c.Sex == sex
}

// DisciplineRestriction bars a set of categories from a discipline (D4.3
// youth-protection rule shape): rule-data, not code, per ADR-005 §4.
type DisciplineRestriction struct {
	DisciplineCode      string   `json:"disciplineCode"`
	BarredCategoryCodes []string `json:"barredCategoryCodes"`
	Citation            string   `json:"citation"`
}

// YouthProtectionRule is one D4.3 youth-protection band: the maximum
// stadium race distance and the "at most one race at/above a threshold
// distance per competition day" cap that apply to a set of category codes
// (SYS-014, UC-005 #2/#3). Rule-shaped data, not code, per ADR-005 §4 — like
// DisciplineRestriction above.
type YouthProtectionRule struct {
	// CategoryCodes names every category this rule applies to (typically a
	// tier's M/W pair, e.g. "U12 M"/"U12 W").
	CategoryCodes []string `json:"categoryCodes"`
	// MaxStadiumDistanceM caps the longest stadium-venue race distance this
	// category may enter, in metres; zero means no cap.
	MaxStadiumDistanceM int `json:"maxStadiumDistanceM"`
	// MaxOneRaceAtOrAboveM: at most one race at/above this distance (metres)
	// per competition day; zero means the cap does not apply to this
	// category.
	MaxOneRaceAtOrAboveM int    `json:"maxOneRaceAtOrAboveM"`
	Citation             string `json:"citation"`
}

// CategoryScheme is the SyRS §2 CategoryScheme entity: a versioned,
// data-defined set of Category rows plus discipline restrictions (SYS-005).
// It is interpreted generically by the resolver functions below — a new
// scheme (built-in or organizer-authored) requires no code change (UC-002
// #4).
type CategoryScheme struct {
	ID                     string                  `json:"id"`
	Version                string                  `json:"version"`
	Name                   string                  `json:"name"`
	Source                 string                  `json:"source"`
	Transition             string                  `json:"transition"`
	Notes                  string                  `json:"notes"`
	Categories             []Category              `json:"categories"`
	DisciplineRestrictions []DisciplineRestriction `json:"disciplineRestrictions"`
	// YouthProtectionRules ships the D4.3 max-distance / one-race-per-day
	// bands (SYS-014); a scheme with none (e.g. UBS Kids Cup, which has its
	// own distance-free format) simply has an empty slice.
	YouthProtectionRules []YouthProtectionRule `json:"youthProtectionRules"`
}

// ParseCategoryScheme decodes and validates a category-scheme data file
// (the ADR-005 "data interpreter" entry point). It is generic: any
// well-formed document — built-in or organizer-authored — loads the same way
// (UC-002 #4), and callers keep the parsed value with a version identity for
// audit/traceability.
func ParseCategoryScheme(data []byte) (*CategoryScheme, error) {
	var s CategoryScheme
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse category scheme: %w", err)
	}
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("parse category scheme: %w", err)
	}
	return &s, nil
}

// Validate checks the scheme's structural invariants: identity, version,
// non-empty and uniquely coded categories, and that every discipline
// restriction references known category codes.
func (s *CategoryScheme) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("scheme: id is required")
	}
	if s.Version == "" {
		return fmt.Errorf("scheme %q: version is required (must carry a version identity)", s.ID)
	}
	if len(s.Categories) == 0 {
		return fmt.Errorf("scheme %q: at least one category is required", s.ID)
	}
	seen := make(map[string]bool, len(s.Categories))
	for _, c := range s.Categories {
		if c.Code == "" {
			return fmt.Errorf("scheme %q: category with empty code", s.ID)
		}
		if seen[c.Code] {
			return fmt.Errorf("scheme %q: duplicate category code %q", s.ID, c.Code)
		}
		seen[c.Code] = true
		if c.MaxAge != nil && *c.MaxAge < c.MinAge {
			return fmt.Errorf("scheme %q: category %q has maxAge < minAge", s.ID, c.Code)
		}
	}
	for _, r := range s.DisciplineRestrictions {
		for _, code := range r.BarredCategoryCodes {
			if !seen[code] {
				return fmt.Errorf("scheme %q: discipline restriction %q references unknown category %q", s.ID, r.DisciplineCode, code)
			}
		}
	}
	for _, r := range s.YouthProtectionRules {
		if len(r.CategoryCodes) == 0 {
			return fmt.Errorf("scheme %q: youth protection rule has no category codes", s.ID)
		}
		for _, code := range r.CategoryCodes {
			if !seen[code] {
				return fmt.Errorf("scheme %q: youth protection rule references unknown category %q", s.ID, code)
			}
		}
	}
	return nil
}

// YouthProtectionFor returns the youth-protection rule that applies to
// categoryCode, if any (SYS-014, UC-005 #2/#3).
func (s *CategoryScheme) YouthProtectionFor(categoryCode string) (YouthProtectionRule, bool) {
	for _, r := range s.YouthProtectionRules {
		for _, code := range r.CategoryCodes {
			if code == categoryCode {
				return r, true
			}
		}
	}
	return YouthProtectionRule{}, false
}

// CategoryByCode returns the category with the given code, if present.
func (s *CategoryScheme) CategoryByCode(code string) (Category, bool) {
	for _, c := range s.Categories {
		if c.Code == code {
			return c, true
		}
	}
	return Category{}, false
}

// ResolveDefaultCategory computes an athlete's default category for a given
// competition year, per the calendar-year transition rule (D4.2, UC-002
// #1-#2): age = asOf.Year() − birthYear; the first non-elective category
// (ascending Rank) whose age range and sex match wins. Elective categories
// (e.g. Masters bands) are never returned by default — they exist for
// EligibleCategories/EvaluateEntry only.
func (s *CategoryScheme) ResolveDefaultCategory(birthYear int, sex Sex, asOf time.Time) (Category, error) {
	age := asOf.Year() - birthYear
	best := rankedCategories(s.Categories)
	for _, c := range best {
		if c.Elective {
			continue
		}
		if c.MatchesSex(sex) && c.InAgeRange(age) {
			return c, nil
		}
	}
	return Category{}, fmt.Errorf("scheme %q: no default category for sex %q at age %d (birth year %d, as of %d)", s.ID, sex, age, birthYear, asOf.Year())
}

// EligibleCategories returns every category (default and elective) whose
// age range and sex match, ordered by Rank ascending. This is the candidate
// set an operator may choose among for entry (default plus any Masters/open
// division reachable by age alone).
func (s *CategoryScheme) EligibleCategories(birthYear int, sex Sex, asOf time.Time) []Category {
	age := asOf.Year() - birthYear
	var out []Category
	for _, c := range rankedCategories(s.Categories) {
		if c.MatchesSex(sex) && c.InAgeRange(age) {
			out = append(out, c)
		}
	}
	return out
}

// EntryDecision is the outcome of evaluating an entry into a target category
// against an athlete's default category (UC-002 #3).
type EntryDecision struct {
	Allowed     bool
	StartedUp   bool
	StartedDown bool
	Reason      string
}

// EvaluateEntry decides whether an athlete whose default category is
// athleteDefault may enter target, per the scheme's start-up/start-down
// flags (UC-002 #3). Categories of different Sex never match (an athlete
// cannot start up/down across sexes) — a structural rule, not a scheme flag.
func (s *CategoryScheme) EvaluateEntry(athleteDefault, target Category) EntryDecision {
	if target.Sex != "" && athleteDefault.Sex != "" && target.Sex != athleteDefault.Sex {
		return EntryDecision{Allowed: false, Reason: fmt.Sprintf("category %q does not match athlete sex %q", target.Code, athleteDefault.Sex)}
	}
	if target.Code == athleteDefault.Code {
		return EntryDecision{Allowed: true}
	}
	switch {
	case target.Rank > athleteDefault.Rank:
		if target.StartUpAllowed {
			return EntryDecision{Allowed: true, StartedUp: true}
		}
		return EntryDecision{Allowed: false, Reason: fmt.Sprintf("scheme %q does not permit starting up into %q", s.ID, target.Code)}
	case target.Rank < athleteDefault.Rank:
		if target.StartDownAllowed {
			return EntryDecision{Allowed: true, StartedDown: true}
		}
		return EntryDecision{Allowed: false, Reason: fmt.Sprintf("scheme %q does not permit starting down into %q", s.ID, target.Code)}
	default:
		// Same rank, different code (e.g. two elective bands sharing a
		// rank) — same-rank cross-category entry is not a start-up/down
		// scenario the scheme models; reject conservatively.
		return EntryDecision{Allowed: false, Reason: fmt.Sprintf("category %q is not reachable from %q", target.Code, athleteDefault.Code)}
	}
}

// CheckDisciplineEligibility reports whether an athlete whose (default)
// category code is athleteCategoryCode may compete in disciplineCode, per
// the scheme's discipline restrictions (D4.3 youth-protection rule, UC-002
// #3). The restriction is athlete-category-based: it applies regardless of
// which event category the athlete is entered under (e.g. a U14 athlete
// stays barred from 400mH even when started up into a Men event).
func (s *CategoryScheme) CheckDisciplineEligibility(disciplineCode, athleteCategoryCode string) (bool, string) {
	for _, r := range s.DisciplineRestrictions {
		if r.DisciplineCode != disciplineCode {
			continue
		}
		for _, barred := range r.BarredCategoryCodes {
			if barred == athleteCategoryCode {
				return false, r.Citation
			}
		}
	}
	return true, ""
}

// rankedCategories returns a copy of cats sorted by Rank ascending, stable
// for equal ranks (insertion order preserved).
func rankedCategories(cats []Category) []Category {
	out := make([]Category, len(cats))
	copy(out, cats)
	// Simple stable insertion sort: category lists are small (tens of
	// rows), and stability must be preserved for same-rank ties.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Rank < out[j-1].Rank; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
