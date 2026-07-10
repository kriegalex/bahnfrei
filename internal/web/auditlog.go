// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import "net/http"

// --- privileged-action audit surfacing (TASK-013, SYS-091/UC-022 #2): an
// office/organizer-visible view over the append-only audit log (SYS-046). ---

// auditLogWindow is how many recent privileged events the view renders
// (PoC scope; pagination is a later concern once meets accumulate more
// history than fits one screen usefully).
const auditLogWindow = 200

type auditRowView struct {
	When   string
	Who    string
	Action string
	Entity string
	Reason string
}

type auditLogView struct {
	Rows []auditRowView
}

func (s *Server) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	events, err := s.auth.PrivilegedAuditLog(r.Context(), actor, auditLogWindow)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	v := auditLogView{}
	for _, e := range events {
		v.Rows = append(v.Rows, auditRowView{
			When:   e.TS.UTC().Format("2006-01-02 15:04:05 MST"),
			Who:    e.ActorName,
			Action: e.Action,
			Entity: e.EntityType + ":" + e.EntityID,
			Reason: e.Reason,
		})
	}
	p := basePageData(r, s.cats)
	p.Title = p.T("audit.title")
	_ = auditLogPage(p, v).Render(r.Context(), w)
}
