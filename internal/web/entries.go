// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/domain"
)

// --- Online entries (TASK-016, UC-003/UC-006, SYS-011/012/015/017/018):
// individual/club-bulk/relay entry submission (entry-submitter level and
// above), the organizer's entry-standard exception report, bib assignment,
// and the fee schedule/summary. ---

// maxBulkEntryRows/relayLegRows/relayReserveRows size the repeated-row forms
// (mirrors meets.templ's maxSessionRows precedent): blank rows are ignored
// on submit, so these are generous upper bounds, not requirements.
const (
	maxBulkEntryRows = 20
	relayLegRows     = 4
	relayReserveRows = 2
)

// --- view models (templates see only strings/flags, never app/store types) ---

type entryEventOptionView struct {
	ID      string
	Label   string
	IsRelay bool
}

type entryRowView struct {
	EntryID       string
	Version       string
	EventLabel    string
	Who           string // athlete name, or club name for a relay entry
	Seed          string
	Status        string
	FailsStandard bool
	FeeCHF        string
	IsRelay       bool
	RelayTeamID   string
	RelayVersion  string
	Composition   []string
	Reserves      []string
	CanEditRelay  bool
	// EligibilityOutcome is the entry's SYS-014 evaluation (TASK-017,
	// UC-005), blank when there is none to show (relay entries, or an
	// individual entry with no flags at all).
	EligibilityOutcome string
	EligibilityFlags   []string
}

type entriesView struct {
	MeetID          string
	MeetName        string
	Events          []entryEventOptionView
	RelayEvents     []entryEventOptionView
	MyEntries       []entryRowView
	BulkRows        []int
	RelayLegRows    []int
	RelayReserveRow []int
	// IndividualForm carries the individual-entry form's state (OQ-075,
	// UC-038 #4): on a fresh GET this is the zero value (empty fields, no
	// errors); after a failed submit, handleEntryIndividualSubmit overlays
	// the submitted values and per-field errors so the re-rendered page
	// both names what to fix and preserves the operator's input.
	IndividualForm individualEntryFormView
}

// individualEntryFormView is the re-renderable state of the individual
// online-entry form (UC-038 #4's "online entry" representative form).
type individualEntryFormView struct {
	Event                string
	FirstName            string
	LastName             string
	BirthYear            string
	Sex                  string
	Club                 string
	Seed                 string
	PublicationWithdrawn bool
	Errors               FieldErrors
}

type exceptionRowView struct {
	EventLabel string
	Who        string
	Seed       string
	Standard   string
}

type exceptionsView struct {
	MeetID   string
	MeetName string
	Rows     []exceptionRowView
}

type bibRowView struct {
	ParticipantID string
	Version       string
	Bib           string
	Name          string
	Club          string
}

type clubOptionView struct {
	ID, Name string
}

type bibsView struct {
	MeetID   string
	MeetName string
	Rows     []bibRowView
	Clubs    []clubOptionView
	// Query is the DEC-021/TASK-038 search box's current value (the "q"
	// query-param) — filters Rows only; Clubs always lists every club in
	// the meet regardless of search, since the bulk-assign dropdown needs
	// the full set.
	Query string
}

type clubFeeRowView struct {
	ClubName          string
	IndividualEntries int
	RelayEntries      int
	TotalCHF          string
}

type feesView struct {
	MeetID        string
	MeetName      string
	Version       string
	EntryFeeCHF   string
	RelayFeeCHF   string
	Clubs         []clubFeeRowView
	TotalCHF      string
	HasFeeSummary bool
}

// joinStrings renders a leg/reserve name list for the template ("," joined,
// "–" for an empty list rather than a blank cell).
func joinStrings(items []string) string {
	if len(items) == 0 {
		return "–"
	}
	return strings.Join(items, ", ")
}

func intRange(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// centsToCHF renders a Rappen/cents amount as a two-decimal CHF string.
func centsToCHF(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	s := fmt.Sprintf("%d.%02d", cents/100, cents%100)
	if neg {
		s = "-" + s
	}
	return s
}

// chfToCents parses a CHF amount (decimal, "." or "," separator) into
// Rappen/cents. An empty string is zero, not an error.
func chfToCents(s string) (int64, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	if s == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return int64(math.Round(f * 100)), nil
}

func entryEventOptions(opts []app.EntryEventOption, relay bool) []entryEventOptionView {
	var out []entryEventOptionView
	for _, o := range opts {
		if o.IsRelay != relay {
			continue
		}
		out = append(out, entryEventOptionView{ID: o.EventID, Label: o.Label, IsRelay: o.IsRelay})
	}
	return out
}

func (s *Server) entryRowsFrom(p PageData, details []app.EntryDetail) []entryRowView {
	rows := make([]entryRowView, 0, len(details))
	for _, d := range details {
		row := entryRowView{
			EntryID:       d.ID,
			Version:       intToStr(d.Version),
			EventLabel:    s.entryEventLabel(p, d.Event),
			Seed:          d.SeedPerformance,
			Status:        p.T("entry.status." + string(d.Status)),
			FailsStandard: d.FailsStandard,
			FeeCHF:        centsToCHF(d.FeeCents),
		}
		if d.RelayTeam != nil {
			row.IsRelay = true
			row.Who = d.ClubName
			row.RelayTeamID = d.RelayTeam.ID
			row.RelayVersion = intToStr(d.RelayTeam.Version)
			row.Composition = d.RelayTeam.Composition
			row.Reserves = d.RelayTeam.Reserves
			row.CanEditRelay = d.Event.EntryDeadline == nil || time.Now().Before(*d.Event.EntryDeadline)
		} else {
			row.Who = d.AthleteName
			if d.ClubName != "" {
				row.Who += " (" + d.ClubName + ")"
			}
			if d.Eligibility.Outcome != "" {
				row.EligibilityOutcome = p.T("eligibility.outcome." + string(d.Eligibility.Outcome))
				for _, fl := range d.Eligibility.Flags {
					row.EligibilityFlags = append(row.EligibilityFlags, p.T("eligibility.flag."+fl.Code))
				}
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func (s *Server) entryEventLabel(p PageData, e app.EventRecord) string {
	name := s.localizedDisciplineName(p, e.DisciplineCode)
	return name + " (" + strings.Join(e.CategoryCodes, ", ") + ")"
}

func (s *Server) entriesView(r *http.Request, p PageData, actor app.Session, meetID string) (entriesView, error) {
	detail, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		return entriesView{}, err
	}
	opts, err := s.results.OpenEntryEvents(r.Context(), meetID)
	if err != nil {
		return entriesView{}, err
	}
	mine, err := s.results.MyEntries(r.Context(), actor, meetID)
	if err != nil {
		return entriesView{}, err
	}
	return entriesView{
		MeetID: detail.ID, MeetName: detail.Name,
		Events: entryEventOptions(opts, false), RelayEvents: entryEventOptions(opts, true),
		MyEntries: s.entryRowsFrom(p, mine),
		BulkRows:  intRange(maxBulkEntryRows), RelayLegRows: intRange(relayLegRows), RelayReserveRow: intRange(relayReserveRows),
	}, nil
}

func (s *Server) handleEntries(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	p := basePageData(r, s.cats)
	v, err := s.entriesView(r, p, actor, meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p.Title = v.MeetName + " — " + p.T("entries.title")
	if msg := r.URL.Query().Get("err"); msg != "" {
		p.FlashError = p.T("entries.error." + msg)
	}
	_ = entriesPage(p, v).Render(r.Context(), w)
}

// entryFlashKey maps an app-layer entry-submission error onto the
// "entries.error.*" key suffix rendered on redirect.
func entryFlashKey(err error) string {
	switch {
	case errors.Is(err, app.ErrEntryDeadlinePassed):
		return "deadline"
	case errors.Is(err, app.ErrEntryLimitReached):
		return "limit"
	case errors.Is(err, app.ErrSeedPerformanceRequired):
		return "seed_required"
	case errors.Is(err, app.ErrDuplicateEntry):
		return "duplicate"
	case errors.Is(err, app.ErrEntriesClosed):
		return "closed"
	case errors.Is(err, app.ErrNotRelayEntry):
		return "not_relay"
	case errors.Is(err, app.ErrConflict):
		return "conflict"
	default:
		return "invalid"
	}
}

func redirectEntriesError(w http.ResponseWriter, r *http.Request, meetID string, err error) {
	http.Redirect(w, r, "/meets/"+meetID+"/entries?err="+entryFlashKey(err), http.StatusSeeOther)
}

// individualEntryBirthYearBounds mirrors the plausible-birth-year range the
// roster/entry forms already communicate via hint text (entries.hint.birth_year):
// no athlete competing today was born before 1900, and a birth year in the
// future is never valid.
func individualEntryBirthYearBounds() (min, max int) {
	return 1900, time.Now().Year()
}

func (s *Server) handleEntryIndividualSubmit(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	p := basePageData(r, s.cats)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	birthYearStr := strings.TrimSpace(r.FormValue("birth_year"))
	birthYear, _ := strconv.Atoi(birthYearStr)
	form := individualEntryFormView{
		Event:                r.FormValue("event"),
		FirstName:            strings.TrimSpace(r.FormValue("first_name")),
		LastName:             strings.TrimSpace(r.FormValue("last_name")),
		BirthYear:            birthYearStr,
		Sex:                  r.FormValue("sex"),
		Club:                 strings.TrimSpace(r.FormValue("club")),
		Seed:                 strings.TrimSpace(r.FormValue("seed")),
		PublicationWithdrawn: r.FormValue("publication_withdrawn") == "true",
	}

	// OQ-075/UC-038 #4: attribute each validation failure to its own field
	// and re-render with the submitted values intact, rather than
	// redirecting to a page-level flash that loses the operator's input.
	errs := FieldErrors{}
	if form.FirstName == "" {
		errs["first_name"] = p.T("entries.field_error.first_name.required")
	}
	if form.LastName == "" {
		errs["last_name"] = p.T("entries.field_error.last_name.required")
	}
	minYear, maxYear := individualEntryBirthYearBounds()
	if birthYearStr == "" || birthYear < minYear || birthYear > maxYear {
		errs["birth_year"] = p.T("entries.field_error.birth_year.invalid")
	}
	if form.Seed == "" {
		errs["seed"] = p.T("entries.field_error.seed.required")
	}
	if len(errs) > 0 {
		form.Errors = errs
		s.renderEntriesFormError(w, r, p, meetID, form)
		return
	}

	in := app.IndividualEntryInput{
		EventID:         form.Event,
		FirstName:       form.FirstName,
		LastName:        form.LastName,
		BirthYear:       birthYear,
		Sex:             domain.Sex(form.Sex),
		Club:            form.Club,
		SeedPerformance: form.Seed,
		// SYS-103/UC-023 (TASK-023): the entry flow collects the
		// publication-consent choice up front, mirroring the roster form.
		PublicationWithdrawn: form.PublicationWithdrawn,
	}
	if _, err := s.results.SubmitIndividualEntry(r.Context(), actor, meetID, in); err != nil {
		// ErrSeedPerformanceRequired is the one SubmitIndividualEntry
		// business error that names a single field; the rest (deadline
		// passed, entry limit reached, duplicate, entries closed) are
		// whole-form/state conditions with no one field to fix, so they
		// keep the existing page-level flash redirect.
		if errors.Is(err, app.ErrSeedPerformanceRequired) {
			form.Errors = FieldErrors{"seed": p.T("entries.field_error.seed.required")}
			s.renderEntriesFormError(w, r, p, meetID, form)
			return
		}
		redirectEntriesError(w, r, meetID, err)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/entries", http.StatusSeeOther)
}

// renderEntriesFormError re-renders the entries page with the individual
// form's submitted values and field errors overlaid onto an otherwise
// freshly loaded view (open events, the submitter's existing entries).
func (s *Server) renderEntriesFormError(w http.ResponseWriter, r *http.Request, p PageData, meetID string, form individualEntryFormView) {
	actor, _ := sessionFromContext(r.Context())
	v, err := s.entriesView(r, p, actor, meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	v.IndividualForm = form
	p.Title = v.MeetName + " — " + p.T("entries.title")
	w.WriteHeader(http.StatusUnprocessableEntity)
	_ = entriesPage(p, v).Render(r.Context(), w)
}

func (s *Server) handleEntryBulkSubmit(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	in := app.BulkEntryInput{Club: strings.TrimSpace(r.FormValue("club"))}
	for i := 0; i < maxBulkEntryRows; i++ {
		suffix := "_" + strconv.Itoa(i)
		first := strings.TrimSpace(r.FormValue("bulk_first_name" + suffix))
		last := strings.TrimSpace(r.FormValue("bulk_last_name" + suffix))
		if first == "" && last == "" {
			continue
		}
		birthYear, _ := strconv.Atoi(r.FormValue("bulk_birth_year" + suffix))
		in.Lines = append(in.Lines, app.BulkEntryLine{
			FirstName: first, LastName: last, BirthYear: birthYear,
			Sex:             domain.Sex(r.FormValue("bulk_sex" + suffix)),
			EventID:         r.FormValue("bulk_event" + suffix),
			SeedPerformance: strings.TrimSpace(r.FormValue("bulk_seed" + suffix)),
			// SYS-103/UC-023 (TASK-023): per-line consent — per person,
			// never per batch.
			PublicationWithdrawn: r.FormValue("bulk_publication_withdrawn"+suffix) == "true",
		})
	}
	if _, err := s.results.SubmitClubBulkEntries(r.Context(), actor, meetID, in); err != nil {
		redirectEntriesError(w, r, meetID, err)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/entries", http.StatusSeeOther)
}

// relayLegsFromForm reads up to n leg rows named prefix+"_first_name_i" etc.
// from the submitted form; a row is included once either name is non-blank.
func relayLegsFromForm(r *http.Request, prefix string, n int) []app.RelayLegInput {
	var out []app.RelayLegInput
	for i := 0; i < n; i++ {
		suffix := "_" + strconv.Itoa(i)
		first := strings.TrimSpace(r.FormValue(prefix + "_first_name" + suffix))
		last := strings.TrimSpace(r.FormValue(prefix + "_last_name" + suffix))
		if first == "" && last == "" {
			continue
		}
		birthYear, _ := strconv.Atoi(r.FormValue(prefix + "_birth_year" + suffix))
		out = append(out, app.RelayLegInput{
			FirstName: first, LastName: last, BirthYear: birthYear,
			Sex: domain.Sex(r.FormValue(prefix + "_sex" + suffix)),
			// SYS-103/UC-023 (TASK-023): per-leg consent — each relay
			// member is a natural person with their own choice.
			PublicationWithdrawn: r.FormValue(prefix+"_publication_withdrawn"+suffix) == "true",
		})
	}
	return out
}

func (s *Server) handleEntryRelaySubmit(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	in := app.RelayEntryInput{
		EventID:     r.FormValue("relay_event"),
		Club:        strings.TrimSpace(r.FormValue("relay_club")),
		Composition: relayLegsFromForm(r, "leg", relayLegRows),
		Reserves:    relayLegsFromForm(r, "reserve", relayReserveRows),
	}
	if _, err := s.results.SubmitRelayEntry(r.Context(), actor, meetID, in); err != nil {
		redirectEntriesError(w, r, meetID, err)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/entries", http.StatusSeeOther)
}

func (s *Server) handleEntryRelayCompositionUpdate(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	entryID := r.PathValue("entry")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	version, _ := strconv.ParseInt(r.FormValue("relay_version"), 10, 64)
	composition := relayLegsFromForm(r, "edit_leg", relayLegRows)
	reserves := relayLegsFromForm(r, "edit_reserve", relayReserveRows)
	if _, err := s.results.UpdateRelayComposition(r.Context(), actor, meetID, entryID, version, composition, reserves); err != nil {
		redirectEntriesError(w, r, meetID, err)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/entries", http.StatusSeeOther)
}

// --- entry-standard exception report (UC-003 #5, SYS-015) ---

func (s *Server) handleEntryExceptions(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	detail, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	failing, err := s.results.EntryExceptions(r.Context(), actor, meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = detail.Name + " — " + p.T("entries.exceptions.title")
	v := exceptionsView{MeetID: detail.ID, MeetName: detail.Name}
	for _, d := range failing {
		v.Rows = append(v.Rows, exceptionRowView{
			EventLabel: s.entryEventLabel(p, d.Event),
			Who:        d.AthleteName,
			Seed:       d.SeedPerformance,
			Standard:   d.Event.EntryStandard,
		})
	}
	_ = entryExceptionsPage(p, v).Render(r.Context(), w)
}

// --- bib assignment (UC-006 #1/#2, SYS-018) ---

func (s *Server) bibsView(r *http.Request, meetID string) (bibsView, error) {
	detail, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		return bibsView{}, err
	}
	participants, err := s.results.Participants(r.Context(), meetID)
	if err != nil {
		return bibsView{}, err
	}
	clubs, err := s.results.ClubNamesFor(r.Context(), participants)
	if err != nil {
		return bibsView{}, err
	}
	q := searchQuery(r)
	v := bibsView{MeetID: detail.ID, MeetName: detail.Name, Query: q}
	seen := map[string]bool{}
	for _, p := range participants {
		club := ""
		if len(p.Athlete.ClubIDs) > 0 {
			id := p.Athlete.ClubIDs[0]
			club = clubs[id]
			if !seen[id] && club != "" {
				seen[id] = true
				v.Clubs = append(v.Clubs, clubOptionView{ID: id, Name: club})
			}
		}
		if !app.MatchesParticipantSearch(q, p.Athlete.FirstName, p.Athlete.LastName, p.Bib, club) {
			continue
		}
		v.Rows = append(v.Rows, bibRowView{
			ParticipantID: p.ID, Version: intToStr(p.Version), Bib: p.Bib,
			Name: p.Athlete.FirstName + " " + p.Athlete.LastName, Club: club,
		})
	}
	sort.Slice(v.Clubs, func(i, j int) bool { return v.Clubs[i].Name < v.Clubs[j].Name })
	return v, nil
}

func (s *Server) handleBibs(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	v, err := s.bibsView(r, meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = v.MeetName + " — " + p.T("bibs.title")
	if msg := r.URL.Query().Get("err"); msg != "" {
		p.FlashError = p.T("bibs.error." + msg)
	}
	_ = bibsPage(p, v).Render(r.Context(), w)
}

func bibFlashKey(err error) string {
	switch {
	case errors.Is(err, app.ErrBibRequired):
		return "required"
	case errors.Is(err, app.ErrDuplicateParticipant):
		return "duplicate"
	case errors.Is(err, app.ErrConflict):
		return "conflict"
	default:
		return "invalid"
	}
}

func (s *Server) handleBibAssign(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	participantID := r.PathValue("participant")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	version, _ := strconv.ParseInt(r.FormValue("version"), 10, 64)
	bib := strings.TrimSpace(r.FormValue("bib"))
	if err := s.results.AssignBib(r.Context(), actor, meetID, participantID, version, bib); err != nil {
		http.Redirect(w, r, "/meets/"+meetID+"/bibs?err="+bibFlashKey(err), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/bibs", http.StatusSeeOther)
}

func (s *Server) handleBibBulkAssign(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	clubID := r.FormValue("club")
	start, _ := strconv.Atoi(r.FormValue("start"))
	if _, err := s.results.BulkAssignBibsByClub(r.Context(), actor, meetID, clubID, start); err != nil {
		http.Redirect(w, r, "/meets/"+meetID+"/bibs?err="+bibFlashKey(err), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/bibs", http.StatusSeeOther)
}

// handleBibsPDF renders the printable bib list, one section per club plus a
// trailing section for unaffiliated participants (UC-006 #1: "a printable
// bib list per club exists").
func (s *Server) handleBibsPDF(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	v, err := s.bibsView(r, meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	data, err := pdfBuildBibList(p, v.MeetName, v.Rows)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writePDF(w, data, "bib-list.pdf")
}
