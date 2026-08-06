# Usability audit — release-0.1 run (2026-08-06, post-M4)

**Checklist:** `docs/requirements/usability-audit-checklist.md` §1–§3 (SYS-117, UC-038 #3–#4;
§3 = SYS-147/148, UC-039). **This is the per-release run UC-038 #3 names** for the 0.1 tag,
executed against `main` after the full M4 volunteer-usability wave (TASK-044…050) merged.

**Method:** live walk against the real server (`serve --tls-mode off`, fresh data dir), driven
through a real browser (Playwright) at 360×740 and 1280×800, plus scripted HTTP for fixture
steps. **Fixture (per §4):** UBS Kids Cup template meet (8 rostered athletes, published,
`vreni` field official assigned, `otto` competition office) **plus** a custom "Springmeeting"
meet created through the real `/meets/new` form (including the deliberate end-before-start
error case) carrying a **high-jump event** so the vertical capture family could be walked —
the UKC template has no vertical discipline. Error cases exercised: invalid meet dates,
invalid capture mark (`abc`), correction without reason. Limitation: a live online-entry
submission could not be walked — the fixture meets' entry windows are closed (no Meldeschluss
configured), so the individual-entry form does not render; its §2 rows remain T-verified
(`TestOnlineEntryIndividualFieldErrorsOQ075UC038_4`, `inline-field-errors-UC038.spec.ts`).

## Summary

| Findings | Count |
|---|---|
| Critical | **0** |
| Major | 0 |
| Minor | 4 (N1–N4 below) |

**Both Criticals from the 2026-08 volunteer walkthrough are verified fixed live** (F1 →
TASK-044, F2 → TASK-045; repro'd with the exact original scenarios). **UC-038 #3's "zero open
critical findings" precondition is satisfied for the 0.1 tag** — subject to founder
ratification of the still-proposed STR-046/SYS-147–153, whose §3 rows this run walks.

## §1 — NN/g heuristics (delta since the 2026-07 run and the 2026-08 walkthrough)

| # | Result | Evidence from this run |
|---|--------|------------------------|
| H1 | Pass | Save shows "Speichert …" **synchronously at the cell**, then "Gespeichert" after ack; the row's Resultat/Pkt. cells fill (3.47 / 413) without reload; the status region never contradicts per-cell state (the enqueue race that could show "alle übertragen" mid-save was fixed in the TASK-045 merge). |
| H3 | Pass | New user control: a rejected capture offers "Verwerfen" at the cell; discarding clears the rejection and the cell. Destructive confirms unchanged (TASK-034/041/047). |
| H9 | Pass | Invalid mark renders a plain-language, localized reason at the cell ("Ungültige Eingabe — bitte Marke oder Zeichen prüfen (X, –, r).") — **not** a connectivity error; a subsequent valid save applies immediately (no queue blocking). Meet-setup end-before-start renders inline at `end_date` with `aria-invalid` and full input preservation. Correction-without-reason renders "Eine Korrektur erfordert einen Grund." inline. |
| H2/H4–H8/H10 | Pass | Unchanged from prior runs; spot-checked. Discipline names now localized on dashboard/capture/hub (TASK-050) — see N1 for the one missed surface. |

## §2 — Form conventions

Meet setup: walked live incl. the error case — F1–F4 pass (visible labels, hints, inline
error at the offending field, input preserved). Result correction: reason-missing error
walked live — F3 pass; happy-path live walk was inconclusive due to this run's scripted
driver guessing form field names, **not** a product failure (flow pinned by
`TestCaptureAnnounceAndCorrectionFlowSYS046SYS047UC015Web` and the TASK-040 web tests).
Online entry: not walkable (see fixture limitation), T-verified. F5/F6 unchanged (TASK-034
confirms; OQ-077 progress-feedback remains open, Minor).

## §3 — Point-of-competition mobile conventions (first per-release run of the new section)

Walked at 360×740 on: field-horizontal capture (UKC ZoneLJ), track capture (60 m), vertical
capture (high jump), check-in.

| # | Result | Measured |
|---|--------|----------|
| M1 | **Pass** all four surfaces | `scrollWidth` 360 = viewport everywhere (was 589/980 px pre-M4). Vertical grid: page never scrolls sideways; the height×trial matrix scrolls in its own region (the documented OQ-131 trade-off). |
| M2 | **Pass** | First capture input at y≈571 px < 640; compact `h1.capture-title` + context line replaced the former full-width heading. |
| M3 | **Pass** (primary) | Every primary capture control ≥44 px (min measured exactly 44). Chrome elements at 21–22 px: header nav links (WCAG 2.5.8 inline exception) and the locale `<select>` (109×21 — conforms via the spacing exception: a 24 px circle centered on it intersects no adjacent target; measured). See N4. |
| M4 | **Pass** | `inputmode="decimal"` + `enterkeyhint="done"` on mark/time inputs; vertical trial input `inputmode="none"`; letter markers enterable via X/–/r (o/x/–/r vertical) quick-action buttons. Track: timing select 44 px; DQ-rule field hidden until status=DQ. |
| M5 | **Pass** | Pending badge appears synchronously on save, confirmed badge on ack (text, not color-only); derived Resultat/Pkt. update in place. |

## New findings this run (all Minor — fix opportunistically)

| # | Finding | Where | Trace |
|---|---------|-------|-------|
| N1 | Check-in page heading renders the discipline in English ("Springmeeting Muttenz — **High Jump** — Check-in") — the TASK-050 localization sweep covered capture index/unit pages and the hub programme but missed the check-in heading. | `internal/web` check-in page title/h1 | SYS-111 defect |
| N2 | Entries page with a closed entry window: the page-level empty state correctly explains "keine Bewerbe offen für Online-Meldungen", but the "Meine Meldungen" empty state below still says "über eines der Formulare **oben** melden" — a next-step instruction pointing at forms that are not rendered. | `internal/web` entries templates | SYS-152 (contradictory empty-state copy) |
| N3 | Public filter count does not pluralize: "1 **Ergebnisse**". | `public.filter.count` i18n key (needs singular/plural forms) | SYS-110 |
| N4 | Locale `<select>` in the page header is 21 px tall — WCAG 2.5.8-conformant via the spacing exception (verified geometrically), but below the checklist's comfort bar; a 24 px min-height token on selects closes it. | `base.css`/`tokens.css` | SYS-116 polish |

## Outcome

**Zero open Critical findings → UC-038 #3 satisfied for release 0.1.** The four Minors above
do not block per the checklist's own severity rules; they are small, well-localized fixes
suitable for one S-sized cleanup slice or opportunistic inclusion in the next task touching
those files.
