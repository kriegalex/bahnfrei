// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import "net/http"

// --- field-official per-event scoping (TASK-013, SYS-090 "assignable per
// meet"; UC-022 #1): the office's assignment matrix for one meet. ---

type officialUnitColumn struct {
	UnitID string
	Label  string
}

type officialRowView struct {
	AccountID string
	Label     string // display name (username)
	Assigned  []bool // aligned with officialsView.Columns
}

type officialsView struct {
	MeetID   string
	MeetName string
	Columns  []officialUnitColumn
	Rows     []officialRowView
}

func (s *Server) handleFieldOfficials(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	detail, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	units, err := s.results.CaptureUnits(r.Context(), actor, meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	rows, err := s.results.FieldOfficialAssignments(r.Context(), actor, meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}

	v := officialsView{MeetID: detail.ID, MeetName: detail.Name}
	for _, u := range units {
		v.Columns = append(v.Columns, officialUnitColumn{UnitID: u.UnitID, Label: u.DisciplineName})
	}
	for _, row := range rows {
		rv := officialRowView{AccountID: row.AccountID, Label: row.DisplayName + " (" + row.Username + ")"}
		for _, u := range units {
			rv.Assigned = append(rv.Assigned, row.AssignedUnitIDs[u.UnitID])
		}
		v.Rows = append(v.Rows, rv)
	}

	p := basePageData(r, s.cats)
	p.Title = v.MeetName + " — " + p.T("officials.title")
	if msg := r.URL.Query().Get("err"); msg != "" {
		p.FlashError = p.T("officials.error." + msg)
	}
	_ = officialsPage(p, v).Render(r.Context(), w)
}

func (s *Server) handleFieldOfficialAssign(w http.ResponseWriter, r *http.Request) {
	s.setFieldOfficialUnit(w, r, true)
}

func (s *Server) handleFieldOfficialUnassign(w http.ResponseWriter, r *http.Request) {
	s.setFieldOfficialUnit(w, r, false)
}

func (s *Server) setFieldOfficialUnit(w http.ResponseWriter, r *http.Request, assign bool) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	accountID := r.FormValue("account_id")
	unitID := r.FormValue("unit_id")

	var err error
	if assign {
		err = s.results.AssignFieldOfficialUnit(r.Context(), actor, meetID, unitID, accountID)
	} else {
		err = s.results.UnassignFieldOfficialUnit(r.Context(), actor, meetID, unitID, accountID)
	}
	if err != nil {
		http.Redirect(w, r, "/meets/"+meetID+"/officials?err=invalid", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/officials", http.StatusSeeOther)
}
