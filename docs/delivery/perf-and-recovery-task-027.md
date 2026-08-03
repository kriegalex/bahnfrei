# TASK-027 — Performance & recovery: results and environment caveats

**Date of record:** 2026-07-13 (TASK-027 baseline); SYS-122 re-measured 2026-08-03 (TASK-035)
**Scope:** SYS-120/121 benchmarks, SYS-130 recovery drill, SYS-105 egress-blocked
public-asset check, SYS-122 2,000-concurrent-viewer load test (UC-017 #4).
**Traceability:** rows SYS-105/120/121/122/130 in `docs/requirements/traceability-matrix.md`.
**Open questions raised:** OQ-066 (SYS-122 unmeetable on the original read architecture;
resolved by DEC-015/TASK-035 — see Deliverable 4), OQ-067 (no entry-search feature exists to
benchmark), OQ-094 (TASK-035: the render cache's staleness ceiling for writes outside its
invalidation hook).

## How to run

| Suite | Command | Gate |
|---|---|---|
| SYS-105 egress check | `go test -run TestPublicSurfaceRendersWithEgressBlockedSYS105 ./internal/web/` | default `go test ./...` (fast, deterministic) |
| SYS-120/121 benchmarks, SYS-130 drill, SYS-122 scaling profile | `scripts/run-perf-tests.sh` (or `… quick`) | `perf` build tag, on demand |
| plus the full 2,000-viewer SYS-122 run | `scripts/run-perf-tests.sh full` | `perf` build tag, on demand |

**Memory confinement is mandatory for the perf suite** — see the incident below.
`scripts/run-perf-tests.sh` wraps every invocation in
`systemd-run --user --scope -p MemoryMax=12G -p MemorySwapMax=0` with
`GOMEMLIMIT=10GiB` (no sudo needed; the Go runtime GC-throttles at 10 GiB before
the 12 GiB hard cap kills the scope). Never invoke the SYS-122 tests with a bare
`go test` — the script refuses to run them at all on hosts without `systemd-run`.

## Environment

All numbers below were measured on the development host: **16-core CPU, 31 GiB
RAM, NVMe, CachyOS Linux** — substantially stronger than SYS-120's named
reference environment ("commodity 4-core CPU, 8 GB RAM class") and not a
controlled/idle machine. Consequences:

- Bounded-suite budgets are asserted at **spec budget × 4** (CI-headroom
  ceiling) with the literal spec budget logged, so noisy-neighbour CI runs
  don't flake; every literal budget was ALSO met on this host, with the
  margins below.
- The measured margins (>100×) are wide enough that a 4-core/8 GB host will
  still clear the literal SYS-120/121 budgets comfortably; a confirmation run
  on reference-class hardware is cheap (`scripts/run-perf-tests.sh quick`) but
  was not part of this record.
- The SYS-122 failure is architectural (queueing + per-request allocation, see
  OQ-066), so a slower host only makes it worse; the finding transfers.
- `t.TempDir()` databases live on real NVMe here (not tmpfs), so
  `synchronous=FULL` fsyncs are genuinely exercised.

## OOM incident (2026-07-13) — why confinement is mandatory

The first, unconfined 2,000-viewer SYS-122 run reached **~27 GB RSS** and was
OOM-killed by the host kernel — twice, each time taking the whole terminal
session (and every running agent) with it. The blow-up is a real server-side
measurement, not harness overhead: ~64 MiB of in-flight heap per concurrent
viewer (see the scaling table below), dominated by
`ResultsService.Standings`' per-request athletes×disciplines matrix and view
model at reference scale, held live by thousands of convoyed in-flight
requests. All subsequent runs were confined; both capped runs that exceeded the
limit died **inside** the scope with the host unaffected (journal: "The kernel
OOM killer killed some processes in this unit … 12G memory peak").

## Deliverable 1 — SYS-120/121 benchmarks

`internal/app/perf_bench_test.go` (build tag `perf`). Fixture:
`internal/apptest.SeedLargeMeet(SYS120Scale)` — a synthetic meet at the
SYS-120 reference floor (**1,500 athletes, 4,000 entries, 250 event-units, 3
days**), bulk-inserted store-level in batched transactions (fixture setup is
not the operation under test), one round+unit per event, track and
field-horizontal disciplines, ~2/3 of entries carrying settled results.

Measured 2026-07-13 (post-merge with main @ `da245e3`, confined scope):

| Operation (SYS-120 wording) | Test | p95 measured | Spec budget | Verdict |
|---|---|---|---|---|
| List load (meet detail: programme + rounds + units) | `TestSYS120ReferenceScaleOperatorBudgets/MeetDetailListLoad` | **7.9 ms** (n=30) | ≤2 s | PASS (≈250×) |
| Entry search — proxied by full participant-roster load; no search feature exists, **OQ-067** | `…/ParticipantRosterSearch` | **10.5 ms** (n=20) | ≤2 s | PASS (≈190×) |
| Result save (`SaveTrackResult`, the real capture path) | `…/ResultSave` | **12.5 ms** (n=30) | ≤2 s | PASS (≈160×) |
| SYS-121: standings recompute after a `CorrectResult` | `…/StandingsRecomputeAfterCorrection` | **75.8 ms** (n=10) | ≤5 s | PASS (≈65×) |
| SYS-121: seeding generation, 200-entry event, heats+lanes | `TestSYS121SeedingGenerationBudget` | **46 ms** (single gen) | ≤10 s | PASS (≈215×) |

Method notes: nearest-rank p95 over N wall-clock reps in one process — a
regression guard, not a production SLO measurement. The seeding benchmark
builds its 200 entries through the real entry/check-in service path (mirroring
`internal/web`'s `seededMeetFixture`), then times one `GenerateHeats` call.

## Deliverable 2 — SYS-130 two-minute recovery drill

`internal/web/recovery_test.go` `TestServeRecoverySYS130` (build tag `perf`).
Prior art it builds on: `internal/store` `TestKill9Durability` (TASK-003) and
`internal/app` `TestCaptureCrashRecoverySYS081UC020_2` (TASK-014) both prove
kill-9 durability up to "the database reopens". This drill closes the remaining
SYS-130 gap — "the SYSTEM is operational again" — at the HTTP layer:

1. Child OS process runs the real `web.Server` (full route/middleware stack,
   TLS off — orthogonal to recovery) on a real listener; bootstraps an admin
   over `POST /setup`, creates a UKC meet, registers 42 athletes, then loops
   real capture form-POSTs, printing `confirmed <athlete> <mark>` only after
   each HTTP 303 (durable under `synchronous=FULL`).
2. Parent SIGKILLs the child mid-burst after 25 confirmations.
3. Parent starts a fresh server on the **same** on-disk database and measures
   time to a 200 from `/healthz`, then logs in and verifies every acknowledged
   save via `UnitCapture`.

Measured 2026-07-13 (confined, 3 consecutive pre-merge runs + 1 post-merge run,
all green):

| Assertion | Budget | Measured | Verdict |
|---|---|---|---|
| Operational (HTTP 200) after restart | ≤2 min | **~23 ms** (22.5–22.8 ms across runs) | PASS |
| Acknowledged-data loss | 0 | **0** — all 25 confirmed (athlete, mark) pairs present exactly | PASS |
| Post-restart login | works | admin session re-established | PASS |

Caveat: the drill measures process restart + WAL replay + consistency check +
listener readiness. It does NOT measure host reboot or a process supervisor's
restart delay — SYS-130's "automatic restart" half is a deployment concern
(systemd unit / container restart policy, TASK-028 packaging); the budget is
asserted from the moment the replacement process starts, per SYS-130's "within
2 minutes **of host availability**".

## Deliverable 3 — SYS-105 egress-blocked public-asset check

`internal/web/egress_test.go` `TestPublicSurfaceRendersWithEgressBlockedSYS105`
— **default gate** (no build tag): fast and deterministic.

Approach chosen: a Go test intercepting outbound dials, NOT a Playwright
route-block. Rationale (per the task brief's either/or): the poisoned
`http.DefaultTransport` proves the **server process** performs no non-loopback
egress while rendering (a future server-side telemetry call would trip it),
which a browser-side route-block structurally cannot see; it adds zero tooling
and zero e2e flake exposure. The browser's view is covered by the complementary
static scan: every crawled page's rendered HTML is asserted free of absolute
off-origin `href`/`src`/`action` references — exactly what a real browser would
have fetched.

Executed 2026-07-13 (and in every default-gate run since): PASS —
- the TASK-029 link-following crawler reaches all five public page shapes
  (overview, timetable, start lists, results, `results/live` fragment) with
  non-empty bodies under the dial block;
- `/static/htmx.min.js`, `/static/base.css`, `/static/public-live.js` all serve
  200 from the embedded FS (ADR-003: no CDN);
- zero external resource references in any rendered public page;
- zero non-loopback dial attempts.

## Deliverable 4 — SYS-122 / UC-017 #4 2,000-concurrent-viewer load test

`internal/web/load_test.go` (build tag `perf`): goroutine-based clients (no
new external tool — justification in the file header) against the real
`web.Server` over loopback TCP, backed by the same SYS-120 reference corpus.
Two tests: `TestSYS122ViewerScalingProfile` (100/250/500 viewers, memory- and
latency-instrumented) and `TestSYS122TwoThousandConcurrentViewers` (the
literal budget assertion).

The original 2026-07-13 measurement recorded this budget as **unmeetable on
the then-current read architecture** (evidence preserved in OQ-066's history
and the OOM incident above) and named the fix: a pooled WAL read-connection
set plus a per-meet render cache for the public results page/fragment,
invalidated by the existing results-changed bus event. **DEC-015/TASK-035
implemented that fix** (`store.Store.ReadDB()`, `internal/web/publiccache.go`
— see ADR-004 §9); it is now measurably met with wide margin.

Measured 2026-08-03 (confined scope, 12 GiB cap / GOMEMLIMIT=10GiB, post
TASK-035):

| Concurrent viewers | p95 page render | Errors | SSE delivery p95 | Peak heap (process) | Per-viewer |
|---|---|---|---|---|---|
| 100 (+5 SSE) | 185.8 ms | 0 | 39.0 ms | 85.0 MiB | 0.85 MiB |
| 250 (+13 SSE) | 210.9 ms | 0 | 11.3 ms | 91.1 MiB | 0.36 MiB |
| 500 (+25 SSE) | 251.0 ms | 0 | 11.8 ms | 103.9 MiB | 0.21 MiB |
| 2,000 (+100 SSE), literal SYS-122 assertion | 343–377 ms (two consecutive runs) | 0 | ~12 ms | 179–184 MiB | ~0.09 MiB |

**Verdict vs SYS-122: PASS**, roughly 8–9× inside the 3 s p95 budget at the
full 2,000-viewer scale, 0 errors at every scale (budget ≤0.1%).
`TestSYS122TwoThousandConcurrentViewers` is green — the assertion text and
budget constants are unchanged from the original (never weakened); only the
server-side architecture changed.

- Peak heap is now **flat, not linear, in viewer count** (85–184 MiB across a
  20× range of viewers): the per-meet render cache turns "one render per
  viewer" into "one render per result change" (one query + one template
  render per cache build, shared read-only by every concurrent viewer of
  that meet+locale), so the old ~64 MiB/viewer in-flight cost is gone —
  what remains scales with concurrent HTTP connections, not with rendering.
- p95 render is now dominated by ordinary per-request overhead (CSRF cookie
  issuance, connection handling), not queueing or recomputation — flat
  across 100→2,000 viewers (186–377 ms) rather than the old linear queueing
  growth (~69 ms per queued request behind the single connection).
- **SYS-071 under load still PASSES**, and improved further: SSE delivery
  p95 ~12 ms at 100 subscribers under full 2,000-viewer pressure (budget
  ≤10 s; was 1.9 s pre-fix) — `web.Bus` was never the bottleneck and is
  unchanged by this amendment.
- A caution from the implementation, kept here for future maintainers of
  this cache: an earlier version of the fix cached the rendered fragment as
  `[]byte` and converted it to a `string` per request for `templ.Raw`/
  `io.WriteString`; that conversion copies, which silently reintroduced an
  O(viewers) allocation of the whole rendered page and OOM-killed the
  2,000-viewer run within 5 seconds (12 GiB cap, confirmed via
  `journalctl --user`: `oom-kill`, 12G memory peak). Caching the fragment as
  an immutable Go `string` (shared by reference, not copied) fixed it — see
  `publicResultsCacheEntry`'s doc comment in `internal/web/publiccache.go`.

Related note for OQ-077 (no in-flight feedback on long operations): every
OPERATOR-facing operation measured in this task sits at ≤76 ms p95 at reference
scale, and the public read path is now also inside its own budget with wide
margin — nothing measured in this task needs a progress affordance. No change
to OQ-077's risk assessment.
