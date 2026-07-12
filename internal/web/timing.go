// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/domain"
)

// --- Timing exchange office UI (TASK-020, UC-014, SYS-060-062): trigger/
// download FinishLynx and generic-CSV exports, upload result files, see
// import conflicts and resolve them one at a time, review ingest history,
// and manage the watched-folder timing agent's bearer credentials
// (ADR-006). ---

// maxTimingUploadBytes bounds a .lif/CSV upload — generous for even a
// large multi-heat meet's single result file, small enough to bound memory
// (mirrors maxImportUploadBytes's precedent, internal/web/import.go).
const maxTimingUploadBytes = 5 << 20

type timingConflictRowView struct {
	ID     string
	Reason string
	Bib    string
	Lane   int
}

type timingBatchRowView struct {
	Filename   string
	CreatedAt  string
	Applied    int
	Conflicted int
	ImportedBy string
}

type timingAgentTokenRowView struct {
	ID        string
	Label     string
	CreatedAt string
	Revoked   bool
}

type timingParticipantOption struct {
	AthleteID string
	Bib       string
	Name      string
}

type timingView struct {
	MeetID       string
	MeetName     string
	Conflicts    []timingConflictRowView
	Batches      []timingBatchRowView
	Tokens       []timingAgentTokenRowView
	Participants []timingParticipantOption
	ImportError  string
	ImportResult *app.TimingImportSummary
	// NewTokenPlaintext is shown exactly once, immediately after
	// CreateTimingAgentToken — never reloaded from storage (only the hash
	// is persisted).
	NewTokenPlaintext string
}

func (s *Server) timingViewFor(r *http.Request, meetID string) (timingView, error) {
	actor, _ := sessionFromContext(r.Context())
	detail, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		return timingView{}, err
	}
	v := timingView{MeetID: detail.ID, MeetName: detail.Name}

	conflicts, err := s.results.ListTimingImportConflicts(r.Context(), actor, meetID)
	if err != nil {
		return timingView{}, err
	}
	for _, c := range conflicts {
		if c.Status != "pending" {
			continue
		}
		v.Conflicts = append(v.Conflicts, timingConflictRowView{ID: c.ID, Reason: c.Reason, Bib: c.Bib, Lane: c.Lane})
	}

	batches, err := s.results.ListTimingImportBatches(r.Context(), actor, meetID)
	if err != nil {
		return timingView{}, err
	}
	for _, b := range batches {
		v.Batches = append(v.Batches, timingBatchRowView{
			Filename: b.Filename, CreatedAt: b.CreatedAt.Format("2006-01-02 15:04"),
			Applied: b.Applied, Conflicted: b.Conflicted, ImportedBy: b.ImportedBy,
		})
	}

	tokens, err := s.results.ListTimingAgentTokens(r.Context(), actor, meetID)
	if err != nil {
		return timingView{}, err
	}
	for _, t := range tokens {
		v.Tokens = append(v.Tokens, timingAgentTokenRowView{
			ID: t.ID, Label: t.Label, CreatedAt: t.CreatedAt.Format("2006-01-02 15:04"), Revoked: t.RevokedAt != nil,
		})
	}

	participants, err := s.results.Participants(r.Context(), meetID)
	if err != nil {
		return timingView{}, err
	}
	for _, p := range participants {
		v.Participants = append(v.Participants, timingParticipantOption{
			AthleteID: p.AthleteID, Bib: p.Bib, Name: p.Athlete.FirstName + " " + p.Athlete.LastName,
		})
	}
	return v, nil
}

func (s *Server) handleTimingIndex(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	v, err := s.timingViewFor(r, meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = v.MeetName + " — " + p.T("timing.title")
	_ = timingPage(p, v).Render(r.Context(), w)
}

// writeDownload sends data as a downloadable file (mirrors writePDF's
// convention, internal/web/printables.go, generalized past PDF).
func writeDownload(w http.ResponseWriter, data []byte, filename, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleTimingExportPPL(w http.ResponseWriter, r *http.Request) {
	s.exportTimingFile(w, r, "ppl")
}
func (s *Server) handleTimingExportSCH(w http.ResponseWriter, r *http.Request) {
	s.exportTimingFile(w, r, "sch")
}
func (s *Server) handleTimingExportEVT(w http.ResponseWriter, r *http.Request) {
	s.exportTimingFile(w, r, "evt")
}

func (s *Server) exportTimingFile(w http.ResponseWriter, r *http.Request, kind string) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	ppl, sch, evt, err := s.results.ExportTimingFiles(r.Context(), actor, meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	switch kind {
	case "ppl":
		writeDownload(w, ppl, "lynx.ppl", "text/plain; charset=utf-8")
	case "sch":
		writeDownload(w, sch, "lynx.sch", "text/plain; charset=utf-8")
	case "evt":
		writeDownload(w, evt, "lynx.evt", "text/plain; charset=utf-8")
	}
}

func (s *Server) handleTimingExportCSV(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	data, err := s.results.ExportGenericCSV(r.Context(), actor, meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	writeDownload(w, data, "timing-exchange.csv", "text/csv; charset=utf-8")
}

func (s *Server) handleTimingImportSubmit(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	v, verr := s.timingViewFor(r, meetID)
	if verr != nil {
		s.renderMeetError(w, r, verr)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = v.MeetName + " — " + p.T("timing.title")

	if err := r.ParseMultipartForm(maxTimingUploadBytes); err != nil {
		v.ImportError = p.T("timing.import.error.upload")
		_ = timingPage(p, v).Render(r.Context(), w)
		return
	}
	format := r.FormValue("format")
	file, header, err := r.FormFile("file")
	if err != nil {
		v.ImportError = p.T("timing.import.error.upload")
		_ = timingPage(p, v).Render(r.Context(), w)
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(file)
	if err != nil {
		v.ImportError = p.T("timing.import.error.upload")
		_ = timingPage(p, v).Render(r.Context(), w)
		return
	}

	summary, err := s.results.ImportTimingFile(r.Context(), actor, meetID, header.Filename, format, data)
	if err != nil {
		v.ImportError = p.T("timing.import.error.invalid")
		_ = timingPage(p, v).Render(r.Context(), w)
		return
	}
	v.ImportResult = &summary
	// Reload the conflict/batch lists so the freshly queued rows show up.
	if reloaded, err := s.timingViewFor(r, meetID); err == nil {
		reloaded.ImportResult = &summary
		v = reloaded
	}
	_ = timingPage(p, v).Render(r.Context(), w)
}

func (s *Server) handleTimingConflictResolve(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	conflictID := r.PathValue("conflict")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	in := app.ResolveTimingConflictInput{
		Action:               r.FormValue("action"),
		ReassignAthleteID:    r.FormValue("reassign_athlete_id"),
		OverrideStatus:       parseQualificationStatus(r.FormValue("override_status")),
		OverrideStatusDetail: r.FormValue("override_status_detail"),
		Reason:               r.FormValue("reason"),
		Escalation:           r.FormValue("escalation"),
	}
	err := s.results.ResolveTimingConflict(r.Context(), actor, meetID, conflictID, in)
	if err != nil {
		http.Redirect(w, r, "/meets/"+meetID+"/timing?err=resolve", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/timing", http.StatusSeeOther)
}

func (s *Server) handleTimingAgentTokenCreate(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	label := r.FormValue("label")
	plaintext, _, err := s.results.CreateTimingAgentToken(r.Context(), actor, meetID, label)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	v, err := s.timingViewFor(r, meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	v.NewTokenPlaintext = plaintext
	p := basePageData(r, s.cats)
	p.Title = v.MeetName + " — " + p.T("timing.title")
	_ = timingPage(p, v).Render(r.Context(), w)
}

func (s *Server) handleTimingAgentTokenRevoke(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	meetID := r.PathValue("id")
	tokenID := r.PathValue("token")
	if err := s.results.RevokeTimingAgentToken(r.Context(), actor, meetID, tokenID); err != nil {
		http.Redirect(w, r, "/meets/"+meetID+"/timing?err=revoke", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/meets/"+meetID+"/timing", http.StatusSeeOther)
}

// parseQualificationStatus is a tiny local helper: the resolve form's
// override_status select only ever offers this system's CR 25
// operator-settable vocabulary, so an empty/unrecognized submission just
// yields StatusNone (ResolveTimingConflict rejects that with
// ErrTimingConflictStatusRequired where a status is actually required).
func parseQualificationStatus(v string) domain.QualificationStatus {
	switch v {
	case "DNS", "DNF", "DQ", "NM":
		return domain.QualificationStatus(v)
	default:
		return domain.StatusNone
	}
}

// --- Timing agent HTTP API (ADR-006's hub-first amendment): a meet-scoped
// Bearer token, never the browser session cookie — see the CSRF exemption
// in middleware.go and cmd/bahnfrei/timingagent.go for the client side. ---

// maxAgentUploadBytes bounds an agent-uploaded .lif/CSV file — the same
// budget the office upload form uses.
const maxAgentUploadBytes = maxTimingUploadBytes

// authenticateAgent resolves the request's Authorization: Bearer header to
// an app.Session scoped to meetID, or writes a 401 and returns ok=false.
// The path's {id} and the token's own meet scope must match — a token
// issued for one meet never authenticates another (defense in depth on
// top of ADR-006's "meet-scoped" design).
func (s *Server) authenticateAgent(w http.ResponseWriter, r *http.Request, meetID string) (app.Session, bool) {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		w.WriteHeader(http.StatusUnauthorized)
		return app.Session{}, false
	}
	token := strings.TrimSpace(strings.TrimPrefix(auth, prefix))
	session, tokenMeetID, err := s.results.AuthenticateTimingAgent(r.Context(), token)
	if err != nil || tokenMeetID != meetID {
		w.WriteHeader(http.StatusUnauthorized)
		return app.Session{}, false
	}
	return session, true
}

func (s *Server) handleAgentImportLIF(w http.ResponseWriter, r *http.Request) {
	s.handleAgentImport(w, r, "lif")
}
func (s *Server) handleAgentImportCSV(w http.ResponseWriter, r *http.Request) {
	s.handleAgentImport(w, r, "csv")
}

func (s *Server) handleAgentImport(w http.ResponseWriter, r *http.Request, format string) {
	meetID := r.PathValue("id")
	session, ok := s.authenticateAgent(w, r, meetID)
	if !ok {
		return
	}
	filename := r.Header.Get("X-Filename")
	if filename == "" {
		filename = "upload." + format
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, maxAgentUploadBytes+1))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if len(data) > maxAgentUploadBytes {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}
	summary, err := s.results.ImportTimingFile(r.Context(), session, meetID, filename, format, data)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(summary)
}

// agentExportManifest is what the agent polls to detect a start-list
// change (ADR-006: "regenerate .ppl/.sch/.evt on start-list changes") —
// SHA-256 of each file's current content, so the agent only rewrites the
// watched-folder copy that actually changed.
type agentExportManifest struct {
	PPLSHA256 string `json:"pplSha256"`
	SCHSHA256 string `json:"schSha256"`
	EVTSHA256 string `json:"evtSha256"`
}

func (s *Server) handleAgentExportManifest(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	session, ok := s.authenticateAgent(w, r, meetID)
	if !ok {
		return
	}
	ppl, sch, evt, err := s.results.ExportTimingFiles(r.Context(), session, meetID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(agentExportManifest{
		PPLSHA256: sha256Hex(ppl), SCHSHA256: sha256Hex(sch), EVTSHA256: sha256Hex(evt),
	})
}

func (s *Server) handleAgentExportFile(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	kind := r.PathValue("kind")
	session, ok := s.authenticateAgent(w, r, meetID)
	if !ok {
		return
	}
	ppl, sch, evt, err := s.results.ExportTimingFiles(r.Context(), session, meetID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var data []byte
	switch kind {
	case "ppl":
		data = ppl
	case "sch":
		data = sch
	case "evt":
		data = evt
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
