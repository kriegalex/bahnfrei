# Glossary

Domain and project terms as used in the requirements baseline. German (DE) / French (FR)
equivalents given where they appear in Swiss federation practice. Sources: `D#` =
`../research/domain-athletics.md`, `C#` = `../research/competitive-analysis.md`.

## Competition structure

| Term | Definition |
|------|------------|
| **Meet / Meeting** (DE: *Wettkampf/Meeting*, FR: *meeting*) | An athletics competition occasion: one venue, one or more days, an event programme. Swiss sanctioned tiers: A/B/C-Meeting (D1.2). |
| **Event** | One discipline contested by one category (or combined categories) at a meet, e.g. `100m / U16 W`. |
| **Discipline** | The athletic exercise itself (100m, high jump, shot put, decathlon…), with per-category technical variants (hurdle heights, implement masses). |
| **Round** | Stage of an event: qualification round(s) → semi-final(s) → final (D2.1). |
| **Unit** | One concurrently-run portion of a round: a *heat* (track), *flight/group* (field), *group* (combined events). |
| **Heat** (DE: *Serie/Lauf*) | One race within a track round. |
| **Flight** (DE: *Gruppe*) | Subdivision of a field event's competitors. |
| **Session** | A block of the timetable (e.g., Saturday morning). |
| **Timetable** (DE: *Zeitplan*, FR: *horaire*) | Published schedule of units; amendments are versioned (SYS-004). |
| **Combined events** (DE: *Mehrkampf*) | Multi-discipline competitions (decathlon, heptathlon…) scored via the World Athletics scoring tables (D5.4). |
| **SVM** (*Schweizer Vereinsmeisterschaft*) | Swiss inter-club team championship with placing-points team scoring; governed by its own Reglement (D5.5). Not "CSI". |

## Entries, categories, eligibility

| Term | Definition |
|------|------------|
| **Entry** (DE: *Anmeldung/Meldung*) | Registration of an athlete/relay team for an event, with optional seed performance. |
| **Seed performance / seed mark** | The performance used to rank entries for seeding (default season best, D2.2). |
| **Category** (DE: *Kategorie*) | Age/sex competition class. Swiss scheme: U10…U23, Men/Women ("Aktive"), Masters in 5-year bands (D4.2). WA formally defines only U18/U20/Masters (D4.1). |
| **Category scheme** | A federation's complete category definition set; configurable in the system (SYS-005). |
| **Starting up/down** | Competing in an older (youth) or younger (masters) category than one's own, where the scheme permits (D4.2). |
| **Licence** (DE: *Lizenz*) | Swiss Athletics' personal, calendar-year competition authorization; required for A/B-Meetings and championships (D3.1). |
| **Youth-protection rules** | Swiss WO limits for U10–U16: maximum race distances, barred disciplines, max one race ≥600m/day (D4.3). |
| **Bib** (DE: *Startnummer*, FR: *dossard*) | The athlete's competition number; unique per meet (SYS-018). |

## Competition day

| Term | Definition |
|------|------------|
| **Call room** (DE: *Callroom*) | Check-in area where athletes are verified before their event; no-shows become DNS (D11.1). |
| **Check-in** | Confirmation that an entered athlete will actually start (UC-007). |
| **Seeding** | Rule-governed distribution of athletes into heats/lanes/orders (TR20; D2.2–D2.3). |
| **Lane groups** | TR20.4 draw: ranks 1–4 → lanes 4–7; 5–6 → 3, 8; 7–8 → 1, 2 (D2.3). |
| **Progression** | Advancement to the next round: `Q` by place, `q` by time/performance, `qR/qJ/qD` by referee/jury/draw (D2.4). |
| **Fastest losers** | Athletes advancing on time despite not placing (`q`). |
| **Countback** | Vertical-jump tie-breaking: fewest attempts at last height, then fewest total failures (D5.6). |
| **TIC** (Technical Information Centre) | Communications hub for technical matters; records official result-announcement times (D11.2). |
| **Protest / Appeal** (DE: *Einspruch / Berufung*) | Oral protest to the Referee within 30 min of result announcement; written appeal to the Jury of Appeal within 30 min of the amended announcement, USD 100 deposit (D8.3). |
| **Jury of Appeal** | Final-instance body for appeals; decision final (D8.3). |

## Results, timing, records

| Term | Definition |
|------|------------|
| **FAT** (Fully Automatic Timing) | Photo-finish timing to 0.01 s; required for records in races ≤800m (D5.1, D6.2). |
| **Hand timing** | Manual stopwatch timing; rounds **up** to 0.1 s and is marked as such (D5.1). |
| **Homologation A/C/D** | Swiss timing-equipment tiers: A = line-scan photo-finish (records-eligible), C = photocell (limited use), D = transponder (D5.1). |
| **Wind-assisted** | Average tailwind > +2.0 m/s: mark ranks but is record/PB-ineligible (D5.3). |
| **Status codes** | CR 25 result vocabulary: DNS, DNF, NM, NH, DQ (+rule), O/X/–, r, Q, q, qR, qJ, qD, YC, YRC, RC, L, P (D5.2). |
| **Records** | WR/AR/NR (world/area/national), meeting record (MR), PB/SB (personal/season best); WA "Best" scheme in D6.1. |
| **Rekordprotokoll** | Swiss record-claim documentation (timing class, zero-test, wind, finish image…) (D6.2). |
| **Zero test** (DE: *Nullschuss*) | Photo-finish start-synchronization verification required for record claims. |
| **World Athletics scoring tables** | Official point tables (Spiriev) used for combined events and World Rankings result scores (D5.4). |
| **Announcement timestamp** | Recorded time a result list version was posted; starts protest/appeal clocks (SYS-047). |
| **Provisional / official result** | Result within / past its protest window (UC-015). |

## Para athletics

| Term | Definition |
|------|------------|
| **Sport class** | World Para Athletics classification: `T` (track/jumps) or `F` (field) + number encoding impairment group (T11–13 vision, T35–38 coordination, T61–64 prosthesis…) (D9.1). |
| **Mixed field** | A unit combining categories and/or sport classes, re-ranked per group in results (CR 25.2–25.3; SYS-052). |

## Ecosystem & interfaces

| Term | Definition |
|------|------------|
| **Seltec TAF3** | *Track and Field 3* — Windows meet-management software by Seltec (correct spelling; seed wrote "setlec"), free to Swiss/German organizers via federation contracts (C1). |
| **LA.portal / LA.net** | Seltec's results-publication and federation/entries platforms (C1). |
| **Alabus** | Third-party SaaS Swiss Athletics uses for licences and competition entries; source of entry imports (C2, OQ-011). |
| **FinishLynx file family** | De-facto timing interchange: `lynx.ppl` (people), `lynx.sch` (schedule), `lynx.evt` (events/lanes) in; `.lif` (results) out (D7.2, C5). |
| **EDM** | Electronic distance measurement for field events (D7.1). |
| **W3C Open Athletics CG** | Community group (ex-"OpenTrack CG", unrelated to the vendor OpenTrack) drafting an open competition data model; design reference, not a mandate (C3, A-012). |
| **Global Calendar** | World Athletics' registry a meet must join for World Ranking status; separate from national sanctioning (C2, D10.1). |
| **World Rankings** | WA points-based athlete ranking (result score + placing score) fed by ratified results, ideally within 24h (D6.3, D10.2). |

## UI & interaction

| Term | Definition |
|------|------------|
| **Contextual help** | Short explanation available at an input's point of use via a "?" help icon (SYS-115). Opens on hover, keyboard focus, and tap/click — never hover-only — and meets WCAG 2.2 SC 1.4.13 (dismissible, hoverable, persistent). Sometimes called a *toggletip* when click/tap-toggled. |
| **Hint text** | Short, permanently visible helper text under an input's label carrying information needed to complete the field (format, units, bounds). Required information lives here, never only inside contextual help (SYS-115/117; GOV.UK Design System pattern). |
| **Help-content registry** | Machine-readable list of which inputs carry contextual help and their localized texts; drives the UC-037 coverage test (SYS-115). |
| **Design token** | Named, single-source visual value (color, type scale, spacing, radius, elevation) from which all surfaces are styled (SYS-116); screens introduce no visual literals outside the tokens. |

## Legal & project

| Term | Definition |
|------|------------|
| **nFADP** (DE: *revDSG*) | Swiss Federal Act on Data Protection (revised, 2023) — applies alongside GDPR (SYS-100…105). |
| **GDPR** | EU General Data Protection Regulation. |
| **WCAG 2.2 AA** | Web accessibility conformance target for public surfaces (SYS-112). |
| **StRS / SyRS** | Stakeholder / System Requirements Specification (ISO/IEC/IEEE 29148). |
| **STR-### / SYS-### / UC-### / ADR-### / TASK-###** | Stable IDs: stakeholder requirement, system requirement, use-case, architecture decision record, work item. Never renumbered — deprecated only. |
| **MVP / Later** | First-release scope vs. in-scope post-MVP (StRS §4). |
| **One-way door** | Hard-to-reverse decision (licence, stack, data model) requiring founder ratification via ADR before build. |
