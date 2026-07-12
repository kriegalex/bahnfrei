// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/domain"
)

// Form wire formats for <input type="date"> and <input type="datetime-local">.
const (
	formDateLayout     = "2006-01-02"
	formDateTimeLayout = "2006-01-02T15:04"
)

// maxSessionRows is how many blank session lines the meet form offers;
// empty rows are ignored on submit.
const maxSessionRows = 8

// --- first-run setup (UC-001 #1, SYS-131) ---

// handleSetupForm offers the first-run admin-account creation when no
// account exists yet; afterwards the route disappears behind a redirect,
// so the quickstart needs no configuration file and no CLI wizardry.
func (s *Server) handleSetupForm(w http.ResponseWriter, r *http.Request) {
	needs, err := s.auth.NeedsBootstrap(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !needs {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = p.T("setup.title")
	_ = setupPage(p).Render(r.Context(), w)
}

func (s *Server) handleSetupSubmit(w http.ResponseWriter, r *http.Request) {
	needs, err := s.auth.NeedsBootstrap(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !needs {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	displayName := strings.TrimSpace(r.FormValue("display_name"))
	password := r.FormValue("password")

	renderErr := func(key string) {
		p := basePageData(r, s.cats)
		p.Title = p.T("setup.title")
		p.FlashError = p.T(key)
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = setupPage(p).Render(r.Context(), w)
	}
	if username == "" || displayName == "" {
		renderErr("setup.error.missing_fields")
		return
	}
	if len(password) < 8 {
		renderErr("setup.error.password_short")
		return
	}
	if _, err := s.auth.Bootstrap(r.Context(), username, displayName, password); err != nil {
		renderErr("setup.error.failed")
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// --- operator meet workspace (SYS-001/002/004/006) ---

func (s *Server) handleMeetsList(w http.ResponseWriter, r *http.Request) {
	meets, err := s.meets.ListMeets(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = p.T("meets.title")
	rows := make([]meetRowView, 0, len(meets))
	for _, m := range meets {
		rows = append(rows, meetRowView{
			ID:     m.ID,
			Name:   m.Name,
			Venue:  m.Venue,
			Dates:  formatDateRange(p, m.StartDate, m.EndDate),
			Tier:   string(m.Tier),
			Status: p.T("meet.status." + string(m.Status)),
		})
	}
	_ = meetsListPage(p, rows).Render(r.Context(), w)
}

func (s *Server) handleMeetNewForm(w http.ResponseWriter, r *http.Request) {
	p := basePageData(r, s.cats)
	p.Title = p.T("meet.new.title")
	_ = meetFormPage(p, s.emptyMeetForm(), "/meets").Render(r.Context(), w)
}

func (s *Server) handleMeetCreate(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	form, req, err := s.parseMeetForm(r)
	if err != nil {
		s.renderMeetForm(w, r, form, "/meets", err)
		return
	}
	rec, err := s.meets.CreateMeet(r.Context(), actor, req)
	if err != nil {
		s.renderMeetForm(w, r, form, "/meets", err)
		return
	}
	http.Redirect(w, r, "/meets/"+rec.ID, http.StatusSeeOther)
}

func (s *Server) handleMeetDetail(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	d, err := s.meets.Meet(r.Context(), meetID)
	if errors.Is(err, app.ErrMeetNotFound) {
		s.handleNotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	versions, err := s.meets.TimetableVersions(r.Context(), meetID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = d.Name
	if msg := r.URL.Query().Get("err"); msg != "" {
		p.FlashError = p.T("meet.flash." + msg)
	}
	_ = meetDetailPage(p, s.meetDetailView(p, d, versions)).Render(r.Context(), w)
}

func (s *Server) handleMeetEditForm(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	d, err := s.meets.Meet(r.Context(), meetID)
	if errors.Is(err, app.ErrMeetNotFound) {
		s.handleNotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	form := s.emptyMeetForm()
	form.Version = d.Version
	form.Name = d.Name
	form.Venue = d.Venue
	form.HomologationRef = d.HomologationRef
	form.StartDate = d.StartDate.Format(formDateLayout)
	form.EndDate = d.EndDate.Format(formDateLayout)
	form.Tier = string(d.Tier)
	form.SchemeID = d.CategorySchemeID
	form.SchemeFixed = true // the scheme is chosen at creation; events already depend on it
	form.ResultsPositioning = string(d.ResultsPositioning)
	form.OfficialSourceName = d.OfficialSourceName
	form.OfficialSourceURL = d.OfficialSourceURL
	for i, sess := range d.Sessions {
		if i >= maxSessionRows {
			break
		}
		form.Sessions[i] = sessionRowView{Day: sess.Day.Format(formDateLayout), Label: sess.Label}
	}
	p := basePageData(r, s.cats)
	p.Title = p.T("meet.edit.title")
	_ = meetFormPage(p, form, "/meets/"+meetID+"/edit").Render(r.Context(), w)
}

func (s *Server) handleMeetEditSubmit(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	actor, _ := sessionFromContext(r.Context())
	form, req, err := s.parseMeetForm(r)
	action := "/meets/" + meetID + "/edit"
	if err != nil {
		s.renderMeetForm(w, r, form, action, err)
		return
	}
	if err := s.meets.UpdateMeet(r.Context(), actor, meetID, form.Version, req); err != nil {
		s.renderMeetForm(w, r, form, action, err)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID, http.StatusSeeOther)
}

func (s *Server) handleMeetArchive(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	actor, _ := sessionFromContext(r.Context())
	version, _ := strconv.ParseInt(r.FormValue("version"), 10, 64)
	if err := s.meets.ArchiveMeet(r.Context(), actor, meetID, version); err != nil {
		s.redirectMeetError(w, r, meetID, err)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID, http.StatusSeeOther)
}

// handleMeetPublish moves a meet from draft to published (TASK-016's
// prerequisite for online entries to open, UC-003 #1).
func (s *Server) handleMeetPublish(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	actor, _ := sessionFromContext(r.Context())
	version, _ := strconv.ParseInt(r.FormValue("version"), 10, 64)
	if err := s.meets.PublishMeet(r.Context(), actor, meetID, version); err != nil {
		s.redirectMeetError(w, r, meetID, err)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID, http.StatusSeeOther)
}

func (s *Server) handleEventCreate(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	actor, _ := sessionFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	entryLimit, _ := strconv.Atoi(r.FormValue("entry_limit"))
	req := app.AddEventRequest{
		DisciplineCode: r.FormValue("discipline"),
		CategoryCodes:  r.Form["categories"],
		EntryStandard:  strings.TrimSpace(r.FormValue("entry_standard")),
		EntryLimit:     entryLimit,
	}
	for _, kind := range []domain.RoundKind{domain.RoundQualification, domain.RoundSemifinal, domain.RoundFinal} {
		if r.FormValue("round_"+string(kind)) != "" {
			req.Rounds = append(req.Rounds, kind)
		}
	}
	if v := r.FormValue("entry_deadline"); v != "" {
		t, err := time.Parse(formDateTimeLayout, v)
		if err != nil {
			s.redirectMeetError(w, r, meetID, errBadInput)
			return
		}
		req.EntryDeadline = &t
	}
	if _, err := s.meets.AddEvent(r.Context(), actor, meetID, req); err != nil {
		s.redirectMeetError(w, r, meetID, err)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID, http.StatusSeeOther)
}

func (s *Server) handleUnitSchedule(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	unitID := r.PathValue("unit")
	actor, _ := sessionFromContext(r.Context())
	version, _ := strconv.ParseInt(r.FormValue("version"), 10, 64)
	at, err := time.Parse(formDateTimeLayout, r.FormValue("scheduled_at"))
	if err != nil {
		s.redirectMeetError(w, r, meetID, errBadInput)
		return
	}
	location := strings.TrimSpace(r.FormValue("location"))
	if err := s.meets.ScheduleUnit(r.Context(), actor, unitID, version, at, location); err != nil {
		s.redirectMeetError(w, r, meetID, err)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID, http.StatusSeeOther)
}

func (s *Server) handleTimetablePublish(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	actor, _ := sessionFromContext(r.Context())
	v, err := s.meets.PublishTimetable(r.Context(), actor, meetID)
	if err != nil {
		s.redirectMeetError(w, r, meetID, err)
		return
	}
	// Live update for public timetable viewers (SYS-071 transport; the
	// public page subscribes with later tasks' live-results work).
	s.bus.Publish("meet-"+meetID, Event{
		Name: "timetable",
		Data: fmt.Sprintf(`{"version":%d}`, v.Version),
	})
	http.Redirect(w, r, "/meets/"+meetID, http.StatusSeeOther)
}

func (s *Server) handleSanctioning(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	sum, err := s.meets.SanctioningSummary(r.Context(), meetID)
	if errors.Is(err, app.ErrMeetNotFound) {
		s.handleNotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = p.T("sanctioning.title")
	_ = sanctioningPage(p, s.sanctioningView(p, sum)).Render(r.Context(), w)
}

// --- public surface (SYS-090 unauthenticated read) ---

// handlePublicTimetable serves the meet's current published timetable at a
// stable URL (UC-001 #4: "the current public timetable shows the amended
// time").
func (s *Server) handlePublicTimetable(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	rec, v, err := s.meets.PublicTimetable(r.Context(), meetID)
	if errors.Is(err, app.ErrMeetNotFound) {
		s.handleNotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = rec.Name
	view := publicTimetableView{
		MeetID:      rec.ID,
		MeetName:    rec.Name,
		Venue:       rec.Venue,
		Dates:       formatDateRange(p, rec.StartDate, rec.EndDate),
		Version:     v.Version,
		PublishedAt: p.FormatDateTime(v.PublishedAt),
		Rows:        s.unitRows(p, v.Entries),
	}
	// SYS-074/SYS-111: the public timetable localizes discipline names the
	// same way the public results page does; the operator meet-detail Units
	// table (the other unitRows caller) keeps the canonical catalog name,
	// consistent with the operator Programme table on the same page (PoC
	// scope — see docs/requirements/open-questions-and-assumptions.md).
	for i, e := range v.Entries {
		view.Rows[i].Discipline = s.localizedDisciplineName(p, e.DisciplineCode)
	}
	_ = publicTimetablePage(p, view).Render(r.Context(), w)
}

// --- form parsing and view mapping ---

// errBadInput marks unparseable form input (dates, numbers).
var errBadInput = errors.New("bad form input")

// resultsPositionings lists the SYS-076 positioning choices offered on the
// meet form, in a stable order (federation_official first: the default).
var resultsPositionings = []string{
	string(domain.ResultsPositioningFederationOfficial),
	string(domain.ResultsPositioningPrimary),
}

func (s *Server) emptyMeetForm() meetFormView {
	return meetFormView{
		Tiers:               []string{"A-Meeting", "B-Meeting", "C-Meeting"},
		Schemes:             s.meets.CategorySchemes(),
		SchemeID:            domain.SchemeSwissAthletics,
		Sessions:            make([]sessionRowView, maxSessionRows),
		ResultsPositionings: resultsPositionings,
		ResultsPositioning:  string(domain.ResultsPositioningFederationOfficial),
	}
}

// parseMeetForm maps the meet form to an app.MeetRequest, returning the
// re-renderable form state alongside so validation errors keep the
// operator's input.
func (s *Server) parseMeetForm(r *http.Request) (meetFormView, app.MeetRequest, error) {
	form := s.emptyMeetForm()
	if err := r.ParseForm(); err != nil {
		return form, app.MeetRequest{}, errBadInput
	}
	form.Version, _ = strconv.ParseInt(r.FormValue("version"), 10, 64)
	form.Name = strings.TrimSpace(r.FormValue("name"))
	form.Venue = strings.TrimSpace(r.FormValue("venue"))
	form.HomologationRef = strings.TrimSpace(r.FormValue("homologation_ref"))
	form.StartDate = r.FormValue("start_date")
	form.EndDate = r.FormValue("end_date")
	form.Tier = r.FormValue("tier")
	if v := r.FormValue("scheme"); v != "" {
		form.SchemeID = v
	}
	if v := r.FormValue("results_positioning"); v != "" {
		form.ResultsPositioning = v
	}
	form.OfficialSourceName = strings.TrimSpace(r.FormValue("official_source_name"))
	form.OfficialSourceURL = strings.TrimSpace(r.FormValue("official_source_url"))
	for i := 0; i < maxSessionRows; i++ {
		form.Sessions[i] = sessionRowView{
			Day:   r.FormValue(fmt.Sprintf("session_day_%d", i)),
			Label: strings.TrimSpace(r.FormValue(fmt.Sprintf("session_label_%d", i))),
		}
	}

	req := app.MeetRequest{
		Name:               form.Name,
		Venue:              form.Venue,
		HomologationRef:    form.HomologationRef,
		Tier:               form.Tier,
		CategorySchemeID:   form.SchemeID,
		ResultsPositioning: form.ResultsPositioning,
		OfficialSourceName: form.OfficialSourceName,
		OfficialSourceURL:  form.OfficialSourceURL,
	}
	if req.Name == "" || form.StartDate == "" || form.EndDate == "" {
		return form, req, errBadInput
	}
	var err error
	if req.StartDate, err = time.Parse(formDateLayout, form.StartDate); err != nil {
		return form, req, errBadInput
	}
	if req.EndDate, err = time.Parse(formDateLayout, form.EndDate); err != nil {
		return form, req, errBadInput
	}
	for _, row := range form.Sessions {
		if row.Day == "" && row.Label == "" {
			continue
		}
		day, err := time.Parse(formDateLayout, row.Day)
		if err != nil {
			return form, req, errBadInput
		}
		req.Sessions = append(req.Sessions, app.SessionPlan{Day: day, Label: row.Label})
	}
	return form, req, nil
}

func (s *Server) renderMeetForm(w http.ResponseWriter, r *http.Request, form meetFormView, action string, err error) {
	p := basePageData(r, s.cats)
	p.Title = p.T("meet.new.title")
	p.FlashError = flashFor(p, err)
	w.WriteHeader(statusFor(err))
	_ = meetFormPage(p, form, action).Render(r.Context(), w)
}

// redirectMeetError sends the operator back to the meet page with a
// one-shot localized error key in the query string (POST-redirect-GET, so
// a refresh never re-submits).
func (s *Server) redirectMeetError(w http.ResponseWriter, r *http.Request, meetID string, err error) {
	http.Redirect(w, r, "/meets/"+meetID+"?err="+flashKeyFor(err), http.StatusSeeOther)
}

func flashKeyFor(err error) string {
	switch {
	case errors.Is(err, app.ErrConflict):
		return "conflict"
	case errors.Is(err, app.ErrMeetNotFound):
		return "not_found"
	case errors.Is(err, errBadInput):
		return "bad_input"
	default:
		return "invalid"
	}
}

func flashFor(p PageData, err error) string {
	return p.T("meet.flash." + flashKeyFor(err))
}

func statusFor(err error) int {
	if errors.Is(err, app.ErrConflict) {
		return http.StatusConflict
	}
	return http.StatusUnprocessableEntity
}

// View models: templates see only strings and flags, never app/store
// types (the PageData boundary rule, view.go).

type meetRowView struct {
	ID, Name, Venue, Dates, Tier, Status string
}

type sessionRowView struct {
	Day, Label string
}

type meetFormView struct {
	Version         int64
	Name            string
	Venue           string
	HomologationRef string
	StartDate       string
	EndDate         string
	Tier            string
	Tiers           []string
	SchemeID        string
	Schemes         []string
	SchemeFixed     bool
	Sessions        []sessionRowView
	// ResultsPositioning fields expose the SYS-076 organizer configuration
	// on the create/edit meet form.
	ResultsPositioning  string
	ResultsPositionings []string
	OfficialSourceName  string
	OfficialSourceURL   string
}

type programmeRowView struct {
	EventID     string
	Discipline  string
	Categories  string
	CaptureType string
	Rounds      string
	Deadline    string
	// RoundLinks lists this event's rounds (id + localized label) for the
	// check-in/seeding workspace links (TASK-018, UC-007/008/009).
	RoundLinks []programmeRoundLinkView
}

type programmeRoundLinkView struct {
	RoundID string
	Label   string
}

type unitRowView struct {
	UnitID     string
	Version    int64
	Discipline string
	Categories string
	Round      string
	When       string
	Location   string
	Scheduled  bool
}

type disciplineOptionView struct {
	Code, Name string
}

type timetableVersionRowView struct {
	Version     int
	PublishedAt string
}

type meetDetailView struct {
	ID              string
	Version         int64
	Name            string
	Venue           string
	HomologationRef string
	Dates           string
	Tier            string
	Status          string
	Archived        bool
	// Draft gates the publish action (TASK-016, UC-003 #1 prerequisite):
	// only a draft meet can be published.
	Draft       bool
	SchemeID    string
	Sessions    []sessionRowView
	Programme   []programmeRowView
	Units       []unitRowView
	Versions    []timetableVersionRowView
	Disciplines []disciplineOptionView
	Categories  []string
	// ResultsPositioningLabel is the localized SYS-076 configuration
	// summary shown to the organizer ("Federation channel is official —
	// source: …" / "This system is the primary publication").
	ResultsPositioningLabel string
}

type sanctioningView struct {
	MeetName        string
	Venue           string
	HomologationRef string
	Dates           string
	Organizer       string
	Tier            string
	Sessions        []sessionRowView
	Categories      string
	Disciplines     string
	GeneratedAt     string
	Complete        bool
	Missing         []string
}

type publicTimetableView struct {
	MeetID      string
	MeetName    string
	Venue       string
	Dates       string
	Version     int
	PublishedAt string
	Rows        []unitRowView
}

func formatDateRange(p PageData, start, end time.Time) string {
	if start.Equal(end) {
		return p.FormatDate(start)
	}
	return p.FormatDate(start) + " – " + p.FormatDate(end)
}

func (s *Server) unitRows(p PageData, entries []app.TimetableEntry) []unitRowView {
	rows := make([]unitRowView, 0, len(entries))
	for _, e := range entries {
		row := unitRowView{
			UnitID:     e.UnitID,
			Version:    e.UnitVersion,
			Discipline: e.DisciplineCode,
			Categories: strings.Join(e.CategoryCodes, ", "),
			Round:      p.T("round." + e.RoundKind),
			Location:   e.Location,
		}
		if disc, ok := s.meets.Catalog().ByCode(e.DisciplineCode); ok {
			row.Discipline = disc.Name
		}
		if e.ScheduledAt != nil {
			row.Scheduled = true
			row.When = p.FormatDateTime(*e.ScheduledAt)
		}
		rows = append(rows, row)
	}
	return rows
}

func (s *Server) meetDetailView(p PageData, d app.MeetDetail, versions []app.TimetableVersion) meetDetailView {
	view := meetDetailView{
		ID:                      d.ID,
		Version:                 d.Version,
		Name:                    d.Name,
		Venue:                   d.Venue,
		HomologationRef:         d.HomologationRef,
		Dates:                   formatDateRange(p, d.StartDate, d.EndDate),
		Tier:                    string(d.Tier),
		Status:                  p.T("meet.status." + string(d.Status)),
		Archived:                d.Status == domain.MeetArchived,
		Draft:                   d.Status == domain.MeetDraft,
		SchemeID:                d.CategorySchemeID,
		Units:                   s.unitRows(p, d.Units),
		ResultsPositioningLabel: resultsPositioningSummary(p, d.ResultsPositioning, d.OfficialSourceName),
	}
	for _, sess := range d.Sessions {
		view.Sessions = append(view.Sessions, sessionRowView{
			Day: p.FormatDate(sess.Day), Label: sess.Label,
		})
	}
	for _, pe := range d.Programme {
		row := programmeRowView{
			EventID:     pe.ID,
			Discipline:  pe.DisciplineName,
			Categories:  strings.Join(pe.CategoryCodes, ", "),
			CaptureType: p.T("family." + string(pe.Family)),
		}
		if row.Discipline == "" {
			row.Discipline = pe.DisciplineCode
		}
		kinds := make([]string, 0, len(pe.Rounds))
		for _, round := range pe.Rounds {
			label := p.T("round." + string(round.Kind))
			kinds = append(kinds, label)
			row.RoundLinks = append(row.RoundLinks, programmeRoundLinkView{RoundID: round.ID, Label: label})
		}
		row.Rounds = strings.Join(kinds, " → ")
		if pe.EntryDeadline != nil {
			row.Deadline = p.FormatDateTime(*pe.EntryDeadline)
		}
		view.Programme = append(view.Programme, row)
	}
	for _, v := range versions {
		view.Versions = append(view.Versions, timetableVersionRowView{
			Version:     v.Version,
			PublishedAt: p.FormatDateTime(v.PublishedAt),
		})
	}
	for _, disc := range s.meets.Catalog().Disciplines {
		view.Disciplines = append(view.Disciplines, disciplineOptionView{Code: disc.Code, Name: disc.Name})
	}
	if scheme, ok := s.meets.Scheme(d.CategorySchemeID); ok {
		for _, cat := range scheme.Categories {
			view.Categories = append(view.Categories, cat.Code)
		}
	}
	return view
}

// resultsPositioningSummary renders the organizer-facing SYS-076 summary
// line for the meet workspace: which channel is the official source, so
// the organizer can see at a glance what their public pages will show.
func resultsPositioningSummary(p PageData, positioning domain.ResultsPositioning, sourceName string) string {
	if positioning == domain.ResultsPositioningPrimary {
		return p.T("meet.results_positioning.primary")
	}
	name := sourceName
	if name == "" {
		name = p.T("meet.results_positioning.source_unconfigured")
	}
	return p.T("meet.results_positioning.federation_official", "source", name)
}

// localizedDisciplineName resolves a discipline code to its localized
// display name for public surfaces (SYS-074): a per-locale
// "discipline.<code>" catalog key if one is shipped, falling back to the
// catalog's canonical (English) name otherwise (PoC scope — see
// docs/requirements/open-questions-and-assumptions.md).
func (s *Server) localizedDisciplineName(p PageData, code string) string {
	if name := p.T("discipline." + code); !strings.HasPrefix(name, "[[") {
		return name
	}
	if disc, ok := s.meets.Catalog().ByCode(code); ok {
		return disc.Name
	}
	return code
}

func (s *Server) sanctioningView(p PageData, sum app.SanctioningSummary) sanctioningView {
	view := sanctioningView{
		MeetName:        sum.Meet.Name,
		Venue:           sum.Meet.Venue,
		HomologationRef: sum.Meet.HomologationRef,
		Dates:           formatDateRange(p, sum.Meet.StartDate, sum.Meet.EndDate),
		Organizer:       sum.Meet.Organizer,
		Tier:            string(sum.Meet.Tier),
		Categories:      strings.Join(sum.Categories, ", "),
		Disciplines:     strings.Join(sum.Disciplines, ", "),
		GeneratedAt:     p.FormatDateTime(sum.GeneratedAt),
	}
	for _, sess := range sum.Sessions {
		view.Sessions = append(view.Sessions, sessionRowView{
			Day: p.FormatDate(sess.Day), Label: sess.Label,
		})
	}
	complete, missing := sum.Complete()
	view.Complete = complete
	view.Missing = missing
	return view
}
