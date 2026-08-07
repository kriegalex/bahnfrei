// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"fmt"
	"io/fs"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// This file makes the WCAG AA contrast requirement DEC-036/TASK-056
// carries machine-checkable: it parses tokens.css directly (the embedded
// copy that actually ships, not a hand-copied literal) and computes the
// contrast ratio of every foreground/background pair the shell CSS
// actually renders together. A future retint that regresses contrast
// fails this test instead of shipping silently — scripts/check-style-
// tokens.sh only checks that literals live in tokens.css, never that
// they are legible together.

// tokenDeclRe matches a CSS custom-property declaration ("--name: value;")
// anywhere in the file; good enough for tokens.css's single flat :root
// block (no nested rules, no media queries around the color tokens).
var tokenDeclRe = regexp.MustCompile(`--([a-zA-Z0-9-]+)\s*:\s*([^;]+);`)

func parseTokensCSS(css string) map[string]string {
	tokens := map[string]string{}
	for _, m := range tokenDeclRe.FindAllStringSubmatch(css, -1) {
		name := strings.TrimSpace(m[1])
		if _, exists := tokens[name]; exists {
			continue // first declaration wins (root defines each token once)
		}
		tokens[name] = strings.TrimSpace(m[2])
	}
	return tokens
}

var varRefRe = regexp.MustCompile(`^var\(--([a-zA-Z0-9-]+)\)$`)

// resolveTokenColor looks up a named token and follows var(--other) chains
// (e.g. --focus-ring-color: var(--color-accent)) to a concrete color.
func resolveTokenColor(tokens map[string]string, name string) (r, g, b float64, err error) {
	val, ok := tokens[name]
	if !ok {
		return 0, 0, 0, fmt.Errorf("token --%s not found in tokens.css", name)
	}
	return resolveColorValue(tokens, val, 0)
}

func resolveColorValue(tokens map[string]string, val string, depth int) (float64, float64, float64, error) {
	if depth > 5 {
		return 0, 0, 0, fmt.Errorf("var() reference chain too deep resolving %q", val)
	}
	val = strings.TrimSpace(val)
	if m := varRefRe.FindStringSubmatch(val); m != nil {
		ref, ok := tokens[m[1]]
		if !ok {
			return 0, 0, 0, fmt.Errorf("token --%s (referenced via var()) not found", m[1])
		}
		return resolveColorValue(tokens, ref, depth+1)
	}
	return parseCSSColor(val)
}

func parseCSSColor(val string) (float64, float64, float64, error) {
	val = strings.TrimSpace(val)
	switch {
	case strings.HasPrefix(val, "#"):
		hex := val[1:]
		if len(hex) == 3 {
			expanded := make([]byte, 0, 6)
			for i := 0; i < 3; i++ {
				expanded = append(expanded, hex[i], hex[i])
			}
			hex = string(expanded)
		}
		if len(hex) != 6 && len(hex) != 8 {
			return 0, 0, 0, fmt.Errorf("unsupported hex color %q", val)
		}
		r, err1 := strconv.ParseInt(hex[0:2], 16, 64)
		g, err2 := strconv.ParseInt(hex[2:4], 16, 64)
		b, err3 := strconv.ParseInt(hex[4:6], 16, 64)
		if err1 != nil || err2 != nil || err3 != nil {
			return 0, 0, 0, fmt.Errorf("malformed hex color %q", val)
		}
		return float64(r), float64(g), float64(b), nil
	case strings.HasPrefix(val, "rgb(") || strings.HasPrefix(val, "rgba("):
		open := strings.Index(val, "(")
		closeIdx := strings.LastIndex(val, ")")
		if open < 0 || closeIdx < 0 || closeIdx < open {
			return 0, 0, 0, fmt.Errorf("malformed rgb()/rgba() value %q", val)
		}
		parts := strings.Split(val[open+1:closeIdx], ",")
		if len(parts) < 3 {
			return 0, 0, 0, fmt.Errorf("rgb()/rgba() value %q has fewer than 3 components", val)
		}
		r, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		g, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		b, err3 := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
		if err1 != nil || err2 != nil || err3 != nil {
			return 0, 0, 0, fmt.Errorf("malformed rgb()/rgba() components in %q", val)
		}
		return r, g, b, nil
	default:
		return 0, 0, 0, fmt.Errorf("unsupported color syntax %q (only #hex and rgb()/rgba() are handled)", val)
	}
}

// srgbChannelToLinear and relativeLuminance implement the WCAG 2.x formula
// (https://www.w3.org/TR/WCAG21/#dfn-relative-luminance).
func srgbChannelToLinear(c float64) float64 {
	c = c / 255.0
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func relativeLuminance(r, g, b float64) float64 {
	return 0.2126*srgbChannelToLinear(r) + 0.7152*srgbChannelToLinear(g) + 0.0722*srgbChannelToLinear(b)
}

// contrastRatio implements the WCAG 2.x contrast-ratio formula
// (https://www.w3.org/TR/WCAG21/#dfn-contrast-ratio): (L1+0.05)/(L2+0.05)
// with L1 the lighter of the two relative luminances.
func contrastRatio(r1, g1, b1, r2, g2, b2 float64) float64 {
	l1 := relativeLuminance(r1, g1, b1)
	l2 := relativeLuminance(r2, g2, b2)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

// TestTokensCSSContrastPairsMeetWCAG_AA_SYS116_DEC036 asserts every
// foreground/background pair the shell CSS actually renders together
// meets WCAG AA: 4.5:1 for normal text, 3:1 for large text/UI
// affordances (the focus ring, and the accent used as a link/large-text
// color). Pairs mirror base.css's real usage (see design-system.md §2's
// component inventory): the page body (fg/bg), the accent button/badge
// (accent-fg on accent, e.g. .help-trigger), the accent as a standalone
// large-text/UI color (focus ring, links), each status token's own
// bg/fg pairing (offline-status chip, three states), and every status
// -fg / danger-fg color used standalone as text/border color directly on
// the page background (.cell-save-badge, .error, .field-error, border-
// left rules) — not just inside its own tinted chip.
func TestTokensCSSContrastPairsMeetWCAG_AA_SYS116_DEC036(t *testing.T) {
	data, err := fs.ReadFile(staticAssets, "static/tokens.css")
	if err != nil {
		t.Fatalf("reading embedded tokens.css: %v", err)
	}
	tokens := parseTokensCSS(string(data))

	const (
		normalText = 4.5
		largeOrUI  = 3.0
	)

	type pair struct {
		desc     string
		fg, bg   string
		minRatio float64
	}
	pairs := []pair{
		{"body text: --color-fg on --color-bg", "color-fg", "color-bg", normalText},
		{"accent button/badge text: --color-accent-fg on --color-accent (.help-trigger)", "color-accent-fg", "color-accent", normalText},
		{"accent as large text/UI color on page bg (links, .brand wordmark)", "color-accent", "color-bg", largeOrUI},
		{"focus ring on page bg (UI affordance)", "focus-ring-color", "color-bg", largeOrUI},
		{"success chip: --color-success-fg on --color-success-bg (.offline-status)", "color-success-fg", "color-success-bg", normalText},
		{"warning chip: --color-warning-fg on --color-warning-bg (.offline-status)", "color-warning-fg", "color-warning-bg", normalText},
		{"warning-strong chip: --color-warning-strong-fg on --color-warning-strong-bg", "color-warning-strong-fg", "color-warning-strong-bg", normalText},
		{"info chip: --color-info-fg on --color-info-bg (.offline-status)", "color-info-fg", "color-info-bg", normalText},
		{"danger text on page bg (.error, .field-error, invalid border)", "color-danger-fg", "color-bg", normalText},
		{"success-fg used standalone on page bg (.cell-save-badge, border-left)", "color-success-fg", "color-bg", normalText},
		{"warning-fg used standalone on page bg (border-left, .offline-status[data-state=offline] text reused elsewhere)", "color-warning-fg", "color-bg", normalText},
		{"info-fg used standalone on page bg (.cell-save-badge, border-left)", "color-info-fg", "color-bg", normalText},
	}

	for _, p := range pairs {
		fr, fg, fb, err := resolveTokenColor(tokens, p.fg)
		if err != nil {
			t.Errorf("%s: resolving --%s: %v", p.desc, p.fg, err)
			continue
		}
		br, bgc, bb, err := resolveTokenColor(tokens, p.bg)
		if err != nil {
			t.Errorf("%s: resolving --%s: %v", p.desc, p.bg, err)
			continue
		}
		ratio := contrastRatio(fr, fg, fb, br, bgc, bb)
		if ratio < p.minRatio {
			t.Errorf("%s: contrast %.2f:1 is below the required %.1f:1 (--%s vs --%s)", p.desc, ratio, p.minRatio, p.fg, p.bg)
		}
	}
}
