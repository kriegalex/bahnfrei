# Competitive & Market Analysis — Athletics Tournament Management Systems

Status: Phase A research deliverable (implementation-free). Prepared for the requirements
baseline. All capabilities/pricing not independently confirmed from a primary source are marked
**TBD** — nothing here is invented.

## Executive summary

- **Name correction:** the founder's seed statement wrote "**Setlec**" — the correct name is
  **Seltec** (company **Seltec GmbH**, trading as **Seltec Sports**; product **Track and Field 3**,
  short **TAF3**). See [C1](#c1-seltec-sports-track-and-field-3--lanet--laportal).
- **Swiss Athletics does not run one monolithic "system."** It operates a federated stack of at
  least three independent platforms with manual/CSV bridges between them: **TAF3/Seltec** (free,
  Windows desktop meet-day software), **Alabus** (a third-party association-management SaaS used
  for athlete licences and competition entries), and **World Athletics' own Global Calendar**
  (separate registration required for World Ranking Competition status). See
  [C2](#c2-swiss-athletics-as-a-system-of-systems).
- **There is no single mandatory World Athletics data-interchange standard.** Rules governance
  (Competition and Technical Rules) is centralised, but result-data interoperability is driven by a
  community-led, not-yet-widely-adopted effort: the **W3C Open Athletics Community Group**
  (formerly "OpenTrack CG"), incubating a Schema.org-based competition data model. World Athletics
  itself procures results/statistics services via commercial RFP rather than mandating an open
  format. See [C3](#c3-world-athletics-rules-governance--data-interoperability).
- **The incumbent landscape is fragmented by design**: national federations each mandate or
  recommend a different desktop tool (Germany/Switzerland/Austria/Luxembourg → Seltec TAF3;
  Netherlands → Seltec TAF3 *or* Atletiek.nu; USA → HyTek Meet Manager / MeetPro / RaceTab), almost
  all of it **Windows-only, desktop-first, and file/USB-based** for timing integration. Modern
  cloud-native entrants (OpenTrack, Roster Athletics, Athletics.app) are still niche and mostly
  paid/closed-source.
- **No actively maintained, general-purpose open-source meet-management system was found.** The
  only OSS artefact in this space is the W3C Open Athletics vocabulary/spec work (not a meet
  manager) and narrow single-purpose tools (e.g. `CrossMgr` for cross-country/cycling timing
  integration). This is a genuine gap an OSS athletics tournament system can fill.
- **Top differentiation opportunities**: browser-based/offline-capable architecture (vs.
  Windows-only desktop clients), true multilingual DE/FR/IT/EN support out of the box, native
  para-athletics classification handling, an open and documented data-exchange format aligned with
  the emerging W3C Open Athletics model, transparent open licensing (vs. opaque federation-funded
  bundling), and a modern integration story for common timing hardware (FinishLynx, ALGE-Timing,
  TimeTronics) via their existing file/network protocols.

---

## C1. Seltec Sports (Track and Field 3 / LA.net / LA.portal)

### C1.1 Company and naming (verified)

- Company: **Seltec GmbH**, trading as **Seltec Sports** — [seltec-sports.com](https://www.seltec-sports.com/).
- Support contact numbers published by Swiss Athletics are German (+49 69 247 538 970) and Austrian
  (+43 720 601 776), and a Facebook business listing shows "Seltec GmbH | Bad Vöslau" (Austria) —
  [Swiss Athletics TAF3 page](https://swiss-athletics.ch/wettkaempfe/wettkampfsupport/taf3-/-seltec/),
  [Seltec GmbH Facebook](https://www.facebook.com/seltecsports/). Exact registered HQ location is
  **TBD** (not confirmed from a primary corporate-registry source); the company clearly operates
  across the DACH region (Germany/Austria/Switzerland) plus Luxembourg and the Netherlands.
- The seed statement's "Setlec" is a misspelling; all primary sources (Swiss Athletics, DLV-affiliated
  regional federations, Seltec's own domains) consistently use **Seltec**.

### C1.2 Product family (verified from vendor + federation sources)

| Component | What it is | Source |
|---|---|---|
| **Track and Field 3 (TAF3)** | Windows-based, network-capable desktop meet-management application for stadium/track & field events — individual events, combined events (multi-events), and team competitions. Modules include a Technical Client (field events, electronic distance measurement/EDM), a Timing Client (interface to timing systems), a "Stellplatzmanager" (call-room/marshalling), and "Web.TEC 2" (infield result entry via smartphone). | [BW Leichtathletik FAQ](https://www.bwleichtathletik.de/home/wettkampf/wettkampfsoftware), [Seltec Wiki FAQ](https://wiki.seltec-sports.net/doku.php?id=taf3_faq), [Seltec Wiki downloads](https://wiki.seltec-sports.net/doku.php?id=downloads) |
| **LA.net 3** | Federation/club management and online-entries platform ("sports federation management and online entries"), branded per-federation (e.g. `lanet3.de`, `lanet3.com`). TAF3 can import entries directly from LA.net 3. | [Seltec Offering page](https://www.seltec-sports.com/offering/), [LA.net 3 login](https://lanet3.de/Identity/Account/Login) |
| **LA.portal** | The umbrella term for Seltec's results-publication websites — a single TAF3 upload publishes results to one of several federation-branded fronts: the generic [laportal.net](https://www.laportal.net/Competitions/Current), the DLV's [ergebnisse.leichtathletik.de](https://ergebnisse.leichtathletik.de), Swiss Athletics' **slv.laportal.net**, and Atletiekunie's **uitslagen.atletiek.nl**. Supports both a manual "upload current state" mode and a genuine live-push mode (a "Liveserver 2" process on the meet-office PC streams every status/result change) — requires only a low-bandwidth internet connection (mobile data is explicitly supported). | [Seltec Wiki: LA.portal](https://wiki.seltec-sports.net/doku.php?id=taf3_laportal) |
| **"LiveTicker" / "LA.live"** | No distinctly branded product by either name was found. This most likely refers colloquially to the **live-results push feature inside LA.portal/TAF3** described above (Liveserver 2). Flag as a naming discrepancy in the founder's seed statement — **verify with the founder** whether they meant a specific product or the live-results feature generally. | Not found under these names in vendor/federation sources — **TBD** |

### C1.3 Platform, licensing, and federation adoption (verified)

- **Platform:** TAF3 is explicitly documented as **Windows-only** ("Windows-basiertes
  netzwerkfähiges Programm") — [BW Leichtathletik FAQ](https://www.bwleichtathletik.de/home/wettkampf/wettkampfsoftware).
  No macOS/Linux client exists per any source found.
- **Germany (DLV):** Since **1 December 2019**, TAF3 has been the *bundeseinheitliche*
  (nationally standardised) competition software of the Deutscher Leichtathletik-Verband and its
  20 regional ("Landesverband") member federations, under a framework agreement between DLV and
  Seltec. Costs are borne collectively by DLV and the state federations so that individual clubs
  and meet organisers incur **no additional cost** —
  [Bayerischer LA-Verband](https://blv-sport.de/service/seltec-software).
- **Switzerland:** Swiss Athletics states TAF3 "**kann von allen Wettkampfveranstaltern in der
  Schweiz kostenlos genutzt werden**" (can be used free of charge by all competition organisers in
  Switzerland) — [Swiss Athletics TAF3/Seltec page](https://swiss-athletics.ch/wettkaempfe/wettkampfsupport/taf3-/-seltec/).
- **Netherlands:** Atletiekunie (Dutch federation) recognises **two** competition-administration
  systems: Seltec (TAF) and the homegrown **Atletiek.nu** — [Atletiek.nu](https://www.atletiek.nu/Atletiekunie.nl).
  Only Atletiek.nu currently has a one-way data connection into Atletiekunie's own back-office
  system ("Volta"); the reverse (Volta → Atletiek.nu, or any connection to Seltec) is not
  described as existing.
- **Luxembourg:** the national federation FLA references "Compétitions Seltec" on its site —
  [FLA](https://www.fla.lu/competitions-cache-271213v4) (content behind a cache page; treat
  adoption detail as **TBD** pending direct confirmation).
- **Pricing model for a non-federation buyer:** **TBD** — every confirmed pricing data point is a
  federation-wide bulk/framework arrangement; no public per-seat or per-meet price list was found.
- **Known limitations (from primary/vendor sources, not independent reviews):** Windows-only;
  a fresh SQL-like database file is required per competition (re-using one across events risks
  overwriting results, and competitions auto-close/lock after 48 hours); a training document
  explicitly instructs attendees to bring "a laptop with at least Windows 7"; live-publishing can
  fail silently on networks with an all-in-one router (Fritz!Box-style) misconfiguring a default
  gateway — Seltec Wiki documents a manual IP-configuration workaround —
  [Seltec Wiki: LA.portal](https://wiki.seltec-sports.net/doku.php?id=taf3_laportal). Independent
  (non-federation, non-vendor) user reviews/complaints were **not found**; forum threads
  (`seltec-sports.net/forum`) exist but a systematic reading of user-reported bugs was out of scope
  for this pass — flagged as **TBD** for deeper review if needed.

---

## C2. Swiss Athletics as a "system of systems"

The founder's brief to "take Swiss Athletics into account" is best read as: **integrate with the
federated stack Swiss Athletics actually operates**, not with a single named product. Verified
components:

1. **TAF3/Seltec** — meet-day operations (entries import, seeding, heats/rounds, timing interface,
   results, live publication to `slv.laportal.net`). Free for organisers, per C1.
2. **Alabus** — a third-party, general-purpose association/federation-management SaaS
   ("alabus associations") used by Swiss Athletics for **athlete/club licensing and competition
   entry administration**. TAF3 imports Alabus-held entries via a dedicated
   "Import/Export → Swiss Athletics" menu; only category/discipline combinations are transferred —
   **rounds and the time schedule must be re-created manually in TAF3**, and re-downloads
   overwrite/delete athlete entries that were removed in Alabus in the meantime (manually entered
   ones are preserved) — [Seltec Wiki: Swiss online entries FAQ](https://wiki.seltec-sports.net/doku.php?id=faq_schweiz_onlinemeldungen).
   Alabus itself is not athletics-specific; its marketing page is blocked to automated fetches by
   `robots.txt`, so its exact feature set for Swiss Athletics is **partially TBD** — confirm
   directly with Swiss Athletics or via manual browsing.
3. **Swiss Athletics' own competition-registration portal** (`swiss-athletics.ch`) — organisers
   log in with a licence number/password to register a competition with the federation itself
   (distinct from Alabus and from TAF3) — [Wettkampfverwaltung/Registrierung](https://swiss-athletics.ch/wettkaempfe/wettkampfsupport/wettkampfverwaltung/).
   Post-event, organisers owe Swiss Athletics a starting-fee levy per its `Gebührenreglement` and
   must submit a paper/Word "Startgeldabgabeformular."
4. **World Athletics' Global Calendar** — registering a meet as a **World Ranking Competition** is
   an entirely separate step, done directly on `globalcalendar.worldathletics.org`, not through
   Swiss Athletics' own portal or TAF3 — same source as above.
5. **Athlete licence/category system** — Swiss Athletics licence numbers ("Loginnummer/
   Lizenznummer") double as the login credential for competition entry — [Wettkampfanmeldung](https://swiss-athletics.ch/wettkaempfe/veranstaltungen/wettkampfanmeldung/).

### C2.1 The sanctioned-meet results chain, link by link (researched 2026-07-05)

For a Swiss sanctioned stadium meet, results must travel four hops. Openness verified per hop:

| Hop | Mechanism today | Open to third-party software? |
|---|---|---|
| Entries → meet software | Alabus → TAF3 via built-in "Import/Export → Swiss Athletics" (Alabus credentials) | **Unknown/TBD** — Alabus export format undocumented publicly; TAF3 also imports plain CSV/Excel entry lists, and organisers can always export from Alabus manually |
| Live/public results | TAF3 → LA.portal (`slv.laportal.net`) via "Extras → Auf LA.Portal veröffentlichen" + Liveserver push | **No** — Seltec-proprietary; but nothing prevents publishing results on one's own site *in addition* (policy check = OQ-013c) |
| **Results → federation DB (feeds Bestenliste, records, limits)** | TAF3 "Meeting Upload" to **Alabus**, required **same day** ("Alabus stops offering the competition for upload later" — [Seltec Wiki CH results FAQ](https://wiki.seltec-sports.net/doku.php?id=faq_schweiz_resultate)) | **This is the lock-in point.** The only tooling for the Alabus meeting upload lives inside TAF3. Fallback: a per-athlete manual web form ("Resultate melden", requires link to an official Rangliste) — infeasible for whole meets — [swiss-athletics.ch](https://swiss-athletics.ch/wettkaempfe/resultate/bestenliste/resultate-melden/) |
| Recognition | WO 2026: results from non-sanctioned competitions "receive no official recognition (Bestenliste, records, limits)" — [WO 2026 PDF](https://swiss-athletics.ch/fileadmin/user_upload/www.swiss-athletics.ch/wettkaempfe/Wettkampfsupport/Reglemente_und_Unterlagen/Nationale_Reglemente/WO_2026_d.pdf) | **RESOLVED (full WO read, 2026-07-05): the WO mandates TAF3 by name.** WO 2026 **§5.6a**: "Meisterschafts- und offizielle Wettkämpfe **müssen** mit der von Swiss Athletics kostenlos zur Verfügung gestellten Auswertungssoftware **TAF3** administriert werden"; **§5.6b** requires activating the TAF3 "auf LA-Portal veröffentlichen" function; **§5.3b** requires linking athletes to licence numbers *in TAF3* as the only route into the Bestenliste. So for championship + official competitions the lock is **normative, not just technical** — closed by regulation *and* by the Alabus seam. |

**The regulatory layers around this, for clarity (verified 2026-07-05):**

- **World Athletics certifies competition *equipment*, not meet software.** The WA
  Certification System covers photo-finish/timing systems, track equipment, implements etc.
  ([WA certified-equipment list](https://worldathletics.org/download/download?filename=023600cb-7331-4aa4-b746-1718f8ce410c.pdf&urlslug=CERTIFICATES%20-%20Certified%20Competition%20Equipment));
  there is no WA certification or homologation regime for competition-management software.
  Seltec holds no international "certification" — its position is purely contractual/regulatory
  per federation.
- **Germany (DLV):** a framework contract (Rahmenvertrag) with Seltec since the 2020 season
  covers TAF3 + exclusive results/Bestenliste functionality via leichtathletik.de
  ([BLV](https://blv-sport.de/service/seltec-software)). But the DLV Bestenliste **auto-ingests
  from two systems — TAF3 or COSA WIN** ([bestenliste.leichtathletik.de](https://bestenliste.leichtathletik.de/Performances?performanceList=4ccb20ca-2309-4462-9f18-ef1cf06db244&environment=0&year=2021&showForeigners=1)) —
  i.e., even the tightest Seltec market is not single-vendor at the rule level.
- **Switzerland:** the TAF3 mandate lives in the federation's own WO §5.6 (above) — an
  association rule Swiss Athletics can change annually, not a state regulation or an
  international requirement. Precedents for multi-system recognition: DLV (TAF3 + COSA WIN),
  Atletiekunie (two recognised systems, C4).

**Crucially, the youth-series tier has an official non-TAF3 path:** the Nachwuchsserien
organiser tool (UBS Kids Cup / Visana Sprint / Mille Gruyère family) lets organisers choose
between **"Excel-Sheet" (downloadable template) and "TAF3 von Seltec"** for results processing,
with results uploaded via the organiser login either way ([Visana Sprint tool guide](https://www.visanasprint.ch/fileadmin/user_upload/www.visanasprint.ch/veranstalter/dokumente_und_downloads/Tool_Nachwuchsserien/Anleitung_und_Funktionen_des_Tools_der_Nachwuchsserien_VS.pdf),
[Mille Gruyère guide](https://www.mille-gruyere.ch/fileadmin/user_upload/www.mille-gruyere.ch/Veranstalter/Dokumente_Downloads/Anleitung_und_Funktionen_des_Tools_der_Nachwuchsserien_MG.pdf)).
TAF3 is **free** for all Swiss organisers ([Swiss Athletics TAF3 page](https://swiss-athletics.ch/fr/competitions/support-de-competition/taf3-/-seltec/))
but at this tier demonstrably not required. Note also the Dutch precedent: Atletiekunie
officially recognises **two** competing systems (C4) — federation single-vendor mandates are
not a law of nature. And the Swiss gymnastics federation STV runs athletics events on its own
"STV-Contest" software, outside the Seltec chain entirely.

**Public TAF3 documentation exists in useful depth:** the Seltec wiki (public), a full TAF3
manual PDF (publicly hosted), and a TAF3 **result-export CSV** format stable enough that
third-party tools parse it ([weissreto/seltec-taf3-result-export](https://github.com/weissreto/seltec-taf3-result-export)).
TAF3 also ingests timing results as FinishLynx-family `.lif` files per event — a documented
seam through which an external system's results could, in principle, enter TAF3.

**Implication for requirements:** a Swiss-market-aware system should not assume "integrate with
Swiss Athletics" means one API. It plausibly means: (a) accept/produce entry data compatible with
what Alabus exports (format **TBD** — likely CSV/proprietary; needs direct verification), (b)
support the same manual re-entry-of-rounds workflow TAF3 requires unless a richer sync can be
negotiated, (c) support the federation's own competition-registration and fee-reporting process as
a manual/administrative step, and (d) support (or at least not block) separate World Athletics
Global Calendar registration for sanctioned meets. This should be logged as an open question in
`docs/requirements/open-questions-and-assumptions.md`.

---

## C3. World Athletics: rules governance & data interoperability

- **Rules source:** World Athletics' **Competition and Technical Rules** are the authoritative
  rulebook; sanctioning and result recognition are conditioned on compliance with these rules and
  on a prior sanctioning application — [World Athletics World Rankings basics](https://worldathletics.org/world-ranking-rules/basics).
- **World Rankings / sanctioned-meet submission:** results must be officially ratified by World
  Athletics or one of its Area Associations/Member Federations, and competition results should be
  submitted **"as soon as possible and, ideally, no later than 24 hours after the end of the
  competition"** to keep World Rankings, top-lists and qualification pathways current — [World
  Rankings Competitions Procedures (PDF)](https://assets.aws.worldathletics.org/document/63c917632d4be3c6ee496553.pdf).
  Domestic federations must nominate which National Permit competitions they intend to propose for
  World Rankings Competition status **no later than 60 days before** the event date — same source.
- **No single mandated result-data interchange format was found.** World Athletics runs its own
  procurement process for results/statistics tooling rather than publishing an open data
  specification — see its own **"Request for Proposal – Results and Statistical Services"**
  document, confirming this function is commercially outsourced rather than standardised in the
  open — [worldathletics.org RFP – Results & Statistical Services](https://worldathletics.org/download/download?filename=add43f20-bf15-46b5-aa99-71c010077f50.pdf&urlslug=Request+for+proposal+-+results+and+statistics+service).
- **The closest thing to an open standard is community-driven, not WA-mandated**: the
  **W3C Open Athletics Community Group** (historically and still informally called "OpenTrack CG",
  unrelated to the commercial vendor OpenTrack — see naming caution in C4) — a forum "composed of
  sports data enthusiasts," supported by **European Athletics**, that meets around the
  **AthTech Conference series** (`athtech.run`) — [w3c/opentrack-cg on GitHub](https://github.com/w3c/opentrack-cg).
  It has produced:
  - an **Athletics Conceptual Model** — [spec/model](https://w3c.github.io/opentrack-cg/spec/model/)
  - a **Competition Data Model and Vocabulary** based on Schema.org — [spec/competition](https://w3c.github.io/opentrack-cg/spec/competition/)
  - a working draft on **Sports Competition Digital Credentials** — [spec/credentials-uc](https://w3c.github.io/opentrack-cg/spec/credentials-uc/)

  This is a **W3C Community Group** deliverable, not a W3C Recommendation and not formally adopted
  by World Athletics — treat as an emerging/candidate standard, useful as a design reference for
  an OSS system's data model, but **not** a compliance requirement today. Prior-generation
  approaches referenced by the group include the IOC's **ODF** (Olympic Data Feed) and IPTC's
  **SportsML** (which the group explicitly notes has **no athletics-specific vocabulary**) — same
  GitHub source.
- **Historical IAAF/World Athletics XML result formats**: repeatedly referenced in general
  athletics-tech discourse but **no primary specification document was located** in this research
  pass — mark as **TBD**, worth a targeted follow-up query directly against
  `worldathletics.org/about-iaaf/documents/technical-information` if the architecture phase needs
  it.

---

## C4. Other meet-management systems — capability matrix

| System | Scope | Platform | Timing integration | Live results | Licensing | Notable adopters |
|---|---|---|---|---|---|---|
| **Seltec TAF3** (+ LA.net/LA.portal) | Track & field, combined events, team meets | Windows desktop, client/server LAN | Proprietary "Timing Client"; EDM for field events | Yes (LA.portal, upload or true live push) | Federation-funded, free to organisers where deployed (DE, CH) | DLV (Germany, mandatory since 2019), Swiss Athletics, NL (alongside Atletiek.nu), Luxembourg (FLA) — [C1](#c1-seltec-sports-track-and-field-3--lanet--laportal) |
| **HyTek Meet Manager for Track & Field (TFMM)** | Track & field, cross country, road; described as "most widely used meet management software in the world," used in 100+ countries | Windows | Photo-finish interface + button-finish interface (FinishLynx-class devices) | Via companion "Meet Mobile" app | Commercial (Active Network); per-licence pricing page exists but figures not captured in this pass — **TBD** | Global — [HY-TEK Meet Manager](https://hytek.active.com/track-meet-management.html), [Prices page](https://activenetwork.my.salesforce-sites.com/hytekswimming/articles/en_US/Article/Prices-for-Meet-Manager-for-Track-Field) |
| **OpenTrack** | Cloud competition management: entries, seeding, results capture, federation management | Web/cloud (SaaS) | FinishLynx and TimeTronics auto-upload integration | Yes | Commercial SaaS; "Federation Management" package in use by 5 countries per vendor | Vendor claims global federation customers — [OpenTrack product](https://opentrack.run/product/) — note: **this "OpenTrack" is an unrelated commercial company/brand**, distinct from the W3C "OpenTrack CG" data-standards effort in C3; do not conflate the two in requirements docs |
| **FinishLynx / LynxPad / FieldLynx** | Photo-finish timing (FinishLynx), lightweight companion meet-management (LynxPad), networked field-event scoring (FieldLynx) | Windows | Is itself the timing system; LynxPad is file-based (.lif/.evt/.sch, see C5) | ResulTV display product | Commercial; LynxPad pitched as cheaper than full meet-management suites | Reported "thousands of athletics stadiums across five continents" — [FinishLynx](https://finishlynx.com/product/software/finishlynx-results-software/), [LynxPad](https://finishlynx.com/product/event-management/lynxpad/) |
| **Athletic.net** | Results aggregation/statistics site + entry tools (AthleticLIVE, AthleticRUNMEET) for US school/club scene | Web | Ingests results from meet-management software (incl. FinishLynx LIFs) | Yes, real-time scoring/qualifying lines | Commercial/freemium; primarily US-centric | US high-school/college scene — [Athletic.net](https://www.athletic.net/), [FinishLynx LIF scoreboard setup](https://support.athletic.net/article/00j157j60y-setup-a-live-scoreboard-with-finish-lynx-lifs) |
| **Roster Athletics** | Cloud competition + registration management, admin portal + mobile field-results app + consumer app | Web/cloud + mobile apps | ALGE-Timing (OPTIc3.NET), TimeTronics (MacFinish, Argus) | Yes, via consumer app | Commercial | **England Athletics** — [Roster Athletics × England Athletics](https://www.englandathletics.org/competitions-and-events/roster-athletics/), [integrations](https://support.rosterathletics.com/en/support/solutions/folders/44001195027) |
| **Athletics.app** | Competition discovery, entries/payment, results, consumer app; federation "customised solutions" (CRM/API/national top-lists) available | Web/cloud + mobile | "Integrates with all major brands, including Finishlynx and Timetronics" per vendor | Yes | Commercial (pricing page exists, not captured) | Vendor claims 15,000+ competitions, 300k+ athlete profiles indexed — [Athletics.app](https://www.athletics.app/), [timing systems page](https://www.athletics.app/home/competition-management/timing-systems/) |
| **MeetPro / MeetPro2** (DirectAthletics) | Track & field + cross country meet management, integrates with DirectAthletics/TFRRS | Windows and macOS | FinishLynx, FieldLynx, Flash Timing (T&F); MyLaps, IPICO, Time Machine, NK Interval 2000, Sprint 8, ClassFive, ChronoTrack (XC) | Web/scoreboard split-scores | Commercial, "every feature included, free upgrades," 30-day free trial | US collegiate/club scene — [tfmeetpro.com](https://tfmeetpro.com/), [features](https://tfmeetpro.com/features.html) |
| **RaceTab** | Track & field, cross country, road races | Windows | Compatible with most automatic timing systems; works alongside Athletic.net for entries/posting | Via partner platforms | **Free**, distributed/sponsored by MileSplit.us | US high-school/club scene — [RaceTab overview](https://ia.milesplit.com/articles/60765/racetab-free-meet-management-software) |
| **Atletiek.nu** | Track & field competition administration, one of two systems recognised by Atletiekunie (NL) | Web | FinishLynx & TimeTronics | Yes (LED scoreboards, per-athlete schedules) | Commercial; consumer app free with optional €3.99/month ad-free tier | Netherlands (Atletiekunie) — [Atletiek.nu](https://www.atletiek.nu/Atletiekunie.nl), [Atletiekunie integration note](https://www.atletiekunie.nl/kenniscentrum/wedstrijdorganisatie/baanwedstrijden/uitslagenverwerking/) |
| **Tilastopaja** | Named in the research brief; **not independently verified in this pass** — known in the field as a statistics/rankings data service rather than a meet-management system | **TBD** | **TBD** | **TBD** | **TBD** | **TBD** — flag for follow-up if the architecture needs a statistics-provider comparison |
| **AthleTIC** | Named in the research brief; no matching product was found under this exact name in this research pass | **TBD** | **TBD** | **TBD** | **TBD** | **TBD** — possibly a conflation with Athletic.net/Athletics.app; verify with founder |
| **W3C Open Athletics / "OpenTrack CG"** | Not a meet-management product — a data-model/vocabulary standards effort (see C3) | N/A | N/A | N/A | Open (W3C Software and Document License) | European Athletics-backed community group — [github.com/w3c/opentrack-cg](https://github.com/w3c/opentrack-cg) |

No actively maintained, general-purpose **open-source** meet-management application (entries →
seeding → timing → results, end to end) was located. The only adjacent open-source code found was
`CrossMgr` (cycling/cross-country race timing, includes a `FinishLynx.py` integration module) —
[esitarski/CrossMgr on GitHub](https://github.com/esitarski/CrossMgr/blob/master/FinishLynx.py) —
which is not athletics-specific and not a full meet manager. This gap is the core opportunity for
this engagement.

---

## C5. Timing-system ecosystem (integration targets)

An athletics meet-management system must be able to talk to whichever timing/measurement hardware
a venue already owns. Verified integration targets:

- **FinishLynx** (Lynx System Developers) — the de facto standard photo-finish system; described
  as "the gold standard for track and field results for 30+ years" — [FinishLynx](https://finishlynx.com/product/software/finishlynx-results-software/).
  Its companion file formats are the most concretely documented interchange mechanism found in
  this whole research pass:
  - **`.lif` (Lynx Information File)** — one per race; comma-separated: place, ID, lane, last
    name, first name, affiliation, time, licence, delta time, reaction time, splits, time-trial
    start time, user1–3 — [FinishLynx file formats](https://finishlynx.com/file-formats-meet-manager/).
  - **`lynx.evt`** — start lists / event & entrant definitions.
  - **`lynx.sch`** — event schedule.
  - **`lynx.ppl`** — ID-to-name/affiliation lookup table.
  These are propagated parent→child across multi-computer timing setups —
  [FinishLynx Database Files manual](https://help.finishlynx.com/Content/OnlineManual/DatabaseFiles.htm).
  Any OSS system targeting drop-in compatibility with existing Swiss/EU stadium timing rooms should
  plan to **import/export the `.lif`/`.evt`/`.sch` family** as a first-class integration, not just
  a generic CSV.
- **ALGE-Timing** — Austrian precision timing manufacturer; integrates with third-party platforms
  via its **OPTIc3.NET** software (confirmed via Roster Athletics' integration docs) —
  [ALGE-TIMING athletics](https://alge-timing.com/en/sports/athletics-t&f/athletics), [Roster ALGE integration](https://support.rosterathletics.com/en/support/solutions/folders/44001195027).
- **TimeTronics** — Belgian timing manufacturer; integrates via **MacFinish** and **Argus** —
  [TimeTronics athletics](https://www.timetronics.be/athletics), [Roster TimeTronics Argus integration](https://support.rosterathletics.com/en/support/solutions/articles/44002385675-timetronics-argus-integration).
- **Swiss Timing / OMEGA** (Swatch Group) — the certified provider for Olympic Games and World
  Athletics Championships-level events; contract with the IOC extended through 2032 —
  [Swiss Timing athletics](https://www.swisstiming.com/sports/athletics/). Realistically out of
  reach as an integration target for grassroots/regional meets — relevant mainly as context for
  what "top tier" timing looks like, not as a near-term integration requirement.
- Cross-cutting observation: **OpenTrack (commercial vendor)** explicitly lists integrations for
  "ALGE, FinishLynx, Seiko, Swiss Timing, and MacFinish/Timetronics" as its baseline timing-vendor
  support — i.e. the same handful of vendors recur across every competing platform, confirming
  these are the **table-stakes integration targets** for any new entrant —
  [OpenTrack competition management](https://opentrack.run/product/competition-management.html).

---

## C6. Gap analysis & open-source opportunity

### What incumbents do poorly

- **Platform lock-in.** TAF3, HyTek TFMM, RaceTab, and (older) MeetPro releases are Windows-first
  or Windows-only desktop applications; several require a fresh local database file per
  competition with manual re-entry of rounds/schedule after each entries sync (TAF3/Alabus flow,
  C2) — brittle, error-prone, and hard to run cross-platform or from a tablet/browser.
- **Fragmented, non-interoperable federation stacks.** Even a single national federation
  (Switzerland) chains together three to four separately-operated systems (Alabus, TAF3/Seltec,
  Swiss Athletics' own portal, World Athletics' Global Calendar) with manual, one-way, partial data
  syncs rather than one coherent pipeline (C2). This is the single clearest structural weakness an
  integrated OSS system can improve on.
- **No open, athlete-facing data standard in production use.** The only open standard effort
  (W3C Open Athletics, C3) is a community-group draft, not adopted by World Athletics or any
  federation studied here. Every commercial vendor exposes its own proprietary export/API surface.
- **Opaque pricing.** Every commercial system studied either bundles cost into a federation-wide
  contract invisible to the end organiser (Seltec/DLV, Seltec/Swiss Athletics) or simply does not
  publish pricing (OpenTrack, Athletics.app, HyTek's public price page was not captured in this
  pass) — poor terrain for a small/volunteer-run club evaluating options.
- **Multilingual support is inconsistent.** No evidence was found of any incumbent natively
  supporting the Swiss quadrilingual context (DE/FR/IT/EN) as a first-class, symmetric feature;
  Seltec's Swiss-specific documentation notes a French variant exists ("TAF 3 - En français") but
  this reads as a bolt-on rather than a core i18n architecture.
- **Para-athletics classification support:** **not verified as a first-class feature in any
  system studied** — no vendor page or federation FAQ reviewed described explicit
  classification-aware seeding/scoring (e.g. World Para Athletics class-based heats/records). Flag
  as an explicit open question for the requirements team — this may be a meaningful
  differentiator or may simply be under-documented by vendors.
- **Offline/venue-network robustness is treated as an afterthought.** Seltec's own documentation
  describes live-publishing failures caused by common consumer router/gateway misconfigurations at
  the venue (C1) — i.e. even the incumbent free-to-use system has known, documented fragility in
  exactly the "on-site/offline venue operation" scenario this engagement's brief calls out as
  important.

### Table-stakes features (must match to be viable)

- Entries, seeding, heats/rounds, multi-round progression, combined-events scoring.
- Import/export compatible with the FinishLynx `.lif`/`.evt`/`.sch` family (C5) at minimum;
  ALGE-Timing and TimeTronics integration as secondary targets.
- Live or near-live results publication reachable from a low-bandwidth/mobile venue connection.
- Federation-facing result submission/ratification workflow compatible with World Athletics'
  "ideally within 24 hours" sanctioning expectation (C3).
- Printable official documents (start lists, heat sheets, results, records, protest forms) — every
  incumbent studied supports this baseline.

### Differentiation opportunities for an OSS system

1. **Cross-platform, offline-first architecture** (not Windows-only desktop) — directly answers a
   documented incumbent weakness (C1, C6).
2. **First-class DE/FR/IT/EN multilingual UI and data model**, not a bolted-on translation.
3. **An explicit, versioned, open data-exchange schema**, ideally aligned with (and contributing
   back to) the W3C Open Athletics Community Group's Competition Data Model rather than yet
   another proprietary format (C3).
4. **Native para-athletics classification support** as a modelled first-class concept rather than
   a workaround — an area no incumbent was confirmed to handle explicitly.
5. **Transparent, permissive OSS licensing** with no federation-bundled pricing opacity — a
   selling point against every commercial incumbent studied.
6. **A coherent single pipeline for the "system of systems" problem** Swiss Athletics exemplifies
   (C2): licensing/entries → competition registration → meet-day operations → results → federation
   ratification → World Athletics sanctioning, ideally reducing today's several manual hand-offs to
   one traceable flow (while still integrating with Alabus/Swiss Athletics/World Athletics as
   external systems of record where replacing them is out of scope).

---

## C7. Field observations & live-results pipeline (UBS Kids Cup Le Mouret, 4 July 2026)

Founder field visit + follow-up verification (2026-07-05). Screenshots in this directory:
`swiss-athletics-ubs-kids-cup-{timetable,classement,participants}.png`.

### C7.1 The federation live-results page IS Seltec (verified)

`https://www.swiss-athletics.ch/fr/competitions/resultats/live-results/` contains an
**iframe embedding `https://slv.laportal.net/Competitions/Current?lang=fr`** (verified in the
page HTML, 2026-07-05). There is no separate Swiss Athletics results system: the observed
pipeline is **TAF3 at the venue → "Liveserver 2" push → LA.portal (`slv.laportal.net`) →
iframed into swiss-athletics.ch**. Whoever publishes results "on swiss-athletics.ch"
publishes through Seltec.

- No public LA.portal API or developer documentation was found (searches 2026-07-05); entry
  into the official surface appears possible **only via Seltec tooling** → closed ecosystem
  until a club/federation contact proves otherwise (OQ-013).
- Consequence for this project: our public results are **supplementary/unofficial** for
  sanctioned meets; the federation-visible channel stays Seltec unless negotiated otherwise
  (STR-042/SYS-076).

### C7.2 UBS Kids Cup tooling chain (verified from primary docs)

- **UBS Kids Cup** (with Visana Sprint and Mille Gruyère) is a Swiss youth series with its own
  event registry at `kidscup.swiss-athletics.ch` and its own 3-discipline format (60 m sprint,
  zone long jump, 200 g ball throw) scored by a **series-specific points table** — not the
  World Athletics scoring tables ([ubs-kidscup.ch](https://www.ubs-kidscup.ch/de/fuer-teilnehmer/startmoeglichkeiten/offene-lokale-ausscheidung)).
- Since the 2023 season, organizers of these youth series log into Seltec **with their Swiss
  Athletics organizer credentials** and load the competition directly into TAF3
  ([Mille Gruyère organizer manual, "Veranstaltermanual Seltec"](https://www.mille-gruyere.ch/fileadmin/user_upload/www.mille-gruyere.ch/Veranstalter/Dokumente_Downloads/Veranstaltermanual-Seltec_Mille-Gruyere_DE.pdf)).
- TAF3's Swiss licence includes a Toolbox function **"Generiere UKC"** that creates the UBS
  Kids Cup combined event per the current rulebook from existing individual events
  ([Seltec wiki: TAF3 Toolbox](https://wiki.seltec-sports.net/doku.php?id=taf3_faq_toolbox)).
- A legacy standalone Windows "UBS Kids Cup Auswertungsprogramm" also exists/existed for local
  eliminations ([manual PDF](https://www.ubs-kidscup.ch/fileadmin/ubskidscup/downloads/Handbuch_Auswertungssoftware_lokale_Ausscheidung.pdf)) — evidence that lightweight local-elimination
  tooling is an accepted pattern.
- The same Toolbox page confirms: TAF3 fetches athlete PB/SB from a central Seltec results
  pool and looks up World Athletics IDs (central athlete data services), and a **Para licence
  module** exists ("Parapunkte") — partially resolving TBD-003: Seltec does have para scoring
  support as a paid module.

### C7.3 UI observations relevant to requirements (from screenshots)

| Observation | Requirement relevance |
|---|---|
| Result lists show official-status line with **two timestamps**: scheduled time and "Resultats officiels 16:14:55" | Confirms announcement-timestamp model (SYS-047) |
| Public lists show **birth year** (AN column) for all children, and full names incl. kids born 2020; some without club ("7H GP ecole") | Privacy bar is LOW in the incumbent — our SYS-100 stance (minimize; birth year only where category-implied) is a differentiator, but must not block federation-conformant output |
| Mixed-language UI: tab "Veranstaltungen" (DE) among French tabs, "Resultats" unaccented | Confirms bolt-on i18n in incumbent (C6); symmetric i18n is a differentiator |
| Heat notation "6 SR, 25 P." (Serien/heats, participants), heat-place "1./V" (1st in heat 5) | Result-presentation conventions to reproduce (SYS-045/UC-010) |
| Marks with comma decimals ("7,67"), points column per UKC table, PB flags in Info column | Locale formatting (SYS-110); series-specific scoring (SYS-053); PB/SB flags (SYS-049) |
| Categories are per birth year (M7…M15, W7…W15), plus "UKC for all" division | Category schemes must support year-granular youth series (SYS-005), mixed-division events (SYS-052) |

**Founder field report (source: founder, 2026-07-04):** manual field-event measures (ball
throw, zone long jump) are taken by volunteers on the infield and entered later/elsewhere;
sprint timing was electronic; participant lists with bib numbers were printed from Seltec;
results were captured live in a room during the meet as slips came in; **the #1 need observed
for parents/families is fast, accurate access to results**.

---

## C8. Field-capture connectivity patterns across the industry (researched 2026-07-05)

How do existing systems get field-event results from the pit/circle into the meet system —
officials' devices over mobile data, or a venue network? Verified per system:

| System (tier) | Field-capture client | Transport | Offline tolerance | Source |
|---|---|---|---|---|
| **Seltec Web.TEC 2** (DE/CH club→national) | Official's own smartphone, browser | **Cloud relay**: TAF3 in the meet office uploads start lists to Seltec's server at `tec2.laportal.net`; phones reach that server over their **own mobile data**; results flow back to TAF3 via the Liveserver connection. Not venue Wi-Fi. | Fragile — vendor FAQ warns that re-uploading start lists "may lose results already captured on the device" | [Seltec Wiki Web.TEC 2 FAQ](https://wiki.seltec-sports.net/doku.php?id=taf3_faq_webtec) |
| **Seltec Technical Client** (bigger meets) | Meet-owned netbooks/laptops | Venue LAN (wired/Wi-Fi), incl. EDM | n/a (assumes stable LAN) | [BW-LA FAQ](https://www.bwleichtathletik.de/home/wettkampf/wettkampfsoftware) |
| **Atletiek.nu Jury-app** (NL, federation-recognised) | Official's own phone, native app | **Cloud sync** to atletiek.nu over the official's own mobile data | **Yes, explicit**: "syncs automatically with Atletiek.nu, but you can also continue working offline" | [Jury-app product page](https://www.atletiek.nu/home/competition-management/jury-app/), [Play Store](https://play.google.com/store/apps/details?id=nu.atletiek.juryapp&hl=nl) |
| **Roster Athletics Meet Mgmt app** (England Athletics et al.) | Official's own phone, native app | **Cloud sync** to the Roster platform | Not documented in this pass — TBD | [Play Store](https://play.google.com/store/apps/details?id=io.rosterathletics.admin&hl=en), [vendor](https://about.rosterathletics.com/en-US/competition-management) |
| **AthleticFIELD / AthleticLIVE** (US HS/college) | Either officials' own devices **or** meet-owned devices | **Both modes offered**: internet sync ("use the internet and allow people to use their own devices") or local-network sync ("use an internal network and purchase your own devices") | Local-sync mode exists precisely for poor-connectivity venues | [K2 Timing product page](https://results.k2timing.com/athletic-field), [Local Sync docs](https://support.athletic.net/article/2i0vsngaks-setup-local-sync) |
| **FieldLynx** (professional/college) | Meet-owned Windows devices | Dedicated venue network, wired or "AirLynx" wireless AP | n/a (the network is part of the paid timing installation) | [FinishLynx FieldLynx page](https://finishlynx.com/product/fieldlynx-event-software/) |
| **Swiss Timing OVR** (Diamond League/championships) | Professional crew, dedicated terminals | Dedicated cabled **on-venue results network**, results consolidated locally then distributed | n/a (engineered infrastructure, paid staff) | [Swiss Timing services](https://www.swisstiming.com/services/), [Athletics consulting datasheet](https://www-swt-com.web.swisstiming.com/fileadmin/Resources/Data/Datasheets/DOCM_AT_SalesConsulting_0818_EN.pdf) |

**Pattern.** The industry splits cleanly by who owns the network:

- **Professional tier** (Swiss Timing, FieldLynx installations): dedicated venue networks built
  by paid technicians — nobody expects volunteers to run these.
- **Volunteer/club tier** (Seltec Web.TEC 2, Atletiek.nu, Roster): **nobody builds field-wide
  venue Wi-Fi.** The converged pattern is *officials' own phones over their own mobile data,
  relayed through a cloud server*, with the meet-office system syncing to that same cloud.
  The best implementation (Atletiek.nu) adds explicit offline tolerance in the capture app;
  Seltec's (browser-based, no offline layer) is documented by the vendor as lossy under
  reconnection — a durability gap an OSS entrant can beat.
- **AthleticFIELD** is the only surveyed product offering both transports; its local-network
  mode exists for venues with meet-owned devices and poor internet.

**Implication for architecture (feeds ADR-002/ADR-004):** field capture via a cloud/hub relay
over officials' mobile data is not an exotic idea — it is the *established practice* at exactly
our target tier, including by Swiss Athletics' own incumbent. A venue-LAN-only design would be
the outlier. Conversely, an internet-reachable relay (our hub role) is a *prerequisite* for this
pattern: phones on LTE cannot reach a venue-LAN-only node.

## Sources

Primary/vendor/federation sources cited inline above; consolidated list:

- Seltec: [seltec-sports.com](https://www.seltec-sports.com/), [seltec-sports.com/offering](https://www.seltec-sports.com/offering/), [wiki.seltec-sports.net](https://wiki.seltec-sports.net/doku.php?id=start), [laportal.net](https://www.laportal.net/Competitions/Current), [lanet3.de](https://lanet3.de/Identity/Account/Login)
- Swiss Athletics: [swiss-athletics.ch TAF3/Seltec](https://swiss-athletics.ch/wettkaempfe/wettkampfsupport/taf3-/-seltec/), [Wettkampfverwaltung](https://swiss-athletics.ch/wettkaempfe/wettkampfsupport/wettkampfverwaltung/), [Wettkampfanmeldung](https://swiss-athletics.ch/wettkaempfe/veranstaltungen/wettkampfanmeldung/)
- German DLV/regional federations: [Bayerischer LA-Verband](https://blv-sport.de/service/seltec-software), [BW Leichtathletik](https://www.bwleichtathletik.de/home/wettkampf/wettkampfsoftware), [BLV-online](https://www.blv-online.de/wettkampf/wettkampfsoftware/)
- Netherlands: [Atletiek.nu](https://www.atletiek.nu/Atletiekunie.nl), [Atletiekunie uitslagenverwerking](https://www.atletiekunie.nl/kenniscentrum/wedstrijdorganisatie/baanwedstrijden/uitslagenverwerking/)
- Luxembourg: [FLA](https://www.fla.lu/competitions-cache-271213v4)
- World Athletics: [World Rankings basics](https://worldathletics.org/world-ranking-rules/basics), [World Rankings Competitions Procedures PDF](https://assets.aws.worldathletics.org/document/63c917632d4be3c6ee496553.pdf), [RFP – Results & Statistical Services](https://worldathletics.org/download/download?filename=add43f20-bf15-46b5-aa99-71c010077f50.pdf&urlslug=Request+for+proposal+-+results+and+statistics+service), [Global Calendar](https://globalcalendar.worldathletics.org/)
- W3C Open Athletics: [github.com/w3c/opentrack-cg](https://github.com/w3c/opentrack-cg), [Conceptual Model](https://w3c.github.io/opentrack-cg/spec/model/), [Competition Data Model](https://w3c.github.io/opentrack-cg/spec/competition/)
- HyTek: [hytek.active.com track-meet-management](https://hytek.active.com/track-meet-management.html)
- OpenTrack (commercial vendor): [opentrack.run](https://opentrack.run/), [Competition Management](https://opentrack.run/product/competition-management.html), [Open Data policy](https://opentrack.run/philosophy/opendata.html)
- FinishLynx/LynxPad/FieldLynx: [finishlynx.com](https://finishlynx.com/product/software/finishlynx-results-software/), [file formats](https://finishlynx.com/file-formats-meet-manager/), [Database Files manual](https://help.finishlynx.com/Content/OnlineManual/DatabaseFiles.htm), [LynxPad](https://finishlynx.com/product/event-management/lynxpad/)
- Athletic.net: [athletic.net](https://www.athletic.net/), [LIF scoreboard setup](https://support.athletic.net/article/00j157j60y-setup-a-live-scoreboard-with-finish-lynx-lifs)
- Roster Athletics: [about.rosterathletics.com](https://about.rosterathletics.com/en-US/competition-management), [England Athletics partnership](https://www.englandathletics.org/competitions-and-events/roster-athletics/), [timing integrations](https://support.rosterathletics.com/en/support/solutions/folders/44001195027)
- Athletics.app: [athletics.app](https://www.athletics.app/), [timing systems](https://www.athletics.app/home/competition-management/timing-systems/)
- MeetPro: [tfmeetpro.com](https://tfmeetpro.com/), [features](https://tfmeetpro.com/features.html)
- RaceTab: [MileSplit RaceTab overview](https://ia.milesplit.com/articles/60765/racetab-free-meet-management-software)
- ALGE-Timing: [alge-timing.com](https://alge-timing.com/en/sports/athletics-t&f/athletics)
- TimeTronics: [timetronics.be/athletics](https://www.timetronics.be/athletics)
- Swiss Timing/Omega: [swisstiming.com/sports/athletics](https://www.swisstiming.com/sports/athletics/)
- Adjacent OSS reference: [esitarski/CrossMgr](https://github.com/esitarski/CrossMgr/blob/master/FinishLynx.py)

## Open items for `docs/requirements/open-questions-and-assumptions.md`

- Confirm exact Alabus product scope/export format used by Swiss Athletics (page blocked to
  automated fetch by `robots.txt`; needs manual browsing or a direct federation inquiry).
- Confirm/deny whether "Tilastopaja" and "AthleTIC" (named in the original research brief) refer
  to real, relevant systems — neither was located under those names in this pass.
- Decide whether to pursue alignment with the W3C Open Athletics Community Group's draft data
  model as a design input for this project's own data model (C3), including possibly contributing
  upstream.
- Verify whether any incumbent has first-class para-athletics classification support — not
  confirmed either way in this pass; may be a genuine gap or simply undocumented by vendors.
