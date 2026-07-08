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
//go:embed data/category-schemes/*.json data/disciplines/*.json data/scoring/*.json data/templates/*.json
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
)

var builtinScoringTableFiles = map[string]string{
	ScoringTableUBSKidsCup: "data/scoring/ubs-kids-cup.json",
}

var builtinTemplateFiles = map[string]string{
	TemplateUBSKidsCup: "data/templates/ubs-kids-cup.json",
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
