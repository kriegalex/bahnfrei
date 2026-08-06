# Usability audit checklist (SYS-117, UC-038 #3–#4)

**Status:** TASK-032 deliverable, §3 (point-of-competition mobile conventions) added by TASK-045
from the 2026-08-06 volunteer walkthrough. Same verification mechanism as the accessibility
manual audit (`accessibility-manual-audit-checklist.md`, SYS-112 UC-026 #2–#3): a per-release,
dated, in-repo run log — Inspection/Demonstration, not a CI gate, because heuristic judgment
calls (§1), cross-form consistency (§2) and point-of-competition mobile ergonomics (§3) are not
fully mechanically checkable (though §3's target-size/scroll/`inputmode` rows are also covered
by an automated Playwright suite, `e2e/tests/mobile-capture-UC039.spec.ts`, which narrows but
does not replace the manual walk). Run it **once per release** (UC-038 #3: "zero open critical
findings" at release) against a meet seeded with realistic content — meet setup, at least one
online entry, one result correction, and a capture session for each discipline family — so every
representative form in §2 and every operator surface in §3 is exercised. Record the run's date,
auditor, and outcome per row below (append a dated run log at the bottom — do not overwrite
prior runs). The next scheduled run is release 0.1 (TASK-028); see
`docs/delivery/usability-audit-2026-07.md` for this task's own first run.

Severity: **Critical** (blocks release per UC-038 #3), **Major** (file as a defect/open question,
does not block), **Minor** (note, fix opportunistically).

## 1. Nielsen Norman Group's 10 usability heuristics

Source: Jakob Nielsen, *10 Usability Heuristics for User Interface Design*, Nielsen Norman Group
(1994, updated 2024-01-30). <https://www.nngroup.com/articles/ten-usability-heuristics/>

| # | Heuristic | What to check on Bahnfrei's operator + public surfaces | Severity if failed |
|---|-----------|----------------------------------------------------------|---------------------|
| H1 | Visibility of system status | Every save/submit gives feedback within ~1s (spinner, redirect, or inline confirmation); the offline-capture badge and office blip-tolerance banner (SYS-087) always reflect the real connectivity state, never stale. | Major |
| H2 | Match between system and the real world | Terminology matches the domain glossary (`glossary.md`) and the operator's own vocabulary (heat, round, DNS, DQ, bib) — no internal jargon (e.g. entity/table names) leaking into UI text. | Minor |
| H3 | User control and freedom | Destructive/irreversible actions (erase athlete data, retention purge, account disable) require explicit confirmation (SYS-117); there is a clear way back from a multi-step form (e.g. bulk entry) without losing already-entered rows. | Critical if a destructive action has no confirmation |
| H4 | Consistency and standards | The same action (submit, cancel, confirm) uses the same wording/placement across forms; the design-system inventory (`docs/architecture/design-system.md` §2) is the single source every form should visibly match. | Major |
| H5 | Error prevention | Required fields are marked; numeric fields constrain input (`min`/`max`/`type=number`) rather than accepting free text then rejecting it; a destructive action shows what will happen before it happens. | Major |
| H6 | Recognition rather than recall | A field's format/unit/bound constraint is visible as hint text at the point of entry (SYS-117), not only in a help page or after a failed submit; a multi-step flow (e.g. relay entry) does not require remembering values entered on a prior screen. | Major |
| H7 | Flexibility and efficiency of use | Keyboard-only operation is possible for expert flows (SYS-114, TASK-030's own scope — cross-reference, do not re-litigate here); bulk operations exist where a single-row workflow would be repetitive (bulk entry, bulk bib assignment). | Minor (SYS-114 itself is TASK-030's gate) |
| H8 | Aesthetic and minimalist design | Pages show only what the current task needs; tables/forms are not cluttered with rarely-used columns/fields by default. | Minor |
| H9 | Help users recognize, diagnose, and recover from errors | A validation error names the field and states what to fix, in plain language, not an error code; the input is not cleared. | Critical if input is lost or the field/fix is not identifiable |
| H10 | Help and documentation | Where a control's purpose is not self-evident, contextual help exists at the point of need (TASK-031, SYS-115) rather than only in an external document. | Minor (SYS-115/TASK-031's own gate) |

## 2. SYS-117 form conventions (own requirement text + GOV.UK Design System precedent)

Source: SYS-117 (`system-requirements.md`); GOV.UK Design System, *Text input* component —
"Hint text" guidance. <https://design-system.service.gov.uk/components/text-input/>

Walk every representative form named below and check each row.

**Representative forms:** meet setup (`/meets/new`, `/meets/{id}/edit`), online individual/bulk/
relay entry (`/meets/{id}/entries`), result correction (capture unit page, correction fields).

| # | Convention | Pass criterion | Severity if failed |
|---|------------|-----------------|---------------------|
| F1 | Permanently visible label | Every input has a `<label>` (or equivalent) that stays visible while the field has a value — never a placeholder standing in as the only label. | Critical |
| F2 | Visible constraint hint | A field with a real constraint (date format, numeric bounds, required units) shows that constraint as on-screen hint text (`.hint`, §3 design-system inventory), one short sentence, not only in a tooltip/help popup. | Major |
| F3 | Inline validation error | A submitted validation error renders at the field it concerns (not only as a page-level banner), states what to fix, and the field is marked (`aria-invalid`, per the `.field-error` convention design-system.md documents). | Critical |
| F4 | Input preserved on error | A failed submission re-renders the form with every previously entered value intact — nothing the user typed is silently lost. | Critical |
| F5 | Destructive-action confirmation | Erase/purge/disable-account/archive-meet actions require an explicit confirmation step, not a single click. | Critical |
| F6 | Long-running-operation feedback | An operation that can take more than ~1s (import, export, backup) shows visible progress or an explicit "in progress" state, not a silently blocked UI. | Major |

## 3. Point-of-competition mobile conventions (SYS-147/148, UC-039)

Source: WCAG 2.2 SC 2.5.8 *Target Size (Minimum)*
(<https://www.w3.org/TR/WCAG22/#target-size-minimum>) — the 24x24 CSS px floor every
interactive element must clear; Apple Human Interface Guidelines, *Layout* — 44x44 pt minimum
tappable area (<https://developer.apple.com/design/human-interface-guidelines/layout>) and
Material Design 3, *Accessibility — Touch target size* — 48x48 dp
(<https://m3.material.io/foundations/accessible-design/overview>) as the best-practice tier
this project targets (44px) for a *primary* control above WCAG's bare floor; WHATWG HTML,
`inputmode`/`enterkeyhint` attributes
(<https://html.spec.whatwg.org/multipage/interaction.html#input-modalities>) for
virtual-keyboard hints. Added TASK-045 from the volunteer walkthrough's F2 (Critical: 589/980
px wide capture tables at 360 px, 24 px controls, no `inputmode`) and F3 (Major: no visible
save-state feedback) — walk these rows on **operator surfaces used at the point of
competition** (result capture of every family, check-in, unit standings), not the public
surfaces §1/§2 already cover, at a 360 px viewport.

| # | Convention | Pass criterion | Severity if failed |
|---|------------|-----------------|---------------------|
| M1 | No page-level horizontal scroll for the primary task | `document.documentElement.scrollWidth` does not exceed the viewport width at 360 px while performing the surface's primary action (capturing a mark, confirming a check-in row). An inner scrolling region (e.g. a wide, inherently two-dimensional matrix) is acceptable only if the page itself never scrolls sideways to reach it. | Critical |
| M2 | First actionable row within the first viewport | The first capture/check-in row is visible within the first 640 px of page height at 360 px width, without scrolling past header chrome — a full "MeetName — Discipline" heading plus back-links consuming most of the screen fails this. | Major |
| M3 | Touch-target size | Every primary control (mark/time input, save button, status/timing select) reaches at least 44x44 CSS px; **no** interactive element anywhere on the surface is smaller than WCAG 2.2 SC 2.5.8's 24x24 px floor. | Critical if under 24px; Major if under 44px |
| M4 | Virtual-keyboard hints | Every mark/time/wind input declares an `inputmode` (and, where a single field is the last stop before submit, `enterkeyhint`) appropriate to its format — while any documented non-numeric marker (e.g. D5.2's X/–/r) the field also accepts remains enterable by some means (a quick-action button, a select) without requiring the full alphanumeric keyboard. | Major |
| M5 | Visible save-state feedback | An asynchronous save shows a pending state at the entry itself until acknowledged, then a confirmed (or failed) state, each distinguishable by more than color (text/icon) and appearing within ~500 ms of the state change; any value derived from the save (a computed result, points, rank) displayed on the same page updates automatically, without a manual reload. | Major |

## 4. Running the checklist

1. Seed a meet through the real product forms (setup → meet → events → online entry → check-in
   → capture → one correction), the same fixture shape `e2e/helpers/seed.ts` and the Go web tests
   already build, so every representative form in §2 has real content and a real error case to
   trigger (e.g. submit a meet-edit form with an invalid date, or a bib already in use).
2. Walk §1 once against the whole surface (not per-form — these are cross-cutting judgment calls).
3. Walk §2 once per representative form named above.
4. Walk §3 once at a 360 px viewport against each point-of-competition operator surface (every
   capture family, check-in, unit standings).
5. Record every finding with its row ID (H#, F# or M#), the exact page/form, and severity. A
   Critical finding blocks the release per UC-038 #3 until fixed or explicitly waived by the founder in
   `open-questions-and-assumptions.md`.
6. Append the run to the log below — do not overwrite a prior run.

## Run log

| Date | Auditor | Meet fixture | Findings | Outcome |
|------|---------|--------------|----------|---------|
| 2026-07-13 | TASK-032 (Sonnet, design-system audit) | UBS Kids Cup seed (`e2e/helpers/seed.ts` fixture shape, exercised via the existing Go web-handler test fixtures) | See `docs/delivery/usability-audit-2026-07.md` | Zero Critical findings open at merge (two Major findings recorded as OQ-074/OQ-075, in-scope-fixable items fixed in this same task — see the linked report) |
| 2026-08-06 | Tech lead (exploratory volunteer-persona walkthrough — not the per-release checklist run; graded against H1/H3/H9 where applicable) | Fresh UBS Kids Cup seed driven through a real browser (Playwright), 360 px and 1280 px viewports, roles field official / office / organizer / public | 2 Critical (sync rejections misreported as connectivity loss + queue wedge; capture surface not operable at phone width), 4 Major, 6 Minor — see `docs/delivery/usability-audit-volunteer-2026-08.md`; new requirements STR-046/SYS-147–153 proposed, tasks TASK-044…050 scheduled (M4) | **Criticals open** — per UC-038 #3 they gate the 0.1 tag until TASK-044/TASK-045 land or the founder waives them |
