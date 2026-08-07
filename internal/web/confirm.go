// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"net/http"

	"github.com/kriegalex/bahnfrei/internal/app"
)

// --- OQ-074 destructive-action confirmation flow (usability-audit H3/F5,
// UC-038 #3). ---
//
// The shell's CSP is strict (`default-src 'self'`, no inline JS —
// securityHeaders in middleware.go), so the usual `onclick="return
// confirm(...)"` pattern cannot run. Every irreversible/destructive action
// instead gets a real GET confirm sub-page (GET-then-POST): a plain
// navigation, not a JS dialog, so the back button, bookmarking and reload
// all behave the way an operator expects, and the page works with
// JavaScript disabled. The four actions flagged by the audit — athlete
// erasure, account disable, meet archive, retention purge — all use the
// same confirmView/confirmPage shape for consistency; each confirm page's
// own handler builds the entity-specific description text.
//
// Friction level is deliberately not uniform:
//   - Account disable and meet archive get a plain Confirm/Cancel step.
//     Disable is reversible (an "enable" action exists); archive is a
//     status change, not data destruction.
//   - Athlete erasure and retention purge — the two actions the ASVS L2
//     review (docs/delivery/reviews/asvs-l2-task-026.md, OQ-074) singled
//     out as irreversible *and* unrecoverable — additionally require a
//     typed-confirmation string (TypedConfirm below) before the POST is
//     honored: the operator must type the athlete's bib (or "ERASE" if the
//     participant has none yet) / the literal "PURGE". This is the
//     documented defense-in-depth choice: the review notes ASVS L3-style
//     re-authentication is not required for the L2 pass, and a typed
//     token achieves the same "prove you meant it" friction as password
//     re-entry without adding a second password prompt (which would need
//     new plumbing into the per-IP failed-attempt limiter, ratelimit.go,
//     for a control this task's scope does not otherwise touch) or new
//     phishing-shaped UX. A typed string is also the industry-precedent
//     pattern for irreversible actions (e.g. "type the repository name to
//     delete it").
//
// TASK-053 (DEC-030) adds a fifth flow on the same GET-confirm-sub-page
// shape: account password reset. It is reversible (the operator logs back
// in with the temporary password and completes the forced change-password
// step) so it takes the plain Confirm/Cancel friction level like disable —
// but needs the admin to type the new temporary password inline, so
// confirmView grows an optional PasswordField rather than repurposing
// TypedConfirm (which means "type this exact value to prove intent", not
// "enter arbitrary new data").

// confirmField is one hidden <input> carried through a confirm sub-page's
// POST form (e.g. an optimistic-concurrency version, a default reason).
type confirmField struct {
	Name, Value string
}

// confirmView is the shared destructive-action confirmation page (see the
// package doc above). TypedConfirm == "" means the plain Confirm/Cancel
// step with no typed-token friction.
type confirmView struct {
	Title             string
	Description       string
	FormAction        string
	CancelHref        string
	ConfirmLabel      string
	Hidden            []confirmField
	TypedConfirm      string // expected exact value; "" disables the typed-confirmation step
	TypedConfirmField string // form field name carrying the typed value
	TypedConfirmLabel string // localized "type X to confirm" label
	// FieldErr is set when a POST's typed confirmation did not match, so
	// this same page re-renders with an inline error at that one field
	// (OQ-075's convention) instead of silently failing.
	FieldErr string
	// Inapplicable means the action this confirm page fronts has nothing
	// to affect right now (TASK-047/SYS-152/UC-041 #4): a link that was
	// live when the referring page rendered can go stale by the time the
	// operator lands here (someone else finished the last capture, closed
	// check-in, etc.). Description then carries the explanation instead of
	// a scope count, and confirmPage renders it with no form/confirm
	// button — a real state, not a live action with nothing to confirm.
	Inapplicable bool
	// PasswordField, when set, renders a type="password" input on the
	// confirm page (TASK-053's account password reset): the admin types
	// the account's new temporary password inline, on the same
	// GET-confirm-sub-page navigation every other account mutation on this
	// surface already uses, rather than a further page. PasswordFieldErr
	// mirrors FieldErr's re-render-with-inline-error shape for this field.
	PasswordField      string
	PasswordFieldLabel string
	PasswordFieldErr   string
}

func (s *Server) renderConfirm(w http.ResponseWriter, r *http.Request, p PageData, v confirmView, status int) {
	p.Title = v.Title
	w.WriteHeader(status)
	_ = confirmPage(p, v).Render(r.Context(), w)
}

// --- meet archive ---

func (s *Server) handleMeetArchiveConfirm(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	d, err := s.meets.Meet(r.Context(), meetID)
	if err != nil {
		s.renderMeetError(w, r, err)
		return
	}
	p := basePageData(r, s.cats)
	v := confirmView{
		Title:        p.T("meet.archive.confirm.title"),
		Description:  p.T("meet.archive.confirm.description", "name", d.Name),
		FormAction:   "/meets/" + meetID + "/archive",
		CancelHref:   "/meets/" + meetID,
		ConfirmLabel: p.T("meet.archive"),
		Hidden:       []confirmField{{"version", intToStr(d.Version)}},
	}
	s.renderConfirm(w, r, p, v, http.StatusOK)
}

// --- account disable ---

// accountUsername looks up one account's username among the accounts the
// actor is authorized to list — kept a thin lookup rather than a new
// AuthService method since account-count is PoC-scale (UC-022) and the
// confirm page only needs the display string, never the full record (the
// view-boundary rule in view.go: templates never see store types).
func (s *Server) accountUsername(r *http.Request, actor app.Session, accountID string) (username string, ok bool, err error) {
	accounts, err := s.auth.ListAccounts(r.Context(), actor)
	if err != nil {
		return "", false, err
	}
	for _, a := range accounts {
		if a.ID == accountID {
			return a.Username, true, nil
		}
	}
	return "", false, nil
}

func (s *Server) handleAccountDisableConfirm(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	accountID := r.PathValue("id")
	username, ok, err := s.accountUsername(r, actor, accountID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		s.handleNotFound(w, r)
		return
	}
	p := basePageData(r, s.cats)
	v := confirmView{
		Title:        p.T("accounts.disable.confirm.title"),
		Description:  p.T("accounts.disable.confirm.description", "username", username),
		FormAction:   "/admin/accounts/" + accountID + "/disable",
		CancelHref:   "/admin",
		ConfirmLabel: p.T("accounts.action.disable"),
	}
	s.renderConfirm(w, r, p, v, http.StatusOK)
}

// --- account password reset (TASK-053, DEC-030, SYS-090/091): the
// meet-morning-lockout fix, no email infrastructure. ---

// accountResetConfirmView builds the reset confirm page. fieldErr is set
// only when re-rendering after the POST rejected a too-short password.
func (s *Server) accountResetConfirmView(p PageData, accountID, username, fieldErr string) confirmView {
	return confirmView{
		Title:              p.T("accounts.reset.confirm.title"),
		Description:        p.T("accounts.reset.confirm.description", "username", username),
		FormAction:         "/admin/accounts/" + accountID + "/reset",
		CancelHref:         "/admin",
		ConfirmLabel:       p.T("accounts.action.reset"),
		PasswordField:      "new_password",
		PasswordFieldLabel: p.T("accounts.reset.confirm.password_label"),
		PasswordFieldErr:   fieldErr,
	}
}

func (s *Server) handleAccountResetConfirm(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	accountID := r.PathValue("id")
	username, ok, err := s.accountUsername(r, actor, accountID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		s.handleNotFound(w, r)
		return
	}
	p := basePageData(r, s.cats)
	s.renderConfirm(w, r, p, s.accountResetConfirmView(p, accountID, username, ""), http.StatusOK)
}

// --- athlete erasure (SYS-101, UC-024 #2): typed-confirmation friction ---

// participantDisplay looks up one meet's participant by athlete id, for the
// confirm page's description and its typed-confirmation expected value.
func (s *Server) participantDisplay(r *http.Request, meetID, athleteID string) (name, bib string, ok bool, err error) {
	participants, err := s.results.Participants(r.Context(), meetID)
	if err != nil {
		return "", "", false, err
	}
	for _, p := range participants {
		if p.AthleteID == athleteID {
			return p.Athlete.FirstName + " " + p.Athlete.LastName, p.Bib, true, nil
		}
	}
	return "", "", false, nil
}

// eraseConfirmToken is the string an operator must type to confirm athlete
// erasure: the bib when one is assigned (short, already visible on every
// roster/privacy list — low recall effort, still a deliberate act), or the
// literal "ERASE" for the rare participant with no bib yet.
func eraseConfirmToken(bib string) string {
	if bib != "" {
		return bib
	}
	return "ERASE"
}

func (s *Server) handlePrivacyEraseConfirm(w http.ResponseWriter, r *http.Request) {
	meetID := r.PathValue("id")
	athleteID := r.PathValue("athlete")
	name, bib, ok, err := s.participantDisplay(r, meetID, athleteID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		s.handleNotFound(w, r)
		return
	}
	p := basePageData(r, s.cats)
	s.renderConfirm(w, r, p, s.eraseConfirmView(p, meetID, athleteID, name, bib, ""), http.StatusOK)
}

func (s *Server) eraseConfirmView(p PageData, meetID, athleteID, name, bib, fieldErr string) confirmView {
	token := eraseConfirmToken(bib)
	return confirmView{
		Title:             p.T("privacy.erase.confirm.title"),
		Description:       p.T("privacy.erase.confirm.description", "name", name),
		FormAction:        "/meets/" + meetID + "/privacy/" + athleteID + "/erase",
		CancelHref:        "/meets/" + meetID + "/privacy",
		ConfirmLabel:      p.T("privacy.action.erase"),
		Hidden:            []confirmField{{"reason", p.T("privacy.erase.default_reason")}},
		TypedConfirm:      token,
		TypedConfirmField: "confirm_text",
		TypedConfirmLabel: p.T("privacy.erase.confirm.typed_label", "token", token),
		FieldErr:          fieldErr,
	}
}

// --- retention purge (SYS-102, UC-024 #3): typed-confirmation friction ---

// retentionPurgeConfirmToken is the fixed literal an operator must type to
// confirm a retention purge. Deliberately not localized (an ASCII token
// typed verbatim, matching the "type DELETE"/"type the repo name" pattern
// other systems use for irreversible actions) — there is no single natural
// per-action identifier the way an athlete's bib is for erasure, since the
// purge is instance-wide, not scoped to one entity.
const retentionPurgeConfirmToken = "PURGE"

func (s *Server) handleRetentionPurgeConfirm(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	p := basePageData(r, s.cats)
	s.renderConfirm(w, r, p, s.retentionPurgeConfirmView(r, actor, p, ""), http.StatusOK)
}

// retentionPurgeConfirmView builds the SYS-102 retention-purge confirm
// page. TASK-047/SYS-152/UC-041 #5: it states the affected scope — how
// many athletes and meets currently sit outside the retention window — by
// re-running RetentionPurgePreviewCount's two read-only queries (the same
// ones purge() itself runs first), cheaply available since they are plain
// row-count lookups with no write. Falls back to the plain days-only
// description if the privacy service is unavailable or the preview query
// itself fails, rather than blocking the confirm page on it.
func (s *Server) retentionPurgeConfirmView(r *http.Request, actor app.Session, p PageData, fieldErr string) confirmView {
	desc := p.T("privacy.retention.confirm.description", "days", intToStr(int64(app.DefaultRetentionDays)))
	if s.privacy != nil {
		if athletes, meets, err := s.privacy.RetentionPurgePreviewCount(r.Context(), actor, app.DefaultRetentionDays); err == nil {
			desc = p.T("privacy.retention.confirm.description_scoped",
				"days", intToStr(int64(app.DefaultRetentionDays)),
				"athletes", intToStr(int64(athletes)),
				"meets", intToStr(int64(meets)))
		}
	}
	return confirmView{
		Title:             p.T("privacy.retention.confirm.title"),
		Description:       desc,
		FormAction:        "/admin/privacy/purge",
		CancelHref:        "/admin",
		ConfirmLabel:      p.T("privacy.retention.action"),
		TypedConfirm:      retentionPurgeConfirmToken,
		TypedConfirmField: "confirm_text",
		TypedConfirmLabel: p.T("privacy.retention.confirm.typed_label", "token", retentionPurgeConfirmToken),
		FieldErr:          fieldErr,
	}
}
