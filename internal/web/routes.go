// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

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
	// Forced change-password step (TASK-053, DEC-030, SYS-090/091): reached
	// after an admin-issued reset. Any authenticated session may use it
	// (not instance-admin gated) since it only ever changes the actor's own
	// password; forcePasswordChangeGate (middleware.go) is what routes a
	// forced session here from everywhere else.
	mux.HandleFunc("GET /change-password", s.handleChangePasswordForm)
	mux.HandleFunc("POST /change-password", s.handleChangePasswordSubmit)
	mux.HandleFunc("GET /locale", s.handleLocaleSwitch)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /events/{topic}", s.handleEvents)
	mux.Handle("GET /static/", staticHandler())
	// Browsers probe GET /favicon.ico at the root regardless of the <link
	// rel="icon"> in layout.templ (which points at /static/favicon.ico for
	// the normal page-load path); serving it here too closes that 404
	// (F11/TASK-050).
	mux.HandleFunc("GET /favicon.ico", s.handleFavicon)
	// The capture service worker is served from a root-path URL so it can
	// claim the /meets/…/capture/ scope (UC-034 #3); see handleServiceWorker.
	mux.HandleFunc("GET /capture-sw.js", s.handleServiceWorker)
	// Component-gallery fixture (TASK-032, UC-038 #2, SYS-116): dev/fixture
	// scope, deliberately never linked from the shell nav (layout.templ).
	// Static markup only — no meet data, no PII, no privileged action — so
	// it is safe unauthenticated, like /healthz.
	mux.HandleFunc("GET /dev/design-gallery", s.handleDesignGallery)
	mux.HandleFunc("/", s.handleNotFound)

	// First-run setup (UC-001 #1): available only while no account exists.
	mux.HandleFunc("GET /setup", s.handleSetupForm)
	mux.HandleFunc("POST /setup", s.handleSetupSubmit)

	// Instance account administration (TASK-013, SYS-090/091, UC-022):
	// create/disable accounts and assign instance-wide roles. Instance-
	// admin only (CapManageAccounts).
	admin := func(h http.HandlerFunc) http.HandlerFunc {
		return requireRole(app.RoleInstanceAdmin, s.cats, h)
	}
	mux.HandleFunc("GET /admin", admin(s.handleAccountsList))
	// One-action backup download (TASK-014, SYS-084, UC-020 #3) lives on
	// the same instance-admin surface.
	mux.HandleFunc("GET /admin/backup", admin(s.handleBackupDownload))
	// SYS-102 retention-purge manual trigger (TASK-023, UC-024 #3):
	// instance-admin, the same tier as backup — a whole-instance,
	// irreversible action.
	mux.HandleFunc("GET /admin/privacy", admin(s.handleRetentionPurgeForm))
	// OQ-074 (TASK-034): a GET confirm sub-page in front of the purge POST,
	// with a typed-confirmation token (defense-in-depth on this
	// irreversible, instance-wide action per the ASVS review's note).
	mux.HandleFunc("GET /admin/privacy/purge/confirm", admin(s.handleRetentionPurgeConfirm))
	mux.HandleFunc("POST /admin/privacy/purge", admin(s.handleRetentionPurge))
	mux.HandleFunc("POST /admin/accounts", admin(s.handleAccountCreate))
	mux.HandleFunc("POST /admin/accounts/{id}/enable", admin(s.handleAccountEnable))
	// OQ-074: plain GET confirm sub-page (reversible action — enable exists).
	mux.HandleFunc("GET /admin/accounts/{id}/disable/confirm", admin(s.handleAccountDisableConfirm))
	mux.HandleFunc("POST /admin/accounts/{id}/disable", admin(s.handleAccountDisable))
	mux.HandleFunc("POST /admin/accounts/{id}/role", admin(s.handleAccountRoleChange))
	// TASK-053/DEC-030: admin-issued one-time password reset — the
	// meet-morning-lockout fix, no email infrastructure. OQ-074-style GET
	// confirm sub-page (the accounts page's existing mutation-confirmation
	// pattern) in front of the POST.
	mux.HandleFunc("GET /admin/accounts/{id}/reset/confirm", admin(s.handleAccountResetConfirm))
	mux.HandleFunc("POST /admin/accounts/{id}/reset", admin(s.handleAccountResetPassword))

	// Meet setup workspace (UC-001 #2–#5), organizer-gated (SYS-090).
	organize := func(h http.HandlerFunc) http.HandlerFunc {
		return requireRole(app.RoleMeetOrganizer, s.cats, h)
	}
	// office is declared here — ahead of the roster/standings routes it
	// originally guarded — so the meet-detail hub route just below can use
	// it too (TASK-043).
	office := func(h http.HandlerFunc) http.HandlerFunc {
		return requireRole(app.RoleCompetitionOffice, s.cats, h)
	}
	mux.HandleFunc("GET /meets", organize(s.handleMeetsList))
	mux.HandleFunc("GET /meets/new", organize(s.handleMeetNewForm))
	mux.HandleFunc("POST /meets", organize(s.handleMeetCreate))
	mux.HandleFunc("GET /meets/from-template", organize(s.handleTemplateMeetForm))
	mux.HandleFunc("POST /meets/from-template", organize(s.handleTemplateMeetCreate))
	// The meet-detail hub (TASK-043, OQ-111, SYS-090/091/114) is opened to
	// competition-office sessions: it is the only link target the
	// roster/entries/reconciliation pages' back-to-meet link offers, and
	// the only route reaching check-in, seeding, timing exchange, entries
	// import/eligibility and privacy. meetDetailPage/meetDetailView
	// (meets.templ/meets.go) filter the rendered action list on
	// p.CanOrganize so organizer-only actions (edit, archive, publish,
	// sanctioning, bib/fee/exception management, programme/timetable
	// mutation) stay hidden for an office session; their POST/GET routes
	// below remain organize()-gated, unweakened.
	mux.HandleFunc("GET /meets/{id}", office(s.handleMeetDetail))
	mux.HandleFunc("GET /meets/{id}/edit", organize(s.handleMeetEditForm))
	mux.HandleFunc("POST /meets/{id}/edit", organize(s.handleMeetEditSubmit))
	// OQ-074: plain GET confirm sub-page (a status change, not data
	// destruction — no typed confirmation).
	mux.HandleFunc("GET /meets/{id}/archive/confirm", organize(s.handleMeetArchiveConfirm))
	mux.HandleFunc("POST /meets/{id}/archive", organize(s.handleMeetArchive))
	mux.HandleFunc("POST /meets/{id}/publish", organize(s.handleMeetPublish))
	mux.HandleFunc("POST /meets/{id}/events", organize(s.handleEventCreate))
	mux.HandleFunc("POST /meets/{id}/units/{unit}/schedule", organize(s.handleUnitSchedule))
	mux.HandleFunc("POST /meets/{id}/timetable/publish", organize(s.handleTimetablePublish))
	mux.HandleFunc("GET /meets/{id}/sanctioning", organize(s.handleSanctioning))

	// Online entries (TASK-016, UC-003, SYS-011/012/015): entry-submitter
	// level and above (CapSubmitEntries, SYS-090).
	submitEntries := func(h http.HandlerFunc) http.HandlerFunc {
		return requireRole(app.RoleEntrySubmitter, s.cats, h)
	}
	mux.HandleFunc("GET /meets/{id}/entries", submitEntries(s.handleEntries))
	mux.HandleFunc("POST /meets/{id}/entries/individual", submitEntries(s.handleEntryIndividualSubmit))
	mux.HandleFunc("POST /meets/{id}/entries/bulk", submitEntries(s.handleEntryBulkSubmit))
	mux.HandleFunc("POST /meets/{id}/entries/relay", submitEntries(s.handleEntryRelaySubmit))
	mux.HandleFunc("POST /meets/{id}/entries/{entry}/relay-composition", submitEntries(s.handleEntryRelayCompositionUpdate))

	// Entry-standard exceptions, bib assignment and the fee schedule/summary
	// (TASK-016, UC-006, SYS-015/017/018): organizer level, matching UC-006's
	// "operator (organizer)" actor.
	mux.HandleFunc("GET /meets/{id}/entries/exceptions", organize(s.handleEntryExceptions))
	mux.HandleFunc("GET /meets/{id}/bibs", organize(s.handleBibs))
	mux.HandleFunc("GET /meets/{id}/bibs.pdf", organize(s.handleBibsPDF))
	mux.HandleFunc("POST /meets/{id}/bibs/bulk", organize(s.handleBibBulkAssign))
	mux.HandleFunc("POST /meets/{id}/bibs/{participant}", organize(s.handleBibAssign))
	mux.HandleFunc("GET /meets/{id}/fees", organize(s.handleFees))
	mux.HandleFunc("POST /meets/{id}/fees/schedule", organize(s.handleFeeScheduleSubmit))
	mux.HandleFunc("GET /meets/{id}/fees/export", organize(s.handleFeeExport))

	// Roster & standings (UC-033): competition-office level and above —
	// day-of-competition surfaces (SYS-090). office is declared above,
	// alongside organize.
	mux.HandleFunc("GET /meets/{id}/roster", office(s.handleRoster))
	mux.HandleFunc("POST /meets/{id}/roster", office(s.handleRosterAdd))
	// Participant identity correction (TASK-049, SYS-150/UC-043): a
	// dedicated form page per roster row, not an inline/modal edit — same
	// GET-form/POST-submit shape as the meet-edit form (meets.go).
	mux.HandleFunc("GET /meets/{id}/roster/{participant}/edit", office(s.handleParticipantEditForm))
	mux.HandleFunc("POST /meets/{id}/roster/{participant}/edit", office(s.handleParticipantEditSubmit))
	mux.HandleFunc("GET /meets/{id}/standings", office(s.handleStandings))
	mux.HandleFunc("GET /meets/{id}/export/ukc-series", office(s.handleSeriesUploadExport))

	// Data-subject rights (TASK-023, SYS-101/SYS-103, UC-023 #2/#3,
	// UC-024 #1/#2): office level and above, per-meet entry point onto an
	// athlete's (instance-global) data.
	mux.HandleFunc("GET /meets/{id}/privacy", office(s.handlePrivacyList))
	mux.HandleFunc("POST /meets/{id}/privacy/{athlete}/consent", office(s.handlePrivacyConsentToggle))
	mux.HandleFunc("GET /meets/{id}/privacy/{athlete}/export", office(s.handlePrivacyExport))
	// OQ-074: a GET confirm sub-page with a typed-confirmation token (type
	// the athlete's bib) in front of the erase POST — the strongest
	// friction of the four destructive actions, matching the ASVS review's
	// defense-in-depth note on this irreversible-and-unrecoverable action.
	mux.HandleFunc("GET /meets/{id}/privacy/{athlete}/erase/confirm", office(s.handlePrivacyEraseConfirm))
	mux.HandleFunc("POST /meets/{id}/privacy/{athlete}/erase", office(s.handlePrivacyErase))

	// CSV entry import & eligibility exceptions (TASK-017, UC-004/UC-005,
	// SYS-013/014/010): competition-office level (CapOfficeActions,
	// SYS-090) — same tier as check-in/day-of-competition operations.
	mux.HandleFunc("GET /meets/{id}/entries/import", office(s.handleEntriesImportForm))
	mux.HandleFunc("POST /meets/{id}/entries/import", office(s.handleEntriesImportSubmit))
	mux.HandleFunc("GET /meets/{id}/entries/eligibility", office(s.handleEligibilityList))
	mux.HandleFunc("POST /meets/{id}/entries/{entry}/eligibility/override", office(s.handleEligibilityOverride))

	// Check-in / call-room and DNS handling (TASK-018, UC-007, SYS-025):
	// office level, matching UC-007's "operator (check-in/call room)" actor.
	mux.HandleFunc("GET /meets/{id}/events/{event}/checkin", office(s.handleCheckIn))
	mux.HandleFunc("GET /meets/{id}/events/{event}/checkin/close/confirm", office(s.handleCheckInCloseConfirm))
	mux.HandleFunc("POST /meets/{id}/events/{event}/checkin/close", office(s.handleCheckInClose))
	mux.HandleFunc("POST /meets/{id}/entries/{entry}/confirm", office(s.handleCheckInConfirm))
	mux.HandleFunc("POST /meets/{id}/entries/{entry}/reinstate", office(s.handleCheckInReinstate))

	// Heat seeding, lane draws and round progression (TASK-018, UC-008/009,
	// SYS-026-030): office level, matching UC-008/009's "operator
	// (competition office)" actor.
	mux.HandleFunc("GET /meets/{id}/events/{event}/rounds/{round}/seeding", office(s.handleSeeding))
	mux.HandleFunc("POST /meets/{id}/events/{event}/rounds/{round}/seeding/generate", office(s.handleSeedingGenerate))
	mux.HandleFunc("POST /meets/{id}/events/{event}/rounds/{round}/seeding/override", office(s.handleSeedingOverride))
	mux.HandleFunc("POST /meets/{id}/events/{event}/rounds/{round}/advance", office(s.handleAdvanceRound))
	mux.HandleFunc("POST /meets/{id}/events/{event}/rounds/{round}/manual-advance", office(s.handleManualAdvance))

	// Printables (TASK-011, UC-018 subset, SYS-072, PoC scope): the UKC
	// result list as PDF is office-level, matching the standings page it
	// mirrors.
	mux.HandleFunc("GET /meets/{id}/standings.pdf", office(s.handleResultListPDF))

	// Field-official per-event scoping (TASK-013, SYS-090 "assignable per
	// meet"; UC-022 #1): office level and above assigns which units a
	// field official may capture on this meet.
	mux.HandleFunc("GET /meets/{id}/officials", office(s.handleFieldOfficials))
	mux.HandleFunc("POST /meets/{id}/officials/assign", office(s.handleFieldOfficialAssign))
	mux.HandleFunc("POST /meets/{id}/officials/unassign", office(s.handleFieldOfficialUnassign))

	// Privileged-action audit surfacing (TASK-013, SYS-091/UC-022 #2):
	// office level and above; instance-wide, not meet-scoped, since
	// privileged actions (account changes, role grants) are not all
	// meet-local.
	mux.HandleFunc("GET /audit", office(s.handleAuditLog))

	// Field & track capture (TASK-008, UC-011/UC-010 subset): the on-venue
	// capture surface, field-official level and above (SYS-090). Per-event
	// scoping (TASK-013, UC-022 #1) is enforced inside internal/app —
	// captureRole here is only the coarse role floor.
	captureRole := func(h http.HandlerFunc) http.HandlerFunc {
		return requireRole(app.RoleFieldOfficial, s.cats, h)
	}
	mux.HandleFunc("GET /meets/{id}/capture", captureRole(s.handleCaptureIndex))
	mux.HandleFunc("GET /meets/{id}/capture/{unit}", captureRole(s.handleCaptureUnit))
	mux.HandleFunc("GET /meets/{id}/capture/{unit}/standings", captureRole(s.handleCaptureStandings))
	mux.HandleFunc("GET /meets/{id}/capture/{unit}/record-checklist/{athlete}", captureRole(s.handleRecordChecklist))
	mux.HandleFunc("POST /meets/{id}/capture/{unit}/attempt", captureRole(s.handleCaptureAttempt))
	mux.HandleFunc("POST /meets/{id}/capture/{unit}/track", captureRole(s.handleCaptureTrack))
	mux.HandleFunc("GET /meets/{id}/capture/{unit}/sheet.pdf", captureRole(s.handleCaptureSheetPDF))

	// Vertical jump capture (TASK-021, UC-012/SYS-043): the unit page and
	// its standings fragment reuse the routes above — handleCaptureUnit/
	// handleCaptureStandings branch on discipline family internally. Trial
	// capture is field-official level (same floor as attempt/track);
	// configuring the bar-height progression (including a jump-off height)
	// is office-only, matching SYS-043's "office-configurable".
	mux.HandleFunc("POST /meets/{id}/capture/{unit}/vertical-trial", captureRole(s.handleCaptureVerticalTrial))
	mux.HandleFunc("POST /meets/{id}/capture/{unit}/vertical-heights", office(s.handleCaptureVerticalHeights))

	// Full track capture & corrections (TASK-019, UC-010/UC-015,
	// SYS-040/046/047): wind is entered at the same capture-role floor as
	// ordinary results (a field official records the race's wind reading);
	// announcing a result list and correcting it once announced are office
	// actions (UC-015's "operator (competition office)" actor).
	mux.HandleFunc("POST /meets/{id}/capture/{unit}/wind", captureRole(s.handleCaptureWind))
	mux.HandleFunc("POST /meets/{id}/capture/{unit}/announce", office(s.handleCaptureAnnounce))
	mux.HandleFunc("POST /meets/{id}/capture/{unit}/correct", office(s.handleCaptureCorrect))

	// Bulk "mark remaining as DNS" (TASK-041, DEC-025/OQ-070, SYS-114/
	// SYS-046): office-only, scoped to still-open track units — the TASK-034
	// confirm sub-page pattern (GET confirm fronting the POST) rather than a
	// bare button.
	mux.HandleFunc("GET /meets/{id}/capture/{unit}/bulk-dns/confirm", office(s.handleCaptureBulkDNSConfirm))
	mux.HandleFunc("POST /meets/{id}/capture/{unit}/bulk-dns", office(s.handleCaptureBulkDNS))

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

	// Timing exchange (TASK-020, UC-014, SYS-060-062, ADR-006): office-level
	// export/import/conflict-resolution and timing-agent token management.
	mux.HandleFunc("GET /meets/{id}/timing", office(s.handleTimingIndex))
	mux.HandleFunc("GET /meets/{id}/timing/export/ppl", office(s.handleTimingExportPPL))
	mux.HandleFunc("GET /meets/{id}/timing/export/sch", office(s.handleTimingExportSCH))
	mux.HandleFunc("GET /meets/{id}/timing/export/evt", office(s.handleTimingExportEVT))
	mux.HandleFunc("GET /meets/{id}/timing/export/csv", office(s.handleTimingExportCSV))
	mux.HandleFunc("POST /meets/{id}/timing/import", office(s.handleTimingImportSubmit))
	mux.HandleFunc("POST /meets/{id}/timing/conflicts/{conflict}/resolve", office(s.handleTimingConflictResolve))
	mux.HandleFunc("POST /meets/{id}/timing/agents", office(s.handleTimingAgentTokenCreate))
	mux.HandleFunc("POST /meets/{id}/timing/agents/{token}/revoke", office(s.handleTimingAgentTokenRevoke))

	// Timing-agent HTTP API (ADR-006's hub-first amendment): the watched-
	// folder agent on the timing PC authenticates with its own meet-scoped
	// Bearer token (internal/web/timing.go's authenticateAgent), never a
	// browser session — deliberately unguarded by requireRole/office here
	// (and exempted from CSRF, middleware.go) since it is not a
	// cookie-authenticated surface at all.
	mux.HandleFunc("POST /agent/v1/meets/{id}/lif", s.handleAgentImportLIF)
	mux.HandleFunc("POST /agent/v1/meets/{id}/csv", s.handleAgentImportCSV)
	mux.HandleFunc("GET /agent/v1/meets/{id}/exports/manifest", s.handleAgentExportManifest)
	mux.HandleFunc("GET /agent/v1/meets/{id}/exports/{kind}", s.handleAgentExportFile)

	// Public read (SYS-090/070): unauthenticated, stable /m/{id}/... URLs
	// that keep serving a meet's archived state after it closes (UC-017).
	mux.HandleFunc("GET /m/{id}", s.handlePublicMeet)
	mux.HandleFunc("GET /m/{id}/timetable", s.handlePublicTimetable)
	mux.HandleFunc("GET /m/{id}/startlists", s.handlePublicStartLists)
	mux.HandleFunc("GET /m/{id}/results", s.handlePublicResults)
	mux.HandleFunc("GET /m/{id}/results/live", s.handlePublicResultsLive)

	var h http.Handler = mux
	h = csrfMiddleware()(h)
	// Bound the request body before CSRF parses it (see limitRequestBody).
	h = limitRequestBody(maxRequestBodyBytes)(h)
	// TASK-053: must run after sessionMiddleware sets the context (so it can
	// read MustChangePassword) but before the router, so it can intercept
	// every route, not just a hand-picked set.
	h = s.forcePasswordChangeGate(h)
	h = sessionMiddleware(s.sess)(h)
	h = localeMiddleware(s.cats)(h)
	h = s.securityHeaders(h)
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
	// A logged-in session below the meet-organizer floor gets its "my
	// assignments" dashboard instead of the generic hub content (TASK-042,
	// DEC-025 — closes OQ-089): competition office, field official and
	// entry submitter each have a meet-scoped panel; organizer+ (CanOrganize)
	// keeps the unchanged /meets-pointing flow below.
	if actor, ok := sessionFromContext(r.Context()); ok && !p.CanOrganize {
		v, err := s.buildAssignmentsDashboard(r.Context(), p, actor)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		_ = assignmentsDashboardPage(p, v).Render(r.Context(), w)
		return
	}
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

	// Anti-automation gate (SYS-092, ASVS L2 V2.2.1): once an IP has burned
	// its failed-attempt budget, reject before spending an argon2id verify.
	// The generic "invalid credentials" message is reused deliberately so a
	// throttled response discloses nothing about account existence.
	ip := clientIP(r)
	if s.loginLimiter.blocked(ip) {
		p := basePageData(r, s.cats)
		p.Title = p.T("auth.login.title")
		p.FlashError = p.T("auth.login.error")
		w.Header().Set("Retry-After", strconv.Itoa(int(loginFailWindow.Seconds())))
		w.WriteHeader(http.StatusTooManyRequests)
		_ = loginPage(p).Render(r.Context(), w)
		return
	}

	session, err := s.auth.Login(r.Context(), username, password)
	if err != nil {
		p := basePageData(r, s.cats)
		p.Title = p.T("auth.login.title")
		switch {
		case errors.Is(err, app.ErrInvalidCredentials):
			// Only bad credentials count toward the brute-force budget; a
			// disabled account (correct password) does not.
			s.loginLimiter.fail(ip)
			p.FlashError = p.T("auth.login.error")
			w.WriteHeader(http.StatusUnauthorized)
		case errors.Is(err, app.ErrAccountDisabled):
			// Correct credentials, but the account was disabled (SYS-091):
			// distinguishable from ErrInvalidCredentials without disclosing
			// anything to a guesser, since it only ever surfaces after a
			// successful password check (see app.ErrAccountDisabled's doc).
			p.FlashError = p.T("auth.login.disabled")
			w.WriteHeader(http.StatusForbidden)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
		_ = loginPage(p).Render(r.Context(), w)
		return
	}

	// Successful login clears the failure counter for this source.
	s.loginLimiter.reset(ip)

	http.SetCookie(w, &http.Cookie{ // #nosec G124 -- Secure is deliberately conditional on r.TLS, not a literal true: TLSModeLocal's self-signed venue deployments and TLSModeOff (dev/e2e-only) still need the cookie sent over plain HTTP
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
	http.SetCookie(w, &http.Cookie{ // #nosec G124 -- Secure is deliberately conditional on r.TLS, matching handleLogin's session cookie (self-signed venue / dev-only plaintext modes)
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
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
		http.SetCookie(w, &http.Cookie{ // #nosec G124 -- Secure is deliberately conditional on r.TLS: this non-sensitive locale preference still needs to be set over plain HTTP in venue-local self-signed / dev-only modes
			Name:     localeCookieName,
			Value:    string(lang),
			Path:     "/",
			SameSite: http.SameSiteLaxMode,
			Secure:   r.TLS != nil,
		})
	}
	http.Redirect(w, r, sameOriginRedirectTarget(r.Header.Get("Referer"), r.Host), http.StatusSeeOther) // #nosec G710 -- sameOriginRedirectTarget (below) rejects any non-root-relative/off-host value and falls back to "/"
}

// sameOriginRedirectTarget closes OQ-079: redirecting to the raw Referer
// header after a locale switch can send the browser to an absolute
// off-origin URL if the Referer happens to name one. Assessed low severity —
// the value comes from the victim's own browser-set header, never an
// attacker-supplied query parameter, so it was never a practical phishing
// primitive — but a same-origin/relative-only guard closes it cheaply:
// accept the Referer only when it is root-relative (a single leading "/",
// not "//" which browsers treat as protocol-relative/off-origin) or when its
// host matches the current request's host; anything else — a different
// host, a malformed URL, or an empty header — falls back to "/".
func sameOriginRedirectTarget(referer, host string) string {
	if referer == "" {
		return "/"
	}
	if strings.HasPrefix(referer, "/") && !strings.HasPrefix(referer, "//") {
		return referer
	}
	u, err := url.Parse(referer)
	if err != nil || u.Host == "" || !strings.EqualFold(u.Host, host) {
		return "/"
	}
	return referer
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

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	p := basePageData(r, s.cats)
	p.Title = p.T("error.not_found")
	_ = notFoundPage(p).Render(r.Context(), w)
}

// handleDesignGallery renders the TASK-032 component-gallery fixture (see
// gallery.templ for scope/rationale).
func (s *Server) handleDesignGallery(w http.ResponseWriter, r *http.Request) {
	p := basePageData(r, s.cats)
	p.Title = p.T("gallery.title")
	_ = galleryPage(p).Render(r.Context(), w)
}
