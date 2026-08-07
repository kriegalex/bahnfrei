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
	ParticipantID string
	Version       string
	Bib           string
	Name          string
	BirthYear     string
	Club          string
	// Anonymized mirrors the athlete's SYS-101 erasure state (TASK-049):
	// an erased participant renders with no edit link at all, rather than
	// one that errors on click.
	Anonymized bool
	// OutOfCompetition mirrors store.Participant.OutOfCompetition
	// (TASK-036/OQ-091): drives the roster-row toggle's current label and
	// the submitted "value" it flips to.
	OutOfCompetition bool
}

type rosterView struct {
	MeetID   string
	MeetName string
	Rows     []rosterRowView
	// Query is the DEC-021/TASK-038 roster search box's current value (the
	// "q" query-param), redisplayed so the search state survives a page
	// reload/bookmark like every other list filter in this app.
	Query string
}

func (s *Server) rosterView(r *http.Request, meetID string) (rosterView, error) {
	detail, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		return rosterView{}, err
	}
	q := searchQuery(r)
	v := rosterView{MeetID: detail.ID, MeetName: detail.Name, Query: q}
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
		if !app.MatchesParticipantSearch(q, p.Athlete.FirstName, p.Athlete.LastName, p.Bib, club) {
			continue
		}
		v.Rows = append(v.Rows, rosterRowView{
			ParticipantID:    p.ID,
			Version:          intToStr(p.Version),
			Bib:              p.Bib,
			Name:             p.Athlete.FirstName + " " + p.Athlete.LastName,
			BirthYear:        strconv.Itoa(p.Athlete.BirthYear),
			Club:             club,
			Anonymized:       p.Athlete.Anonymized,
			OutOfCompetition: p.OutOfCompetition,
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
		// SYS-103/UC-023: the entry flow collects the publication-consent
		// choice up front — an unchecked box (the default) means results
		// are publicly listed as usual; checking it withdraws consent from
		// the start (see domain.PublicationConsent for the opt-out shape).
		PublicationWithdrawn: r.FormValue("publication_withdrawn") == "true",
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

// handleRosterOutOfCompetitionToggle flips a participant's ausser
// Konkurrenz/hors concours flag (TASK-052, OQ-091, over TASK-036's
// `ResultsService.SetOutOfCompetition`): one same-page POST per roster row,
// version-guarded from the roster row's own rendered version like the
// identity-correction form's bib field, audited by SetOutOfCompetition
// itself. The web layer resolves the participant id from the roster path
// (never a store type, architecture.md §3) to the athlete id
// SetOutOfCompetition takes.
func (s *Server) handleRosterOutOfCompetitionToggle(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	participantID := r.PathValue("participant")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	version, _ := strconv.ParseInt(r.FormValue("version"), 10, 64)
	value := r.FormValue("value") == "true"

	participants, err := s.results.Participants(r.Context(), meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	athleteID := ""
	for _, p := range participants {
		if p.ID == participantID {
			athleteID = p.AthleteID
			break
		}
	}
	if athleteID == "" {
		s.handleNotFound(w, r)
		return
	}

	if err := s.results.SetOutOfCompetition(r.Context(), actor, meetID, athleteID, version, value); err != nil {
		v, verr := s.rosterView(r, meetID)
		if verr != nil {
			s.renderMeetError(w, r, verr)
			return
		}
		p := basePageData(r, s.cats)
		p.Title = v.MeetName + " — " + p.T("roster.title")
		status := http.StatusUnprocessableEntity
		flashKey := "roster.out_of_competition.flash.invalid"
		if errors.Is(err, app.ErrConflict) {
			status = http.StatusConflict
			flashKey = "roster.out_of_competition.flash.conflict"
		}
		p.FlashError = p.T(flashKey)
		w.WriteHeader(status)
		_ = rosterPage(p, v).Render(r.Context(), w)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/roster", http.StatusSeeOther)
}

// --- participant identity correction (TASK-049, SYS-150/UC-043) ---

// participantFormView is the roster-edit form's view model: current values
// pre-filled (GET) or the just-submitted, possibly-invalid values
// preserved on a validation failure (POST) — the same shape every
// version-guarded edit form in this app uses (meetFormView's precedent).
type participantFormView struct {
	MeetID        string
	MeetName      string
	ParticipantID string
	Version       int64
	FirstName     string
	LastName      string
	BirthYear     string
	Sex           string
	Club          string
	Bib           string
	Reason        string
	Errors        FieldErrors
}

// handleParticipantEditForm serves the pre-filled correction form (UC-043:
// "the form must show current values"). The web layer never names a store
// type (architecture.md §3, depguard's web-goes-through-app rule): it
// finds the row by ranging over ResultsService.Participants' result with
// an inferred element type, never a `store.` qualifier.
func (s *Server) handleParticipantEditForm(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	participantID := r.PathValue("participant")
	detail, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	participants, err := s.results.Participants(r.Context(), meetID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	idx := -1
	for i, p := range participants {
		if p.ID == participantID {
			idx = i
			break
		}
	}
	if idx < 0 || participants[idx].Athlete.Anonymized {
		// TASK-049: an erased participant is never editable — same 404 as
		// a participant that does not exist, rather than a form that
		// errors on submit.
		s.handleNotFound(w, r)
		return
	}
	found := participants[idx]
	club := ""
	if len(found.Athlete.ClubIDs) > 0 {
		if names, err := s.results.ClubNamesFor(r.Context(), participants[idx:idx+1]); err == nil {
			club = names[found.Athlete.ClubIDs[0]]
		}
	}
	form := participantFormView{
		MeetID: meetID, MeetName: detail.Name, ParticipantID: found.ID, Version: found.Version,
		FirstName: found.Athlete.FirstName, LastName: found.Athlete.LastName,
		BirthYear: strconv.Itoa(found.Athlete.BirthYear), Sex: string(found.Athlete.Sex),
		Club: club, Bib: found.Bib,
	}
	p := basePageData(r, s.cats)
	p.Title = p.T("roster.edit.title")
	_ = participantFormPage(p, form).Render(r.Context(), w)
}

// participantIdentityFlashKey maps an UpdateParticipantIdentity business
// error onto the "roster.edit.flash.*" key suffix the form's page-level
// alert renders (the conflict/duplicate/anonymized cases are whole-form
// state, not attributable to one input field — mirrors
// flashKeyFor/entryFlashKey's precedent).
func participantIdentityFlashKey(err error) string {
	switch {
	case errors.Is(err, app.ErrConflict):
		return "conflict"
	case errors.Is(err, app.ErrDuplicateParticipant):
		return "duplicate"
	case errors.Is(err, app.ErrAthleteAnonymized):
		return "anonymized"
	default:
		return "invalid"
	}
}

func participantIdentityStatus(err error) int {
	if errors.Is(err, app.ErrConflict) {
		return http.StatusConflict
	}
	return http.StatusUnprocessableEntity
}

func (s *Server) handleParticipantEditSubmit(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	participantID := r.PathValue("participant")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	detail, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = p.T("roster.edit.title")

	version, _ := strconv.ParseInt(r.FormValue("version"), 10, 64)
	birthYearStr := strings.TrimSpace(r.FormValue("birth_year"))
	birthYear, _ := strconv.Atoi(birthYearStr)
	form := participantFormView{
		MeetID: meetID, MeetName: detail.Name, ParticipantID: participantID, Version: version,
		FirstName: strings.TrimSpace(r.FormValue("first_name")),
		LastName:  strings.TrimSpace(r.FormValue("last_name")),
		BirthYear: birthYearStr,
		Sex:       r.FormValue("sex"),
		Club:      strings.TrimSpace(r.FormValue("club")),
		Bib:       strings.TrimSpace(r.FormValue("bib")),
		Reason:    strings.TrimSpace(r.FormValue("reason")),
	}

	renderErr := func(status int, flashKey string, errs FieldErrors) {
		form.Errors = errs
		if flashKey != "" {
			p.FlashError = p.T("roster.edit.flash." + flashKey)
		}
		w.WriteHeader(status)
		_ = participantFormPage(p, form).Render(r.Context(), w)
	}

	// OQ-075/UC-038 #4: attribute each validation failure to its own
	// field and re-render with the submitted values intact, mirroring
	// handleEntryIndividualSubmit's precedent.
	errs := FieldErrors{}
	if form.LastName == "" {
		errs["last_name"] = p.T("roster.field_error.last_name.required")
	}
	minYear, maxYear := s.results.ParticipantIdentityBirthYearBounds()
	if birthYearStr == "" || birthYear < minYear || birthYear > maxYear {
		errs["birth_year"] = p.T("roster.field_error.birth_year.invalid")
	}
	if form.Sex != string(domain.SexMale) && form.Sex != string(domain.SexFemale) {
		errs["sex"] = p.T("roster.field_error.sex.invalid")
	}
	if !app.ValidBibFormat(form.Bib) {
		errs["bib"] = p.T("roster.field_error.bib.invalid")
	}
	if len(errs) > 0 {
		renderErr(http.StatusUnprocessableEntity, "", errs)
		return
	}

	in := app.ParticipantIdentityInput{
		FirstName: form.FirstName, LastName: form.LastName, BirthYear: birthYear,
		Sex: domain.Sex(form.Sex), Club: form.Club, Bib: form.Bib, Reason: form.Reason,
	}
	if _, err := s.results.UpdateParticipantIdentity(r.Context(), actor, meetID, participantID, version, in); err != nil {
		if errors.Is(err, app.ErrParticipantIdentityInvalid) {
			renderErr(http.StatusUnprocessableEntity, "invalid", nil)
			return
		}
		renderErr(participantIdentityStatus(err), participantIdentityFlashKey(err), nil)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/roster", http.StatusSeeOther)
}

// --- division standings (UC-033 #2–#4, SYS-052/053) ---

type markCellView struct {
	Mark   string
	Points string
	// Flags is the SYS-049 record/best flag codes (e.g. "MR", "PB"),
	// comma-joined for display — "" when none earned.
	Flags string
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
	// AnchorID is the public results page's per-category jump-nav target
	// (SYS-153/UC-042 #2, TASK-048) — set by buildPublicResultsView
	// (public.go); the operator standings page (handleStandings) leaves it
	// at its zero value, unused, since it has no jump nav.
	AnchorID string
}

type standingsView struct {
	MeetID                string
	MeetName              string
	StatusLabel           string
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
	// TASK-036/DEC-016: FINAL semantics (unranked-at-bottom, the 1-point
	// floor) once the division's series is complete, PROVISIONAL
	// otherwise — same completeness rule every other standings-derived
	// surface applies (public results, PDF, series-upload export).
	standings, err := s.results.CurrentStandings(r.Context(), meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	v := standingsView{
		MeetID:                detail.ID,
		MeetName:              detail.Name,
		StatusLabel:           standingsStatusLabel(p, standings.Final),
		SeriesUploadAvailable: s.results.SeriesUploadAvailable(r.Context(), meetID),
	}
	for _, code := range standings.Disciplines {
		v.Disciplines = append(v.Disciplines, s.localizedDisciplineName(p, code))
	}
	for _, div := range standings.Divisions {
		dv := divisionView{Code: div.CategoryCode}
		for _, row := range div.Rows {
			rv := standingRowView{
				Rank:      rankLabel(row),
				Bib:       row.Bib,
				Name:      row.FirstName + " " + row.LastName,
				Club:      row.ClubName,
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

// standingsStatusLabel renders the provisional/final line every "final
// list" surface shows (TASK-036, DEC-016/OQ-020): "Finalstand" is the
// literal header word the evidenced LV Langenthal Gesamtrangliste uses
// once a division's series is complete; live/in-progress standings are
// labeled provisional.
func standingsStatusLabel(p PageData, final bool) string {
	if final {
		return p.T("standings.final")
	}
	return p.T("standings.provisional")
}

// rankLabel renders a standings row's rank cell: the numeric competition
// rank, or a blank cell when the row is unranked (TASK-036, DEC-016/OQ-020)
// — the evidenced LV Langenthal Gesamtrangliste's unranked rows carry no
// rank number at all, in either sense (missing discipline or
// out-of-competition).
func rankLabel(row app.StandingRow) string {
	if row.Rank == 0 {
		return ""
	}
	return strconv.Itoa(row.Rank)
}

// totalLabel renders a standings row's total cell: the numeric total, or
// the Swiss TAF3 unranked marker (TASK-036, DEC-016/OQ-020) when the row is
// unranked — "n.a." for an out-of-competition participant (evidenced: LV
// Langenthal Gesamtrangliste 17.05.2025, Thome Lauriane W12 — every mark
// present, still unranked), "aufg." for a participant missing a series
// discipline entirely (evidenced: the same list's "aufg." rows, e.g. Joao
// Daniella M14). A PROVISIONAL row is never unranked outside the
// out-of-competition case (Standings always ranks by partial total).
func totalLabel(p PageData, row app.StandingRow) string {
	if row.Rank > 0 {
		return strconv.Itoa(row.Total)
	}
	if row.OutOfCompetition {
		return p.T("standings.unranked_out_of_competition")
	}
	return p.T("standings.unranked_missing")
}

// renderMeetError maps app-layer errors onto the workspace's error pages.
func (s *Server) renderMeetError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, app.ErrMeetNotFound) {
		s.handleNotFound(w, r)
		return
	}
	http.Error(w, "internal error", http.StatusInternalServerError)
}
