# Usability audit checklist (SYS-117, UC-038 #3–#4)

**Status:** TASK-032 deliverable. Same verification mechanism as the accessibility manual audit
(`accessibility-manual-audit-checklist.md`, SYS-112 UC-026 #2–#3): a per-release, dated, in-repo
run log — Inspection/Demonstration, not a CI gate, because heuristic judgment calls (§1) and
cross-form consistency (§2) are not mechanically checkable. Run it **once per release** (UC-038
#3: "zero open critical findings" at release) against a meet seeded with realistic content —
meet setup, at least one online entry, and one result correction — so every representative form
in §2 is exercised. Record the run's date, auditor, and outcome per row below (append a dated run
log at the bottom — do not overwrite prior runs). The next scheduled run is release 0.1
(TASK-028); see `docs/delivery/usability-audit-2026-07.md` for this task's own first run.

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

## 3. Running the checklist

1. Seed a meet through the real product forms (setup → meet → events → online entry → check-in
   → capture → one correction), the same fixture shape `e2e/helpers/seed.ts` and the Go web tests
   already build, so every representative form in §2 has real content and a real error case to
   trigger (e.g. submit a meet-edit form with an invalid date, or a bib already in use).
2. Walk §1 once against the whole surface (not per-form — these are cross-cutting judgment calls).
3. Walk §2 once per representative form named above.
4. Record every finding with its row ID (H# or F#), the exact page/form, and severity. A Critical
   finding blocks the release per UC-038 #3 until fixed or explicitly waived by the founder in
   `open-questions-and-assumptions.md`.
5. Append the run to the log below — do not overwrite a prior run.

## Run log

| Date | Auditor | Meet fixture | Findings | Outcome |
|------|---------|--------------|----------|---------|
| 2026-07-13 | TASK-032 (Sonnet, design-system audit) | UBS Kids Cup seed (`e2e/helpers/seed.ts` fixture shape, exercised via the existing Go web-handler test fixtures) | See `docs/delivery/usability-audit-2026-07.md` | Zero Critical findings open at merge (two Major findings recorded as OQ-074/OQ-075, in-scope-fixable items fixed in this same task — see the linked report) |
