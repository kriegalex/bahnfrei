// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// Public find-your-athlete filter island (SYS-153/UC-042, TASK-048): every
// [data-public-filter] root (the public results fragment and the public
// start-lists page — public.templ) already renders its own search form
// (searchForm, reused from the DEC-021/TASK-038 operator convention),
// "n results" count and every filterable row/section, server-side and
// unfiltered. This island turns that into an instant, zero-round-trip
// filter for anyone with JS: it hides non-matching [data-filter-row]
// elements and their enclosing [data-filter-section] as the visitor
// types, updates the count, and intercepts the form's submit so a plain
// Enter never fires a page reload — the server-side ?q= GET path
// (public.go's filterPublicResultsView/filterPublicStartListsView) stays
// the no-JS fallback only (SYS-153: "functional without client-side
// scripting"). Compiled to /static/public-filter.js; the CSP forbids
// inline scripts.
(function () {
  "use strict";

  interface FilterState {
    root: HTMLElement;
    input: HTMLInputElement;
    countEl: HTMLElement | null;
    countTemplate: string;
    query: string;
  }

  function normalize(s: string): string {
    return s.trim().toLowerCase();
  }

  function rowMatches(row: HTMLElement, q: string): boolean {
    if (q === "") {
      return true;
    }
    const name = (row.dataset.filterName || "").toLowerCase();
    const bib = (row.dataset.filterBib || "").toLowerCase();
    const club = (row.dataset.filterClub || "").toLowerCase();
    return name.indexOf(q) !== -1 || bib.indexOf(q) !== -1 || club.indexOf(q) !== -1;
  }

  /** Hides non-matching rows, hides any [data-filter-section] left with no
   *  visible row (but only sections that HAD rows to begin with — an
   *  already-empty section, e.g. the "no participants yet" message, has no
   *  [data-filter-row] at all and must never be touched here), and
   *  refreshes the "n results" count. */
  function applyFilter(state: FilterState): void {
    const q = normalize(state.query);
    let visible = 0;
    const rows = state.root.querySelectorAll<HTMLElement>("[data-filter-row]");
    rows.forEach(function (row) {
      const match = rowMatches(row, q);
      row.hidden = !match;
      if (match) {
        visible++;
      }
    });
    const sections = state.root.querySelectorAll<HTMLElement>("[data-filter-section]");
    sections.forEach(function (section) {
      const allRows = section.querySelectorAll<HTMLElement>("[data-filter-row]");
      if (allRows.length === 0) {
        return; // nothing to filter in this section — leave it exactly as rendered
      }
      const hasVisible = section.querySelector<HTMLElement>("[data-filter-row]:not([hidden])");
      section.hidden = !hasVisible;
    });
    if (state.countEl) {
      state.countEl.textContent = state.countTemplate.replace("{n}", String(visible));
    }
  }

  /** Wires one [data-public-filter] root: finds its search input and count
   *  element, applies the current filter immediately (covers both a fresh
   *  page load and a DOM swap where a query is already known), and binds
   *  live filtering + submit interception. initialQuery, when given,
   *  overrides the input's own (freshly server-rendered, always empty)
   *  value — used when re-binding after a live-refresh swap (UC-042 #3). */
  function bind(root: HTMLElement, initialQuery: string | null): FilterState | null {
    const input = root.querySelector<HTMLInputElement>('input[name="q"]');
    if (!input) {
      return null;
    }
    const countEl = root.querySelector<HTMLElement>("[data-filter-count]");
    const countTemplate = countEl ? countEl.dataset.filterCountTemplate || "" : "";
    const state: FilterState = {
      root: root,
      input: input,
      countEl: countEl,
      countTemplate: countTemplate,
      query: initialQuery !== null ? initialQuery : input.value,
    };
    input.value = state.query;
    applyFilter(state);

    input.addEventListener("input", function () {
      state.query = input.value;
      applyFilter(state);
    });

    const form = input.closest("form");
    if (form) {
      form.addEventListener("submit", function (ev) {
        // Filtering already happened live as the visitor typed (the
        // "input" listener above), and the render cache is keyed on
        // meet+locale only, never on q (ADR-004 §9) — so a JS-enabled
        // Enter/submit needs no round trip at all. Only reached when this
        // script loaded, so the no-JS ?q= GET fallback is untouched.
        ev.preventDefault();
        state.query = input.value;
        applyFilter(state);
      });
    }
    return state;
  }

  document.querySelectorAll<HTMLElement>("[data-public-filter]").forEach(function (root) {
    let state = bind(root, null);

    // Only the public results page has a live-refresh section (public
    // results' #public-results, swapped wholesale by public-live.js on
    // every SSE "results" event) — start lists has none. When present, its
    // innerHTML replacement also replaces this filter root and its input,
    // silently resetting any typed query; watch for that swap and restore
    // the query + re-apply the filter against the fresh rows (UC-042 #3).
    // One observer per live section, set up once: rebinding on each swap
    // only re-reads state, it never creates another observer.
    const liveSection = root.closest<HTMLElement>("[data-sse-refresh]");
    if (!liveSection) {
      return;
    }
    const observer = new MutationObserver(function () {
      const freshRoot = liveSection.querySelector<HTMLElement>("[data-public-filter]");
      if (!freshRoot) {
        return;
      }
      state = bind(freshRoot, state ? state.query : "");
    });
    observer.observe(liveSection, { childList: true });
  });
})();
