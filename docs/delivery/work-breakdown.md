# Work Breakdown — Phase B backlog (TASK-###)

**Document status:** Phase B1 baseline, created 2026-07-05 on gate-2 ratification (DEC-014).
Coordinated by the Fable Tech Lead per `CLAUDE.md`; each TASK is assigned to worker agents at
the cheapest sufficient tier and is **done only when its acceptance tests pass and trace to a
`SYS-###`** (typecheck + lint + tests green; traceability matrix updated in the same change).

**Conventions.** Size: S (≈½ day-agent), M (≈1–2), L (≈3+). Tier = default worker model
(escalate only on proven need). IDs are stable; do not renumber. A TASK bundling several UCs
is still merged slice-by-slice (one PR per UC where practical).

## Milestones

| Milestone | Outcome | Demo test |
|---|---|---|
| **M0 — Foundation** | Repo, CI gates, storage, domain core, server shell all green | CI proves SYS-141 gates on a hello-meet |
| **M1 — Kids Cup PoC** | A volunteer runs a complete UBS Kids Cup meet on the founder's hub: template setup → phones capturing offline-tolerantly → live results → printed lists → official series upload file | The founder demos it to the local clubs (DEC-011) |
| **M2 — Club meet complete** | All remaining MVP UCs: online entries, check-in/seeding/progression, track + vertical + combined capture, FinishLynx exchange, records, privacy, i18n/a11y, omx export | A full club evening meet runs end-to-end |
| **M3 — Hardening & release 0.1** | ASVS L2 pass, performance/recovery drills, packaging, docs, public release per SYS-146 | Tagged release installable per UC-001 in ≤30 min |
| **Later** | Evidence-gated and deferred scope | — |

## M0 — Foundation

| TASK | Title | Traces | Depends | Tier | Size |
|---|---|---|---|---|---|
| TASK-001 | Repo scaffold & OSS governance: Go module + directory layout, `LICENSE` (AGPL-3.0-only), SPDX headers + CI check, DCO (`Signed-off-by`) enforcement, CONTRIBUTING, code of conduct, docs licence CC-BY-SA-4.0 | ADR-001, ADR-003, SYS-146 | — | Sonnet | S |
| TASK-002 | CI pipeline: 3-OS build matrix, `golangci-lint`, `tsc` for islands, tests + coverage gates (≥90% domain / ≥80% overall), dependency licence scan, vulnerability scan | SYS-140, SYS-141, ADR-001 §3–4 | TASK-001 | Sonnet | M |
| TASK-003 | Storage foundation: `modernc.org/sqlite`, WAL + `synchronous=FULL`, migration runner, ULIDs, optimistic-versioning helper, append-only audit log (trigger-enforced), startup consistency check, kill-9 durability test harness | ADR-004 §1–3, SYS-081, SYS-046; UC-020 #1–#2 | TASK-001 | Sonnet (Opus review on durability semantics) | L |
| TASK-004 | Domain core & rule-data plumbing: SyRS §2 entities incl. namespaced `externalIds` (ADR-005 §6), category-scheme resolver as data interpreter, Swiss Athletics + UKC category schemes and discipline catalog as versioned data files | ADR-005, SYS-003, SYS-005; UC-002 | TASK-003 | Sonnet | L |
| TASK-005 | Server shell: HTTP server, templ/HTMX base layout, SSE bus, sessions + RBAC roles, adaptive password hashing, i18n message catalogs DE/FR + pseudo-locale CI build, certmagic/ACME TLS | SYS-090, SYS-091, SYS-110, SYS-093; ADR-003 | TASK-003 | Sonnet | L |

## M1 — Kids Cup PoC (the demo spine: UC-001 → UC-033 → capture → UC-034 → UC-035)

| TASK | Title | Traces | Depends | Tier | Size |
|---|---|---|---|---|---|
| TASK-006 | Install & meet creation: hub quickstart (binary + container), meet/venue/timetable setup, ≤30-min fresh-install E2E | UC-001; SYS-131, SYS-001/002/004/006 | TASK-004, TASK-005 | Sonnet | M |
| TASK-007 | UKC template & scoring: built-in UBS Kids Cup meet template, points table as data with official-table fixtures, division standings, missing-discipline ranking | UC-033 #1–#5; SYS-053, SYS-052 | TASK-006 | Sonnet | M |
| TASK-008 | Field capture UI: horizontal-attempt grid (UC-011) + manual track times for UKC 60 m (UC-010 subset), statuses/provenance, live standings | UC-011, UC-010 (subset); SYS-040–042, SYS-045 | TASK-006 | Sonnet | L |
| TASK-009 | Offline capture island: service worker + durable local queue, event-unit checkout, idempotent replay, reconciliation view, office blip tolerance | UC-034 #1–#7; SYS-085–087; ADR-004 §8 | TASK-008 | **Opus** (hardest client piece; anti-Web.TEC semantics) | L |
| TASK-010 | Public live results (PoC scope): public meet pages, SSE live updates, stable URLs, DE/FR, unofficial-results labeling | UC-017 (subset), SYS-070/071/074/076 | TASK-006 | Sonnet | M |
| TASK-011 | Printables (PoC scope): capture sheets (attempt grids, track lanes) + UKC result lists as PDF; Go PDF library selection recorded in `architecture.md` | UC-018 (subset); SYS-072 | TASK-007 | Sonnet | M |
| TASK-012 | Series upload export: UKC organizer-template file via excelize, template-as-data, fixture-verified structure and edge cases | UC-035 #1–#3; SYS-077 | TASK-007 | Sonnet | S |
| TASK-013 | PoC accounts & audit: organizer/office/field-official roles, per-event scoping, privileged-action audit surfacing | UC-022; SYS-090/091 | TASK-005 | Sonnet | M |
| TASK-014 | Durability & backup (PoC confidence): one-action backup artifact, restore-to-fresh-install E2E, crash-mid-capture recovery drill | UC-020 #1–#3; SYS-081/084/130 | TASK-003, TASK-006 | Sonnet | M |
| TASK-015 | M1 demo assembly: seeded demo meet, club-visit demo script, connectivity-chaos test run (blips + offline field phones) | DEC-011; UC-034 #1/#2/#7 | TASK-007…014 | Haiku (assembly) + Sonnet (chaos suite) | S |

## M2 — Club meet complete (remaining MVP)

| TASK | Title | Traces | Depends | Tier | Size |
|---|---|---|---|---|---|
| TASK-016 | Online entries: individual/club/relay entry flows, bib assignment, fee summary | UC-003, UC-006; SYS-011/012/015/017/018 | M1 | Sonnet | L |
| TASK-017 | Entry import & eligibility: CSV/Alabus mapping profile import, eligibility validation | UC-004, UC-005; SYS-013/014/010 | TASK-016 | Sonnet | M |
| TASK-018 | Check-in, seeding & progression: DNS handling, heat seeding + lane draws, round progression — rule fixtures from D-references | UC-007, UC-008, UC-009; SYS-025–030 | TASK-016 | Sonnet | L |
| TASK-019 | Full track capture & corrections: complete UC-010, correction/audit flow, protest clock, announcement timestamps | UC-010, UC-015; SYS-040/041/045–047 | TASK-018 | Sonnet | L |
| TASK-020 | Timing exchange & timing agent: `.ppl/.sch/.evt` out, `.lif` in with conflict resolution; watched-folder agent mode (same binary) on the timing PC | UC-014; SYS-060–062; ADR-006 | TASK-019 | Sonnet | L |
| TASK-021 | Vertical jumps & combined events: height-progression capture, WA scoring engine as data-interpreter with reference fixtures | UC-012, UC-013; SYS-043/044/031 | TASK-018 | Sonnet | L |
| TASK-022 | Records & mixed-category fields: records/bests flagging + documentation checklist; mixed fields with per-category extraction | UC-016, UC-028; SYS-049–052 | TASK-019 | Sonnet | M |
| TASK-023 | Privacy: public data minimization, consent enforcement, data-subject rights, retention purge jobs | UC-023, UC-024; SYS-100–103 | TASK-016 | Sonnet + **Opus privacy review** (CLAUDE.md guardrail; nFADP/GDPR gate) | M |
| TASK-024 | i18n completion & accessibility: full DE/FR operator+public surfaces, WCAG audit of public pages | UC-025, UC-026; SYS-110–113 | M1 | Sonnet | M |
| TASK-025 | omx/v1 round-trip & concurrency proof: full-meet export/import property tests; 10-operator concurrency suite | UC-027, UC-021; SYS-073, SYS-083, SYS-144 | TASK-019 | Sonnet | M |
| TASK-029 | Privacy-review follow-ups (see `docs/delivery/reviews/privacy-review-task-023.md` #1/#2/#4): erasure and retention purge redact `entry`/`participant`/`result` audit-payload PII via `store.RedactAuditPII`; add a public-path crawler regression asserting a withdrawn athlete's name renders nowhere public | SYS-101, SYS-102; UC-024 | TASK-023 | Sonnet | S |

## M3 — Hardening & release 0.1

| TASK | Title | Traces | Depends | Tier | Size |
|---|---|---|---|---|---|
| TASK-026 | Security hardening: OWASP ASVS L2 checklist pass, threat-model review, dependency posture | SYS-092 | M2 | **Opus** | M |
| TASK-027 | Performance & recovery: SYS-120-class benchmarks, 2-minute recovery drill (SYS-130), egress-blocked public-asset check (SYS-105), 2,000-concurrent-viewer public-results load test (UC-017 #4, SYS-122) | SYS-120/121/130/105/122 | M2 | Sonnet | M |
| TASK-028 | Release 0.1: per-OS artifacts + container image, quickstart + operator runbook (network-kit page per ADR-002 v2; operator privacy docs per SYS-104 incl. pseudonymization-vs-anonymization framing, privacy-review-task-023 #3), support matrix, defect policy, release process; replace M0/M1 scaffolding leftovers — `/` landing page still renders the TASK-005 `home.placeholder` ("not yet operational", DE/FR) instead of hub content, and the CLI no-subcommand fallback prints the same instead of usage text (`cmd/bahnfrei/main.go`) | SYS-131/132/143/146, SYS-104 | TASK-026/027/030/031/032 | Sonnet | M |
| TASK-030 | Operator keyboard-only efficiency: keyboard-only E2E pass of check-in/result-entry/status flows and bulk multi-athlete operations (matrix gap found at M2 review — SYS-114 had no owning TASK) | SYS-114; UC-007/UC-010 flows | M2 | Sonnet | S |
| TASK-031 | Contextual input help: reusable help-icon component (opens on hover + focus + tap; Escape-dismiss, hoverable, persistent per WCAG 2.2 SC 1.4.13; AT-associated), machine-readable help-content registry (each entry bound to a route/screen so the UC-037 #1 coverage test is mechanical), DE/FR help texts for all registered inputs, hint-text pass so hard constraints stay visible on-screen (founder request 2026-07-13) | UC-037; SYS-115 | M2 | Sonnet | M |
| TASK-032 | Design system & usability audit: consolidate existing styles into documented tokens + component inventory (in-repo), CI style-conformance check, component-gallery fixture page + focus-visible keyboard-walk e2e over it, author `docs/requirements/usability-audit-checklist.md` (NN/g heuristics + SYS-117 form conventions) and run it once against release 0.1 (founder request 2026-07-13) | UC-038; SYS-116, SYS-117 | M2 | Sonnet | M |
| TASK-033 | Coverage headroom hardening: raise overall statement coverage from 80.7% to ≥83% before other M3 slices add code (M2-close margin over the SYS-140 80% floor was 0.7 pt). Target the concentrated gaps (CI-equivalent profile, 2026-07-13): `internal/app` 78.1% (exchange.go, entry.go, seeding.go, import.go, sync.go), `internal/web` 76.9% (**accounts.go 39.0%**, verticaljump.go 60.3%, **privacy.go 61.6%**, entries.go, timing.go), `cmd/bahnfrei` 75.6% (timingagent.go), `internal/store/timingexchange.go`. Emphasis on error-path and handler tests, not number-chasing; the low-coverage auth/privacy handlers are direct TASK-026 ASVS input, so schedule first in M3, parallel to or before TASK-026. **Ratchet on completion:** amend SYS-140's overall floor to the achieved margin (target ≥83%) and update the matching threshold in `scripts/check-coverage.sh` in the same commit — gate and requirement must never diverge | SYS-140 | M2 | Sonnet | M |
| TASK-034 | Critical usability-finding fixes required for release 0.1: destructive-action confirmation flow for erase/disable/archive/purge (CSP-safe: confirm sub-page or non-inline island, per OQ-074; consider re-auth friction on athlete erasure + retention purge per ASVS review's defense-in-depth note) and inline field-level validation errors that name the field, state what to fix and preserve input (UC-038 #4 is a T-verified criterion; OQ-075) using TASK-032's documented `.field-error`/`aria-invalid` convention; sweep small hygiene closures: OQ-079 same-origin Referer guard in `handleLocaleSwitch`, OQ-063 meet-existence check in `ExportGenericCSV` | UC-038 #3–#4; SYS-117, SYS-116 | TASK-026, TASK-031, TASK-032 | Sonnet | M |
| TASK-035 | Public-results read path per DEC-015 (OQ-066): amend ADR-004 with a read-path section — per-meet cached render of the public results page/fragment, invalidated by the existing results-changed bus event, plus a pooled WAL read connection set (`store.Open`'s single-connection cap becomes writer-only; the ratified single-WRITER invariant is unchanged). SSE delivery is untouched (measured within budget). Done = `TestSYS122TwoThousandConcurrentViewers` passes against the **literal** SYS-122 budget (p95 ≤3 s, errors ≤0.1% at 2,000 viewers, deliberately never weakened per TASK-027's rule) and the scaling profile is re-run with post-fix numbers recorded in `docs/delivery/perf-and-recovery-task-027.md` | SYS-122, SYS-071; UC-017 #4; ADR-004 amendment | TASK-027 | Sonnet (Fable reviews the ADR amendment — one-way-door adjacency) | M |
| TASK-036 | UKC final-standings semantics per DEC-016 (OQ-020): amend UC-033 #3 first (spec-driven), then introduce a final-vs-provisional distinction in `Standings` — live standings keep rank-by-partial-total labeled provisional; final lists (printed/exported/published once a division is complete) list athletes missing a series discipline **unranked at the bottom** per the official TAF3 convention, and the UKC points-table data gains the **1-point floor** for a present-but-no-valid-attempt discipline. Fixture-verify against the LV Langenthal official Rangliste evidence (URL in OQ-020); sweep every surface that renders a final list (public results, PDF result lists, series-upload export, omx snapshot). Investigate the observed all-marks-but-"n.a." case (suggests an out-of-competition status): add an out-of-competition entry flag if cheap, else log a narrowed OQ | SYS-053; UC-033 #3 (amended) | M2 | Sonnet | M |
| TASK-037 | Release distribution per DEC-017/018/019: GHCR publish step in the release workflow (`ghcr.io/kriegalex/bahnfrei`), cosign keyless signing of the container image and `checksums.txt` in CI with verification instructions in the ops docs, support-window wording in `docs/ops/release-process.md` §6 confirmed as the real 0.x policy (latest-only, best-effort); values-only edits to release-process §4/§5/§6 and `docs/ops/quickstart.md` §1 Option B; document the Gatekeeper/SmartScreen workaround for the (deliberately unsigned, DEC-019) macOS/Windows binaries | SYS-146, SYS-131; OQ-086/087/088 | TASK-028 | Sonnet | S |
| TASK-038 | Entry/roster search per DEC-021 (OQ-067): server-side query-param filter on the operator roster and entries lists (name/bib/club at minimum), DE/FR labels, keyboard-reachable; add the reserved SYS-120 benchmark sub-test (`TestSYS120ReferenceScaleOperatorBudgets` gains a real search budget instead of the full-roster proxy) | SYS-120, SYS-114 | M2 | Sonnet | S |
| TASK-039 | Licence number on online entry per DEC-023 (OQ-033): optional licence-number field on the UC-003 individual/bulk entry forms, feeding the existing SYS-014 `HasLicence` evaluation and athlete external-ID enrichment (same licence join-key semantics as the CSV import path, WO §5.3b) — removes the operator round-trip that currently blocks every online entry at licence-required meet tiers | UC-003; SYS-014 | M2 | Sonnet | S |
| TASK-040 | Field-event correction UI per DEC-024 (OQ-036): extend the capture page's correction form (reason/escalation inputs, `capture.templ`) to horizontal and vertical field rows, wired to the existing `CorrectResult` endpoint — settled-level semantics unchanged (attempt-level resumable corrections stay *Later* per DEC-024); protest-window and escalation behavior identical to the track form | UC-015; SYS-047 | M2 | Sonnet | S |
| TASK-041 | Bulk "mark remaining as DNS" per DEC-025 (OQ-070): one action on a still-open unit marking every entry without a captured result as DNS — the second real SYS-114 bulk operation; goes through the TASK-034 confirm sub-page pattern (bulk status write), audited per SYS-046; extend the TASK-030 keyboard-only E2E suite to cover it | SYS-114, SYS-046; UC-010 flows | TASK-034 | Sonnet | S |
| TASK-042 | "My assignments" dashboard per DEC-025 (OQ-089): logged-in landing surface for non-organizer roles — field officials see the meets/units they are scoped to (TASK-013 per-event scoping), competition office its meets, entry submitters their submitted entries per meet; replaces the "use the link your organizer sent you" home copy for those roles (`internal/web/pages.templ`), organizer flow unchanged; DE/FR | SYS-090/091, SYS-114 | M2 | Sonnet | M |
| TASK-043 | Office-reachable meet hub per OQ-111: the `/meets/{id}` meet-detail page is organizer-gated (`organize(s.handleMeetDetail)`), yet it is the only link target the roster/entries/reconciliation pages' `roster.back_to_meet` offers and the only hub reaching check-in, seeding, timing exchange, reconciliation, entries import/eligibility and privacy — so a competition-office session 403s out of its own highest-frequency surfaces. Open the meet-detail hub to office-level sessions with a capability-filtered action list (organizer-only actions — meet edit, archive, account/assignment administration — stay hidden and their POST routes stay organizer-gated); every link the hub renders for a role must resolve for that role (test that, not just the hub's status code); organizer view unchanged; DE/FR for any new copy | SYS-090/091, SYS-114 | TASK-042 | Sonnet | S |

## M4 — Volunteer usability (proposed 2026-08-06; requirements SYS-147–153/STR-046 pending founder ratification)

Source: `docs/delivery/usability-audit-volunteer-2026-08.md` (persona walkthrough against the
real server). Per UC-038 #3, the two Critical findings (F1/F2 → TASK-044/TASK-045) gate the
0.1 release tag until fixed or explicitly waived by the founder.

| ID | Description | Traces | Depends on | Tier | Size |
|----|-------------|--------|------------|------|------|
| TASK-044 | Sync outcome taxonomy & recovery (walkthrough F1, Critical): the sync endpoint gains per-op non-retryable rejection statuses (validation/authz/auth) instead of whole-batch 400; the capture island (`islands/src/capture-offline.ts`) classifies outcomes — only transport/5xx retries or reads as connectivity trouble; a rejection renders at the offending cell with a localized reason and a correct-or-discard affordance, never blocks other queued ops, and the status region never reports "alle übertragen" alongside a pending count; session expiry mid-capture prompts re-authentication with the queue preserved across re-login. MUST preserve the sync version-authority invariant (`internal/sync/doc.go` — acks carry authoritative versions, never blind-increment) and extend the chaos e2e with rejected-op and expired-session scenarios | SYS-149; UC-040 | — | Sonnet | L |
| TASK-045 | Mobile capture ergonomics & save-state feedback (walkthrough F2 Critical, F3): responsive capture and check-in layouts at ≤480 px (row-card or equivalent pattern — the design call is documented in `design-system.md`), compact page header so the first row is on the first viewport, ≥44 px touch targets via tokens, `inputmode`/`enterkeyhint` on mark/time/wind inputs, visible pending/confirmed/failed cell states (token-based, more than color — today `data-pending` has no styling at all), row result/points update on ack without reload; track-row progressive disclosure (per-race timing-method as unit-level default, DQ-rule field only when status=DQ); extend `usability-audit-checklist.md` with a "point-of-competition mobile" row set; Playwright mobile-viewport + axe e2e | SYS-147, SYS-148; UC-039 | TASK-044 (failed-state rendering) | Sonnet | L |
| TASK-046 | Task-first navigation & assignment context (walkthrough F5): office home gains per-meet check-in/capture/reconciliation links; field-official assignments show localized discipline, scheduled time and location, deep-linking to capture; meet hub grouped by task area (preparation / competition day / publication); extend the TASK-043 link-walk to assert every operator surface is reachable from its role's home; ratification of SYS-151 answers OQ-113 | SYS-151; UC-041 #1–#3 | — | Sonnet | M |
| TASK-047 | Empty states & inapplicable-action gating (walkthrough F7/F8): why-empty + next-step copy across operator list surfaces (check-in, standings, entries, capture standings blocks); hide/disable close-check-in and bulk-DNS when nothing can be affected, with reason; affected-row count on bulk/destructive confirm pages (TASK-034/041 confirm sub-pages); DE/FR | SYS-152; UC-041 #4–#5 | — | Sonnet | S |
| TASK-048 | Public find-your-athlete (walkthrough F9): name/bib/club filter + per-category jump navigation on public start lists and results; progressive enhancement (server-rendered fallback), filter survives SSE updates; MUST respect the ADR-004 §9 render-cache design — filter client-side over the cached fragment or account for cache keying, never fragment the per-meet/per-locale cache per query | SYS-153; UC-042 | — | Sonnet | M |
| TASK-049 | Participant identity correction (walkthrough F4): office edit of name/birth year/sex/club/bib — version-guarded, audited via the SYS-046 mechanism, propagates to roster/start lists/capture/standings/exports, never re-scores captured marks; the tricky part is category re-derivation on birth-year/sex change (standings category move) and bib-uniqueness on bib change; uses the TASK-034 FieldErrors + confirm conventions | SYS-150; UC-043 | — | Sonnet | M |
| TASK-050 | Localization & UI hygiene sweep (walkthrough F6/F11 — defects vs existing SYS-110/111/116, no new requirement): localized discipline names on the assignments dashboard, capture pages and meet-hub programme/timetable (the standings path already localizes — reuse it); operator-facing timestamps in the meet's local timezone with locale formatting (no raw UTC); fix the htmx CSP inline-style console error (htmx config or hashed style); serve a neutral placeholder favicon to stop the per-page 404 (final icon awaits OQ-061 branding); regression tests pin localized discipline rendering per surface | SYS-110, SYS-111, SYS-116 | — | Sonnet | S |

## Release-0.1 polish

| ID | Description | Traces | Depends on | Tier | Size |
|----|-------------|--------|------------|------|------|
| TASK-051 | Release-0.1 audit Minor cleanup (`usability-audit-2026-08-r01.md` N1–N4): N1 localize the check-in page heading's discipline name (the TASK-050 sweep missed this surface — reuse its view-struct-layer localization path); N2 make the "Meine Meldungen" empty state's next-step copy conditional on whether entry forms actually render (closed entry window must not point at "Formulare oben"); N3 singular/plural forms for `public.filter.count` ("1 Ergebnisse"); N4 24 px min-height token for `<select>` controls (locale switcher is 109×21). DE/FR + regenerated pseudo-locale; regression tests pin N1–N3 | SYS-110/111, SYS-152, SYS-116 | M4 | Sonnet | S |

## Later (not scheduled; triggers noted)

| Item | Trigger |
|---|---|
| UC-019 venue-node role + ADR-004 §4–5 sync | Evidence gate per ADR-002 v2 §3 (a season of hub-first meets, or a committed no-uplink venue) |
| UC-036 TAF3 coexistence exports (SYS-078) | First sanctioned-meet coexistence need; OQ-013/014 progress |
| Visana Sprint / Mille Gruyère templates (SYS-077 *Later* part) | Club demand after UKC PoC |
| Relay completeness bundle (DEC-020): manual relay result capture, timing roster convention, per-leg eligibility, entry standards, relay-scoped erasure test (OQ-027/032/049/057/059) | First post-0.1 club meet that runs relays |
| UC-029 team scoring, UC-030 sponsors, UC-031 scoreboard feed, UC-032 EDM | StRS Later priorities |
| Alabus/official-tier recognition work | OQ-014 (recognition ask to Swiss Athletics, from PoC traction) |

## Orchestration rules (Fable Tech Lead)

1. Workers get one TASK (or one UC within a TASK), the relevant spec excerpts by ID, and
   return summaries + diffs — not raw tool output.
2. Merge order follows the dependency column; independent tasks fan out in parallel.
3. Every merge updates `traceability-matrix.md` (test IDs replace `Phase B` placeholders).
4. Anything smelling like a new one-way door stops work and becomes an ADR proposal first.
