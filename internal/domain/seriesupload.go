// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SeriesUploadColumnField names the data field one column of a series
// results-upload sheet draws from. SeriesUploadFieldDivision/Total etc. are
// fixed per-row values; SeriesUploadFieldDisciplineMark/Points are not
// single columns but a repeating pair — one per meet discipline, in the
// meet template's event order (SYS-053) — interpreted generically by
// internal/app's SeriesUploadExport, never special-cased by discipline
// name.
type SeriesUploadColumnField string

// The field vocabulary a SeriesUploadTemplate column may name.
const (
	SeriesUploadFieldRank      SeriesUploadColumnField = "rank"
	SeriesUploadFieldBib       SeriesUploadColumnField = "bib"
	SeriesUploadFieldLastName  SeriesUploadColumnField = "lastName"
	SeriesUploadFieldFirstName SeriesUploadColumnField = "firstName"
	SeriesUploadFieldBirthYear SeriesUploadColumnField = "birthYear"
	SeriesUploadFieldSex       SeriesUploadColumnField = "sex"
	SeriesUploadFieldClub      SeriesUploadColumnField = "club"
	SeriesUploadFieldDivision  SeriesUploadColumnField = "division"
	SeriesUploadFieldTotal     SeriesUploadColumnField = "total"
)

var validSeriesUploadFields = map[SeriesUploadColumnField]bool{
	SeriesUploadFieldRank: true, SeriesUploadFieldBib: true,
	SeriesUploadFieldLastName: true, SeriesUploadFieldFirstName: true,
	SeriesUploadFieldBirthYear: true, SeriesUploadFieldSex: true,
	SeriesUploadFieldClub: true, SeriesUploadFieldDivision: true,
	SeriesUploadFieldTotal: true,
}

// SeriesUploadColumn is one fixed (non-discipline) column of the upload
// sheet: which field feeds it and its header label.
type SeriesUploadColumn struct {
	Field  SeriesUploadColumnField `json:"field"`
	Header string                  `json:"header"`
}

// SeriesUploadTemplate is a versioned, data-defined description of a Swiss
// youth-series organizer-portal results-upload workbook (SYS-077): the
// sheet name and column layout the series' upload tooling expects, kept as
// data — like the meet template and scoring table it is paired with — so a
// season's template revision is a data-file swap, not a code change
// (UC-035 #3). internal/app's SeriesUploadExport interprets this shape
// generically: fixed identity columns, then one mark+points column pair per
// meet discipline (in template order), then fixed trailing columns.
//
// Provenance note (OQ-023, docs/requirements/open-questions-and-assumptions.md):
// the exact byte-for-byte column layout of the official Nachwuchsserien
// "Vorlage Resultatimport" organizer-upload file could not be retrieved
// offline. This shipped template is an assumption grounded in two verified
// primary sources: (1) the Mille Gruyère/Visana Sprint organizer manual
// confirms a downloadable Excel "Vorlage Resultatimport" exists for the
// non-TAF3 results path; (2) a real UBS Kids Cup TAF3 result-list export
// (Gesamtrangliste, Langenthal 2025) shows the column semantics this
// template mirrors (rank, bib, name, birth year, club, per-discipline
// mark+points, total). Swap this file for the verified template once
// obtained — no code change required.
type SeriesUploadTemplate struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Name    string `json:"name"`
	Source  string `json:"source"`

	SheetName       string               `json:"sheetName"`
	IdentityColumns []SeriesUploadColumn `json:"identityColumns"`
	// DisciplineMarkHeader/DisciplinePointsHeader are fmt-style patterns
	// with exactly one "%s" verb, substituted with the discipline's
	// display name — one mark+points column pair is emitted per meet
	// discipline, in template event order.
	DisciplineMarkHeader   string               `json:"disciplineMarkHeader"`
	DisciplinePointsHeader string               `json:"disciplinePointsHeader"`
	TrailingColumns        []SeriesUploadColumn `json:"trailingColumns"`

	// MissingMarkPlaceholder fills a mark cell for a discipline the athlete
	// has no scoring result for (UC-035 #2) — the row is never dropped and
	// the cell is never silently blank. A qualification-status code (DNS,
	// NM, DQ, …) renders in place of this placeholder when the athlete has
	// one.
	MissingMarkPlaceholder string `json:"missingMarkPlaceholder"`

	// UnrankedMissingLabel/UnrankedOutOfCompetitionLabel fill the rank and
	// total columns of a FINAL-standings row TASK-036/DEC-016 leaves
	// unranked, mirroring the official TAF3 convention (LV Langenthal
	// Gesamtrangliste, 17.05.2025): "aufg." for a participant missing a
	// series discipline entirely, "n.a." for one flagged out-of-competition
	// (OQ-090/OQ-091). Both optional — an unset label falls back to
	// MissingMarkPlaceholder, and a provisional (not yet complete) export
	// never uses either since rows still rank by partial total.
	UnrankedMissingLabel          string `json:"unrankedMissingLabel,omitempty"`
	UnrankedOutOfCompetitionLabel string `json:"unrankedOutOfCompetitionLabel,omitempty"`
}

// ParseSeriesUploadTemplate decodes and validates a series-upload template
// data file (the SYS-077 data-interpreter entry point — built-in and
// operator-supplied files load identically, mirroring ParseMeetTemplate /
// ParseScoringTable).
func ParseSeriesUploadTemplate(data []byte) (*SeriesUploadTemplate, error) {
	var t SeriesUploadTemplate
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse series upload template: %w", err)
	}
	if err := t.Validate(); err != nil {
		return nil, fmt.Errorf("parse series upload template: %w", err)
	}
	return &t, nil
}

// Validate checks the template's structural invariants.
func (t *SeriesUploadTemplate) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("series upload template: id is required")
	}
	if t.Version == "" {
		return fmt.Errorf("series upload template %q: version is required (must carry a version identity)", t.ID)
	}
	if t.SheetName == "" {
		return fmt.Errorf("series upload template %q: sheetName is required", t.ID)
	}
	if len(t.IdentityColumns) == 0 {
		return fmt.Errorf("series upload template %q: at least one identity column is required", t.ID)
	}
	for i, c := range t.IdentityColumns {
		if err := validateSeriesUploadColumn(t.ID, "identity", i, c); err != nil {
			return err
		}
	}
	for i, c := range t.TrailingColumns {
		if err := validateSeriesUploadColumn(t.ID, "trailing", i, c); err != nil {
			return err
		}
	}
	if !strings.Contains(t.DisciplineMarkHeader, "%s") {
		return fmt.Errorf("series upload template %q: disciplineMarkHeader must contain a %%s discipline-name placeholder", t.ID)
	}
	if !strings.Contains(t.DisciplinePointsHeader, "%s") {
		return fmt.Errorf("series upload template %q: disciplinePointsHeader must contain a %%s discipline-name placeholder", t.ID)
	}
	if t.MissingMarkPlaceholder == "" {
		return fmt.Errorf("series upload template %q: missingMarkPlaceholder is required (UC-035 #2)", t.ID)
	}
	return nil
}

func validateSeriesUploadColumn(templateID, section string, i int, c SeriesUploadColumn) error {
	if !validSeriesUploadFields[c.Field] {
		return fmt.Errorf("series upload template %q: %s column %d has unknown field %q", templateID, section, i, c.Field)
	}
	if c.Header == "" {
		return fmt.Errorf("series upload template %q: %s column %d has no header", templateID, section, i)
	}
	return nil
}
