// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"encoding/json"
	"fmt"
)

// ImportField names the canonical entry/athlete field one column of an entry
// import file maps to (SYS-013). The vocabulary is fixed; a mapping profile
// only says which CSV header feeds each field, mirroring the
// SeriesUploadColumnField precedent (seriesupload.go) for the inverse
// (export) direction.
type ImportField string

// The field vocabulary an ImportColumnMapping may name.
const (
	ImportFieldFirstName       ImportField = "firstName"
	ImportFieldLastName        ImportField = "lastName"
	ImportFieldBirthYear       ImportField = "birthYear"
	ImportFieldSex             ImportField = "sex"
	ImportFieldClub            ImportField = "club"
	ImportFieldLicenceNo       ImportField = "licenceNo"
	ImportFieldEventCode       ImportField = "eventCode"
	ImportFieldCategoryCode    ImportField = "categoryCode"
	ImportFieldSeedPerformance ImportField = "seedPerformance"
)

var validImportFields = map[ImportField]bool{
	ImportFieldFirstName: true, ImportFieldLastName: true, ImportFieldBirthYear: true,
	ImportFieldSex: true, ImportFieldClub: true, ImportFieldLicenceNo: true,
	ImportFieldEventCode: true, ImportFieldCategoryCode: true, ImportFieldSeedPerformance: true,
}

// RequiredImportFields are the fields every mapping profile must map for a
// row to be structurally importable (UC-004 #1/#3): person identity, age
// category and the event/discipline targeted. Club, licence number and seed
// performance are optional per row.
var RequiredImportFields = []ImportField{
	ImportFieldFirstName, ImportFieldLastName, ImportFieldBirthYear, ImportFieldSex,
	ImportFieldEventCode, ImportFieldCategoryCode,
}

// ImportColumnMapping is one CSV column of an ImportMappingProfile: which
// canonical field it feeds and the exact header text the source file uses.
type ImportColumnMapping struct {
	Field  ImportField `json:"field"`
	Header string      `json:"header"`
}

// ImportMappingProfile is a versioned, data-defined description of an entry
// import file's column layout (SYS-013): which header maps to which
// canonical field. Shipped built-in profiles are data, per ADR-005 §4 —
// "system-native" (this project's own documented CSV shape) and "alabus"
// (modelled against the Swiss Athletics/Alabus export column conventions
// described in the specs — see OQ-030 for the sample-format gap this ships
// against). A new source format is a new data file, not a code change.
type ImportMappingProfile struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Name    string `json:"name"`
	Source  string `json:"source"`
	Notes   string `json:"notes"`

	// EventCodeFormat documents how the eventCode column's value maps to a
	// meet's programme when no direct identifier exists (system-native and
	// alabus both use "<disciplineCode>/<categoryCode>", e.g. "100m/U16 M")
	// — a project convention, not a third-party format.
	EventCodeFormat string `json:"eventCodeFormat"`

	Columns []ImportColumnMapping `json:"columns"`
}

// ParseImportMappingProfile decodes and validates a mapping-profile data
// file (the ADR-005 "data interpreter" entry point, mirroring
// ParseCategoryScheme/ParseSeriesUploadTemplate).
func ParseImportMappingProfile(data []byte) (*ImportMappingProfile, error) {
	var p ImportMappingProfile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse import mapping profile: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("parse import mapping profile: %w", err)
	}
	return &p, nil
}

// Validate checks the profile's structural invariants: identity, version,
// known fields, non-duplicate field/header assignment, and every required
// field mapped to a column.
func (p *ImportMappingProfile) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("import mapping profile: id is required")
	}
	if p.Version == "" {
		return fmt.Errorf("import mapping profile %q: version is required (must carry a version identity)", p.ID)
	}
	if len(p.Columns) == 0 {
		return fmt.Errorf("import mapping profile %q: at least one column is required", p.ID)
	}
	seenField := map[ImportField]bool{}
	seenHeader := map[string]bool{}
	for _, c := range p.Columns {
		if !validImportFields[c.Field] {
			return fmt.Errorf("import mapping profile %q: unknown field %q", p.ID, c.Field)
		}
		if c.Header == "" {
			return fmt.Errorf("import mapping profile %q: column for field %q has no header", p.ID, c.Field)
		}
		if seenField[c.Field] {
			return fmt.Errorf("import mapping profile %q: field %q mapped more than once", p.ID, c.Field)
		}
		seenField[c.Field] = true
		if seenHeader[c.Header] {
			return fmt.Errorf("import mapping profile %q: header %q mapped more than once", p.ID, c.Header)
		}
		seenHeader[c.Header] = true
	}
	for _, f := range RequiredImportFields {
		if !seenField[f] {
			return fmt.Errorf("import mapping profile %q: required field %q is not mapped", p.ID, f)
		}
	}
	return nil
}

// HeaderFor returns the CSV header mapped to field, if any.
func (p *ImportMappingProfile) HeaderFor(field ImportField) (string, bool) {
	for _, c := range p.Columns {
		if c.Field == field {
			return c.Header, true
		}
	}
	return "", false
}
