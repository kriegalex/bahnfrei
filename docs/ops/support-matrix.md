# Support matrix (SYS-132)

**Traces:** SYS-132, SYS-122 → STR-039, STR-036. **Status:** release 0.1.

## 1. Server (hub / venue-local host)

| | Hub mode | Venue-local mode |
|---|---|---|
| OS | Mainstream Linux server distribution (the pure-Go, no-cgo build — `modernc.org/sqlite`, ADR-003/ADR-004 — has no OS-specific runtime dependency; any distribution with a working TLS/network stack is expected to work) | Any of the release targets below, single laptop-class machine |
| Hardware | Whatever the deployment is sized for; SYS-120's reference class ("commodity 4-core CPU, 8 GB RAM") is the documented performance baseline (see §3) | Single volunteer laptop — commodity consumer hardware, no server-grade requirement |
| Build targets shipped | `linux/amd64`, `linux/arm64` (server-class ARM, e.g. Graviton/Ampere) | `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64` — `scripts/build-release.sh` |
| Container image | Built from the repo `Dockerfile` (distroless runtime base); `linux/amd64`/`linux/arm64` | n/a (binary is the simpler venue path) |

There is no `windows/arm64` or `linux/386` target for 0.1 — not requested by any known deployment
target; add on demand.

## 2. Operator client (browser)

Per SYS-132: *"operator clients SHALL require only a current mainstream web browser"* — no native
app, no browser extension, no plugin. Supported: the **latest two major versions** of each
mainstream rendering engine:

| Engine | Representative browsers |
|---|---|
| Chromium | Chrome, Edge, Brave, Opera |
| Gecko | Firefox |
| WebKit | Safari (macOS/iOS) |

Public-facing pages additionally target **WCAG 2.2 Level AA** (SYS-112) and are legible on phones
≥360 px wide (SYS-113) — see `docs/requirements/usability-audit-checklist.md` for the audit that
verifies both per release. The operator UI is keyboard-operable end to end for high-frequency
actions (SYS-114).

## 3. Measured performance envelope

Reference environment for the SYS-120/121 budgets: **commodity 4-core CPU, 8 GB RAM class.**
Numbers below were measured on a stronger development host (16-core, 31 GiB RAM, NVMe — see
`docs/delivery/perf-and-recovery-task-027.md` for the full methodology and a slower-host
extrapolation) with a memory-confined test harness (`systemd-run … MemoryMax=12G`,
`GOMEMLIMIT=10GiB`), because an earlier unconfined run OOM-killed the host.

**Operator-facing operations:** ≤76 ms p95 at reference scale for every measured action (check-in,
result capture, corrections) — no operator-visible slowness at any tested load (SYS-120/121 pass
with wide margin).

**Public live-results viewers (SYS-122) — target NOT met, tracked as OQ-066:**

| Concurrent viewers | p95 page render | Peak heap (confined process) | Memory per viewer |
|---|---|---|---|
| 100 (+5 SSE subscribers) | 6.77 s | 6.4 GiB | **64.4 MiB** |
| 250 (+13 SSE subscribers) | 17.2 s | 9.8 GiB | ~39 MiB (GC-pressure artifact, not a real economy — see the perf doc) |
| 500+ | — | **OOM-killed at the 12 GiB confinement cap** | — |
| 2,000 (SYS-122's literal target) | — | **OOM-killed at the 12 GiB confinement cap, ~32 s in** | — |

**What this means for a deployment today:** the honestly-supportable public-viewer count on a
reference-class host is on the order of the **100-viewer measurement (≈64 MiB in-flight per
viewer, ≈6.4 GiB peak heap)** — a small-to-mid club meet's realistic spectator load — not the
2,000-viewer target SYS-122 specifies. 250 concurrent viewers already approaches double-digit
seconds of page-render latency and ~10 GiB of heap; treat that as the current practical ceiling,
not a safe operating point. Above roughly 500 viewers, expect the process to be killed by memory
pressure on commodity hardware, not a graceful slowdown.

**Root cause (not a hardware problem — it transfers to any host, including faster ones):**
uncached per-request results-page rendering (`ResultsService.Standings` recomputes its full
athletes×disciplines view model on every request) held live by convoyed in-flight requests against
a single-writer SQLite connection. **SSE live-update delivery itself is fine at every tested
scale** (27 ms–1.9 s p95, well inside its own budget) — the bottleneck is exclusively the
synchronous page-render path, not the live-update fan-out.

**Path to closing OQ-066** (a founder/tech-lead architecture decision, not attempted in this
release): pooled read connections plus per-meet caching of the rendered public results
page/fragment, invalidated on the existing results-changed event — one render per result change
instead of one per viewer — and/or redefining the SYS-122 "reference hosting size" to include a
caching reverse proxy in front of the hub.

**Operator recommendation until OQ-066 closes:** for any meet where public spectator traffic might
exceed ~100–150 concurrent viewers (a live-streamed regional meet, a large school event), plan for
either a caching reverse proxy in front of the hub or accept degraded/unavailable public results
under peak load — competition-day capture and office operation are unaffected either way, since
they don't share this bottleneck.
