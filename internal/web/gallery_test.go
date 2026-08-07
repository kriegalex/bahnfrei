// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"net/http"
	"strings"
	"testing"
)

// TestDesignGalleryRendersEveryInventoriedComponentSYS116UC038_2 asserts the
// dev/fixture gallery page (UC-038 #2, SYS-116) is reachable unauthenticated
// (it carries no meet data/PII) and renders one instance of every component
// docs/architecture/design-system.md inventories, in every documented state
// (default, disabled, invalid/error) — the e2e keyboard walk
// (e2e/tests/design-gallery.spec.ts) covers the focus-visible/interactive
// half of UC-038 #2 that a Go HTTP test cannot.
func TestDesignGalleryRendersEveryInventoriedComponentSYS116UC038_2(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	resp := mustGet(t, client, base+"/dev/design-gallery")
	body := bodyString(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /dev/design-gallery = %d, want 200; body=%s", resp.StatusCode, body)
	}

	wantIDs := []string{
		"gallery-button-default",
		"gallery-button-disabled",
		"gallery-link-default",
		"gallery-input-text",
		"gallery-input-number",
		"gallery-input-date",
		"gallery-input-checkbox",
		"gallery-select",
		"gallery-fieldset-a",
		"gallery-fieldset-b",
		"gallery-input-disabled",
		"gallery-input-invalid",
		"gallery-input-invalid-error",
		"help-gallery-example",
		"gallery-status-online",
		"gallery-status-offline",
		"gallery-status-syncing",
		"gallery-unofficial-label",
		"gallery-alert-error",
		"gallery-offline-banner",
	}
	for _, id := range wantIDs {
		if !strings.Contains(body, `id="`+id+`"`) {
			t.Errorf("gallery page missing component id=%q", id)
		}
	}

	// Disabled state (SYS-116: every interactive component documents one).
	if !strings.Contains(body, `disabled`) {
		t.Errorf("gallery page missing a disabled-state control")
	}
	// Invalid/error state paired with an inline message (SYS-117, UC-038 #4).
	if !strings.Contains(body, `aria-invalid="true"`) {
		t.Errorf("gallery page missing an aria-invalid control")
	}
	// Not linked from the shell nav (dev/fixture scope).
	if strings.Contains(bodyString(t, mustGet(t, client, base+"/healthz")), "/dev/design-gallery") {
		t.Errorf("dev/design-gallery must not be linked from public surfaces")
	}
}

// TestSelectMinHeightTokenSYS116TASK051N4 covers N4 (release-0.1 usability
// audit, TASK-051): the header locale <select> measured 109×21px —
// WCAG-conformant via the 2.5.8 spacing exception, but below the
// checklist's 24px comfort bar. tokens.css now defines
// --control-min-height-sm (24px) and base.css applies it to every
// <select>; scripts/check-style-tokens.sh enforces that the value itself
// lives only in tokens.css. This Go test is the CSS-presence half N4's
// finding calls for (an e2e pixel measurement duplicating
// mobile-capture-UC039.spec.ts's pattern is unnecessary — the rule is
// unconditional CSS, not viewport- or state-dependent).
func TestSelectMinHeightTokenSYS116TASK051N4(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	tokens := bodyString(t, mustGet(t, client, base+"/static/tokens.css"))
	if !strings.Contains(tokens, "--control-min-height-sm:") {
		t.Errorf("tokens.css missing --control-min-height-sm: %s", tokens)
	}

	css := bodyString(t, mustGet(t, client, base+"/static/base.css"))
	if !strings.Contains(css, "select {\n  min-height: var(--control-min-height-sm);\n}") {
		t.Errorf("base.css missing the select min-height rule wired to --control-min-height-sm: %s", css)
	}
}

// TestDesignGalleryNotLinkedFromShellNavSYS116UC038_2 pins the "not public
// nav" scope constraint: the gallery route exists, but the authenticated
// shell nav (layout.templ) never links to it.
func TestDesignGalleryNotLinkedFromShellNavSYS116UC038_2(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	if _, err := deps.auth.Bootstrap(t.Context(), "admin", "Admin", "s3cret-passphrase"); err != nil {
		t.Fatal(err)
	}
	home := bodyString(t, mustGet(t, client, base+"/"))
	if strings.Contains(home, "/dev/design-gallery") {
		t.Errorf("home page shell nav must not link to the dev/fixture gallery")
	}
}
