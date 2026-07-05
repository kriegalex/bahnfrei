# ADR-003 — Technology stack: Go backend, SQLite, server-rendered UI with HTMX + SSE

**Status:** **Accepted** — ratified by the founder 2026-07-05 (one-way door; Go-vs-Node annex reviewed)
**Date:** 2026-07-05
**Traces:** SYS-131–133, SYS-140–142, SYS-080/081, SYS-110, SYS-112, STR-038, DEC-002, DEC-006; depends on ADR-002

## Context

ADR-002 requires a single self-contained, cross-platform (Windows/macOS/Linux) executable that
a volunteer can run on a laptop, serving browser clients on a LAN, with embedded storage,
embedded UI assets, zero external services, and CI-enforceable static typing and coverage
(SYS-140–142). The stack must also be attractive/learnable for OSS contributors (STR-038) and
boring enough to survive a decade of volunteer maintenance.

## Decision

| Layer | Choice | Key reason |
|-------|--------|-----------|
| Language/runtime | **Go** (current stable) | Single static binary per OS/arch via cross-compilation; assets embedded with `go:embed`; strong typing (SYS-141); mainstream and learnable |
| Database | **SQLite** via a CGO-free driver (e.g. `modernc.org/sqlite`) | Embedded (no DB server to install — SYS-131/133); WAL + synchronous=FULL supports the durability contract (SYS-081, detail in ADR-004); keeps cross-compilation trivial |
| HTTP/UI model | **Server-rendered HTML (Go templates/templ), progressively enhanced with HTMX; SSE for live updates** | Offline-embeddable, no client build framework churn; SSR is the accessibility-friendly default (SYS-112); SSE covers the ≤10 s public-update budget (SYS-071) without websocket infrastructure |
| Client-side islands | **TypeScript, minimal, per-widget** (e.g. field-event capture grid, keyboard-first office actions per SYS-114) | Type-checked (SYS-141) without adopting a SPA framework |
| PDFs | Server-side Go PDF generation (library choice = implementation detail, fixture-tested per SYS-072) | No headless-browser dependency in the binary |
| i18n | Standard message-catalog extraction (DE/FR complete; pseudo-locale CI build proves translation-only extension per SYS-110) | DEC-008 |
| Tests | Go `testing` (+ `testify`-class asserts), rule-engine fixture suites (SYS-142), Playwright for browser E2E incl. egress-blocked offline suite (UC-019) | SYS-140 gates |
| CI | GitHub Actions (or equivalent): build matrix, vet/lint (golangci-lint), typecheck (tsc for islands), tests+coverage, licence & vulnerability scans | SYS-141, ADR-001 |

## Alternatives considered

| Option | Why rejected |
|--------|--------------|
| TypeScript full-stack (Node/SvelteKit or Next) | Largest contributor pool, but self-contained cross-platform binary packaging is second-class; heavier at runtime on old volunteer laptops; framework churn is a real 10-year maintenance risk for a volunteer project |
| Rust (axum) + SQLite | Equally good binary story, better raw performance (unneeded per SYS-120); steeper contributor on-ramp conflicts with STR-038 |
| C# / .NET self-contained publish | Technically solid; smaller grassroots-OSS contributor overlap in this domain; heavier single-file artifacts |
| Python (Django) | Weakest packaging/typing story for a double-click venue binary |
| SPA frontend (React/Vue) + JSON API | Two codebases and an API contract to version for one product; SSR+HTMX yields the needed interactivity with far less surface; a JSON API for third parties remains possible later (SYS-144 covers schemas) |

## Consequences

- One language for all domain logic; rule engines (seeding, scoring, eligibility, countback)
  are plain Go packages with fixture tests traceable to D-references (SYS-142).
- The binary target makes "download → double-click → meet" real (UC-001 #1); release artifacts
  are per-OS files, plus a container image for hub/homelab deployment.
- HTMX-style hypermedia UI is less familiar to some contributors than React — mitigated by
  strict UI conventions documented in `architecture.md` and the small TS-island escape hatch.
- Accepting Go's plainness over expressive type systems is deliberate: reviewability by
  occasional volunteer contributors beats sophistication (DEC-002/STR-038).

## Annex A — Ecosystem reality check: Go vs Node/TS (ratification support, 2026-07-05)

Scored against *this project's* actual needs, not ecosystems in the abstract.

### Library coverage for our concrete requirements

| Need (requirement) | Go | Node/TS | Edge |
|---|---|---|---|
| Embedded SQLite (ADR-004) | `modernc.org/sqlite` — pure Go, no cgo, cross-compiles trivially | `better-sqlite3` — excellent but a **native module**, the classic single-binary blocker; `bun:sqlite` ties you to Bun | Go |
| Single self-contained binary (ADR-002/006) | Native: `go build` + `go:embed`, trivial cross-compilation, ~15–30 MB | Improving but young: Node 25.5 `--build-sea` ([one-step SEA, Jan 2026](https://progosling.com/en/dev-digest/2026-01/nodejs-25-5-build-sea-single-executable)); native add-ons still awkward; Bun `--compile` works but binaries embed the runtime (~60–100 MB) and commit to Bun | Go, clearly |
| xlsx exports — youth-series templates (SYS-077) | `excelize` — mature, active | `exceljs`/SheetJS — equally strong | tie |
| PDF generation (SYS-072) | **Weakest Go spot**: `gofpdf` archived; maintained forks (`go-pdf/fpdf`) + `maroto` are adequate for tabular meet documents but unglamorous | Richer (`pdfkit`, or Puppeteer at the cost of bundling Chromium) | Node |
| Built-in TLS/ACME for self-hosted hubs (ADR-002 v2) | `certmagic`/`autocert` — best-in-class (powers Caddy); "one binary, gets its own cert" | Weak natively; ecosystem norm is "put a reverse proxy in front" — an extra moving part for a self-hosting club | Go, concretely |
| SSR templates + HTMX + SSE | `html/template`/`templ`; SSE is stdlib | Equally fine | tie |
| i18n (SYS-110) | `x/text` + `go-i18n` — sufficient | `i18next` — richer | Node, marginally |
| Watched-folder timing exchange (ADR-006) | `fsnotify` | `chokidar` | tie |

### Structural comparison

| Dimension | Go | Node/TS |
|---|---|---|
| Contributor pool | Large (consistently top-10 language); **smaller than JS** | The largest pool in software |
| Onboarding a casual volunteer | One toolchain (build/test/fmt/vet/embed built in); famously small language | Must choose runtime (Node/Deno/Bun), bundler, framework; ESM/CJS scars persist |
| 10-year churn risk | **Go 1 compatibility promise has held since 2012** — 14 years of code that still compiles; stdlib-heavy culture | Framework/runtime churn is the ecosystem's defining trait; a 2016 Node app is a rewrite today |
| Typical dependency tree | Tens of direct+transitive deps | Hundreds to thousands transitive |
| Supply-chain exposure (SYS-092/141) | Small tree + module proxy + sumdb checksums | Ongoing major incidents: Sept 2025 worm compromising 180+ packages incl. 18 with 2.6 B weekly downloads ([Palo Alto](https://www.paloaltonetworks.com/blog/cloud-security/npm-supply-chain-attack/), [CISA alert](https://www.cisa.gov/news-events/alerts/2025/09/23/widespread-supply-chain-compromise-impacting-npm-ecosystem)); another namespace compromise June 2026 ([Unit 42](https://unit42.paloaltonetworks.com/monitoring-npm-supply-chain-attacks/)) |
| Proof our exact shape works | **PocketBase** (Go+SQLite single-binary backend), Gitea/Forgejo, Caddy, Miniflux, Navidrome, Syncthing — all volunteer-maintained, cross-platform, single-binary, some decade-plus | Ghost, n8n — healthy self-hosted Node projects, but tellingly all standardize on **Docker** distribution, not a double-clickable file |
| Where the pool argument is blunted | The UI layer is HTML templates + **TypeScript islands anyway** — web-skilled contributors can contribute without writing Go | — |

### Does hub-first (DEC-013) change the calculus?

Partially — an MVP that is "a server on a VPS" could ship as a container, where Node is
perfectly at home. But two binary targets survive in MVP regardless: the **timing agent** on
the meet's Windows timing PC (ADR-006 amendment) and the club self-host story ("download one
file" beats "install Docker" for a volunteer homelab); and the deferred venue node (ADR-002
v2 §3) would reopen the packaging problem at the worst time. Choosing Node now would optimize
for the phase where the choice matters least.

### Verdict

Go is not just viable — for this artifact shape (self-hosted, single-binary, SQLite-embedded,
decade-horizon, volunteer-maintained) it is the 2026 default, with PocketBase as the
existence proof. The honest costs: a smaller contributor pool than JS (mitigated by the TS/HTML
surface), and PDF generation as the one genuinely weaker library area (mitigated by tabular
document needs + fixture tests; worst case, print-CSS covers operator printing). The decisive
positives for *this* project are packaging, the ACME story for self-hosting clubs, dependency
hygiene, and churn resistance. If the founder weighs contributor pool above all, the fallback
is TypeScript on **Bun** (`--compile` + `bun:sqlite`) — accepting runtime-vendor risk and npm
supply-chain exposure as the price.
