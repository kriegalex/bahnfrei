// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"net/http"
	"sort"
	"strconv"

	"github.com/kriegalex/bahnfrei/internal/app"
)

// --- CSV entry import & eligibility exceptions (TASK-017, UC-004/UC-005,
// SYS-013/014/010): mapping-profile CSV import with a preview-before-commit
// step and per-row status, plus the office worklist for flagged entries and
// its override action. Builds on TASK-016's entries workspace
// (internal/web/entries.go) — same meet-scoped nav, same view-model
// discipline (templates see only strings/flags, never app/store types). ---

const maxImportUploadBytes = 5 << 20 // 5 MiB: generous for a CSV entry list, small enough to bound memory

type importProfileOption struct{ ID, Name string }

type importRowView struct {
	RowNumber          int
	Status             string
	Reason             string
	Who                string
	EventCode          string
	CategoryCode       string
	EligibilityOutcome string
	EligibilityFlags   []string
}

type importReportView struct {
	ProfileID string
	Accepted  int
	Updated   int
	Rejected  int
	Committed bool
	Rows      []importRowView
}

type entriesImportView struct {
	MeetID   string
	MeetName string
	Profiles []importProfileOption
	Report   *importReportView
}

// importProfileOptions lists the built-in mapping profiles offered on the
// import form (system-native first, Alabus second — matches SYS-013(a)/(b)
// ordering).
func importProfileOptions() []importProfileOption {
	return []importProfileOption{
		{ID: "system-native", Name: "System-native CSV"},
		{ID: "alabus", Name: "Swiss Athletics / Alabus (assumption, OQ-030)"},
	}
}

func importReportViewFrom(p PageData, report app.ImportReport) importReportView {
	v := importReportView{
		ProfileID: report.ProfileID, Accepted: report.Accepted, Updated: report.Updated,
		Rejected: report.Rejected, Committed: report.Committed,
	}
	for _, r := range report.Rows {
		row := importRowView{
			RowNumber: r.RowNumber, Status: p.T("import.status." + string(r.Status)), Reason: r.Reason,
			Who: r.FirstName + " " + r.LastName, EventCode: r.EventCode, CategoryCode: r.CategoryCode,
		}
		if r.Eligibility.Outcome != "" {
			row.EligibilityOutcome = p.T("eligibility.outcome." + string(r.Eligibility.Outcome))
			for _, fl := range r.Eligibility.Flags {
				row.EligibilityFlags = append(row.EligibilityFlags, p.T("eligibility.flag."+fl.Code))
			}
		}
		v.Rows = append(v.Rows, row)
	}
	return v
}

func (s *Server) handleEntriesImportForm(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	detail, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = detail.Name + " — " + p.T("import.title")
	v := entriesImportView{MeetID: detail.ID, MeetName: detail.Name, Profiles: importProfileOptions()}
	_ = entriesImportPage(p, v).Render(r.Context(), w)
}

func (s *Server) handleEntriesImportSubmit(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	detail, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = detail.Name + " — " + p.T("import.title")
	v := entriesImportView{MeetID: detail.ID, MeetName: detail.Name, Profiles: importProfileOptions()}

	// The global request-body cap (limitRequestBody, middleware.go) already
	// bounds the upload before the CSRF middleware parses it; this argument is
	// only the in-memory-vs-tempfile threshold.
	if err := r.ParseMultipartForm(maxImportUploadBytes); err != nil {
		p.FlashError = p.T("import.error.upload")
		_ = entriesImportPage(p, v).Render(r.Context(), w)
		return
	}
	profileID := r.FormValue("profile")
	file, _, err := r.FormFile("file")
	if err != nil {
		p.FlashError = p.T("import.error.upload")
		_ = entriesImportPage(p, v).Render(r.Context(), w)
		return
	}
	defer func() { _ = file.Close() }()

	var report app.ImportReport
	if r.FormValue("action") == "commit" {
		report, err = s.results.CommitCSVImport(r.Context(), actor, meetID, profileID, file)
	} else {
		report, err = s.results.PreviewCSVImport(r.Context(), actor, meetID, profileID, file)
	}
	if err != nil {
		p.FlashError = p.T("import.error.invalid")
		_ = entriesImportPage(p, v).Render(r.Context(), w)
		return
	}
	rv := importReportViewFrom(p, report)
	v.Report = &rv
	_ = entriesImportPage(p, v).Render(r.Context(), w)
}

// --- eligibility exceptions (office worklist, UC-005) ---

type eligibilityRowView struct {
	EntryID        string
	Version        string
	EventLabel     string
	Who            string
	Outcome        string
	Flags          []string
	Overridden     bool
	OverriddenBy   string
	OverrideReason string
}

type eligibilityView struct {
	MeetID   string
	MeetName string
	Rows     []eligibilityRowView
}

func (s *Server) handleEligibilityList(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	detail, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	flagged, err := s.results.EligibilityExceptions(r.Context(), actor, meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = detail.Name + " — " + p.T("eligibility.title")
	if msg := r.URL.Query().Get("err"); msg != "" {
		p.FlashError = p.T("eligibility.error." + msg)
	}
	v := eligibilityView{MeetID: detail.ID, MeetName: detail.Name}
	for _, d := range flagged {
		row := eligibilityRowView{
			EntryID: d.ID, Version: intToStr(d.Eligibility.Version),
			EventLabel: s.entryEventLabel(p, d.Event), Who: d.AthleteName,
			Outcome:    p.T("eligibility.outcome." + string(d.Eligibility.Outcome)),
			Overridden: d.Eligibility.Overridden, OverriddenBy: d.Eligibility.OverriddenBy,
			OverrideReason: d.Eligibility.OverrideReason,
		}
		for _, fl := range d.Eligibility.Flags {
			row.Flags = append(row.Flags, p.T("eligibility.flag."+fl.Code))
		}
		v.Rows = append(v.Rows, row)
	}
	sort.Slice(v.Rows, func(i, j int) bool { return v.Rows[i].EventLabel < v.Rows[j].EventLabel })
	_ = eligibilityExceptionsPage(p, v).Render(r.Context(), w)
}

func (s *Server) handleEligibilityOverride(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	entryID := r.PathValue("entry")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	version, _ := strconv.ParseInt(r.FormValue("version"), 10, 64)
	reason := r.FormValue("reason")
	if _, err := s.results.OverrideEligibility(r.Context(), actor, meetID, entryID, version, reason); err != nil {
		http.Redirect(w, r, "/meets/"+meetID+"/entries/eligibility?err=invalid", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/entries/eligibility", http.StatusSeeOther)
}
