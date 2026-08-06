// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"embed"
	"io/fs"
	"net/http"
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
// favicon.ico is a neutral, brand-free placeholder icon (F11/TASK-050 — the
// prior absence 404'd on every page load); OQ-061 (organizer branding) is
// still open, so this placeholder is expected to be replaced once that
// question is ratified.
//
//go:embed static/htmx.min.js static/htmx-LICENSE static/tokens.css static/base.css static/capture.js static/public-live.js static/public-filter.js static/capture-offline.js static/office-banner.js static/service-worker.js static/help.js static/favicon.ico
var staticAssets embed.FS

// staticHandler serves the embedded static assets under /static/.
func staticHandler() http.Handler {
	sub, err := fs.Sub(staticAssets, "static")
	if err != nil {
		panic(err) // embed layout is fixed at compile time
	}
	return http.StripPrefix("/static/", http.FileServerFS(sub))
}

// handleFavicon serves the embedded placeholder favicon at the conventional
// root path (F11/TASK-050): browsers request GET /favicon.ico regardless of
// the <link rel="icon"> in layout.templ, so /static/favicon.ico alone still
// left that bare-path request 404ing.
func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	body, err := staticAssets.ReadFile("static/favicon.ico")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/vnd.microsoft.icon")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(body)
}

// handleServiceWorker serves the capture-surface service worker (UC-034 #3)
// from a root-path URL (/capture-sw.js). A service worker may only claim a
// scope at or below its own script path, so the worker that must control
// /meets/{id}/capture/… is served here rather than under /static/. The
// Service-Worker-Allowed header explicitly permits the /meets/ scope.
func (s *Server) handleServiceWorker(w http.ResponseWriter, r *http.Request) {
	body, err := staticAssets.ReadFile("static/service-worker.js")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Service-Worker-Allowed", "/meets/")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(body)
}
