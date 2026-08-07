# Support matrix

**Status:** release 0.1.

## 1. Server (hub / venue-local host)

| | Hub mode | Venue-local mode |
|---|---|---|
| OS | Mainstream Linux server distribution (the pure-Go, no-cgo build — `modernc.org/sqlite`, see [ADR-003](../architecture/adr/ADR-003-technology-stack.md)/[ADR-004](../architecture/adr/ADR-004-storage-durability-and-sync.md) — has no OS-specific runtime dependency; any distribution with a working TLS/network stack is expected to work) | Any of the release targets below, single laptop-class machine |
| Hardware | Whatever the deployment is sized for; the reference class ("commodity 4-core CPU, 8 GB RAM") is the documented performance baseline (see §3) | Single volunteer laptop — commodity consumer hardware, no server-grade requirement |
| Build targets shipped | `linux/amd64`, `linux/arm64` (server-class ARM, e.g. Graviton/Ampere) | `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64` — `scripts/build-release.sh` |
| Container image | Built from the repo `Dockerfile` (distroless runtime base); `linux/amd64`/`linux/arm64` | n/a (binary is the simpler venue path) |

There is no `windows/arm64` or `linux/386` target for 0.1 — not requested by any known deployment
target; add on demand.

## 2. Operator client (browser)

By design, operator clients require only a current mainstream web browser — no native
app, no browser extension, no plugin. Supported: the **latest two major versions** of each
mainstream rendering engine:

| Engine | Representative browsers |
|---|---|
| Chromium | Chrome, Edge, Brave, Opera |
| Gecko | Firefox |
| WebKit | Safari (macOS/iOS) |

Public-facing pages additionally target **WCAG 2.2 Level AA** and are legible on phones
≥360 px wide — see `docs/requirements/usability-audit-checklist.md` for the audit that
verifies both per release. The operator UI is keyboard-operable end to end for high-frequency
actions.

## 3. Measured performance envelope

Reference environment for the performance budgets below: **commodity 4-core CPU, 8 GB RAM class.**
Numbers below were measured on a stronger development host (16-core, 31 GiB RAM, NVMe — see
`docs/delivery/perf-and-recovery-task-027.md` for the full methodology and a slower-host
extrapolation) with a memory-confined test harness (`systemd-run … MemoryMax=12G`,
`GOMEMLIMIT=10GiB`), because an earlier unconfined run OOM-killed the host.

**Operator-facing operations:** ≤76 ms p95 at reference scale for every measured action (check-in,
result capture, corrections) — no operator-visible slowness at any tested load.

**Public live-results viewers — target met** (a pooled WAL read-connection set plus a per-meet
public-results render cache — see
[ADR-004](../architecture/adr/ADR-004-storage-durability-and-sync.md) §9):

| Concurrent viewers | p95 page render | Peak heap (confined process) | Memory per viewer |
|---|---|---|---|
| 100 (+5 SSE subscribers) | 185.8 ms | 85.0 MiB | 0.85 MiB |
| 250 (+13 SSE subscribers) | 210.9 ms | 91.1 MiB | 0.36 MiB |
| 500 (+25 SSE subscribers) | 251.0 ms | 103.9 MiB | 0.21 MiB |
| 2,000 (the target concurrent-viewer count, +100 SSE subscribers) | 343–377 ms | 179–184 MiB | ~0.09 MiB |

**What this means for a deployment today:** the reference-class host clears the
2,000-concurrent-viewer budget (≤3s p95, ≤0.1% errors) with roughly an 8–9× latency margin, and
peak process memory stays flat (well under 200 MiB) across the whole 100→2,000-viewer range
rather than growing with viewer count — 2,000 simultaneous spectators is now a safe, ordinary
operating point, not a special case requiring a reverse proxy or a smaller expectation.

**Why this holds** (transfers to any host, including slower ones): the results page's expensive
work — the athletes×disciplines standings computation and the HTML render — now happens once per
result change (invalidated by the same event that drives the SSE live-refresh push), not once per
viewer; concurrent viewers share that one cached render and a pooled set of read-only SQLite
connections instead of convoying behind the single writer connection. **SSE live-update delivery**
remains fine at every tested scale (11–39 ms p95, well inside its 10s budget) and is unchanged by
this fix.

**Operator guidance:** no special provisioning is needed for expected public spectator traffic —
competition-day capture, office operation and public results all run on the one binary with no
external caching layer required.
