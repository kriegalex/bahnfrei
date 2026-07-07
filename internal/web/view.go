// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"strconv"

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
	CSRFToken   string
	// FlashError, when non-empty, renders as a one-shot alert (e.g. a
	// failed login attempt); it is never persisted.
	FlashError string
}

// T looks up a localized, parameter-substituted message (see
// i18n.Catalogs.Text).
func (p PageData) T(key string, args ...string) string {
	return p.Cats.Text(p.Locale, key, args...)
}

// localeLabel renders the display name of loc in the page's own language,
// e.g. "Französisch" while viewing the DE catalog.
func localeLabel(p PageData, loc i18n.Locale) string {
	return p.Cats.Text(p.Locale, "locale."+string(loc))
}

// intToStr renders an integer for a template attribute/value position.
func intToStr(v int64) string { return strconv.FormatInt(v, 10) }
