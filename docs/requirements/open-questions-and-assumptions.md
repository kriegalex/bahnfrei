# Open Questions & Assumptions

**Document status:** DRAFT — updated continuously through Phase A; consolidated at the A6 QA pass.
Open questions are addressed to the **founder** unless another owner is named. Where an answer
is pending, work proceeds on the stated assumption (marked **A-###**); assumptions are
requirements-relevant and traced where used.

## 1. Open questions for the founder (OQ-###)

| ID | Question | Why it matters | Working assumption until answered |
|----|----------|----------------|-----------------------------------|
| OQ-001 | **Scope of "athletics tournaments":** track & field stadium meets only, or also road running, cross-country, trail, and indoor meets? | Determines domain model breadth (courses, transponder timing, mass fields) | A-001 |
| OQ-002 | **Primary initial user:** which meet scale is the MVP target — club/regional meets, or national-championship grade from day one? | Drives MVP cut, performance targets, and feature depth (e.g., call room, jury workflows) | A-002 |
| OQ-003 | **Relationship to Swiss Athletics:** is the goal (a) interoperate with their existing ecosystem, (b) be adoptable by them, or (c) merely learn from it? Is there any existing contact/mandate? | Changes integration requirements from "import/export compatible" to "certified integration" | A-003 |
| OQ-004 | **Open-source licence:** preference (e.g., AGPL-3.0 vs Apache-2.0/MIT) and stance on commercial hosting by third parties? | One-way door; affects contributions, commercial ecosystem, timing-vendor integrations | A-004 |
| OQ-005 | **Governance & funding:** solo-founder project, foundation/association (CH Verein), or company-backed? Any budget for infrastructure/hosting? | Affects sustainability requirements and hosting-model requirements | A-005 |
| OQ-006 | **Hosting model:** self-hosted per organizer, central SaaS instance, or both? Who operates the reference instance? | Drives architecture-relevant NFRs (multi-tenancy, offline sync) — must be answered before Phase B ADRs | A-006 |
| OQ-007 | **Para athletics:** in scope for MVP, later, or out of scope? | Classification/scoring adds real complexity; affects data model early | A-007 |
| OQ-008 | **Languages:** confirm DE/FR/IT/EN all required at launch, or DE/FR first? | i18n effort and translation workflow | A-008 |
| OQ-009 | **Name/brand** for the project? | Repo, docs, domain registration | A-009 |
| OQ-010 | **Existing data/relationships:** does the founder have access to real meet data, a pilot club/meet, or timing hardware for testing? | Validation strategy for Phase B acceptance tests | A-010 |
| OQ-011 | **Alabus entry-data format:** can the founder (or a Swiss Athletics / club contact) obtain a sample Alabus entries export and its format spec? The Alabus marketing site blocks automated access; the TAF3↔Alabus flow is documented only from the Seltec side ([research](../research/competitive-analysis.md#c2-swiss-athletics-as-a-system-of-systems)) | "Interoperate with Swiss Athletics" concretely means importing what Alabus exports; without a sample, that requirement stays format-TBD | A-011 |
| OQ-012 | **Data-standard alignment:** should the project align its exchange schema with the W3C Open Athletics Community Group draft Competition Data Model (and possibly contribute upstream)? No federation mandates it today | Shapes the export/API surface; reversible but strategically visible | A-012 |

## 2. Working assumptions (A-###)

> **Status 2026-07-05:** founder answers received (§4). Confirmed: A-002, A-003, A-005,
> A-010, A-011, A-012. Superseded/updated by decisions: A-001 → DEC-001 (road/XC now **out of
> scope**, indoor stadium stays in), A-004 → DEC-004 (AGPL-3.0 lead candidate), A-006 →
> DEC-006 (homelab-first; desktop-app option to be evaluated in Phase B0), A-007 → DEC-007
> (para **out of MVP**, not merely reduced), A-008 → DEC-008 (DE/FR complete incl. operator
> surfaces; IT/EN later), A-009 → DEC-009 (shortlist in §6).

| ID | Assumption | Basis |
|----|------------|-------|
| A-001 | MVP scope is **outdoor & indoor stadium track & field** (incl. combined events and relays); road/XC/trail are **later**, not out of scope. | Seed says "athletics tournaments"; named parallels (Seltec TAF, Swiss Athletics meets) are stadium-centric |
| A-002 | MVP targets **club → regional/cantonal meets** (≤ ~1000 athletes, 1–3 days), engineered so championship-grade operation is a growth path, not a rewrite. | Realistic OSS adoption path; incumbents are entrenched at national level |
| A-003 | Interoperability stance: **(a) coexist/interoperate** — import licence/entry data, export federation-conformant results; no assumption of official mandate. | No stated mandate in seed |
| A-004 | Licence TBD-owner: founder. Requirements are written licence-neutral but assume **OSI-approved copyleft or permissive** licence, compatible with community contribution. | Seed: "open source" |
| A-005 | Community project with **lightweight governance documented in-repo** (maintainers file, contribution guide); no paid staff assumed. | Seed silent on funding |
| A-006 | **Both** hosting modes are eventually needed; MVP assumes **single-organizer deployment** (one meet at a time) with offline-capable venue operation; multi-tenant SaaS is later. | Venue reality + OSS adoption path; flagged as Phase B ADR gate item |
| A-007 | Para athletics: **data-model-ready in MVP** (sport class on athlete/result), full workflows (RAZA scoring, classification management) later. | Integrated Swiss meets exist; full support is costly |
| A-008 | All four languages **DE/FR/IT/EN** are required for public-facing surfaces at launch; operator/admin surfaces may launch DE/EN first. | CLAUDE.md locale mandate; Swiss federation practice |
| A-009 | Working name **"OpenMeet"** used in docs only; no branding decisions implied. | Placeholder |
| A-010 | No privileged data access assumed; verification uses public formats, synthetic data, and published vendor file-format specs. | Conservative default |
| A-011 | Until an Alabus sample is available, Swiss entry import is specified against a **generic documented CSV mapping** plus the publicly documented TAF3 import behaviour (categories/disciplines only; rounds & schedule created in-system). | [Seltec wiki on the Swiss entries flow](../research/competitive-analysis.md#c2-swiss-athletics-as-a-system-of-systems) |
| A-012 | The system's exchange schema is **self-defined, versioned, and openly documented**, with the W3C Open Athletics draft used as a design reference (not a compliance target). | It is a community-group draft, not adopted by any federation studied |

## 3. TBD register

Items marked TBD elsewhere in the baseline are consolidated here at the A6 pass.

| ID | Item | Owner | Blocking? |
|----|------|-------|-----------|
| TBD-001 | Historical IAAF/World Athletics XML result format — no primary spec located; needed only if federation submission requires it | Research (Phase B, on demand) | No |
| TBD-002 | Seltec user-reported limitations beyond vendor docs (forum review out of scope in first pass) | Research (optional) | No |
| TBD-003 | Para-athletics support level in incumbent systems — unverified either way; affects differentiation claim strength, not requirements | Research (optional) | No |
| TBD-004 | Exact Alabus feature set / export format (see OQ-011) | Founder / federation contact | Blocks final Swiss-entry-import spec detail, not the baseline |
| TBD-005 | SVM (Schweizer Vereinsmeisterschaft) point table & nationality quotas — only secondarily sourced; primary SVM Reglement PDF fetch failed | Research (before any SYS with exact point values) | Blocks exact team-scoring values only; scheme is specified as configurable |
| TBD-006 | "RAZA" para scoring — could not be verified to exist under that name; no requirement is built on it | Research (only if para scoring becomes MVP) | No |
| TBD-007 | World Athletics results-submission format/API — appears federation-mediated, no public API found | Founder / Swiss Athletics contact | No — result export is specified as file-based |
| TBD-008 | Track tie-break rule text (CR&TR Rule 26) and field-event flight-size thresholds — not extracted verbatim yet | Research (Phase B, before implementing ranking logic) | No |
| TBD-009 | Swiss licence fee amounts (CHF 60/120 secondary only) and Gebührenreglement details | Research (only if fee handling in scope) | No |

## 4. Founder answers (received 2026-07-05) and resulting decisions

Founder answers verbatim (lightly formatted), each converted into a decision **DEC-###** that
updates the corresponding assumption. Requirement deltas were applied to the baseline on
2026-07-05 (see §5).

| OQ | Founder answer (verbatim) | Decision |
|----|---------------------------|----------|
| OQ-001 | "track & field stadium meets only" | **DEC-001:** Scope = stadium track & field only. Road/XC/trail move from *Later* to **out of scope** until revisited. |
| OQ-002 | "club/regional meets, the goal is to help small clubs with no IT budget. big events certainly have more budget and more tools" | **DEC-002:** MVP optimizes for **small volunteer-run clubs with zero IT budget**; championship-grade is a growth path, not a target. Youth-series meets (UBS Kids Cup style) are a canonical MVP scenario. |
| OQ-003 | "the goal is to learn what kind of constraints (input / output) systems from swiss athletics and Seltec TAF3 would put on our system. Currently, some timings are done via electronic systems (track running / sprinting), but other are taken by humans on the field (ball throw, long jump, etc...) and entered manually into a system (unknown for now). the list of participants with their number is managed in Seltec (I saw the printed sheets)" | **DEC-003:** Interop stance = **learn & coexist**: treat Seltec/Swiss Athletics as fixed environment constraints (import/export at their boundaries), not integration partners. Verified pipeline in [research C7](../research/competitive-analysis.md#c7-field-observations--live-results-pipeline-ubs-kids-cup-le-mouret-4-july-2026). |
| OQ-004 | "I want a license that allows adoption and improvement by clubs, but not for a private company to easily make money off of this project" | **DEC-004:** Licence direction = **strong copyleft; AGPL-3.0 is the leading candidate** (closes the SaaS loophole). Note: no OSI licence can prohibit commercial use outright; AGPL makes proprietary capture unattractive. Final choice = ADR-001 at the Phase B gate. |
| OQ-005 | "solo for now, TBD if adopted by clubs with small club sponsoring it" | **DEC-005:** Solo-founder governance; keep in-repo governance lightweight; revisit if clubs adopt. |
| OQ-006 | "homelab hosted for now. later, if a club wants to use it, it may need to live in the cloud, but with modest budgets. maybe we should investigate if a desktop offline app is even possible and matches the constraints" | **DEC-006:** Deployment target #1 = **founder homelab / single self-hosted instance**; cloud later at modest cost. **Phase B0 must evaluate a desktop/offline-first app vs. self-hosted web app as an explicit ADR with trade-offs** (ADR-002 candidate). |
| OQ-007 | "out of scope for now, local meetings are not para-athletic" | **DEC-007:** Para athletics = **out of scope for MVP** (kept data-model-friendly: adding a class field later must not require redesign). Mixed **age-category** fields remain MVP (youth series need them). |
| OQ-008 | "DE/FR, but allow for easy internationalization" | **DEC-008:** Launch languages = **DE + FR complete** (public *and* operator surfaces); i18n architecture must make adding IT/EN a translation task, not an engineering task. |
| OQ-009 | "I let you brainstorm something and check against existing offerings. Core idea: athletics, switzerland, small clubs hosting events with volunteers" | **DEC-009 (RESOLVED 2026-07-05):** Name = **Bahnfrei** (founder pick from shortlist §6). Name/logo rights held personally by the founder per ADR-001 §5. |
| — *(gate record)* | Founder, 2026-07-05: "I ratify all six ADRs as proposed, keep the name Bahnfrei" | **DEC-014: Phase B gate 2 PASSED.** ADR-001 (AGPL-3.0-only + DCO), ADR-002 v2 (hub-first), ADR-003 (Go/SQLite/HTMX), ADR-004 v2 (storage + capture queue), ADR-005 (domain model + omx/v1 + external IDs), ADR-006 (FinishLynx-first + timing agent) — all **Accepted**. Phase B implementation is unblocked; backlog in `docs/delivery/work-breakdown.md`. |
| OQ-010 | "no access for now, could be asked, but probably not or with a lot of convincing (GDPR, anonymization, ...)" | **DEC-010:** Verification uses **synthetic data only**; the public LA.portal result pages (already-published data) may be used as structural reference. |
| OQ-011 | "absolutely no idea. if I have a PoC to show to the local clubs, I may obtain something" | **DEC-011:** **PoC-first strategy**: build against the documented generic CSV mapping (A-011); an Alabus sample becomes an ask once a PoC exists to show clubs. |
| OQ-012 | "is this gonna become mainstream? also in switzerland? I guess that if we must choose something from scratch, we can align with W3C Open Athletics Community Group" | **DEC-012:** Honest answer: **no evidence it will become mainstream** — it is a community-group draft no federation has adopted (C3). Use it as the design reference where we define schemas from scratch (per A-012); do not promise conformance. |
| OQ-016 *(founder direction, 2026-07-05, ADR-002 ratification discussion)* | "we will be expecting some kind of online connectivity to a central hub. the system should be however tolerant to LTE and WIFI issues and not stop working at the first network issue in the meet timing room or out in the field […] should not cost a fortune to deploy, as the target is small clubs […] my goal is not to have a SaaS page with subscriptions […] I build the open source foundation and they use the stack and host the stack (made as easy as possible by us) on their own" | **DEC-013: hub-first connectivity posture.** Expected = connectivity to a central **self-hosted** hub; MVP = interruption tolerance (SYS-085–087, UC-034); full no-internet venue operation deferred behind an evidence gate (SYS-080 → *Later*, UC-019); **field-wide venue Wi-Fi = non-goal** (research C8: club-tier incumbents all use officials' phones + LTE via cloud relay); deployment cost sized for small clubs; **no subscription-SaaS business** — clubs self-host. Re-scopes ADR-002 (v2) and ADR-004 (v2); adds Swiss-chain interop SYS-077/078 (C2.1). |

## 5. Requirement deltas applied from the answers (2026-07-05)

| Change | Where |
|--------|-------|
| Para: STR-030 re-scoped to mixed **age-category** fields (MVP); para sport classes → out of MVP scope (Later) | StRS §3.5/§4, SYS-052, UC-028 |
| Languages: DE/FR complete at launch (public + operator); IT/EN → Later, must be translation-only effort | STR-033, SYS-110, UC-025 |
| Road/XC/trail from Later → out of scope | StRS §4 |
| NEW STR-042: public results labeled unofficial; federation channel remains the official source of truth for sanctioned meets | StRS §3.4, SYS-076, UC-017 |
| NEW STR-043: Swiss youth-series formats (UBS Kids Cup first) with series-specific scoring tables | StRS §3.3, SYS-053, UC-033 |
| Licence direction (AGPL-lead) recorded as constraint note | SyRS CON-02 |
| Desktop-app-vs-web evaluation mandated for Phase B0 | DEC-006 → ADR-002 candidate |

## 6. Project name shortlist (DEC-009, for founder pick)

Criteria: works across DE/FR/IT/EN, evokes athletics + grassroots volunteering, no collision
found with existing athletics software (checked 2026-07-05 via web search; **domain/trademark
checks still TBD before adoption**).

| Candidate | Reading | Notes |
|-----------|---------|-------|
| **Bahnfrei** | DE starter's call "Bahn frei!" (track clear!) | Distinctive, sympathetic, DE-leaning; understandable in CH; no software collision found |
| **PisteLibre** | FR mirror of the same idea | FR-leaning twin; could pair as bilingual brand "Bahnfrei / PisteLibre" |
| **OpenLane** | EN, "the open lane" | Neutral EN; collision risk with US automotive marketplace "OPENLANE" in other domain — likely acceptable, verify trademark class |
| **Startklar** | DE/CH "ready to start" | Warm, volunteer-flavored; common word → weak trademark, domains likely taken |
| **Anlauf** | DE "run-up" (jump approach) + "getting started" | Nice double meaning; DE-only comprehension |

Avoided: anything with "OpenTrack" (existing commercial vendor + W3C group naming collision,
C3/C4), "Athletica" (club-name collisions), "Meet*" generics (HyTek "Meet Manager", "MeetPro").

## 7. New/updated open items from this round

| ID | Item | Owner | Blocking? |
|----|------|-------|-----------|
| OQ-013 | **Club validation of the results pipeline:** confirm with the UBS Kids Cup host club: (a) results were entered in TAF3 on-site (in the results room), (b) live push used Liveserver/LA.portal, (c) whether a club may additionally publish results elsewhere (any federation policy against it), (d) how manual field measures reached TAF3 (paper slips → typed in?) | Founder (has club contact from the meet) | Blocks STR-042 fine-tuning and the official-channel strategy, not the baseline |
| TBD-003 *(updated)* | Para support in incumbents: **partially resolved** — Seltec offers a paid "Para" licence module with para-points support ([research C7.2](../research/competitive-analysis.md)) | closed enough for now | No |
| OQ-014 *(updated 2026-07-05)* | **Official-tier entry (C2.1):** the lock is double — (1) **normative:** WO 2026 §5.6a mandates TAF3 by name for Meisterschafts- und offizielle Wettkämpfe (see OQ-015 resolution), so entering this tier requires Swiss Athletics to amend/waive an association rule it can change annually; (2) **technical:** results must reach Alabus same-day and the only upload tooling is inside TAF3. The eventual ask to Swiss Athletics (once a PoC has club traction, per DEC-011) is therefore *recognition as an additional permitted system + a documented upload path* — precedents: DLV Bestenliste auto-ingests from **two** systems (TAF3 + COSA WIN), Atletiekunie recognises two systems. | Founder → Swiss Athletics Geschäftsstelle | Blocks the **official/championship** tier only; the youth-series/school/club tier has an official Excel path (C2.1) and is not blocked |
| OQ-015 *(RESOLVED 2026-07-05)* | **Does the WO mandate specific software? Yes.** WO 2026 §5.6a: championship and official competitions **must** be administered with TAF3 (provided free by Swiss Athletics); §5.6b mandates activating its LA.portal live publication; §5.3b makes licence-number linking *in TAF3* the only route into the Bestenliste. Note: a federation rule (revisable annually), not state law; World Athletics certifies equipment only, not meet software (C2.1). | closed | No — but it upgrades OQ-014 from "file format ask" to "recognition ask" |
| OQ-017 *(TASK-004, 2026-07-06)* | **Swiss WO §1.2 start-up ceilings:** D4.2's "eligible to start up to" column is self-flagged as ambiguous; the shipped rule data (`internal/domain/data/category-schemes/swiss-athletics.json`) approximates start-up/start-down as per-category booleans, and masters early start-up defaults to `false` (unevidenced). Verify exact age ceilings against WO §1.2 and refine the scheme data (version bump). | Founder / next Swiss Athletics contact | No — scheme data is versioned; refinement is a data-file change |
| OQ-018 *(TASK-004, 2026-07-06)* | **D4.3 youth discipline bars:** the parenthetical barring 150/200/300/400 m flat sprints for U16-and-younger contradicts standard youth-meet practice and this repo's own examples — likely a transcription artifact. Rule data ships only the unambiguous bars (300 mH/400 mH, steeplechase). Verify WO §1.5 directly and correct the data file either way. | Founder / next Swiss Athletics contact | No — same versioned-data path as OQ-017 |
| OQ-019 *(TASK-004, 2026-07-06)* | **Youth hurdle-height/implement variants:** U16/U18/U20 hurdle heights are NOT populated in `internal/domain/data/disciplines/catalog.json` — the only secondary source found uses UK age groups (U13/U15/U17/U20) that don't map onto WA/Swiss categories, and the WA CR&TR PDF yielded no extractable text. Obtain primary CR&TR / Swiss WO tables and complete the variants before any meet uses youth hurdles (TASK-008+ capture flows). | Tech lead (data follow-up task) | Blocks youth-hurdles events only; not the UKC PoC (60 m flat, ZoneLJ, ball throw) |
| OQ-020 *(TASK-007, 2026-07-08)* | **UKC ranking with a missing discipline:** the Reglement (Stand Dezember 2025, §3 Rangierung) defines tie-breaking over "allen drei absolvierten Disziplinen" but is silent on athletes who complete fewer than three. Shipped behaviour (assumption, fixture-tested): a missing/non-scoring discipline contributes 0 points, the athlete ranks by the remaining total, and standings/exports show the gap explicitly (UC-033 #3). Confirm against official UKC result lists or the series office; also note the Reglement cites "Wertungstabelle 2010" while the published PDF is titled "Wertungstabelle 2025 — Version UBS Kids Cup" (the PDF is authoritative; the table ships as versioned data either way). | Founder / UBS Kids Cup series office | No — versioned scoring/ranking data+logic; a correction is a small, traced change |
