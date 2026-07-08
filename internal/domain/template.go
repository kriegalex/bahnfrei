// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"encoding/json"
	"fmt"
)

// TemplateEvent is one discipline of a meet template, with the series'
// attempt count (rule data, e.g. UBS Kids Cup: 1 sprint attempt, 3 long
// jump, 3 ball throw).
type TemplateEvent struct {
	DisciplineCode string `json:"disciplineCode"`
	Attempts       int    `json:"attempts"`
}

// MeetTemplate is a versioned, data-defined competition template (SYS-053):
// the category scheme, scoring table and event programme a series
// prescribes, such that creating a meet from it needs only a date and a
// venue (UC-033 #1). Like all rule-shaped data (ADR-005 §4) templates are
// data, not code — a new series (Visana Sprint, Mille Gruyère, … "Later"
// per SYS-053) is a new file, not a new code path.
type MeetTemplate struct {
	ID               string          `json:"id"`
	Version          string          `json:"version"`
	Name             string          `json:"name"`
	Source           string          `json:"source"`
	CategorySchemeID string          `json:"categorySchemeID"`
	ScoringTableID   string          `json:"scoringTableID"`
	Tier             string          `json:"tier"`
	Notes            string          `json:"notes"`
	Events           []TemplateEvent `json:"events"`
}

// ParseMeetTemplate decodes and validates a meet-template data file. The
// scheme/table/discipline references it names are resolved by the consumer
// against its loaded data sets (the app layer), keeping the parser generic.
func ParseMeetTemplate(data []byte) (*MeetTemplate, error) {
	var t MeetTemplate
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse meet template: %w", err)
	}
	if err := t.Validate(); err != nil {
		return nil, fmt.Errorf("parse meet template: %w", err)
	}
	return &t, nil
}

// Validate checks the template's structural invariants.
func (t *MeetTemplate) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("meet template: id is required")
	}
	if t.Version == "" {
		return fmt.Errorf("meet template %q: version is required (must carry a version identity)", t.ID)
	}
	if t.CategorySchemeID == "" {
		return fmt.Errorf("meet template %q: categorySchemeID is required", t.ID)
	}
	if len(t.Events) == 0 {
		return fmt.Errorf("meet template %q: at least one event is required", t.ID)
	}
	seen := make(map[string]bool, len(t.Events))
	for _, ev := range t.Events {
		if ev.DisciplineCode == "" {
			return fmt.Errorf("meet template %q: event with empty discipline code", t.ID)
		}
		if seen[ev.DisciplineCode] {
			return fmt.Errorf("meet template %q: duplicate event discipline %q", t.ID, ev.DisciplineCode)
		}
		seen[ev.DisciplineCode] = true
		if ev.Attempts < 1 {
			return fmt.Errorf("meet template %q: event %q needs at least one attempt", t.ID, ev.DisciplineCode)
		}
	}
	return nil
}
