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
			w.WriteHeader(http.StatusForbidden)
			p := basePageData(r, cats)
			p.Title = p.T("error.forbidden")
			_ = forbiddenPage(p).Render(r.Context(), w)
			return
		}
		next(w, r)
	}
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
	http.SetCookie(w, &http.Cookie{
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
// sniffing protections.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; frame-ancestors 'none'")
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
	}
	return p
}
