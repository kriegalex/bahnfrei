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
	// CloseCount is the number of rows still "entered" (not yet confirmed,
	// not already DNS) — exactly what CloseCheckIn would set to DNS
	// (TASK-047/SYS-152/UC-041 #4-5): 0 gates the "Check-in schliessen"
	// action off (hidden, with a reason, rather than a live action with
	// nothing to do) and is the scope count the close-confirm sub-page
	// states before its confirm button.
	CloseCount int
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
		isEntered := row.Status == domain.EntryEntered
		v.Rows = append(v.Rows, checkInRowView{
			EntryID: row.ID, Version: strconv.FormatInt(row.Version, 10),
			Who: who, ClubName: row.ClubName, Status: p.T("entry.status." + string(row.Status)),
			IsEntered: isEntered, IsConfirmed: row.Status == domain.EntryConfirmed,
			IsDNS: row.Status == domain.EntryDNS,
		})
		if isEntered {
			v.CloseCount++
		}
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
		http.Redirect(w, r, sameOriginRedirectTarget(r.Header.Get("Referer"), r.Host), http.StatusSeeOther) // #nosec G710 -- sameOriginRedirectTarget (routes.go) rejects any non-root-relative/off-host value and falls back to "/"
		return
	}
	http.Redirect(w, r, sameOriginRedirectTarget(r.Header.Get("Referer"), r.Host), http.StatusSeeOther) // #nosec G710 -- sameOriginRedirectTarget (routes.go) rejects any non-root-relative/off-host value and falls back to "/"
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
	http.Redirect(w, r, sameOriginRedirectTarget(r.Header.Get("Referer"), r.Host), http.StatusSeeOther) // #nosec G710 -- sameOriginRedirectTarget (routes.go) rejects any non-root-relative/off-host value and falls back to "/"
}

// handleCheckInCloseConfirm serves the TASK-034-style GET confirm sub-page
// for closing check-in (TASK-047/SYS-152/UC-041 #4-5): states how many
// still-"entered" entries will be set to DNS, and — when there are none
// (zero entries, or every entry already confirmed/DNS/scratched) — renders
// an explanatory inapplicable state instead of a live confirm button, the
// same guard the checkInPage list view itself applies to the action link
// (CloseCount == 0 hides it there too).
func (s *Server) handleCheckInCloseConfirm(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID, eventID := r.PathValue("id"), r.PathValue("event")
	p := basePageData(r, s.cats)
	v, err := s.checkInView(r, p, actor, meetID, eventID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	cv := confirmView{
		Title:      v.EventLabel + " — " + p.T("checkin.close.confirm.title"),
		FormAction: "/meets/" + meetID + "/events/" + eventID + "/checkin/close",
		CancelHref: "/meets/" + meetID + "/events/" + eventID + "/checkin",
	}
	if v.CloseCount > 0 {
		cv.Description = p.T("checkin.close.confirm.description", "n", intToStr(int64(v.CloseCount)))
		cv.ConfirmLabel = p.T("checkin.close")
	} else {
		cv.Inapplicable = true
		cv.Description = p.T("checkin.close.confirm.inapplicable")
	}
	s.renderConfirm(w, r, p, cv, http.StatusOK)
}

func (s *Server) handleCheckInClose(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID, eventID := r.PathValue("id"), r.PathValue("event")
	_, _ = s.results.CloseCheckIn(r.Context(), actor, meetID, eventID)
	http.Redirect(w, r, "/meets/"+meetID+"/events/"+eventID+"/checkin", http.StatusSeeOther)
}
