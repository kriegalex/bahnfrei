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

	"github.com/kriegalex/bahnfrei/internal/app"
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
	// AnchorID is the per-event jump-nav target (SYS-153/UC-042 #2): the
	// heat-sheet section's own discipline+category grouping is the
	// "category" this page already renders, so the jump nav targets it
	// directly rather than inventing a second grouping. Index-based
	// ("heat-event-0", …) rather than slugging DisciplineLabel/Categories:
	// those are free-text/localized strings that may collide or contain
	// characters awkward in an HTML id, and the index is already stable
	// for the lifetime of one render.
	AnchorID string
}

type publicStartListsView struct {
	MeetID   string
	MeetName string
	Rows     []publicStartListRowView
	// HeatEvents lists every event whose heats/flights have been generated
	// (TASK-018): shown alongside the flat roster below, additively — an
	// event with no seeded round simply does not appear here.
	HeatEvents []publicHeatEventView
	// Query is the SYS-153/UC-042 find-your-athlete filter's current value
	// (the "q" query-param — DEC-021/TASK-038 convention, searchQuery),
	// redisplayed so a no-JS filtered page survives reload/bookmark like
	// every other list filter in this app.
	Query string
	// FilterCountLabel is the localized "n results" line (UC-042 #1): the
	// no-JS baseline the public-filter.ts island updates live as the
	// visitor types, computed once here from whatever the current Query
	// already filtered server-side.
	FilterCountLabel string
	// FilterCountTemplate is the same message with "{n}" left
	// unsubstituted — the raw template the island re-interpolates
	// client-side after every filter keystroke (same data-attribute
	// convention as capture-offline.ts's data-i18n-pending).
	FilterCountTemplate string
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
	for evIdx, ev := range heatSheets {
		label := ev.DisciplineName
		if label == "" {
			label = ev.EventID
		}
		hev := publicHeatEventView{
			DisciplineLabel: label,
			Categories:      strings.Join(ev.CategoryCodes, ", "),
			AnchorID:        "heat-event-" + strconv.Itoa(evIdx),
		}
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

	// SYS-153/UC-042 #1: the start-list page is never cached (unlike the
	// public results fragment, ADR-004 §9), so there is no cache-safety
	// concern in filtering it server-side on every request — a JS-enabled
	// visitor still filters entirely client-side over the full, unfiltered
	// rows (public-filter.ts), so this only ever pays for itself on the
	// no-JS ?q= fallback path.
	view = filterPublicStartListsView(p, view, searchQuery(r))
	_ = publicStartListsPage(p, view).Render(r.Context(), w)
}

// filterPublicStartListsView narrows v.Rows and v.HeatEvents to entries
// matching q (name/bib/club for the flat roster; name/club for heat rows,
// which carry no bib — DEC-021/TASK-038's app.MatchesParticipantSearch,
// reused verbatim), dropping any heat unit/round/event left with no
// matching rows (UC-042 #1: "matching rows and their categories remain
// visible"). Always sets Query/FilterCountLabel/FilterCountTemplate, even
// for an empty q, so the no-JS and JS-enhanced paths render from the same
// fields. A no-op filter (q == "") still runs the copy/rebuild below —
// cheap at this page's scale and it keeps one code path for both cases.
func filterPublicStartListsView(p PageData, v publicStartListsView, q string) publicStartListsView {
	v.Query = q
	v.FilterCountTemplate = p.T("public.filter.count")

	var rows []publicStartListRowView
	for _, row := range v.Rows {
		if app.MatchesParticipantSearch(q, row.Name, "", row.Bib, row.Club) {
			rows = append(rows, row)
		}
	}
	count := len(rows)

	var events []publicHeatEventView
	for _, ev := range v.HeatEvents {
		var rounds []publicHeatRoundView
		for _, round := range ev.Rounds {
			var units []publicHeatUnitView
			for _, u := range round.Units {
				var uRows []publicHeatRowView
				for _, row := range u.Rows {
					if app.MatchesParticipantSearch(q, row.Name, "", "", row.Club) {
						uRows = append(uRows, row)
					}
				}
				if len(uRows) == 0 {
					continue
				}
				u.Rows = uRows
				count += len(uRows)
				units = append(units, u)
			}
			if len(units) == 0 {
				continue
			}
			round.Units = units
			rounds = append(rounds, round)
		}
		if len(rounds) == 0 {
			continue
		}
		ev.Rounds = rounds
		events = append(events, ev)
	}

	v.Rows = rows
	v.HeatEvents = events
	v.FilterCountLabel = p.T("public.filter.count", "n", strconv.Itoa(count))
	return v
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
	// Query is the SYS-153/UC-042 find-your-athlete filter's current value.
	// Always "" for the cached (ADR-004 §9) render — see
	// filterPublicResultsView's doc comment for why a non-empty query never
	// reaches the cache.
	Query string
	// FilterCountLabel/FilterCountTemplate mirror
	// publicStartListsView's fields of the same name: the localized
	// "n results" line and its raw "{n}"-templated form for
	// public-filter.ts to re-interpolate client-side.
	FilterCountLabel    string
	FilterCountTemplate string
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
	total := 0
	for divIdx, div := range standings.Divisions {
		// AnchorID: SYS-153/UC-042 #2's per-category jump nav targets each
		// division directly — divisions are exactly this page's existing
		// category grouping, so no new grouping concept is introduced.
		// Index-based rather than slugging div.CategoryCode: category
		// codes can contain spaces ("U18 W") and are not guaranteed
		// distinct from an HTML-id-safe character set.
		dv := divisionView{Code: div.CategoryCode, AnchorID: "div-" + strconv.Itoa(divIdx)}
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
		total += len(dv.Rows)
		v.Divisions = append(v.Divisions, dv)
	}
	v.FilterCountTemplate = p.T("public.filter.count")
	v.FilterCountLabel = p.T("public.filter.count", "n", strconv.Itoa(total))
	return v, nil
}

// filterPublicResultsView narrows v.Divisions to rows matching q
// (name/bib/club, case-insensitive substring — DEC-021/TASK-038's
// app.MatchesParticipantSearch, reused verbatim) and drops any division
// left with no matching rows (UC-042 #1: "only matching rows and their
// categories remain visible"). Recomputes FilterCountLabel from the
// filtered set; FilterCountTemplate is locale-only and untouched.
//
// ADR-004 §9 cache-safety (SYS-153/UC-042, TASK-048): this is called ONLY
// from handlePublicResults' q != "" branch, which builds its own,
// uncached publicResultsView first (see that handler) — a filtered view
// is never the thing s.publicResults.getOrBuild stores. The per-meet
// render cache stays keyed on (meetID, locale) alone; it never sees q, so
// an unbounded set of ?q= values can never grow the cache map (the exact
// OOM class ADR-004 §9/TASK-035 fixed). The cost is one uncached
// Standings()+render per no-JS filtered request — acceptable because it
// is the rare progressive-enhancement fallback (SYS-153: "functional
// without client-side scripting"), not the common case: a JS-enabled
// visitor never sends ?q= at all, filtering entirely client-side over the
// already-cached, already-rendered fragment (public-filter.ts).
func filterPublicResultsView(p PageData, v publicResultsView, q string) publicResultsView {
	if q == "" {
		return v
	}
	var kept []divisionView
	count := 0
	for _, div := range v.Divisions {
		var rows []standingRowView
		for _, row := range div.Rows {
			if app.MatchesParticipantSearch(q, row.Name, "", row.Bib, row.Club) {
				rows = append(rows, row)
			}
		}
		if len(rows) == 0 {
			continue
		}
		div.Rows = rows
		count += len(rows)
		kept = append(kept, div)
	}
	v.Divisions = kept
	v.Query = q
	v.FilterCountLabel = p.T("public.filter.count", "n", strconv.Itoa(count))
	return v
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
//
// SYS-153/UC-042 #1 (TASK-048): a non-empty "q" is the no-JS find-your-
// athlete fallback and takes a completely separate, uncached path —
// s.publicResults (the ADR-004 §9 render cache) is never consulted or
// populated for it, so ?q= URLs cannot fragment the cache map. See
// filterPublicResultsView's doc comment for the full rationale. An empty
// q (the common case: no filter entered, or a JS-enabled visitor who
// never round-trips a query at all) is indistinguishable from a plain
// request and takes the normal cached path.
func (s *Server) handlePublicResults(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	q := searchQuery(r)
	p := basePageData(r, s.cats)
	p.Title = p.T("public.results.title")
	if q != "" {
		v, err := s.buildPublicResultsView(r, meetID)
		if err != nil {
			s.renderMeetError(w, r, err)
			return
		}
		v = filterPublicResultsView(p, v, q)
		p.Title = v.MeetName + " — " + p.T("public.results.title")
		_ = publicResultsPage(p, v, publicResultsFragment(p, v)).Render(r.Context(), w)
		return
	}
	v, frag, err := s.cachedPublicResults(r, meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
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
