# Visual Identity Trends — Athletics and Sports Web Products

**Purpose:** evidence base for the Bahnfrei brand-identity decision deferred by **DEC-022**
(OQ-061: release 0.1 ships the neutral design-token set; brand is decided later as a values-only
change to `internal/web/static/tokens.css`). This survey identifies what visual conventions exist
in the market — the goal is trend awareness, **not** imitation. It recommends nothing; the brand
choice remains a founder decision.

**Method:** four segments surveyed in parallel on **2026-08-07** — governing bodies, direct
competitor meet-management/results platforms, elite meet/series sites, and broader sports digital
products. Colors were extracted from each site's own production sources (`theme-color` meta tags,
CSS custom properties, hex-frequency analysis of live stylesheets, published brand/style guides)
rather than eyeballed. Claims that could not be verified against a primary source are marked
**unverified**. Sites that could not be fetched are listed in §4.

---

## 1. Segment findings

### 1.1 Governing bodies

| Site | Primary | Secondary/accents | Background | Display type | Source |
|---|---|---|---|---|---|
| World Athletics | Orange `#ff873c` (`--primary-colour`) | Cyan `#69d7e1`, violet `#bd94ff`, live-red `#ed1c24`, WA+ green `#A5FA64` | Dark-leaning (`#0E0E0E`/`#262626` shells) | Custom condensed "World Athletics" + PP Formula | [worldathletics.org](https://worldathletics.org) + [compiled CSS](https://worldathletics.org/_next/static/css/2ed59739a2bc4d464d96.css) |
| European Athletics | Dark navy `#00053e` (shell only; CSS bundle blocked — rest unverified) | — | Dark shell | — | [european-athletics.com](https://european-athletics.com) |
| Swiss Athletics | Red `#E30613` (`--primary-color`) | Dark reds `#C21515`/`#8A0401`; navy-slate `#233D4D` | Light (`#F2F2F2` bg, grey neutrals) | Neue Haas Grotesk Display | [swiss-athletics.ch](https://swiss-athletics.ch) + main.css |
| DLV (Germany) | Red `#ee3124` (tile-color meta) | Black-red-gold per 2023 rebrand (gold hex unverified) | Unverified (CSS blocked) | Unverified | [leichtathletik.de](https://www.leichtathletik.de); [rebrand article](https://www.designtagebuch.de/deutscher-leichtathletik-verband-dlv-startet-mit-neuem-visuellen-erscheinungsbild/) |
| FFA (France) | Dark navy `#0d2366` | Mid-blues `#294088`/`#435ba4`, coral `#ff4d52`, green `#4cab4c` | Light (grey surfaces) | Arquitecta/Outfit + Lato | [athle.fr](https://www.athle.fr) + [styles.css](https://www.athle.fr/css/styles.css?v=10) |
| UK Athletics | Red `#e30613` | Aqua `#36bbd5`, navy `#222c53` | Light | Korolev Condensed + FedraSans | [uka.org.uk](https://www.uka.org.uk) + theme main.css |

Takeaways: national federations converge on nearly identical **flag red** (`#E30613` /
`#ee3124` / `#e30613` — Swiss, German, British, three unrelated organizations in one hex band);
**deep navy** recurs as the institutional secondary everywhere; World Athletics deliberately
breaks the red convention (orange on a dark, multi-accent, heavily tokenized system) to
differentiate the global body; warm-primary + cool-secondary pairing recurs (orange+cyan,
red+navy, red+aqua); condensed display type over a plain body sans is near-universal.

### 1.2 Meet-management / results platforms (direct competitive set)

| Site | Primary | Status/data colors | Background | Type | Polish | Source |
|---|---|---|---|---|---|---|
| Seltec LA.portal | Yellow `#ffcc00` (theme-color) | Round chips: grey `#a9a9a9` → lightsalmon (live) → lightgreen (official) | Light, striped tables | System sans | Dated-utilitarian | [laportal.net](https://www.laportal.net/Competitions/Current) |
| DLV Ergebnisse (Seltec) | Tan-gold `#c3b487` skin; DLV red `#ee3124` frames it | 5-state chips: grey `#a9a9a9`, live `#FF0033`, protest `#FF1493`, startlist-official `#FFE600`, finished/official `#69E6B4` | Light; gender-tinted rows | Self-hosted Roboto | Dated-utilitarian (same codebase as LA.portal) | [ergebnisse.leichtathletik.de](https://ergebnisse.leichtathletik.de) + dlvyellowlayout.css |
| Seltec corporate | Amber `#FFBE00` + mint-teal `#5eead4` | — | Dark marketing site | Kelson + Open Sans | Modern (unlike its product) | [seltec-sports.com](https://www.seltec-sports.com) |
| OpenTrack | Track Red `#C63527`, Track Black `#101820`, Track Blue `#0274BD`, Light Steel `#C7D4DD` | — | Light | Proxima Soft (Nunito open fallback) | **High-water mark** — published style guide | [opentrack.run/style-guide.html](https://opentrack.run/style-guide.html) |
| Roster Athletics | White theme-color; tile blue `#2b5797`; rest unverified (SPA) | — | — | Modern PWA shell | Modern (partial data) | [rosterathletics.com](https://www.rosterathletics.com) |
| Athletic.net | Yellow `#ffd843`; maroon hero `#770401` | Category color-coding; stock Bootstrap blue `#0d6efd` leaks through | Light | System stack | Hybrid | [athletic.net](https://www.athletic.net) + compiled CSS |
| RaceResult | Dark red `#a81815`/`#c41011` | — | Light | URW DIN (licensed) | Modern | [raceresult.com](https://www.raceresult.com) |
| Atletiek.nu | **Blocked** (bot interstitial) — no claims | — | — | — | — | — |

Takeaways: **yellow/gold is the de facto color of the incumbent results layer** (Seltec's two
deployments + Athletic.net); red anchors the corporate/timing-industry side (OpenTrack,
RaceResult, Seltec-corp, Athletic.net hero). **Every inspectable results grid is
light-background** — no dark-mode results surface exists anywhere in the competitive set. Live
state is signaled by **discrete status chips in a grey → orange/red → green traffic-light
progression** (Seltec adds yellow for startlist-official and pink for protest), never by row
shading. The design bar is low: the median is dated ASP.NET/jQuery chrome; only OpenTrack has an
intentional, documented visual system, and typography investment (OpenTrack, RaceResult) is what
visibly separates modern entrants from the legacy median.

### 1.3 Elite meets & series

| Site | Primary | Background | Display type | Source |
|---|---|---|---|---|
| Wanda Diamond League | Navy-indigo `#0e003c` (`--accent-color`), blue `#005bac` | Light chrome + black gradient overlays on photos | Prometo | [compiled theme CSS](https://www.diamondleague.com/wp-content/themes/diamondleague/public/css/app.7ebf7b.css) |
| Weltklasse Zürich | Indigo navy `#141B4D` (theme-color) | Light components, dark tint | Neue Haas Unica | [weltklassezuerich.ch/de/](https://www.weltklassezuerich.ch/de/) |
| Athletissima Lausanne | Reds `#F10021`/`#DC0B28` + golds `#E6B036`/`#FFBC7D` | Light with saturated accent blocks | CargoD + Roboto | [Elementor CSS](https://athletissima.ch/wp-content/uploads/elementor/css/post-28834.css) |
| BMW Berlin Marathon | BMW blue `#0066b1` | Light + diagonal dark photo overlays | Spezia | [compiled TYPO3 CSS](https://www.bmw-berlin-marathon.com) |
| Memorial Van Damme | Mid blue `#0061AE` | Light, corporate | Lato | [child-theme.css](https://memorialvandamme.be/wp-content/themes/golazo-theme-memorialvandamme/css/child-theme.css) |
| Bislett Games / Golden Spike | **Unreachable** (fetch failure / bot challenge) | — | — | — |

Takeaways: **blue/navy is the broadcast register** — four of five reachable sites anchor on it;
Athletissima is the lone red/gold "festival" exception. Dark is a **photo-treatment device**
(black gradient overlays on heroes), not page chrome — the canvas underneath stays light. Every
top-tier meet buys a bespoke display face for headlines while body copy stays workhorse;
"energy" is produced by photography, sliders, and countdowns, not by palette.

### 1.4 Broader sports products

| Product | Primary | Live-data conventions | Background | Type | Source |
|---|---|---|---|---|---|
| Strava | Orange `#fc5200` (28× in prod CSS; legacy `#FC4C02` no longer present) | — | Light, **warm** cream neutrals | Proprietary "Boathouse" | [prod CSS](https://web-assets.strava.com/assets/landing-pages/_next/static/css/05496e7e9ab8f708.css) |
| Zwift | Orange `#fb6418` (106×) + blue family `#0093d1` | Green/red/yellow semantic tokens | Light + near-black bands; no dark mode | ZwiftChrono/Fondo/Sprint (bespoke, incl. condensed timer face) | [prod CSS](https://content-cdn.zwift.com/zwift-web-core/2.246.1/_next/static/css/styles.97699eb2.chunk.css) |
| ESPN | Muted red `#d00` on dominant grayscale (cited `#E4002B` unverified) | Gray canvas + sparse red/blue accents | **Both** — real dark mode (`#101113`) | BentonSans condensed | [page.css](https://a.espncdn.com/redesign/0.784.1/css/page.css) |
| UEFA | Deep navy `#001b9a` | Tokenized (`--pk-*`) with light/dark text variants | Light default, theming infra present | Champions ExtraBold | [base.css](https://www.uefa.com/CompiledAssets/UefaCom/css/competitions/corporate/base.css) |
| Sofascore | Blue-violet `#374DF5`; theme-colors `#2c3ec4`/`#2a3543` | Green `#0BB32A` = live/win, red `#E73B3B` = loss/card; gradients only for probability/heatmaps | **Both** — dark-mode-first token set | Sofascore Sans + Condensed (for score grids) | [prod CSS](https://www.sofascore.com/_next/static/css/8ba693c92ddaf941.css) |
| Flash Results | No extractable palette — 2010s static template | Static per-meet HTML tables | — | Yanone Kaffeesatz + script | [flashresults.com](https://www.flashresults.com) |
| Race Roster | No distinct brand CSS found (WordPress defaults only) — inconclusive | — | — | System stack | [raceroster.com](https://raceroster.com) |

Takeaways: the dominant formula is **one hot accent on a large disciplined neutral field**.
Hue clusters by audience: **orange = athlete-facing/motivational** (Strava, Zwift — and World
Athletics), **blue = institutional/live-data** (UEFA, Sofascore, ESPN's interactive blue).
**Dark mode correlates with "checked repeatedly for live numbers"** (ESPN, Sofascore), not with
marketing sites. Green/red live-delta coloring is universal where observable. Mature products
expose named token systems and commission condensed numeric/tabular type; the utilitarian ones
expose no brand system at all — systemization, not hue choice, is what reads as quality.

---

## 2. Cross-segment trends

- **T1 — Hue semantics are established and consistent.** Flag red = national federation
  identity; navy/blue = institutional trust and broadcast; yellow/gold = the incumbent
  timing/results layer (Seltec ecosystem, Athletic.net); orange = athlete energy (Strava, Zwift,
  World Athletics). A palette choice therefore carries an association whether intended or not.
- **T2 — One saturated accent over disciplined neutrals is the universal formula** in every
  polished product surveyed (Strava, Zwift, ESPN, Sofascore, OpenTrack, RaceResult, Diamond
  League). Multi-accent palettes appear only in dated products or deliberate festival branding
  (Athletissima).
- **T3 — Data surfaces are light.** Every inspectable athletics results grid is
  light-background; dark appears as marketing-hero treatment or as an explicit dark *mode* in
  mature live-score products outside athletics (ESPN, Sofascore). A dark-capable token set is a
  differentiator no athletics competitor currently has, but light must remain the default
  register (and matches SYS-113 outdoor-legibility needs).
- **T4 — Round/result status has a de-facto market convention:** discrete chips in a grey
  (not started) → orange/red (live) → green (official) traffic-light progression, optionally
  yellow (startlist official) and pink (protest) — exactly the Seltec scheme Swiss operators
  already read fluently. Bahnfrei's existing semantic tokens (`--color-success/warning/info`,
  used for SYS-087 offline states and the SYS-076 unofficial label) are directionally
  compatible; status must also never be color-alone (SYS-148, WCAG).
- **T5 — Typography is the visible maturity signal.** Condensed/bespoke display or numeric
  faces (often specifically for dense score tables: Sofascore Sans Condensed, ZwiftChrono,
  BentonSans) over a neutral body sans; system-font-only stacks read as the dated median.
  Constraint: ADR-002/ADR-003 forbid web-font fetches, so any Bahnfrei type upgrade must be a
  self-hosted open-source face (cf. OpenTrack documenting open-source Nunito as its fallback).
- **T6 — Token-based theming is the "real design system" proxy** (World Athletics, UEFA,
  Sofascore expose it; legacy products expose nothing). Bahnfrei's `tokens.css` + CI conformance
  check already matches the mature pattern; branding is genuinely a values-only edit (DEC-022).
- **T7 — Neutral undertone carries brand temperature:** warm creams for athlete-facing warmth
  (Strava) vs. cool blue-greys for data-console authority (ESPN, Sofascore) — a free variable
  independent of the accent hue.
- **T8 — The competitive design bar is low.** The athletics results market's median is dated
  utilitarian chrome; OpenTrack alone has a documented visual system. A coherent tokenized
  identity with considered type would place Bahnfrei at or above the segment's high-water mark
  without any exotic design investment.

## 3. Implications for a future Bahnfrei identity (options, not decisions)

- **Crowded hue space:** flag red risks reading as (or falsely implying affiliation with) Swiss
  Athletics/DLV/UKA; yellow/gold reads as the Seltec incumbent; generic mid-blue blends into the
  elite-meet/institutional mass. Orange collides with World Athletics and Strava/Zwift but at
  least signals "athlete energy" rather than "authority".
- **Open hue space:** green, teal, and violet families are essentially unclaimed across all four
  segments (greens appear only as "official/success" status, which a green *brand* accent would
  conflict with — that tension would need resolving; teal/violet carry no such conflict).
- **Conventions worth adopting regardless of hue:** single-accent formula (T2); light data
  surfaces (T3); the grey→live→green status-chip grammar operators already know (T4); a
  self-hostable condensed face for results tables (T5); warm-vs-cool neutral undertone chosen
  deliberately (T7).
- All of this lands as token values in `tokens.css` per DEC-022; no template or code change.

## 4. Coverage gaps

- **Atletiek.nu** (bot interstitial), **Bislett Games / Golden Spike Ostrava** (fetch
  failure / bot challenge): no claims made.
- **European Athletics**: shell color only; main CSS bundle blocked (Cloudflare).
- **Roster Athletics**: meta-tag colors only; compiled SPA palette not retrievable without a JS
  renderer.
- Results-app palettes of OpenTrack (`data.opentrack.run`) and Roster/Athletic.net in-app grids
  were not inspected — only their public/marketing surfaces.
