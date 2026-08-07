// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"strconv"
	"strings"
	"time"

	"github.com/kriegalex/bahnfrei/internal/web/i18n"
)

// PageData is the view model every page template renders from: locale
// state (SYS-110), the authenticated user (if any, SYS-090/091), and CSRF
// protection for any form on the page. Keeping this one small struct as
// the template boundary means templates never see *app.Session, *sql.DB,
// or any store type directly — only what rendering needs.
type PageData struct {
	Locale   i18n.Locale
	Locales  []i18n.Locale
	Cats     i18n.Catalogs
	Title    string
	Username string // "" when anonymous
	LoggedIn bool
	// CanOrganize gates the operator navigation (SYS-090: the meets
	// workspace is for meet-organizer roles and above).
	CanOrganize bool
	// CanManageAccounts gates the account-administration nav link
	// (TASK-013, SYS-090: instance-admin only).
	CanManageAccounts bool
	// CanViewAudit gates the privileged-action audit log nav link
	// (TASK-013, SYS-091/UC-022 #2: office level and above).
	CanViewAudit bool
	CSRFToken    string
	// FlashError, when non-empty, renders as a one-shot alert (e.g. a
	// failed login attempt); it is never persisted.
	FlashError string
	// CurrentPath is the request's URL path (set once in basePageData from
	// r.URL.Path), consumed only by NavCurrent below for the header nav's
	// aria-current="page" indicator (DEC-037, TASK-057) — never rendered
	// or otherwise inspected by a template directly.
	CurrentPath string
}

// T looks up a localized, parameter-substituted message (see
// i18n.Catalogs.Text).
func (p PageData) T(key string, args ...string) string {
	return p.Cats.Text(p.Locale, key, args...)
}

// NavCurrent reports whether the request path sits inside the nav section
// rooted at prefix (an exact match, or one path segment deeper) — the
// header nav's aria-current="page" indicator (DEC-037, TASK-057, SYS-116).
// A prefix match rather than an exact one, since a section's own sub-pages
// (e.g. "/meets/{id}") should still mark "Meets" current, not just the
// section's own index route.
func (p PageData) NavCurrent(prefix string) bool {
	return p.CurrentPath == prefix || strings.HasPrefix(p.CurrentPath, prefix+"/")
}

// TPlural resolves base+".one" or base+".other" per i18n.PluralOne(p.Locale,
// n) and substitutes "{n}" with n (SYS-110, N3/TASK-051 pluralization —
// e.g. "1 Ergebnis" vs "0"/"2+" "Ergebnisse").
func (p PageData) TPlural(base string, n int) string {
	return p.Cats.TextPlural(p.Locale, base, n)
}

// FilterCountSingularSet lists, comma-separated, the small counts that
// select the ".one" (singular) plural category for the page's locale per
// i18n.PluralOne — "1" for German, "0,1" for French. public-filter.ts
// reads this to pick the right client-side count template as the
// live-filtered count changes (SYS-110, N3/TASK-051 pluralization).
func (p PageData) FilterCountSingularSet() string {
	if i18n.PluralOne(p.Locale, 0) {
		return "0,1"
	}
	return "1"
}

// dateDisplayLayout/dateTimeDisplayLayout are the SYS-110 "documented
// project convention" for rendering dates and timestamps to a person:
// day.month.year (Swiss/DE/FR convention — both MVP launch languages share
// it, C7.3). This is intentionally distinct from formDateLayout /
// formDateTimeLayout in meets.go, which are the fixed ISO wire formats
// HTML <input type="date"/"datetime-local"> requires regardless of locale;
// form pre-fill values must keep using those, never FormatDate/FormatDateTime.
const (
	dateDisplayLayout     = "02.01.2006"
	dateTimeDisplayLayout = "02.01.2006 15:04"
)

// FormatDate renders t for display in the page's locale (SYS-110: "dates
// ... format per locale convention"). Per-locale divergence is a hook, not
// yet a need: DE and FR (the only MVP launch languages, DEC-008) share the
// same day.month.year convention.
func (p PageData) FormatDate(t time.Time) string {
	return t.Format(dateDisplayLayout)
}

// FormatDateTime renders t for display in the meet's local time (SYS-110:
// "locale-correct formatting SHALL apply" — Swiss meets run on local time,
// not raw UTC; a volunteer reading "publiziert am 06.08.2026 05:29 UTC" at
// 07:29 local has no reason to trust the clock, F6/TASK-050). Every stored
// instant is UTC internally (see the store package's *.UTC() write path);
// this converts to the server process's local zone for display. There is
// currently no per-meet timezone field in the data model — Swiss Athletics
// meets are all Europe/Zurich in practice, and the server is assumed to run
// in that zone, but a multi-timezone deployment (or a server misconfigured
// away from the meet's zone) would render a wrong local time with no way to
// correct it from the product. Raised as OQ-137 (does bahnfrei need an
// explicit per-meet timezone field, independent of the server's OS zone?)
// rather than inventing that schema change here.
func (p PageData) FormatDateTime(t time.Time) string {
	return t.Local().Format(dateTimeDisplayLayout)
}

// localeLabel renders the display name of loc in the page's own language,
// e.g. "Französisch" while viewing the DE catalog.
func localeLabel(p PageData, loc i18n.Locale) string {
	return p.Cats.Text(p.Locale, "locale."+string(loc))
}

// intToStr renders an integer for a template attribute/value position.
func intToStr(v int64) string { return strconv.FormatInt(v, 10) }
