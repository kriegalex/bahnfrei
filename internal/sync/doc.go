// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

// Package sync documents the offline-capture wire protocol and (later) hosts
// the deferred publication-queue dispatcher/applier.
//
// # Publication queue (deferred)
//
// The publication-queue dispatcher (venue) and applier (hub) plus the
// entries-snapshot pull (ADR-004 §4/§5; SYS-082) are deferred pending the
// ADR-002 v2 §3 evidence gate for the venue-node role.
//
// # Offline capture queue — wire protocol (TASK-009, UC-034; SYS-085/086; ADR-004 §8)
//
// The offline capture queue (ADR-004 §8) is the sole intentionally-offline
// write path in the MVP. A field official's device checks out one event unit,
// captures attempts into a durable, ULID-keyed local queue while
// disconnected, and replays that queue idempotently on reconnection. The
// SERVER half lives in internal/app (ResultsService checkout + replay +
// reconciliation) and internal/web (the two JSON endpoints below). The CLIENT
// island (durable queue, offline indicator, service worker) is slice 2 and is
// built against exactly this contract. This is the app's ONE JSON API; every
// other surface is server-rendered HTML + HTMX (ADR-003), because an ordered,
// per-op-acknowledged, idempotent batch cannot be expressed as a form post.
//
// ## Checkout model (SYS-086)
//
// At most one active checkout holds a unit. A checkout carries:
//   - generation: a per-unit monotonically increasing counter. An office
//     override/reassign, or a device re-taking a released lock, bumps it. A
//     device holding an older generation is provably stale.
//   - startListVersion: bumped when the office revises the unit's start list.
//   - token: an opaque holder credential (ULID), reissued on every
//     reassign/override.
//
// The device caches {token, generation, startListVersion} and stamps every
// queued op's batch with it, resending the whole stamp verbatim. The TOKEN is
// the authorization (ADR-004 §8: replays are accepted only from the unit's
// current checkout holder): when a live checkout exists, a batch whose token
// does not match it routes to reconciliation as stale_checkout regardless of
// the generation it claims — a client cannot write as the holder by
// misreporting its generation. The generation is the staleness/audit signal;
// the start-list version detects office-side start-list changes.
//
// ## Endpoints
//
// All endpoints are field-official gated (SYS-090) and require the
// double-submit CSRF token in the X-CSRF-Token header (SYS-092).
//
//	POST /meets/{id}/capture/{unit}/checkout
//	  Request:  {"deviceLabel": "tablet-A"}          (deviceLabel optional; defaults "web")
//	  Response: {"token": "01H…", "generation": 1, "startListVersion": 0}
//	  409 {"error":"checked_out_by_another"} if another device actively holds
//	  the lock — the office must override.
//
//	POST /meets/{id}/capture/{unit}/sync
//	  Request:
//	    {
//	      "token": "01H…",
//	      "deviceLabel": "tablet-A",
//	      "generation": 1,
//	      "startListVersion": 0,
//	      "ops": [
//	        {"opId":"01H…","athleteId":"01H…","seq":1,"value":"6.12","wind":"1.2"},
//	        {"opId":"01H…","athleteId":"01H…","seq":2,"value":"X"}
//	      ]
//	    }
//	  Response:
//	    {"results":[
//	       {"opId":"01H…","status":"applied"},
//	       {"opId":"01H…","status":"duplicate"},
//	       {"opId":"01H…","status":"reconciliation","reason":"stale_checkout"}
//	    ]}
//
// Field notes:
//   - opId is a CLIENT-generated ULID and the idempotency key (SYS-085). The
//     client MUST reuse the same opId across retries of the same capture, and
//     MUST NOT reuse an opId for a different capture or a different unit: op
//     ids are globally unique. A batch containing an opId the server already
//     recorded for ANOTHER unit fails with HTTP 400 (a client bug — the
//     capture is not acknowledged, not applied, and not a "duplicate";
//     regenerate the ULID and resubmit). Acking it as duplicate would
//     silently drop the capture, the loss mode SYS-086 forbids.
//   - token must be the live checkout's credential; see "Checkout model".
//   - value uses the same grid notation as the online capture form: a mark in
//     metres ("6.12"), or the D5.2 symbols X (foul), – or - (pass), r
//     (retirement). wind is optional and only meaningful where the discipline
//     is wind-relevant.
//   - ops are applied strictly in submission order.
//
// ## Per-op status vocabulary (in the response)
//
//   - "applied":        the op was applied to the unit on this call.
//   - "duplicate":      the op was already applied by an earlier batch; a
//                       flaky reconnect re-sent it and no second write was
//                       made (exactly-once, SYS-085).
//   - "reconciliation": the op could not be applied and was queued for the
//                       office reconciliation view (never silently discarded,
//                       SYS-086). "reason" is one of:
//                         - "stale_checkout":     the batch's token is not
//                           the live checkout's credential, its generation is
//                           superseded, or no live checkout holds the unit
//                           (e.g. after an office override, UC-034 #5).
//                         - "start_list_change":  the batch's startListVersion
//                           is behind the unit's — the office revised the
//                           start list after checkout (UC-034 #4).
//                         - "conflict":           applying would overwrite a
//                           diverging attempt captured meanwhile; surfaced,
//                           not merged.
//
// ## Guarantees
//
//   - Exactly-once: a terminal decision per opId is recorded in a dedupe
//     ledger; repeated/interleaved batches never double-apply.
//   - Never silent discard: an unappliable op always becomes a pending
//     reconciliation item the office applies or explicitly (audited) discards
//     (SYS-086; the documented Web.TEC 2 loss mode C2.1 is the named
//     anti-pattern).
//   - Shared write path: applied ops go through the same TASK-008 capture
//     service (validation, standings recompute, SSE notify) — no second write
//     path.
package sync
