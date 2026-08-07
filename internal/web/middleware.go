// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"github.com/kriegalex/bahnfrei/internal/app"
	"github.com/kriegalex/bahnfrei/internal/web/i18n"
)

// Cookie names for the shell's session, CSRF and locale state.
const (
	sessionCookieName = "bf_session"
	csrfCookieName    = "bf_csrf"
	localeCookieName  = "bf_locale"
)

type ctxKey int

const (
	ctxKeySession ctxKey = iota
	ctxKeyLocale
	ctxKeyCSRF
)

// sessionFromContext returns the request's session, if authenticated.
func sessionFromContext(ctx context.Context) (app.Session, bool) {
	s, ok := ctx.Value(ctxKeySession).(app.Session)
	return s, ok
}

func localeFromContext(ctx context.Context) i18n.Locale {
	loc, ok := ctx.Value(ctxKeyLocale).(i18n.Locale)
	if !ok {
		return i18n.Default
	}
	return loc
}

func csrfFromContext(ctx context.Context) string {
	tok, _ := ctx.Value(ctxKeyCSRF).(string)
	return tok
}

// sessionMiddleware resolves the bearer session cookie (if any) to a live
// app.Session and stores it on the request context (SYS-091). Unlike
// requireRole, it never rejects a request — anonymous access to public
// pages is a first-class case (SYS-090's "unauthenticated public read").
func sessionMiddleware(sessions *app.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if c, err := r.Cookie(sessionCookieName); err == nil {
				if s, err := sessions.Lookup(c.Value); err == nil {
					ctx = context.WithValue(ctx, ctxKeySession, s)
					// Authenticated responses can carry personal data
					// (athlete PII, exports); keep them out of shared and
					// browser caches (SYS-092, ASVS L2 V8.2.1 / nFADP data
					// minimization). A later handler is free to override.
					w.Header().Set("Cache-Control", "no-store")
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// requireRole rejects requests whose session role does not meet min,
// serving a localized 403 (SYS-090 least privilege).
func requireRole(min app.Role, cats i18n.Catalogs, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, ok := sessionFromContext(r.Context())
		role := app.RolePublic
		if ok {
			role = s.Role
		}
		if !role.AtLeast(min) {
			renderForbidden(w, r, cats)
			return
		}
		next(w, r)
	}
}

// forcePasswordChangeExemptPaths are the routes a session with
// MustChangePassword=true may still reach (TASK-053): the change-password
// step itself (GET renders the form, POST submits it), logout (an operator
// who does not want to complete the step right now can still leave), and
// the handful of asset/utility routes no page can function without
// (static assets, favicon, the locale switcher, healthz).
var forcePasswordChangeExemptPaths = map[string]bool{
	"/change-password": true,
	"/logout":          true,
	"/locale":          true,
	"/healthz":         true,
	"/favicon.ico":     true,
}

// forcePasswordChangeGate redirects every request on an authenticated
// session with MustChangePassword=true to the change-password step
// (TASK-053, DEC-030, SYS-090/091), until that step clears the flag — an
// admin-issued temporary password only ever grants access to setting a real
// one. It must sit in the middleware chain after sessionMiddleware (so the
// session is already on the context) and applies to state-changing POSTs
// exactly like GETs, so a forced-change session cannot route around the
// gate by acting on a route directly.
func (s *Server) forcePasswordChangeGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sess, ok := sessionFromContext(r.Context()); ok && sess.MustChangePassword {
			if !forcePasswordChangeExemptPaths[r.URL.Path] && !strings.HasPrefix(r.URL.Path, "/static/") {
				http.Redirect(w, r, "/change-password", http.StatusSeeOther)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// renderForbidden serves the shared localized 403 page (SYS-090 least
// privilege): the coarse role gate above and the per-event unit-scoping
// gate (TASK-013, UC-022 #1) both end here so a denial always looks the
// same to the operator.
func renderForbidden(w http.ResponseWriter, r *http.Request, cats i18n.Catalogs) {
	w.WriteHeader(http.StatusForbidden)
	p := basePageData(r, cats)
	p.Title = p.T("error.forbidden")
	_ = forbiddenPage(p).Render(r.Context(), w)
}

// localeMiddleware resolves the active locale for the request — from the
// bf_locale cookie if it names a locale this instance has loaded,
// otherwise from a coarse Accept-Language match, otherwise
// i18n.Default — and stores it on the context (SYS-110: "users SHALL be
// able to switch language per session").
func localeMiddleware(cats i18n.Catalogs) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			loc := resolveLocale(r, cats)
			ctx := context.WithValue(r.Context(), ctxKeyLocale, loc)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func resolveLocale(r *http.Request, cats i18n.Catalogs) i18n.Locale {
	if c, err := r.Cookie(localeCookieName); err == nil {
		if loc := i18n.Locale(c.Value); cats.Has(loc) {
			return loc
		}
	}
	for _, tag := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag = strings.TrimSpace(strings.SplitN(tag, ";", 2)[0])
		tag = strings.ToLower(strings.SplitN(tag, "-", 2)[0])
		if loc := i18n.Locale(tag); cats.Has(loc) {
			return loc
		}
	}
	return i18n.Default
}

// csrfMiddleware implements a double-submit-cookie CSRF defense (SYS-092:
// baseline ASVS-L2 hygiene). A random token is issued as an HttpOnly
// cookie on first contact; forms embed the same value (read server-side
// from the request context, so no JavaScript access to the cookie is
// needed) as a hidden "csrf_token" field. State-changing methods must
// present a matching value or are rejected.
func csrfMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The timing-agent API (TASK-020, ADR-006) authenticates with a
			// meet-scoped Bearer token, never the browser session cookie the
			// double-submit-cookie scheme below defends — a page a victim's
			// browser is tricked into POSTing to can never know or attach
			// that token, so CSRF does not apply to it (the standard
			// bearer-token-API exemption; see internal/web/timing.go).
			if strings.HasPrefix(r.URL.Path, "/agent/v1/") {
				next.ServeHTTP(w, r)
				return
			}
			tok, err := ensureCSRFCookie(w, r)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			ctx := context.WithValue(r.Context(), ctxKeyCSRF, tok)
			r = r.WithContext(ctx)

			if isStateChanging(r.Method) {
				if err := checkCSRF(r, tok); err != nil {
					http.Error(w, "CSRF check failed", http.StatusForbidden)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// maxRequestBodyBytes is the hard ceiling on any single request body
// (SYS-092, ASVS L2 V12.1.1: bound request/upload size to prevent memory- or
// disk-exhaustion DoS). It sits comfortably above the largest legitimate
// upload — a 5 MiB timing/import file plus its multipart envelope
// (maxImportUploadBytes / maxTimingUploadBytes) — and below anything a
// well-behaved client would ever send.
const maxRequestBodyBytes = 8 << 20

// limitRequestBody caps every request body at max. It MUST sit outside the
// CSRF middleware in the chain: checkCSRF reads the token via r.FormValue,
// which parses the whole multipart body (up to Go's 32 MiB default) — so
// without this cap in front, an oversized upload is fully buffered before any
// handler runs, defeating a per-handler MaxBytesReader entirely. Honest
// clients (browsers, the timing agent) send Content-Length and get a clean
// 413; chunked or mis-declared bodies are still bounded by the wrapped reader.
func limitRequestBody(max int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > max {
				http.Error(w, "request entity too large", http.StatusRequestEntityTooLarge)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, max)
			next.ServeHTTP(w, r)
		})
	}
}

func isStateChanging(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

var errCSRFMismatch = errors.New("csrf token missing or mismatched")

func checkCSRF(r *http.Request, cookieToken string) error {
	if cookieToken == "" {
		return errCSRFMismatch
	}
	submitted := r.Header.Get("X-CSRF-Token")
	if submitted == "" {
		submitted = r.FormValue("csrf_token")
	}
	if subtle.ConstantTimeCompare([]byte(submitted), []byte(cookieToken)) != 1 {
		return errCSRFMismatch
	}
	return nil
}

func ensureCSRFCookie(w http.ResponseWriter, r *http.Request) (string, error) {
	if c, err := r.Cookie(csrfCookieName); err == nil && c.Value != "" {
		return c.Value, nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tok := base64.RawURLEncoding.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{ // #nosec G124 -- Secure is deliberately conditional on r.TLS, matching the session cookie: self-signed venue-local and dev-only plaintext modes still need this cookie set
		Name:     csrfCookieName,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	})
	return tok, nil
}

// securityHeaders sets the small set of response headers appropriate for a
// server-rendered, HTMX-enhanced app with no third-party origins (every
// asset is embedded, ADR-003) — a conservative CSP, clickjacking and MIME
// sniffing protections (SYS-092, ASVS L2 V14.4).
//
// HSTS is sent ONLY in ACME mode. In venue-local mode the certificate is
// self-signed, so operators reach the hub past a browser trust prompt;
// HSTS would forbid that click-through and lock the venue out of its own
// system (a self-inflicted DoS). TLSModeOff serves plaintext, where HSTS is
// meaningless. So HSTS is correct only where a publicly-trusted cert exists.
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	hsts := s.cfg.TLS.Mode == TLSModeACME
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; frame-ancestors 'none'")
		if hsts {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// basePageData builds the PageData common to every page from request
// context set up by the middlewares above.
func basePageData(r *http.Request, cats i18n.Catalogs) PageData {
	loc := localeFromContext(r.Context())
	p := PageData{
		Locale:    loc,
		Locales:   []i18n.Locale{i18n.DE, i18n.FR},
		Cats:      cats,
		CSRFToken: csrfFromContext(r.Context()),
	}
	if s, ok := sessionFromContext(r.Context()); ok {
		p.LoggedIn = true
		p.Username = s.Username
		p.CanOrganize = s.Role.AtLeast(app.RoleMeetOrganizer)
		p.CanManageAccounts = s.Role.AtLeast(app.RoleInstanceAdmin)
		p.CanViewAudit = s.Role.AtLeast(app.RoleCompetitionOffice)
	}
	return p
}
