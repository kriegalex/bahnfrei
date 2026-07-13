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

## Later (not scheduled; triggers noted)

| Item | Trigger |
|---|---|
| UC-019 venue-node role + ADR-004 §4–5 sync | Evidence gate per ADR-002 v2 §3 (a season of hub-first meets, or a committed no-uplink venue) |
| UC-036 TAF3 coexistence exports (SYS-078) | First sanctioned-meet coexistence need; OQ-013/014 progress |
| Visana Sprint / Mille Gruyère templates (SYS-077 *Later* part) | Club demand after UKC PoC |
| UC-029 team scoring, UC-030 sponsors, UC-031 scoreboard feed, UC-032 EDM | StRS Later priorities |
| Alabus/official-tier recognition work | OQ-014 (recognition ask to Swiss Athletics, from PoC traction) |

## Orchestration rules (Fable Tech Lead)

1. Workers get one TASK (or one UC within a TASK), the relevant spec excerpts by ID, and
   return summaries + diffs — not raw tool output.
2. Merge order follows the dependency column; independent tasks fan out in parallel.
3. Every merge updates `traceability-matrix.md` (test IDs replace `Phase B` placeholders).
4. Anything smelling like a new one-way door stops work and becomes an ADR proposal first.
