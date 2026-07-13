// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/domain"
	"github.com/kriegalex/bahnfrei/internal/pdf"
)

// --- vertical jump capture (TASK-021: UC-012, SYS-043) ---
//
// A separate page/view from the horizontal-attempt grid and track form
// (capture.go/capture.templ): heights replace trial-count as the series
// shape, and the SYS-043 countback ranking replaces next-best-mark/time
// ordering. Wired into the shared /meets/{id}/capture/{unit} route by a
// small family branch in handleCaptureUnit/handleCaptureStandings
// (capture.go) rather than new GET routes, so the capture index's links
// need no change. Online-only for M2: the offline capture island
// (capture-offline.js) is scoped to horizontal-attempt capture only — see
// OQ-046 in open-questions-and-assumptions.md.

type verticalCellView struct {
	Value   string // "o", "x", "-", "r", or "" (not yet captured)
	Version string
}

type verticalRowView struct {
	AthleteID string
	Bib       string
	Name      string
	// Cells[heightIdx][seq-1]
	Cells  [][3]verticalCellView
	Result string
	Points string
}

type verticalStandingRowView struct {
	Rank           string
	Name           string
	Bib            string
	BestHeight     string
	AttemptsAtBest string
	TotalFailures  string
	Flags          string // localized "eliminated"/"retired"/"tie for first", space-joined
	Points         string
}

type verticalCaptureView struct {
	MeetID         string
	MeetName       string
	UnitID         string
	Discipline     string
	Heights        []string
	NextHeightHint string // a suggested next height for the "add a height" form
	Rows           []verticalRowView
	Standings      []verticalStandingRowView
	CanOffice      bool
}

func (s *Server) verticalCaptureView(r *http.Request, meetID, unitID string) (verticalCaptureView, error) {
	uc, err := s.results.VerticalCapture(r.Context(), meetID, unitID)
	if err != nil {
		return verticalCaptureView{}, err
	}
	p := basePageData(r, s.cats)
	v := verticalCaptureView{
		MeetID:     uc.Meet.ID,
		MeetName:   uc.Meet.Name,
		UnitID:     unitID,
		Discipline: s.localizedDisciplineName(p, uc.DisciplineCode),
		Heights:    uc.Heights,
	}
	if actor, ok := sessionFromContext(r.Context()); ok {
		v.CanOffice = actor.Role.AtLeast(app.RoleCompetitionOffice)
	}
	if len(uc.Heights) > 0 {
		v.NextHeightHint = nextHeightHint(uc.Heights[len(uc.Heights)-1], uc.DisciplineCode)
	}

	names := map[string]string{}
	bibs := map[string]string{}
	for _, row := range uc.Rows {
		names[row.AthleteID] = row.FirstName + " " + row.LastName
		bibs[row.AthleteID] = row.Bib
		rv := verticalRowView{
			AthleteID: row.AthleteID,
			Bib:       row.Bib,
			Name:      row.FirstName + " " + row.LastName,
			Cells:     make([][3]verticalCellView, len(row.Cells)),
		}
		for hi, height := range row.Cells {
			for si, cell := range height {
				if cell.Kind == "" {
					rv.Cells[hi][si] = verticalCellView{Version: "0"}
					continue
				}
				rv.Cells[hi][si] = verticalCellView{
					Value:   verticalDisplay(cell.Kind),
					Version: strconv.FormatInt(cell.Version, 10),
				}
			}
		}
		if res := row.Result; res != nil {
			rv.Result = res.Mark
			if res.Status != domain.StatusNone {
				rv.Result = strings.TrimSpace(rv.Result + " " + domain.RenderStatus(res.Status, res.StatusDetail))
			}
			if res.Points != nil {
				rv.Points = strconv.Itoa(*res.Points)
			}
		}
		v.Rows = append(v.Rows, rv)
	}

	for _, st := range uc.Standings {
		rv := verticalStandingRowView{
			Name:           names[st.AthleteID],
			Bib:            bibs[st.AthleteID],
			BestHeight:     st.BestHeight,
			AttemptsAtBest: strconv.Itoa(st.AttemptsAtBest),
			TotalFailures:  strconv.Itoa(st.TotalFailures),
		}
		if st.Rank > 0 {
			rv.Rank = strconv.Itoa(st.Rank)
		}
		if st.Points != nil {
			rv.Points = strconv.Itoa(*st.Points)
		}
		var flags []string
		if st.TieForFirst {
			flags = append(flags, p.T("capture.vertical.tie_for_first"))
		}
		if st.Retired {
			flags = append(flags, p.T("capture.vertical.retired"))
		} else if st.Eliminated {
			flags = append(flags, p.T("capture.vertical.eliminated"))
		}
		rv.Flags = strings.Join(flags, " · ")
		v.Standings = append(v.Standings, rv)
	}
	return v, nil
}

// verticalDisplay renders a stored trial kind as the grid's editable
// single-letter notation.
func verticalDisplay(k domain.QualificationStatus) string {
	switch k {
	case domain.StatusO:
		return "o"
	case domain.StatusX:
		return "x"
	case domain.StatusPass:
		return "-"
	case domain.StatusR:
		return "r"
	}
	return ""
}

// parseVerticalValue maps the grid's single-letter notation to a trial
// kind (D5.2: O clear, X foul, – pass, r retirement).
func parseVerticalValue(value string) (domain.QualificationStatus, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "o":
		return domain.StatusO, true
	case "x":
		return domain.StatusX, true
	case "-", "–":
		return domain.StatusPass, true
	case "r":
		return domain.StatusR, true
	default:
		return "", false
	}
}

// nextHeightHint suggests the next bar height for the "configure/extend
// heights" form: the last configured height plus the discipline's default
// WA increment (domain.DefaultHeightIncrementCM — 3 cm HJ, 10 cm PV),
// falling back to no suggestion for an unknown discipline code.
func nextHeightHint(lastHeight, disciplineCode string) string {
	inc, ok := domain.DefaultHeightIncrementCM[disciplineCode]
	if !ok {
		return ""
	}
	centi, err := domain.ParseCentiMark(lastHeight)
	if err != nil {
		return ""
	}
	return domain.FormatCentiMark(centi+int64(inc), 2)
}

func (s *Server) handleCaptureVerticalUnit(w http.ResponseWriter, r *http.Request) {
	meetID, unitID := r.PathValue("id"), r.PathValue("unit")
	v, err := s.verticalCaptureView(r, meetID, unitID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = v.MeetName + " — " + v.Discipline
	_ = verticalCapturePage(p, v).Render(r.Context(), w)
}

// handleCaptureVerticalStandings serves the standings fragment the
// vertical capture page's live refresh swaps in on SSE "results" events
// (mirrors handleCaptureStandings for horizontal/track).
func (s *Server) handleCaptureVerticalStandings(w http.ResponseWriter, r *http.Request) {
	meetID, unitID := r.PathValue("id"), r.PathValue("unit")
	v, err := s.verticalCaptureView(r, meetID, unitID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	_ = verticalCaptureStandings(p, v).Render(r.Context(), w)
}

func (s *Server) handleCaptureVerticalTrial(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID, unitID := r.PathValue("id"), r.PathValue("unit")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	kind, ok := parseVerticalValue(r.FormValue("value"))
	if !ok {
		s.renderVerticalCaptureError(w, r, meetID, unitID, "capture.error.invalid", "")
		return
	}
	heightIdx, _ := strconv.Atoi(r.FormValue("height"))
	seq, _ := strconv.Atoi(r.FormValue("seq"))
	version, _ := strconv.ParseInt(r.FormValue("version"), 10, 64)
	in := app.VerticalTrialInput{
		AthleteID: r.FormValue("athlete"), HeightIdx: heightIdx, Seq: seq,
		Kind: kind, ExpectedVersion: version,
	}
	if _, err := s.results.SaveVerticalTrial(r.Context(), actor, meetID, unitID, in); err != nil {
		var conflict *app.VerticalTrialConflictError
		switch {
		case errors.Is(err, app.ErrUnitNotAssigned):
			renderForbidden(w, r, s.cats)
		case errors.Is(err, app.ErrCorrectionRequired):
			s.renderVerticalCaptureError(w, r, meetID, unitID, "capture.error.correction_required", "")
		case errors.As(err, &conflict):
			s.renderVerticalCaptureError(w, r, meetID, unitID, "capture.error.conflict", conflict.Current.Display())
		default:
			s.renderVerticalCaptureError(w, r, meetID, unitID, "capture.error.invalid", "")
		}
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/capture/"+unitID, http.StatusSeeOther)
}

// handleCaptureVerticalHeights configures (first time) or extends
// (jump-off) a unit's bar-height progression — office-only (SYS-043).
func (s *Server) handleCaptureVerticalHeights(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID, unitID := r.PathValue("id"), r.PathValue("unit")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var heights []string
	// Each "heights" value may itself be a comma-separated series: the
	// first-time configuration form is a single text input the operator
	// fills as "1.60, 1.65, 1.70" (its placeholder and visible hint text
	// say exactly that — TASK-031/SYS-117), while the extend form posts
	// one hidden input per already-configured height.
	for _, raw := range r.Form["heights"] {
		for _, part := range strings.Split(raw, ",") {
			if h := strings.TrimSpace(part); h != "" {
				heights = append(heights, h)
			}
		}
	}
	if add := strings.TrimSpace(r.FormValue("add_height")); add != "" {
		heights = append(heights, add)
	}
	version, _ := strconv.ParseInt(r.FormValue("version"), 10, 64)
	if _, err := s.results.SetVerticalHeights(r.Context(), actor, meetID, unitID, app.VerticalHeightsInput{
		Heights: heights, ExpectedVersion: version,
	}); err != nil {
		if _, forbidden := err.(app.ErrForbidden); forbidden {
			renderForbidden(w, r, s.cats)
			return
		}
		s.renderVerticalCaptureError(w, r, meetID, unitID, "capture.error.invalid", "")
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/capture/"+unitID, http.StatusSeeOther)
}

func (s *Server) renderVerticalCaptureError(w http.ResponseWriter, r *http.Request, meetID, unitID, key, arg string) {
	v, err := s.verticalCaptureView(r, meetID, unitID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = v.MeetName + " — " + v.Discipline
	if arg != "" {
		p.FlashError = p.T(key, "stored", arg)
	} else {
		p.FlashError = p.T(key)
	}
	w.WriteHeader(http.StatusUnprocessableEntity)
	_ = verticalCapturePage(p, v).Render(r.Context(), w)
}

// handleVerticalCaptureSheetPDF renders a vertical-jump unit's capture
// sheet: bib/name plus one column per configured bar height (UC-018 #1:
// "vertical: height columns"), mirroring captureSheetDocument's shape for
// horizontal/track.
func (s *Server) handleVerticalCaptureSheetPDF(w http.ResponseWriter, r *http.Request, meetName string) {
	unitID := r.PathValue("unit")
	uc, err := s.results.VerticalCapture(r.Context(), r.PathValue("id"), unitID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	header := pdf.Header{
		DocTitle:         p.T("pdf.capture_sheet.title"),
		MeetName:         meetName,
		Subtitle:         s.localizedDisciplineName(p, uc.DisciplineCode),
		GeneratedAtLabel: p.T("pdf.generated"),
		GeneratedAt:      time.Now(),
	}
	cols := []pdf.Column{
		{Header: p.T("roster.bib"), Weight: 1},
		{Header: p.T("roster.name"), Weight: 3},
	}
	for _, h := range uc.Heights {
		cols = append(cols, pdf.Column{Header: h, Weight: 1})
	}
	rows := make([][]string, 0, len(uc.Rows))
	for _, row := range uc.Rows {
		cells := []string{row.Bib, row.FirstName + " " + row.LastName}
		for _, height := range row.Cells {
			var trial []string
			for _, cell := range height {
				if cell.Kind != "" {
					trial = append(trial, string(cell.Kind))
				}
			}
			cells = append(cells, strings.Join(trial, ""))
		}
		rows = append(rows, cells)
	}
	data, err := pdf.Build(pdf.Document{Header: header, Sections: []pdf.Section{{Columns: cols, Rows: rows}}})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writePDF(w, data, "capture-sheet-"+unitID+".pdf")
}
