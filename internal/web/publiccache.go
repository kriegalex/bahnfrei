// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package web

import (
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/kriegalex/bahnfrei/internal/web/i18n"
)

// --- ADR-004 read-path amendment (TASK-035/DEC-015, OQ-066): the public
// results page's per-viewer cost was dominated by ResultsService.Standings
// recomputing the whole meet — an athletes×disciplines performance matrix
// plus the rendered view model — uncached, on every request (~64 MiB of
// in-flight heap per concurrent viewer at the SYS-120 reference scale,
// measured in docs/delivery/perf-and-recovery-task-027.md). This cache
// turns that into one build (query + template render) per result change
// instead of one per viewer: both buildPublicResultsView's output and its
// already-rendered results-fragment HTML are reused across every
// concurrent viewer of the same meet+locale until the next
// results-changed event (or the TTL below) invalidates them. Caching the
// rendered bytes (not just the data) matters at the 2,000-viewer scale:
// profiling showed that even with the view model cached, re-walking a
// reference-scale meet's rows into HTML on every one of 2,000 concurrent
// requests still missed the 3s p95 budget — see
// docs/delivery/perf-and-recovery-task-027.md. ---

// publicResultsCacheTTL bounds how long a cached entry may serve without
// an explicit invalidation. The primary, sub-second invalidation path is
// event-driven: ResultsService.OnResultsChanged (server.go) invalidates
// the affected meet the moment a capture write or consent change commits
// — the same hook that already fires the SSE "results" event, so a cache
// entry is never staler than the live-refresh island itself for those
// writes. The TTL is defense-in-depth for the handful of public-surface-
// affecting writes not (yet) wired to that hook — meet-metadata edits
// (MeetService.UpdateMeet) and PrivacyService erasure/retention-purge
// (SYS-101/SYS-102) — see OQ-094. Kept well inside SYS-071's 10s
// live-update budget.
const publicResultsCacheTTL = 5 * time.Second

// publicResultsCacheKey identifies one cached render. Results are
// rendered with locale-specific labels baked in (SYS-074: discipline
// names, status line, unofficial-results badge), so the cache is keyed
// per (meet, locale) rather than per meet alone — a French and a German
// viewer of the same meet never share an entry.
type publicResultsCacheKey struct {
	meetID string
	locale i18n.Locale
}

// publicResultsCacheEntry is one cached, already-localized view of a
// meet's public results, plus its pre-rendered results-fragment HTML
// (exactly what publicResultsFragment(p, view) produces — see
// cachedPublicResults in public.go). fragmentHTML holds no
// session-specific content: publicResultsFragment never reads CSRF token,
// username or login state, only p.T(...) (locale-scoped) and the cached
// view — so reusing it across every viewer of the same (meet, locale) is
// safe. It is a string, not []byte, deliberately: a Go string is
// immutable, so handing the SAME string value to every concurrent
// request's templ.Raw/io.WriteString call is a cheap header copy (~16
// bytes) that shares the one underlying backing array — converting to/
// from []byte per request would copy the whole rendered page on every
// viewer, silently reintroducing the O(viewers) allocation this cache
// exists to remove (measured: reintroducing that copy alone took peak
// heap from hundreds of MiB back to several GiB at 500 viewers). The page
// shell around it (nav, login state, CSRF token) still renders fresh per
// request from the caller's own session — see
// handlePublicResults/handlePublicResultsLive in public.go — so a cached
// entry can never leak one viewer's session chrome into another's
// response.
type publicResultsCacheEntry struct {
	view         publicResultsView
	fragmentHTML string
	builtAt      time.Time
}

// publicResultsCache is the per-meet render cache. A singleflight.Group
// collapses concurrent misses for the same key onto one build — the
// SYS-122 thundering-herd case, where thousands of viewers can arrive
// against a cold or just-invalidated cache within the same instant —
// rather than each paying the full query/compute/render cost
// independently.
type publicResultsCache struct {
	mu      sync.RWMutex
	entries map[publicResultsCacheKey]publicResultsCacheEntry
	group   singleflight.Group
}

func newPublicResultsCache() *publicResultsCache {
	return &publicResultsCache{entries: make(map[publicResultsCacheKey]publicResultsCacheEntry)}
}

// invalidate drops every cached locale variant for meetID. Wired to
// ResultsService.OnResultsChanged in server.go.
func (c *publicResultsCache) invalidate(meetID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.entries {
		if k.meetID == meetID {
			delete(c.entries, k)
		}
	}
}

// getOrBuild returns the cached entry for key if present and fresh,
// building it via build otherwise. Concurrent callers for the same key
// share one build.
func (c *publicResultsCache) getOrBuild(key publicResultsCacheKey, build func() (publicResultsCacheEntry, error)) (publicResultsCacheEntry, error) {
	if e, ok := c.lookup(key); ok {
		return e, nil
	}
	sfKey := key.meetID + "|" + string(key.locale)
	v, err, _ := c.group.Do(sfKey, func() (any, error) {
		// Re-check: another goroutine may have populated the entry between
		// our lookup above and winning the singleflight race.
		if e, ok := c.lookup(key); ok {
			return e, nil
		}
		e, err := build()
		if err != nil {
			return nil, err
		}
		e.builtAt = time.Now()
		c.mu.Lock()
		c.entries[key] = e
		c.mu.Unlock()
		return e, nil
	})
	if err != nil {
		return publicResultsCacheEntry{}, err
	}
	return v.(publicResultsCacheEntry), nil
}

func (c *publicResultsCache) lookup(key publicResultsCacheKey) (publicResultsCacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[key]
	if !ok || time.Since(e.builtAt) > publicResultsCacheTTL {
		return publicResultsCacheEntry{}, false
	}
	return e, true
}
