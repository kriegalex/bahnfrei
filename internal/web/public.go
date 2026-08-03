// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"

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

// publicHeatRowView is one entry's line within a public heat/flight section
// (TASK-018, UC-008/009, SYS-026/027/029): lane and qualification code, once
// heat seeding/progression has run for that round.
type publicHeatRowView struct {
	Name, Club, Lane, Qualification string
}

type publicHeatUnitView struct {
	Label string
	Rows  []publicHeatRowView
}

type publicHeatRoundView struct {
	RoundLabel string
	Units      []publicHeatUnitView
}

type publicHeatEventView struct {
	DisciplineLabel string
	Categories      string
	Rounds          []publicHeatRoundView
}

type publicStartListsView struct {
	MeetID   string
	MeetName string
	Rows     []publicStartListRowView
	// HeatEvents lists every event whose heats/flights have been generated
	// (TASK-018): shown alongside the flat roster below, additively — an
	// event with no seeded round simply does not appear here.
	HeatEvents []publicHeatEventView
}

// handlePublicStartLists serves the meet's participants (UC-017 #2):
// the live roster (this PoC has no separate start-list publication step
// distinct from the roster — see
// docs/requirements/open-questions-and-assumptions.md, OQ-021), plus any
// generated heat/lane breakdown per event (UC-008/009).
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
		// SYS-100/SYS-103 (UC-023 #1/#2): the central minimization/consent
		// functions are the single choke point every public renderer must
		// call before showing an athlete's identity — never per-page ad-hoc
		// logic. This start-list row already carries no birth date (only
		// BirthYear), no licence number, no contact data (SYS-100); the
		// name/club may still need SYS-103 consent suppression.
		first, last := domain.PublicDisplayNameFor(part.Athlete.Consent, part.Athlete.FirstName, part.Athlete.LastName)
		club = domain.PublicDisplayClubFor(part.Athlete.Consent, club)
		view.Rows = append(view.Rows, publicStartListRowView{
			Bib:       part.Bib,
			Name:      strings.TrimSpace(first + " " + last),
			BirthYear: strconv.Itoa(part.Athlete.BirthYear),
			Club:      club,
		})
	}

	heatSheets, err := s.results.PublicHeatSheets(r.Context(), meetID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	for _, ev := range heatSheets {
		label := ev.DisciplineName
		if label == "" {
			label = ev.EventID
		}
		hev := publicHeatEventView{DisciplineLabel: label, Categories: strings.Join(ev.CategoryCodes, ", ")}
		for _, round := range ev.Rounds {
			hr := publicHeatRoundView{RoundLabel: p.T("round." + string(round.RoundKind))}
			for i, u := range round.Units {
				hu := publicHeatUnitView{Label: p.T("seeding.heat") + " " + strconv.Itoa(i+1)}
				for _, row := range u.Rows {
					lane := ""
					if row.Lane != 0 {
						lane = strconv.Itoa(row.Lane)
					}
					// SYS-100/SYS-103 (TASK-023, UC-023 #1/#2): heat/lane rows
					// are public athlete identity too — same central consent
					// choke point as the flat roster rows above. AthleteName is
					// already a joined display string, so it rides in the
					// first-name position with an empty last name.
					name, _ := domain.PublicDisplayNameFor(row.Consent, row.AthleteName, "")
					club := domain.PublicDisplayClubFor(row.Consent, row.ClubName)
					hu.Rows = append(hu.Rows, publicHeatRowView{
						Name: name, Club: club, Lane: lane, Qualification: string(row.Qualification),
					})
				}
				hr.Units = append(hr.Units, hu)
			}
			hev.Rounds = append(hev.Rounds, hr)
		}
		view.HeatEvents = append(view.HeatEvents, hev)
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
	// TASK-036/DEC-016: FINAL semantics (unranked-at-bottom, the 1-point
	// floor) once the division's series is complete, PROVISIONAL
	// otherwise — the public page never re-implements the completeness
	// check.
	standings, err := s.results.CurrentStandings(r.Context(), meetID)
	if err != nil {
		return publicResultsView{}, err
	}
	p := basePageData(r, s.cats)

	v := publicResultsView{
		MeetID:     d.ID,
		MeetName:   d.Name,
		Venue:      d.Venue,
		Dates:      formatDateRange(p, d.StartDate, d.EndDate),
		StatusLine: p.T("meet.status."+string(d.Status)) + " · " + standingsStatusLabel(p, standings.Final),
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
			// SYS-100/SYS-103, UC-023 #1/#2: the same central minimization/
			// consent functions handlePublicStartLists calls — a fresh
			// fetch of this same code path from the SSE-refreshed live
			// fragment (handlePublicResultsLive) can never diverge from the
			// full page, and a mid-meet consent change (UC-023 #3) is
			// picked up on the very next request since nothing here caches
			// consent state.
			first, last := domain.PublicDisplayNameFor(row.Consent, row.FirstName, row.LastName)
			club := domain.PublicDisplayClubFor(row.Consent, row.ClubName)
			rv := standingRowView{
				Rank:      rankLabel(row),
				Bib:       row.Bib,
				Name:      strings.TrimSpace(first + " " + last),
				Club:      club,
				BirthYear: strconv.Itoa(row.BirthYear),
				Total:     totalLabel(p, row),
			}
			for _, m := range row.Marks {
				cell := markCellView{Mark: markOrGap(m), Flags: strings.Join(m.RecordFlags, ", ")}
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

// cachedPublicResults resolves meetID's public-results view AND its
// pre-rendered results-fragment HTML through the per-meet render cache
// (ADR-004 read-path amendment, TASK-035/OQ-066), building both together
// on a miss — shared by the full page and the live-refresh fragment (see
// buildPublicResultsView's own doc comment: one code path for both
// remains true, only the result may now be reused across viewers). The
// fragment is rendered ONCE here (per cache build, not per request) from
// a locale-only PageData (no session fields: publicResultsFragment only
// ever reads p.T(...)), so it is safe to reuse verbatim across every
// viewer of the same (meet, locale) regardless of their own session/CSRF
// state; it comes back as a string specifically so callers can hand it
// straight to templ.Raw/io.WriteString with no copy (see
// publicResultsCacheEntry's doc comment).
func (s *Server) cachedPublicResults(r *http.Request, meetID string) (publicResultsView, string, error) {
	loc := localeFromContext(r.Context())
	key := publicResultsCacheKey{meetID: meetID, locale: loc}
	e, err := s.publicResults.getOrBuild(key, func() (publicResultsCacheEntry, error) {
		v, err := s.buildPublicResultsView(r, meetID)
		if err != nil {
			return publicResultsCacheEntry{}, err
		}
		var buf bytes.Buffer
		if err := publicResultsFragment(PageData{Locale: loc, Cats: s.cats}, v).Render(r.Context(), &buf); err != nil {
			return publicResultsCacheEntry{}, err
		}
		return publicResultsCacheEntry{view: v, fragmentHTML: buf.String()}, nil
	})
	if err != nil {
		return publicResultsView{}, "", err
	}
	return e.view, e.fragmentHTML, nil
}

// handlePublicResults serves the full public results page (UC-017 #2):
// page shell rendered fresh per request (nav, login state, CSRF token —
// all session-specific), the results section spliced in as the cached,
// pre-rendered fragment (templ.Raw) rather than recomputed.
func (s *Server) handlePublicResults(w http.ResponseWriter, r *http.Request) {
	v, frag, err := s.cachedPublicResults(r, r.PathValue("id"))
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = v.MeetName + " — " + p.T("public.results.title")
	_ = publicResultsPage(p, v, templ.Raw(frag)).Render(r.Context(), w)
}

// handlePublicResultsLive serves just the results fragment the public
// results page's live-refresh island swaps in on the meet's "results" SSE
// event (UC-017 #1, SYS-071): a fresh fetch of this endpoint is what
// proves a confirmed result reaches an already-open public page — the
// per-meet render cache is invalidated by that same event (server.go)
// before any client's live-refresh fetch can land, so this never serves a
// stale render for the update it is proving. Serves the cached string
// directly (no per-request template walk, no copy).
func (s *Server) handlePublicResultsLive(w http.ResponseWriter, r *http.Request) {
	_, frag, err := s.cachedPublicResults(r, r.PathValue("id"))
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	_, _ = io.WriteString(w, frag)
}
