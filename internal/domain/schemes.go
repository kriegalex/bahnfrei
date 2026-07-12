// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"embed"
	"fmt"
)

// Built-in category-scheme and discipline-catalog data files, versioned and
// shipped with the binary (ADR-005 §4: "rule-shaped data is data, not
// code"). These are loaded through the same generic ParseCategoryScheme /
// ParseDisciplineCatalog interpreter used for organizer-authored files
// (UC-002 #4) — embedding is purely a distribution mechanism, not a special
// code path.
//
//go:embed data/category-schemes/*.json data/disciplines/*.json data/scoring/*.json data/templates/*.json data/series-uploads/*.json data/import-profiles/*.json data/seeding/*.json
var builtinData embed.FS

// Built-in category-scheme identifiers (SYS-005).
const (
	SchemeSwissAthletics = "swiss-athletics"
	SchemeUBSKidsCup     = "ubs-kids-cup"
)

// BuiltinCategoryScheme loads and parses one of the shipped, verified
// category schemes by ID (SchemeSwissAthletics, SchemeUBSKidsCup).
func BuiltinCategoryScheme(id string) (*CategoryScheme, error) {
	path, ok := builtinSchemeFiles[id]
	if !ok {
		return nil, fmt.Errorf("unknown built-in category scheme %q", id)
	}
	data, err := builtinData.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read built-in category scheme %q: %w", id, err)
	}
	return ParseCategoryScheme(data)
}

// BuiltinCategorySchemes loads every shipped category scheme, keyed by ID.
func BuiltinCategorySchemes() (map[string]*CategoryScheme, error) {
	out := make(map[string]*CategoryScheme, len(builtinSchemeFiles))
	for id := range builtinSchemeFiles {
		s, err := BuiltinCategoryScheme(id)
		if err != nil {
			return nil, err
		}
		out[id] = s
	}
	return out, nil
}

var builtinSchemeFiles = map[string]string{
	SchemeSwissAthletics: "data/category-schemes/swiss-athletics.json",
	SchemeUBSKidsCup:     "data/category-schemes/ubs-kids-cup.json",
}

// Built-in scoring-table and meet-template identifiers (SYS-053).
const (
	ScoringTableUBSKidsCup = "ubs-kids-cup"
	TemplateUBSKidsCup     = "ubs-kids-cup"
	// TemplateWADecathlon/TemplateWAHeptathlon are the SYS-031/UC-013
	// combined-events templates (TASK-021): their meets score through
	// CombinedScoringTableWA2001 instead of a lookup ScoringTable.
	TemplateWADecathlon  = "wa-decathlon"
	TemplateWAHeptathlon = "wa-heptathlon"
)

var builtinScoringTableFiles = map[string]string{
	ScoringTableUBSKidsCup: "data/scoring/ubs-kids-cup.json",
}

var builtinTemplateFiles = map[string]string{
	TemplateUBSKidsCup:   "data/templates/ubs-kids-cup.json",
	TemplateWADecathlon:  "data/templates/wa-decathlon.json",
	TemplateWAHeptathlon: "data/templates/wa-heptathlon.json",
}

// BuiltinScoringTable loads and parses one of the shipped scoring tables by
// ID (SYS-053 / CON-01: series scoring tables ship as data).
func BuiltinScoringTable(id string) (*ScoringTable, error) {
	path, ok := builtinScoringTableFiles[id]
	if !ok {
		return nil, fmt.Errorf("unknown built-in scoring table %q", id)
	}
	data, err := builtinData.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read built-in scoring table %q: %w", id, err)
	}
	return ParseScoringTable(data)
}

// BuiltinScoringTables loads every shipped scoring table, keyed by ID.
func BuiltinScoringTables() (map[string]*ScoringTable, error) {
	out := make(map[string]*ScoringTable, len(builtinScoringTableFiles))
	for id := range builtinScoringTableFiles {
		t, err := BuiltinScoringTable(id)
		if err != nil {
			return nil, err
		}
		out[id] = t
	}
	return out, nil
}

// BuiltinMeetTemplate loads and parses one of the shipped competition
// templates by ID (SYS-053, UC-033 #1).
func BuiltinMeetTemplate(id string) (*MeetTemplate, error) {
	path, ok := builtinTemplateFiles[id]
	if !ok {
		return nil, fmt.Errorf("unknown built-in meet template %q", id)
	}
	data, err := builtinData.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read built-in meet template %q: %w", id, err)
	}
	return ParseMeetTemplate(data)
}

// BuiltinMeetTemplates loads every shipped meet template, keyed by ID.
func BuiltinMeetTemplates() (map[string]*MeetTemplate, error) {
	out := make(map[string]*MeetTemplate, len(builtinTemplateFiles))
	for id := range builtinTemplateFiles {
		t, err := BuiltinMeetTemplate(id)
		if err != nil {
			return nil, err
		}
		out[id] = t
	}
	return out, nil
}

// Built-in series-upload template identifier (SYS-077).
const SeriesUploadUBSKidsCup = "ubs-kids-cup"

var builtinSeriesUploadFiles = map[string]string{
	SeriesUploadUBSKidsCup: "data/series-uploads/ubs-kids-cup.json",
}

// BuiltinSeriesUploadTemplate loads and parses one of the shipped series
// results-upload templates by ID (SYS-077, UC-035): the organizer-portal
// upload-file layout, shipped as data so a season's template revision is a
// data-file swap (UC-035 #3).
func BuiltinSeriesUploadTemplate(id string) (*SeriesUploadTemplate, error) {
	path, ok := builtinSeriesUploadFiles[id]
	if !ok {
		return nil, fmt.Errorf("unknown built-in series upload template %q", id)
	}
	data, err := builtinData.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read built-in series upload template %q: %w", id, err)
	}
	return ParseSeriesUploadTemplate(data)
}

// BuiltinSeriesUploadTemplates loads every shipped series upload template,
// keyed by ID.
func BuiltinSeriesUploadTemplates() (map[string]*SeriesUploadTemplate, error) {
	out := make(map[string]*SeriesUploadTemplate, len(builtinSeriesUploadFiles))
	for id := range builtinSeriesUploadFiles {
		t, err := BuiltinSeriesUploadTemplate(id)
		if err != nil {
			return nil, err
		}
		out[id] = t
	}
	return out, nil
}

// Built-in combined-events scoring-table identifier (SYS-044, TASK-021).
const CombinedScoringTableWA2001 = "wa-combined-events-2001"

var builtinCombinedScoringTableFiles = map[string]string{
	CombinedScoringTableWA2001: "data/scoring/wa-combined-events-2001.json",
}

// BuiltinCombinedScoringTable loads and parses one of the shipped WA
// combined-events formula tables by ID (SYS-044 / ADR-005 §4: the 2001
// IAAF/WA formula coefficients ship as data, interpreted generically).
func BuiltinCombinedScoringTable(id string) (*CombinedScoringTable, error) {
	path, ok := builtinCombinedScoringTableFiles[id]
	if !ok {
		return nil, fmt.Errorf("unknown built-in combined scoring table %q", id)
	}
	data, err := builtinData.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read built-in combined scoring table %q: %w", id, err)
	}
	return ParseCombinedScoringTable(data)
}

// BuiltinCombinedScoringTables loads every shipped combined-events scoring
// table, keyed by ID.
func BuiltinCombinedScoringTables() (map[string]*CombinedScoringTable, error) {
	out := make(map[string]*CombinedScoringTable, len(builtinCombinedScoringTableFiles))
	for id := range builtinCombinedScoringTableFiles {
		t, err := BuiltinCombinedScoringTable(id)
		if err != nil {
			return nil, err
		}
		out[id] = t
	}
	return out, nil
}

// Built-in seeding-rules identifier (SYS-026/027, TASK-018): World Athletics
// TR20 seeding, heat-distribution and lane-draw rules (D2.2-D2.4).
const SeedingRulesTR20 = "tr20"

var builtinSeedingRulesFiles = map[string]string{
	SeedingRulesTR20: "data/seeding/tr20.json",
}

// BuiltinSeedingRules loads and parses one of the shipped seeding-rules data
// files by ID (SYS-026/027 — "the applied rule set SHALL be visible to the
// operator", D2.2).
func BuiltinSeedingRules(id string) (*SeedingRules, error) {
	path, ok := builtinSeedingRulesFiles[id]
	if !ok {
		return nil, fmt.Errorf("unknown built-in seeding rules %q", id)
	}
	data, err := builtinData.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read built-in seeding rules %q: %w", id, err)
	}
	return ParseSeedingRules(data)
}

// builtinDisciplineCatalogFile is the shipped discipline catalog (SYS-003).
// A var (not const) so tests can exercise the read-error path.
var builtinDisciplineCatalogFile = "data/disciplines/catalog.json"

// BuiltinDisciplineCatalog loads and parses the shipped discipline catalog.
func BuiltinDisciplineCatalog() (*DisciplineCatalog, error) {
	data, err := builtinData.ReadFile(builtinDisciplineCatalogFile)
	if err != nil {
		return nil, fmt.Errorf("read built-in discipline catalog: %w", err)
	}
	return ParseDisciplineCatalog(data)
}

// Built-in entry-import mapping-profile identifiers (SYS-013, TASK-017).
const (
	ImportProfileSystemNative = "system-native"
	ImportProfileAlabus       = "alabus"
)

var builtinImportMappingProfileFiles = map[string]string{
	ImportProfileSystemNative: "data/import-profiles/system-native.json",
	ImportProfileAlabus:       "data/import-profiles/alabus.json",
}

// BuiltinImportMappingProfile loads and parses one of the shipped entry
// import mapping profiles by ID (SYS-013: system-native CSV, or the Alabus
// assumption profile — see its data file's "source" note and OQ-030 for the
// unverified-format caveat).
func BuiltinImportMappingProfile(id string) (*ImportMappingProfile, error) {
	path, ok := builtinImportMappingProfileFiles[id]
	if !ok {
		return nil, fmt.Errorf("unknown built-in import mapping profile %q", id)
	}
	data, err := builtinData.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read built-in import mapping profile %q: %w", id, err)
	}
	return ParseImportMappingProfile(data)
}

// BuiltinImportMappingProfiles loads every shipped import mapping profile,
// keyed by ID.
func BuiltinImportMappingProfiles() (map[string]*ImportMappingProfile, error) {
	out := make(map[string]*ImportMappingProfile, len(builtinImportMappingProfileFiles))
	for id := range builtinImportMappingProfileFiles {
		p, err := BuiltinImportMappingProfile(id)
		if err != nil {
			return nil, err
		}
		out[id] = p
	}
	return out, nil
}
