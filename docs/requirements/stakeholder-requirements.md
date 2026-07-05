# Stakeholder Requirements Specification (StRS)

**Project:** Open-Source Athletics Tournament Management System (working name: *OpenMeet*, TBD)
**Document status:** DRAFT — Phase A baseline
**Standard:** ISO/IEC/IEEE 29148:2018 (stakeholder requirements layer)
**Locale/regulatory context:** Switzerland / EU (nFADP, GDPR), multilingual DE/FR/IT/EN

> Stakeholder requirements are written in stakeholder language and are **implementation-free**.
> `SHALL` marks mandatory requirements. IDs are stable: never renumbered, only deprecated.
> Each requirement records **Source** (stakeholder class or founder seed) and **Rationale**.
> Traceability to system requirements is maintained in `traceability-matrix.md`.

---

## 1. Purpose and scope

### 1.1 Purpose
Provide the stakeholder-level requirements baseline for an open-source system to manage
athletics (track & field) tournaments — from meet announcement and entries through
competition-day operation to official results, records, and federation reporting.

### 1.2 Founder seed statement (verbatim source)
> "I want to create a state of the art, modern, well coded, well tested, bug free, open source
> system to manage athletics tournaments. Parallel systems to take into account are Swiss
> Athletics and Seltec Sports."

Vague terms in the seed ("state of the art", "well coded", "well tested", "bug free") are
converted into measurable quality requirements in the SyRS (see `system-requirements.md`);
they are **not** used as requirements language here.

### 1.3 Business context
Athletics meets in Switzerland and the EU are today run predominantly on proprietary,
Windows-centric tooling (see `../research/competitive-analysis.md`). An open-source,
modern alternative targets: club and regional meets first (MVP), with a path to national
championship-grade operation; openness of data; multilingual Swiss operation; and reliable
on-site operation with unstable venue connectivity.

## 2. Stakeholder identification and analysis

Stakeholder classes, their goals, needs, pain points, and constraints. Class codes are used
as **Source** references by the STR items in section 3.

| Code | Stakeholder class | Relationship to system |
|------|-------------------|------------------------|
| SH-ATH | Athletes | Compete; enter meets; consume start lists/results; own personal data |
| SH-COA | Coaches | Enter/manage athletes; monitor schedules and results |
| SH-CLB | Clubs (member organizations) | Enter teams/athletes; pay fees; host meets |
| SH-ORG | Meet organizers / organizing committee | Announce, plan, staff, and run meets; own the event |
| SH-SEC | Competition office / secretary | Operate the system on meet day: entries, seeding, results capture |
| SH-OFF | Officials & judges (incl. referee, starter, jury of appeal) | Conduct events per rules; record and sign results; handle protests |
| SH-TIM | Timing & measurement providers/crews | Photo-finish, wind, EDM; exchange start lists and results with the system |
| SH-FED | Federations (Swiss Athletics; World Athletics indirectly) | Sanction meets; licences; categories; records; receive official results |
| SH-SPE | Spectators | Follow live start lists, results, schedules on their own devices |
| SH-MED | Media | Timely, quotable results and stats; data feeds |
| SH-SPO | Sponsors | Visibility on public surfaces (results pages, boards) |
| SH-PAR | Para athletes & classifiers | Compete in integrated meets; correct classes and scoring |
| SH-OPS | System operators / hosts / admins | Deploy, configure, back up, and support the system |
| SH-DEV | OSS contributors & maintainers | Understand, extend, and maintain the codebase and project |

### 2.1 Athletes (SH-ATH)
- **Goals:** compete in the right events/categories; know when and where to compete; get correct results and records credit.
- **Needs:** simple entry (or entry via club/coach); visible entry status; accurate seeding info; timely start lists and results; personal data handled lawfully.
- **Pain points:** late/opaque timetables; wrong categories; results errors discovered too late for protest; PBs/records not credited; personal data published without control.
- **Constraints:** licence required for sanctioned meets; category determined by birth year; may compete in a language other than their own (DE/FR/IT/EN).

### 2.2 Coaches (SH-COA)
- **Goals:** get athletes entered correctly and on time; plan warm-up around a reliable timetable.
- **Needs:** bulk entries; view of entry standards and seeding; schedule change notifications; access to results for planning.
- **Pain points:** entry deadlines/formats differing per meet; no visibility of who is entered; day-of schedule slips not communicated.
- **Constraints:** manages many athletes across categories; often works from a phone at the venue.

### 2.3 Clubs (SH-CLB)
- **Goals:** enter squads efficiently; correct billing of entry fees; host their own meets affordably.
- **Needs:** club-level entry management; relay team composition; fee overview/invoicing; when hosting: affordable tooling that volunteers can operate.
- **Pain points:** licence/membership data re-entry; per-seat or per-event pricing of incumbent tools; dependence on the one volunteer who knows the software.
- **Constraints:** volunteer-run; limited budget and IT skills.

### 2.4 Meet organizers / organizing committee (SH-ORG)
- **Goals:** deliver a meet that runs on time, per rules, within budget; get sanctioning; publish credible results quickly.
- **Needs:** meet announcement & event programme setup; entry management with limits/standards; timetable planning; staffing/officials assignment overview; sponsor visibility on public surfaces; a system that keeps working when venue internet drops.
- **Pain points:** juggling several disconnected tools (entries portal, seeding tool, timing software, website); Windows-only tooling; single-operator bottlenecks; recovering from crashes mid-meet.
- **Constraints:** meets range from ~50-athlete club evenings to multi-day championships with 1000+ athletes; strict competition rules; fixed dates — no second chance.

### 2.5 Competition office / secretary (SH-SEC)
- **Goals:** correct entries → seedings → heats → results with minimal manual work under time pressure.
- **Needs:** fast bulk operations (check-in, DNS handling, reseeding); rule-conformant automatic seeding with manual override; capture of manual results where no timing link exists; audit trail of changes.
- **Pain points:** re-typing data between systems; last-minute entries and scratches; undoing mistakes under pressure; knowledge concentrated in one expert user.
- **Constraints:** high workload peaks on meet day; operates the system continuously for hours; may be offline.

### 2.6 Officials & judges (SH-OFF)
- **Goals:** run events per World Athletics / Swiss Athletics rules; record valid, signed results; resolve protests correctly.
- **Needs:** correct event/heat sheets at the right place and time; field-event result capture (attempts, heights, wind); clear status codes (DNS/DNF/DQ/NM/r); protest/appeal handling within rule deadlines.
- **Pain points:** paper shuffling between infield and competition office; illegible/mis-keyed cards; result changes after protests not propagating everywhere.
- **Constraints:** rules prescribe procedures and deadlines; many officials are older volunteers — low tolerance for fiddly UIs; infield positions may lack connectivity.

### 2.7 Timing & measurement providers (SH-TIM)
- **Goals:** deliver certified times/measurements into the official results with zero transcription.
- **Needs:** exchange of start lists/events to the timing system and results back, via the file/protocol formats their equipment speaks; clear ownership of the authoritative time.
- **Pain points:** vendor-specific one-off integrations; manual re-keying when formats mismatch; ambiguity about which system holds the official result.
- **Constraints:** heterogeneous hardware (photo-finish, wind gauges, EDM, scoreboards); local network on-site; timing crew is often a third party.

### 2.8 Federations (SH-FED)
- **Goals:** rule-conformant, sanctioned competition; trustworthy results into national records/rankings and onward to World Athletics.
- **Needs:** meets configured with federation-conformant categories and rules; licence validation of entrants; official results in the formats/channels the federation requires; record flagging.
- **Pain points:** non-conformant or incomplete result submissions; unofficial category schemes; results published that were never sanctioned.
- **Constraints:** Swiss Athletics category system and licence regime; World Athletics technical rules; existing federation infrastructure the system must fit into, not replace.

### 2.9 Spectators (SH-SPE)
- **Goals:** follow friends/family and the competition live and after the fact.
- **Needs:** public schedule, start lists, and live results on their own phones without installing anything; accessible presentation.
- **Pain points:** venue PA/paper as only information source; results appearing hours later; illegible mobile pages.
- **Constraints:** congested venue networks; wide device/ability range.

### 2.10 Media (SH-MED)
- **Goals:** report quickly with correct names, marks, and context (records, standings).
- **Needs:** near-real-time results, printable/exportable formats, stable public URLs, data feed where possible.
- **Pain points:** waiting on PDFs; retyping from photos of printouts.
- **Constraints:** deadlines minutes after events end.

### 2.11 Sponsors (SH-SPO)
- **Goals:** visibility proportional to sponsorship.
- **Needs:** logo/name placement on public result surfaces and printouts.
- **Pain points:** tooling with no sponsor surface at all.
- **Constraints:** organizer-controlled; must not degrade usability.

### 2.12 Para athletes & classifiers (SH-PAR)
- **Goals:** compete in integrated meets with correct sport classes and fair scoring.
- **Needs:** para classes recorded per World Para Athletics; RAZA/points where used; accessible interfaces.
- **Pain points:** mainstream meet software treating para events as free-text exceptions.
- **Constraints:** classification is externally governed; small event fields mixed across classes.

### 2.13 System operators / admins (SH-OPS)
- **Goals:** run the system reliably for one meet or as a hosted service, with low effort.
- **Needs:** straightforward deployment; backup/restore; user/role management; monitoring; lawful data processing support (retention, deletion, export).
- **Pain points:** snowflake installs; no upgrade path; being the de-facto data controller without tooling support.
- **Constraints:** may be a volunteer with one laptop, or a hosting provider serving many meets; nFADP/GDPR obligations.

### 2.14 OSS contributors & maintainers (SH-DEV)
- **Goals:** a project worth contributing to that survives its founders.
- **Needs:** clear licence and governance; documented architecture and domain model; automated tests and CI that gate changes; approachable first-contribution path.
- **Pain points:** unclear licensing; untested legacy cores; domain knowledge locked in maintainers' heads.
- **Constraints:** volunteer time; multilingual, niche domain.

## 3. Stakeholder requirements (STR)

Conventions: **Source** = stakeholder class(es) (§2) and, where applicable, a research
reference (`D#` = `../research/domain-athletics.md`, `C#` = `../research/competitive-analysis.md`,
`A-###`/`OQ-###` = `open-questions-and-assumptions.md`). **Priority**: `MVP` (first usable
release) or `Later` (post-MVP, in scope). Out-of-scope items are listed in §4.
Requirements are implementation-free: they say what stakeholders need, not how it is built.

### 3.1 Meet lifecycle & sanctioning

| ID | Requirement | Rationale | Source | Priority |
|----|-------------|-----------|--------|----------|
| STR-001 | Meet organizers SHALL be able to define a meet — name, venue, date(s), sessions, offered disciplines per category, entry deadlines and conditions — themselves, without vendor or expert assistance. | Volunteer-run clubs cannot depend on paid support; incumbent tooling concentrates this knowledge in single experts. | SH-ORG, SH-CLB; C6 | MVP |
| STR-002 | Meet organizers SHALL be able to plan, publish, and amend the competition timetable, with amendments becoming visible to participants and officials promptly. | Day-of schedule slips are a recurring stakeholder pain point; protest deadlines and warm-up planning hang off the timetable. | SH-ORG, SH-COA, SH-OFF; D1.3 | MVP |
| STR-003 | Meet organizers SHALL be able to record and produce the information a federation requires to sanction the meet (meet tier, venue, categories/disciplines, dates) in time for the federation's advance-registration deadline (Swiss Athletics: ≥30 days). | Sanctioning is a precondition for licensed competition and record eligibility; the system must support, not replace, the federation's process. | SH-ORG, SH-FED; D3.4, D10.1, C2 | MVP |
| STR-004 | The system SHALL support meets ranging from single-session club evenings to multi-day, multi-session championships. | The realistic adoption path starts at club meets but must not dead-end below championship scale. | SH-ORG; D1.2, A-002 | MVP |

### 3.2 Entries & registration

| ID | Requirement | Rationale | Source | Priority |
|----|-------------|-----------|--------|----------|
| STR-005 | Athletes, coaches, and clubs SHALL be able to submit entries online before the entry deadline — including relay teams — and see the status of their entries at any time. | Entry opacity is a top athlete/coach pain point; club-level bulk entry is how most entries actually happen. | SH-ATH, SH-COA, SH-CLB | MVP |
| STR-006 | The competition office SHALL be able to take over entries provided through federation entry channels (e.g., a Swiss Athletics/Alabus entries export) without re-typing athlete data. | The documented Swiss flow forces manual re-entry between systems today; transcription is the main error source. | SH-SEC, SH-FED; C2, OQ-011 | MVP |
| STR-007 | Entries SHALL be checked against the applicable eligibility rules — age category by birth year, licence requirement for the meet tier, category start-up/start-down rules, and youth-protection limits (maximum distances, barred disciplines) — with violations flagged before competition day. | Eligibility errors discovered on meet day cause DQs and protests; the rules are precise and checkable. | SH-SEC, SH-FED, SH-ATH; D3.1, D4.2, D4.3 | MVP |
| STR-008 | The competition office SHALL be able to process late entries, scratches, and entry changes on meet day per the organizer's policy, with every change recorded (who, what, when). | Late churn is universal; an audit trail protects officials in protest situations. | SH-SEC, SH-OFF | MVP |
| STR-009 | Meet organizers SHALL be able to see entry-fee obligations per club and athlete, suitable for invoicing, and record the federation starting-fee levy data owed after the meet. | Fee handling exists in every real meet; the Swiss levy report is a mandatory post-event duty. | SH-ORG, SH-CLB; C2, D3.6 | MVP (overview); payment processing Later |
| STR-010 | Meet organizers SHALL be able to assign athlete bib numbers, individually and in bulk. | Bibs are the physical join key between call room, timing, and results. | SH-ORG, SH-SEC; D3.2 | MVP |

### 3.3 Competition-day operation

| ID | Requirement | Rationale | Source | Priority |
|----|-------------|-----------|--------|----------|
| STR-011 | The competition office SHALL be able to record athlete presence (check-in / call-room confirmation) and mark non-reporting athletes DNS, feeding directly into final start lists. | Call-room confirmation determines the actual field; DNS handling is rule-defined. | SH-SEC, SH-OFF; D5.2, D11.1 | MVP |
| STR-012 | The competition office SHALL be able to produce seedings — heats, lane assignments, and field-event start orders/flights — that conform to the applicable World Athletics rules, with the ability to review and manually override any automatic assignment before publication. | Rule-conformant seeding (TR20 lane groups, same-club separation) is skilled, error-prone manual work today; overrides are needed because the referee has final authority. | SH-SEC, SH-OFF; D2.2, D2.3 | MVP |
| STR-013 | The competition office SHALL be able to advance athletes to subsequent rounds according to the configured progression (places, fastest losers, referee/jury advancement), producing the next round's start lists. | Multi-round events are standard at championships; progression codes (Q/q/qR/qJ/qD) are rule-defined. | SH-SEC; D2.4, D5.2 | MVP |
| STR-014 | Officials and the competition office SHALL be able to record track-event results — times, placings, wind readings, and result statuses (DNS, DNF, DQ with rule reference, etc.) — for each heat. | Core purpose of the system; status codes are rule-defined and appear on official results. | SH-OFF, SH-SEC; D5.1–D5.3 | MVP |
| STR-015 | Officials SHALL be able to record field-event competitions attempt by attempt — distances, heights with bar progression (O/X/–), per-attempt wind where applicable, retirement (r) — with standings visible as the event unfolds. | Field events are live, incremental competitions; paper round-trips to the competition office are a documented pain point. | SH-OFF; D5.2, D11.3 | MVP |
| STR-016 | The system SHALL score combined events per the official World Athletics scoring tables, showing per-discipline points and running totals after each discipline. | Combined events are ubiquitous in Swiss youth athletics; manual table lookup is slow and error-prone. | SH-SEC, SH-ATH, SH-COA; D5.4 | MVP |
| STR-017 | Meet organizers SHALL be able to run team/club-scored competitions with a configurable scoring scheme (e.g., SVM-style placing points), with team standings computed from individual results. | Inter-club competition is a pillar of Swiss athletics; exact tables vary and change, so the scheme must be configurable. | SH-ORG, SH-CLB; D5.5, TBD-005 | Later |
| STR-018 | Official times and measurements SHALL flow between the timing/measurement systems in use at the venue and the competition record without manual transcription, in both directions (start lists out, results in). | Transcription between meet management and timing is the biggest single error and delay source; local file exchange (e.g., FinishLynx .LIF family) is the industry pattern. | SH-TIM, SH-SEC; D7.2, C5 | MVP |
| STR-019 | The competition office SHALL be able to correct results (including after protests and appeals), with the correction propagating to every place the result appears, the official announcement time of each result recorded, and a complete history of changes preserved. | The 30-minute protest/appeal clocks run from announcement; corrections that don't propagate produce inconsistent official records. | SH-OFF, SH-SEC, SH-ATH; D8.3, D11.3 | MVP |
| STR-020 | All meet-day operations SHALL remain fully usable at the venue when internet connectivity is degraded or absent; public results SHALL catch up automatically once connectivity returns. *(Staged per DEC-013, 2026-07-05: the connectivity posture is "expect a central self-hosted hub, tolerate interruptions" — tolerance to degraded/interrupted connectivity is MVP; operation with internet fully absent is the deferred venue-node role.)* | Venue connectivity is unreliable in practice; the timing-integration pattern is local; a meet cannot pause for the network. Club-tier incumbents run field capture over officials' phones + mobile data via a cloud relay (C8) — full-day offline is the rarer case (indoor halls, dead zones). | SH-ORG, SH-SEC, SH-TIM; D11.4, C6, C8, DEC-013 | MVP (interruption tolerance — SYS-085–087); Later (fully offline venue — SYS-080) |
| STR-021 | Several operators SHALL be able to work on the same meet at the same time (e.g., competition office plus infield field-event capture) without overwriting each other's work. | Real meets are run by teams; single-operator bottlenecks are a documented incumbent weakness; founder observed live capture in a results room while slips arrived all day (C7.3). | SH-SEC, SH-OFF; C1.2 (Web.TEC pattern), C7.3 | MVP |
| STR-043 | The system SHALL support Swiss youth-series competition formats — first the **UBS Kids Cup** (3 disciplines: 60 m sprint, zone long jump, 200 g ball throw; per-birth-year divisions; series-specific points table) — as configurable competition templates with their own scoring tables, so a small club can run a series meet without expert setup. | Youth-series local eliminations are exactly the small-club, volunteer-run MVP scenario (DEC-002); the founder's observed meet was a UKC; scoring is NOT the WA tables (C7.2). | SH-CLB, SH-ORG, SH-FED; C7.2, DEC-002 | MVP (UKC); Later (Visana Sprint, Mille Gruyère templates) |

### 3.4 Results, records & publication

| ID | Requirement | Rationale | Source | Priority |
|----|-------------|-----------|--------|----------|
| STR-022 | Spectators, athletes, and coaches SHALL be able to follow the timetable, start lists, and results of a public meet live on their own web-connected devices, without installing an application or creating an account. | Primary spectator need; incumbent live platforms prove the demand; account walls exclude casual followers. | SH-SPE, SH-ATH, SH-COA; C1.2 | MVP |
| STR-023 | The competition office SHALL be able to produce printable official documents: start lists, heat/field-event sheets (for manual capture fallback), result lists, and record documentation. | Paper remains the rule-mandated fallback and the official artifact for signatures and posting; every incumbent supports this baseline. | SH-SEC, SH-OFF; C6, D11.3 | MVP |
| STR-024 | The system SHALL flag performances that equal or better applicable records and bests (meeting records, national/area/world where reference data is loaded, personal/season bests where history is known), and SHALL assemble the documentation a record claim requires (e.g., Swiss Rekordprotokoll data: timing homologation, zero-test, wind, finish image reference). | Records are the sport's currency; missed record paperwork means lost ratification (30-day WA deadline). | SH-ATH, SH-FED, SH-ORG; D6.1, D6.2 | MVP (flagging); Later (full dossier assembly) |
| STR-025 | Meet organizers SHALL be able to deliver official results to the federation(s) in a form the federation accepts, promptly after the meet (World Athletics expectation: ideally within 24 hours for ranking-relevant meets). | Result submission closes the sanctioning loop; without it results don't count for rankings/records. | SH-FED, SH-ORG; D10.2, C3, TBD-007 | MVP (export); Later (direct submission if a channel exists) |
| STR-026 | Media SHALL be able to obtain complete, correctly-attributed results promptly after each event, in re-usable (copy/export-friendly) form. | Media deadlines are minutes after events; retyping from PDFs is the pain point. | SH-MED | MVP (via public results + export); Later (dedicated feed) |
| STR-027 | Meet organizers SHALL be able to give sponsors visibility on public-facing surfaces (live results pages, printed lists) under organizer control. | Sponsorship funds the meets; tooling without sponsor surfaces loses organizers money. | SH-SPO, SH-ORG | Later |
| STR-028 | Results of past meets SHALL remain publicly accessible after the meet ends, at stable addresses. | Results are the durable output; athletes/media/statisticians rely on them for years. | SH-ATH, SH-MED, SH-SPE | MVP |
| STR-042 | Where a meet's official results are published through a federation channel (today: Seltec LA.portal embedded in swiss-athletics.ch), the system's own public results SHALL be clearly labeled as unofficial/supplementary, SHALL name the official source, and SHALL NOT claim official status; organizers SHALL be able to configure this labeling per meet (e.g., disable it for unsanctioned meets where the system IS the primary publication). | The federation surface is the recognized source of truth for sanctioned meets; the pipeline into it is Seltec-only (research C7.1); families need fast access without being misled about status. | Founder (field report 2026-07-04), SH-SPE, SH-FED; C7, OQ-013 | MVP |

### 3.5 Categories & para athletics

| ID | Requirement | Rationale | Source | Priority |
|----|-------------|-----------|--------|----------|
| STR-029 | Age/gender category schemes SHALL be definable per federation/competition — including category codes, birth-year bounds, calendar-year transitions, and start-up/start-down eligibility — with the Swiss Athletics scheme (U10…U23, Men/Women, Masters 5-year bands) available as a built-in default. | WA formally defines only U18/U20/Masters; national schemes differ and change; hard-coding one scheme blocks non-Swiss adoption. | SH-FED, SH-ORG; D4.1, D4.2 | MVP |
| STR-030 | The system SHALL support mixed **age-category** fields (e.g., combined U16/U18 races, youth-series divisions) whose results are presented split and re-ranked per category group. Para sport classes and para-specific workflows are **out of MVP scope** (DEC-007); the data model SHALL NOT preclude adding sport classes later. | CR 25.2–25.3 permits mixed fields; youth-series meets rely on them. Founder: local meets are not para-athletic (DEC-007). | SH-ORG, SH-FED; D9.2, DEC-007 | MVP (mixed age categories); Later (para classes) |

### 3.6 Privacy & data protection

| ID | Requirement | Rationale | Source | Priority |
|----|-------------|-----------|--------|----------|
| STR-031 | Personal data of participants SHALL be processed lawfully under the Swiss nFADP and the EU GDPR: collected minimally for competition purposes, with data-subject rights (access, rectification, erasure where compatible with the sporting record) supported, and with clarity about what is published publicly versus held internally. | Legal obligation on organizers/operators; results publication is legitimate but must be bounded (e.g., no birth dates or licence numbers on public pages). | SH-ATH, SH-OPS, SH-FED | MVP |
| STR-032 | Data of minors (a large share of participants: U10–U18) SHALL receive heightened protection: only competition-necessary data published, and support for handling parental/guardian consent where publication goes beyond the necessary sporting record (e.g., photos, birth dates). | Most Swiss meet participants are youth categories; nFADP/GDPR treat minors' data with special care. | SH-ATH (minors), SH-CLB, SH-OPS; D4.2 | MVP |

### 3.7 Accessibility, language & usability

| ID | Requirement | Rationale | Source | Priority |
|----|-------------|-----------|--------|----------|
| STR-033 | All user-facing surfaces (public, participant, and operator) SHALL be available complete in **German and French** at first release, with the language switchable by the user; the system SHALL be built so adding a further language (Italian, English, …) is a translation task requiring no engineering change. | Founder decision DEC-008; Swiss bilingual reality first, full CH quadrilingual as growth; incumbents' bolt-on i18n is a documented weakness (C6, C7.3). | SH-ATH, SH-SPE, SH-FED; C6, DEC-008 | MVP (DE/FR); Later (IT/EN packs) |
| STR-034 | Public-facing surfaces SHALL be usable by people with disabilities (assistive-technology compatible, legible in venue conditions such as bright sunlight on phones). | Spectators and para athletes span the full ability range; accessibility is also a public-sector adoption criterion. | SH-SPE, SH-PAR | MVP |
| STR-035 | A first-time operator with domain knowledge (a club volunteer who understands athletics) SHALL be able to run a simple meet after at most a half-day of self-guided preparation; frequent operations SHALL be efficient for experienced users (bulk actions, keyboard-friendly). | "The one volunteer who knows the software" is the single point of failure the system must remove. | SH-SEC, SH-ORG, SH-CLB; C6 | MVP |

### 3.8 Openness & sustainability

| ID | Requirement | Rationale | Source | Priority |
|----|-------------|-----------|--------|----------|
| STR-036 | The system SHALL be released under an OSI-approved open-source licence, and SHALL be usable free of licence cost by any organizer hosting it themselves. | Founder seed ("open source"); transparent cost is a differentiator against federation-bundled pricing opacity. | Founder, SH-CLB, SH-DEV; C6, OQ-004 | MVP |
| STR-037 | All competition data SHALL be exportable by its owner in complete, documented, open formats at any time; no capability of the system may depend on data the owner cannot get back out. | No-lock-in is the core OSS promise and a direct answer to the fragmented proprietary landscape. | SH-ORG, SH-FED, SH-DEV; C6, A-012 | MVP |
| STR-038 | The project SHALL be organized so that new contributors can understand, build, test, and safely change it: documented architecture and domain model, automated tests gating changes, a documented contribution and governance process. | An OSS project without contributor infrastructure dies with its founder; this operationalizes "well coded / well tested" at project level. | SH-DEV, Founder; OQ-005 | MVP |

### 3.9 Operation & hosting

| ID | Requirement | Rationale | Source | Priority |
|----|-------------|-----------|--------|----------|
| STR-039 | A technically ordinary operator SHALL be able to install the system for a single meet on commodity hardware (e.g., one laptop at the venue), and to back up and restore all meet data. | The MVP deployment reality is one volunteer, one laptop; backup/restore is the last line of defense mid-meet. | SH-OPS, SH-ORG; A-006 | MVP |
| STR-040 | Access to system capabilities SHALL be controlled by role (e.g., organizer, competition office, field-event official, read-only public), so that officials and helpers can be given exactly the access their function needs. | Meets involve many temporary helpers; least-privilege prevents accidents and satisfies data-protection duties. | SH-OPS, SH-ORG, SH-OFF | MVP |
| STR-041 | Once captured, competition data (entries, results, corrections) SHALL survive application crashes, device restarts, and power loss without loss of any confirmed data. | A meet cannot be re-run; incumbent lore (fresh database per meet, 48h auto-lock) shows how fragile the status quo is. | SH-SEC, SH-ORG; C1.3 | MVP |

## 4. Scope boundaries

*(Updated 2026-07-05 per founder decisions DEC-001…DEC-012.)*

### 4.1 In scope, MVP
Stadium (outdoor/indoor) track & field meets end-to-end, optimized for **small volunteer-run
clubs with no IT budget** (DEC-002): meet setup → entries (online + import) → eligibility
checks → seeding → competition-day capture (track, field, combined events incl. **UBS Kids
Cup format**) → timing-system exchange → results, records flagging, corrections/protest
support → live public results (**DE/FR**, labeled unofficial where a federation channel is
the official source) → printable documents → federation-acceptable result export → archive.
Offline-capable venue operation; role-based multi-operator use; single self-hosted
deployment (founder homelab first, DEC-006).

### 4.2 In scope, Later
Additional language packs (IT/EN — translation-only by design); further youth-series
templates (Visana Sprint, Mille Gruyère); team/club competition scoring (SVM-style); payment
processing for entry fees; sponsor surfaces; dedicated media feeds; full record-dossier
assembly; para athletics (sport classes, para scoring — DEC-007); direct machine-to-machine
federation submission (pending TBD-007/OQ-013); multi-tenant hosted (SaaS) operation at
modest cost.

### 4.3 Out of scope
Road running, cross-country, and trail events (DEC-001); replacing federation systems of
record (licensing/member management à la Alabus, federation calendars, World Athletics Global
Calendar, the Seltec LA.portal official-results channel); timing hardware control (the system
exchanges data with timing systems, it does not operate cameras/guns); doping control
management; athlete coaching/training analytics; general club administration (finances,
memberships); ticketing; venue homologation management; live video/streaming production.
