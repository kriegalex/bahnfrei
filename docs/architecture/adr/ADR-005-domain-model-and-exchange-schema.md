# ADR-005 — Domain model ownership & open exchange schema

**Status:** **Accepted** — ratified by the founder 2026-07-05 (one-way door: schema identity & versioning policy; incl. §6 external-identifiers amendment)
**Date:** 2026-07-05
**Traces:** SYS-073, SYS-144, SYS-005, SYS-053, CON-01, STR-037, DEC-012, A-012; SyRS §2

## Context

No federation-mandated interchange format exists (C3); the W3C Open Athletics CG draft is a
useful vocabulary but unadopted (DEC-012). Meanwhile everything rule-shaped changes yearly
(categories, scoring tables, youth-series rules — CON-01) and lock-in-free data export is a
core promise (STR-037).

## Decision

1. **The SyRS §2 conceptual model is implemented as the canonical domain model** (Meet,
   Event, Round/Unit, Athlete, Entry, Participation/Result, CategoryScheme, Discipline,
   Record references, Audit event), owned by this project.
2. **One published, versioned exchange schema** — working name `omx` (Open Meet eXchange),
   `omx/v1` — as JSON Schema documents in-repo, covering full-meet export/import (UC-027
   round-trip property). Flat CSV exports derive from the same definitions. Breaking changes
   bump the major version and follow the documented deprecation policy (SYS-144).
3. **Naming aligns with the W3C Open Athletics CG vocabulary where a term exists** (fields,
   discipline codes, unit concepts); divergences are recorded in a mapping table in the schema
   docs. We do not claim conformance and do not block on the CG's evolution (DEC-012); if the
   CG standard matures into federation adoption, an `omx→CG` exporter is an additive feature.
4. **Rule-shaped data is data, not code** (CON-01): category schemes, scoring tables (World
   Athletics formula parameters, UKC points table), progression templates, and youth-series
   competition templates (SYS-053) live as versioned data files shipped with releases,
   loadable/replaceable without code changes (UC-033 #5).
5. **Identifiers:** ULIDs for entities; human-facing stable keys where the domain has them
   (licence numbers, bib numbers scoped per meet); public URLs derive from stable slugs
   (SYS-070).
6. **External identifiers are first-class, typed attributes — never overloaded onto our own
   IDs** *(amended 2026-07-05 after the C2.1 Swiss-chain research)*: Athlete carries a set of
   federation identifiers (Swiss Athletics licence number first — WO 2026 §5.3b makes it the
   only join key into the Bestenliste; WA athlete ID later); Meet carries federation
   competition identifiers (Swiss Athletics Wettkampfverwaltung ID, WA Global Calendar ID —
   TAF3 itself added a WA-competition-ID field in v7010); Club carries the federation club
   code. `omx/v1` serializes them in a namespaced `externalIds` map so new federations are
   additive, not breaking. Exports into federation-facing formats (SYS-077/078) MUST emit
   the federation's own codes (categories, disciplines, licence numbers) — the W3C-aligned
   internal naming never leaks into a file another system has to ingest.

## Alternatives considered

| Option | Why rejected |
|--------|--------------|
| Adopt W3C OA CG draft as *the* schema | It is a draft with no adopters (C3); coupling our compatibility promises to an unstable third-party document risks breaking STR-037's "documented, stable" guarantee |
| Invent nothing, export only CSV | CSV alone cannot carry the full meet structure losslessly (UC-027 #2 round-trip fails); JSON+CSV covers both machine and spreadsheet consumers |
| Model rules in code (enums/constants) | Yearly federation changes would require releases for data changes; violates CON-01 and hurts non-programmer maintainability |

## Consequences

- The schema documentation becomes a public artifact other tools could adopt — the openness
  differentiator from C6 — and the fixture format for our own acceptance tests.
- Maintaining a CG mapping table costs little and keeps the door open to future alignment.
- Data-driven rules mean the rule engines must be written as interpreters of configuration
  (scoring-table evaluator, category resolver) — slightly more upfront design, directly
  verified by the SYS-142 fixture suites.
- **Relationship to the Seltec ↔ Swiss Athletics chain: adjacent, not conflicting.** We never
  write into their pipeline (Alabus ↔ TAF3 ↔ LA.portal); all contact happens at our own
  adapter boundaries — entry import (SYS-013), youth-series template export (SYS-077),
  TAF3-compatible exports (SYS-078) — producing/consuming *their* documented formats with
  *their* codes (Decision §6). Producing format-compatible files for interoperability is
  lawful and does not touch Seltec systems; the proprietary LA.portal protocol stays
  untouched (rejected in ADR-006); official-status labeling is handled by SYS-076.
