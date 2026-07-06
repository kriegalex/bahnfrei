// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package i18n

import "strings"

// Pseudo is the pseudo-locale key (not a shipped language). It exists to
// prove, mechanically, that adding a language is a translation-only change
// (SYS-110, UC-025 #2): GeneratePseudo derives a complete catalog from the
// reference locale's key set alone, with no code change and no
// hand-written translation file — the same shape a real IT/EN addition
// would take, minus the human translation step.
const Pseudo Locale = "qps-ploc" // CLDR convention for pseudo-locale tags

// accentMap swaps a handful of ASCII letters for accented look-alikes, the
// classic pseudo-localization trick that flushes out any code path
// (truncation, non-UTF-8-safe string handling, hard-coded font
// assumptions) that only happens to work for plain ASCII DE/FR strings.
var accentMap = map[rune]rune{
	'a': 'á', 'e': 'é', 'i': 'í', 'o': 'ó', 'u': 'ú',
	'A': 'Á', 'E': 'É', 'I': 'Í', 'O': 'Ó', 'U': 'Ú',
}

// GeneratePseudo derives a pseudo-locale catalog from ref: every value is
// accented and padded (expansion mimics the ~30% length growth typical of
// German/French translations, a common source of layout bugs), and bracket
// markers make missing-key fallbacks in rendered pages easy to spot by
// eye. It is a pure function of ref, so it can regenerate identically at
// any time — nothing here is hand-authored per key.
func GeneratePseudo(ref Catalog) Catalog {
	out := make(Catalog, len(ref))
	for k, v := range ref {
		out[k] = pseudoize(v)
	}
	return out
}

// pseudoize accents every letter of s except inside "{placeholder}" spans:
// Text()'s substitution matches placeholders literally, so a pseudo-locale
// that mangled them would break every parameterized string it touched —
// a real translation file must leave them intact too.
func pseudoize(s string) string {
	var b strings.Builder
	b.WriteByte('[')
	inPlaceholder := false
	for _, r := range s {
		switch r {
		case '{':
			inPlaceholder = true
		case '}':
			inPlaceholder = false
		default:
			if !inPlaceholder {
				if accented, ok := accentMap[r]; ok {
					r = accented
				}
			}
		}
		b.WriteRune(r)
	}
	b.WriteString(" ~~~]") // expansion padding
	return b.String()
}
