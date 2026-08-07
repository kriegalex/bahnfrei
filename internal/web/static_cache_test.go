// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"net/http"
	"strings"
	"testing"
)

// TestStaticAssetCorrectFingerprintIsImmutableDEC039TASK059 covers the
// happy path of the DEC-039/TASK-059 cache-discipline pass: a request
// carrying the asset's CURRENT content fingerprint (as produced by
// assetURL/computeAssetHashes) gets a long-lived, immutable Cache-Control,
// since the URL itself changes the next time the byte content does.
func TestStaticAssetCorrectFingerprintIsImmutableDEC039TASK059(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	hash, ok := assetHashes["/static/base.css"]
	if !ok {
		t.Fatal("assetHashes missing /static/base.css")
	}

	resp := mustGet(t, client, base+"/static/base.css?v="+hash)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /static/base.css?v=%s = %d, want 200", hash, resp.StatusCode)
	}
	if got := resp.Header.Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want the immutable long-lived directive", got)
	}
}

// TestStaticAssetNoFingerprintIsRevalidatableDEC039TASK059 covers a
// request with no "v" query parameter at all (an old template reference,
// or a person opening the URL directly): it must still succeed with the
// CURRENT bytes, and get a revalidation-friendly header rather than the
// long-lived immutable one (there is no fingerprint in the URL to make
// immutable caching safe).
func TestStaticAssetNoFingerprintIsRevalidatableDEC039TASK059(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	resp := mustGet(t, client, base+"/static/base.css")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /static/base.css = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}
	body := bodyString(t, resp)
	if len(body) == 0 {
		t.Error("GET /static/base.css returned an empty body")
	}
}

// TestStaticAssetStaleFingerprintServesCurrentBytesDEC039TASK059 is the
// stale-reference case a redeploy produces: a browser (or a bookmarked/
// shared link) holding a "?v=" from a PREVIOUS deploy must never 404 —
// it gets the CURRENT bytes with the same revalidation-friendly header as
// an unversioned request, so nothing breaks for old references.
func TestStaticAssetStaleFingerprintServesCurrentBytesDEC039TASK059(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	current, ok := assetHashes["/static/base.css"]
	if !ok {
		t.Fatal("assetHashes missing /static/base.css")
	}
	staleHash := current + "stale"
	if staleHash == current {
		t.Fatal("test setup: staleHash must differ from the current hash")
	}

	resp := mustGet(t, client, base+"/static/base.css?v="+staleHash)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /static/base.css?v=%s = %d, want 200 (never 404 on a stale v)", staleHash, resp.StatusCode)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache for a stale fingerprint", got)
	}

	// The bytes served must be the CURRENT asset, not a 404 body or a
	// cached-forever stale copy.
	currentResp := mustGet(t, client, base+"/static/base.css?v="+current)
	staleBody := bodyString(t, resp)
	currentBody := bodyString(t, currentResp)
	if staleBody != currentBody {
		t.Error("a stale ?v= must still serve the CURRENT bytes, not something else")
	}
}

// TestServiceWorkerRevalidatableAndVersionedDEC039TASK059 covers the SW
// script's own headers (always revalidatable, per DEC-039 — a browser
// must re-check it promptly on every navigation so a deploy's byte-
// different worker installs and prunes the previous cache) and the
// %BUNDLE_HASH% substitution: the served script's CACHE constant must
// contain the CURRENT combined fingerprint, not the literal placeholder.
func TestServiceWorkerRevalidatableAndVersionedDEC039TASK059(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	resp := mustGet(t, client, base+"/capture-sw.js")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /capture-sw.js = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}
	body := bodyString(t, resp)
	if strings.Contains(body, "%BUNDLE_HASH%") {
		t.Error("service worker script still contains the unsubstituted %BUNDLE_HASH% placeholder")
	}
	wantCache := `"bahnfrei-capture-` + bundleHash + `"`
	if !strings.Contains(body, wantCache) {
		t.Errorf("service worker script does not contain the current bundle-hash cache name %s", wantCache)
	}
}

// TestFaviconStaysReachableAndRevalidatableDEC039TASK059 pins the
// favicon's un-versioned-by-design status (DEC-039): a bare request (the
// browser's fixed /favicon.ico convention leaves no room for a "?v="
// query) and one with a stray query string both succeed and both get the
// revalidation-friendly header, never the long-lived immutable one.
func TestFaviconStaysReachableAndRevalidatableDEC039TASK059(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)

	for _, path := range []string{"/favicon.ico", "/favicon.ico?v=whatever"} {
		resp := mustGet(t, client, base+path)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, resp.StatusCode)
		}
		if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
			t.Errorf("GET %s Cache-Control = %q, want no-cache", path, got)
		}
	}
}

// TestLayoutReferencesCarryCurrentFingerprintDEC039TASK059 is the
// rendered-page guard the brief calls for: a template that reverts to a
// literal "/static/…" string (bypassing assetURL) would otherwise pass
// every other test silently. It asserts the layout's CSS/JS references
// carry the CURRENT "?v=" fingerprint for every asset assetURL is
// expected to cover.
func TestLayoutReferencesCarryCurrentFingerprintDEC039TASK059(t *testing.T) {
	deps := newTestServer(t, TLSConfig{Mode: TLSModeLocal})
	client, base := newTestClient(t, deps)
	setupAndLogin(t, client, base)

	resp := mustGet(t, client, base+"/")
	body := bodyString(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", resp.StatusCode)
	}

	for _, asset := range []string{"/static/htmx.min.js", "/static/tokens.css", "/static/base.css", "/static/help.js", "/static/office-banner.js"} {
		hash, ok := assetHashes[asset]
		if !ok {
			t.Fatalf("assetHashes missing %s", asset)
		}
		want := asset + "?v=" + hash
		if !strings.Contains(body, want) {
			t.Errorf("rendered layout does not reference %q (fingerprinted URL missing)", want)
		}
		if strings.Contains(body, `"`+asset+`"`) {
			t.Errorf("rendered layout still references the un-fingerprinted literal %q", asset)
		}
	}
}
