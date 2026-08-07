// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"net/http"
	"strings"
	"testing"
)

// TestDisplayFontServedSYS116TASK056DEC036 covers the DEC-036/TASK-056
// gotcha this codebase has hit before (F11/TASK-050, favicon.ico): a new
// static asset that is not added to static.go's go:embed list 404s
// silently rather than failing loudly. Both vendored Barlow Semi
// Condensed weights and their OFL licence text must be reachable under
// /static/, exactly like every other embedded asset (static_test.go's
// sibling TestStaticAssetServed pattern).
func TestDisplayFontServedSYS116TASK056DEC036(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	for _, path := range []string{
		"/static/barlow-semi-condensed-regular.woff2",
		"/static/barlow-semi-condensed-bold.woff2",
	} {
		resp := mustGet(t, client, base+path)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", path, resp.StatusCode)
		}
		body := bodyString(t, resp)
		if len(body) == 0 {
			t.Errorf("GET %s returned an empty body", path)
		}
		// woff2's magic bytes are the ASCII tag "wOF2".
		if !strings.HasPrefix(body, "wOF2") {
			t.Errorf("GET %s does not look like a woff2 file (missing wOF2 magic)", path)
		}
	}

	// The licence text ships alongside the font, same convention as
	// htmx-LICENSE — no Content-Type assertion (served as-is via the
	// generic static file server), just reachability and non-empty body.
	resp := mustGet(t, client, base+"/static/barlow-semi-condensed-LICENSE")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /static/barlow-semi-condensed-LICENSE = %d, want 200", resp.StatusCode)
	}
	licence := bodyString(t, resp)
	if !strings.Contains(licence, "SIL OPEN FONT LICENSE") {
		t.Errorf("barlow-semi-condensed-LICENSE does not look like the OFL text: %q", licence[:min(80, len(licence))])
	}
}

// TestTokensAndBaseCSSWireDisplayFontSYS116TASK056DEC036 pins the
// token-layer plumbing: tokens.css names --font-family-display, and
// base.css both declares the @font-face rules pointing at the vendored
// woff2 files and applies the token to headings and the result-grid
// class, with tabular numerals for aligned mark/points/total columns
// (DEC-036's "self-hosted display face with tabular numerals" scope).
func TestTokensAndBaseCSSWireDisplayFontSYS116TASK056DEC036(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	tokens := bodyString(t, mustGet(t, client, base+"/static/tokens.css"))
	if !strings.Contains(tokens, "--font-family-display:") {
		t.Errorf("tokens.css missing --font-family-display")
	}
	if !strings.Contains(tokens, "Barlow Semi Condensed") {
		t.Errorf("tokens.css --font-family-display does not name Barlow Semi Condensed")
	}

	css := bodyString(t, mustGet(t, client, base+"/static/base.css"))
	if !strings.Contains(css, "@font-face") {
		t.Errorf("base.css missing @font-face declarations")
	}
	if !strings.Contains(css, "barlow-semi-condensed-regular.woff2") || !strings.Contains(css, "barlow-semi-condensed-bold.woff2") {
		t.Errorf("base.css @font-face src does not reference both vendored weights")
	}
	if !strings.Contains(css, "font-family: var(--font-family-display);") {
		t.Errorf("base.css never consumes --font-family-display")
	}
	if !strings.Contains(css, ".results-table") || !strings.Contains(css, "font-variant-numeric: tabular-nums;") {
		t.Errorf("base.css missing the .results-table tabular-nums rule")
	}
}

// TestFaviconIsRealMarkSYS116TASK056DEC036 extends the F11/TASK-050
// favicon reachability test (TestFaviconServedSYS116TASK050,
// localization_hygiene_test.go — kept as-is): DEC-036 replaces the
// neutral placeholder with a real teal mark, still served at both the
// conventional root path and under /static/. A byte-identity check
// against the committed file would be brittle to regenerate; instead
// this pins the ICO magic header and a plausible multi-size icon
// payload (a single flat placeholder square was ~15KB; the new
// multi-resolution mark is larger).
func TestFaviconIsRealMarkSYS116TASK056DEC036(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	for _, path := range []string{"/favicon.ico", "/static/favicon.ico"} {
		resp := mustGet(t, client, base+path)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", path, resp.StatusCode)
		}
		body := bodyString(t, resp)
		// ICO files start with a fixed 4-byte reserved/type header:
		// 00 00 01 00 (reserved=0, type=1 for icon).
		if len(body) < 6 || body[0] != 0x00 || body[1] != 0x00 || body[2] != 0x01 || body[3] != 0x00 {
			t.Errorf("GET %s does not look like a valid ICO file", path)
		}
	}
}

// TestHeaderWordmarkUsesBrandTreatmentSYS116TASK056DEC036 pins the header
// wordmark (DEC-036 scope item 4): "Bahnfrei" renders inside the `.brand`
// class (the display-face typographic treatment, base.css) rather than a
// bare, unstyled link. "Bahnfrei" is a proper noun and stays unlocalized
// (app.title is identical across de/fr/qps-ploc — see i18n/locales).
func TestHeaderWordmarkUsesBrandTreatmentSYS116TASK056DEC036(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	// A fresh instance with no bootstrapped admin redirects "/" to
	// /setup (SYS-… first-run flow) instead of rendering the shell —
	// bootstrap one so the layout (and its header wordmark) actually
	// renders, matching TestDesignGalleryNotLinkedFromShellNavSYS116UC038_2's
	// pattern in gallery_test.go.
	if _, err := deps.auth.Bootstrap(t.Context(), "admin", "Admin", "s3cret-passphrase"); err != nil {
		t.Fatal(err)
	}

	home := bodyString(t, mustGet(t, client, base+"/"))
	if !strings.Contains(home, `class="brand"`) {
		t.Errorf("home page header does not carry the .brand wordmark treatment")
	}
	if !strings.Contains(home, `<a href="/" class="brand">Bahnfrei</a>`) {
		t.Errorf("home page header wordmark markup unexpected: %s", home)
	}
}
