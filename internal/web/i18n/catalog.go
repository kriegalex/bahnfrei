// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

// Package i18n implements the message-catalog mechanism for SYS-110: every
// user-facing string is externalized under a stable key, DE and FR ship
// complete at first release, and adding a language is a translation-only
// change (no code change) — proven by discovering every locale file that
// exists under locales/*.json rather than hard-coding a fixed list, and by
// the pseudo-locale in pseudo.go (UC-025 #2).
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed locales/*.json
var localesFS embed.FS

// Locale is a BCP-47-ish language tag used as a catalog key ("de", "fr").
type Locale string

const (
	// DE and FR are the two complete launch languages (DEC-008).
	DE Locale = "de"
	FR Locale = "fr"
	// Default is used when no session/query/header locale is recognized.
	Default = DE
)

// requiredLocales are the launch languages that MUST ship complete
// (SYS-110); Load fails if either is missing. Any other *.json file under
// locales/ is picked up automatically with no further code change — that
// is the mechanism SYS-110 requires ("adding a language SHALL require
// only a translation file").
var requiredLocales = []Locale{DE, FR}

// referenceLocale is the authoritative key set every catalog must cover
// completely (UC-025 #1: "zero missing-key fallbacks"). DE is the
// reference: it is edited first when a new string is introduced, and every
// other catalog is checked against its keys.
const referenceLocale = DE

// Catalog maps message keys to localized text for one locale. Values may
// contain "{name}"-style placeholders substituted by Text.
type Catalog map[string]string

// Catalogs holds every loaded locale, keyed by Locale.
type Catalogs map[Locale]Catalog

// Load discovers and parses every embedded locales/*.json file — this is
// the "no code change" half of SYS-110: a new locale file dropped in this
// directory is picked up without touching Load itself. It fails closed:
// any discovered locale missing a key present in the reference locale is
// an error (UC-025 #1), and either required launch language missing at
// all is an error.
func Load() (Catalogs, error) {
	entries, err := fs.ReadDir(localesFS, "locales")
	if err != nil {
		return nil, fmt.Errorf("read locales dir: %w", err)
	}

	cats := make(Catalogs)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		loc := Locale(strings.TrimSuffix(name, ".json"))
		data, err := localesFS.ReadFile("locales/" + name)
		if err != nil {
			return nil, fmt.Errorf("read locale %q: %w", loc, err)
		}
		var c Catalog
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("parse locale %q: %w", loc, err)
		}
		cats[loc] = c
	}

	for _, loc := range requiredLocales {
		if _, ok := cats[loc]; !ok {
			return nil, fmt.Errorf("required locale %q not found under locales/", loc)
		}
	}

	ref, ok := cats[referenceLocale]
	if !ok {
		return nil, fmt.Errorf("reference locale %q not found under locales/", referenceLocale)
	}
	for loc, c := range cats {
		if loc == referenceLocale {
			continue
		}
		if missing := missingKeys(ref, c); len(missing) > 0 {
			return nil, fmt.Errorf("locale %q missing keys: %s", loc, strings.Join(missing, ", "))
		}
	}
	return cats, nil
}

// ReferenceKeys returns the sorted key set every catalog must cover.
func (c Catalogs) ReferenceKeys() []string {
	ref := c[referenceLocale]
	keys := make([]string, 0, len(ref))
	for k := range ref {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// missingKeys returns the keys present in ref but absent (or empty) in got,
// sorted for stable error messages.
func missingKeys(ref, got Catalog) []string {
	var missing []string
	for k, v := range ref {
		if v == "" {
			continue // an intentionally blank reference string requires nothing
		}
		if got[k] == "" {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	return missing
}

// Text looks up key in locale (falling back to the reference locale, then
// to the bracketed key itself, so a lookup miss is visible rather than a
// blank space in rendered HTML) and substitutes any "{name}" placeholders
// from args (given as alternating name/value pairs).
func (c Catalogs) Text(loc Locale, key string, args ...string) string {
	msg, ok := c[loc][key]
	if !ok || msg == "" {
		if msg, ok = c[referenceLocale][key]; !ok || msg == "" {
			return "[[" + key + "]]"
		}
	}
	for i := 0; i+1 < len(args); i += 2 {
		msg = strings.ReplaceAll(msg, "{"+args[i]+"}", args[i+1])
	}
	return msg
}

// Has reports whether loc is a catalog this instance knows about.
func (c Catalogs) Has(loc Locale) bool {
	_, ok := c[loc]
	return ok
}

// Locales returns every loaded locale tag, sorted.
func (c Catalogs) Locales() []Locale {
	out := make([]Locale, 0, len(c))
	for loc := range c {
		out = append(out, loc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
