// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"sort"
	"strings"

	"github.com/a-h/templ"
)

// staticFS embeds the shell's static assets so the binary is fully
// self-contained (ADR-002/ADR-003: no runtime asset fetch, no CDN — the
// venue-local mode must work with zero internet access). htmx.min.js is
// htmx 2.0.4, vendored verbatim (Zero-Clause BSD license, htmx-LICENSE);
// tokens.css and base.css are project-authored (TASK-032: tokens.css holds
// every design token, base.css consumes them — docs/architecture/design-system.md).
//
// capture.js is the capture page's SSE-driven standings refresh (UC-011 #4).
// public-live.js is the public results page's SSE-driven refresh (UC-017
// #1, SYS-071) — a separate file so the unauthenticated surface has no
// dependency on the authenticated capture page's script.
// capture-offline.js, office-banner.js and service-worker.js are the TASK-009
// offline-capture islands (UC-034; SYS-085/087), compiled from islands/*.ts by
// scripts/build-islands — the emitted JS is committed and embedded so the
// binary is self-contained (ADR-002/ADR-003).
// help.js is the contextual-help island (TASK-031, SYS-115): open/close
// behavior for the help-icon component (help.templ) — a static asset
// because the CSP forbids inline scripts.
// public-filter.js is the public find-your-athlete filter island
// (TASK-048, SYS-153/UC-042), compiled from islands/src/public-filter.ts —
// loaded on both the public results and start-list pages.
// favicon.ico is the ratified Bahnfrei brand mark (DEC-036, TASK-056: a
// teal track-lane roundrect, replacing the F11/TASK-050 neutral
// placeholder now that OQ-061's brand question is decided).
// capture-markers.js is the letter-marker quick-action wiring (SYS-147,
// UC-039 #3, TASK-045): a small, hand-authored, family-agnostic script
// (not a TS island) loaded by the field-horizontal and vertical-jump
// capture pages.
// barlow-semi-condensed-{regular,bold}.woff2 and -LICENSE are the
// self-hosted display face (DEC-036, TASK-056; SIL OFL 1.1, licence text
// alongside per the htmx-LICENSE precedent) — latin-subset, vendored
// once at build/dev time (ADR-002/ADR-003: no runtime fetch), served
// under /static/ like every other asset here and consumed via
// tokens.css's --font-family-display.
//
//go:embed static/htmx.min.js static/htmx-LICENSE static/tokens.css static/base.css static/capture.js static/capture-markers.js static/public-live.js static/public-filter.js static/capture-offline.js static/office-banner.js static/service-worker.js static/help.js static/favicon.ico static/barlow-semi-condensed-regular.woff2 static/barlow-semi-condensed-bold.woff2 static/barlow-semi-condensed-LICENSE
var staticAssets embed.FS

// assetHashes maps each asset's "/static/…" URL path to a content
// fingerprint (the first 12 hex chars of the SHA-256 of its embedded
// bytes), and bundleHash is a single fingerprint combining every embedded
// asset in a deterministic order. Both are computed once at package init
// — the embed.FS is fixed at compile time, so there is nothing to
// recompute at request time (DEC-039/TASK-059).
//
// assetHashes backs assetURL (below) and the static handler's
// immutable-vs-revalidate decision (staticHandler below). bundleHash seeds
// the service worker's cache name (handleServiceWorker below) so a deploy
// that changes any static asset changes the worker script's bytes, and its
// existing activate handler prunes the previous cache.
var (
	assetHashes map[string]string
	bundleHash  string
)

func init() {
	assetHashes, bundleHash = computeAssetHashes(staticAssets)
}

// computeAssetHashes walks the embedded static/ tree in sorted path order
// (for determinism — fs.WalkDir does not itself guarantee an order across
// Go versions) and returns the per-asset hash map plus a combined hash
// over every asset's bytes.
func computeAssetHashes(fsys embed.FS) (map[string]string, string) {
	var paths []string
	err := fs.WalkDir(fsys, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		panic(err) // embed layout is fixed at compile time
	}
	sort.Strings(paths)

	hashes := make(map[string]string, len(paths))
	combined := sha256.New()
	for _, p := range paths {
		b, err := fsys.ReadFile(p)
		if err != nil {
			panic(err) // embed layout is fixed at compile time
		}
		sum := sha256.Sum256(b)
		urlPath := "/static/" + strings.TrimPrefix(p, "static/")
		hashes[urlPath] = hex.EncodeToString(sum[:])[:12]
		combined.Write(b)
	}
	return hashes, hex.EncodeToString(combined.Sum(nil))[:12]
}

// assetURL returns p (a "/static/…" path) with its current content
// fingerprint appended as a "v" query parameter (DEC-039/TASK-059):
// templates use this instead of a literal "/static/…" string for every
// CSS/JS reference, so a byte change to the asset changes the URL the
// browser is asked to fetch. Root-path entries (/favicon.ico,
// /capture-sw.js) are deliberately NOT run through this helper — their own
// handlers set a revalidation-friendly Cache-Control instead (see
// handleFavicon/handleServiceWorker below).
//
// An unrecognized path (impossible for a compiled-in literal, but cheap to
// guard) is returned unchanged rather than panicking, so a template render
// never 500s over an asset-URL typo.
func assetURL(p string) templ.SafeURL {
	if h, ok := assetHashes[p]; ok {
		return templ.URL(p + "?v=" + h)
	}
	return templ.URL(p)
}

// staticHandler serves the embedded static assets under /static/, adding
// the DEC-039/TASK-059 cache-discipline headers ahead of the generic file
// server: a request whose "v" query parameter matches the asset's current
// fingerprint is safe to cache forever (the URL itself changes on the next
// deploy); anything else — no "v", or a stale one from a previous
// deploy — gets the CURRENT bytes with a revalidation header, so an old
// reference never breaks (no 404, no permanently-stale cache).
func staticHandler() http.Handler {
	sub, err := fs.Sub(staticAssets, "static")
	if err != nil {
		panic(err) // embed layout is fixed at compile time
	}
	fileServer := http.StripPrefix("/static/", http.FileServerFS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h, ok := assetHashes[r.URL.Path]; ok && r.URL.Query().Get("v") == h {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	})
}

// handleFavicon serves the embedded favicon at the conventional root path
// (F11/TASK-050): browsers request GET /favicon.ico regardless of the
// <link rel="icon"> in layout.templ, so /static/favicon.ico alone still
// left that bare-path request 404ing. Served un-versioned by design
// (DEC-039/TASK-059): the fixed browser-chrome convention for
// /favicon.ico leaves no room to plumb a "?v=" fingerprint through, so it
// gets the same revalidation-friendly header as any other unversioned
// static reference.
func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	body, err := staticAssets.ReadFile("static/favicon.ico")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/vnd.microsoft.icon")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(body)
}

// handleServiceWorker serves the capture-surface service worker (UC-034 #3)
// from a root-path URL (/capture-sw.js). A service worker may only claim a
// scope at or below its own script path, so the worker that must control
// /meets/{id}/capture/… is served here rather than under /static/. The
// Service-Worker-Allowed header explicitly permits the /meets/ scope.
//
// The embedded script contains the literal placeholder "%BUNDLE_HASH%" in
// its CACHE constant (islands/sw/service-worker.ts); it is substituted
// with the current bundleHash here, at serve time, so a deploy that
// changes any static asset produces a byte-different worker script. The
// browser re-checks a Cache-Control: no-cache script on every navigation,
// installs the new (byte-different) worker, and its existing "activate"
// handler deletes any cache whose name no longer matches — DEC-039/TASK-059.
func (s *Server) handleServiceWorker(w http.ResponseWriter, r *http.Request) {
	body, err := staticAssets.ReadFile("static/service-worker.js")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	rendered := bytes.ReplaceAll(body, []byte("%BUNDLE_HASH%"), []byte(bundleHash))
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Service-Worker-Allowed", "/meets/")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(rendered)
}
