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
)

// --- create meet from template (SYS-053, UC-033 #1) ---

type templateOption struct {
	ID   string
	Name string
}

type templateFormView struct {
	Templates  []templateOption
	TemplateID string
	Name       string
	Venue      string
	Date       string
}

func (s *Server) templateForm() templateFormView {
	f := templateFormView{Date: time.Now().Format(formDateLayout)}
	for _, t := range s.meets.MeetTemplates() {
		f.Templates = append(f.Templates, templateOption{ID: t.ID, Name: t.Name})
	}
	return f
}

func (s *Server) handleTemplateMeetForm(w http.ResponseWriter, r *http.Request) {
	p := basePageData(r, s.cats)
	p.Title = p.T("meet.template.title")
	_ = templateMeetPage(p, s.templateForm()).Render(r.Context(), w)
}

func (s *Server) handleTemplateMeetCreate(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	f := s.templateForm()
	f.TemplateID = r.FormValue("template")
	f.Name = strings.TrimSpace(r.FormValue("name"))
	f.Venue = strings.TrimSpace(r.FormValue("venue"))
	f.Date = r.FormValue("date")

	renderErr := func(status int, key string) {
		p := basePageData(r, s.cats)
		p.Title = p.T("meet.template.title")
		p.FlashError = p.T(key)
		w.WriteHeader(status)
		_ = templateMeetPage(p, f).Render(r.Context(), w)
	}
	date, err := time.Parse(formDateLayout, f.Date)
	if err != nil || f.Venue == "" {
		renderErr(http.StatusUnprocessableEntity, "meet.template.error.invalid")
		return
	}
	rec, err := s.meets.CreateMeetFromTemplate(r.Context(), actor, app.TemplateMeetRequest{
		TemplateID: f.TemplateID,
		Name:       f.Name,
		Venue:      f.Venue,
		Date:       date,
	})
	if err != nil {
		renderErr(http.StatusUnprocessableEntity, "meet.template.error.invalid")
		return
	}
	http.Redirect(w, r, "/meets/"+rec.ID, http.StatusSeeOther)
}

// --- participants roster (C7.3 participant list; feeds UC-033 #2) ---

type rosterRowView struct {
	Bib       string
	Name      string
	BirthYear string
	Club      string
}

type rosterView struct {
	MeetID   string
	MeetName string
	Rows     []rosterRowView
}

func (s *Server) rosterView(r *http.Request, meetID string) (rosterView, error) {
	detail, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		return rosterView{}, err
	}
	v := rosterView{MeetID: detail.ID, MeetName: detail.Name}
	participants, err := s.results.Participants(r.Context(), meetID)
	if err != nil {
		return rosterView{}, err
	}
	clubs, err := s.results.ClubNamesFor(r.Context(), participants)
	if err != nil {
		return rosterView{}, err
	}
	for _, p := range participants {
		club := ""
		if len(p.Athlete.ClubIDs) > 0 {
			club = clubs[p.Athlete.ClubIDs[0]]
		}
		v.Rows = append(v.Rows, rosterRowView{
			Bib:       p.Bib,
			Name:      p.Athlete.FirstName + " " + p.Athlete.LastName,
			BirthYear: strconv.Itoa(p.Athlete.BirthYear),
			Club:      club,
		})
	}
	return v, nil
}

func (s *Server) handleRoster(w http.ResponseWriter, r *http.Request) {
	v, err := s.rosterView(r, r.PathValue("id"))
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = v.MeetName + " — " + p.T("roster.title")
	_ = rosterPage(p, v).Render(r.Context(), w)
}

func (s *Server) handleRosterAdd(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	birthYear, _ := strconv.Atoi(r.FormValue("birth_year"))
	in := app.ParticipantInput{
		FirstName: strings.TrimSpace(r.FormValue("first_name")),
		LastName:  strings.TrimSpace(r.FormValue("last_name")),
		BirthYear: birthYear,
		Sex:       domain.Sex(r.FormValue("sex")),
		Club:      strings.TrimSpace(r.FormValue("club")),
		Bib:       strings.TrimSpace(r.FormValue("bib")),
	}
	if _, err := s.results.RegisterParticipant(r.Context(), actor, meetID, in); err != nil {
		v, verr := s.rosterView(r, meetID)
		if verr != nil {
			s.renderMeetError(w, r, verr)
			return
		}
		p := basePageData(r, s.cats)
		p.Title = v.MeetName + " — " + p.T("roster.title")
		switch {
		case errors.Is(err, app.ErrDuplicateParticipant):
			p.FlashError = p.T("roster.error.duplicate")
		default:
			p.FlashError = p.T("roster.error.invalid")
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = rosterPage(p, v).Render(r.Context(), w)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/roster", http.StatusSeeOther)
}

// --- division standings (UC-033 #2–#4, SYS-052/053) ---

type markCellView struct {
	Mark   string
	Points string
}

type standingRowView struct {
	Rank      string
	Bib       string
	Name      string
	Club      string
	BirthYear string
	Cells     []markCellView
	Total     string
}

type divisionView struct {
	Code string
	Rows []standingRowView
}

type standingsView struct {
	MeetID                string
	MeetName              string
	Disciplines           []string
	Divisions             []divisionView
	SeriesUploadAvailable bool
}

func (s *Server) handleStandings(w http.ResponseWriter, r *http.Request) {
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
	v := standingsView{
		MeetID:                detail.ID,
		MeetName:              detail.Name,
		SeriesUploadAvailable: s.results.SeriesUploadAvailable(r.Context(), meetID),
	}
	for _, code := range standings.Disciplines {
		v.Disciplines = append(v.Disciplines, s.localizedDisciplineName(p, code))
	}
	for _, div := range standings.Divisions {
		dv := divisionView{Code: div.CategoryCode}
		for _, row := range div.Rows {
			rv := standingRowView{
				Rank:      strconv.Itoa(row.Rank),
				Bib:       row.Bib,
				Name:      row.FirstName + " " + row.LastName,
				Club:      row.ClubName,
				BirthYear: strconv.Itoa(row.BirthYear),
				Total:     strconv.Itoa(row.Total),
			}
			for _, m := range row.Marks {
				cell := markCellView{Mark: markOrGap(m)}
				if m.Points != nil {
					cell.Points = strconv.Itoa(*m.Points)
				}
				rv.Cells = append(rv.Cells, cell)
			}
			dv.Rows = append(dv.Rows, rv)
		}
		v.Divisions = append(v.Divisions, dv)
	}
	p.Title = v.MeetName + " — " + p.T("standings.title")
	_ = standingsPage(p, v).Render(r.Context(), w)
}

// handleSeriesUploadExport serves the SYS-077 series results-upload
// workbook (UC-035 #1–#3) as an XLSX download. A meet whose template names
// no series-upload template (or an unconfigured service) renders the
// standings-page 404, matching the roster/standings not-found handling.
func (s *Server) handleSeriesUploadExport(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	data, filename, err := s.results.SeriesUploadExport(r.Context(), meetID)
	if err != nil {
		if errors.Is(err, app.ErrNoSeriesUploadTemplate) {
			s.handleNotFound(w, r)
			return
		}
		s.renderMeetError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

// markOrGap renders a performance cell: the mark, the status code (DNS,
// NM, …), or the explicit gap marker for a missing discipline (UC-033 #3).
func markOrGap(m domain.CombinedPerformance) string {
	switch {
	case m.Status != domain.StatusNone:
		return string(m.Status)
	case m.Mark != "":
		return m.Mark
	default:
		return "–"
	}
}

// renderMeetError maps app-layer errors onto the workspace's error pages.
func (s *Server) renderMeetError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, app.ErrMeetNotFound) {
		s.handleNotFound(w, r)
		return
	}
	http.Error(w, "internal error", http.StatusInternalServerError)
}
