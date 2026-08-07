// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/domain"
)

// --- Heat seeding, lane draws and round progression (TASK-018, UC-008/009,
// SYS-026-030): office-level workspace per event/round. ---

type heatSheetRowView struct {
	EntryID        string
	Version        string
	Who            string
	ClubName       string
	SeedMark       string
	Lane           string
	Qualification  string
	ManualOverride bool
}

type heatSheetUnitView struct {
	UnitID string
	Label  string
	Rows   []heatSheetRowView
}

// tieEntryView is one tied entry the operator must manually resolve
// (UC-009 #2): its display name (resolved from the current heat sheet,
// where available) and a ready-to-submit manual-advance mini-form.
type tieEntryView struct {
	EntryID string
	Who     string
}

type tieView struct {
	Mark           string
	RemainingSlots string
	Entries        []tieEntryView
}

type seedingView struct {
	MeetID       string
	MeetName     string
	EventID      string
	EventLabel   string
	RoundID      string
	RoundLabel   string
	RulesID      string
	RulesVersion string
	Units        []heatSheetUnitView
	UnitOptions  []unitOptionView
	Tie          *tieView
}

type unitOptionView struct {
	UnitID string
	Label  string
}

func (s *Server) seedingView(r *http.Request, p PageData, actor app.Session, meetID, eventID, roundID string) (seedingView, error) {
	meet, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		return seedingView{}, err
	}
	ev, err := s.results.EventDetail(r.Context(), actor, meetID, eventID)
	if err != nil {
		return seedingView{}, err
	}
	label := ev.DisciplineName
	if label == "" {
		label = ev.DisciplineCode
	}
	sheet, err := s.results.HeatSheetFor(r.Context(), actor, meetID, eventID, roundID)
	if err != nil {
		return seedingView{}, err
	}
	v := seedingView{
		MeetID: meet.ID, MeetName: meet.Name, EventID: eventID, EventLabel: label,
		RoundID: roundID, RoundLabel: p.T("round." + string(sheet.RoundKind)),
		RulesID: sheet.RulesID, RulesVersion: sheet.RulesVersion,
	}
	whoByEntry := map[string]string{}
	for i, u := range sheet.Units {
		uv := heatSheetUnitView{UnitID: u.UnitID, Label: p.T("seeding.heat") + " " + strconv.Itoa(i+1)}
		v.UnitOptions = append(v.UnitOptions, unitOptionView{UnitID: u.UnitID, Label: uv.Label})
		for _, row := range u.Rows {
			whoByEntry[row.EntryID] = row.AthleteName
			lane := ""
			if row.Lane != 0 {
				lane = strconv.Itoa(row.Lane)
			}
			uv.Rows = append(uv.Rows, heatSheetRowView{
				EntryID: row.EntryID, Version: strconv.FormatInt(row.Version, 10),
				Who: row.AthleteName, ClubName: row.ClubName, SeedMark: row.SeedMark,
				Lane: lane, Qualification: string(row.Qualification), ManualOverride: row.ManualOverride,
			})
		}
		v.Units = append(v.Units, uv)
	}

	if mark := r.URL.Query().Get("tie_mark"); mark != "" {
		tie := &tieView{Mark: mark, RemainingSlots: r.URL.Query().Get("tie_slots")}
		for _, id := range strings.Split(r.URL.Query().Get("tie_entries"), ",") {
			if id == "" {
				continue
			}
			tie.Entries = append(tie.Entries, tieEntryView{EntryID: id, Who: whoByEntry[id]})
		}
		v.Tie = tie
	}
	return v, nil
}

func (s *Server) handleSeeding(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID, eventID, roundID := r.PathValue("id"), r.PathValue("event"), r.PathValue("round")
	p := basePageData(r, s.cats)
	v, err := s.seedingView(r, p, actor, meetID, eventID, roundID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p.Title = v.MeetName + " — " + p.T("seeding.title")
	_ = seedingPage(p, v).Render(r.Context(), w)
}

func seedingRedirect(meetID, eventID, roundID string) string {
	return "/meets/" + meetID + "/events/" + eventID + "/rounds/" + roundID + "/seeding"
}

func (s *Server) handleSeedingGenerate(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID, eventID, roundID := r.PathValue("id"), r.PathValue("event"), r.PathValue("round")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	maxHeatSize, _ := strconv.Atoi(r.FormValue("max_heat_size"))
	trackLanes, _ := strconv.Atoi(r.FormValue("track_lanes"))
	if _, err := s.results.GenerateHeats(r.Context(), actor, meetID, eventID, roundID, app.GenerateHeatsRequest{
		MaxHeatSize: maxHeatSize, TrackLanes: trackLanes,
	}); err != nil {
		if _, forbidden := err.(app.ErrForbidden); forbidden {
			renderForbidden(w, r, s.cats)
			return
		}
		s.renderSeedingError(w, r, meetID, eventID, roundID, seedingGenerateFlashKey(err))
		return
	}
	http.Redirect(w, r, seedingRedirect(meetID, eventID, roundID), http.StatusSeeOther)
}

// seedingGenerateFlashKey maps a GenerateHeats error onto the
// "seeding.generate.error.*" key the seeding page's page-level alert
// renders (TASK-054/OQ-140): the known empty/insufficient-pool rejection
// (no confirmed/checked-in entries to seed) gets its own actionable
// message telling the operator what to do — confirm or check in entries
// first — matching SYS-117's "state what to fix"; anything else falls back
// to a generic actionable message instead of stringifying the raw service
// error into the UI (mirrors participantIdentityFlashKey's precedent in
// standings.go).
func seedingGenerateFlashKey(err error) string {
	if errors.Is(err, app.ErrEmptySeedingPool) {
		return "seeding.generate.error.empty_pool"
	}
	return "seeding.generate.error.failed"
}

// renderSeedingError re-renders the seeding page with a page-level flash
// error (SYS-152: distinguishable from the unchanged "no heats yet" empty
// state) instead of silently redirecting as if nothing happened
// (TASK-054/OQ-140) — mirrors renderCaptureError's precedent in capture.go.
func (s *Server) renderSeedingError(w http.ResponseWriter, r *http.Request, meetID, eventID, roundID, key string) {
	actor, _ := sessionFromContext(r.Context())
	p := basePageData(r, s.cats)
	v, err := s.seedingView(r, p, actor, meetID, eventID, roundID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p.Title = v.MeetName + " — " + p.T("seeding.title")
	p.FlashError = p.T(key)
	w.WriteHeader(http.StatusUnprocessableEntity)
	_ = seedingPage(p, v).Render(r.Context(), w)
}

func (s *Server) handleSeedingOverride(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID, eventID, roundID := r.PathValue("id"), r.PathValue("event"), r.PathValue("round")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	entryID := r.FormValue("entry_id")
	targetUnit := r.FormValue("target_unit")
	lane, _ := strconv.Atoi(r.FormValue("lane"))
	version, _ := strconv.ParseInt(r.FormValue("version"), 10, 64)
	if err := s.results.OverrideAssignment(r.Context(), actor, meetID, eventID, roundID, entryID, targetUnit, lane, version); err != nil {
		if _, forbidden := err.(app.ErrForbidden); forbidden {
			renderForbidden(w, r, s.cats)
			return
		}
		s.renderSeedingError(w, r, meetID, eventID, roundID, seedingOverrideFlashKey(err))
		return
	}
	http.Redirect(w, r, seedingRedirect(meetID, eventID, roundID), http.StatusSeeOther)
}

// seedingOverrideFlashKey maps an OverrideAssignment error onto the
// "seeding.override.error.*" key the seeding page's page-level alert
// renders (TASK-055): a stale expectedVersion — another session already
// moved this entry — gets the same "reload and retry" conflict wording used
// across the roster/standings/entries/bibs/fees/meet surfaces; anything else
// falls back to one generic actionable message rather than stringifying the
// raw service error into the UI.
func seedingOverrideFlashKey(err error) string {
	if errors.Is(err, app.ErrConflict) {
		return "seeding.override.error.conflict"
	}
	return "seeding.override.error.failed"
}

func (s *Server) handleAdvanceRound(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID, eventID, roundID := r.PathValue("id"), r.PathValue("event"), r.PathValue("round")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	topN, _ := strconv.Atoi(r.FormValue("top_n"))
	fastestK, _ := strconv.Atoi(r.FormValue("fastest_k"))
	capacity, _ := strconv.Atoi(r.FormValue("finals_capacity"))
	outcome, err := s.results.AdvanceRound(r.Context(), actor, meetID, eventID, roundID, app.AdvancementRequest{
		TopN: topN, FastestK: fastestK,
		Standard: strings.TrimSpace(r.FormValue("standard")), BetterDirection: r.FormValue("better_direction"),
		FinalsCapacity: capacity,
	})
	if err != nil {
		if _, forbidden := err.(app.ErrForbidden); forbidden {
			renderForbidden(w, r, s.cats)
			return
		}
		s.renderSeedingError(w, r, meetID, eventID, roundID, seedingAdvanceFlashKey(err))
		return
	}
	dest := seedingRedirect(meetID, eventID, roundID)
	if outcome.Tie != nil {
		dest += "?tie_mark=" + outcome.Tie.Mark +
			"&tie_entries=" + strings.Join(outcome.Tie.EntryIDs, ",") +
			"&tie_slots=" + strconv.Itoa(outcome.Tie.RemainingSlots)
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

// seedingAdvanceFlashKey maps an AdvanceRound error onto the
// "seeding.advance.error.*" key the seeding page's page-level alert renders
// (TASK-055): the two known, operator-actionable rejections — a track round
// with a heat that has no settled results yet (ErrRoundNotComplete) and a
// round that was never seeded at all (ErrRoundNotSeeded) — each get their
// own message telling the operator what to do first; anything else falls
// back to one generic actionable message.
func seedingAdvanceFlashKey(err error) string {
	switch {
	case errors.Is(err, app.ErrRoundNotComplete):
		return "seeding.advance.error.incomplete"
	case errors.Is(err, app.ErrRoundNotSeeded):
		return "seeding.advance.error.not_seeded"
	default:
		return "seeding.advance.error.failed"
	}
}

func (s *Server) handleManualAdvance(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID, eventID, roundID := r.PathValue("id"), r.PathValue("event"), r.PathValue("round")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	entryID := r.FormValue("entry_id")
	code := r.FormValue("code")
	if err := s.results.ManualAdvance(r.Context(), actor, meetID, eventID, roundID, entryID, domain.QualificationStatus(code)); err != nil {
		if _, forbidden := err.(app.ErrForbidden); forbidden {
			renderForbidden(w, r, s.cats)
			return
		}
		s.renderSeedingError(w, r, meetID, eventID, roundID, seedingManualAdvanceFlashKey(err))
		return
	}
	http.Redirect(w, r, seedingRedirect(meetID, eventID, roundID), http.StatusSeeOther)
}

// seedingManualAdvanceFlashKey maps a ManualAdvance error onto the
// "seeding.manual_advance.error.*" key the seeding page's page-level alert
// renders (TASK-055): an illegal manual-qualification code (not one of
// Q/q/qR/qJ/qD) and an entry that is no longer seeded in this round (the
// round changed since the tie panel was rendered) each get their own
// actionable message; anything else falls back to one generic message.
func seedingManualAdvanceFlashKey(err error) string {
	switch {
	case errors.Is(err, app.ErrInvalidQualificationCode):
		return "seeding.manual_advance.error.invalid_code"
	case errors.Is(err, app.ErrEntryNotInRound):
		return "seeding.manual_advance.error.entry_not_in_round"
	default:
		return "seeding.manual_advance.error.failed"
	}
}
