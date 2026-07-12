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

// --- field & track capture (TASK-008: UC-011, UC-010 subset) ---

type captureUnitView struct {
	UnitID     string
	Discipline string
	Family     string // i18n key suffix (family.track, …)
}

type captureIndexView struct {
	MeetID   string
	MeetName string
	Units    []captureUnitView
}

// handleCaptureIndex lists the units a field official can open for entry.
func (s *Server) handleCaptureIndex(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	detail, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	actor, _ := sessionFromContext(r.Context())
	units, err := s.results.CaptureUnits(r.Context(), actor, meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	v := captureIndexView{MeetID: detail.ID, MeetName: detail.Name}
	for _, u := range units {
		v.Units = append(v.Units, captureUnitView{
			UnitID:     u.UnitID,
			Discipline: u.DisciplineName,
			Family:     string(u.Family),
		})
	}
	p := basePageData(r, s.cats)
	p.Title = v.MeetName + " — " + p.T("capture.title")
	_ = captureIndexPage(p, v).Render(r.Context(), w)
}

// attemptCellView is one trial cell of the horizontal-attempt grid: the
// stored state (for display) plus what the correction form needs.
type attemptCellView struct {
	Seq     string
	Display string // "6.12", "X", "–", "r" — "" when not yet captured
	Value   string // pre-fill for the input field
	Wind    string
	Version string // stored attempt version; "0" = new capture
}

type captureRowView struct {
	AthleteID string
	Bib       string
	Name      string
	Cells     []attemptCellView
	Result    string // settled: best mark or rendered status
	Points    string
	// Lane is the heat-seeded lane context (TASK-018, SYS-026/027), "" when
	// none (non-laned event, or the unit is not seeded yet).
	Lane string
}

type standingRowView2 struct {
	Rank   string // "" for unranked rows
	Name   string
	Bib    string
	Mark   string // provenance-marked (SYS-041)
	Status string
	Points string
}

type captureView struct {
	MeetID       string
	MeetName     string
	UnitID       string
	Discipline   string
	IsField      bool
	WindRelevant bool
	Attempts     int
	CutTo        int
	Standings    []standingRowView2
	Continuation []string // bib+name in competing order after the cut
	Rows         []captureRowView
	// Track form state (UC-010 subset).
	TrackTimings  []string
	TrackStatuses []string
	// CurrentWind is the unit's stored per-race wind reading (SYS-040,
	// UC-010 #4), "" when unset; only meaningful when WindRelevant.
	CurrentWind string
	// Protest is the unit's SYS-047 protest-clock state (UC-015 #1/#4).
	Protest protestView
	// CanOffice gates the office-only announce/correction controls
	// (UC-015's "operator (competition office)" actor) — read-only display
	// context, the server enforces the real authorization on submit.
	CanOffice bool
	// TrackFormAction is the track row forms' POST target: "track" (plain
	// capture) before announcement, "correct" (reason/escalation required,
	// office-only) once the unit's results are announced (UC-015 #2).
	TrackFormAction string
}

// protestView is the capture page's SYS-047 protest-clock display: whether
// the unit's results have been announced, and if so whether the 30-minute
// window is still open (provisional) or has elapsed (official, UC-015 #4).
type protestView struct {
	Announced      bool
	Official       bool
	RemainingMin   string
	CorrectionMode bool // Announced && !Official-agnostic: any edit past announcement is a correction
}

func buildProtestView(state domain.ProtestState) protestView {
	pv := protestView{Announced: state.Announced, Official: state.Official, CorrectionMode: state.Announced}
	if state.Announced && !state.Official {
		mins := int(state.Remaining / time.Minute)
		if state.Remaining%time.Minute != 0 {
			mins++ // round up so "0 min left" never shows while still open
		}
		pv.RemainingMin = strconv.Itoa(mins)
	}
	return pv
}

func (s *Server) captureView(r *http.Request, meetID, unitID string) (captureView, error) {
	uc, err := s.results.UnitCapture(r.Context(), meetID, unitID)
	if err != nil {
		return captureView{}, err
	}
	protestState, err := s.results.UnitProtestState(r.Context(), meetID, unitID)
	if err != nil {
		return captureView{}, err
	}
	trackAction := "track"
	if protestState.Announced {
		trackAction = "correct"
	}
	v := captureView{
		MeetID:          uc.Meet.ID,
		MeetName:        uc.Meet.Name,
		Protest:         buildProtestView(protestState),
		UnitID:          unitID,
		Discipline:      uc.DisciplineName,
		IsField:         uc.Family == domain.FamilyFieldHorizontal,
		WindRelevant:    uc.WindRelevant,
		Attempts:        uc.Config.Attempts,
		CutTo:           uc.Config.CutTo,
		TrackTimings:    []string{string(domain.TimingManual), string(domain.TimingElectronic)},
		TrackStatuses:   []string{string(domain.StatusDNS), string(domain.StatusDNF), string(domain.StatusDQ)},
		TrackFormAction: trackAction,
	}
	if actor, ok := sessionFromContext(r.Context()); ok {
		v.CanOffice = actor.Role.AtLeast(app.RoleCompetitionOffice)
	}
	if v.WindRelevant {
		if wind, err := s.results.UnitWind(r.Context(), meetID, unitID); err != nil {
			return captureView{}, err
		} else if wind != nil {
			v.CurrentWind = strconv.FormatFloat(*wind, 'f', 1, 64)
		}
	}

	names := map[string]string{}
	bibs := map[string]string{}
	for _, row := range uc.Rows {
		names[row.AthleteID] = row.FirstName + " " + row.LastName
		bibs[row.AthleteID] = row.Bib
		rv := captureRowView{
			AthleteID: row.AthleteID,
			Bib:       row.Bib,
			Name:      row.FirstName + " " + row.LastName,
		}
		if row.Lane != 0 {
			rv.Lane = strconv.Itoa(row.Lane)
		}
		for i, a := range row.Attempts {
			cell := attemptCellView{Seq: strconv.Itoa(i + 1), Version: "0"}
			if a != nil {
				cell.Display = a.Display()
				cell.Value = a.Display()
				cell.Version = strconv.FormatInt(a.Version, 10)
				if a.Wind != nil {
					cell.Wind = strconv.FormatFloat(*a.Wind, 'f', 1, 64)
				}
			}
			rv.Cells = append(rv.Cells, cell)
		}
		if res := row.Result; res != nil {
			rv.Result = markWithProvenance(res.Mark, res.Timing)
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
		rv := standingRowView2{
			Name:   names[st.AthleteID],
			Bib:    bibs[st.AthleteID],
			Mark:   markWithProvenance(st.Mark, st.Timing),
			Status: domain.RenderStatus(st.Status, st.StatusDetail),
		}
		if st.Rank > 0 {
			rv.Rank = strconv.Itoa(st.Rank)
		}
		if st.Points != nil {
			rv.Points = strconv.Itoa(*st.Points)
		}
		v.Standings = append(v.Standings, rv)
	}
	for _, athleteID := range uc.Continuation {
		v.Continuation = append(v.Continuation, strings.TrimSpace(bibs[athleteID]+" "+names[athleteID]))
	}
	return v, nil
}

// markWithProvenance renders a mark with its timing provenance: hand times
// carry the conventional "h" marker so hand and FAT stay distinguishable in
// every output (SYS-041, D5.1).
func markWithProvenance(mark string, timing domain.Timing) string {
	if mark != "" && timing == domain.TimingManual {
		return mark + " h"
	}
	return mark
}

func (s *Server) handleCaptureUnit(w http.ResponseWriter, r *http.Request) {
	meetID, unitID := r.PathValue("id"), r.PathValue("unit")
	actor, ok := sessionFromContext(r.Context())
	if ok {
		// Per-event scoping (TASK-013, SYS-090/UC-022 #1): a field official
		// may only open a unit assigned to them — checked before anything
		// else so an unassigned unit is denied, not just checked-out-and-
		// then-shown.
		if err := s.results.CheckUnitAccess(r.Context(), actor, meetID, unitID); err != nil {
			renderForbidden(w, r, s.cats)
			return
		}
		// Opening a unit takes its capture lock for the field official
		// (SYS-086): best-effort — a busy lock must not block the read of
		// the capture page (the office reconciles a contested lock).
		_, _ = s.results.EnsureCheckout(r.Context(), actor, meetID, unitID, deviceLabelOr(r.Header.Get("X-Device-Label")))
	}
	v, err := s.captureView(r, meetID, unitID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = v.MeetName + " — " + v.Discipline
	_ = capturePage(p, v).Render(r.Context(), w)
}

// handleCaptureStandings serves the standings fragment the capture page's
// live refresh swaps in on SSE "results" events (UC-011 #4) — only the
// standings, so a refresh never clobbers a half-typed attempt form.
func (s *Server) handleCaptureStandings(w http.ResponseWriter, r *http.Request) {
	meetID, unitID := r.PathValue("id"), r.PathValue("unit")
	if actor, ok := sessionFromContext(r.Context()); ok {
		if err := s.results.CheckUnitAccess(r.Context(), actor, meetID, unitID); err != nil {
			renderForbidden(w, r, s.cats)
			return
		}
	}
	v, err := s.captureView(r, meetID, unitID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	_ = captureStandings(p, v).Render(r.Context(), w)
}

// parseAttemptValue maps the grid's single-input notation to an attempt:
// a distance, or the D5.2 symbols X (foul), – (pass), r (retirement).
func parseAttemptValue(value string) (domain.AttemptKind, string) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "x":
		return domain.AttemptFoul, ""
	case "-", "–":
		return domain.AttemptPass, ""
	case "r":
		return domain.AttemptRetire, ""
	default:
		return domain.AttemptValid, strings.TrimSpace(value)
	}
}

func (s *Server) handleCaptureAttempt(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID, unitID := r.PathValue("id"), r.PathValue("unit")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	kind, mark := parseAttemptValue(r.FormValue("value"))
	seq, _ := strconv.Atoi(r.FormValue("seq"))
	version, _ := strconv.ParseInt(r.FormValue("version"), 10, 64)
	in := app.FieldAttemptInput{
		AthleteID:       r.FormValue("athlete"),
		Seq:             seq,
		Kind:            kind,
		Mark:            mark,
		ExpectedVersion: version,
	}
	if windStr := strings.TrimSpace(r.FormValue("wind")); windStr != "" {
		wind, err := strconv.ParseFloat(strings.ReplaceAll(windStr, ",", "."), 64)
		if err != nil {
			s.renderCaptureError(w, r, meetID, unitID, "capture.error.invalid", "")
			return
		}
		in.Wind = &wind
	}

	if _, err := s.results.SaveFieldAttempt(r.Context(), actor, meetID, unitID, in); err != nil {
		var conflict *app.AttemptConflictError
		switch {
		case errors.Is(err, app.ErrUnitNotAssigned):
			renderForbidden(w, r, s.cats)
		case errors.As(err, &conflict):
			s.renderCaptureError(w, r, meetID, unitID, "capture.error.conflict", conflict.Current.Display())
		default:
			s.renderCaptureError(w, r, meetID, unitID, "capture.error.invalid", "")
		}
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/capture/"+unitID, http.StatusSeeOther)
}

func (s *Server) handleCaptureTrack(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID, unitID := r.PathValue("id"), r.PathValue("unit")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	in := app.TrackResultInput{
		AthleteID:    r.FormValue("athlete"),
		Time:         strings.TrimSpace(r.FormValue("time")),
		Timing:       domain.Timing(r.FormValue("timing")),
		Status:       domain.QualificationStatus(r.FormValue("status")),
		StatusDetail: strings.TrimSpace(r.FormValue("status_detail")),
	}
	if _, err := s.results.SaveTrackResult(r.Context(), actor, meetID, unitID, in); err != nil {
		switch {
		case errors.Is(err, app.ErrUnitNotAssigned):
			renderForbidden(w, r, s.cats)
		case errors.Is(err, app.ErrCorrectionRequired):
			s.renderCaptureError(w, r, meetID, unitID, "capture.error.correction_required", "")
		default:
			s.renderCaptureError(w, r, meetID, unitID, "capture.error.invalid", "")
		}
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/capture/"+unitID, http.StatusSeeOther)
}

// handleCaptureCorrect amends a settled result on an already-announced unit
// (UC-015 #2/#3): office-only (routed under the office role floor), a
// reason is mandatory (SYS-046) and an escalation reference is additionally
// required once the protest window has elapsed (SYS-047).
func (s *Server) handleCaptureCorrect(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID, unitID := r.PathValue("id"), r.PathValue("unit")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	in := app.CorrectionInput{
		Mark:         strings.TrimSpace(r.FormValue("time")),
		Timing:       domain.Timing(r.FormValue("timing")),
		Status:       domain.QualificationStatus(r.FormValue("status")),
		StatusDetail: strings.TrimSpace(r.FormValue("status_detail")),
		Reason:       strings.TrimSpace(r.FormValue("reason")),
		Escalation:   strings.TrimSpace(r.FormValue("escalation")),
	}
	if _, err := s.results.CorrectResult(r.Context(), actor, meetID, unitID, r.FormValue("athlete"), in); err != nil {
		switch {
		case errors.Is(err, app.ErrCorrectionReasonRequired):
			s.renderCaptureError(w, r, meetID, unitID, "capture.error.reason_required", "")
		case errors.Is(err, app.ErrEscalationRequired):
			s.renderCaptureError(w, r, meetID, unitID, "capture.error.escalation_required", "")
		default:
			if _, forbidden := err.(app.ErrForbidden); forbidden {
				renderForbidden(w, r, s.cats)
				return
			}
			s.renderCaptureError(w, r, meetID, unitID, "capture.error.invalid", "")
		}
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/capture/"+unitID, http.StatusSeeOther)
}

// handleCaptureAnnounce posts a unit's current result list (UC-015 #1):
// office-only, starts the 30-minute protest window (SYS-047).
func (s *Server) handleCaptureAnnounce(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID, unitID := r.PathValue("id"), r.PathValue("unit")
	if _, err := s.results.AnnounceUnitResults(r.Context(), actor, meetID, unitID); err != nil {
		if _, forbidden := err.(app.ErrForbidden); forbidden {
			renderForbidden(w, r, s.cats)
			return
		}
		s.renderCaptureError(w, r, meetID, unitID, "capture.error.invalid", "")
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/capture/"+unitID, http.StatusSeeOther)
}

// handleCaptureWind records the unit's single per-race wind reading
// (SYS-040, UC-010 #4): field-official capture-scoped, same as ordinary
// track/field capture — a non-wind-relevant discipline is rejected.
func (s *Server) handleCaptureWind(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID, unitID := r.PathValue("id"), r.PathValue("unit")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	windStr := strings.ReplaceAll(strings.TrimSpace(r.FormValue("wind")), ",", ".")
	wind, err := strconv.ParseFloat(windStr, 64)
	if err != nil {
		s.renderCaptureError(w, r, meetID, unitID, "capture.error.invalid", "")
		return
	}
	if err := s.results.SetUnitWind(r.Context(), actor, meetID, unitID, wind); err != nil {
		switch {
		case errors.Is(err, app.ErrUnitNotAssigned):
			renderForbidden(w, r, s.cats)
		default:
			if _, forbidden := err.(app.ErrForbidden); forbidden {
				renderForbidden(w, r, s.cats)
				return
			}
			s.renderCaptureError(w, r, meetID, unitID, "capture.error.invalid", "")
		}
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/capture/"+unitID, http.StatusSeeOther)
}

// renderCaptureError re-renders the capture page with a localized flash;
// arg (when non-empty) is substituted as {stored} (the conflict message
// shows what the other session captured, UC-021 #2).
func (s *Server) renderCaptureError(w http.ResponseWriter, r *http.Request, meetID, unitID, key, arg string) {
	v, err := s.captureView(r, meetID, unitID)
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
	_ = capturePage(p, v).Render(r.Context(), w)
}
