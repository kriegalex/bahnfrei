// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"errors"
	"net/http"

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/web/i18n"
)

// routes builds the full handler tree with its middleware chain:
// security headers -> locale resolution -> session resolution -> CSRF ->
// mux. Order matters: CSRF needs the session/locale-independent cookie
// jar already writable, and route handlers need locale+session on the
// context.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", s.handleHome)
	mux.HandleFunc("GET /login", s.handleLoginForm)
	mux.HandleFunc("POST /login", s.handleLoginSubmit)
	mux.HandleFunc("POST /logout", s.handleLogout)
	mux.HandleFunc("GET /locale", s.handleLocaleSwitch)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /events/{topic}", s.handleEvents)
	mux.HandleFunc("GET /admin", requireRole(app.RoleInstanceAdmin, s.cats, s.handleAdminPlaceholder))
	mux.Handle("GET /static/", staticHandler())
	// The capture service worker is served from a root-path URL so it can
	// claim the /meets/…/capture/ scope (UC-034 #3); see handleServiceWorker.
	mux.HandleFunc("GET /capture-sw.js", s.handleServiceWorker)
	mux.HandleFunc("/", s.handleNotFound)

	// First-run setup (UC-001 #1): available only while no account exists.
	mux.HandleFunc("GET /setup", s.handleSetupForm)
	mux.HandleFunc("POST /setup", s.handleSetupSubmit)

	// Meet setup workspace (UC-001 #2–#5), organizer-gated (SYS-090).
	organize := func(h http.HandlerFunc) http.HandlerFunc {
		return requireRole(app.RoleMeetOrganizer, s.cats, h)
	}
	mux.HandleFunc("GET /meets", organize(s.handleMeetsList))
	mux.HandleFunc("GET /meets/new", organize(s.handleMeetNewForm))
	mux.HandleFunc("POST /meets", organize(s.handleMeetCreate))
	mux.HandleFunc("GET /meets/from-template", organize(s.handleTemplateMeetForm))
	mux.HandleFunc("POST /meets/from-template", organize(s.handleTemplateMeetCreate))
	mux.HandleFunc("GET /meets/{id}", organize(s.handleMeetDetail))
	mux.HandleFunc("GET /meets/{id}/edit", organize(s.handleMeetEditForm))
	mux.HandleFunc("POST /meets/{id}/edit", organize(s.handleMeetEditSubmit))
	mux.HandleFunc("POST /meets/{id}/archive", organize(s.handleMeetArchive))
	mux.HandleFunc("POST /meets/{id}/events", organize(s.handleEventCreate))
	mux.HandleFunc("POST /meets/{id}/units/{unit}/schedule", organize(s.handleUnitSchedule))
	mux.HandleFunc("POST /meets/{id}/timetable/publish", organize(s.handleTimetablePublish))
	mux.HandleFunc("GET /meets/{id}/sanctioning", organize(s.handleSanctioning))

	// Roster & standings (UC-033): competition-office level and above —
	// day-of-competition surfaces (SYS-090).
	office := func(h http.HandlerFunc) http.HandlerFunc {
		return requireRole(app.RoleCompetitionOffice, s.cats, h)
	}
	mux.HandleFunc("GET /meets/{id}/roster", office(s.handleRoster))
	mux.HandleFunc("POST /meets/{id}/roster", office(s.handleRosterAdd))
	mux.HandleFunc("GET /meets/{id}/standings", office(s.handleStandings))

	// Field & track capture (TASK-008, UC-011/UC-010 subset): the on-venue
	// capture surface, field-official level and above (SYS-090; per-event
	// scoping arrives with TASK-013).
	captureRole := func(h http.HandlerFunc) http.HandlerFunc {
		return requireRole(app.RoleFieldOfficial, s.cats, h)
	}
	mux.HandleFunc("GET /meets/{id}/capture", captureRole(s.handleCaptureIndex))
	mux.HandleFunc("GET /meets/{id}/capture/{unit}", captureRole(s.handleCaptureUnit))
	mux.HandleFunc("GET /meets/{id}/capture/{unit}/standings", captureRole(s.handleCaptureStandings))
	mux.HandleFunc("POST /meets/{id}/capture/{unit}/attempt", captureRole(s.handleCaptureAttempt))
	mux.HandleFunc("POST /meets/{id}/capture/{unit}/track", captureRole(s.handleCaptureTrack))

	// Offline capture queue (TASK-009, UC-034 / SYS-085/086): the checkout
	// and replay endpoints are this app's one JSON API (see internal/sync
	// doc.go and internal/web/sync.go for why). Field-official level and above.
	mux.HandleFunc("POST /meets/{id}/capture/{unit}/checkout", captureRole(s.handleUnitCheckout))
	mux.HandleFunc("POST /meets/{id}/capture/{unit}/sync", captureRole(s.handleUnitSync))

	// Checkout override, start-list revision and reconciliation are office
	// actions (SYS-086, audited per SYS-046).
	mux.HandleFunc("POST /meets/{id}/capture/{unit}/override", office(s.handleCheckoutOverride))
	mux.HandleFunc("POST /meets/{id}/capture/{unit}/revise-startlist", office(s.handleReviseStartList))
	mux.HandleFunc("GET /meets/{id}/reconciliation", office(s.handleReconciliation))
	mux.HandleFunc("POST /meets/{id}/reconciliation/{item}/apply", office(s.handleReconciliationResolve(true)))
	mux.HandleFunc("POST /meets/{id}/reconciliation/{item}/discard", office(s.handleReconciliationResolve(false)))

	// Public read (SYS-090): the current published timetable, stable URL.
	mux.HandleFunc("GET /m/{id}/timetable", s.handlePublicTimetable)

	var h http.Handler = mux
	h = csrfMiddleware()(h)
	h = sessionMiddleware(s.sess)(h)
	h = localeMiddleware(s.cats)(h)
	h = securityHeaders(h)
	return h
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	// A fresh install lands on the setup flow until the first admin
	// account exists (UC-001 #1) — the operator never edits a file.
	if needs, err := s.auth.NeedsBootstrap(r.Context()); err == nil && needs {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	p := basePageData(r, s.cats)
	p.Title = p.T("home.welcome")
	_ = homePage(p).Render(r.Context(), w)
}

func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	p := basePageData(r, s.cats)
	p.Title = p.T("auth.login.title")
	_ = loginPage(p).Render(r.Context(), w)
}

func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")

	session, err := s.auth.Login(r.Context(), username, password)
	if err != nil {
		p := basePageData(r, s.cats)
		p.Title = p.T("auth.login.title")
		if errors.Is(err, app.ErrInvalidCredentials) {
			p.FlashError = p.T("auth.login.error")
			w.WriteHeader(http.StatusUnauthorized)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		_ = loginPage(p).Render(r.Context(), w)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.Token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
		Expires:  session.ExpiresAt,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		s.auth.Logout(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleLocaleSwitch persists the requested locale in a cookie (SYS-110:
// "users SHALL be able to switch language per session") if it names a
// locale this instance has loaded — including the pseudo-locale, which
// exists precisely so it can be selected this way for QA (UC-025 #2) —
// and redirects back to the referring page, or home if there is none.
func (s *Server) handleLocaleSwitch(w http.ResponseWriter, r *http.Request) {
	lang := i18n.Locale(r.URL.Query().Get("lang"))
	if s.cats.Has(lang) {
		http.SetCookie(w, &http.Cookie{
			Name:     localeCookieName,
			Value:    string(lang),
			Path:     "/",
			SameSite: http.SameSiteLaxMode,
			Secure:   r.TLS != nil,
		})
	}
	dest := r.Header.Get("Referer")
	if dest == "" {
		dest = "/"
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

// handleEvents serves the SSE stream for the {topic} path segment (the
// shell's live-update transport, SYS-071/ADR-003); publishing to a topic
// is a later task's job (TASK-006+ carries real content over this bus).
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	sseHandler(s.bus, func(r *http.Request) string { return r.PathValue("topic") })(w, r)
}

// handleAdminPlaceholder demonstrates the RBAC gate (SYS-090): reaching
// this handler at all means requireRole already confirmed an
// instance-admin session. Instance administration itself (account
// management UI) is TASK-013's job, not this shell's.
func (s *Server) handleAdminPlaceholder(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("admin: instance administration is not yet implemented (see TASK-013)\n"))
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	p := basePageData(r, s.cats)
	p.Title = p.T("error.not_found")
	_ = notFoundPage(p).Render(r.Context(), w)
}
