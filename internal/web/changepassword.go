// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/kriegalex/bahnfrei/internal/app"
)

// --- forced change-password step (TASK-053, DEC-030, SYS-090/091): reached
// after an admin-issued reset (accounts.go/confirm.go's ResetPassword UI).
// forcePasswordChangeGate (middleware.go) redirects every other
// authenticated route here until the operator completes it. ---

type changePasswordView struct {
	Errors FieldErrors
}

// mustChangePasswordOrRedirectHome is the shared guard for both handlers
// below: this step only exists for a session actually forced into it — a
// session that has already completed it (or was never forced) is sent
// home instead of getting a live form with nothing to do.
func (s *Server) mustChangePasswordOrRedirectHome(w http.ResponseWriter, r *http.Request) (app.Session, bool) {
	actor, ok := sessionFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return app.Session{}, false
	}
	if !actor.MustChangePassword {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return app.Session{}, false
	}
	return actor, true
}

func (s *Server) handleChangePasswordForm(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mustChangePasswordOrRedirectHome(w, r); !ok {
		return
	}
	p := basePageData(r, s.cats)
	p.Title = p.T("change_password.title")
	_ = changePasswordPage(p, changePasswordView{}).Render(r.Context(), w)
}

func (s *Server) renderChangePasswordForm(w http.ResponseWriter, r *http.Request, p PageData, errs FieldErrors, status int) {
	p.Title = p.T("change_password.title")
	w.WriteHeader(status)
	_ = changePasswordPage(p, changePasswordView{Errors: errs}).Render(r.Context(), w)
}

func (s *Server) handleChangePasswordSubmit(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.mustChangePasswordOrRedirectHome(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// This resubmits the current (temporary) password, so it is the same
	// credential-guessing surface as /login — the shared IP-keyed
	// loginLimiter (TASK-026, ratelimit.go) must cover it too, not just
	// /login itself.
	ip := clientIP(r)
	p := basePageData(r, s.cats)
	if s.loginLimiter.blocked(ip) {
		p.FlashError = p.T("change_password.error.rate_limited")
		w.Header().Set("Retry-After", strconv.Itoa(int(loginFailWindow.Seconds())))
		s.renderChangePasswordForm(w, r, p, nil, http.StatusTooManyRequests)
		return
	}

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	if newPassword != confirmPassword {
		s.renderChangePasswordForm(w, r, p, FieldErrors{
			"confirm_password": p.T("change_password.field_error.confirm_password.mismatch"),
		}, http.StatusUnprocessableEntity)
		return
	}

	if _, err := s.auth.ChangePassword(r.Context(), actor, currentPassword, newPassword); err != nil {
		switch {
		case errors.Is(err, app.ErrInvalidCredentials):
			s.loginLimiter.fail(ip)
			s.renderChangePasswordForm(w, r, p, FieldErrors{
				"current_password": p.T("change_password.field_error.current_password.invalid"),
			}, http.StatusUnprocessableEntity)
		case errors.Is(err, app.ErrPasswordTooShort):
			s.renderChangePasswordForm(w, r, p, FieldErrors{
				"new_password": p.T("change_password.field_error.new_password.too_short"),
			}, http.StatusUnprocessableEntity)
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	s.loginLimiter.reset(ip)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
