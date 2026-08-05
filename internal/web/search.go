// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"net/http"
	"strings"
)

// searchQuery reads and trims the "q" query-param DEC-021 (OQ-067,
// TASK-038) names for the operator roster/entries search: a plain GET
// query-string filter, so the search box works as a plain form submit (no
// JS required, progressive-enhancement-friendly) and its state is
// bookmarkable/shareable like every other list view in this app. Shared by
// the roster and bib-assignment pages (standings.go/entries.go) — both are
// name/bib/club participant lists filtered through the same
// Participants()/app.MatchesParticipantSearch pipeline.
func searchQuery(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("q"))
}
