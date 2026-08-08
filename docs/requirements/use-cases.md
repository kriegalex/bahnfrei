# Use-Cases (UC-###) — Executable Acceptance Criteria

**Document status:** DRAFT — Phase A baseline
**Purpose:** the agent-facing layer. Each UC is a vertical slice: independently buildable,
independently verifiable, traced **up** to `SYS-###`/`STR-###` and, in Phase B, **down** to
automated tests. Acceptance criteria are Given/When/Then and are automatable by default;
inspection- or demonstration-verified criteria are permitted where the traceability matrix
records that verification method (D/I — e.g. UC-026 #2, UC-038 #3).

> Conventions: *operator* = authenticated user with the named role (SYS-090). Marks use
> athletics notation (`10.85` s, `6.42` m, `1.83` m). Priorities: **MVP** / **Later**.
> Phase B: a UC is *done* only when every criterion has a passing automated test and the
> traceability matrix links them.

## Index

| UC | Title | Priority | Traces up (primary) |
|----|-------|----------|---------------------|
| UC-001 | Install and create a meet | MVP | SYS-131, SYS-001, SYS-002, SYS-004, SYS-006 |
| UC-002 | Category schemes & discipline catalog | MVP | SYS-003, SYS-005 |
| UC-003 | Online entries (individual, club, relay) | MVP | SYS-011, SYS-012, SYS-015 |
| UC-004 | Entry import from file | MVP | SYS-013, SYS-010 |
| UC-005 | Eligibility validation | MVP | SYS-014 |
| UC-006 | Bib assignment & fee summary | MVP | SYS-018, SYS-017 |
| UC-007 | Check-in and DNS handling | MVP | SYS-025 |
| UC-008 | Heat seeding & lane draws | MVP | SYS-026, SYS-027, SYS-028 |
| UC-009 | Round progression | MVP | SYS-029, SYS-030 |
| UC-010 | Track result capture | MVP | SYS-040, SYS-041, SYS-045, SYS-050 |
| UC-011 | Horizontal field event capture | MVP | SYS-042 |
| UC-012 | Vertical jump capture | MVP | SYS-043 |
| UC-013 | Combined events | MVP | SYS-031, SYS-044 |
| UC-014 | Timing system exchange (FinishLynx) | MVP | SYS-060, SYS-061, SYS-062 |
| UC-015 | Result correction, audit & protest clock | MVP | SYS-046, SYS-047 |
| UC-016 | Records & bests flagging | MVP | SYS-049, SYS-050, SYS-051 |
| UC-017 | Public live results & archive | MVP | SYS-070, SYS-071, SYS-074, SYS-122 |
| UC-018 | Printable official documents | MVP | SYS-072 |
| UC-019 | Offline venue operation (venue-node role) | Later *(re-scoped, DEC-013)* | SYS-080, SYS-082, SYS-093 |
| UC-020 | Durability, backup & restore | MVP | SYS-081, SYS-084, SYS-130 |
| UC-021 | Multi-operator concurrency | MVP | SYS-083 |
| UC-022 | Accounts, roles & audit of privileged actions | MVP | SYS-090, SYS-091 |
| UC-023 | Public data minimization & consent enforcement | MVP | SYS-100, SYS-103 |
| UC-024 | Data-subject rights & retention | MVP | SYS-101, SYS-102 |
| UC-025 | Multilingual surfaces | MVP | SYS-110, SYS-111 |
| UC-026 | Accessibility of public surfaces | MVP | SYS-112, SYS-113 |
| UC-027 | Result export & federation delivery | MVP | SYS-073 |
| UC-028 | Mixed-category fields (para classes: Later) | MVP | SYS-052, SYS-010 (D9.2, DEC-007) |
| UC-029 | Team/club scoring | Later | SYS-048 |
| UC-030 | Sponsor surfaces | Later | SYS-075 |
| UC-031 | Scoreboard live feed | Later | SYS-064 |
| UC-032 | EDM field-measurement intake | Later | SYS-063 |
| UC-033 | UBS Kids Cup meet from template | MVP | SYS-053, SYS-052 |
| UC-034 | Offline-tolerant field capture & walk-by sync | MVP | SYS-085, SYS-086, SYS-087 |
| UC-035 | Swiss youth-series results upload file | MVP | SYS-077 |
| UC-036 | TAF3 coexistence exports | Later | SYS-078 |
| UC-037 | Contextual input help | MVP | SYS-115 |
| UC-038 | Design-system conformance & usability audit | MVP | SYS-116, SYS-117 |

Cross-cutting quality requirements (SYS-140…146, SYS-120/121, SYS-114, SYS-104/105,
SYS-132/133) are verified by CI gates, benchmarks, and inspection — see
`traceability-matrix.md`; they do not form separate UCs.

---

## UC-001 Install and create a meet — MVP

**Actors:** operator (organizer), fresh host machine.
**Traces:** SYS-131, SYS-001, SYS-002, SYS-004, SYS-006 → STR-001, STR-002, STR-003, STR-004, STR-039.

1. **Given** a supported machine without the system installed, **when** a tester follows the
   quickstart verbatim, **then** a running system with an admin account exists in ≤30 minutes
   wall-clock and no configuration file was hand-edited.
2. **Given** an organizer account, **when** they create a meet with name, venue, two competition
   days, two sessions/day, and tier `C-Meeting`, **then** the meet appears in status `draft` with
   exactly those attributes retrievable.
3. **Given** a draft meet, **when** the organizer adds events `100m / U16 W`, `Shot Put / U16 W`,
   and `4×100m / U16 W` each with round structure and entry deadline, **then** the event
   programme lists all three with discipline-correct capture types (track / horizontal field /
   relay) and deadlines.
4. **Given** a meet with scheduled units, **when** the organizer publishes the timetable, then
   amends one unit's time, **then** both versions are retained with timestamps and the current
   public timetable shows the amended time (delivery latency per UC-017).
5. **Given** a configured meet, **when** the organizer requests the sanctioning summary,
   **then** a document is produced containing tier, venue, dates, categories and disciplines,
   suitable for federation registration (field completeness checked against SYS-006).

## UC-002 Category schemes & discipline catalog — MVP

**Actors:** operator (organizer/admin).
**Traces:** SYS-003, SYS-005 → STR-029, STR-001, STR-007.

1. **Given** a new meet in year 2027, **when** the organizer selects the built-in Swiss Athletics
   scheme, **then** an athlete born 2012 resolves to `U16` and one born 1990 to `Men/Women`,
   per the WO 2026 birth-year windows (D4.2).
2. **Given** the Swiss scheme, **when** category assignment runs on 1 January, **then** athletes
   transition categories by calendar year (an athlete `U16` on 31 Dec is `U18` on 1 Jan when
   crossing the bound).
3. **Given** a U14 athlete, **when** entered in a U16 event where the scheme permits starting up,
   **then** the entry is accepted and marked "started up"; **when** entered in a `Men` 400m
   Hurdles event (barred for U16-and-younger, D4.3), **then** the entry is rejected with the rule
   shown.
4. **Given** an admin defining a custom scheme (codes, birth-year windows, up/down rules),
   **when** saved and applied to a meet, **then** category resolution uses the custom scheme
   without code changes.
5. **Given** the discipline catalog, **when** listing `100mH / U18 W` vs `100mH / Women`,
   **then** each carries its category-correct technical variant (hurdle height/spacing) from the
   built-in data.

## UC-003 Online entries — MVP

**Actors:** athlete, club entry submitter.
**Traces:** SYS-011, SYS-012, SYS-015 → STR-005.

1. **Given** a published meet with open entries, **when** an athlete submits an entry with a seed
   performance before the deadline, **then** the entry is stored with source `online`, the
   submitter receives a confirmation, and the entry is visible to them with status `entered`.
2. **Given** a club submitter, **when** they enter 15 athletes across 6 events in one bulk
   operation, **then** all 15 entries exist and a per-club entry list shows them.
3. **Given** an event whose deadline has passed, **when** an online entry is attempted
   (including by direct request forgery), **then** it is rejected server-side.
4. **Given** a relay event, **when** a club enters a team of 4 named athletes in order plus 2
   reserves, **then** the team entry stores the ordered composition and permits changes until the
   configured deadline, rejecting them after it.
5. **Given** an event with entry standard `12.20` (100m), **when** an entry with seed `12.85` is
   submitted, **then** the entry is stored but marked as failing the standard and appears on the
   organizer's exception report (SYS-015).

## UC-004 Entry import from file — MVP

**Actors:** operator (competition office).
**Traces:** SYS-013, SYS-010 → STR-006.

1. **Given** a documented system-native CSV of 200 entries, **when** imported, **then** 200
   entries and their athletes (name, birth date, sex, club, licence no.) exist and the import
   report shows 200 accepted / 0 rejected.
2. **Given** the same file imported again, **when** the import completes, **then** no duplicates
   exist (idempotency) and the report shows updates, not inserts.
3. **Given** a file with 3 malformed rows (missing birth year, unknown event code, bad category),
   **when** imported, **then** valid rows import, the 3 rows are rejected, and each rejection
   carries a row number and reason.
4. **Given** an Alabus-mapping profile *(activated once a sample is available, A-011)*,
   **when** a Swiss entries export is imported, **then** athletes map with licence numbers and
   category/discipline combinations per the profile. *(Acceptance data TBD-004/OQ-011.)*

## UC-005 Eligibility validation — MVP

**Actors:** operator (competition office).
**Traces:** SYS-014 → STR-007.

1. **Given** a licence-required meet tier (Swiss B-Meeting), **when** an entry has no licence
   number, **then** it is flagged `licence missing` before start-list generation.
2. **Given** an athlete born 2015 (U12), **when** entered for 3000m stadium (max 2000m per
   D4.3), **then** the entry is flagged with the youth-protection rule reference.
3. **Given** a U12 athlete already entered for a 600m race on day 1, **when** a second ≥600m
   race entry on the same day is added, **then** it is flagged (max one per day, D4.3).
4. **Given** a flagged entry, **when** an authorized operator overrides with a reason,
   **then** the entry proceeds and the override (actor, reason, timestamp) is in the audit
   trail.
5. **Given** the eligibility engine, **when** the reference test suite from D4.2/D4.3 fixtures
   runs (every category bound, up/down rule, and youth limit), **then** all expected
   verdicts match (SYS-142).

## UC-006 Bib assignment & fee summary — MVP

**Actors:** operator (organizer).
**Traces:** SYS-018, SYS-017 → STR-010, STR-009.

1. **Given** 300 accepted entries from 12 clubs, **when** the operator assigns bibs from ranges
   per club, **then** every athlete has exactly one unique bib within the meet and a printable
   bib list per club exists.
2. **Given** an attempt to assign a duplicate bib manually, **then** it is rejected.
3. **Given** a fee schedule (per-entry fee, relay fee), **when** the fee summary is generated,
   **then** per-club totals equal the entries × schedule arithmetic (verified against a fixture),
   and a post-meet participation summary suitable for the Swiss levy report is exportable.

## UC-007 Check-in and DNS handling — MVP

**Actors:** operator (check-in/call room).
**Traces:** SYS-025 → STR-011.

1. **Given** a unit with 8 entered athletes and check-in open, **when** 7 confirm and the
   deadline passes, **then** the operator's one action marks the unconfirmed athlete `DNS` and
   the final start list shows 7 starters + 1 DNS.
2. **Given** a DNS-marked athlete, **when** the referee reinstates them before the start
   (override), **then** they return to the start list and the change is audited.
3. **Given** check-in state changes, **when** viewed from a second operator session, **then**
   the state is current (within the concurrency guarantees of UC-021).

## UC-008 Heat seeding & lane draws — MVP

**Actors:** operator (competition office).
**Traces:** SYS-026, SYS-027, SYS-028 → STR-012.

1. **Given** 21 confirmed 100m entries with seed times and 8 lanes, **when** heats are generated,
   **then** 3 heats of 7 exist, seeded serpentine by seed mark, and no two of the top-3 seeds
   share a heat.
2. **Given** two athletes of the same club in adjacent seed positions, **when** heats are
   generated, **then** they are in different heats where mathematically possible (TR20).
3. **Given** a 400m final field ranked 1–8, **when** lanes are drawn, **then** ranks 1–4
   occupy lanes 4–7 (random within group), ranks 5–6 lanes 3/8, ranks 7–8 lanes 1/2
   (TR20.4, D2.3); repeated draws produce different in-group permutations (randomness).
4. **Given** a 9-lane track and 8 athletes, **when** lanes are drawn with lane 1 configured
   unused, **then** assignments shift per the rule's adapted numbering (D2.3).
5. **Given** a generated heat sheet, **when** the operator swaps two athletes manually and
   then regenerates after a scratch, **then** the manual swap survives regeneration (SYS-028)
   unless explicitly released.

## UC-009 Round progression — MVP

**Actors:** operator (competition office).
**Traces:** SYS-029, SYS-030 → STR-013.

1. **Given** 3 heats with results and progression `top 2 places + 2 fastest`, **when**
   progression runs, **then** exactly 6 `Q` and 2 `q` athletes advance, `q` selected by time
   from a single timing source, and the semi-final start list is generated.
2. **Given** a tie for the last time qualifier, **then** the tie is surfaced to the operator
   with the rule-appropriate options (both advance if capacity, else draw `qD`).
3. **Given** a referee decision advancing an athlete, **when** recorded, **then** the athlete
   advances with code `qR` and appears on the next round's list.
4. **Given** a long-throw qualification with standard 14.00m and finals capacity 12, **when**
   5 athletes beat the standard, **then** those 5 are `Q`, the next 7 by mark are `q`, and the
   final's start order is generated.

## UC-010 Track result capture — MVP

**Actors:** operator (competition office / timing).
**Traces:** SYS-040, SYS-041, SYS-045, SYS-050 → STR-014.

1. **Given** a finished 100m heat, **when** FAT times to 0.01s, wind `+1.4`, and finishing order
   are saved, **then** the unit's results rank correctly, display wind, and are marked FAT.
2. **Given** hand-timed input `11.32`, **then** the stored official mark is `11.4` (round-up,
   D5.1) and marked hand-timed on all outputs.
3. **Given** one athlete DNF and one DQ under TR16.8, **when** statuses are set, **then** the
   result list renders `DNF` and `DQ (TR16.8)` per CR25 convention and a DQ without rule
   reference is rejected (SYS-045).
4. **Given** wind `+2.3` on a 200m, **when** results save, **then** every mark in that race is
   flagged wind-assisted for record/PB purposes but competition placing is unaffected
   (SYS-050).
5. **Given** the rounding/wind engines, **when** the reference test suite (D5.1/D5.3 fixture
   values) runs, **then** all expected values match (SYS-142).

## UC-011 Horizontal field event capture — MVP

**Actors:** operator (field official).
**Traces:** SYS-042 → STR-015.

1. **Given** a long-jump flight of 12 with 3+3 attempts and a cut to top 8, **when** attempts
   are captured (`6.12`, `X`, `–`, `r`, per-attempt wind), **then** after round 3 exactly the
   top 8 by best mark continue, in rule-correct order.
2. **Given** two athletes with best `6.42`, **when** ranked, **then** the better second-best
   mark decides (next-best tie-break), and equal full series rank equal.
3. **Given** an athlete retiring after attempt 4 (`r`), **then** their best mark still ranks
   and the status shows `r`.
4. **Given** live capture, **when** each attempt saves, **then** current standings are
   recomputed and visible to other sessions within the UC-021 guarantees.

## UC-012 Vertical jump capture — MVP

**Actors:** operator (field official).
**Traces:** SYS-043 → STR-015.

1. **Given** a high-jump progression `1.60/1.65/1.70/1.75`, **when** an athlete records
   `O, XO, XXO, XXX`, **then** their best is `1.70` and they are eliminated at `1.75`.
2. **Given** an athlete passing (`–`) at 1.65 after `X` at 1.60's second attempt, **when** they
   fail twice more at 1.65, **then** elimination triggers on three consecutive failures across
   heights.
3. **Given** two athletes clearing `1.83`, **when** countback runs, **then** fewer attempts at
   1.83, then fewer total failures, decide; a persisting tie for **first place** is flagged for
   jump-off/shared-first resolution per rule, operator-selectable.
4. **Given** the countback engine, **when** the reference fixture suite runs (documented CR&TR
   examples), **then** all placements match (SYS-142).

## UC-013 Combined events — MVP

**Actors:** operator (competition office).
**Traces:** SYS-031, SYS-044 → STR-016.

1. **Given** a U18 W heptathlon in 2 groups, **when** the 100mH results save, **then** each
   athlete's points equal the World Athletics scoring-table formula output (reference fixtures,
   e.g. published table values) and standings merge both groups.
2. **Given** disciplines 1–4 complete, **when** standings are viewed, **then** cumulative points
   and ranks are correct after each discipline (fixture-verified).
3. **Given** an athlete DNF in one discipline, **then** they score 0 for it and continue per rule
   (status visible), while a withdrawal (`r`) removes them from subsequent start lists.
4. **Given** a wind-relevant combined-events record candidate, **then** wind legality uses the
   average across wind-relevant disciplines (D5.3/D5.5 rule) for flagging (with SYS-050).

## UC-014 Timing system exchange (FinishLynx) — MVP

**Actors:** operator (timing), FinishLynx-class system on venue LAN.
**Traces:** SYS-060, SYS-061, SYS-062 → STR-018.

1. **Given** a seeded meet, **when** timing export runs, **then** `lynx.ppl`, `lynx.sch`,
   `lynx.evt` files appear in the configured directory and validate field-for-field against
   the published FinishLynx format (fixture comparison).
2. **Given** a start-list change (lane swap), **when** re-export runs, **then** the files
   reflect the change.
3. **Given** a valid `.lif` results file for heat 2 of event 5, **when** the watcher ingests it,
   **then** times (0.01s), places, and reaction times attach to the correct unit and the results
   appear as provisional pending confirmation.
4. **Given** a `.lif` whose bibs don't all match the start list, **then** ingestion presents a
   conflict view (no silent overwrite) and the operator resolves per athlete.
5. **Given** a unit that already has manually captured results, **when** a `.lif` for it
   arrives, **then** the operator must choose (keep/replace/merge) explicitly.
6. **Given** the generic CSV import/export (SYS-062), **when** the same round-trip runs,
   **then** data is preserved losslessly (schema-documented).

## UC-015 Result correction, audit & protest clock — MVP

**Actors:** operator (competition office), referee.
**Traces:** SYS-046, SYS-047 → STR-019.

1. **Given** a posted result list, **when** it is published, **then** the announcement
   timestamp is recorded and the protest window (30 min) countdown state is visible for that
   unit (D8.3).
2. **Given** a protest upheld changing places 2 and 3, **when** the correction is applied with
   reason, **then** placings, progression (if affected), records flags, public pages, and
   exports all reflect the change within one publication cycle, and the amended list gets a new
   announcement timestamp (opening the appeal window).
3. **Given** any correction, **then** the audit trail holds actor, timestamp, before/after,
   and reason; audit entries are immutable (no API/UI path mutates them).
4. **Given** a result within its protest window, **then** public surfaces mark it
   `provisional`; after window expiry with no protest, status becomes `official` without
   operator action.

## UC-016 Records & bests flagging — MVP

**Actors:** system; operator (competition office).
**Traces:** SYS-049, SYS-050, SYS-051 → STR-024.

1. **Given** a loaded meeting-record list with `100m U18 W = 11.90`, **when** a result `11.85`
   (wind `+1.1`, FAT) saves, **then** it is flagged `MR` in operator view, public results, and
   exports.
2. **Given** the same mark with wind `+2.4`, **then** no record flag is set (wind-assisted) but
   the placing stands.
3. **Given** a hand-timed mark better than a record requiring FAT (races ≤800m, D6.2),
   **then** no record flag is set and the reason is inspectable.
4. **Given** a flagged record, **when** the record checklist is generated, **then** it contains
   the Rekordprotokoll data available in-system (timing class, wind, zero-test field, image
   reference field, competitor count) with gaps explicitly marked for manual completion.
5. **Given** an athlete with in-system history `PB 12.02`, **when** they run `11.95` legal,
   **then** the result is flagged `PB` (and `SB` logic analogous).

## UC-017 Public live results & archive — MVP

**Actors:** public visitor (no account).
**Traces:** SYS-070, SYS-071, SYS-074, SYS-122 → STR-022, STR-028.

1. **Given** a live meet with connectivity, **when** the competition office confirms a result,
   **then** an already-open public results page reflects it within 10 s (p95, measured by
   automated probe).
2. **Given** a public meet page, **when** fetched without authentication on a 360px-wide
   viewport, **then** timetable, start lists, and results are readable and navigable
   (no login wall, no app prompt).
3. **Given** a meet closed 12 months ago, **when** its result URLs are fetched, **then** they
   return the archived results at the same stable URLs.
4. **Given** the reference hosting size, **when** a load test simulates 2,000 concurrent
   viewers, **then** p95 page render ≤3 s and no errors >0.1% (SYS-122).
5. **Given** a meet configured with a federation channel as the official source (SYS-076),
   **when** any public results page or publication export renders, **then** it carries the
   "unofficial results" label naming/linking the official source, in the page language;
   **given** a meet configured as primary publication, **then** no such label appears.

## UC-018 Printable official documents — MVP

**Actors:** operator (competition office).
**Traces:** SYS-072 → STR-023.

1. **Given** any seeded unit, **when** documents are generated, **then** PDF start lists and
   capture sheets (track: lanes/bibs; field: attempt grids; vertical: height columns) match the
   current data and carry meet/session/unit headers and generation timestamps.
2. **Given** a completed unit, **when** the result PDF is generated, **then** it shows marks,
   wind, statuses per CR25 codes, record flags, and the announcement timestamp.
3. **Given** the meet language set to FR, **then** all document headings/labels render in
   French from the domain glossary (with UC-025).

## UC-019 Offline venue operation (venue-node role) — Later

**Actors:** operators on venue LAN; internet absent.
**Traces:** SYS-080, SYS-082, SYS-093 → STR-020.
> **Re-scoped 2026-07-05 (DEC-013 / ADR-002 v2):** the venue-node role is deferred behind an
> evidence gate (a season of hub-first meets, or a committed no-uplink venue). MVP connectivity
> tolerance is UC-034. Criteria below remain the definition of done for the deferred role.

1. **Given** the venue server started with no internet route, **when** operators perform
   check-in, seeding, capture, corrections, printing, and timing exchange, **then** every
   function succeeds (automated E2E suite runs with egress blocked).
2. **Given** results confirmed while offline, **when** internet connectivity returns, **then**
   queued publications reach the public surface automatically, in order, without operator
   action, and the backlog indicator drains to zero.
3. **Given** offline operation, **then** no feature degrades due to unreachable third-party
   resources (fonts, scripts, tiles — verified by the egress-blocked suite).
4. **Given** LAN clients connecting to the venue server, **then** operator functions work in a
   current browser without internet-dependent certificate validation blocking access
   (SYS-093/SYS-080 interplay; mechanism is a Phase B ADR).

## UC-020 Durability, backup & restore — MVP

**Actors:** operator (admin).
**Traces:** SYS-081, SYS-084, SYS-130 → STR-041, STR-039.

1. **Given** a meet mid-capture, **when** the application process is killed hard and restarted,
   **then** every previously confirmed write is present and the system is operational in
   ≤2 minutes (automated crash test).
2. **Given** a simulated power loss (VM kill) during a burst of result saves, **when** the host
   restarts, **then** the system recovers to a consistent state: all confirmed saves present,
   no partially applied records (consistency checker green).
3. **Given** a one-action backup taken during the meet, **when** restored onto a fresh
   installation, **then** the full meet state is reproduced (record counts, checksums equal)
   in ≤15 minutes.

## UC-021 Multi-operator concurrency — MVP

**Actors:** ≥10 concurrent operator sessions.
**Traces:** SYS-083 → STR-021.

1. **Given** 10 sessions writing to different units concurrently (simulated), **then** all
   writes persist; none is lost or misattributed.
2. **Given** two sessions editing the same athlete's same result, **when** the second saves,
   **then** the conflict is detected and surfaced with both versions — never silent
   last-write-wins.
3. **Given** infield capture (UC-011) plus office corrections (UC-015) on the same event,
   **then** standings converge to a state consistent with the audit-trail order.

## UC-022 Accounts, roles & privileged-action audit — MVP

**Actors:** admin, per-meet roles.
**Traces:** SYS-090, SYS-091 → STR-040.

1. **Given** a field-official account scoped to Shot Put U16 W, **when** it attempts to edit a
   100m result or another meet, **then** access is denied and the attempt logged.
2. **Given** any privileged action (role grant, override, correction), **then** an audit event
   exists (actor, action, target, timestamp).
3. **Given** the permission matrix (SYS-090 roles), **when** the automated authorization test
   sweeps every role × capability pair, **then** outcomes match the documented matrix exactly.
4. **Given** stored credentials, **then** inspection confirms only adaptive hashes at rest and
   sessions expire per configuration.

## UC-023 Public data minimization & consent enforcement — MVP

**Actors:** system; public visitor.
**Traces:** SYS-100, SYS-103 → STR-031, STR-032.

1. **Given** any public page or publication export, **when** crawled by the automated PII
   scanner, **then** no birth date (beyond category-implied birth year), licence number,
   contact detail, or consent flag appears.
2. **Given** a minor whose publication consent is withdrawn, **when** public results render,
   **then** their entry is suppressed/neutralized per the configured legal mode while the
   internal official result remains intact and exportable to the federation.
3. **Given** consent flags changed mid-meet, **then** the public surface reflects the change
   within one publication cycle.

## UC-024 Data-subject rights & retention — MVP

**Actors:** operator (admin) acting on a request.
**Traces:** SYS-101, SYS-102 → STR-031.

1. **Given** a person in the system, **when** a subject-access export runs, **then** it contains
   all their stored personal data machine-readably, and nothing about other persons.
2. **Given** an erasure request compatible with the sporting record, **when** executed,
   **then** identifying data is removed/pseudonymized, official results integrity is preserved,
   and the action is documented.
3. **Given** retention configured to purge contact data 90 days post-meet, **when** the period
   elapses, **then** the purge runs and a verification query finds no out-of-retention data.

## UC-025 Multilingual surfaces — MVP

**Actors:** public visitor; operator.
**Traces:** SYS-110, SYS-111 → STR-033.

1. **Given** any surface (public, participant, operator), **when** each of DE and FR is
   selected, **then** all UI text, discipline/category/status labels render in that language
   with zero missing-key fallbacks (automated completeness check against extraction, DEC-008).
2. **Given** a pseudo-locale translation file added to the build with no code change,
   **when** the pseudo-locale is selected, **then** all surfaces render it completely — proving
   adding IT/EN later is translation-only (SYS-110).
3. **Given** locale rendering, **then** dates/numbers format per locale while performance-mark
   notation follows the documented project convention consistently in both languages
   (fixture test).

## UC-026 Accessibility of public surfaces — MVP

**Actors:** public visitor using assistive technology.
**Traces:** SYS-112, SYS-113 → STR-034.

1. **Given** every public page type (timetable, start list, live results, result list),
   **when** the automated WCAG 2.2 AA scan runs, **then** zero violations of automatable
   criteria.
2. **Given** the manual audit checklist (screen reader walk-through of following one athlete
   through a meet), **then** every step is completable and documented per release.
3. **Given** live-updating results, **then** updates are announced accessibly (no
   focus-stealing; polite live regions) — verified in the audit.

## UC-027 Result export & federation delivery — MVP

**Actors:** operator (organizer).
**Traces:** SYS-073 → STR-025, STR-026, STR-037.

1. **Given** a completed meet, **when** the full export runs, **then** CSV and structured
   (JSON) artifacts validate against the published schema and contain all official results
   with statuses, wind, categories, record flags, and announcement timestamps.
2. **Given** the export and a fresh system, **when** the export is re-imported, **then** the
   official results reconstruct equivalently (round-trip property test).
3. **Given** a meet closed at time T, **then** the export is available immediately at T
   (supports the ≤24h federation submission expectation, D10.2) — no post-processing delay.

## UC-028 Mixed-category fields — MVP *(para classes: Later, DEC-007)*

**Actors:** operator.
**Traces:** SYS-052, SYS-010 → STR-030, STR-029 (D9.2, C7.3).

1. **Given** a 100m unit combining `U16 M` and `U18 M` entrants for lack of entries (mixed
   field per CR 25.2–25.3), **when** results save, **then** the system produces both the race
   result and per-category re-ranked presentations, each printable/publishable.
2. **Given** a youth-series meet with per-birth-year divisions (e.g., UKC `M12`/`M13` running
   in one heat), **then** the same split-presentation mechanism applies per division.
3. *(Later)* **Given** an athlete with sport class `T38`, **when** entered, **then** the class
   is stored and the split-presentation mechanism extends to class groups.

## UC-029 Team/club scoring — Later

**Traces:** SYS-048 → STR-017. *(Blocked on TBD-005 for exact SVM values; scheme configurability
is the requirement.)*

1. **Given** a configured placing-points scheme (points per place per discipline, quotas),
   **when** individual results finalize, **then** team standings compute per the scheme
   (fixture-verified) and update on corrections (with UC-015).
2. **Given** nationality quotas configured per league, **then** only quota-conformant scorers
   count, and violations are surfaced.

## UC-030 Sponsor surfaces — Later

**Traces:** SYS-075 → STR-027.

1. **Given** organizer-uploaded sponsor assets and placements, **then** public pages and PDFs
   render them in the defined slots without overlapping competition data (visual regression
   test).

## UC-031 Scoreboard live feed — Later

**Traces:** SYS-064 → STR-018, STR-022.

1. **Given** a live unit, **when** the feed endpoint is consumed, **then** current standings
   update within the same latency budget as public pages and conform to a documented schema.

## UC-032 EDM field-measurement intake — Later

**Traces:** SYS-063 → STR-018. *(Requires device-protocol selection; Phase B ADR.)*

1. **Given** a connected EDM device (target protocol TBD), **when** a measurement is taken,
   **then** it lands as the current athlete's attempt pending official confirmation.

## UC-033 UBS Kids Cup meet from template — MVP

**Actors:** operator (small-club organizer); the founder's canonical scenario (C7.2, DEC-002).
**Traces:** SYS-053, SYS-052 → STR-043, STR-016.

1. **Given** a fresh system, **when** the organizer creates a meet from the built-in UBS Kids
   Cup template with a date and venue, **then** the meet exists with the 3 UKC disciplines
   (60 m, zone long jump, 200 g ball throw), per-birth-year divisions (M/W 7–15), and the UKC
   combined scoring — with no further configuration.
2. **Given** UKC results for one athlete (e.g., 60 m `8.42`, long jump `4.12`, ball `38.50`),
   **when** saved, **then** the points per discipline and total equal the official UKC points
   table values (reference fixtures from the published table), and division standings update.
3. **Given** live/in-progress capture, **when** an athlete is missing a discipline, **then**
   the (provisional) standings still rank them by their partial total, the gap shown explicitly.
   **Given** the division's series is complete (every discipline's results announced) and FINAL
   standings are computed, **then** a discipline the athlete attempted but recorded no valid
   result for scores the official 1-point floor and still counts as present (fixture-verified
   against the published points table), while an athlete missing a discipline entirely — no
   result at all, not even an invalid-attempt status — is listed **unranked at the bottom** of
   the division instead of ranked by partial total: the official TAF3 convention (DEC-016,
   fixture-verified against the LV Langenthal official Rangliste, 17.05.2025,
   https://lvl.ch/images/resultate/2025/Gesamtrangliste_UBSKidsCup_2025.pdf). Every rendering
   surface (public results, printed result lists, series-upload export, omx snapshot) applies
   FINAL semantics once the series is complete and PROVISIONAL labeling otherwise.
4. **Given** the completed meet, **when** results are exported/printed, **then** per-division
   ranking lists (classement) match the observed federation presentation structure: rank, bib,
   name, club, birth year, per-discipline marks, points, total (C7.3).
5. **Given** the UKC points table shipped as data (CON-01), **when** the series publishes a
   revised table, **then** an operator/maintainer can update it without code changes
   (data-file swap verified by test).

## UC-034 Offline-tolerant field capture & walk-by sync — MVP

**Actors:** field official on their own phone (browser, no app install); connectivity
intermittent (admin-table hotspot, patchy venue Wi-Fi, or the device's own mobile data).
The MVP answer to "small clubs cannot build field-wide Wi-Fi" (DEC-013, research C8).
**Traces:** SYS-085, SYS-086, SYS-087 → STR-020, STR-021, STR-041.

1. **Given** a device that has checked out its assigned event unit (SYS-086), **when**
   connectivity to the server is cut, **then** attempt/result entry continues uninterrupted
   with a clear offline indicator (automated browser test with network blocked mid-event).
2. **Given** attempts captured while disconnected, **when** connectivity returns (e.g., the
   official walks back into hotspot range), **then** the captures submit automatically and in
   order with no operator action, standings update, and repeated flaky reconnects produce no
   duplicates (idempotent replay verified).
3. **Given** an offline device whose page is reloaded or browser restarted, **then**
   previously captured attempts survive locally and still sync on reconnection.
4. **Given** the office changes the start list of a checked-out unit while the device is
   offline, **when** the device reconnects, **then** its captures are surfaced for
   reconciliation — never silently discarded (anti-pattern test named for the documented
   Web.TEC 2 loss mode, C2.1).
5. **Given** a lost/dead device, **when** the office overrides the checkout (audited,
   SYS-046), **then** capture resumes on another device; if the original device later
   reconnects, its stale captures land in reconciliation, not silently applied.
6. **Given** the device reaches the server over mobile data instead of Wi-Fi, **then**
   criteria 1–3 hold identically (transport-agnostic test matrix).
7. **Given** an office operator surface (check-in, seeding) during a ≤5-minute connectivity
   interruption, **then** no confirmed write is lost, the degraded state is indicated, and
   work resumes without restart, re-login, or re-entry (SYS-087).

## UC-035 Swiss youth-series results upload file — MVP

**Actors:** operator (small-club organizer) fulfilling series upload duties without TAF3
(the official Excel path, C2.1).
**Traces:** SYS-077 → STR-043, STR-025, STR-042.

1. **Given** a completed UBS Kids Cup meet (UC-033), **when** the organizer runs the "series
   upload export", **then** the produced file conforms to the current official UKC organizer
   template (structure verified against a fixture derived from the published template) and
   contains every participant with marks, points, and division data.
2. **Given** an athlete with a missing discipline or non-scoring status, **then** the export
   represents them per the series convention rather than silently dropping rows (fixture-
   verified edge cases).
3. **Given** a new season's template revision shipped as a data file, **then** exports conform
   to the new template with no code change (data-swap test, SYS-077).

## UC-036 TAF3 coexistence exports — Later

**Actors:** organizer of a sanctioned meet that must finish in TAF3 (Alabus seam, OQ-014).
**Traces:** SYS-078 → STR-018, STR-025, STR-037, STR-042.

1. **Given** a completed event unit, **when** exported as a per-event `.lif` file, **then**
   the file round-trips through our own SYS-061 importer bit-consistently and follows the
   published FinishLynx field order (fixture from the public format documentation).
2. **Given** a meet's results, **when** exported as TAF3-compatible CSV, **then** the
   structure matches the publicly documented TAF3 result-export layout (fixture-verified;
   ingestion into a real TAF3 is a manual demonstration until OQ-013/OQ-014 give access).

## UC-037 Contextual input help — MVP

**Actors:** operator (first-time volunteer), participant entering online entries.
**Traces:** SYS-115 → STR-044, STR-035, STR-034.

1. **Given** the help-content registry (SYS-115), **when** each screen containing a
   registered input renders, **then** every registered input shows a help icon adjacent to
   its label — no registered input lacks one (rendering-time coverage test over the
   registry).
2. **Given** a help icon, **when** activated by (a) pointer hover, (b) keyboard focus,
   (c) click/tap, **then** the same short help text appears, localized (asserted in DE and
   FR).
3. **Given** open help content, **then** Escape dismisses it without moving focus, the
   pointer can be moved onto the content without it disappearing, and it persists until
   dismissed or de-hovered/blurred (WCAG 2.2 SC 1.4.13 — Playwright).
4. **Given** a help trigger and its content, **then** the trigger is keyboard-focusable with
   an accessible name, the content is programmatically associated with the trigger, and the
   automated WCAG 2.2 AA scan of a help-open state reports zero violations.
5. **Given** any field with a hard input constraint (format, units, mandatory), **then** the
   constraint is visible on-screen as label/hint text without activating help (fixtures for
   representative fields; full sweep via the SYS-117 release audit).

## UC-038 Design-system conformance & usability audit — MVP

**Actors:** contributor (via CI), release auditor.
**Traces:** SYS-116, SYS-117 → STR-045, STR-038.

1. **Given** the in-repo design-token and component documentation, **when** the CI
   style-conformance check runs over all templates and stylesheets, **then** zero raw
   visual literals (colors, font sizes, spacing) occur outside the token definitions
   (documented allowlist for the mechanical check's known limits).
2. **Given** every interactive component in the inventory, **then** a keyboard walk shows a
   visible focus indicator per component, and each documented component state renders
   (automated e2e; contrast per the existing WCAG AA scans).
3. **Given** a release candidate, **when** the SYS-117 usability audit checklist is
   executed, **then** the completed, dated record is committed in-repo with zero open
   critical findings (I — same mechanism as the accessibility manual audit checklist).
4. **Given** representative forms (meet setup, online entry, result correction), **then**
   each input shows a permanently visible label and visible constraint hints, and a
   submitted validation error renders inline at the field, states what to fix, and
   preserves the user's input (T).

## UC-039 Volunteer mobile capture — MVP *(ratified 2026-08-07, DEC-027)*

**Actors:** field official (volunteer, own phone).
**Traces:** SYS-147, SYS-148 → STR-046, STR-035, STR-041.
**Source:** volunteer walkthrough findings F2/F3 (`../delivery/usability-audit-volunteer-2026-08.md`).

1. **Given** a capture unit page of each family (track, horizontal, vertical) rendered at a
   360×740 viewport, **when** the operator captures a mark for any athlete, **then** the
   mark input and its save control are operable without horizontal page scrolling
   (`document.documentElement.scrollWidth` ≤ viewport width — Playwright mobile viewport).
2. **Given** the same viewport, **then** the first capture row is visible within the first
   640 px of page height without scrolling past chrome, and every primary capture control
   (mark input, save, status select) has a hit target ≥44×44 CSS px (mechanical DOM audit;
   no interactive element under 24×24 px).
3. **Given** mark, time and wind inputs, **then** each declares a virtual-keyboard hint
   (`inputmode`/`enterkeyhint`) appropriate to its format while the documented letter
   markers (X/–/r) remain enterable.
4. **Given** a mark saved while online, **then** the cell shows a pending state until the
   server acknowledgment and a confirmed state after it, each distinguishable by more than
   color, appearing within 500 ms of the state change; the row's derived result/points cells
   update without a manual reload.
5. **Given** a mark saved while offline, **then** the cell's pending state persists and the
   existing status region (SYS-087) reflects the queued count until reconnection sync
   confirms it.

## UC-040 Truthful sync-failure handling & recovery — MVP *(ratified 2026-08-07, DEC-027)*

**Actors:** field official; competition office.
**Traces:** SYS-149 → STR-046, STR-041, STR-020.
**Source:** volunteer walkthrough finding F1 (`../delivery/usability-audit-volunteer-2026-08.md`).

1. **Given** a queued capture the server rejects as invalid (e.g. malformed mark), **when**
   sync runs, **then** the rejection renders at the offending cell with a plain-language
   reason, is not presented as a connectivity problem, and is not retried automatically.
2. **Given** one rejected operation and other valid queued operations, **then** the valid
   operations apply on the same or next sync cycle — a rejection never blocks the queue
   (chaos-style e2e).
3. **Given** a rejected operation, **then** the operator can correct the value or discard
   the queued operation from the capture page, and the status region's pending count
   reflects the outcome immediately.
4. **Given** the operator's session expires while operations are queued, **when** sync next
   runs, **then** the UI prompts re-authentication, the queue survives re-login, and pending
   operations apply afterwards without re-entry (cf. SYS-087).
5. **Given** any point in time, **then** the connectivity/status region never simultaneously
   reports "all transferred" and a non-zero pending count.

## UC-041 Task-first navigation & empty states — MVP *(ratified 2026-08-07, DEC-027)*

**Actors:** competition-office volunteer; field official.
**Traces:** SYS-151, SYS-152 → STR-035, STR-046, STR-045.
**Source:** volunteer walkthrough findings F5/F7/F8 (`../delivery/usability-audit-volunteer-2026-08.md`); OQ-113.

1. **Given** a competition-office session's home, **then** check-in, result
   capture/reconciliation, roster and standings of each of its meets are each reachable
   within two link activations (link-walk e2e, extending the TASK-043 pattern).
2. **Given** a field official's home, **then** each assigned unit shows its localized
   discipline name and, where scheduled, time and location, and links directly to its
   capture page.
3. **Given** any operator surface, **then** it is reachable through rendered links starting
   from its role's home — no surface requires a typed URL.
4. **Given** an operator list surface in an empty state (e.g. check-in with no entries),
   **then** the page states why it is empty and the next step where one exists, and actions
   that cannot apply (close check-in, bulk DNS with nothing to affect) are hidden or
   disabled with the reason shown.
5. **Given** an applicable bulk or destructive action, **then** its confirmation step states
   the number of rows it will affect.

## UC-042 Public find-your-athlete — MVP *(ratified 2026-08-07, DEC-027)*

**Actors:** spectator/parent on a phone.
**Traces:** SYS-153 → STR-022, STR-034.
**Source:** volunteer walkthrough finding F9 (`../delivery/usability-audit-volunteer-2026-08.md`).

1. **Given** a published meet's public results or start lists on a phone, **when** the
   visitor filters by name, bib or club, **then** only matching rows (and their categories)
   remain visible; without client-side scripting the same filter works via a full-page
   round trip.
2. **Given** the unfiltered page, **then** per-category jump navigation renders at the top
   and each category heading links back to the top.
3. **Given** the live results page with a filter applied, **then** a live update (SSE
   refresh) does not clear the filter.

## UC-043 Participant data correction — MVP *(ratified 2026-08-07, DEC-027)*

**Actors:** competition office.
**Traces:** SYS-150 → STR-035, STR-031.
**Source:** volunteer walkthrough finding F4 (`../delivery/usability-audit-volunteer-2026-08.md`).

1. **Given** a rostered participant, **when** office corrects name, birth year, sex, club or
   bib, **then** the change is optimistic-version-guarded (a concurrent edit yields the
   standard conflict error, input preserved) and takes effect on roster, start lists,
   capture pages, standings and exports — including category re-derivation where birth year
   or sex changed.
2. **Given** a correction, **then** an audit row records actor, before/after values and
   timestamp via the SYS-046 mechanism.
3. **Given** a participant with captured results, **then** identity correction never alters
   or re-scores captured marks.
