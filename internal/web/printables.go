// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/pdf"
)

// --- printables (TASK-011, UC-018 subset, SYS-072: capture sheets + UKC
// result lists as PDF) ---

// writePDF sends data as a downloadable PDF response. "inline" lets the
// browser preview it (an office workstation's usual expectation) while
// still naming the file for a manual save.
func writePDF(w http.ResponseWriter, data []byte, filename string) {
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="`+filename+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handleCaptureSheetPDF renders a unit's capture sheet: a horizontal-attempt
// grid for field events, a lane sheet for track (UC-018 #1). The sheet
// reflects whatever has already been captured — an empty cell is either a
// gap for hand-writing (paper backup) or a trial not yet run, matching the
// same current-data snapshot the on-screen capture grid renders.
func (s *Server) handleCaptureSheetPDF(w http.ResponseWriter, r *http.Request) {
	meetID, unitID := r.PathValue("id"), r.PathValue("unit")
	detail, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	uc, err := s.results.UnitCapture(r.Context(), meetID, unitID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	data, err := pdf.Build(s.captureSheetDocument(p, detail.Name, uc))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writePDF(w, data, "capture-sheet-"+unitID+".pdf")
}

// captureSheetDocument assembles the printable pdf.Document from a unit's
// capture view: bib/name plus per-trial columns for field-horizontal
// events (SYS-042 series shape), or bib/name plus blank lane/time columns
// for track — lane draws are not yet modeled (TASK-018, M2), so the lane
// column is left for the official to fill by hand, same as the timing
// column (documented PoC limitation).
func (s *Server) captureSheetDocument(p PageData, meetName string, uc app.UnitCaptureView) pdf.Document {
	header := pdf.Header{
		DocTitle:         p.T("pdf.capture_sheet.title"),
		MeetName:         meetName,
		Subtitle:         s.localizedDisciplineName(p, uc.DisciplineCode),
		GeneratedAtLabel: p.T("pdf.generated"),
		GeneratedAt:      time.Now(),
	}

	var section pdf.Section
	if uc.Family == domain.FamilyFieldHorizontal {
		cols := []pdf.Column{
			{Header: p.T("roster.bib"), Weight: 1},
			{Header: p.T("roster.name"), Weight: 3},
		}
		for i := 1; i <= uc.Config.Attempts; i++ {
			cols = append(cols, pdf.Column{Header: p.T("capture.trial_abbr") + intToStr(int64(i)), Weight: 1})
		}
		rows := make([][]string, 0, len(uc.Rows))
		for _, row := range uc.Rows {
			cells := make([]string, 0, len(cols))
			cells = append(cells, row.Bib, row.FirstName+" "+row.LastName)
			for _, a := range row.Attempts {
				display := ""
				if a != nil {
					display = a.Display()
				}
				cells = append(cells, display)
			}
			rows = append(rows, cells)
		}
		section = pdf.Section{Columns: cols, Rows: rows}
	} else {
		cols := []pdf.Column{
			{Header: p.T("pdf.lane"), Weight: 1},
			{Header: p.T("roster.bib"), Weight: 1},
			{Header: p.T("roster.name"), Weight: 3},
			{Header: p.T("capture.time"), Weight: 1},
		}
		rows := make([][]string, 0, len(uc.Rows))
		for _, row := range uc.Rows {
			rows = append(rows, []string{"", row.Bib, row.FirstName + " " + row.LastName, ""})
		}
		section = pdf.Section{Columns: cols, Rows: rows}
	}
	return pdf.Document{Header: header, Sections: []pdf.Section{section}}
}

// handleResultListPDF renders the meet's UKC result list: every division's
// combined standings as its own table, in the same rank/bib/name/club/
// birth-year/marks/total shape as the standings page (UC-018 #2, reusing
// TASK-007's division-standings computation unchanged).
func (s *Server) handleResultListPDF(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	detail, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	standings, err := s.results.Standings(r.Context(), meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	data, err := pdf.Build(s.resultListDocument(p, detail.Name, standings))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writePDF(w, data, "result-list.pdf")
}

// resultListDocument assembles the printable pdf.Document from the meet's
// standings: discipline column headers are localized the same way the
// public results page localizes them (UC-018 #3, SYS-074's mechanism), and
// a missing discipline renders the same explicit gap the standings page
// shows (UC-033 #3) via markOrGap. A printed result list is a
// "publication-intended export" per SYS-100's own wording (it is meant to
// be posted at the venue) — so it goes through the same SYS-103 consent
// minimization as the public web pages (domain.PublicDisplayNameFor/
// PublicDisplayClubFor), unlike the office-only standings page
// (handleStandings) or the series-upload federation export (which UC-023
// #2 explicitly keeps un-minimized: "exportable to the federation").
func (s *Server) resultListDocument(p PageData, meetName string, standings app.MeetStandings) pdf.Document {
	header := pdf.Header{
		DocTitle:         p.T("standings.title"),
		MeetName:         meetName,
		GeneratedAtLabel: p.T("pdf.generated"),
		GeneratedAt:      time.Now(),
	}

	disciplineNames := make([]string, len(standings.Disciplines))
	for i, code := range standings.Disciplines {
		disciplineNames[i] = s.localizedDisciplineName(p, code)
	}

	sections := make([]pdf.Section, 0, len(standings.Divisions))
	for _, div := range standings.Divisions {
		cols := []pdf.Column{
			{Header: p.T("standings.rank"), Weight: 1},
			{Header: p.T("roster.bib"), Weight: 1},
			{Header: p.T("roster.name"), Weight: 3},
			{Header: p.T("roster.club"), Weight: 2},
			{Header: p.T("roster.birth_year"), Weight: 1},
		}
		for _, name := range disciplineNames {
			cols = append(cols, pdf.Column{Header: name, Weight: 1.5})
		}
		cols = append(cols, pdf.Column{Header: p.T("standings.total"), Weight: 1})

		rows := make([][]string, 0, len(div.Rows))
		for _, row := range div.Rows {
			first, last := domain.PublicDisplayNameFor(row.Consent, row.FirstName, row.LastName)
			club := domain.PublicDisplayClubFor(row.Consent, row.ClubName)
			cells := []string{
				strconv.Itoa(row.Rank), row.Bib, strings.TrimSpace(first + " " + last),
				club, strconv.Itoa(row.BirthYear),
			}
			for _, m := range row.Marks {
				cell := markOrGap(m)
				if m.Points != nil {
					cell += " (" + strconv.Itoa(*m.Points) + ")"
				}
				cells = append(cells, cell)
			}
			cells = append(cells, strconv.Itoa(row.Total))
			rows = append(rows, cells)
		}
		sections = append(sections, pdf.Section{Heading: div.CategoryCode, Columns: cols, Rows: rows})
	}
	return pdf.Document{Header: header, Sections: sections}
}
