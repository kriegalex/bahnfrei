# Accessibility manual audit checklist (SYS-112, UC-026 #2–#3)

**Status:** TASK-024 deliverable. UC-026 #1 (automatable WCAG 2.2 AA rules) is covered by
`e2e/tests/a11y-public-SYS113.spec.ts` (axe-core, runs every CI build). This checklist covers
what automated tooling structurally cannot: a real assistive-technology walk-through (#2) and a
judgment call on live-region behaviour (#3). Run it **once per release** against the public
surfaces (`GET /m/{id}`, `/timetable`, `/startlists`, `/results`) of a meet seeded with at least
one multi-day programme, a published timetable, and a captured result, so every page has real
content. Record the run's date, tester, browser/AT combination, and outcome per step below (append
a dated run log at the bottom of this file — do not overwrite prior runs).

## UC-026 #2: screen-reader walk-through — "follow one athlete through a meet"

Use a real screen reader (NVDA/JAWS on Windows, VoiceOver on macOS/iOS, TalkBack on Android — pick
one and note which) and a real or emulated ≥360px-wide viewport. Every step below must be
**completable using the screen reader alone** (no sighted assistance, no mouse).

| # | Step | Pass criterion |
|---|------|-----------------|
| 1 | Land on the meet overview `GET /m/{id}` (e.g. from a shared link). | The page title, meet name, venue, dates and status are announced without extra navigation; the `lang` attribute matches the announced language (SYS-110). |
| 2 | Use landmark/heading navigation to jump straight to the primary content, skipping the header nav. | The skip link (or landmark navigation) reaches `#main` in one step; the header links (`nav`, labelled "Hauptnavigation"/"Navigation principale") are separately reachable but not forced first. |
| 3 | From the overview, follow the link to the public timetable. | The link text alone (no surrounding context) identifies its destination ("Zeitplan"/"Horaire" or the timetable link label); the timetable table's column headers (`th scope="col"`) are announced when navigating into a data cell, so a cell's meaning ("14:30", discipline, category) is unambiguous without seeing the row visually. |
| 4 | Locate one specific athlete's event on the public start list. | The start-list table is reachable, its headers announce per cell (bib/name/birth year/club), and the wide-table `.table-scroll` wrapper (`tabindex="0"`) is itself reachable and operable by keyboard if the table is wider than the viewport. |
| 5 | Navigate to the public results page and locate that athlete's row in their division's standings table. | The division heading (`h2`) announces before its table; per-cell headers announce discipline/rank/total; a still-empty discipline cell ("–") is announced as a real (if unhelpful) value, not skipped/silent. |
| 6 | Switch the page language (DE ↔ FR) via the locale-switcher form and repeat step 5. | The language switch is operable via keyboard alone (`Tab` to the `<select>`, arrow keys to change, then either the native change-triggered submit or explicitly activating the "Anwenden"/"Appliquer" button); the resulting page's `lang` attribute and announced content both reflect the new language. |

## UC-026 #3: live-updating results — no focus-stealing, polite announcement

| # | Step | Pass criterion |
|---|------|-----------------|
| 7 | On the public results page, trigger a new result save from a second (operator) session while the screen reader is parked mid-page on the results table. | The screen-reader focus is **not** moved when the `#public-results` section refreshes (it lives inside `<main aria-live="polite">`); the update is announced politely (queued after the reader finishes its current utterance), not interrupting speech mid-sentence. |
| 8 | Repeat step 7 while actively reading unrelated content elsewhere on the page (e.g. the nav). | No unexpected/duplicate announcement of unrelated, unchanged page regions — only the swapped `#public-results` content is announced. |

## Notes for the runner

- Steps 1–6 are also exercised functionally (without a screen reader) by
  `internal/web` `TestPublicPagesAnonymousAccessSYS070UC017_2` and the `a11y-public-SYS113.spec.ts`
  axe scan — this checklist adds the assistive-technology-specific judgment calls those cannot
  make (announcement order, focus behaviour, whether a `th scope="col"` actually reads naturally).
- If a step fails, file it as a normal defect referencing this checklist's step number and the
  `SYS-112`/`UC-026` IDs; do not silently adjust the checklist to match a known failure.

## Run log

| Date | Tester | AT / browser | Outcome |
|------|--------|--------------|---------|
| — | — | — | Not yet run — first run due before the next tagged release. |
