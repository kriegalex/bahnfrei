// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
)

// DisciplineFamily is the discipline grouping vocabulary fixed by SyRS §2.
type DisciplineFamily string

const (
	FamilyTrack           DisciplineFamily = "track"
	FamilyFieldHorizontal DisciplineFamily = "field-horizontal"
	FamilyFieldVertical   DisciplineFamily = "field-vertical"
	FamilyCombined        DisciplineFamily = "combined"
	FamilyRelay           DisciplineFamily = "relay"
)

// DisciplineUnit is the kind of mark a discipline is recorded in.
type DisciplineUnit string

const (
	UnitTime     DisciplineUnit = "time"
	UnitDistance DisciplineUnit = "distance"
	UnitHeight   DisciplineUnit = "height"
	UnitPoints   DisciplineUnit = "points"
)

// DisciplineVariant is a per-category technical variant of a Discipline
// (SYS-003): hurdle height/spacing, implement mass, etc. A Discipline may
// have zero variants (most track/jump events need none) or several (one per
// category tier that differs technically).
type DisciplineVariant struct {
	CategoryCode     string   `json:"categoryCode"`
	HurdleHeightM    *float64 `json:"hurdleHeightM,omitempty"`
	HurdleSpacingM   *float64 `json:"hurdleSpacingM,omitempty"`
	DistanceToFirstM *float64 `json:"distanceToFirstM,omitempty"`
	// HurdleCount is the number of hurdles on the course for this category
	// (nil when the variant carries no hurdle layout, or when only the
	// implement differs). A hurdle variant with a count but no height means
	// the height is meet-defined within a source-cited range (WO 2026
	// §8.1.3 footnote: "Die Hürdenhöhe muss in der jeweiligen Ausschreibung
	// publiziert werden").
	HurdleCount     *int     `json:"hurdleCount,omitempty"`
	ImplementMassKg *float64 `json:"implementMassKg,omitempty"`
	Citation        string   `json:"citation"`
}

// Discipline is the SyRS §2 Discipline entity: code, family, unit,
// wind-relevance, and per-category technical variants (SYS-003).
type Discipline struct {
	Code         string              `json:"code"`
	Name         string              `json:"name"`
	Family       DisciplineFamily    `json:"family"`
	Unit         DisciplineUnit      `json:"unit"`
	WindRelevant bool                `json:"windRelevant"`
	Source       string              `json:"source"`
	Variants     []DisciplineVariant `json:"variants"`
}

// VariantFor returns the technical variant for categoryCode, if the
// discipline defines one (UC-002 #5: "100mH / U18 W vs 100mH / Women, each
// carries category-correct technical variant... from the built-in data").
func (d Discipline) VariantFor(categoryCode string) (DisciplineVariant, bool) {
	for _, v := range d.Variants {
		if v.CategoryCode == categoryCode {
			return v, true
		}
	}
	return DisciplineVariant{}, false
}

// DisciplineCatalog is the SyRS §2 built-in discipline catalog (SYS-003):
// a versioned, data-defined list of Disciplines. Organizers may define
// additional custom disciplines (SYS-003) by constructing/merging their own
// DisciplineCatalog — the catalog format itself requires no code change to
// grow (ADR-005 §4).
type DisciplineCatalog struct {
	ID          string       `json:"id"`
	Version     string       `json:"version"`
	Name        string       `json:"name"`
	Source      string       `json:"source"`
	Notes       string       `json:"notes"`
	Disciplines []Discipline `json:"disciplines"`
}

// ParseDisciplineCatalog decodes and validates a discipline-catalog data
// file (the ADR-005 "data interpreter" entry point).
func ParseDisciplineCatalog(data []byte) (*DisciplineCatalog, error) {
	var c DisciplineCatalog
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse discipline catalog: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("parse discipline catalog: %w", err)
	}
	return &c, nil
}

// Validate checks the catalog's structural invariants: identity, version,
// non-empty and uniquely coded disciplines with a known family/unit.
func (c *DisciplineCatalog) Validate() error {
	if c.ID == "" {
		return fmt.Errorf("catalog: id is required")
	}
	if c.Version == "" {
		return fmt.Errorf("catalog %q: version is required (must carry a version identity)", c.ID)
	}
	if len(c.Disciplines) == 0 {
		return fmt.Errorf("catalog %q: at least one discipline is required", c.ID)
	}
	seen := make(map[string]bool, len(c.Disciplines))
	for _, d := range c.Disciplines {
		if d.Code == "" {
			return fmt.Errorf("catalog %q: discipline with empty code", c.ID)
		}
		if seen[d.Code] {
			return fmt.Errorf("catalog %q: duplicate discipline code %q", c.ID, d.Code)
		}
		seen[d.Code] = true
		switch d.Family {
		case FamilyTrack, FamilyFieldHorizontal, FamilyFieldVertical, FamilyCombined, FamilyRelay:
		default:
			return fmt.Errorf("catalog %q: discipline %q has unknown family %q", c.ID, d.Code, d.Family)
		}
	}
	return nil
}

// ByCode returns the discipline with the given code, if present.
func (c *DisciplineCatalog) ByCode(code string) (Discipline, bool) {
	for _, d := range c.Disciplines {
		if d.Code == code {
			return d, true
		}
	}
	return Discipline{}, false
}

// trackDistancePattern matches a leading run of digits followed by "m" in a
// track/relay discipline code (e.g. "800m", "3000mSC", "300mH") — the D4.3
// youth-protection distance rules (SYS-014, UC-005 #2/#3) key off this
// numeric distance, not a separate catalog field, since the code already
// carries it for every stadium track/hurdles/steeplechase discipline.
var trackDistancePattern = regexp.MustCompile(`^(\d+)m`)

// TrackDistanceMeters extracts the race distance in metres encoded in a
// track/relay discipline code's leading digits (e.g. "800m" -> 800,
// "3000mSC" -> 3000, "300mH" -> 300). Returns false for codes with no
// leading-digit distance (field disciplines, "60m" hurdles-style codes
// without digits, etc. still parse fine as long as they start with digits).
func TrackDistanceMeters(code string) (int, bool) {
	m := trackDistancePattern.FindStringSubmatch(code)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}
