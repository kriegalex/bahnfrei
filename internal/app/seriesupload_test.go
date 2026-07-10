// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import (
	"bytes"
	"context"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// openSeriesUpload calls SeriesUploadExport and opens the resulting bytes
// as a workbook for structural assertions.
func openSeriesUpload(t *testing.T, results *ResultsService, meetID string) (*excelize.File, string) {
	t.Helper()
	data, filename, err := results.SeriesUploadExport(context.Background(), meetID)
	if err != nil {
		t.Fatalf("SeriesUploadExport: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("excelize.OpenReader: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f, filename
}

// TestSeriesUploadExportSYS077UC035_1 covers UC-035 #1 (SYS-077): the
// exported workbook conforms to the shipped organizer-template structure
// (sheet name, header row, column order — a fixture derived from the
// published template, TestBuiltinUKCSeriesUploadTemplate's shape) and every
// participant appears with marks, points and division data.
func TestSeriesUploadExportSYS077UC035_1(t *testing.T) {
	meets, results, _ := newTestResults(t)
	rec := createUKCMeet(t, meets)

	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014,
		Sex: domain.SexFemale, Club: "LC Test", Bib: "101",
	})
	save(t, results, rec.ID, ResultInput{AthleteID: anna.AthleteID, DisciplineCode: "60m", Mark: "8.42", Timing: domain.TimingElectronic})
	save(t, results, rec.ID, ResultInput{AthleteID: anna.AthleteID, DisciplineCode: "ZoneLJ", Mark: "4.12"})
	save(t, results, rec.ID, ResultInput{AthleteID: anna.AthleteID, DisciplineCode: "BallThrow200g", Mark: "38.50"})

	max := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Max", LastName: "Model", BirthYear: 2014, Sex: domain.SexMale, Bib: "201",
	})
	save(t, results, rec.ID, ResultInput{AthleteID: max.AthleteID, DisciplineCode: "60m", Mark: "8.42", Timing: domain.TimingElectronic})

	f, filename := openSeriesUpload(t, results, rec.ID)

	if filename == "" {
		t.Error("filename is empty")
	}
	sheets := f.GetSheetList()
	if len(sheets) != 1 || sheets[0] != "Rangliste" {
		t.Fatalf("sheets = %v, want exactly [Rangliste] (fixture-verified sheet name)", sheets)
	}
	sheet := sheets[0]

	wantHeaders := []string{
		"Kategorie", "Rang", "StNr", "Name", "Vorname", "Jg", "Geschlecht", "Verein",
		"60 metres", "Punkte 60 metres",
		"Zone Long Jump (UKC)", "Punkte Zone Long Jump (UKC)",
		"200 g Ball Throw (UKC)", "Punkte 200 g Ball Throw (UKC)",
		"Total",
	}
	for i, want := range wantHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		got, err := f.GetCellValue(sheet, cell)
		if err != nil {
			t.Fatalf("GetCellValue(%s): %v", cell, err)
		}
		if got != want {
			t.Errorf("header col %d = %q, want %q", i+1, got, want)
		}
	}

	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	if len(rows) != 3 { // header + Anna (W12) + Max (M12)
		t.Fatalf("got %d rows, want 3 (header + 2 participants)", len(rows))
	}

	byBib := map[string][]string{}
	for _, row := range rows[1:] {
		byBib[row[2]] = row // StNr is column 3 (index 2)
	}
	annaRow, ok := byBib["101"]
	if !ok {
		t.Fatalf("Anna (bib 101) missing from export, want every participant present")
	}
	wantAnna := map[int]string{
		0: "W12", 2: "101", 3: "Muster", 4: "Anna", 5: "2014", 6: "W", 7: "LC Test",
		8: "8.42", 9: "710", 10: "4.12", 12: "38.50", 14: "1838", // 710+548+580
	}
	for col, want := range wantAnna {
		if col >= len(annaRow) {
			t.Fatalf("Anna row too short: %v", annaRow)
		}
		if annaRow[col] != want {
			t.Errorf("Anna row col %d = %q, want %q (full row: %v)", col, annaRow[col], want, annaRow)
		}
	}
	// Sex "W" was requested as SexFemale ("W" per domain.SexFemale) — verify
	// the exported code matches the domain vocabulary exactly.
	if annaRow[6] != string(domain.SexFemale) {
		t.Errorf("Anna sex column = %q, want domain.SexFemale (%q)", annaRow[6], domain.SexFemale)
	}

	maxRow, ok := byBib["201"]
	if !ok {
		t.Fatalf("Max (bib 201) missing from export")
	}
	if maxRow[0] != "M12" {
		t.Errorf("Max division column = %q, want M12", maxRow[0])
	}
	if maxRow[8] != "8.42" || maxRow[9] != "599" {
		t.Errorf("Max 60m mark/points = %q/%q, want 8.42/599 (male scoring column)", maxRow[8], maxRow[9])
	}
}

// TestSeriesUploadExportSYS077UC035_2 covers UC-035 #2 (SYS-077): an athlete
// with a missing discipline or a non-scoring status (DNS) is represented
// explicitly per the series convention — the row is never dropped, the
// missing mark renders the template's placeholder, and a status code
// overrides the placeholder where the athlete has one.
func TestSeriesUploadExportSYS077UC035_2(t *testing.T) {
	meets, results, _ := newTestResults(t)
	rec := createUKCMeet(t, meets)

	gap := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Gina", LastName: "Gap", BirthYear: 2015, Sex: domain.SexFemale, Bib: "1",
	})
	save(t, results, rec.ID, ResultInput{AthleteID: gap.AthleteID, DisciplineCode: "60m", Mark: "8.42", Timing: domain.TimingElectronic})
	save(t, results, rec.ID, ResultInput{AthleteID: gap.AthleteID, DisciplineCode: "ZoneLJ", Mark: "4.12"})
	// BallThrow200g left uncaptured entirely — the missing-discipline case.

	dns := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Dana", LastName: "NoShow", BirthYear: 2015, Sex: domain.SexFemale, Bib: "2",
	})
	save(t, results, rec.ID, ResultInput{AthleteID: dns.AthleteID, DisciplineCode: "60m", Status: domain.StatusDNS})
	save(t, results, rec.ID, ResultInput{AthleteID: dns.AthleteID, DisciplineCode: "ZoneLJ", Mark: "3.00"})
	save(t, results, rec.ID, ResultInput{AthleteID: dns.AthleteID, DisciplineCode: "BallThrow200g", Mark: "20.00"})

	f, _ := openSeriesUpload(t, results, rec.ID)
	sheet := f.GetSheetList()[0]
	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	if len(rows) != 3 { // header + 2 participants — neither row dropped
		t.Fatalf("got %d rows, want 3 (no row silently dropped)", len(rows))
	}

	byBib := map[string][]string{}
	for _, row := range rows[1:] {
		byBib[row[2]] = row
	}

	gapRow, ok := byBib["1"]
	if !ok {
		t.Fatalf("Gina (missing discipline) dropped from export — must never be silently dropped")
	}
	// BallThrow200g mark/points are columns 12/13 (0-indexed).
	if gapRow[12] != "–" {
		t.Errorf("Gina's missing BallThrow200g mark = %q, want the template placeholder \"–\"", gapRow[12])
	}
	if gapRow[13] != "" {
		t.Errorf("Gina's missing BallThrow200g points = %q, want empty", gapRow[13])
	}

	dnsRow, ok := byBib["2"]
	if !ok {
		t.Fatalf("Dana (DNS) dropped from export — must never be silently dropped")
	}
	if dnsRow[8] != "DNS" {
		t.Errorf("Dana's 60m mark = %q, want status code DNS rendered in place of the mark", dnsRow[8])
	}
	if dnsRow[9] != "" {
		t.Errorf("Dana's 60m points = %q, want empty (DNS scores no points)", dnsRow[9])
	}
}

// TestSeriesUploadExportSYS077UC035_3 covers UC-035 #3 (SYS-077): swapping
// the series-upload template document — a new season's revision — changes
// the exported workbook's sheet name and columns with no code change (the
// data-swap test, mirroring TestUC033_5_ScoringTableSwapAtServiceLevel for
// SYS-053).
func TestSeriesUploadExportSYS077UC035_3(t *testing.T) {
	meets, results, st := newTestResults(t)
	rec := createUKCMeet(t, meets)
	anna := register(t, results, rec.ID, ParticipantInput{
		FirstName: "Anna", LastName: "Muster", BirthYear: 2014, Sex: domain.SexFemale, Bib: "1",
	})
	save(t, results, rec.ID, ResultInput{AthleteID: anna.AthleteID, DisciplineCode: "60m", Mark: "8.42", Timing: domain.TimingElectronic})

	revised, err := domain.ParseSeriesUploadTemplate([]byte(`{
		"id": "ubs-kids-cup", "version": "2027-revised", "name": "revised template",
		"sheetName": "Meldebogen2027",
		"identityColumns": [
			{"field": "bib", "header": "Startnummer"},
			{"field": "lastName", "header": "Nachname"}
		],
		"disciplineMarkHeader": "Leistung %s",
		"disciplinePointsHeader": "Pkt %s",
		"trailingColumns": [{"field": "total", "header": "Gesamtpunkte"}],
		"missingMarkPlaceholder": "n/a"
	}`))
	if err != nil {
		t.Fatalf("ParseSeriesUploadTemplate: %v", err)
	}

	schemes, err := domain.BuiltinCategorySchemes()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := domain.BuiltinDisciplineCatalog()
	if err != nil {
		t.Fatal(err)
	}
	tables, err := domain.BuiltinScoringTables()
	if err != nil {
		t.Fatal(err)
	}
	templates, err := domain.BuiltinMeetTemplates()
	if err != nil {
		t.Fatal(err)
	}
	swapped := NewResultsService(st.DB(), catalog, schemes, tables, templates)
	swapped.SetSeriesUploadTemplates(map[string]*domain.SeriesUploadTemplate{
		domain.SeriesUploadUBSKidsCup: revised,
	})

	f, _ := openSeriesUpload(t, swapped, rec.ID)
	sheets := f.GetSheetList()
	if len(sheets) != 1 || sheets[0] != "Meldebogen2027" {
		t.Fatalf("sheets = %v, want [Meldebogen2027] from the revised template (no code change)", sheets)
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	wantHeader := []string{"Startnummer", "Nachname", "Leistung 60 metres", "Pkt 60 metres",
		"Leistung Zone Long Jump (UKC)", "Pkt Zone Long Jump (UKC)",
		"Leistung 200 g Ball Throw (UKC)", "Pkt 200 g Ball Throw (UKC)", "Gesamtpunkte"}
	if len(rows) < 1 || len(rows[0]) != len(wantHeader) {
		t.Fatalf("header row = %v, want %v", rows[0], wantHeader)
	}
	for i, want := range wantHeader {
		if rows[0][i] != want {
			t.Errorf("header col %d = %q, want %q", i, rows[0][i], want)
		}
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (header + Anna)", len(rows))
	}
	if rows[1][0] != "1" || rows[1][1] != "Muster" {
		t.Errorf("Anna row = %v, want bib 1 / Muster in the revised column order", rows[1])
	}
	// The missing ZoneLJ/BallThrow200g marks use the revised placeholder.
	if rows[1][4] != "n/a" {
		t.Errorf("missing ZoneLJ mark = %q, want revised placeholder n/a", rows[1][4])
	}
}

// TestSeriesUploadExportNotConfigured: a meet whose template names no
// series-upload template (or whose service has none wired) reports a clear
// error rather than an empty/garbled export.
func TestSeriesUploadExportNotConfigured(t *testing.T) {
	meets, _, st := newTestResults(t)
	rec := createUKCMeet(t, meets)

	schemes, err := domain.BuiltinCategorySchemes()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := domain.BuiltinDisciplineCatalog()
	if err != nil {
		t.Fatal(err)
	}
	tables, err := domain.BuiltinScoringTables()
	if err != nil {
		t.Fatal(err)
	}
	templates, err := domain.BuiltinMeetTemplates()
	if err != nil {
		t.Fatal(err)
	}
	unwired := NewResultsService(st.DB(), catalog, schemes, tables, templates)
	_, _, err = unwired.SeriesUploadExport(context.Background(), rec.ID)
	if err == nil {
		t.Error("SeriesUploadExport with no series-upload templates wired: want error, got nil")
	}
}
