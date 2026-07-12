// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"net/http"
	"strconv"

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/domain"
)

// --- Check-in / call-room and DNS handling (TASK-018, UC-007, SYS-025):
// confirm or DNS entries per event before seeding. Competition-office level
// and above (mirrors routes.go's `office` wrapper). ---

type checkInRowView struct {
	EntryID     string
	Version     string
	Who         string
	ClubName    string
	Status      string
	IsEntered   bool
	IsConfirmed bool
	IsDNS       bool
}

type checkInView struct {
	MeetID     string
	MeetName   string
	EventID    string
	EventLabel string
	Rows       []checkInRowView
}

func (s *Server) checkInView(r *http.Request, p PageData, actor app.Session, meetID, eventID string) (checkInView, error) {
	meet, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		return checkInView{}, err
	}
	ev, err := s.results.EventDetail(r.Context(), actor, meetID, eventID)
	if err != nil {
		return checkInView{}, err
	}
	label := ev.DisciplineName
	if label == "" {
		label = ev.DisciplineCode
	}
	rows, err := s.results.CheckInRoster(r.Context(), actor, meetID, eventID)
	if err != nil {
		return checkInView{}, err
	}
	v := checkInView{MeetID: meet.ID, MeetName: meet.Name, EventID: eventID, EventLabel: label}
	for _, row := range rows {
		who := row.AthleteName
		if row.RelayTeam != nil {
			who = row.ClubName + " (" + p.T("entries.relay.title") + ")"
		}
		v.Rows = append(v.Rows, checkInRowView{
			EntryID: row.ID, Version: strconv.FormatInt(row.Version, 10),
			Who: who, ClubName: row.ClubName, Status: p.T("entry.status." + string(row.Status)),
			IsEntered: row.Status == domain.EntryEntered, IsConfirmed: row.Status == domain.EntryConfirmed,
			IsDNS: row.Status == domain.EntryDNS,
		})
	}
	return v, nil
}

func (s *Server) handleCheckIn(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID, eventID := r.PathValue("id"), r.PathValue("event")
	p := basePageData(r, s.cats)
	v, err := s.checkInView(r, p, actor, meetID, eventID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p.Title = v.MeetName + " — " + p.T("checkin.title")
	_ = checkInPage(p, v).Render(r.Context(), w)
}

func (s *Server) handleCheckInConfirm(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID, entryID := r.PathValue("id"), r.PathValue("entry")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	version, _ := strconv.ParseInt(r.FormValue("version"), 10, 64)
	if err := s.results.ConfirmCheckIn(r.Context(), actor, meetID, entryID, version); err != nil {
		http.Redirect(w, r, r.Header.Get("Referer"), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, r.Header.Get("Referer"), http.StatusSeeOther)
}

func (s *Server) handleCheckInReinstate(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID, entryID := r.PathValue("id"), r.PathValue("entry")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	version, _ := strconv.ParseInt(r.FormValue("version"), 10, 64)
	_ = s.results.ReinstateEntry(r.Context(), actor, meetID, entryID, version)
	http.Redirect(w, r, r.Header.Get("Referer"), http.StatusSeeOther)
}

func (s *Server) handleCheckInClose(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID, eventID := r.PathValue("id"), r.PathValue("event")
	_, _ = s.results.CloseCheckIn(r.Context(), actor, meetID, eventID)
	http.Redirect(w, r, "/meets/"+meetID+"/events/"+eventID+"/checkin", http.StatusSeeOther)
}
