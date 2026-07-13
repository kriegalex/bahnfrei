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
