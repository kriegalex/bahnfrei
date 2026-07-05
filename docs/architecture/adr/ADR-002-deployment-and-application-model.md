# ADR-002 — Deployment & application model: hub-first web app in a single self-contained executable

**Status:** **Accepted** (v2 — hub-first re-scope per DEC-013) — ratified by the founder 2026-07-05 (one-way door)
**Date:** 2026-07-05 (v2 same day, after industry research C8 and founder direction DEC-013)
**Traces:** SYS-080 *(L)*, SYS-081–087, SYS-090, SYS-093, SYS-011, SYS-070/071, SYS-131–133, DEC-006, DEC-013, OQ-006

## Context

The founder asked explicitly whether "a desktop offline app is even possible and matches the
constraints" (DEC-006). The requirements pull in three directions at once:

- **Venue mode** must run fully offline on one volunteer laptop and serve **multiple concurrent
  operators** (SYS-080, SYS-083) — check-in on a phone at the call room, field capture on a
  tablet at the long-jump pit, the competition office on the laptop.
- **Online entries and public live results** need an internet-reachable instance
  (SYS-011, SYS-070/071) — founder homelab first, cheap cloud later.
- **Volunteer operability**: install-to-running in ≤30 minutes without editing config files
  (SYS-131), browser-only clients (SYS-132), no paid services (SYS-133).

A classic single-user desktop GUI app (the TAF3 model) fails SYS-083: it cannot give the
infield tablet and the call-room phone concurrent access. A third-party-hosted SaaS fails
self-hosting and zero recurring cost (SYS-133, DEC-013).

**Re-scope 2026-07-05 (DEC-013).** Industry research (competitive-analysis **C8**) showed that
at our tier nobody builds field-wide venue Wi-Fi: club-tier incumbents (Seltec Web.TEC 2 via
`tec2.laportal.net`, Atletiek.nu's jury app, Roster) all run field capture on officials' own
phones over mobile data through a cloud relay; dedicated venue networks exist only at the
professional tier with paid technicians. The founder set the connectivity posture accordingly:
*expect connectivity to a central self-hosted hub; tolerate LTE/Wi-Fi interruptions everywhere
(timing room and infield) without stopping; keep deployment cheap for small clubs; no
subscription-SaaS business — clubs host the stack themselves.* Consequently SYS-080 (full
no-internet venue operation) moved to *Later* and SYS-085–087 (interruption tolerance,
event-unit checkout, graceful degradation) were added as MVP.

## Decision

**One product, one binary, two instance roles — delivered hub-first (DEC-013).** The system is
a **web application whose server is a single self-contained executable** (no runtime to
install, assets embedded). The two roles remain the target architecture; their delivery is
phased:

1. **Hub node — MVP, the primary role.** A self-hosted instance (founder homelab first; any
   club's VPS or homelab later) behind TLS. It hosts the full meet lifecycle: online entries
   before the meet, **all competition-day operation** (office UI, check-in, seeding, capture,
   corrections, printing), and public live results/archive. Operator devices — office laptop,
   call-room phone, infield phones — reach it over whatever transport exists: a phone hotspot
   at the admin table, venue Wi-Fi where it happens to exist, or each device's own mobile
   data. This matches verified club-tier industry practice (C8).
2. **Interruption tolerance is the MVP offline story (SYS-085–087, UC-034).** Field capture
   uses event-unit checkout with a durable client-side queue: capture continues through
   connectivity loss and replays automatically and idempotently on reconnection ("walk-by
   sync"); office surfaces degrade gracefully and reconnect without restart. **Field-wide
   venue Wi-Fi is an explicit non-goal**; the minimum network kit for a meet is one phone
   hotspot at the admin table.
3. **Venue node — deferred, not deleted (evidence-gated).** The same binary can later run as a
   venue-local server for full no-internet operation (SYS-080, now *Later*; UC-019), connected
   to the hub by the one-way, meet-scoped flows specified in ADR-004 §4–5 (deferred with it):
   hub → venue entries snapshot; venue → hub ordered publication queue (SYS-082). Build
   triggers: a season of hub-first meets producing evidence that connectivity failures
   materially disrupted operation (indoor halls, cell congestion at larger meets), or a
   committed target meet that cannot assume any uplink. Two guardrails keep the retrofit
   additive: the data model stays meet-scoped (ADR-004), and the timing exchange runs as a
   small local agent on the timing PC talking to a server URL — hub today, venue node later
   (ADR-006), so photo-finish works identically in both phases.
4. Once both roles exist, a meet can run hub-only (the MVP norm), venue-only (no hub: printed
   + LAN-visible results), or venue + hub (big meets).

## Alternatives considered

| Option | Why rejected |
|--------|--------------|
| Native/Electron desktop GUI app | Single-operator by construction (SYS-083 fails); Electron adds ~200 MB and a chromium security treadmill; still needs a sync story for public results |
| Cloud-only SaaS webapp | Fails offline venue operation (SYS-080); recurring cost conflicts with zero-IT-budget clubs (DEC-002) |
| Two separate products (desktop capture tool + web portal) | Double maintenance, schema drift, the exact fragmentation we criticize incumbents for (C2/C6) |
| Browser-local PWA with in-browser storage (no server) | Multi-operator LAN use requires a server anyway; browser storage eviction risks confirmed-result durability (SYS-081) |

## Consequences

- Multi-operator capability comes free on any device with a browser (SYS-083). For a meet
  organizer, "install" is now *zero* — open the hub's URL; the ≤30-minute quickstart target
  (SYS-131, UC-001) applies to self-hosting the hub: one binary or one container, TLS via
  built-in ACME.
- MVP always speaks TLS (hub role, SYS-093). The venue-LAN plain-HTTP trade-off of v1 is
  deferred along with the venue node and returns only at that gate (UC-019 #4).
- **The hub becomes the meet-day single point of failure — accepted for MVP, with named
  mitigations:** interruption tolerance (SYS-085–087) absorbs transport blips; printed capture
  sheets (SYS-072) remain the documented fallback; disaster recovery is restore-anywhere from
  the one-file backup (SYS-084). The founder carries meet-day ops only for instances he hosts;
  a self-hosting club is its own operator and remains the nFADP/GDPR controller of its
  athletes' data — the project runs **no subscription-SaaS business** (DEC-013).
- Swiss-chain position (C2.1): hub-first covers the youth-series/school/club tier end-to-end
  today via the official non-TAF3 upload path (SYS-077/UC-035); sanctioned-meet coexistence
  goes through TAF3-compatible exports (SYS-078/UC-036, *Later*) until the Alabus seam opens
  (OQ-014).
- The same binary in two roles keeps homelab operation trivial (one systemd unit or container)
  and makes "cloud later, modest budget" a VPS-sized problem, not a re-architecture (DEC-006).
- Multi-tenant SaaS (Later, StRS §4.2) becomes "many hub instances" or a tenancy layer on the
  hub role — deferred, not precluded, and explicitly not a subscription business run by the
  project (DEC-013).

## Annex A — Deployment-model comparison matrix (ratification support, 2026-07-05)

**A** = local desktop GUI only (the TAF3 model) · **B** = proposed: one binary, venue + hub
roles · **C** = traditional cloud SaaS.

| Criterion (requirement) | A — Local GUI only | B — Proposed local-first binary | C — Traditional SaaS |
|---|---|---|---|
| Offline venue operation (SYS-080) | ✅ inherently offline | ✅ venue node fully offline from cold start | ❌ hard dependency on venue internet mid-meet |
| Multiple concurrent operators (SYS-083) | ❌ one screen = one operator; extra stations need extra installs + a shared-file DB nightmare | ✅ any phone/tablet/laptop with a browser on the LAN | ✅ (only while online) |
| Online entries pre-meet (SYS-011) | ❌ needs a second, separate web product | ✅ hub role | ✅ core strength |
| Public live results (SYS-070/071) | ⚠️ export-and-upload dance; TAF3's silent live-publish failures live here | ✅ ordered publication queue; offline just delays, never loses | ✅ instant while venue is online; ❌ blind when it isn't |
| Install ≤30 min, no config editing (SYS-131) | ✅ per-OS installer | ✅ one file, double-click | ✅ nothing to install (but org/account onboarding) |
| Browser-only client devices (SYS-132) | ❌ typically Windows-only operator machine | ✅ | ✅ |
| Zero recurring cost (SYS-133, DEC-002) | ✅ | ✅ hub optional; homelab or ~€5 VPS | ❌ someone pays hosting + ops monthly, forever |
| Result durability / failure modes | ⚠️ loose per-meet database files (TAF3's documented loss mode) | ✅ single WAL DB, confirmed writes, audit log (ADR-004) | ✅ good in the cloud — but a venue outage mid-meet stops *capture*, not just publishing |
| Sync complexity | High and manual (export/import by humans) | Moderate, one-way, meet-scoped (ADR-004) | None — only because offline is unsupported |
| Security / privacy surface | Small; data stays on the laptop | LAN-HTTP trade-off at venue + TLS hub; club self-hosts, stays data controller | Largest: public multi-tenant surface, account system; operator becomes a GDPR/nFADP **processor** needing DPAs with every club |
| Decade-scale maintainability | ⚠️ GUI-toolkit churn, per-OS build matrix | ✅ one codebase, boring server-rendered web | ⚠️ continuous ops; the product dies when the operator stops paying |
| Path to multi-tenant SaaS later | ❌ re-architecture | ✅ many hubs, or a tenancy layer on the hub role | ✅ native |
| **Requirements verdict** | **Fails SYS-011, SYS-070/071, SYS-083** | **Meets all** | **Fails SYS-080, SYS-133** |

Reading: A and C each hard-fail requirements on opposite sides — A can't do the online half,
C can't do the offline half. B is not a compromise between them but a superset: A's offline
venue plus C's online reach, at the cost of the (bounded, one-way) sync in ADR-004.
