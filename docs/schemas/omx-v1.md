# omx/v1 — Open Meet eXchange, version 1

**Traces:** SYS-073, SYS-144 · ADR-005 · UC-027 · TASK-025

## What this is

`omx/v1` is this project's open, documented, versioned interchange format for a full meet
(ADR-005 §2): everything needed to reconstruct a meet's **official results** — meet, sessions,
clubs, athletes, relay teams, events, rounds, units, entries, unit assignments, and settled
results (statuses, wind, marks, points, placing, record flags, unit-level announcement
timestamps). It is the machine-readable half of SYS-073's "CSV and a self-describing structured
format"; the flat results CSV (`exchange.EncodeOMXResultsCSV`) derives from the same
`exchange.Document` shape.

## Where it lives

- **Canonical schema document (the published artifact SYS-144 requires):**
  [`internal/exchange/schema/omx-v1.schema.json`](../../internal/exchange/schema/omx-v1.schema.json)
  — a JSON Schema (2020-12 vocabulary subset: `type`, `properties`, `required`, `items`, `enum`,
  `$defs`/`$ref`, `additionalProperties`). It is embedded into the `exchange` package binary via
  `go:embed`, exposed as `exchange.OMXV1Schema()`, and is the exact byte sequence
  `exchange.ValidateOMXSchema` checks an export against — the documentation and the runtime
  contract can never drift apart, because they are the same file.
- **Go implementation:** `internal/exchange/omx.go` (the `Document`/`*Doc` types, `EncodeOMX`,
  `DecodeOMX`, `ValidateOMX` for referential integrity, `EncodeOMXResultsCSV`) and
  `internal/exchange/jsonschema.go` (the minimal, dependency-free JSON Schema subset validator
  `ValidateOMXSchema` runs on).
- **Orchestration (store reads/writes):** `internal/app/omx.go` —
  `ResultsService.ExportOMXDocument`/`ExportOMXResultsCSV` (organizer/office-authorized,
  synchronous — UC-027 #3's "available immediately at T, no post-processing delay") and
  `ImportOMXDocument` (reconstructs a full meet from a `Document` into any store, normally a
  fresh one — UC-027 #2's round-trip scenario).

## Versioning and deprecation policy (SYS-144)

`omx` is versioned like a public API, major-version-in-the-name (`omx/v1`, `omx/v2`, ...):

1. **Within `omx/v1`,** new fields may be added to any object **as optional, additive-only**
   changes — no version bump. Every reader (this codebase's own `DecodeOMX` included) MUST
   ignore unrecognized properties rather than reject the document
   (`TestDecodeOMXToleratesAdditiveUnknownFields`). Existing required fields, their types, and
   their meaning never change within v1.
2. **Anything else is breaking:** removing/renaming a field, changing a field's type, or
   promoting an optional field to required. A breaking change ships as `omx/v2` with its own
   schema document (`schema/omx-v2.schema.json`, once it exists) and its own `SchemaVersion`
   constant; it is a **new, additive format**, not an in-place edit of `omx/v1`.
3. **A `v1` reader refuses a `v2` (or any non-`v1`) document outright** — `DecodeOMX` checks
   `schemaVersion` and returns an error rather than guessing
   (`TestDecodeOMXRejectsWrongSchemaVersion`). There is no silent best-effort cross-version
   parsing.
4. **Deprecation window:** once `omx/v2` ships, `omx/v1` export/import remains supported for at
   least one full MVP release cycle so integrations have a documented migration window, per the
   engagement's general "documented deprecation policy" requirement (SYS-144). Deprecation itself
   (the day a version's support window formally starts) is announced in this file's changelog
   section below, not silently dropped from a release.
5. **CI schema-diff check:** not yet implemented (residual gap — see
   `docs/requirements/open-questions-and-assumptions.md`, this task's OQ register). Today the
   contract is enforced by `TestOMXV1SchemaIsPublishedAndSelfValidating` (the schema is valid
   JSON, self-consistent, and a conforming document validates against the exact embedded bytes)
   plus `TestDecodeOMXRejectsWrongSchemaVersion`/`TestDecodeOMXToleratesAdditiveUnknownFields`
   proving the runtime behaviour the policy above describes; an automated PR-time diff gate that
   flags a schema-file edit as breaking/non-breaking is future work.

## Scope notes (see `open-questions-and-assumptions.md` for the full entries)

- **Individual results only (OQ-057).** `results[].athleteId` is always an individual athlete;
  relay-team race results have no capture path anywhere in this system yet (the same gap
  TASK-020's FinishLynx export names as OQ-049). Relay **rosters** (relay teams, their entries)
  do round-trip. Adding relay results later is additive: an optional `relayTeamId` alongside
  `athleteId` on a result row, no v2 required.
- **`recordFlags` ships empty (OQ-058).** The field exists in `omx/v1` from the start so
  TASK-022 (records & mixed-category fields) landing record/best-flag computation does not
  require a schema version bump — only its Go-side generator/producer changes.
- **No correction/audit history (OQ-056).** `omx/v1` carries only the current **settled** state
  of each result ("official results", UC-027 #1) — who corrected what and when stays in the
  SYS-046 append-only audit log, not this interchange format.

## Changelog

- **2026-07-13 (TASK-025):** `omx/v1` introduced.
