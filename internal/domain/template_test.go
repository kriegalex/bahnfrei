// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import (
	"testing"
	"time"
)

// TestBuiltinUKCTemplate verifies the shipped UBS Kids Cup template against
// the Reglement facts (UC-033 #1, SYS-053): the three disciplines with the
// prescribed attempt counts, and references that resolve against the other
// built-in data sets without code.
func TestBuiltinUKCTemplate(t *testing.T) {
	tpl, err := BuiltinMeetTemplate(TemplateUBSKidsCup)
	if err != nil {
		t.Fatalf("BuiltinMeetTemplate: %v", err)
	}

	wantAttempts := map[string]int{"60m": 1, "ZoneLJ": 3, "BallThrow200g": 3}
	if len(tpl.Events) != len(wantAttempts) {
		t.Fatalf("template has %d events, want %d", len(tpl.Events), len(wantAttempts))
	}
	for _, ev := range tpl.Events {
		want, ok := wantAttempts[ev.DisciplineCode]
		if !ok {
			t.Errorf("unexpected template discipline %q", ev.DisciplineCode)
			continue
		}
		if ev.Attempts != want {
			t.Errorf("%s attempts = %d, want %d (Reglement §3)", ev.DisciplineCode, ev.Attempts, want)
		}
	}

	// Every reference the template names must resolve in the built-in data.
	catalog, err := BuiltinDisciplineCatalog()
	if err != nil {
		t.Fatalf("BuiltinDisciplineCatalog: %v", err)
	}
	for _, ev := range tpl.Events {
		if _, ok := catalog.ByCode(ev.DisciplineCode); !ok {
			t.Errorf("template discipline %q not in the built-in catalog", ev.DisciplineCode)
		}
	}
	scheme, err := BuiltinCategoryScheme(tpl.CategorySchemeID)
	if err != nil {
		t.Fatalf("template category scheme %q: %v", tpl.CategorySchemeID, err)
	}
	if _, err := BuiltinScoringTable(tpl.ScoringTableID); err != nil {
		t.Fatalf("template scoring table %q: %v", tpl.ScoringTableID, err)
	}

	// Per-birth-year divisions M/W 7–15 (UC-033 #1), and M7/W7 admit
	// younger children (Reglement category table footnote).
	nonElective := 0
	for _, c := range scheme.Categories {
		if !c.Elective {
			nonElective++
		}
	}
	if nonElective != 18 {
		t.Errorf("UKC scheme has %d default divisions, want 18 (M/W 7-15)", nonElective)
	}
	asOf := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	got, err := scheme.ResolveDefaultCategory(2021, SexFemale, asOf) // age 5
	if err != nil {
		t.Fatalf("ResolveDefaultCategory(younger child): %v", err)
	}
	if got.Code != "W7" {
		t.Errorf("age-5 girl resolves to %q, want W7 (Reglement: 'auch jüngere Kinder startberechtigt')", got.Code)
	}

	if _, err := BuiltinMeetTemplates(); err != nil {
		t.Errorf("BuiltinMeetTemplates: %v", err)
	}
	if _, err := BuiltinMeetTemplate("no-such-template"); err == nil {
		t.Error("BuiltinMeetTemplate(unknown): want error, got nil")
	}
}

func TestParseMeetTemplateRejectsBadData(t *testing.T) {
	cases := []struct {
		name string
		doc  string
	}{
		{"missing id", `{"version": "1", "categorySchemeID": "s", "events": [{"disciplineCode": "60m", "attempts": 1}]}`},
		{"missing version", `{"id": "x", "categorySchemeID": "s", "events": [{"disciplineCode": "60m", "attempts": 1}]}`},
		{"missing scheme", `{"id": "x", "version": "1", "events": [{"disciplineCode": "60m", "attempts": 1}]}`},
		{"no events", `{"id": "x", "version": "1", "categorySchemeID": "s", "events": []}`},
		{"empty discipline", `{"id": "x", "version": "1", "categorySchemeID": "s", "events": [{"disciplineCode": "", "attempts": 1}]}`},
		{"duplicate discipline", `{"id": "x", "version": "1", "categorySchemeID": "s", "events": [
			{"disciplineCode": "60m", "attempts": 1}, {"disciplineCode": "60m", "attempts": 3}]}`},
		{"zero attempts", `{"id": "x", "version": "1", "categorySchemeID": "s", "events": [{"disciplineCode": "60m", "attempts": 0}]}`},
		{"not json", `[`},
	}
	for _, tc := range cases {
		if _, err := ParseMeetTemplate([]byte(tc.doc)); err == nil {
			t.Errorf("%s: want error, got nil", tc.name)
		}
	}
}
