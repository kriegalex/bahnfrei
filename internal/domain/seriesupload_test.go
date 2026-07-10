// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package domain

import "testing"

// TestBuiltinUKCSeriesUploadTemplate verifies the shipped UBS Kids Cup
// series-upload template loads and carries the shape SYS-077 requires: a
// sheet name, identity columns covering the athlete-identity fields UC-035
// #1 needs, a discipline mark/points header pattern, and a non-empty
// missing-mark placeholder for UC-035 #2.
func TestBuiltinUKCSeriesUploadTemplate(t *testing.T) {
	tpl, err := BuiltinSeriesUploadTemplate(SeriesUploadUBSKidsCup)
	if err != nil {
		t.Fatalf("BuiltinSeriesUploadTemplate: %v", err)
	}
	if tpl.SheetName == "" {
		t.Error("sheetName is empty")
	}
	if tpl.MissingMarkPlaceholder == "" {
		t.Error("missingMarkPlaceholder is empty (UC-035 #2)")
	}
	wantFields := map[SeriesUploadColumnField]bool{
		SeriesUploadFieldBib: false, SeriesUploadFieldLastName: false,
		SeriesUploadFieldFirstName: false, SeriesUploadFieldBirthYear: false,
		SeriesUploadFieldClub: false, SeriesUploadFieldDivision: false,
	}
	for _, c := range tpl.IdentityColumns {
		if _, ok := wantFields[c.Field]; ok {
			wantFields[c.Field] = true
		}
	}
	for f, seen := range wantFields {
		if !seen {
			t.Errorf("identity columns miss field %q (UC-035 #1 athlete identity)", f)
		}
	}
	foundTotal := false
	for _, c := range tpl.TrailingColumns {
		if c.Field == SeriesUploadFieldTotal {
			foundTotal = true
		}
	}
	if !foundTotal {
		t.Error("trailing columns miss the total field (UC-035 #1)")
	}

	if _, err := BuiltinSeriesUploadTemplates(); err != nil {
		t.Errorf("BuiltinSeriesUploadTemplates: %v", err)
	}
	if _, err := BuiltinSeriesUploadTemplate("no-such-template"); err == nil {
		t.Error("BuiltinSeriesUploadTemplate(unknown): want error, got nil")
	}

	// The UKC meet template names this series-upload template (SYS-077 ↔
	// SYS-053 cross-reference).
	meetTpl, err := BuiltinMeetTemplate(TemplateUBSKidsCup)
	if err != nil {
		t.Fatalf("BuiltinMeetTemplate: %v", err)
	}
	if meetTpl.SeriesUploadTemplateID != SeriesUploadUBSKidsCup {
		t.Errorf("meet template seriesUploadTemplateID = %q, want %q", meetTpl.SeriesUploadTemplateID, SeriesUploadUBSKidsCup)
	}
}

func TestParseSeriesUploadTemplateRejectsBadData(t *testing.T) {
	valid := `"identityColumns": [{"field": "bib", "header": "StNr"}],
		"disciplineMarkHeader": "%s", "disciplinePointsHeader": "Punkte %s",
		"missingMarkPlaceholder": "-"`
	cases := []struct {
		name string
		doc  string
	}{
		{"missing id", `{"version": "1", ` + valid + `}`},
		{"missing version", `{"id": "x", ` + valid + `}`},
		{"missing sheet name", `{"id": "x", "version": "1", ` + valid + `}`},
		{"no identity columns", `{"id": "x", "version": "1", "sheetName": "s",
			"disciplineMarkHeader": "%s", "disciplinePointsHeader": "Punkte %s",
			"missingMarkPlaceholder": "-"}`},
		{"unknown identity field", `{"id": "x", "version": "1", "sheetName": "s",
			"identityColumns": [{"field": "nope", "header": "X"}],
			"disciplineMarkHeader": "%s", "disciplinePointsHeader": "Punkte %s",
			"missingMarkPlaceholder": "-"}`},
		{"empty column header", `{"id": "x", "version": "1", "sheetName": "s",
			"identityColumns": [{"field": "bib", "header": ""}],
			"disciplineMarkHeader": "%s", "disciplinePointsHeader": "Punkte %s",
			"missingMarkPlaceholder": "-"}`},
		{"mark header missing placeholder", `{"id": "x", "version": "1", "sheetName": "s",
			"identityColumns": [{"field": "bib", "header": "StNr"}],
			"disciplineMarkHeader": "fixed", "disciplinePointsHeader": "Punkte %s",
			"missingMarkPlaceholder": "-"}`},
		{"points header missing placeholder", `{"id": "x", "version": "1", "sheetName": "s",
			"identityColumns": [{"field": "bib", "header": "StNr"}],
			"disciplineMarkHeader": "%s", "disciplinePointsHeader": "fixed",
			"missingMarkPlaceholder": "-"}`},
		{"missing placeholder", `{"id": "x", "version": "1", "sheetName": "s",
			"identityColumns": [{"field": "bib", "header": "StNr"}],
			"disciplineMarkHeader": "%s", "disciplinePointsHeader": "Punkte %s"}`},
		{"not json", `[`},
	}
	for _, tc := range cases {
		if _, err := ParseSeriesUploadTemplate([]byte(tc.doc)); err == nil {
			t.Errorf("%s: want error, got nil", tc.name)
		}
	}
}

// TestSeriesUploadTemplateIsData proves UC-035 #3: a revised template
// document (different sheet name, different column set) parses and
// validates through the same generic interpreter with no code change — the
// data-swap test.
func TestSeriesUploadTemplateIsData(t *testing.T) {
	revised := `{
		"id": "ubs-kids-cup", "version": "2027-test", "name": "revised template",
		"sheetName": "Resultate2027",
		"identityColumns": [
			{"field": "bib", "header": "Startnummer"},
			{"field": "lastName", "header": "Nachname"},
			{"field": "division", "header": "Kat."}
		],
		"disciplineMarkHeader": "Leistung %s",
		"disciplinePointsHeader": "Pkt %s",
		"trailingColumns": [{"field": "total", "header": "Gesamt"}],
		"missingMarkPlaceholder": "n/a"
	}`
	tpl, err := ParseSeriesUploadTemplate([]byte(revised))
	if err != nil {
		t.Fatalf("ParseSeriesUploadTemplate(revised): %v", err)
	}
	if tpl.SheetName != "Resultate2027" {
		t.Errorf("SheetName = %q, want Resultate2027", tpl.SheetName)
	}
	if tpl.MissingMarkPlaceholder != "n/a" {
		t.Errorf("MissingMarkPlaceholder = %q, want n/a", tpl.MissingMarkPlaceholder)
	}
}
