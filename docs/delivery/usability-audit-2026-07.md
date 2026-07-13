# Usability audit — first run (2026-07-13, TASK-032)

**Checklist:** `docs/requirements/usability-audit-checklist.md` (SYS-117, UC-038 #3–#4).
**Method:** live inspection against the real server (`go build ./cmd/bahnfrei`, `serve
--tls-mode off`), driven with cookie-jar HTTP requests through the real product forms — the same
recipe `e2e/helpers/seed.ts`/the `verify` skill use — plus a source read of every `internal/web/
*.templ` template for cross-form consistency (H4) and the design-system inventory
(`docs/architecture/design-system.md` §2). This is **not** the release-0.1 audit UC-038 #3 names
(that is scheduled for TASK-028); it is this task's required "run it once against the current
app" baseline, establishing the checklist against real content and surfacing what release 0.1
must close.

**Fixture used:** first-run setup → login (`admin`/`s3cret-passphrase`) → the meet-creation form
(`/meets/new`), including a deliberately invalid submission (`end_date` before `start_date`) to
exercise the validation-error path. A full UBS Kids Cup seed (template meet → roster → publish →
capture) was not re-driven here since `e2e/helpers/seed.ts` and the Go web-handler tests already
exercise that path exhaustively for functional correctness — this audit's purpose is the
*interaction-convention* read, which the meet-setup form already exposes fully (labels, hints,
validation, destructive actions elsewhere in the same templates).

**Correction to the initial live read (F1):** the meet-creation form itself has a visible
`<label>` on every field, but a source read of every `internal/web/*.templ` file (prompted by
building the design-system component inventory) found several **placeholder-only inputs with no
label and no accessible name at all** elsewhere in the app: the relay-entry inline-edit rows
(`edit_leg_first_name_N`/`edit_leg_last_name_N`/`edit_leg_birth_year_N`,
`edit_reserve_first_name_N`/`edit_reserve_last_name_N`/`edit_reserve_birth_year_N` in
`entries.templ`), the timing-conflict resolution form's `action` select and its
`override_status_detail`/`reason`/`escalation` inputs (`timing.templ`), the eligibility-override
`reason` input (`import.templ`), and the unit-schedule inline form's `scheduled_at`/`location`
inputs (`meets.templ`). These are small, mechanical, in-scope fixes (reusing the same translation
key already driving each field's placeholder) — **fixed in this task** by adding `aria-label` to
each, plus a new `<label>` wrapping the previously-unlabeled `action` select (new key
`timing.conflicts.action_label`, DE/FR added). This closes the "no accessible name at all" gap;
it does **not** fully satisfy SYS-117's stricter "permanently visible label" bar for these fields
— they remain dense, compact table-row/inline-form cells where a full visible `<label>` would
reflow the layout, which is a visual-design call this task did not make unilaterally. Noted as a
residual nuance under OQ-076 (§ below), not a new open question.

## Summary

| Findings | Count |
|---|---|
| Critical | 2 (OQ-074, OQ-075) |
| Major | 1 (OQ-076) |
| Minor | 1 (OQ-077) |
| Passed cleanly | H1, H2, H4, H7, H8, H10, F4 |
| Fixed during this audit (not counted above) | F1 — 13 placeholder-only inputs across 4 templates given a real accessible name |

**Zero Critical findings were fixable within TASK-032's own scope** (token consolidation, CI
check, gallery, checklist) — both are cross-cutting handler/interaction changes (a confirmation
mechanism; per-field error attribution), not design-token issues. Both are recorded as open
questions (OQ-074, OQ-075) with an explicit note that **they must close before the release-0.1
usability-audit re-run** (TASK-028) can show UC-038 #3's "zero open critical findings." This run
does not itself certify a release; it establishes the checklist and its baseline.

## §1 — NN/g heuristics

| # | Heuristic | Result | Notes |
|---|-----------|--------|-------|
| H1 | Visibility of system status | Pass | Every mutating action is POST-redirect-GET or a direct download; the offline-capture badge/office banner (SYS-087) reflect real connectivity state (already covered by `internal/web` capture/offline tests). |
| H2 | Match between system and the real world | Pass | Operator-facing labels (`Startnummer`, `Wettkampftag`, `Homologations-Referenz`, `Meeting-Typ`) match `glossary.md` vocabulary; no internal identifier leaked into rendered text observed. |
| H3 | User control and freedom | **Critical — OQ-074** | Erase/disable/archive/purge are single-click, no confirmation step. See OQ-074 for detail and why it is out of this task's scope. |
| H4 | Consistency and standards | Pass | Submit buttons consistently say "Speichern"/"Enregistrer" for save actions; the design-system inventory (§2 of `design-system.md`) is the single source every form visibly matches — no divergent button/label pattern found across the templates read. |
| H5 | Error prevention | Pass (partial) | Numeric/date fields already use `type=number`/`type=date` with `min`/`max` where bounded (e.g. `birth_year` `min="1900"`); no free-text field observed standing in for a constrained one. |
| H6 | Recognition rather than recall | **Major — OQ-076** | No per-field hint text exists on the forms inspected; see OQ-076. |
| H7 | Flexibility and efficiency of use | Pass (own gate) | Bulk operations exist (bulk entry, bulk bib assignment); keyboard-only operation is TASK-030's own scope, not re-litigated here. |
| H8 | Aesthetic and minimalist design | Pass | Forms show only the fields relevant to the current step; no unused/rarely-needed field found cluttering a default view. |
| H9 | Help users recognize/diagnose/recover from errors | **Critical — OQ-075** | The rendered error text is plain language ("Die Eingabe konnte nicht gespeichert werden" / could not be saved) but names no field and no fix. Input is preserved (passes the "don't lose data" half). See OQ-075. |
| H10 | Help and documentation | Pass (own gate) | Contextual help is TASK-031/SYS-115's own deliverable, arriving immediately after this task; nothing here blocks it — this task's tokens are ready for it to consume (design-system.md §5). |

## §2 — SYS-117 form conventions

| # | Convention | Result | Notes |
|---|------------|--------|-------|
| F1 | Permanently visible label | **Fixed in this task (13 fields)** | Every input on the meet-creation form itself had a `<label>`. A full `placeholder=` sweep across `internal/web/*.templ` found 13 fields with a placeholder and no label/aria-label anywhere (`entries.templ` relay-edit rows ×6, `timing.templ` conflict form ×4, `import.templ` ×1, `meets.templ` ×2 — see the correction note above). Fixed via `aria-label`/a new `<label>`; the residual "fully visible label" bar for these dense inline fields is left as a design decision, folded into OQ-076. |
| F2 | Visible constraint hint | **Major — OQ-076** | Zero hint text on the meet-creation form (no date-format hint, no explanation of what `homologation_ref`/`results_positioning` mean); the pre-existing `.hint`-classed text elsewhere in the app is page-level guidance, not field-level. |
| F3 | Inline validation error | **Critical — OQ-075** | Confirmed live: a page-level `role="alert"` banner, not an inline/`aria-invalid` field marker. |
| F4 | Input preserved on error | Pass | Confirmed live: the same 422 response re-rendered `name`, `venue`, both dates, tier and scheme exactly as submitted — nothing was lost. |
| F5 | Destructive-action confirmation | **Critical — OQ-074** | Same finding as H3. |
| F6 | Long-running-operation feedback | **Minor — OQ-077** | No explicit progress affordance anywhere; low risk at current scale, see OQ-077. |

## What this task fixed vs. left open

**Fixed in this task** (design-system layer, in scope):
- Consolidated every existing raw color/spacing/radius literal into named tokens
  (`internal/web/static/tokens.css`), refactored `base.css` to consume them.
- Added the `.hint`/`.field-error`/`aria-invalid` CSS conventions and hover/active/disabled
  states that did not exist before (base.css previously only styled `:focus`, not hover/active/
  disabled, and had no error-field convention at all) — these are now documented and demonstrated
  on the gallery page, ready for forms to adopt.
- CI style-conformance gate (`scripts/check-style-tokens.sh`) so no future style bypasses the
  token layer.

**Left open** (OQ-074…OQ-077): the confirmation mechanism, per-field error attribution, and
hint-text content pass are all real product changes across many existing handlers/templates —
outside a design-system-consolidation task's scope, and flagged for prioritization before the
release-0.1 audit re-run.

## Next run

Due at release 0.1 (TASK-028), per `usability-audit-checklist.md`'s own run-log instructions.
Re-run against a full seeded meet (setup → entries → check-in → capture → correction) so every
representative form gets a real error case, and confirm OQ-074/OQ-075/OQ-076 are closed before
declaring zero open Critical findings.
