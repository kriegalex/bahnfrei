# System Requirements Specification (SyRS)

**Project:** Open-Source Athletics Tournament Management System (working name: *OpenMeet*, TBD)
**Document status:** DRAFT — Phase A baseline
**Standard:** ISO/IEC/IEEE 29148:2018 (system requirements layer)

> Each `SYS-###` is testable and traces to one or more `STR-###`
> (`stakeholder-requirements.md`). Verification methods are assigned in
> `traceability-matrix.md`. `D#`/`C#` reference the research documents. IDs are stable —
> never renumbered, only deprecated. **SHALL** marks mandatory behaviour. Priority is
> inherited from the traced STR unless stated. Implementation technology is deliberately
> unspecified — stack decisions are Phase B ADRs.

## 1. System context

The system manages athletics meets end-to-end for organizers, competition offices, officials,
and the public. It exchanges data with: federation entry channels (file-based import),
timing/photo-finish systems (local file exchange), and federation result recipients
(file-based export). It operates in two modes: venue-local (offline-capable) and
internet-connected (public publication). External systems it explicitly does **not** replace:
federation licensing/member systems, federation calendars, World Athletics Global Calendar,
timing hardware control software (D10, C2).

## 2. Domain data model (conceptual)

Entities the system SHALL represent (attributes indicative, not exhaustive; formalized in
Phase B):

| Entity | Key attributes / relationships |
|--------|-------------------------------|
| **Meet** | name, venue, date range, sessions[], meet tier (e.g., CH A/B/C-Meeting), organizer, sanctioning info, status (draft/published/live/closed/archived) |
| **CategoryScheme / Category** | scheme (e.g., Swiss Athletics 2026, WA), category code (U16 W, M45…), birth-year window, start-up/down rules (D4) |
| **Discipline** | code (100m, HJ, SP…), family (track/field-horizontal/field-vertical/combined/relay), units, wind-relevance, per-category variants (hurdle heights, implement masses) |
| **Event** | meet × discipline × category (or combined categories), round structure, entry conditions, status |
| **Round / Unit** | round (qualification/semi/final) → units (heats / flights / groups), scheduled time, venue location |
| **Athlete** | person data, birth date, sex, nationality, club affiliation(s), licence number(s), para sport class(es) (T/F), consent/publication flags |
| **Club/Team** | name, federation code; relay teams as compositions of athletes |
| **Entry** | athlete/relay team × event, seed performance, status (entered/confirmed/scratched/DNS…), source (online/import/manual), fees |
| **Participation/Result** | per unit: lane/position/order, marks (times to 0.01s, distances/heights in m), wind, attempt sequence (X/O/–, per-attempt marks), statuses & qualification codes (D5.2 enum), points (combined/team), placings, record flags |
| **Record/Best reference** | record type (WR/AR/NR/meeting/…), scope, category, discipline, mark, holder, date — loadable reference lists (D6.1) |
| **Official document** | start lists, result lists (versioned, with announcement timestamp), record protocols |
| **User / Role** | accounts, role assignments per meet (§ SYS-090) |
| **Audit event** | actor, timestamp, entity, before/after, reason |

The persisted schema SHALL be documented and versioned (SYS-144); the W3C Open Athletics
draft Competition Data Model serves as a design reference, not a compliance target (A-012).

## 3. Functional requirements

### 3.1 Meet & competition structure

| ID | Requirement | Traces to |
|----|-------------|-----------|
| SYS-001 | The system SHALL let an authorized organizer create, edit, and archive meets with: name, venue, one or more competition days, sessions per day, organizer identity, and meet tier — via the operator interface, with no configuration-file editing. | STR-001, STR-004 |
| SYS-002 | The system SHALL let the organizer compose the event programme as events = discipline × category (including multi-category combined events), each with its round structure and entry conditions. | STR-001 |
| SYS-003 | The system SHALL ship a built-in discipline catalog covering World Athletics outdoor and indoor stadium disciplines (track, field, relays, combined events per D1.1) including per-category technical variants (hurdle heights/spacings, implement masses) for the built-in category schemes, and SHALL allow organizers to define additional custom disciplines. | STR-001, STR-029 |
| SYS-004 | The system SHALL maintain a meet timetable (scheduled time per unit) with draft/published states; every published amendment SHALL be timestamped and retained, and republication SHALL reach public surfaces per SYS-071. | STR-002 |
| SYS-005 | The system SHALL support configurable category schemes (codes, sex, birth-year windows, calendar-year transition, start-up/start-down permissions) and SHALL include as verified built-in defaults the Swiss Athletics scheme (WO 2026 §1.1: U10–U23, Men/Women, Masters 5-year bands per D4.2) and the World Athletics TR3 scheme (U18/U20/Masters per D4.1). | STR-029, STR-007 |
| SYS-006 | The system SHALL record sanctioning-relevant meet data (tier, venue homologation reference, categories/disciplines, dates, organizer) and SHALL produce a human-readable sanctioning summary suitable for federation registration ≥30 days before the meet (D3.4). | STR-003 |

### 3.2 Athletes, clubs & entries

| ID | Requirement | Traces to |
|----|-------------|-----------|
| SYS-010 | The system SHALL maintain athlete records with: full name, birth date, sex, nationality, club affiliation(s), licence number(s), optional para sport class(es), and publication-consent flags. Birth date SHALL be storable as full date with category logic requiring only birth year where the full date is unknown. | STR-005, STR-029, STR-030, STR-031 |
| SYS-011 | The system SHALL accept online entries (individual and club-bulk) for published meets until the configured entry deadline per event, SHALL confirm each entry to the submitter, and SHALL show submitters the current status of their entries. Deadline enforcement SHALL be server-side. | STR-005 |
| SYS-012 | The system SHALL support relay-team entries: team per club × relay event with an ordered leg composition changeable up to the deadline configured for the meet. | STR-005 |
| SYS-013 | The system SHALL import entries and athlete master data from structured files in (a) a documented system-native CSV/JSON format and (b) a mapping profile for Swiss Athletics/Alabus entry exports once a sample is available (A-011/OQ-011); imports SHALL be idempotent (re-import updates rather than duplicates) and SHALL report per-row acceptance/rejection with reasons. | STR-006 |
| SYS-014 | The system SHALL validate every entry against: category eligibility by birth year (per the active category scheme incl. start-up/down rules), licence requirement for the meet tier, and Swiss youth-protection rules (max distances, barred disciplines, max one race ≥600m per day for U10–U14, per D4.3). Violations SHALL be flagged before start-list generation; an authorized operator MAY override with a recorded reason (audit per SYS-046). | STR-007 |
| SYS-015 | The system SHALL support per-event entry conditions: entry standards (min performance), entry limits (max entries per event/club/athlete), and required seed-performance information. Entries failing conditions SHALL be marked and reportable. | STR-005, STR-007 |
| SYS-016 | The system SHALL process late entries, withdrawals (scratches), and entry modifications after the deadline, gated by operator authorization, each recorded in the audit trail. | STR-008 |
| SYS-017 | The system SHALL compute entry-fee totals per athlete and per club from a configurable fee schedule and SHALL export a per-club fee summary and a post-meet participation summary usable for the Swiss starting-fee levy report (C2). Payment processing itself is out of MVP scope. | STR-009 |
| SYS-018 | The system SHALL assign bib numbers individually, in bulk, and from configured ranges (e.g., per club); bib numbers SHALL be unique within a meet and printable as bib assignment lists. | STR-010 |

### 3.3 Check-in, seeding & progression

| ID | Requirement | Traces to |
|----|-------------|-----------|
| SYS-025 | The system SHALL record per-athlete check-in (call-room confirmation) per event with configurable check-in deadlines, and SHALL set non-confirmed athletes to DNS on final start lists per operator action or configured automatic rule (D5.2, D11.1). | STR-011 |
| SYS-026 | The system SHALL generate heats from confirmed entries per World Athletics TR20: seeding basis = ranked seed performances (configurable window, default season best), distribution such that highest-seeded athletes and same-club athletes are placed in different heats where possible, using standard serpentine distribution; the applied rule set SHALL be visible to the operator (D2.2). | STR-012 |
| SYS-027 | For lane-run events, the system SHALL draw lanes per TR20.4 lane groups — ranked 1–4 randomly across lanes 4–7, ranked 5–6 across lanes 3+8, ranked 7–8 across lanes 1–2 (with documented adaptation for tracks with more/fewer lanes, unused inner lanes shifting numbering per D2.3) — and by-lot draw for events not run fully in lanes. | STR-012 |
| SYS-028 | The system SHALL allow an authorized operator to manually override any generated assignment (heat, lane, order, flight) before start-list publication, and to regenerate seedings after entry changes; overrides survive regeneration unless explicitly released. | STR-012, STR-008 |
| SYS-029 | The system SHALL compute round progression per configured rules (top *n* places per heat auto-qualify `Q`, next *k* by time `q`, single timing source for time-based qualification), SHALL support manual advancement with codes `qR`/`qJ`/`qD` (referee/jury/draw), and SHALL generate the next round's start lists from the qualifiers (D2.4, D5.2). | STR-013 |
| SYS-030 | The system SHALL organize field events into flights/groups with configurable size, qualifying standards (`Q` on standard) and/or top-*n* qualification (`q` on performance) to the final round of attempts. | STR-012, STR-013 |
| SYS-031 | The system SHALL organize combined-events competitions as ordered discipline sequences over one or two days, with participants divisible into parallel groups, and per-discipline results feeding the combined scoring (SYS-044). | STR-016 |

### 3.4 Results capture & scoring

| ID | Requirement | Traces to |
|----|-------------|-----------|
| SYS-040 | The system SHALL capture track results per unit: times with 0.01 s resolution for fully automatic timing and 0.1 s for hand timing, finishing order, per-race wind reading where the discipline is wind-relevant, and reaction times where provided by the timing system. | STR-014 |
| SYS-041 | The system SHALL apply the rule-defined hand-timing conversion: manually captured hand times SHALL be rounded **up** to the next 0.1 s (whole second for road events) and marked as hand-timed; hand and FAT times SHALL be distinguishable in all outputs (D5.1). | STR-014, STR-024 |
| SYS-042 | The system SHALL capture horizontal field events attempt-by-attempt: mark in metres (0.01 m), foul (X), pass (–), retirement (r), per-attempt wind where wind-relevant; SHALL support the configured trial count and field-cut after round 3; and SHALL rank by best mark with next-best-mark tie-breaking (D5.2, D5.6). | STR-015 |
| SYS-043 | The system SHALL capture vertical jumps with bar-height progressions: per-height attempts (O/X/–), automatic elimination after three consecutive failures, retirement, and ranking per countback rules (fewest attempts at last cleared height, then fewest total failures), flagging first-place ties that rule-wise may require a jump-off. | STR-015 |
| SYS-044 | The system SHALL score combined events using the official World Athletics scoring-table formulas (D5.4), showing per-discipline points and cumulative standings after each completed discipline; scoring computations SHALL be reproducible from stored raw marks. | STR-016 |
| SYS-045 | The system SHALL restrict result statuses and qualification codes to the CR 25 vocabulary (DNS, DNF, NM, NH, DQ+rule, O/X/–, r, Q, q, qR, qJ, qD, YC, YRC, RC, L, P per D5.2) and SHALL render them on start lists and results per that convention; DQ SHALL require a rule reference. | STR-014, STR-015 |
| SYS-046 | Every change to captured results, entries, and seedings SHALL be recorded in an immutable audit trail (actor, timestamp, before/after values, reason for overrides/corrections); corrections SHALL propagate to all derived artifacts (rankings, progressions, team scores, public results, exports) within one publication cycle (SYS-071). | STR-019, STR-008 |
| SYS-047 | The system SHALL record the official announcement timestamp of each posted result-list version and SHALL display per event the protest window state (30 minutes from announcement; 30 minutes from amended-result announcement for appeals, per D8.3), marking results as provisional/official accordingly. | STR-019 |
| SYS-048 | *(Later)* The system SHALL compute team/club competition standings from individual results per a configurable placing-points scheme (points per place, scored disciplines, athlete quotas — SVM-compatible once TBD-005 is resolved). | STR-017 |
| SYS-049 | The system SHALL flag performances that equal/better applicable reference records and bests: meeting records and loadable national/area/world reference lists (D6.1), plus PB/SB where the athlete's history exists in the system; flags SHALL appear in operator views, public results, and exports. | STR-024 |
| SYS-050 | The system SHALL evaluate wind legality (average tailwind > +2.0 m/s ⇒ wind-assisted) for wind-relevant disciplines and combined-events records (average across wind-relevant disciplines per D5.3) and SHALL exclude wind-assisted marks from record/best flagging while still ranking them in the competition. | STR-024, STR-014 |
| SYS-051 | The system SHALL assemble a record-documentation checklist per flagged record performance (timing homologation class, zero-test done, wind reading, photo-finish image reference, competitors count) covering the Swiss Rekordprotokoll fields (D6.2); *(Later:* generation of the complete submission dossier*)*. | STR-024 |
| SYS-052 | The system SHALL support units whose field combines multiple age categories or series divisions (CR 25.2–25.3, D9.2, C7.3): results SHALL be presentable both as the combined race and as split, re-ranked lists per category/division group, each printable and publishable. *(Later:* the same mechanism extended to para sport classes, DEC-007.*)* | STR-030, STR-029 |
| SYS-053 | The system SHALL support competition templates with **series-specific scoring tables** distinct from the World Athletics tables, shipping the **UBS Kids Cup** template built-in (3 disciplines — 60 m, zone long jump, 200 g ball throw; per-birth-year divisions M/W 7–15; UKC points table; combined standings) such that creating a UKC meet requires selecting the template and a date. Scoring tables SHALL be data, not code (CON-01). *(Later:* Visana Sprint, Mille Gruyère templates.*)* | STR-043, STR-016 |

### 3.5 Timing & measurement interfaces

| ID | Requirement | Traces to |
|----|-------------|-----------|
| SYS-060 | The system SHALL export start lists/schedules/competitor data in the FinishLynx file family (`lynx.ppl`, `lynx.sch`, `lynx.evt` per the published format, D7.2/C5) to a configurable local directory, regenerating on start-list changes. | STR-018 |
| SYS-061 | The system SHALL import FinishLynx `.LIF` result files (documented CSV field order incl. reaction times and splits) from a watched local directory, matching them to the correct unit by event/round/heat identifiers, presenting mismatches and conflicts (existing manual results, unknown bibs) for operator resolution rather than silently overwriting. | STR-018 |
| SYS-062 | The system SHALL provide documented generic CSV import/export for start lists and results as fallback interchange with non-Lynx timing systems (ALGE, TimeTronics — C5). | STR-018, STR-037 |
| SYS-063 | *(Later)* The system SHALL accept field-event measurements from electronic distance measurement (EDM) equipment via a documented interface once a target device protocol is selected (TBD: D7.1/EDM). | STR-018 |
| SYS-064 | *(Later)* The system SHALL provide a machine-readable live output (current standings/results per unit) suitable for driving venue scoreboards/displays. | STR-018, STR-022 |

### 3.6 Publication, documents & export

| ID | Requirement | Traces to |
|----|-------------|-----------|
| SYS-070 | The system SHALL serve public web pages per meet — timetable, start lists, live results, final results — readable without authentication or app installation, at stable URLs that remain valid after the meet (archive). | STR-022, STR-028 |
| SYS-071 | When internet connectivity is available, a result confirmed by the competition office SHALL be visible on public pages within **10 seconds** (p95); timetable/start-list publications within **60 seconds** (p95). | STR-022, STR-002 |
| SYS-072 | The system SHALL generate printable documents (PDF): start lists, per-unit heat/field sheets for manual capture, result lists (with statuses, wind, records flags, announcement timestamp), combined-events standings, bib lists, and the record-documentation checklist (SYS-051). | STR-023 |
| SYS-073 | Result data SHALL be exportable per meet in complete, documented, versioned open formats: CSV and a self-describing structured format (JSON schema published with the project); exports SHALL contain everything needed to reconstruct official results (marks, wind, statuses, categories, records flags, timestamps). | STR-025, STR-026, STR-037 |
| SYS-074 | The public results surface SHALL support the project launch languages (DE/FR per SYS-110) and SHALL be usable on phones (SYS-113); event/discipline/category labels SHALL render localized. | STR-022, STR-033 |
| SYS-075 | *(Later)* Public surfaces and printed documents SHALL support organizer-configured sponsor branding placements that do not obscure competition data. | STR-027 |
| SYS-076 | Per meet, the organizer SHALL configure the official-results positioning: when a federation channel is designated as official (default for sanctioned meets), every public results page and export intended for publication SHALL carry an "unofficial results" label plus a link/reference to the official source; when the system is designated the primary publication (e.g., unsanctioned meets), no such label appears. The label SHALL be rendered in the page language (SYS-110). | STR-042 |
| SYS-077 | The system SHALL produce results exports conforming to the official Swiss youth-series organizer-portal upload templates — UBS Kids Cup first; Visana Sprint and Mille Gruyère *(Later)* — the officially supported non-TAF3 results path at this tier (C2.1); template definitions SHALL ship as replaceable data files so a season's template revision is a data swap, not a code change (CON-01). | STR-043, STR-025, STR-042 |
| SYS-078 | *(Later)* The system SHALL export start lists and results in TAF3-ingestible formats — per-event FinishLynx-family `.lif` result files (the SYS-061 format, produced instead of consumed) and CSV structurally compatible with the publicly documented TAF3 result-export layout — so a sanctioned meet that must finish in TAF3 (C2.1 Alabus seam, OQ-014) can reuse data captured here rather than re-keying. | STR-018, STR-025, STR-037, STR-042 |

### 3.7 Offline operation, durability & concurrency

| ID | Requirement | Traces to |
|----|-------------|-----------|
| SYS-080 | *(Later — re-scoped 2026-07-05 per DEC-013/ADR-002 v2; delivered by the evidence-gated venue-node role)* All competition-day functions (check-in, seeding, capture, corrections, printing, timing exchange) SHALL be fully operable on a venue-local network with **no internet connectivity**, including cold start of the system while offline. MVP posture is hub-connected with interruption tolerance per SYS-085–087. | STR-020 |
| SYS-081 | Confirmed writes (entries, results, corrections) SHALL be durable against application crash, OS restart, and power loss: after restart, the system SHALL recover to a consistent state containing every confirmed write, within 5 minutes, without specialist intervention. | STR-041 |
| SYS-082 | *(Later — activates with the venue-node role, DEC-013: in hub-first MVP the hub publishes directly)* Public publications produced while offline SHALL queue automatically and publish in order once connectivity returns, without operator action; the operator SHALL be able to see the publication backlog state. | STR-020, STR-022 |
| SYS-083 | The system SHALL support at least **10 concurrent operator sessions** on one meet (competition office, infield capture, check-in) with edits to different entities never lost, and concurrent edits to the same entity detected and surfaced for resolution rather than silently last-write-wins. | STR-021 |
| SYS-084 | Backup SHALL be a single operator action (or scheduled) producing one portable artifact containing all meet data; restore from that artifact onto a fresh installation SHALL complete in ≤15 minutes and reproduce the full meet state (verified by checksum/consistency report). | STR-039, STR-041 |
| SYS-085 | Loss of connectivity between an operator device and the server SHALL NOT interrupt result capture: a device that has loaded its assigned event unit SHALL continue capturing attempts/results locally while disconnected, at least for the duration of that unit, and SHALL submit them automatically, in order, and idempotently on reconnection — no operator "sync" action, no duplicates on flaky reconnects, no loss across a page reload or browser restart while offline. This SHALL hold identically whether the transport is venue Wi-Fi or the device's own mobile data. | STR-020, STR-021, STR-041 |
| SYS-086 | Field capture SHALL use an **event-unit checkout** model: at most one device/account at a time holds the capture lock for a unit; the competition office SHALL be able to override/reassign a checkout (audited per SYS-046); captures from a superseded or stale device, and device captures affected by an office-side start-list change, SHALL be surfaced for reconciliation — **never silently discarded** (the documented Web.TEC 2 loss mode, C2.1, is a named anti-pattern). | STR-021, STR-040, STR-041 |
| SYS-087 | All operator surfaces SHALL degrade gracefully during connectivity interruptions: in-flight confirmed writes are never lost, the UI shows a clear offline/degraded indicator, and reconnection is automatic without restart, re-login, or data re-entry; interruptions ≤5 minutes SHALL require no recovery procedure of any kind. | STR-020, STR-041 |

### 3.8 Users, roles & security

| ID | Requirement | Traces to |
|----|-------------|-----------|
| SYS-090 | The system SHALL enforce role-based access with at least: instance admin, meet organizer, competition office, field/event official (scoped to assigned events), entry submitter (club/athlete), and unauthenticated public read; capability grants SHALL follow least privilege and be assignable per meet. | STR-040 |
| SYS-091 | Authentication SHALL be required for all non-public capabilities; credentials SHALL be stored only as modern adaptive hashes; sessions SHALL expire configurably; privileged actions SHALL appear in the audit trail (SYS-046). | STR-040, STR-031 |
| SYS-092 | The system SHALL meet OWASP ASVS Level 2 for all authenticated surfaces; releases SHALL ship with zero known critical/high vulnerabilities (own code and dependencies, verified by automated dependency and static analysis in CI). | STR-036, STR-038, STR-031 |
| SYS-093 | All access over non-local networks SHALL be encrypted in transit (current TLS); the venue-local mode SHALL NOT require internet-dependent certificate infrastructure to function (ties to SYS-080). | STR-031, STR-020 |

### 3.9 Privacy & data protection (nFADP / GDPR)

| ID | Requirement | Traces to |
|----|-------------|-----------|
| SYS-100 | Public surfaces and exports intended for publication SHALL expose at most: athlete name, club/team, nationality (where competition-relevant), category, bib, and competition data (marks, placings, records flags). Birth dates (beyond birth year where category-implied), licence numbers, contact data, and consent flags SHALL never appear on public surfaces. | STR-031, STR-032 |
| SYS-101 | The system SHALL support data-subject rights per nFADP/GDPR: per-person export of all stored personal data (machine-readable), rectification, and erasure/pseudonymization that removes identifying data while preserving the integrity of official competition results (name replaced by a neutral marker only where erasure is legally required and the sporting record allows). | STR-031 |
| SYS-102 | The system SHALL apply configurable retention: personal data not needed for the permanent sporting record (contact details, consent artifacts, audit PII) SHALL be automatically purgeable after a configured period post-meet; defaults SHALL be documented and privacy-protective. | STR-031, STR-032 |
| SYS-103 | The system SHALL record per-athlete publication-consent flags (esp. for minors: photo consent, extended-data consent) and SHALL enforce them on public surfaces automatically (e.g., suppressing a minor's result-page entry where consent is withdrawn and law requires suppression, while preserving the internal official result). | STR-032 |
| SYS-104 | The project SHALL ship operator-facing privacy documentation: a data-processing overview (what is stored where, lawful-basis notes for competition processing), a template privacy notice for meets, and guidance for operators acting as controllers. | STR-031, STR-038 |
| SYS-105 | In self-hosted deployment, all personal data SHALL remain on operator-controlled infrastructure; no telemetry or external service calls containing personal data SHALL occur without explicit opt-in. | STR-031, STR-037 |

### 3.10 Internationalization, accessibility & usability

| ID | Requirement | Traces to |
|----|-------------|-----------|
| SYS-110 | All user-facing text SHALL be externalized and translatable; **all surfaces (public, participant, operator) SHALL ship complete in DE and FR** at first release (DEC-008); adding a language SHALL require only a translation file (no code change — demonstrated by a pseudo-locale build in CI); users SHALL be able to switch language per session; locale-correct formatting SHALL apply (dates, decimal separators per locale convention — the incumbent renders `7,67`, C7.3; the chosen mark-notation convention SHALL be consistent and documented). | STR-033 |
| SYS-111 | Domain vocabulary (disciplines, categories, statuses, official document headings) SHALL be localized from a reviewable glossary so that official documents print correctly in the meet's configured language(s). | STR-033, STR-023 |
| SYS-112 | Public web surfaces SHALL conform to **WCAG 2.2 Level AA** (verified by automated checks plus a manual audit checklist per release). | STR-034 |
| SYS-113 | Public surfaces SHALL be responsive and legible on common phones (≥360 px width) including outdoor/high-brightness legibility (sufficient contrast per WCAG AA, no hover-only interactions). | STR-034, STR-022 |
| SYS-114 | The operator interface SHALL support efficient expert use: all high-frequency competition-office actions (check-in, result entry, status setting) operable keyboard-only, with bulk operations for multi-athlete actions. | STR-035 |
| SYS-115 | Every user-facing input or setting that is not self-explanatory to a first-time user SHALL carry a contextual-help affordance: a help icon ("?") adjacent to its label revealing a short localized explanation. Which inputs qualify SHALL be maintained as a reviewable, machine-readable **help-content registry** (at minimum: all domain-specific fields — e.g. wind reading, seeding method, countback, publication/consent toggles — and all fields with non-obvious format or consequence). The mechanism SHALL open on pointer hover, keyboard focus, **and** click/tap (never hover-only, cf. SYS-113); SHALL satisfy WCAG 2.2 SC 1.4.13 (dismissible via Escape without moving focus, hoverable, persistent); SHALL be exposed to assistive technology (focusable trigger with an accessible name; content programmatically associated with the trigger); and its content SHALL be localized like all UI text (SYS-110). Information *required* to complete the task (mandatory formats, units, legal notices) SHALL remain permanently visible as label or hint text and SHALL NOT exist only inside the help popup. | STR-044, STR-035, STR-034 |
| SYS-116 | All user-facing surfaces SHALL be built from a single in-repo, documented **design system**: named design tokens (color, typography, spacing, sizing, radius, elevation) and a reusable component inventory in which every interactive component defines its default, hover, focus-visible, active, disabled, and error states plus responsive behavior. Screens SHALL NOT introduce visual values (colors, font sizes, spacing) outside the tokens; conformance SHALL be enforced by a CI style-conformance check where mechanically checkable and by the release audit (SYS-117) otherwise. | STR-045, STR-038 |
| SYS-117 | User-facing surfaces SHALL follow documented interaction conventions, verified per release by a **usability audit checklist** (derived from published authoritative guidance: the Nielsen Norman Group usability heuristics and tooltip/contextual-help guidelines; GOV.UK Design System form patterns; WCAG 2.2) with zero open critical findings at release. The conventions SHALL include at minimum: every input has a permanently visible label (placeholder text is never the only label); constraints needed to complete a field (format, units, bounds) are visible as hint text of at most one short sentence; validation errors render inline at the field, state what to fix, and preserve the user's input; destructive or irreversible actions require explicit confirmation; long-running operations show visible progress/feedback. | STR-045, STR-035 |
| SYS-147 *(ratified 2026-08-07, DEC-027)* | Operator surfaces used at the point of competition (result capture, check-in, the assignments dashboard, unit standings) SHALL be fully operable on phones from 360 px viewport width: no horizontal page scrolling for the primary task flow; the first actionable row visible within the first viewport (360×640) without scrolling past page chrome; touch targets for primary actions ≥44×44 CSS px (and no interactive element below WCAG 2.2 SC 2.5.8's 24×24 px minimum); mark/time/wind inputs SHALL declare virtual-keyboard hints (`inputmode`, `enterkeyhint`) fitting their format while retaining the documented letter markers (X/–/r). | STR-046, STR-035 |
| SYS-148 *(ratified 2026-08-07, DEC-027)* | Every asynchronous point-of-capture save SHALL show its state at the entry itself — pending until server acknowledgment, then confirmed, or failed — distinguishable by more than color alone, within 500 ms of each state change; derived values displayed on the same page (result, points, rank) SHALL update automatically after a confirmed save without a manual reload. | STR-046, STR-041 |
| SYS-149 *(ratified 2026-08-07, DEC-027)* | The capture sync pipeline SHALL classify every queued operation's outcome as retryable (transport failure, server unavailable) or non-retryable (validation rejection, authorization/authentication failure); the sync protocol SHALL report rejections per operation, not per batch. Only retryable outcomes SHALL be retried automatically or presented as connectivity problems. A non-retryable rejection SHALL surface at the offending entry with a plain-language reason, SHALL NOT block other queued operations from syncing, and SHALL offer the operator correct-or-discard at the point of capture. An expired session SHALL prompt re-authentication with the queue preserved across re-login (no data re-entry, cf. SYS-087). Aggregate status indicators SHALL be consistent with per-entry states (never "all transferred" while operations are pending). | STR-046, STR-041, STR-020 |
| SYS-150 *(ratified 2026-08-07, DEC-027)* | Competition-office roles SHALL be able to correct a participant's identity data (name, birth year, sex, club, bib) after creation; corrections SHALL be optimistic-version-guarded, audited like result corrections (SYS-046), SHALL propagate to every surface and export (roster, start lists, capture, standings, series upload), including category re-derivation where birth year or sex changed, and SHALL NOT alter or re-score captured marks. (GDPR/nFADP accuracy and rectification also require an operator-side correction path.) | STR-035, STR-031 |
| SYS-151 *(ratified 2026-08-07, DEC-027)* | Navigation SHALL be task-first per role: each authenticated role's home SHALL link directly to that role's day-of-competition primary surfaces (competition office: check-in, result capture/reconciliation, roster, standings; field official: assigned units), each assignment labeled with localized discipline name and, where scheduled, time and location; the meet hub SHALL group its actions by task area (preparation, competition day, publication) rather than one undifferentiated list; and every operator surface SHALL be reachable through rendered links from its role's home (no typed-URL-only surfaces). | STR-035, STR-046 |
| SYS-152 *(ratified 2026-08-07, DEC-027)* | Every operator list surface's empty state SHALL state why it is empty and, where a next step exists, what it is; actions inapplicable in the current state — including bulk and destructive actions — SHALL be hidden or disabled with the reason shown; applicable bulk/destructive actions SHALL state their scope (affected row count) before or at their confirmation step. | STR-035, STR-045 |
| SYS-153 *(ratified 2026-08-07, DEC-027)* | Public start-list and result pages SHALL support finding a specific athlete on a phone: filtering by name, bib number or club, and per-category jump navigation; the pages SHALL remain functional without client-side scripting (progressive enhancement), and an applied filter SHALL survive live result updates. | STR-022, STR-034 |

### 3.11 Performance & scale

| ID | Requirement | Traces to |
|----|-------------|-----------|
| SYS-120 | The system SHALL handle, on commodity hardware (4-core CPU, 8 GB RAM class), a meet of at least **1,500 athletes, 4,000 entries, 250 event-units, 3 competition days** with all operator-visible interactions (list loads, entry search, result save) completing in ≤2 s (p95). | STR-004, STR-035 |
| SYS-121 | Seeding generation for an event with 200 entries (heats + lanes) SHALL complete in ≤10 s; recomputation of standings (combined events, team scores) after a result correction SHALL complete in ≤5 s. | STR-012, STR-016 |
| SYS-122 | When hosted with internet publication, the public results surface SHALL sustain at least **2,000 concurrent viewers** per meet with p95 page render ≤3 s on a documented reference hosting size; live-update latency per SYS-071. | STR-022 |

### 3.12 Reliability & operability

| ID | Requirement | Traces to |
|----|-------------|-----------|
| SYS-130 | After an application or host failure during a meet, the system SHALL be operational again within **2 minutes** of host availability (automatic restart/recovery), with state per SYS-081. | STR-041, STR-020 |
| SYS-131 | A first-time operator following the quickstart SHALL get from installation media/download to a working system ready for meet setup in ≤30 minutes on a supported platform, without editing configuration files. | STR-039, STR-035 |
| SYS-132 | The system SHALL run on commodity operator hardware for venue-local mode (single laptop-class machine serving the venue LAN) and SHALL be deployable on mainstream Linux servers for hosted mode; operator clients SHALL require only a current mainstream web browser (documented support matrix, at minimum latest two major versions of the mainstream engines). | STR-039, STR-036 |
| SYS-133 | The system SHALL NOT require any paid third-party service, licence, or account for full self-hosted operation. | STR-036 |

### 3.13 Maintainability, quality & OSS governance

*(These operationalize the founder's "state of the art, well coded, well tested, bug free".)*

| ID | Requirement | Traces to |
|----|-------------|-----------|
| SYS-140 | Every use-case acceptance criterion (UC-###) SHALL be covered by an automated test; domain-logic modules (seeding, scoring, progression, eligibility, records) SHALL reach ≥90% branch coverage; overall project line coverage SHALL be ≥83%, enforced in CI. *(Floor ratcheted 80%→83% at TASK-033 after the measured baseline reached 83.9%; the gate and this floor move together — see `scripts/check-coverage.sh`.)* | STR-038 |
| SYS-141 | CI SHALL gate every change to the main branch on: successful build, static type check with zero errors, linter with zero errors, full test suite green, dependency vulnerability scan with no new critical/high findings. | STR-038 |
| SYS-142 | Rule-defined computations (scoring tables, hand-time rounding, wind legality, countback, progression, category assignment) SHALL each have reference test suites with documented expected values from the primary rule sources (D-references), so rule conformance is machine-verified. | STR-014–STR-016, STR-024, STR-038 |
| SYS-143 | The project SHALL maintain a public defect policy: no release with known critical/high defects; every fixed defect SHALL gain a regression test; defect severity definitions SHALL be documented. | STR-038 |
| SYS-144 | All external interfaces (import/export schemas, public URLs, any APIs) SHALL be versioned and documented in-repo; breaking changes SHALL follow a documented deprecation policy (semantic versioning). | STR-037, STR-038 |
| SYS-145 | Architecture SHALL be documented as a living baseline plus ADRs (`docs/architecture/`); every one-way-door decision SHALL have an ADR before code depends on it. | STR-038 |
| SYS-146 | The repository SHALL contain: an OSI-approved LICENSE (founder decision OQ-004), CONTRIBUTING guide, code of conduct, maintainer/governance documentation, and a documented release process — before the first public release. | STR-036, STR-038 |

## 4. External interface summary

| Interface | Direction | Form | Requirements |
|-----------|-----------|------|--------------|
| Federation entries (Alabus/Swiss Athletics) | in | file import (CSV; mapping profile) | SYS-013 |
| Timing — FinishLynx family | out/in | `lynx.ppl`/`.sch`/`.evt` out; `.lif` in; local directory | SYS-060, SYS-061 |
| Timing — generic | out/in | documented CSV | SYS-062 |
| Federation results delivery | out | CSV + structured export, printable PDF | SYS-073, SYS-072 |
| Swiss youth-series portals (UBS Kids Cup; Visana Sprint/Mille Gruyère *Later*) | out | results file per official organizer template (shipped as data) | SYS-077 |
| TAF3 coexistence | out | *(Later)* per-event `.lif` + TAF3-compatible result CSV | SYS-078 |
| Public web | out | HTML pages, stable URLs, 4 languages | SYS-070–SYS-074 |
| Scoreboards | out | *(Later)* machine-readable live feed | SYS-064 |
| EDM devices | in | *(Later)* device protocol TBD | SYS-063 |

## 5. Constraints

| ID | Constraint | Source |
|----|-----------|--------|
| CON-01 | Rule conformance follows World Athletics CR&TR (2024 ed. at baseline) and Swiss Athletics WO 2026; rule parameters that change annually (categories, standards) MUST be data, not code. | D1–D8 |
| CON-02 | Licence: **AGPL-3.0-only + DCO, ratified 2026-07-05** (ADR-001 Accepted, DEC-004/DEC-014); OSI-approved per STR-036. | STR-036 |
| CON-03 | Privacy law: nFADP (CH) and GDPR (EU) both apply as design constraints. | STR-031 |
| CON-04 | Competition-day operation SHALL NOT depend on **uninterrupted** internet connectivity: interruptions must not stop capture or lose data (SYS-085–087). Full no-internet operation is the deferred venue-node role (SYS-080 *Later*, DEC-013); field-wide venue Wi-Fi is an explicit non-goal (ADR-002 v2). | STR-020, DEC-013 |
| CON-05 | Phase discipline: no implementation/stack choices in this document; stack, storage, and sync design are Phase B ADRs ratified by the founder. | CLAUDE.md gate |

## 6. Assumptions

Inherited from `open-questions-and-assumptions.md`: A-001 (stadium athletics first),
A-002 (club→regional MVP scale), A-003 (interoperate, no mandate), A-006 (single-organizer
deployment first, SaaS later), A-007 (para data-model-ready), A-008 (language staging),
A-011 (Alabus import via generic mapping until sample available), A-012 (self-defined open
exchange schema, W3C draft as reference).
