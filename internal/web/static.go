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
// base.css is project-authored.
//
//go:embed static/htmx.min.js static/htmx-LICENSE static/base.css
var staticAssets embed.FS

// staticHandler serves the embedded static assets under /static/.
func staticHandler() http.Handler {
	sub, err := fs.Sub(staticAssets, "static")
	if err != nil {
		panic(err) // embed layout is fixed at compile time
	}
	return http.StripPrefix("/static/", http.FileServerFS(sub))
}
