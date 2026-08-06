// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"context"

	"github.com/kriegalex/bahnfrei/internal/app"
)

// assignmentsDashboardView is the "my assignments" landing content for a
// logged-in session below the meet-organizer floor (TASK-042, DEC-025 —
// closes OQ-089). Exactly one of the three slices is populated, matching
// the session's exact role; Panel names which one so the template can pick
// the right heading/empty-state copy even when that slice is empty.
type assignmentsDashboardView struct {
	Panel string // "office" | "field" | "submitter"

	// OfficeMeets is every meet a competition-office session may operate
	// on (SYS-090 gives that role no per-meet scoping — see OQ-110).
	OfficeMeets []meetRowView

	// FieldMeets is the meets/units a field-official session is scoped to
	// (TASK-013 per-event assignment).
	FieldMeets []fieldAssignmentMeetView

	// SubmitterMeets is the meets an entry-submitter session has entries
	// at, each with that session's own entries there.
	SubmitterMeets []submitterMeetView
}

type fieldAssignmentMeetView struct {
	MeetID, MeetName string
	Units            []captureUnitView
}

type submitterMeetView struct {
	MeetID, MeetName string
	Entries          []entryRowView
}

// buildAssignmentsDashboard builds the dashboard content for actor's exact
// role (TASK-042, DEC-025). Each branch calls into an app-layer method that
// re-authorizes and scopes strictly to actor's own account — the same
// defense-in-depth every other handler in this package relies on, so a
// field official can never see a meet/unit it was not assigned (SYS-090)
// even if this switch were ever mis-wired.
func (s *Server) buildAssignmentsDashboard(ctx context.Context, p PageData, actor app.Session) (assignmentsDashboardView, error) {
	var v assignmentsDashboardView
	switch actor.Role {
	case app.RoleCompetitionOffice:
		v.Panel = "office"
		meets, err := s.meets.OfficeMeets(ctx, actor)
		if err != nil {
			return v, err
		}
		for _, m := range meets {
			v.OfficeMeets = append(v.OfficeMeets, meetRowView{
				ID:     m.ID,
				Name:   m.Name,
				Venue:  m.Venue,
				Dates:  formatDateRange(p, m.StartDate, m.EndDate),
				Tier:   string(m.Tier),
				Status: p.T("meet.status." + string(m.Status)),
			})
		}
	case app.RoleFieldOfficial:
		v.Panel = "field"
		assigned, err := s.results.AssignedMeets(ctx, actor)
		if err != nil {
			return v, err
		}
		for _, am := range assigned {
			fm := fieldAssignmentMeetView{MeetID: am.MeetID, MeetName: am.MeetName}
			for _, u := range am.Units {
				// Localized discipline name + schedule (SYS-151/UC-041 #2):
				// the localized-name path already used by standings
				// (localizedDisciplineName, SYS-111), reused here rather
				// than the catalog's English canonical name.
				cv := captureUnitView{
					UnitID:     u.UnitID,
					Discipline: s.localizedDisciplineName(p, u.DisciplineCode),
					Family:     string(u.Family),
				}
				if u.ScheduledAt != nil {
					cv.Scheduled = true
					cv.When = p.FormatDateTime(*u.ScheduledAt)
					cv.Location = u.Location
				}
				fm.Units = append(fm.Units, cv)
			}
			v.FieldMeets = append(v.FieldMeets, fm)
		}
	case app.RoleEntrySubmitter:
		v.Panel = "submitter"
		subs, err := s.results.SubmitterMeets(ctx, actor)
		if err != nil {
			return v, err
		}
		for _, sm := range subs {
			v.SubmitterMeets = append(v.SubmitterMeets, submitterMeetView{
				MeetID:   sm.MeetID,
				MeetName: sm.MeetName,
				Entries:  s.entryRowsFrom(p, sm.Entries),
			})
		}
	}
	return v, nil
}
