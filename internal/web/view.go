// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"strconv"
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
}

// T looks up a localized, parameter-substituted message (see
// i18n.Catalogs.Text).
func (p PageData) T(key string, args ...string) string {
	return p.Cats.Text(p.Locale, key, args...)
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

// FormatDateTime renders t for display, normalized to UTC (the system's
// storage/display convention throughout) with an explicit "UTC" suffix so
// a rendered timestamp is never ambiguous about its zone (SYS-110).
func (p PageData) FormatDateTime(t time.Time) string {
	return t.UTC().Format(dateTimeDisplayLayout) + " UTC"
}

// localeLabel renders the display name of loc in the page's own language,
// e.g. "Französisch" while viewing the DE catalog.
func localeLabel(p PageData, loc i18n.Locale) string {
	return p.Cats.Text(p.Locale, "locale."+string(loc))
}

// intToStr renders an integer for a template attribute/value position.
func intToStr(v int64) string { return strconv.FormatInt(v, 10) }
