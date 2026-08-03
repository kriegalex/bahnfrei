# ADR-004 — Storage, durability & venue↔hub sync

**Status:** **Accepted** (v3 — §4–5 deferred with the venue role per DEC-013; §8 added; §9 added, public read path per DEC-015/TASK-035) — ratified by the founder 2026-07-05, §9 by DEC-015 (one-way door: data model & sync semantics)
**Date:** 2026-07-05
**Traces:** SYS-081–087, SYS-046, SYS-080 *(L)*, SYS-082, SYS-101/102, SYS-071, SYS-122, STR-041; depends on ADR-002 (v2)/003

## Context

A meet cannot be re-run: confirmed writes must survive crash and power loss (SYS-081), backup
must be one action and one artifact (SYS-084), ≥10 concurrent operators must not lose or
silently overwrite each other's work (SYS-083), and public publications produced offline must
catch up automatically (SYS-082). The incumbent's fragility (fresh database file per meet,
48-hour auto-locks, silent live-publishing failures — C1.3) is a direct anti-pattern list.

The read side has its own scale requirement, orthogonal to the write path above: a published
meet's results page must serve up to 2,000 concurrent public spectators at ≤3s p95 with ≤0.1%
errors (SYS-122, UC-017 #4). §1's single connection was deliberately scoped to the write path
(§2's single-writer invariant), but until TASK-035 it was also, incidentally, the *only*
connection — every public read convoyed behind it too. §9 below closes that gap without
touching the write-path decisions above.

## Decision

1. **One SQLite database per instance** (not per meet), WAL mode, `synchronous=FULL`;
   every user-visible confirmation returns only after the transaction is durable (SYS-081).
   Recovery = SQLite's standard crash recovery; a startup consistency check (foreign keys,
   invariant queries) reports green before the UI accepts writes (UC-020).
2. **Single-writer concurrency model:** all writes go through the one server process, using
   short transactions; entity-level **optimistic versioning** (a `version` column checked on
   update) turns same-entity races into surfaced conflicts, never last-write-wins (SYS-083).
3. **Append-only audit log** as a first-class table (actor, entity, before/after JSON, reason,
   timestamp, monotonic sequence). No UPDATE/DELETE on it (enforced by triggers); corrections
   are new rows (SYS-046). Erasure requests (SYS-101) pseudonymize referenced personal fields
   via a documented redaction routine rather than row deletion, preserving sequence integrity.
4. **Publication queue (venue → hub)** *(deferred with the venue role — DEC-013/ADR-002 v2)*: publishing an artifact (start list, result-list
   version, timetable change) appends an ordered, idempotent publication record (ULID-keyed).
   A background dispatcher POSTs batches to the hub over HTTPS with meet-scoped bearer
   credentials; the hub applies them idempotently in order. Offline simply means the queue
   grows; reconnection drains it with no operator action (SYS-082). Backlog state is visible
   in the operator UI.
5. **Entries snapshot (hub → venue)** *(deferred with the venue role — DEC-013/ADR-002 v2)*: before/at meet start the venue node pulls a complete,
   versioned entries snapshot from the hub (or imports the same format from a file). After the
   venue takes ownership of a meet, the hub becomes read-only for that meet's competition data;
   late online changes are rejected with a pointer to the competition office (single source of
   truth — no multi-master).
6. **Backup:** SQLite Online Backup API produces one snapshot file on demand or scheduled;
   restore = start binary pointing at the snapshot (SYS-084). The pre-meet quickstart tells the
   operator to schedule backups to a second medium (USB stick) — a checklist item, not code.
7. **Retention:** configurable purge jobs implement SYS-102 over the same schema.
8. **Offline capture queue (operator device → server) — MVP (DEC-013; SYS-085/086, UC-034):**
   checking out an event unit caches its start list and capture UI on the device (service
   worker + durable browser storage); attempts append to an ordered, ULID-keyed local queue
   that replays idempotently on reconnection. The server accepts replays only from the unit's
   current checkout holder; anything else (stale device, office-side start-list change) routes
   to a reconciliation view — never silent discard (the documented Web.TEC 2 loss mode,
   C2.1/C8, is the named anti-pattern). Scope is deliberately bounded to one checked-out unit
   per device: a client-side write buffer with single-owner semantics, not multi-master sync.
9. **Public read path (DEC-015/TASK-035; SYS-122, SYS-071, UC-017 #4, OQ-066):** the §1
   connection is now writer-only, not the sole connection — §2's single-writer invariant is
   unchanged, it is simply named precisely. Two additions, orthogonal to each other and to
   every write-path decision above:
   - **Pooled WAL read-connection set** (`store.Store.ReadDB()`, `internal/store`): a second
     `*sql.DB` against the same file, `query_only`-pragma'd, pool size 16. SQLite's WAL mode
     lets any number of readers run concurrently with the one writer without contending on it,
     so this removes queueing for read-only public-surface queries (`MeetService.Meet`,
     `ResultsService.Standings`/`Participants`/`ClubNamesFor`) without weakening §2 — no write
     path was moved onto it, and the `query_only` pragma makes an accidental write on it fail
     loudly rather than race the writer connection.
   - **Per-meet, per-locale public-results render cache** (`internal/web`, `publiccache.go`):
     the public results page/fragment's view model (`ResultsService.Standings`'s output plus
     the meet header) AND its rendered results-fragment HTML are built once and reused across
     every concurrent viewer of the same meet+locale, instead of recomputed/re-rendered per
     request. A pooled read connection alone is *necessary but not sufficient* at 2,000
     simultaneous renders — profiling showed even a cached view model still re-walked into HTML
     on every request missed the 3s budget (the athletes×disciplines row/cell walk is
     non-trivial at the SYS-120 reference scale); caching the rendered fragment too is what
     collapses "one render per viewer" to "one render per result change." The cache stores the
     fragment as a Go `string` (immutable) rather than `[]byte` specifically so every concurrent
     request shares the same backing bytes with zero copies — the first implementation cached
     `[]byte` and converted it to a `string` per request, which silently reintroduced an
     O(viewers) allocation of the whole rendered page and OOM-killed the 2,000-viewer run; this
     is recorded as a caution for future work on this cache, not a hedge — see the
     `publicResultsCacheEntry` doc comment. Only the fragment is cached, never the full page:
     the page shell (navigation, login state, CSRF token, all in `layout.templ`) still renders
     fresh per request from the caller's own session and splices the cached fragment in via
     `templ.Raw` (`publicResultsPage`'s `resultsSection templ.Component` parameter), so a
     cached entry can never leak one viewer's session state into another's response.
     Invalidation is the existing results-changed bus event (`ResultsService.OnResultsChanged`,
     extended to also fire on SYS-103 consent changes — previously a gap), the same event that
     already drives the SSE "results" push, plus a 5s TTL as defense-in-depth for the small
     number of public-surface-affecting writes not wired to that hook (meet-metadata edits,
     SYS-101/102 erasure/retention purge — tracked as OQ-094, a candidate follow-up is
     extending the hook to those paths too). SSE delivery itself is untouched: it already met
     its budget under load (TASK-027 measured 1.9s p95 at 100 subscribers against a 10s budget)
     and this amendment does not change `web.Bus` or the `sseHandler`.

## Alternatives considered

| Option | Why rejected |
|--------|--------------|
| Client/server RDBMS (PostgreSQL) | An extra service to install/administer on a volunteer laptop violates SYS-131/133; Postgres remains a possible hub-side option post-MVP if multi-tenancy demands it (kept open by using portable SQL where practical) |
| CRDT/multi-master sync (venue and hub both writable) | Massive complexity for a non-requirement: the competition office is the single authority during a meet (D11); one-way flows match reality |
| Event-sourcing as the primary store | Overkill: we need an audit trail, not full event replay; the append-only audit log gives SYS-046 without rebuilding state from events |
| Per-meet database files (TAF3 model) | The documented incumbent failure mode (C1.3): wrong-file selection, overwrites, no cross-meet athlete history (breaks PB/SB per SYS-049) |
| §9: cache the whole rendered page, not just the results fragment | Rejected: `internal/web/layout.templ` embeds a per-request CSRF token and session-derived nav state in every page (including public ones); caching full-page bytes would serve one viewer's CSRF token and login chrome to every other cached-hit viewer. Caching only the results fragment — the part with no session dependency — and splicing it into a freshly rendered page shell (`publicResultsPage`'s `resultsSection templ.Component` parameter, `templ.Raw`) keeps the CSRF/session boundary intact while still caching the expensive part |
| §9: reverse proxy / CDN in front of the binary | Deferred, not rejected — SYS-131/133 target a self-contained single binary with no required extra service; a documented reverse-proxy option remains open per the OQ-066 discussion if a deployment's own scale ever exceeds this amendment's numbers |

## Consequences

- Durability and backup semantics are testable exactly as UC-020 specifies (kill -9 / VM-kill
  fixtures; snapshot-restore checksum comparison).
- Single-writer + optimistic versioning gives simple, explainable conflict behaviour (UC-021)
  at laptop-scale load (SYS-120 is far below SQLite's write ceiling).
- The one-way sync model is easy to reason about and audit, but means online entry changes
  freeze once the venue takes over — a rule the UI must communicate clearly (competition
  office handles late changes per SYS-016).
- ULIDs as entity/publication IDs give sortable, collision-free identifiers across
  venue/hub without coordination.
- Hub-first (DEC-013) makes the hub the primary store from day one; §4–5 activate only when
  the venue role is built (evidence gate, ADR-002 v2 §3). The client capture queue (§8) is the
  sole intentionally-offline write path in MVP, and its single-owner checkout semantics keep
  it out of multi-master territory.
- §9 makes SYS-122's 2,000-concurrent-viewer budget measurably met (`TestSYS122
  TwoThousandConcurrentViewers`; numbers in `docs/delivery/perf-and-recovery-task-027.md`)
  without touching §1–8: the writer connection, its cap of one, and every write path through it
  are byte-for-byte unchanged.
- The TTL-based staleness ceiling §9 falls back to for writes outside the invalidation hook
  (meet-metadata edits, erasure/retention purge) is a deliberate, bounded trade-off, not an
  oversight — see OQ-094. It applies only to the public results page's cached view; every
  operator-facing read (capture, standings, roster) still reads live, uncached data.
