// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/kriegalex/bahnfrei/internal/app"
)

// --- instance account administration (TASK-013: PoC accounts & audit,
// SYS-090/091, UC-022). Replaces the handleAdminPlaceholder stub TASK-005
// left at GET /admin. ---

// accountRoles lists the SYS-090 roles assignable through this PoC account
// UI, in privilege order; RolePublic is not a provisionable account role
// (it is the implicit, unauthenticated visitor).
var accountRoles = []app.Role{
	app.RoleInstanceAdmin, app.RoleMeetOrganizer, app.RoleCompetitionOffice,
	app.RoleFieldOfficial, app.RoleEntrySubmitter,
}

type roleOption struct {
	Value string
	Label string
}

type accountRowView struct {
	ID          string
	Username    string
	DisplayName string
	Role        string
	RoleLabel   string
	Enabled     bool
	Version     string
}

type accountsView struct {
	Rows  []accountRowView
	Roles []roleOption
}

func (s *Server) roleOptions(p PageData) []roleOption {
	out := make([]roleOption, 0, len(accountRoles))
	for _, r := range accountRoles {
		out = append(out, roleOption{Value: string(r), Label: p.T("role." + string(r))})
	}
	return out
}

func (s *Server) accountsView(r *http.Request, p PageData, actor app.Session) (accountsView, error) {
	accounts, err := s.auth.ListAccounts(r.Context(), actor)
	if err != nil {
		return accountsView{}, err
	}
	v := accountsView{Roles: s.roleOptions(p)}
	for _, a := range accounts {
		v.Rows = append(v.Rows, accountRowView{
			ID: a.ID, Username: a.Username, DisplayName: a.DisplayName,
			Role: a.Role, RoleLabel: p.T("role." + a.Role),
			Enabled: a.Enabled, Version: intToStr(a.Version),
		})
	}
	return v, nil
}

func (s *Server) handleAccountsList(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	p := basePageData(r, s.cats)
	p.Title = p.T("accounts.title")
	if msg := r.URL.Query().Get("err"); msg != "" {
		p.FlashError = p.T("accounts.error." + msg)
	}
	v, err := s.accountsView(r, p, actor)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	_ = accountsPage(p, v).Render(r.Context(), w)
}

// accountFlashKey maps an app-layer error to the "accounts.error.*" key
// suffix rendered on redirect back to the account list.
func accountFlashKey(err error) string {
	switch {
	case errors.Is(err, app.ErrDuplicateUsername):
		return "duplicate"
	case errors.Is(err, app.ErrLastEnabledAdmin):
		return "last_admin"
	default:
		return "invalid"
	}
}

func redirectAccountsError(w http.ResponseWriter, r *http.Request, err error) {
	http.Redirect(w, r, "/admin?err="+accountFlashKey(err), http.StatusSeeOther)
}

func (s *Server) handleAccountCreate(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req := app.CreateAccountRequest{
		Username:    strings.TrimSpace(r.FormValue("username")),
		DisplayName: strings.TrimSpace(r.FormValue("display_name")),
		Password:    r.FormValue("password"),
		Role:        app.Role(r.FormValue("role")),
	}
	if req.Username == "" || req.DisplayName == "" || req.Password == "" {
		redirectAccountsError(w, r, errBadInput)
		return
	}
	if _, err := s.auth.CreateAccount(r.Context(), actor, req); err != nil {
		redirectAccountsError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *Server) handleAccountEnable(w http.ResponseWriter, r *http.Request) {
	s.setAccountEnabled(w, r, true)
}
func (s *Server) handleAccountDisable(w http.ResponseWriter, r *http.Request) {
	s.setAccountEnabled(w, r, false)
}

func (s *Server) setAccountEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	actor, _ := sessionFromContext(r.Context())
	accountID := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if _, err := s.auth.SetAccountEnabled(r.Context(), actor, accountID, enabled, strings.TrimSpace(r.FormValue("reason"))); err != nil {
		redirectAccountsError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *Server) handleAccountRoleChange(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	accountID := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	newRole := app.Role(r.FormValue("role"))
	if _, err := s.auth.ChangeAccountRole(r.Context(), actor, accountID, newRole, strings.TrimSpace(r.FormValue("reason"))); err != nil {
		redirectAccountsError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

// handleAccountResetPassword completes the TASK-053/DEC-030 admin-issued
// password reset: the mutation half of the GET-confirm-sub-page flow
// (handleAccountResetConfirm, confirm.go). A too-short password re-renders
// the same confirm page with an inline field error (OQ-075's convention)
// rather than a page-level flash, since it is directly attributable to the
// one field the operator just typed into.
func (s *Server) handleAccountResetPassword(w http.ResponseWriter, r *http.Request) {
	actor, _ := sessionFromContext(r.Context())
	accountID := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	newPassword := r.FormValue("new_password")
	if _, err := s.auth.ResetPassword(r.Context(), actor, accountID, newPassword, strings.TrimSpace(r.FormValue("reason"))); err != nil {
		if errors.Is(err, app.ErrPasswordTooShort) {
			username, ok, uerr := s.accountUsername(r, actor, accountID)
			if uerr != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if !ok {
				s.handleNotFound(w, r)
				return
			}
			p := basePageData(r, s.cats)
			v := s.accountResetConfirmView(p, accountID, username, p.T("accounts.reset.field_error.new_password.too_short"))
			s.renderConfirm(w, r, p, v, http.StatusUnprocessableEntity)
			return
		}
		redirectAccountsError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}
