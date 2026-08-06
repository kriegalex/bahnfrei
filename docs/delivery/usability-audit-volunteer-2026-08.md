# Usability walkthrough — volunteer personas (2026-08-06)

**Method:** exploratory persona-based walkthrough against the real server (`go build
./cmd/bahnfrei`, `serve --tls-mode off`, fresh data dir), driven through a real browser
(Playwright) — not a source read. Fixture: UBS Kids Cup template meet, 10 rostered athletes,
published timetable and meet; accounts `vreni` (field official, assigned to the 60 m and long
jump units) and `otto` (competition office) created through the real admin forms. Personas and
viewports:

- **Vreni**, once-a-year volunteer at the long-jump station, her own phone (360×740 — the
  SYS-113 floor).
- **Otto**, competition-office volunteer, club laptop (1280×800).
- Organizer (admin) on desktop; a **parent** on a phone reading public results.

This is **not** the per-release SYS-117 checklist run (that walks §1/§2 of
`docs/requirements/usability-audit-checklist.md`); it is a task-flow walkthrough that
exercised surfaces the checklist's form-convention rows do not reach — the offline-capture
island's live behavior, phone-width operator layouts, cross-surface navigation, and empty
states. Where a finding falls under an existing checklist row (H1, H3, H9), it is graded
against that row.

**Authoritative guidance used:** Nielsen Norman Group, *10 Usability Heuristics*
(<https://www.nngroup.com/articles/ten-usability-heuristics/>); WCAG 2.2 — SC 2.5.8 Target
Size (Minimum) (<https://www.w3.org/TR/WCAG22/#target-size-minimum>), SC 1.4.13; ISO
9241-110:2020 interaction principles (self-descriptiveness, error tolerance, suitability for
the task); GOV.UK Design System form and error patterns
(<https://design-system.service.gov.uk/>); Apple HIG (44×44 pt touch targets) and Material
Design (48×48 dp) for the touch-target best-practice tier above WCAG's 24 px floor; WHATWG
`inputmode`/`enterkeyhint` for mobile keyboard hints.

## Summary

| Severity | Count | Findings |
|---|---|---|
| Critical | 2 | F1 (sync rejections misreported as connectivity loss, queue wedged), F2 (capture surface not operable at phone width) |
| Major | 4 | F3 (no visible save state), F4 (no participant identity correction), F5 (navigation dead ends / link soup / missing assignment context), F6 (localization inconsistencies — defect vs SYS-110/111) |
| Minor | 6 | F7–F12 |
| Positive | The TASK-031/032/034 conventions hold everywhere they were applied: visible labels, hints, contextual help, inline field errors with preserved input, confirm sub-pages, honest offline banner for true disconnects, roster search, clean localized standings. |

Per UC-038 #3 (zero open Critical findings at release), **F1 and F2 gate the 0.1 tag** until
fixed (TASK-044, TASK-045) or explicitly waived by the founder.

## Findings

### F1 — Critical: non-retryable sync failures presented as connectivity loss; poisoned op wedges the station

Repro (live, 2026-08-06): on the long-jump capture page, save the mark `abc`. The server
rejects the sync batch with HTTP 400. Observed:

- The UI shows **"Verbindung unterbrochen — Eingaben bleiben erhalten, es wird automatisch
  erneut versucht."** while the status region reads **"Online — alle Erfassungen übertragen ·
  1 ausstehend"** — a false statement (the connection is fine), an internally contradictory
  status ("all transferred" + "1 pending"), and a promise that retrying will help (it never
  will).
- The client retries the 400 forever with backoff; the poisoned op survives page reload
  (by design — the queue is durable) and **blocks every later capture**: saving a valid mark
  over it queues a second op ("2 ausstehend") that never applies. Confirmed: standings never
  received the valid 3.80. The station is wedged until someone deletes the browser's
  IndexedDB (`bahnfrei-capture`) — there is no in-product way to correct or discard a queued
  op.
- Code-level cause (`islands/src/capture-offline.ts` ~L373): **any** non-OK sync response —
  400 validation, 401/403 session expiry (12 h TTL, `internal/web/config.go`), 500 — throws
  into `scheduleRetry()`. The server returns whole-batch 400 rather than per-op rejection
  statuses, and the sync ack vocabulary (`applied`/`duplicate`/`reconciliation`) has no
  rejection case.

Violates: checklist H1 (truthful status — same truthfulness bar SYS-087 sets for
connectivity), H9 (recognize/diagnose/recover from errors), ISO 9241-110 error tolerance.
A session expiring mid-meet produces the same silent wedge behind a fake connectivity
message. Disposition: **SYS-149 / UC-040 / TASK-044**.

### F2 — Critical: the primary capture surface is not operable at phone width

Measured at 360 px viewport (SYS-113's own floor for public pages; field capture runs on
officials' phones — C8, ADR-002): the field-capture table renders **589 px** wide, the track
table **980 px** — the volunteer pans horizontally for every attempt (track: V-columns,
status, save all off-screen). The meet+unit `<h1>` plus header chrome consume most of the
first viewport before the first athlete row. All capture inputs and save buttons are **24 px
tall** — exactly at WCAG 2.2 SC 2.5.8's minimum and far under the 44–48 px HIG/Material
recommendation for a primary control operated outdoors under time pressure. Mark/time/wind
inputs are `type="text"` with no `inputmode`/`enterkeyhint`, so phones raise the full alpha
keyboard to type `3.47`.

No requirement covers this: SYS-113 is explicitly **public-surfaces-only**, and no SYS binds
operator surfaces to any viewport or touch ergonomics. Disposition: **STR-046, SYS-147 /
UC-039 / TASK-045**.

### F3 — Major: a save produces no visible state at the point of action

The capture island sets `data-pending` on the cell form, but no CSS rule targets it — saved
and unsaved cells are visually identical; there is no confirmed state either. The row's
Resultat/Pkt. cells stay empty until a manual reload (the standings fragment below the fold
does update). Under pressure this begets double-taps and doubt ("did that go in?"). Checklist
H1 graded the page-level PRG forms, which pass; the asynchronous island was never visually
audited. Disposition: **SYS-148 / UC-039 #4–#5 / TASK-045**.

### F4 — Major: no way to correct participant identity data

Roster rows have no edit affordance and no route exists: participant mutation is limited to
bib assignment and the out-of-competition flag (`UpdateParticipantBib`,
`UpdateParticipantOutOfCompetition`). A typo'd name, wrong birth year (category!), wrong club
— the most routine day-of data fixes — cannot be corrected in the product at all. This is
also a GDPR/nFADP accuracy-and-rectification concern, not just convenience. Checklist H3
(user control and freedom). Disposition: **SYS-150 / UC-043 / TASK-049**.

### F5 — Major: navigation dead ends, flat link soup, missing assignment context

- Office home links only roster + standings per meet; check-in — the office's core day-of
  task — is reachable only by opening the meet hub and scanning the programme table (and
  capture/reconciliation not at all: OQ-113).
- The meet hub renders ~12 links in one undifferentiated `·`-separated paragraph — no
  grouping by task area or frequency (NN/g menu/IA guidance; ISO 9241-110 suitability for
  the task).
- Vreni's assignment list names the unit ("Zone Long Jump (UKC)") but shows **no scheduled
  time and no location** — the volunteer's first two questions.

Disposition: **SYS-151 / UC-041 #1–#3 / TASK-046** (whose ratification would also answer
OQ-113).

### F6 — Major (defect vs SYS-110/111, no new requirement): localization inconsistencies

- Discipline names render in **English** on the assignments dashboard, capture pages and the
  meet-hub programme/timetable ("60 metres", "Zone Long Jump (UKC)") while standings render
  them localized ("Zonen-Weitsprung (UKC)") — the localized path exists and is simply not
  used everywhere (SYS-111).
- Operator-facing timestamps render as raw UTC ("publiziert am 06.08.2026 05:29 UTC") —
  SYS-110's locale-correct formatting read on time display; Swiss meets run on local time.

Disposition: **defect sweep TASK-050** (regression tests pin each surface).

### F7–F12 — Minor

| # | Finding | Guideline | Disposition |
|---|---------|-----------|-------------|
| F7 | Check-in page for a roster-seeded UKC meet says "Keine Meldungen für dieses Rennen" yet still offers the destructive "Check-in schliessen (… DNS)" action; no explanation why it is empty or what to do (kids are on the capture pages). | NN/g empty-state guidance; H5 error prevention | **SYS-152 / UC-041 #4–#5 / TASK-047**; semantics question **OQ-117** |
| F8 | Empty standings render as full-header tables / seven repeated "Noch keine Resultate erfasst" blocks below every capture page — long, noisy pages on a phone. | H8 minimalist design | **SYS-152 / TASK-047** (fold into empty-state pass) |
| F9 | Public results: no search/filter and no per-category jump navigation — a parent at a 600-child meet scrolls every table (operator-side search shipped in TASK-038; public got nothing). Page does fit 360 px (SYS-113 holds). | Task suitability; NN/g findability | **SYS-153 / UC-042 / TASK-048** |
| F10 | No password reset exists (self-service or admin-performed — only enable/disable/role change); no show-password toggle on login. A volunteer locked out on meet morning needs account recreation. | GOV.UK password-input pattern; NN/g | **OQ-116** (onboarding/recovery model is a founder call) |
| F11 | Every page: htmx CSP console error (blocked inline style injection); `favicon.ico` 404. Cosmetic, but "professional quality" (STR-045) and console hygiene. | — | **TASK-050**; favicon final form awaits OQ-061 branding |
| F12 | No web-app manifest (capture stations can't be installed home-screen full-screen); no dark scheme; operator outdoor-legibility has no requirement (SYS-113 is public-only). Track capture repeats the per-race timing-method select and the rarely-used DQ-rule field on every row. | HIG/Material; progressive disclosure (H8) | **OQ-118**; row-layout part folds into TASK-045's redesign |

## What held up well

Labels, hints and contextual help (TASK-031), inline field errors with preserved input
(TASK-034), destructive-action confirm pages (TASK-034), the honest offline banner for real
disconnects (SYS-087 — verified truthful when the network is actually down), roster search
(TASK-038), keyboard operation (TASK-030), and the localized, legible standings and public
pages. The volunteer-facing *forms* are in good shape; the gaps are concentrated in the
**asynchronous capture experience, phone-width operator layouts, and cross-surface
wayfinding** — exactly the strata the form-convention checklist could not see.

## Dispositions at a glance

| Finding | New requirement | UC | Task | OQ |
|---|---|---|---|---|
| F1 | SYS-149 | UC-040 | TASK-044 | — |
| F2 | STR-046, SYS-147 | UC-039 | TASK-045 | — |
| F3 | SYS-148 | UC-039 | TASK-045 | — |
| F4 | SYS-150 | UC-043 | TASK-049 | — |
| F5 | SYS-151 | UC-041 | TASK-046 | answers OQ-113 |
| F6 | — (defect vs SYS-110/111) | — | TASK-050 | — |
| F7/F8 | SYS-152 | UC-041 | TASK-047 | OQ-117 |
| F9 | SYS-153 | UC-042 | TASK-048 | — |
| F10 | — | — | — | OQ-116 |
| F11 | — | — | TASK-050 | OQ-061 (favicon) |
| F12 | — | — | (partial TASK-045) | OQ-118 |

All new requirements are **proposed, pending founder ratification** (same path as the
2026-07-13 STR-044/045 round, this time sourced from a walkthrough rather than a founder
request).
