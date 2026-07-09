# Architecture Baseline

**Status:** BASELINE — ADR-001…ADR-006 ratified by the founder 2026-07-05 (DEC-014).
This document is the living architecture description; decisions live in `adr/`;
requirements it satisfies are cited as `SYS-###`.

## 1. System context

```
                        ┌─────────────────────────────┐
   athletes/clubs ────▶ │  HUB node (homelab/cloud)   │ ◀──── public: spectators,
   online entries       │  entries · public results   │       media (read-only)
   (SYS-011)            │  archive · TLS (SYS-093)    │
                        └────────────┬────────────────┘
                     entries snapshot│  ▲ publication queue
                     (pull, ADR-004) ▼  │ (ordered, idempotent, SYS-082)
                        ┌────────────┴────────────────┐
   operators on LAN ──▶ │  VENUE node (laptop, LAN,   │ ◀──▶ timing systems
   browsers: office,    │  offline-capable, SYS-080)  │      (FinishLynx file family
   call room, infield   │  single binary (ADR-002/003)│       via shared dir, ADR-006)
                        └─────────────────────────────┘
                                     │ file import/export (CSV, omx/v1)
                                     ▼
                        federation channels (Alabus entries in,
                        results export out — SYS-013/073); printed documents (SYS-072)
```

External systems we integrate with but never replace: Alabus/Swiss Athletics entry channels,
the Seltec LA.portal official-results channel (labeled per SYS-076), World Athletics Global
Calendar (StRS §4.3).

## 2. Runtime & containers

One Go executable (ADR-003), role selected at startup (`venue` default, `hub`), assets and
translations embedded, SQLite storage in a data directory beside the binary (venue) or a
configured path (hub). Browser clients only (SYS-132). Live updates via SSE (SYS-071).

## 3. Module layout (Go packages, target shape)

Module `github.com/kriegalex/bahnfrei`; packages live under `internal/` (not importable
by external modules), the executable under `cmd/bahnfrei/`.

| Package | Responsibility | Key SYS |
|---------|----------------|---------|
| `domain/` | Pure domain: entities (SyRS §2), rule engines — category resolver, seeding (TR20), progression, scoring (WA tables, UKC), countback, eligibility, wind legality, records flagging. **No I/O.** Fixture suites per engine (SYS-142) | 005, 014, 026–031, 040–053 |
| `app/` | Use-case services orchestrating domain + storage + audit; authorization checks (SYS-090); result-confirm flow with provenance | 046, 047, 090 |
| `store/` | SQLite persistence, migrations, optimistic versioning, audit log, backup, retention jobs (ADR-004) | 081, 084, 101, 102 |
| `sync/` | Offline-capture wire protocol (checkout/replay contract, ADR-004 §8 — server half in `app`/`web`); *later*: publication queue dispatcher (venue) and applier (hub), entries-snapshot pull | 082, 085–087 |
| `exchange/` | omx/v1 schema (ADR-005), CSV import/export, Alabus mapping profile, Lynx file adapters + directory watcher (ADR-006) | 013, 060–062, 073 |
| `web/` | HTTP handlers, SSR templates, HTMX endpoints, SSE, i18n rendering, public pages incl. unofficial-results labeling (SYS-076), WCAG-conformant markup | 070–076, 110–114 |
| `pdf/` | Printable documents | 072 |
| `cli/` | Startup, role selection, quickstart bootstrap, backup/restore commands | 131, 084 |

Dependency rule: `domain` imports nothing above it; `web` never touches `store` directly
(goes through `app`). Enforced by a lint rule in CI.

Outside the Go module: `islands/` holds the TypeScript island sources (ADR-003 — strict
`tsconfig` per compilation unit, DOM islands vs. the capture service worker; compiled to
readable ES2020 in `internal/web/static/` by `scripts/build-islands`, emitted JS committed
with a CI freshness check) and `e2e/` holds the Playwright suite (§5.3) that drives the
real binary — per-test server on a temp SQLite DB, seeded through the product's own forms.

## 4. Cross-cutting concerns

- **AuthN/Z:** local accounts, adaptive password hashes, per-meet role assignments
  (SYS-090/091); public pages unauthenticated (SYS-070). Venue LAN trust model per ADR-002.
- **Audit:** append-only log in `store`, written by `app` on every mutating use-case (SYS-046).
- **i18n:** message catalogs DE/FR complete; domain vocabulary from the reviewed glossary
  (SYS-110/111); pseudo-locale CI build proves translation-only extensibility (UC-025 #2).
- **Privacy:** public-surface field allowlist (SYS-100) enforced centrally in `web` view
  models — not per-template; consent flags evaluated at render time (SYS-103); PII scanner
  runs against rendered pages in CI (UC-023 #1).
- **Time:** all rule deadlines (protest clocks) derive from server time with explicit
  timezone handling; announcement timestamps recorded in `app` (SYS-047).
- **Error handling:** confirmed-write semantics — the UI shows success only after durable
  commit (SYS-081).

## 5. Verification strategy (maps to SYS-140…143)

1. **Rule-engine fixtures** (`domain`): expected values from primary sources (D-refs) —
   scoring tables, rounding, wind, countback, lane draws, category bounds (SYS-142).
2. **Use-case tests** (`app` + `store`): every UC criterion automated (SYS-140); crash/
   durability tests via process-kill harness (UC-020).
3. **Browser E2E** (Playwright): operator flows keyboard-only (SYS-114), offline suite with
   egress blocked (UC-019), accessibility scans (UC-026), PII scan (UC-023).
4. **Benchmarks:** reference dataset (1,500 athletes / 4,000 entries / 250 units) for
   SYS-120–122 budgets.
5. **CI gates:** build matrix (Linux/macOS/Windows), vet/lint, tsc, coverage thresholds,
   dependency vulnerability + licence scans (SYS-141, ADR-001).

## 6. Deployment topologies

| Topology | When | Notes |
|----------|------|-------|
| Venue-only | Club evening, no online entries/results | Print + LAN results; export files afterwards |
| Venue + hub | Standard meet (canonical UKC scenario, UC-033) | Entries online on hub → snapshot to venue → live publication queue back to hub |
| Hub-only | Small meet with reliable internet | Venue devices use the hub directly; offline guarantee not available (documented trade-off) |

Hub reference deployment: single container/systemd service + TLS reverse proxy (founder
homelab, DEC-006). Reference sizing documented for SYS-122 load target.

## 7. ADR index

| ADR | Decision | Status |
|-----|----------|--------|
| [ADR-001](adr/ADR-001-license-agpl-3.0.md) | AGPL-3.0-only + DCO | Accepted |
| [ADR-002](adr/ADR-002-deployment-and-application-model.md) | Local-first web app, single binary, venue/hub roles | Accepted |
| [ADR-003](adr/ADR-003-technology-stack.md) | Go + SQLite + SSR/HTMX/SSE + TS islands | Accepted |
| [ADR-004](adr/ADR-004-storage-durability-and-sync.md) | SQLite WAL, single-writer, audit log, one-way sync | Accepted |
| [ADR-005](adr/ADR-005-domain-model-and-exchange-schema.md) | Own domain model, omx/v1 open schema, rules-as-data | Accepted |
| [ADR-006](adr/ADR-006-timing-integration.md) | FinishLynx file family first, watched folder | Accepted |
