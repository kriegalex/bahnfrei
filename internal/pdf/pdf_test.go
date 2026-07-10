// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package pdf

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	extract "github.com/ledongthuc/pdf"
)

// extractText renders doc, parses the resulting bytes back with an
// independent PDF library, and returns the plain text — so tests assert on
// real PDF structure/content rather than merely non-empty bytes.
func extractText(t *testing.T, doc Document) string {
	t.Helper()
	data, err := Build(doc)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatalf("output does not start with a PDF header: %q", data[:min(16, len(data))])
	}
	r, err := extract.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("parse generated pdf: %v", err)
	}
	rd, err := r.GetPlainText()
	if err != nil {
		t.Fatalf("extract plain text: %v", err)
	}
	text, err := io.ReadAll(rd)
	if err != nil {
		t.Fatalf("read extracted text: %v", err)
	}
	return string(text)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// fixedTime is a stable timestamp for assertions on the generation stamp.
var fixedTime = time.Date(2026, 7, 10, 14, 30, 0, 0, time.UTC)

// TestBuildCaptureSheetGridSYS072 exercises the horizontal-attempt grid
// shape a field capture sheet uses: a header (meet, document title,
// discipline subtitle, generation timestamp) plus one table with bib/name
// columns, per-trial columns, and a not-yet-captured (blank) cell — the
// document must "match the current data" (UC-018 #1) including gaps.
func TestBuildCaptureSheetGridSYS072(t *testing.T) {
	doc := Document{
		Header: Header{
			DocTitle:         "Capture sheet",
			MeetName:         "UBS Kids Cup Le Mouret 2026",
			Subtitle:         "Zone Long Jump (UKC)",
			GeneratedAtLabel: "Generated",
			GeneratedAt:      fixedTime,
		},
		Sections: []Section{
			{
				Columns: []Column{
					{Header: "Bib", Weight: 1},
					{Header: "Name", Weight: 3},
					{Header: "T1", Weight: 1},
					{Header: "T2", Weight: 1},
					{Header: "T3", Weight: 1},
				},
				Rows: [][]string{
					{"101", "Anna Muster", "4.12", "X", ""},
					{"102", "Bea Beispiel", "", "", ""},
				},
			},
		},
	}

	text := extractText(t, doc)
	for _, want := range []string{
		"UBS Kids Cup Le Mouret 2026", "Capture sheet", "Zone Long Jump (UKC)",
		"Generated: 2026-07-10 14:30",
		"Bib", "Name", "T1", "T2", "T3",
		"101", "Anna Muster", "4.12",
		"102", "Bea Beispiel",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("extracted text misses %q\n--- full text ---\n%s", want, text)
		}
	}
}

// TestBuildTrackLaneSheetSYS072 exercises the track lane-sheet shape: bib
// and name populated, lane/time columns left blank for manual capture.
func TestBuildTrackLaneSheetSYS072(t *testing.T) {
	doc := Document{
		Header: Header{
			DocTitle:         "Capture sheet",
			MeetName:         "UBS Kids Cup Le Mouret 2026",
			Subtitle:         "60 m",
			GeneratedAtLabel: "Generated",
			GeneratedAt:      fixedTime,
		},
		Sections: []Section{
			{
				Columns: []Column{
					{Header: "Lane", Weight: 1},
					{Header: "Bib", Weight: 1},
					{Header: "Name", Weight: 3},
					{Header: "Time", Weight: 1},
				},
				Rows: [][]string{
					{"", "101", "Anna Muster", ""},
					{"", "102", "Bea Beispiel", ""},
				},
			},
		},
	}

	text := extractText(t, doc)
	for _, want := range []string{"60 m", "Lane", "Bib", "Name", "Time", "101", "Anna Muster", "102", "Bea Beispiel"} {
		if !strings.Contains(text, want) {
			t.Errorf("extracted text misses %q\n--- full text ---\n%s", want, text)
		}
	}
}

// TestBuildResultListMultiDivisionSYS072 exercises the UKC result-list
// shape: multiple division sections in one document, each with its own
// heading, rank/points columns and an explicit gap marker for a missing
// discipline (UC-033 #3 presentation, reused unchanged by SYS-072's result
// list per UC-018 #2).
func TestBuildResultListMultiDivisionSYS072(t *testing.T) {
	doc := Document{
		Header: Header{
			DocTitle:         "Result list",
			MeetName:         "UBS Kids Cup Le Mouret 2026",
			GeneratedAtLabel: "Generated",
			GeneratedAt:      fixedTime,
		},
		Sections: []Section{
			{
				Heading: "W12",
				Columns: []Column{
					{Header: "Rank", Weight: 1},
					{Header: "Bib", Weight: 1},
					{Header: "Name", Weight: 3},
					{Header: "Total", Weight: 1},
				},
				Rows: [][]string{
					{"1", "101", "Anna Muster", "742"},
					{"2", "102", "Bea Beispiel", "–"},
				},
			},
			{
				Heading: "M12",
				Columns: []Column{
					{Header: "Rank", Weight: 1},
					{Header: "Bib", Weight: 1},
					{Header: "Name", Weight: 3},
					{Header: "Total", Weight: 1},
				},
				Rows: [][]string{
					{"1", "201", "Carlo Test", "801"},
				},
			},
		},
	}

	text := extractText(t, doc)
	for _, want := range []string{
		"Result list", "W12", "M12",
		"Anna Muster", "742", "Bea Beispiel",
		"Carlo Test", "801",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("extracted text misses %q\n--- full text ---\n%s", want, text)
		}
	}
}

// TestBuildEmptySectionRendersHeaderOnly ensures a document with no rows
// (e.g. an empty division) still renders a valid, parseable PDF with its
// header — never an error or empty output.
func TestBuildEmptySectionRendersHeaderOnly(t *testing.T) {
	doc := Document{
		Header: Header{
			DocTitle:         "Result list",
			MeetName:         "Empty Meet",
			GeneratedAtLabel: "Generated",
			GeneratedAt:      fixedTime,
		},
	}
	text := extractText(t, doc)
	if !strings.Contains(text, "Empty Meet") {
		t.Errorf("extracted text misses meet name: %q", text)
	}
}

// TestBuildAccentedTextRoundTrips proves the cp1252 translation used for
// DE/FR diacritics does not error or drop content (UC-018 #3: French
// document headings).
func TestBuildAccentedTextRoundTrips(t *testing.T) {
	doc := Document{
		Header: Header{
			DocTitle:         "Liste des résultats",
			MeetName:         "Réunion d'athlétisme",
			GeneratedAtLabel: "Généré",
			GeneratedAt:      fixedTime,
		},
		Sections: []Section{{
			Columns: []Column{{Header: "Athlète", Weight: 1}},
			Rows:    [][]string{{"Müller"}},
		}},
	}
	data, err := Build(doc)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Build returned empty output")
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatalf("output does not start with a PDF header")
	}
}
