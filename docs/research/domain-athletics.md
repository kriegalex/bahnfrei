# Domain Research — Athletics (Track & Field) Competition Management

Status: Phase A research input. Feeds StRS/SyRS drafting. Implementation-free.
Locale context: Switzerland / EU (nFADP + GDPR, DE/FR/IT/EN).

## Summary table — key domain concepts

| Concept | Key facts | Section |
|---|---|---|
| Governing bodies | World Athletics (global), European Athletics (continental), Swiss Athletics (national federation) | D1 |
| Event families | Track, field (jumps/throws), combined events, relays, race walking, road running, cross country, mountain/trail | D1 |
| Round structure | Qualification round(s) → semi-final(s) → final; advancement by place ("Q") and/or time ("q") | D2 |
| Age categories (WA) | Formally only U18, U20, Master (35+) defined in WA Technical Rules; U23 used only at continental/national level, not a WA Rule 3 category | D4 |
| Age categories (Swiss) | U10/U12/U14/U16/U18/U20/U23/Men(M)-Women(W)/Masters M30–M80+ in 5-year bands | D4 |
| Timing methods | Hand Timing, Fully Automatic Timing (FAT) + Photo Finish, Transponder (non-stadium/road only) | D7 |
| Wind limit | +2.0 m/s tailwind (average, direction of running) invalidates record eligibility for sprints/HJ&LJ up to 200m | D5 |
| Status codes | DNS, DNF, NM, DQ, NH, O/X/–, r, Q, q, qR, qJ, qD, YC, RC, L, P (full table D5) | D5 |
| Record types | WR, AR, NR, meeting record, PB, SB, QB, championship best (CHB) — WA abbreviation scheme | D6 |
| Ranking system | Result Score + Placing Score = Performance Score; averaged over ranking period → Ranking Score | D6 |
| Officials | Technical Delegate, Referee, Starter, Judges, Photo Finish Judge, Jury of Appeal, Call Room, TIC Manager, Marshal | D8 |
| Protest deadline | 30 minutes from official result announcement (protest); 30 minutes from amended-result announcement (appeal to Jury) | D8 |
| Para classification | 10 eligible impairment types; T (track/jump)/F (field/throws) classes; numeric ranges by impairment group | D9 |
| Meet sanctioning (CH) | A/B/C-Meetings tiers tied to World Ranking category; ≥30-day advance registration; venue homologation required | D10 |
| Venue reality | Call Room, Technical Information Centre (TIC), often unreliable on-site connectivity → offline-capable tooling matters | D11 |

---

## D1. Competition structure

### D1.1 Event families
World Athletics' Competition and Technical Rules (CR&TR) organise disciplines into these Technical Rules parts: Part II Track Events, Part III Field Events (Vertical Jumps, Horizontal Jumps, Throwing Events), Part IV Combined Events, Part V 200m Standard Oval Track (Short Track), Part VI Race Walking, Part VII Road Races, Part VIII Cross Country/Mountain/Trail races ([World Athletics Competition and Technical Rules, 2024 Edition, Table of Contents](https://worldathletics.org/download/download?filename=d88c772f-56b5-4945-a3a9-0a9d53a8b29d.pdf&urlslug=Competition%20and%20Technical%20Rules%20%E2%80%93%202024%20Edition)).

Swiss Athletics' national competition regulations (Wettkampfordnung, "WO") mirror this with separate discipline lists for outdoor stadium (§8), indoor/hall (§9), and off-stadium championships — cross country, 10km, half marathon, marathon, 100km, mountain running (Berglauf), trail running (§10) ([Swiss Athletics WO 2026, §§8–10](https://www.swiss-athletics.ch/fileadmin/user_upload/www.swiss-athletics.ch/wettkaempfe/Wettkampfsupport/Reglemente_und_Unterlagen/Nationale_Reglemente/WO_2026_d.pdf)).

### D1.2 Meet tiers (Swiss context)
Swiss Athletics classifies officially sanctioned meets into three tiers, each with distinct requirements (WO §4.2):

| Tier | World Ranking category | Timing requirement | Entry rules |
|---|---|---|---|
| A-Meeting | OW/DF/GW/GL/A/B/C/D (Diamond League, Continental Tour, championships) | Homologation A (FAT, ≥1000 lines/sec) mandatory; recommended start information system | Coordination with Swiss Athletics for CH athlete participation; licence required |
| B-Meeting | World Ranking E/F | Homologation A mandatory | Entry/quota limits possible; all Swiss club athletes must hold a licence; must be registered ≥30 days ahead in both the Swiss Athletics tool and the WA Global Calendar |
| C-Meeting | Not a World Ranking meeting | Homologation A required for CH records; Homologation C (photocell) permitted for U18-and-younger / races >400m | Free choice of date/categories/disciplines; unlicensed athletes may compete but their results are excluded from ranking lists |

Source: [Swiss Athletics WO 2026, §4.2.1–4.2.3](https://www.swiss-athletics.ch/fileadmin/user_upload/www.swiss-athletics.ch/wettkaempfe/Wettkampfsupport/Reglemente_und_Unterlagen/Nationale_Reglemente/WO_2026_d.pdf).

### D1.3 Sessions and timetables
Multi-day championships run a published timetable of sessions per day (heats in morning session, semis/finals in evening session is typical), coordinated by the Competition Director and communicated via the Call Room schedule and Technical Information Centre (TIC) (see D11). The CR&TR does not mandate a specific timetable structure — this is left to the Organisers/Technical Delegate(s) per competition regulations ([CR&TR 2024, Rule 25 area / TIC description](https://worldathletics.org/download/download?filename=d88c772f-56b5-4945-a3a9-0a9d53a8b29d.pdf&urlslug=Competition%20and%20Technical%20Rules%20%E2%80%93%202024%20Edition)).

---

## D2. Rounds, heats, seeding, qualification

### D2.1 When qualification rounds are used
"Qualification Rounds shall be held in Track Events in which the number of athletes is too large to allow the competition to be conducted satisfactorily in a single round (final)." Where used, athletes must compete in and qualify through **all** rounds, unless the governing body authorises additional preliminary qualification round(s) at the same or an earlier competition (Technical Rule 20) ([WA Seedings, Draws and Qualification in Track Events, quoting TR20](https://www.northernathletics.co.uk/wp-content/uploads/2025/04/WA-Seeding-Draws-and-Qualifications.pdf)).

### D2.2 Seeding principles
- Seeding is normally based on athletes' best valid performances (including wind-legal readings) during a pre-determined period — defaulting to **Season Best** if unspecified.
- Athletes from the same Member/team, and the best-performed athletes, should be placed in different heats "whenever possible."
- After the first round, exchanges between heats should be made between athletes seeded in the same "group of lanes" (TR20.4.3–20.4.5).
- Training/test performances must never be used for seeding.

([WA Seedings, Draws and Qualification in Track Events](https://www.northernathletics.co.uk/wp-content/uploads/2025/04/WA-Seeding-Draws-and-Qualifications.pdf))

### D2.3 Lane assignment (draws)
For 400m/800m races started in lanes, three separate draws are conducted:
1. Four highest-ranked athletes/teams → lanes 4, 5, 6, 7
2. 5th/6th ranked → lanes 3, 8
3. Two lowest-ranked → lanes 1, 2

For events longer than 800m, relays longer than 4×400m, or any single-round (final-only) event, lanes/starting positions are drawn by lot. Where a stadium has more lanes than athletes, inside lane(s) remain unused and lane numbering shifts accordingly (e.g. on a 9-lane track with 8 athletes, lane 2 is treated as "lane 1" for Rule 20.4 purposes) ([WA Seedings, Draws and Qualification in Track Events, TR20.4](https://www.northernathletics.co.uk/wp-content/uploads/2025/04/WA-Seeding-Draws-and-Qualifications.pdf)).

> **Verified 2026-07-15 (rule-data pass, OQ-034), primary WA CR&TR 2026 TR 20.4:** the "single-round (final-only) → by lot" clause (TR 20.4.6) applies only to competitions under World Rankings definition 1.(a)-(c)/2.(a)-(b); lower-level meets may use different principles per the TR20 interpretation notes. The rank-group → lane-set tables differ by event class (TR 20.4.3 straight races: top4 → lanes 3–6; TR 20.4.4 200m/300m: top3 → 5–7; TR 20.4.5 400m-class: top4 → 4–7), each with 8- and 9-lane variants — encoded in `internal/domain/data/seeding/tr20.json` v`wa-tr20-2026.2`.

### D2.4 Advancement ("progression")
Qualification tables should, where practicable, allow at least the top 2 (ideally top 3) per heat to advance by **place**; remaining slots are filled by **time** ("fastest losers") according to the applicable Technical Regulations or Technical Delegate decision. When athletes qualify by time, only one timing system may be applied for that determination ([WA Seedings, Draws and Qualification, Progression section](https://www.northernathletics.co.uk/wp-content/uploads/2025/04/WA-Seeding-Draws-and-Qualifications.pdf)).

Standard result/qualification abbreviations (Competition Rule 25) — see full table in D5.2: `Q` = qualified by place (track) or standard (field), `q` = qualified by time (track) or performance (field), `qR`/`qJ`/`qD` = advanced by Referee / Jury of Appeal / draw decision.

### D2.5 Field-event flights and combined events groups
Field events with large entry counts are commonly split into qualification groups ("flights") that each attempt a fixed number of trials before the field is cut to a final group for the remaining trials (per Technical Rules Part III, referenced in CR&TR TOC — pp. 152–227). Combined events (decathlon/heptathlon/pentathlon) are organised as a fixed sequence of individual events across 1–2 days, scored per event via the WA Scoring Tables (D6.3); large fields may be split into competition groups run in parallel or in series. **TBD**: exact group-size thresholds are set per competition regulations, not fixed in CR&TR — could not verify a universal numeric threshold.

### D2.6 Qualifying standards
Qualifying/entry standards (times or marks required for entry to a championship) are set per competition by the organising body, not by a universal CR&TR table; World Athletics publishes entry standards separately for its own World Athletics Series events. Swiss Athletics publishes annual "Limiten" (entry standard) lists per category for Swiss Championships (e.g., "Limiten SM Aktive 2026", "Limiten SM U16-U18 2026") on its results/records pages ([Swiss Athletics — Bestenlisten/Limiten example](https://lt-athletics.ch/rekorde-bestenlisten/)) — the exact CH standards are annual and TBD for this document (they are operational data, not structural requirements).

---

## D3. Entries & registration

### D3.1 Swiss Athletics licence system
A Swiss Athletics licence (Lizenz) is required to start at:
- Stadium/indoor championships (WO §4.1.1/4.1.2)
- Official meetings (A/B/C-Meetings, WO §4.2)
- International championships to which Swiss Athletics sends athletes

Conditions: club membership (with exceptions below); the club must itself be a Swiss Athletics "Verein"-category member; all prior-year dues settled. The licence is personal, valid for one calendar year (1 Jan–31 Dec); only **one** Swiss Athletics licence may be held per athlete (an additional foreign licence, to start for a foreign club, is permitted) ([Swiss Athletics WO 2026, §1.3–1.4](https://www.swiss-athletics.ch/fileadmin/user_upload/www.swiss-athletics.ch/wettkaempfe/Wettkampfsupport/Reglemente_und_Unterlagen/Nationale_Reglemente/WO_2026_d.pdf)).

Exceptions:
- **U10/U12/U14**: club membership is *not* required; children without a club get a "kids+athletics" licence (WO §1.4.1).
- **Nachwuchsserien (youth series)**: no licence needed up to and including cantonal/regional finals; a licence is required for the Swiss final (WO §1.4.2).
- **Off-stadium Swiss Championships** (cross, 10km, half marathon, marathon, 100km, mountain, trail): no licence required (WO §1.4.3).

### D3.2 Bib numbers
World Athletics defines "Bib Number / Athlete Bib" as "an athlete's number, name or other suitable identification during the competition" ([WA Terms & Abbreviations, July 2023, Glossary](https://worldathletics.org/download/download?filename=WA_Terms__Abbreviations_EN+-+July+2023v2.pdf&urlslug=Terms+and+Abbreviations)). Call Room Judges verify bibs are worn correctly and match the start list before releasing athletes to the competition area ([CR&TR 2024, Call Room Judges duties](https://worldathletics.org/download/download?filename=d88c772f-56b5-4945-a3a9-0a9d53a8b29d.pdf&urlslug=Competition%20and%20Technical%20Rules%20%E2%80%93%202024%20Edition)).

### D3.3 Age verification
World Athletics Technical Rule 3: "An athlete must be able to provide proof of their age through presentation of a valid passport or other form of evidence as permitted by the applicable regulations... An athlete who fails or refuses to provide such proof shall not be eligible to compete" ([CR&TR 2024, TR3 Age and Sex Categories](https://worldathletics.org/download/download?filename=d88c772f-56b5-4945-a3a9-0a9d53a8b29d.pdf&urlslug=Competition%20and%20Technical%20Rules%20%E2%80%93%202024%20Edition)).

### D3.4 Meet sanctioning / registration deadlines
All meet data (B/C-Meetings) must be entered in the Swiss Athletics competition-management tool at least **30 days** before the event date; late submissions incur an extra fee, and sanctioning ("Bewilligung") is issued no later than 7 working days before the meet. A meet can only be sanctioned if held on a homologated facility (homologation ≤10 years old generally; ≤5 years for a stadium hosting a Swiss Championship) ([Swiss Athletics WO 2026, §5.2](https://www.swiss-athletics.ch/fileadmin/user_upload/www.swiss-athletics.ch/wettkaempfe/Wettkampfsupport/Reglemente_und_Unterlagen/Nationale_Reglemente/WO_2026_d.pdf)).

### D3.5 Relay team composition
Relay teams (4×100m, 4×200m, 4×400m, Medley Relay, Swedish relay) are governed by Technical Rules Part II (relay events). Swiss records are kept separately for "absolute" relay records (all-Swiss-citizen teams) and "club relay" records (same-club/LG teams, minimum half Swiss citizens) — see D6.2. **TBD**: universal minimum/maximum roster size per relay beyond the 4 running legs — governed by each competition's entry regulations, not a fixed CR&TR number.

### D3.6 Entry fees
Entry fees ("Startgeld") are set by each competition per Swiss Athletics fee regulations (Gebührenreglement); not itself enumerated in the WO ([Swiss Athletics WO 2026, §5.4](https://www.swiss-athletics.ch/fileadmin/user_upload/www.swiss-athletics.ch/wettkaempfe/Wettkampfsupport/Reglemente_und_Unterlagen/Nationale_Reglemente/WO_2026_d.pdf)). Licence fees themselves were reported by secondary sources as CHF 60 or 120 depending on category (**TBD** — could not verify against a primary fee schedule in this pass).

---

## D4. Age & gender categories

### D4.1 World Athletics (Technical Rule 3 — formally defined)
> "Under-18 (U18) Men and Women: Any athlete of 16 or 17 years on 31st December in the year of the competition. Under-20 (U20) Men and Women: Any athlete of 18 or 19 years on 31st December in the year of the competition. Master Men and Women: Any athlete who has reached their 35th birthday."

([CR&TR 2024, TR3 Age and Sex Categories](https://worldathletics.org/download/download?filename=d88c772f-56b5-4945-a3a9-0a9d53a8b29d.pdf&urlslug=Competition%20and%20Technical%20Rules%20%E2%80%93%202024%20Edition))

Important nuance for requirements: **"U23" is not a formally defined World Athletics Rule 3 age category.** It appears only as a naming convention for specific continental/national-level competitions (e.g., "Area U23 Championships" in the World Rankings competition-category table) ([World Athletics World Rankings — Categories of Competitions, §1.3](https://worldathletics.org/world-ranking-rules/basics)). Each competition's own regulations determine which age groups apply, whether younger athletes may participate, etc. (CR&TR, note to TR3).

Master age groups (World Masters Athletics), in 5-year bands from 35 upward, through 100+:

| Age range | Male | Female |
|---|---|---|
| 35–39 | M35 | W35 |
| 40–44 | M40 | W40 |
| 45–49 | M45 | W45 |
| 50–54 | M50 | W50 |
| 55–59 | M55 | W55 |
| 60–64 | M60 | W60 |
| 65–69 | M65 | W65 |
| 70–74 | M70 | W70 |
| 75–79 | M75 | W75 |
| 80–84 | M80 | W80 |
| 85–89 | M85 | W85 |
| 90–94 | M90 | W90 |
| 95–99 | M95 | W95 |
| 100+ | M100 | W100 |

([World Masters Athletics FAQ — age categories](https://world-masters-athletics.org/sp_faq/2197/))

### D4.2 Swiss Athletics category system (verified — WO 2026, §1.1)
Swiss Athletics licence categories, by age, with the **oldest age still permitted to start** in that category (note: a "Startberechtigung" upper bound differs slightly from the strict age band, allowing e.g. an 11-year-old to start U12 even if nominally the "youngest" U12 age):

| Category | Age (nominal) | Eligible to start (up to) |
|---|---|---|
| U10 M / U10 W | 9 and younger | 9 and younger |
| U12 M / U12 W | 10–11 | 11 and younger |
| U14 M / U14 W | 12–13 | 13 and younger |
| U16 M / U16 W | 14–15 | 15 and younger |
| U18 M / U18 W | 16–17 | 17 and younger |
| U20 M / U20 W | 18–19 | 19 and younger |
| U23 M / U23 W | 20–22 | 22 and younger |
| Men (M) / Women (W) ("Aktive") | 23–29 | all ages (adult category) |
| Masters M30/W30 | 30–34 | 30 and older |
| Masters M35/W35 | 35–39 | 35 and older |
| Masters M40/W40 | 40–44 | 40 and older |
| Masters M45/W45 | 45–49 | 45 and older |
| Masters M50/W50 | 50–54 | 50 and older |
| Masters M55/W55 | 55–59 | 55 and older |
| Masters M60/W60 | 60–64 | 60 and older |
| Masters M65/W65 | 65–69 | 65 and older |
| Masters M70/W70 | 70–74 | 70 and older |
| Masters M75/W75 | 75–79 | 75 and older |
| Masters M80/W80 | 80 and older | 85 and older |

Category transition happens at the start of the calendar year in which the athlete reaches the category's lower age bound. Younger athletes may start "up" a category (subject to protection rules, WO §1.5); Masters athletes may start "down" into a younger Masters band; anyone may start in the open Men/Women category. Mixing categories within the same discipline at a Swiss or Regional Championship is not permitted except in defined exceptions (relay events, SVM club championship).

([Swiss Athletics WO 2026, §1.1–1.2](https://www.swiss-athletics.ch/fileadmin/user_upload/www.swiss-athletics.ch/wettkaempfe/Wettkampfsupport/Reglemente_und_Unterlagen/Nationale_Reglemente/WO_2026_d.pdf))

### D4.3 Protective rules for youth categories (WO §1.5)
Maximum race distances by category and venue type:

| Category | Stadium/Hall | Off-stadium (road) |
|---|---|---|
| U10 | 1000 m | 2 km |
| U12 | 2000 m | 3 km |
| U14 | 3000 m | 5 km |
| U16 | — | 10 km |
| U18 | — | Half marathon |

U10/U12/U14 athletes may run at most one race of 600m or longer per competition day; may not enter steeplechase; and U10/U12/U14/U16 athletes may not compete in a discipline reserved for an older category (e.g., 150/200/300/400m/300mH/400mH events are barred for U16-and-younger, except in specific relay/SVM contexts).

> **Verified 2026-07-15 (rule-data pass, OQ-018):** the parenthetical is the literal WO 2026 §1.5e rule — "…dürfen an von Swiss Athletics bewilligten Wettkämpfen in den Disziplinen 150 m, 200 m, 300 m, 400 m, 300 mH und 400 mH nicht starten" (exceptions: SM Staffel, SVM) — confirmed against the primary PDF; the steeple bar is §1.5c. Encoded in `internal/domain/data/category-schemes/swiss-athletics.json` v`wo2026.2`.

---

## D5. Results & scoring

### D5.1 Timing precision — hand vs. fully automatic
Three recognised official timekeeping methods: **Hand Timing**, **Fully Automatic Timing (FAT)** from a Photo Finish System, and **Transponder System** timing (only for non-fully-in-stadium races and road/off-stadium events) ([CR&TR 2024, TR19 Timing and Photo Finish](https://worldathletics.org/download/download?filename=d88c772f-56b5-4945-a3a9-0a9d53a8b29d.pdf&urlslug=Competition%20and%20Technical%20Rules%20%E2%80%93%202024%20Edition)).

- **Hand timing rounding**: unless exactly 0.1s, times are rounded **up** to the next 0.1s (e.g. 10.11 → 10.2 on the track); off-stadium/whole-second events round up to the next whole second (e.g. 2:09:44.3 → 2:09:45). Three official Timekeepers (one Chief) time the winner and any record-relevant performance; if two of three watches agree, that time stands; if all three disagree, the middle time is official.
- **FAT accuracy**: the photo-finish composite image must be built from ≥1000 images/second for World Ranking category 1./2. competitions, or ≥100 images/second otherwise, synchronised to a 0.01s time-scale; start-signal-to-timing-system delay must be ≤0.001s.
- A system automatic at the finish but not the start (properly triggered by the Starter's signal) is treated as producing a **Hand Time**; a system automatic at the start but not the finish produces **neither** a valid Hand nor FAT time.

Swiss Athletics adds a national **timing homologation tier system** (WO Anhang 4.1):

| Homologation | Technology | Minimum spec |
|---|---|---|
| A | CCD line-scan camera + PC | ≥1000 vertical lines/sec, synced per WA TR19.1.2/19.13–19.23 (ALGE-Timing and Swiss Timing products confirmed compliant) |
| C | Photocell (light-barrier) chain | ≥2 photocells in series, 20cm height separation, ≤5cm beam width, full lane width |
| D | Transponder timing | Per WA Technical Rule 19.24 |

Homologation A is mandatory for all licence-required meets; Homologation C is permitted only for U18-and-younger categories and races over 400m at C-Meetings, and results on Homologation C are excluded from Swiss records/international-championship qualifying standards ([Swiss Athletics WO 2026, Anhang 4](https://www.swiss-athletics.ch/fileadmin/user_upload/www.swiss-athletics.ch/wettkaempfe/Wettkampfsupport/Reglemente_und_Unterlagen/Nationale_Reglemente/WO_2026_d.pdf)).

### D5.2 Standard status/qualification abbreviations
From Competition Rule 25 (used in start lists and results):

| Meaning | Code |
|---|---|
| Did not start | DNS |
| Did not finish (running/race walking/combined events) | DNF |
| No valid trial recorded (field) | NM |
| Disqualified (+ rule number) | DQ |
| Valid trial (High Jump / Pole Vault) | O |
| Failed trial | X |
| Passed trial | – |
| Retired from competition (field/combined events) | r |
| Qualified by place (track) | Q |
| Qualified by time (track) | q |
| Qualified by standard (field) | Q |
| Qualified by performance (field) | q |
| Advanced to next round by Referee | qR |
| Advanced to next round by Jury of Appeal | qJ |
| Advanced to next round by draw | qD |
| Bent knee (race walking) | > |
| Loss of contact (race walking) | ~ |
| Yellow Card (+ rule number) | YC |
| Second Yellow Card ⇒ Red Card | YRC |
| Red Card (+ rule number) | RC |
| Lane infringement | L |
| Competing under protest | P |
| No height (vertical jumps — NH is the commonly used practical equivalent of NM) | NH |

([CR&TR 2024, Competition Rule 25.2](https://worldathletics.org/download/download?filename=d88c772f-56b5-4945-a3a9-0a9d53a8b29d.pdf&urlslug=Competition%20and%20Technical%20Rules%20%E2%80%93%202024%20Edition); NH cross-checked against [WA Terms & Abbreviations, July 2023](https://worldathletics.org/download/download?filename=WA_Terms__Abbreviations_EN+-+July+2023v2.pdf&urlslug=Terms+and+Abbreviations))

An athlete is deemed **DNS** if, having been on the start list, they do not report to the Call Room, or (having passed through the Call Room) make no attempt to start/compete (CR&TR, Rule 25 area).

### D5.3 Wind measurement
Non-mechanical wind gauges are mandatory for World Ranking category 1./2.(a)(b)(c)(e) competitions and for any performance submitted as a World Record. Gauge placement: beside the straight, adjacent to lane 1, 30m from the finish for 50/60m races, 50m from the finish for 100/110m/200m races; measuring plane 1.22m ± 0.05m high, ≤2m from the track. Measurement window from the gun flash/smoke:

| Event | Measurement window |
|---|---|
| 50m / 50mH / 60m / 60mH | 5 seconds |
| 100m / 100mH | 10s / 13s |
| Other events up to 200m | per event-specific window in TR17.8–17.13 |

A performance is **wind-aided** (ineligible for records) if the wind velocity averages **more than +2.0 m/s** (tailwind) in the direction of running, for sprint and horizontal-jump events up to and including 200m ([CR&TR 2024, TR17 Wind Measurement; CR31.14.5](https://worldathletics.org/download/download?filename=d88c772f-56b5-4945-a3a9-0a9d53a8b29d.pdf&urlslug=Competition%20and%20Technical%20Rules%20%E2%80%93%202024%20Edition)). Swiss Athletics applies the same +2 m/s ceiling to combined-events records, computed as the **average** wind across the wind-affected disciplines (e.g. 100m + longjump + 110mH ÷ 3) ([Swiss Athletics WO 2026, §14.2.3(d)](https://www.swiss-athletics.ch/fileadmin/user_upload/www.swiss-athletics.ch/wettkaempfe/Wettkampfsupport/Reglemente_und_Unterlagen/Nationale_Reglemente/WO_2026_d.pdf)).

### D5.4 Combined-events scoring
Combined events (decathlon, heptathlon, pentathlon, etc.) are scored per discipline using the **World Athletics Scoring Tables of Athletics** (by Dr. Bojidar Spiriev), an event-specific formula converting a mark/time to points; per-event points sum to the total ([World Athletics Scoring Tables of Athletics, PDF](https://worldathletics.org/download/download?filename=b3a75258-100e-4915-9eac-7c7760e775a0.pdf&urlslug=World%20Athletics%20Scoring%20Tables%20of%20Athletics)). These same tables underlie both combined-events scoring and the World Rankings "Result Score" (D6.3) — one canonical scoring engine serves both purposes, which is architecturally relevant.

### D5.5 Team scoring in Swiss inter-club competition (SVM)
Swiss club team competition is the **Schweizer Vereinsmeisterschaft für Leichtathletik (SVM)** — governed by a separate SVM Reglement (not itself part of the WO; referenced by it) ([Swiss Athletics WO 2026, Anhang 8 — Mitgeltende Unterlagen](https://www.swiss-athletics.ch/fileadmin/user_upload/www.swiss-athletics.ch/wettkaempfe/Wettkampfsupport/Reglemente_und_Unterlagen/Nationale_Reglemente/WO_2026_d.pdf)). Note: the research brief's tentative "CSI" could not be verified — **the correct, confirmed abbreviation is SVM**, not "CSI"; no reference to "CSI" was found in any Swiss Athletics primary source consulted. A secondary source (Swiss Athletics reglemente listing, not independently re-verified against the primary PDF in this pass — the specific 2024/2026 SVM Reglement PDF URLs returned HTTP redirects/placeholder pages rather than the document, so the exact point table below is **TBD/unverified against the primary text**) describes a placing-based points system (e.g., 8 points 1st place, 7 for 2nd, 6 for 3rd, descending, summed across disciplines) and nationality quotas for the top leagues (in National League A–C and Promotion League A, at least half of scored athletes per discipline must be Swiss/Liechtenstein citizens; unrestricted in Promotion League B, Junior League, Masters and other youth categories). **Action for requirements team**: fetch the current SVM Reglement PDF directly from `https://www.swiss-athletics.ch/wettkaempfe/wettkampfsupport/reglemente-unterlagen/` before finalising SYS-### requirements that depend on exact point values.

Off-stadium (cross country, road) Swiss team championships (WO §11.7) require teams of 3–5 runners, with the team's scoring based on finishing order (WO Article 10 cross-reference) — exact scoring formula not further detailed in the WO body text examined; **TBD**.

### D5.6 Ties
Not independently re-verified for track events in this pass beyond the general Rule structure; CR&TR Part III (Field Events) contains specific tie-break procedures for jumps/throws (best next-best mark, jump-off for 1st place in some championships) and the World Rankings tie-break at D6.3 (best Performance Score, then second-best, etc.). **TBD**: full track-event tie rule text not extracted in this research pass — flag for direct CR&TR Rule 26 lookup if precision is needed for a SYS requirement.

---

## D6. Records & rankings

### D6.1 Record/best-performance abbreviation scheme (World Athletics)
| Level | Outdoor Senior | Outdoor U20 | Outdoor U18 | Indoor Senior | Indoor U20 |
|---|---|---|---|---|---|
| World | WB | WU20B | WU18B | WIB | WU20IB |
| Area | AB | AU20B | AU18B | AIB | AU20IB |
| National | NB | NU20B | NU18B | NIB | NU20IB |
| Championship | CHB | CHB | CHB | CHB/CPB | CHB/CPB |
| Personal | PB (all levels) | | | | |
| Season | SB (all levels) | | | | |
| Qualification-period | QB | | | | |

(WR/AR/NR are the common shorthand for World/Area/National **Record**, distinct from the "Best"(B)/"List"(L) terminology World Athletics uses formally in its abbreviation scheme.) ([WA Terms & Abbreviations, July 2023, §6](https://worldathletics.org/download/download?filename=WA_Terms__Abbreviations_EN+-+July+2023v2.pdf&urlslug=Terms+and+Abbreviations))

### D6.2 World Record ratification requirements (Competition Rule 31)
Key conditions, verified against the primary rule text:
- Bona fide competition, duly arranged/advertised/authorised in advance by the host Member; ≥3 athletes (individual) or ≥2 teams (relay) as genuine competitors; no mixed-gender competition (with limited exceptions).
- The athlete must be doping-control tested **immediately** after the performance; the sample must go to a WADA-accredited lab; for endurance events ≥400m, must be analysed for Erythropoiesis Stimulating Agents (ESA). Failure to test, non-compliant testing, unsuitable/unanalysed sample, or an anti-doping rule violation each independently blocks ratification.
- Official World Record Application Form must reach the World Athletics Office within **30 days** of the competition, including the competition programme, complete results, and (for track records) the photo-finish and zero-control-test images.
- For races up to and including 800m, **only FAT-timed** performances are ratifiable.
- Outdoor performances up to 200m require wind velocity data; **>2.0 m/s average tailwind voids ratification**.
- For performances up to 400m, a certified Start Information System (reaction-time capture) must have functioned correctly.
- Only the WA President and CEO jointly are authorised to ratify a World Record; disputed cases go to Council.

([CR&TR 2024, Competition Rule 31 — World Records](https://worldathletics.org/download/download?filename=d88c772f-56b5-4945-a3a9-0a9d53a8b29d.pdf&urlslug=Competition%20and%20Technical%20Rules%20%E2%80%93%202024%20Edition))

Swiss Athletics record ratification (WO §14.2.3) additionally requires: performance qualifies for the national "Bestenliste" (best-list); Swiss citizenship (or WA-recognised eligibility to represent Switzerland) for individual records; for **absolute** relay records, the entire team must be Swiss citizens, for **club relay** records at least half must be; Homologation-A timing only; a completed "Rekordprotokoll" (record report) submitted by the organiser and the chief referee (SR-Chef), including the zero-shot verification and finish image for track events. Swiss records exist from category U14 upward (stadium) ([Swiss Athletics WO 2026, §14.2](https://www.swiss-athletics.ch/fileadmin/user_upload/www.swiss-athletics.ch/wettkaempfe/Wettkampfsupport/Reglemente_und_Unterlagen/Nationale_Reglemente/WO_2026_d.pdf)).

### D6.3 World Rankings — basics
`Result Score + Placing Score = Performance Score`. `Ranking Score` = average of Performance Scores over a 12- or 18-month period (event-group dependent), rounded down. Result Scores use the same **World Athletics Scoring Tables of Athletics** as combined events (D5.4), adjusted for wind, Best Legal Jump, or downhill-course drop where relevant. Placing Scores are awarded only in the Final (with exceptions for the top competition categories, which also score the round before the final), and only to athletes representing their own Area/home country at Area/national championships (others score as "out of competition," Result Score only). Ten competition categories (OW, DF, GW, GL, A, B, C, D, E, F) each carry different Placing Score tables, OW (Olympics/World Championships) being richest. World Record bonus: +20 Ranking-Score points for a new WR in a Main Event (+10 for equalling), +10/+5 for Similar Events. Ties broken by best, then second-best, Performance Score ([World Athletics — 1. Basics of the World Rankings, in force from 1 January 2026](https://worldathletics.org/world-ranking-rules/basics)).

---

## D7. Timing & measurement integration

### D7.1 Recognised timing/measurement technologies
- **Photo finish**: FinishLynx, Swiss Timing/Omega, ALGE-Timing are the market-standard systems; ALGE-Timing and Swiss Timing products are explicitly named by Swiss Athletics as meeting WA Homologation-A requirements ([Swiss Athletics WO 2026, Anhang 4.1.1](https://www.swiss-athletics.ch/fileadmin/user_upload/www.swiss-athletics.ch/wettkaempfe/Wettkampfsupport/Reglemente_und_Unterlagen/Nationale_Reglemente/WO_2026_d.pdf)).
- **Transponder timing**: permitted only for non-fully-in-stadium and road/off-stadium events (WA Technical Rule 19.24; Swiss Homologation D).
- **Electronic distance measurement (EDM)**: used for field-event throws/jumps in higher-tier competitions; referenced generically in CR&TR Part III but exact EDM certification protocol not extracted in this pass — **TBD**.
- **Wind gauges**: must be manufactured/calibrated to international standards, verified by a nationally accredited body; non-mechanical (electronic) gauges mandatory for top-tier competitions and any record attempt (D5.3).
- **Start Information System (false-start detection)**: required for World Record ratification up to 400m (reaction-time capture); integrates with the Starter's signal and the FAT system so the overall gun-to-timing-system delay is ≤0.001s.

### D7.2 Data flow between timing systems and meet-management software
FinishLynx (as the dominant photo-finish vendor) documents three file types that flow **into** the timing system from meet-management software (`.PPL` competitor/bib list, `.SCH` event/round/heat schedule, `.EVT` per-heat lane assignments) and one file type that flows **out**: the **`.LIF` (Lynx Information File)** — a comma-separated results file per event/round/heat: `Place, ID, lane, last name, first name, affiliation, <time>, license, <delta time>, <ReacTime>, <splits>, time trial start time, user1, user2, user3`. This is the de facto interchange format most meet-management platforms (Roster Athletics, Athletic.net/AthleticRUNMEET, LynxPad) consume to populate results and drive live scoreboards ([FinishLynx — File Formats for Meet Management Integration](https://finishlynx.com/file-formats-meet-manager/)). This is directly relevant to requirements: the tournament system should be able to **import LIF-format results** (or equivalent CSV) from third-party timing hardware, not just operate as a closed system.

---

## D8. Officiating

### D8.1 Key roles (World Athletics)
| Role | Function |
|---|---|
| Technical Delegate(s) | Set qualifying standards, arrange rounds, approve start lists, decide pre-competition matters; first-instance authority on athlete-eligibility protests |
| Referee (per discipline group: Running/Race Walking, Field, Combined Events, Start) | Overall authority over conduct of events in their area; decides protests or refers to Jury of Appeal |
| Starter / International Starter | Complete control of the start of a race; disqualifies athletes for false starts |
| Judges (incl. Umpires) | Assist the Referee; no independent decision authority; signal trial validity (white/red flag) |
| Chief/Photo Finish Judge | Operates and certifies the FAT/photo-finish system; supervises all photo-finish functions |
| Wind Gauge Operator | Measures and records wind velocity per event |
| Chief Timekeeper + Timekeepers | Hand-time backup/primary timing per TR19 |
| Call Room Judges / Chief Call Room Judge | Check-in athletes, verify uniform/bib/equipment compliance, release athletes to the competition area on schedule |
| Marshal | Controls Field of Play access (athletes, officials, media, volunteers) |
| Jury of Appeal (3, 5, or 7 members) | Final-instance decision on appeals under Rule 8; members must recuse if their own federation's athlete is involved |
| Technical Information Centre (TIC) Manager | Runs the communications hub between team delegations, organisers, Technical Delegates, and competition administration |

([CR&TR 2024, Competition Rules Part II — Officials, and Technical Rules TR16–TR20](https://worldathletics.org/download/download?filename=d88c772f-56b5-4945-a3a9-0a9d53a8b29d.pdf&urlslug=Competition%20and%20Technical%20Rules%20%E2%80%93%202024%20Edition))

### D8.2 Swiss officiating structure
Swiss Athletics uses **SR** (Schiedsrichter/in — referee) as the core official role, with an **SR-Chef/in** (chief referee) leading a competition's officiating team and chairing the Jury; **NTO** (National Technical Official) supplements the Jury at Swiss Championships. A Jury (ideally 3-person, minimum 2-person with the SR-Chef casting any tie-break vote) is convened before every official competition to rule on appeals per a standard "Einspruch und Berufung" (protest and appeal) data sheet. Starters ("Starter/innen") are graded and allocated by a central "Starter-Kommission" or regional "AST-KLV" body, with a fixed number of experienced ("Exp.") vs. standard starters per competition tier (e.g., 3 experienced + 1 standard for the Swiss senior stadium championship) ([Swiss Athletics WO 2026, Anhang 1–3](https://www.swiss-athletics.ch/fileadmin/user_upload/www.swiss-athletics.ch/wettkaempfe/Wettkampfsupport/Reglemente_und_Unterlagen/Nationale_Reglemente/WO_2026_d.pdf)).

### D8.3 Protest and appeal workflow (verified rule text)
1. **Eligibility protests** (an athlete's right to compete) → made to the Technical Delegate(s) before the competition starts; decision appealable to the Jury of Appeal; if unresolved before competition, the athlete competes "under protest" pending referral to the governing body.
2. **Result/conduct protests** → must be made **within 30 minutes** of the official announcement of that event's result; oral, to the Referee, by the athlete, a representative, or a team official; only someone competing in the same round may protest. The Referee may decide directly or refer to the Jury of Appeal.
3. **Appeal to the Jury of Appeal** → must be lodged **within 30 minutes** of (a) the announcement of the amended result following the Referee's decision, or (b) being informed of "no change." The appeal must be **in writing**, signed, and accompanied by a **USD 100 deposit** (forfeited if the appeal fails).
4. The Jury of Appeal's decision (or the Referee's, if no Jury or no appeal made) is **final — no further right of appeal, including to CAS.**

([CR&TR 2024, Competition Rule 8 / Technical Rule 8 — Protests and Appeals](https://worldathletics.org/download/download?filename=d88c772f-56b5-4945-a3a9-0a9d53a8b29d.pdf&urlslug=Competition%20and%20Technical%20Rules%20%E2%80%93%202024%20Edition))

### D8.4 False-start rule (current — since 2010)
Any athlete responsible for a false start is **disqualified immediately** (one-strike rule), except in Combined Events, where the first false start draws a warning (yellow/black card) to the responsible athlete(s) and a general warning to the field; any subsequent false start in that Combined Events race results in disqualification of the responsible athlete(s) only ([CR&TR 2024, Technical Rule 16 — False Start](https://worldathletics.org/download/download?filename=d88c772f-56b5-4945-a3a9-0a9d53a8b29d.pdf&urlslug=Competition%20and%20Technical%20Rules%20%E2%80%93%202024%20Edition); historical context in [World Athletics — "The New False Start Rule"](https://worldathletics.org/news/news/the-new-false-start-rule)).

---

## D9. Para athletics

### D9.1 Classification system
World Para Athletics recognises **10 eligible impairment types**: 8 physical (impaired muscle power, impaired passive range of movement, limb deficiency, leg-length difference, short stature, hypertonia, ataxia, athetosis) plus vision impairment and intellectual impairment. Classification groups athletes into **Sport Classes**, prefixed:
- **T** — track and jump disciplines (running, jumping, wheelchair racing, Frame Running)
- **F** — field/throwing disciplines (standing throws, seated throws)

Class-number ranges by impairment/discipline group (from the official overview):

| Class range | Discipline | Impairment group |
|---|---|---|
| T/F11–13 | Running/jumping / throws | Vision impairment |
| T/F20 | Running/jumping / throws | Intellectual impairment |
| T/F35–38 | Running/jumping / throws | Coordination impairments (hypertonia, ataxia, athetosis) |
| T/F40–41 | Running/jumping / throws | Short stature |
| T/F42–44 | Running/jumping / throws | Lower limb, competing without prosthesis |
| T/F45–47 (T), T/F45–46 (F) | Running/jumping / throws | Upper limb |
| T/F61–64 | Running/jumping / throws | Lower limb, competing with prosthesis |
| T32–34 / F31–34 | Wheelchair racing / seated throws | Coordination impairments |
| T51–54 / F51–57 | Wheelchair racing / seated throws | Limb deficiency, leg-length difference, impaired muscle power/ROM |
| T71–72 | Frame Running | Coordination impairments |

Eligibility requires an eligible impairment **and** meeting minimum impairment criteria under the World Para Athletics Classification Rules and Regulations (February 2023 edition); medical diagnostic forms are submitted through the SDMS online system ([World Para Athletics Classification & Categories](https://www.paralympic.org/athletics/classification)).

### D9.2 Integration into mainstream meets
Competition Rule 25.2–25.3 explicitly permits athletes of different age categories **or para classifications** to compete together in the same race/competition below top level — including to meet minimum-competitor-number requirements — with results presentation rules set out accordingly ([CR&TR 2024, Competition Rule 25.2–25.3](https://worldathletics.org/download/download?filename=d88c772f-56b5-4945-a3a9-0a9d53a8b29d.pdf&urlslug=Competition%20and%20Technical%20Rules%20%E2%80%93%202024%20Edition)). This is directly relevant to requirements: a meet-management system must support **mixed-field races** where results are subsequently split and re-ranked per age/classification group.

### D9.3 RAZA scoring
**TBD — could not verify.** No primary-source confirmation of a "RAZA" scoring system was found in this research pass (searches returned no results, and it does not appear on World Para Athletics' classification page). This may be a naming confusion with the **Raza Points System / Age Factor system** used in wheelchair racing/Masters contexts, or a system specific to a national federation. Flag for a dedicated follow-up search before writing any SYS-### requirement referencing it — do not assume a specific formula exists without further verification.

---

## D10. Sanctioning & data flow

### D10.1 Swiss meet sanctioning (calendar) process
1. Organiser (must itself be a Swiss Athletics member club/federation) enters all meet data in the Swiss Athletics online competition-administration tool ≥30 days before the event.
2. For World-Ranking-eligible meets (A-Meetings), coordination with World Athletics/European Athletics registration deadlines applies in parallel; for B-Meetings, dual registration in **both** the Swiss Athletics tool and the **World Athletics Global Calendar** is required.
3. Swiss Athletics reviews and issues (or refuses/revokes) the sanction ("Bewilligung"), which requires the venue to hold a valid facility homologation.
4. Referee/Starter allocation flows through a central ("Aufgebotsstelle") or regional (KLV) assignment body depending on meet tier.

([Swiss Athletics WO 2026, §4.2.1–4.2.3, §5.2](https://www.swiss-athletics.ch/fileadmin/user_upload/www.swiss-athletics.ch/wettkaempfe/Wettkampfsupport/Reglemente_und_Unterlagen/Nationale_Reglemente/WO_2026_d.pdf))

### D10.2 Results flow to federation databases / world rankings
World Athletics states results must be "officially ratified" by World Athletics, an Area Association, or a Member Federation to count toward World Rankings; a public submission channel exists at World Athletics' "Send Competition Results" page (referenced in the World Rankings navigation: `/records/send-competition-results`) ([World Athletics — 1. Basics of the World Rankings](https://worldathletics.org/world-ranking-rules/basics)). **TBD**: the exact technical submission format/API for results into the World Athletics database was not found in this pass — likely a manual/federation-mediated process rather than a public API; do not assume machine-to-machine integration is available without further confirmation from World Athletics directly.

### D10.3 Result submission formats
The most concretely documented interchange format found is the **FinishLynx `.LIF` file** (D7.2) — a de facto industry-standard CSV used between timing hardware and meet-management software, and in turn often re-exported to federation/results-portal uploads. No universal federation-mandated XML/JSON results schema was identified in this pass (World Athletics' internal systems are not publicly documented at API level); this is an **open question** for the requirements team to resolve with Swiss Athletics directly if system-to-system result submission is in scope.

---

## D11. Venue operations reality

### D11.1 Call Room
A "room or area where the athletes report and undergo certain checking procedures prior to their event" ([WA Terms & Abbreviations, July 2023](https://worldathletics.org/download/download?filename=WA_Terms__Abbreviations_EN+-+July+2023v2.pdf&urlslug=Terms+and+Abbreviations)). The Chief Call Room Judge publishes a schedule (first/final entry times per Call Room, departure time to the competition area) coordinated with the Competition Director; Judges verify uniform compliance, bib correctness against the start list, spike specifications, and permitted equipment before releasing athletes ([CR&TR 2024, Call Room Judges](https://worldathletics.org/download/download?filename=d88c772f-56b5-4945-a3a9-0a9d53a8b29d.pdf&urlslug=Competition%20and%20Technical%20Rules%20%E2%80%93%202024%20Edition)). An athlete who fails to report to the Call Room is recorded **DNS** (D5.2).

### D11.2 Technical Information Centre (TIC)
Mandatory for the top competition tiers, recommended for any multi-day competition; may be physical, virtual, or both. Its core function is smooth communication between team delegations, organisers, Technical Delegates, and competition administration on technical matters — including receiving protests when the Referee is not directly reachable, and recording the official time of result announcements (which starts the 30-minute protest/appeal clocks, D8.3). Larger events may run satellite "Sport Information Desks" (SID) in athlete accommodation, networked back to the TIC ([CR&TR 2024, Technical Information Centre](https://worldathletics.org/download/download?filename=d88c772f-56b5-4945-a3a9-0a9d53a8b29d.pdf&urlslug=Competition%20and%20Technical%20Rules%20%E2%80%93%202024%20Edition)).

### D11.3 Start lists and live results boards
Start lists must be posted/available before each round; results (including amended results after protest decisions) must be posted with their announcement time recorded, since that timestamp is the trigger for protest/appeal deadlines (D8.3). Field-of-play result capture at higher tiers uses "digital infield result capture for live results," falling back to manual scoreboards where unavailable — explicitly required to show each athlete their **trial timer** at A-Meetings ([Swiss Athletics WO 2026, §4.2.1(f)](https://www.swiss-athletics.ch/fileadmin/user_upload/www.swiss-athletics.ch/wettkaempfe/Wettkampfsupport/Reglemente_und_Unterlagen/Nationale_Reglemente/WO_2026_d.pdf)).

### D11.4 Offline capability rationale
No primary source explicitly states "the internet is unreliable at venues," but the structural evidence strongly supports designing for **offline-first operation**:
- Timing hardware (FinishLynx, ALGE, Swiss Timing) produces local files (`.LIF`, etc.) on a **local shared folder/network**, not a cloud API (D7.2) — the reference integration pattern is explicitly local-machine/local-network file exchange.
- The TIC may be run as a **physical-only** operation with no requirement for connectivity beyond the venue.
- Protest/appeal deadlines are **time-of-announcement-driven** (a physical/local event), not dependent on any online system being available.
- Many Swiss C-Meetings (the largest volume tier by count) explicitly permit lower-grade Homologation-C hardware and manual scoreboards, i.e. are designed to function without networked digital infrastructure at all.

This is a reasonable, evidence-supported basis for an **offline/local-first architecture requirement**, but the specific claim "venue internet is commonly unreliable" itself should be flagged as an **inference from these operational patterns**, not a directly cited fact.

---

## Open items requiring follow-up (see also `docs/requirements/open-questions-and-assumptions.md`)
1. **CSI vs. SVM** — the internal research brief tentatively used "CSI"; this research **confirms SVM (Schweizer Vereinsmeisterschaft)** is the correct term for the Swiss inter-club championship. No evidence of "CSI" was found anywhere in Swiss Athletics primary sources.
2. **SVM point-scoring table** — exact per-place point values and nationality-quota rules are only sourced secondarily in this pass (search-engine synthesis); the primary SVM Reglement PDF URLs attempted returned redirects/placeholder pages, not the document. Needs a direct fetch from `swiss-athletics.ch/wettkaempfe/wettkampfsupport/reglemente-unterlagen/` before finalising any SYS-### requirement with specific point values.
3. **RAZA scoring** — could not verify this exists as a named system; do not build a requirement around it without further research.
4. **Field-event flight/group size thresholds and full track tie-break rule text (Rule 26)** — not extracted in this pass; low risk (operational detail, not structural) but flag if precise tie-break logic is needed for a testable SYS-### requirement.
5. **World Athletics results-submission API/format** — no public API found; likely federation-mediated/manual. Confirm directly with Swiss Athletics if system-to-system integration is in scope for MVP or later.
6. **EDM (electronic distance measurement) certification protocol** — not extracted from CR&TR Part III in this pass.
7. **Swiss licence fee amounts (CHF 60/120)** — sourced only secondarily; verify against the current Gebührenreglement if fee handling becomes an in-scope feature.

---

## Sources

- World Athletics, [Competition and Technical Rules — 2024 Edition](https://worldathletics.org/download/download?filename=d88c772f-56b5-4945-a3a9-0a9d53a8b29d.pdf&urlslug=Competition%20and%20Technical%20Rules%20%E2%80%93%202024%20Edition) (primary rulebook; downloaded and parsed directly for this research)
- World Athletics, [Seedings, Draws and Qualification in Track Events](https://www.northernathletics.co.uk/wp-content/uploads/2025/04/WA-Seeding-Draws-and-Qualifications.pdf) (Technical Rule 20 reference document, hosted by Northern Athletics)
- World Athletics, [Terms and Abbreviations, July 2023](https://worldathletics.org/download/download?filename=WA_Terms__Abbreviations_EN+-+July+2023v2.pdf&urlslug=Terms+and+Abbreviations)
- World Athletics, [World Rankings — 1. Basics of the World Rankings](https://worldathletics.org/world-ranking-rules/basics) (in force from 1 January 2026)
- World Athletics, [Scoring Tables of Athletics](https://worldathletics.org/download/download?filename=b3a75258-100e-4915-9eac-7c7760e775a0.pdf&urlslug=World%20Athletics%20Scoring%20Tables%20of%20Athletics)
- World Athletics, ["The New False Start Rule"](https://worldathletics.org/news/news/the-new-false-start-rule)
- World Masters Athletics, [What are the age categories for Master Athletics?](https://world-masters-athletics.org/sp_faq/2197/)
- International Paralympic Committee / World Para Athletics, [Classification & Categories](https://www.paralympic.org/athletics/classification)
- Swiss Athletics, [Wettkampfordnung für Leichtathletik (WO 2026), valid from 1 January 2026](https://www.swiss-athletics.ch/fileadmin/user_upload/www.swiss-athletics.ch/wettkaempfe/Wettkampfsupport/Reglemente_und_Unterlagen/Nationale_Reglemente/WO_2026_d.pdf) (primary national competition regulation; downloaded and parsed directly)
- Swiss Athletics, [Reglemente & Unterlagen (regulations index)](https://www.swiss-athletics.ch/wettkaempfe/wettkampfsupport/reglemente-unterlagen/)
- Swiss Athletics, [Bestenlisten/Limiten example (LT Athletics club mirror)](https://lt-athletics.ch/rekorde-bestenlisten/)
- FinishLynx, [File Formats for Meet Management Integration](https://finishlynx.com/file-formats-meet-manager/)
- Secondary/unverified (flagged inline): search-engine-synthesised summaries on SVM point scoring and Swiss licence fees — see Open Items above.
