# ADR-004 — Storage, durability & venue↔hub sync

**Status:** **Accepted** (v2 — §4–5 deferred with the venue role per DEC-013; §8 added) — ratified by the founder 2026-07-05 (one-way door: data model & sync semantics)
**Date:** 2026-07-05
**Traces:** SYS-081–087, SYS-046, SYS-080 *(L)*, SYS-082, SYS-101/102, STR-041; depends on ADR-002 (v2)/003

## Context

A meet cannot be re-run: confirmed writes must survive crash and power loss (SYS-081), backup
must be one action and one artifact (SYS-084), ≥10 concurrent operators must not lose or
silently overwrite each other's work (SYS-083), and public publications produced offline must
catch up automatically (SYS-082). The incumbent's fragility (fresh database file per meet,
48-hour auto-locks, silent live-publishing failures — C1.3) is a direct anti-pattern list.

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

## Alternatives considered

| Option | Why rejected |
|--------|--------------|
| Client/server RDBMS (PostgreSQL) | An extra service to install/administer on a volunteer laptop violates SYS-131/133; Postgres remains a possible hub-side option post-MVP if multi-tenancy demands it (kept open by using portable SQL where practical) |
| CRDT/multi-master sync (venue and hub both writable) | Massive complexity for a non-requirement: the competition office is the single authority during a meet (D11); one-way flows match reality |
| Event-sourcing as the primary store | Overkill: we need an audit trail, not full event replay; the append-only audit log gives SYS-046 without rebuilding state from events |
| Per-meet database files (TAF3 model) | The documented incumbent failure mode (C1.3): wrong-file selection, overwrites, no cross-meet athlete history (breaks PB/SB per SYS-049) |

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
