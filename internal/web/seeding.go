// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
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
	_, _ = s.results.GenerateHeats(r.Context(), actor, meetID, eventID, roundID, app.GenerateHeatsRequest{
		MaxHeatSize: maxHeatSize, TrackLanes: trackLanes,
	})
	http.Redirect(w, r, seedingRedirect(meetID, eventID, roundID), http.StatusSeeOther)
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
	_ = s.results.OverrideAssignment(r.Context(), actor, meetID, eventID, roundID, entryID, targetUnit, lane, version)
	http.Redirect(w, r, seedingRedirect(meetID, eventID, roundID), http.StatusSeeOther)
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
	dest := seedingRedirect(meetID, eventID, roundID)
	if err == nil && outcome.Tie != nil {
		dest += "?tie_mark=" + outcome.Tie.Mark +
			"&tie_entries=" + strings.Join(outcome.Tie.EntryIDs, ",") +
			"&tie_slots=" + strconv.Itoa(outcome.Tie.RemainingSlots)
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
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
	_ = s.results.ManualAdvance(r.Context(), actor, meetID, eventID, roundID, entryID, domain.QualificationStatus(code))
	http.Redirect(w, r, seedingRedirect(meetID, eventID, roundID), http.StatusSeeOther)
}
