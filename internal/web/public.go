// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"net/http"
	"strconv"

	"github.com/kriegalex/bahnfrei/internal/domain"
)

// --- public meet surface (SYS-070/071/074/076, UC-017): unauthenticated,
// stable /m/{id}/... URLs that keep serving a meet's archived state after
// it closes. handlePublicTimetable (meets.go) predates this file; the
// handlers here round out the overview/start-lists/results pages and the
// live-results refresh fragment. ---

// publicMeetView is the public overview page's view model.
type publicMeetView struct {
	MeetID   string
	MeetName string
	Venue    string
	Dates    string
	Status   string
}

// handlePublicMeet serves the meet overview (UC-017 #2/#3): name, venue,
// dates, links to the other public surfaces.
func (s *Server) handlePublicMeet(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	d, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = d.Name
	view := publicMeetView{
		MeetID:   d.ID,
		MeetName: d.Name,
		Venue:    d.Venue,
		Dates:    formatDateRange(p, d.StartDate, d.EndDate),
		Status:   p.T("meet.status." + string(d.Status)),
	}
	_ = publicMeetPage(p, view).Render(r.Context(), w)
}

// publicStartListRowView is one participant line on the public start-list
// page — the same shape as the office roster (rosterRowView), reused
// unauthenticated.
type publicStartListRowView struct {
	Bib, Name, BirthYear, Club string
}

type publicStartListsView struct {
	MeetID   string
	MeetName string
	Rows     []publicStartListRowView
}

// handlePublicStartLists serves the meet's participants (UC-017 #2):
// currently the live roster (this PoC has no separate start-list
// publication step distinct from the roster — see
// docs/requirements/open-questions-and-assumptions.md).
func (s *Server) handlePublicStartLists(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	d, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	participants, err := s.results.Participants(r.Context(), meetID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	clubs, err := s.results.ClubNamesFor(r.Context(), participants)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = d.Name + " — " + p.T("public.startlists.title")
	view := publicStartListsView{MeetID: d.ID, MeetName: d.Name}
	for _, part := range participants {
		club := ""
		if len(part.Athlete.ClubIDs) > 0 {
			club = clubs[part.Athlete.ClubIDs[0]]
		}
		view.Rows = append(view.Rows, publicStartListRowView{
			Bib:       part.Bib,
			Name:      part.Athlete.FirstName + " " + part.Athlete.LastName,
			BirthYear: strconv.Itoa(part.Athlete.BirthYear),
			Club:      club,
		})
	}
	_ = publicStartListsPage(p, view).Render(r.Context(), w)
}

// publicResultsView is the public results page/fragment's view model: the
// division standings (reusing standings.go's row/cell shapes) plus the
// SYS-076 unofficial-results labeling and a meet-state status line.
type publicResultsView struct {
	MeetID              string
	MeetName            string
	Venue               string
	Dates               string
	StatusLine          string
	ShowUnofficialLabel bool
	OfficialSourceName  string
	OfficialSourceURL   string
	Disciplines         []string
	Divisions           []divisionView
}

// buildPublicResultsView assembles the results view shared by the full
// page (handlePublicResults) and the SSE-refreshed fragment
// (handlePublicResultsLive) — one code path for both, so a live update can
// never render anything the full page would not.
func (s *Server) buildPublicResultsView(r *http.Request, meetID string) (publicResultsView, error) {
	d, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		return publicResultsView{}, err
	}
	standings, err := s.results.Standings(r.Context(), meetID)
	if err != nil {
		return publicResultsView{}, err
	}
	p := basePageData(r, s.cats)

	v := publicResultsView{
		MeetID:     d.ID,
		MeetName:   d.Name,
		Venue:      d.Venue,
		Dates:      formatDateRange(p, d.StartDate, d.EndDate),
		StatusLine: p.T("meet.status." + string(d.Status)),
	}
	// SYS-076: anything other than an explicit "primary" positioning
	// carries the unofficial-results label — the conservative default.
	if d.ResultsPositioning != domain.ResultsPositioningPrimary {
		v.ShowUnofficialLabel = true
		v.OfficialSourceName = d.OfficialSourceName
		v.OfficialSourceURL = d.OfficialSourceURL
		v.StatusLine += " · " + p.T("public.results.unofficial_badge")
	}
	for _, code := range standings.Disciplines {
		v.Disciplines = append(v.Disciplines, s.localizedDisciplineName(p, code)) // SYS-074
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
	return v, nil
}

// handlePublicResults serves the full public results page (UC-017 #2).
func (s *Server) handlePublicResults(w http.ResponseWriter, r *http.Request) {
	v, err := s.buildPublicResultsView(r, r.PathValue("id"))
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = v.MeetName + " — " + p.T("public.results.title")
	_ = publicResultsPage(p, v).Render(r.Context(), w)
}

// handlePublicResultsLive serves just the results fragment the public
// results page's live-refresh island swaps in on the meet's "results" SSE
// event (UC-017 #1, SYS-071): a fresh fetch of this endpoint is what
// proves a confirmed result reaches an already-open public page.
func (s *Server) handlePublicResultsLive(w http.ResponseWriter, r *http.Request) {
	v, err := s.buildPublicResultsView(r, r.PathValue("id"))
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	_ = publicResultsFragment(p, v).Render(r.Context(), w)
}
