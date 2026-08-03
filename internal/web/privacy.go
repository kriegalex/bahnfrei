// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/kriegalex/bahnfrei/internal/app"
)

// --- privacy: SYS-101 data-subject rights and SYS-102 retention purge
// surfaces (TASK-023, UC-023/UC-024). handlePrivacyList/ConsentToggle live
// alongside the roster/standings surfaces they operate on (office-gated,
// per-meet entry point); handleRetentionPurge lives on the instance-admin
// surface alongside backup, matching CapManageRetention's tier. ---

type privacyRowView struct {
	AthleteID  string
	Bib        string
	Name       string
	Minor      bool
	Withdrawn  bool
	Anonymized bool
}

type privacyView struct {
	MeetID   string
	MeetName string
	Rows     []privacyRowView
}

// handlePrivacyList serves one meet's data-subject-rights worklist: every
// participant with their SYS-103 consent status and the export/consent-
// toggle/erase actions (UC-023 #2/#3, UC-024 #1/#2).
func (s *Server) handlePrivacyList(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	detail, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	participants, err := s.results.Participants(r.Context(), meetID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	now := time.Now()
	v := privacyView{MeetID: detail.ID, MeetName: detail.Name}
	for _, p := range participants {
		v.Rows = append(v.Rows, privacyRowView{
			AthleteID:  p.AthleteID,
			Bib:        p.Bib,
			Name:       p.Athlete.FirstName + " " + p.Athlete.LastName,
			Minor:      p.Athlete.IsMinor(now),
			Withdrawn:  p.Athlete.Consent.ResultsPublicationWithdrawn,
			Anonymized: p.Athlete.Anonymized,
		})
	}
	page := basePageData(r, s.cats)
	page.Title = v.MeetName + " — " + page.T("privacy.title")
	if msg := r.URL.Query().Get("err"); msg != "" {
		page.FlashError = page.T("privacy.error." + msg)
	}
	_ = privacyPage(page, v).Render(r.Context(), w)
}

// handlePrivacyConsentToggle updates one athlete's SYS-103
// results-publication-withdrawn flag (UC-023 #3: mid-meet consent
// changes reach public surfaces within one publication cycle — every
// public read recomputes standings live, so this write's very next public
// request already reflects it).
func (s *Server) handlePrivacyConsentToggle(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	athleteID := r.PathValue("athlete")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	withdrawn := r.FormValue("withdrawn") == "true"
	if err := s.results.SetConsent(r.Context(), actor, meetID, athleteID, withdrawn); err != nil {
		http.Redirect(w, r, "/meets/"+meetID+"/privacy?err=invalid", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/privacy", http.StatusSeeOther)
}

// handlePrivacyExport serves the SYS-101 subject-access export (UC-024
// #1) as a JSON download.
func (s *Server) handlePrivacyExport(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	athleteID := r.PathValue("athlete")
	if s.privacy == nil {
		s.handleNotFound(w, r)
		return
	}
	export, err := s.privacy.ExportAthleteData(r.Context(), actor, athleteID)
	if err != nil {
		s.renderPrivacyError(w, err)
		return
	}
	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="athlete-`+athleteID+`-data-export.json"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

// handlePrivacyErase performs the SYS-101 erasure/pseudonymization
// request (UC-024 #2). OQ-074 (TASK-034): reached only via the
// GET .../erase/confirm sub-page's form, and gated again here on the
// server side — the typed-confirmation token (the athlete's bib) must
// match before the irreversible erasure runs; a mismatch re-renders the
// confirm page with an inline field error rather than silently failing or
// (worse) proceeding.
func (s *Server) handlePrivacyErase(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	athleteID := r.PathValue("athlete")
	if s.privacy == nil {
		s.handleNotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	p := basePageData(r, s.cats)
	name, bib, ok, err := s.participantDisplay(r, meetID, athleteID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		s.handleNotFound(w, r)
		return
	}
	if got, want := r.FormValue("confirm_text"), eraseConfirmToken(bib); got != want {
		v := s.eraseConfirmView(p, meetID, athleteID, name, bib, p.T("privacy.erase.confirm.mismatch"))
		s.renderConfirm(w, r, p, v, http.StatusUnprocessableEntity)
		return
	}
	reason := r.FormValue("reason")
	if err := s.privacy.EraseAthlete(r.Context(), actor, athleteID, reason); err != nil {
		http.Redirect(w, r, "/meets/"+meetID+"/privacy?err=invalid", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/privacy", http.StatusSeeOther)
}

// retentionPurgeView shows the instance-admin retention-purge trigger and
// (once run) the last run's report.
type retentionPurgeView struct {
	RetentionDays    int
	LastRunAthletes  int
	LastRunAuditRows int
	HasRun           bool
}

func (s *Server) handleRetentionPurgeForm(w http.ResponseWriter, r *http.Request) {
	page := basePageData(r, s.cats)
	page.Title = page.T("privacy.retention.title")
	_ = retentionPurgePage(page, retentionPurgeView{RetentionDays: app.DefaultRetentionDays}).Render(r.Context(), w)
}

// handleRetentionPurge runs the SYS-102 retention purge on manual admin
// trigger (UC-024 #3). OQ-074 (TASK-034): reached only via the
// GET /admin/privacy/purge/confirm sub-page's form, and gated again here —
// this is an instance-wide, unrecoverable action, so it requires the fixed
// typed-confirmation token (retentionPurgeConfirmToken) in addition to the
// confirm step every destructive action gets.
func (s *Server) handleRetentionPurge(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	if s.privacy == nil {
		s.handleNotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	p := basePageData(r, s.cats)
	if r.FormValue("confirm_text") != retentionPurgeConfirmToken {
		v := s.retentionPurgeConfirmView(p, p.T("privacy.retention.confirm.mismatch"))
		s.renderConfirm(w, r, p, v, http.StatusUnprocessableEntity)
		return
	}
	report, err := s.privacy.PurgeExpired(r.Context(), actor, app.DefaultRetentionDays)
	page := basePageData(r, s.cats)
	page.Title = page.T("privacy.retention.title")
	v := retentionPurgeView{RetentionDays: app.DefaultRetentionDays}
	if err != nil {
		page.FlashError = page.T("privacy.retention.error")
		_ = retentionPurgePage(page, v).Render(r.Context(), w)
		return
	}
	v.HasRun = true
	v.LastRunAthletes = report.AthletesPurged
	v.LastRunAuditRows = report.AuditRowsRedacted
	_ = retentionPurgePage(page, v).Render(r.Context(), w)
}

// renderPrivacyError maps PrivacyService errors onto HTTP responses,
// mirroring handleBackupDownload's app.ErrForbidden type-assertion pattern.
func (s *Server) renderPrivacyError(w http.ResponseWriter, err error) {
	if _, forbidden := err.(app.ErrForbidden); forbidden {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if errors.Is(err, app.ErrAthleteNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	http.Error(w, "internal error", http.StatusInternalServerError)
}
