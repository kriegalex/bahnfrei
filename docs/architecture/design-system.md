# Design System

**Status:** TASK-032 deliverable (STR-045 → SYS-116 → UC-038 #1–#2). Home: architecture rather
than requirements, because this document describes *what was built* (the token set, the
component inventory, the CI mechanism) — the requirement itself lives in `system-requirements.md`
(SYS-116), the acceptance criteria in `use-cases.md` (UC-038), and the traceability row in
`traceability-matrix.md`. Sits alongside `architecture.md` §3 (`web/` package) and §4
(cross-cutting concerns), which this document extends with a design-layer baseline the same way
`adr/ADR-003-technology-stack.md` set the SSR/HTMX/templ baseline it styles.

**Brand identity (DEC-036, TASK-056):** Bahnfrei's visual identity is ratified — a teal accent on
warm cream/warm near-black neutrals, plus a self-hosted condensed display face for headings and
result-grid numerals (`docs/research/visual-identity-trends.md` is the evidence base; the register
entry is `open-questions-and-assumptions.md` §11). Because every stylesheet and template reads
colors/spacing/typography/radius exclusively through the named custom properties in
`internal/web/static/tokens.css`, the retint itself was a values-only edit to that one file — no
template or component markup changed for it. The display-face addition needed one more layer:
`base.css` grew `@font-face` rules (pointing at the vendored `barlow-semi-condensed-*.woff2` files,
embedded in `static.go` like every other asset) and the selectors that *consume*
`--font-family-display` (headings, `.results-table`); `layout.templ`'s header link grew a `.brand`
class for the wordmark treatment. `internal/web/tokens_contrast_test.go` parses `tokens.css`
directly and asserts every foreground/background pair actually in use meets WCAG AA, so a future
retint cannot silently regress contrast. No designed logo exists yet — DEC-036 explicitly scoped
that out; a future logo pass would follow the same values-only-plus-thin-consumption-layer shape.

**Component chrome and form layout (DEC-037, TASK-057):** the brand retint alone left every
interactive component rendering as an unstyled browser default — no button/input chrome, no link
color, a bare-flex-row header nav, and forms that laid label/input pairs out as one wrapping inline
row. This pass is a second, thin layer on top of the same token system: two new tokens
(`--color-accent-hover`, `--color-surface`, §1), three documented button styles and a token-driven
input/select/textarea chrome (§2), a real header-nav treatment with an `aria-current="page"`
indicator, and a stacked `.field`/`.form-grid`/`.form-panel` form-layout convention applied to the
forms most affected (login, first-run setup, change-password, admin account creation, the
create/edit meet form, seeding generate/advance). Dense point-of-competition capture grids
(`.cell-form`, `.stack-table`, marker buttons) are explicitly excluded from the layout convention —
they inherit only the new chrome (border/radius/background), never the stacked geometry — so their
compact row ergonomics and 44 px touch floors are unchanged.

**Structural pass (DEC-038, TASK-058):** chrome alone left the shell's information architecture,
action hierarchy and status vocabulary unaddressed — a founder live-instance walkthrough named
prose paragraphs of middot-separated links, undifferentiated page actions, bold-text status and a
raw multi-select listbox. This pass, on the same token system, adds no new colors: (1) the meet
hub, the results-capture index and the public meet pages replace their middot-separated
navigation paragraphs with `.action-panel`/`.action-list` cards on `--color-surface` — bounded,
line-separated entries, navigation kept apart from page actions (§2); (2) a `.button-row` groups a
page's primary/secondary actions (`.button-primary` for the consequential one — creating a meet,
publishing — `.button-secondary` for the reversible alternate, e.g. archiving, never danger); (3) a
`.status-chip` family generalizes the offline-indicator's colored-pill pattern to any lifecycle
state — meet status today (draft/live/published/closed/archived) — always paired with its text
label (SYS-148: never color-alone); (4) the discipline-add form's category `<select multiple>`
becomes a `.checkbox-grid` inside a `<fieldset>`, same field name and submit semantics, no schema
change; (5) the DEC-037 form-layout sweep (`.field`/`.form-grid`/`.form-row`) is completed over the
remaining office forms (entries individual/bulk/relay, timing import, entries-import), closing
OQ-153 — the reconciliation and officials-assignment pages turned out to have no labeled-field
forms to sweep (only button-only apply/discard/assign actions embedded in a table, structurally
the same dense-row shape the capture grids are already excluded for), and each per-row bulk-entry
table and the relay-composition edit sub-form stay untouched for the same reason table/capture
grids do — see §2's Form layout row for the full list of what moved and what deliberately did not.

## 1. Design tokens

Defined in `internal/web/static/tokens.css` (loaded before `base.css` in every page, see
`layout.templ`), as CSS custom properties on `:root`. This is the *only* file allowed to declare a
raw color/typography/spacing/radius literal — `scripts/check-style-tokens.sh` (§3 below) enforces
that mechanically in CI.

| Category | Tokens | Notes |
|---|---|---|
| Color — surface/text | `--color-bg`, `--color-fg`, `--color-border`, `--color-surface` | Base page colors; `--color-border` is a low-opacity neutral (rgba), used for hairline dividers (header nav underline) and the token-driven input/button border. `--color-surface` (DEC-037, TASK-057) is a near-white band barely lighter than `--color-bg` (1.06:1 between the two) that form panels/cards and the secondary-button background sit on, so controls read against a defined plane rather than floating on the cream canvas. |
| Color — accent | `--color-accent`, `--color-accent-fg`, `--color-accent-hover` | Links, the focus ring, and primary-action styling. `--color-accent-hover` (DEC-037, TASK-057) is one step darker on the same teal scale — the hover background for the primary button and the hover text color for links/the brand wordmark — a real color-token swap, not the opacity fade the shell used before this pass. |
| Color — semantic status | `--color-success-bg`/`-fg`, `--color-warning-bg`/`-fg`, `--color-warning-strong-bg`/`-fg`, `--color-info-bg`/`-fg`, `--color-danger-fg` | Back the offline-capture indicator's three states (SYS-087), the unofficial-results label (SYS-076), the office blip-tolerance banner (UC-034 #7), and inline/flash error text. |
| Typography | `--font-family-base`, `--font-family-display`, `--font-size-base`, `--font-size-sm`, `--font-size-lg`, `--line-height-base`, `--font-weight-bold` | `--font-family-base` is the system-ui stack (body copy, no web-font fetch — ADR-002/ADR-003 offline-first). `--font-family-display` (DEC-036, TASK-056) is Barlow Semi Condensed, a self-hosted OFL latin-subset woff2 (`internal/web/static/barlow-semi-condensed-{regular,bold}.woff2` + `-LICENSE`, embedded in `static.go`, no runtime fetch either) — applied to headings and `.results-table` numerals (`font-variant-numeric: tabular-nums`, using the face's verified `tnum` OpenType feature). Three sizes (body/hint/wordmark), one weight beyond regular. |
| Spacing | `--space-1` … `--space-6` (0.25rem–1.5rem, matching the values already in use across the shell) | A small scale, not a full ramp — extend it only when a new value is genuinely needed, not per-component. |
| Radius | `--radius-sm` (0.4rem) | Badges and banners; nothing in this shell uses a second radius yet. |
| Focus ring | `--focus-ring-color`, `--focus-ring-width`, `--focus-ring-offset` | Kept separate from the raw accent color/spacing tokens so SYS-114's keyboard-focus contract has one named, testable seam. |
| Touch target | `--touch-target-min` (2.75rem/44px) | SYS-147/UC-039 #2: the minimum hit-target size for a primary point-of-competition control operated by touch, outdoors, under time pressure — the Apple HIG (44x44 pt)/Material Design (48x48 dp) floor, above WCAG 2.2 SC 2.5.8's 24x24 px minimum. |
| Chrome control floor | `--control-min-height-sm` (1.5rem/24px) | SYS-116 (TASK-051): every `<select>` gets at least this height — WCAG 2.2 SC 2.5.8's own floor, named explicitly rather than left to a select's intrinsic height. Distinct from `--touch-target-min`, the comfort bar for small chrome controls rather than already-larger primary capture controls. |

`color-scheme: light dark` is declared (browser chrome — scrollbars, form-control native
rendering — adapts to the OS), but the token *values* stay light-only: STR-045/SYS-116 require a
coherent, documented system, not a dark theme, no page ships dark-mode colors to pair with it, and
DEC-036 explicitly keeps light the default register (SYS-113 outdoor legibility). A dark palette
would be a values-only addition to `tokens.css` when wanted — the same mechanism the DEC-036 retint
itself used — recorded as an assumption, not an open question (see
`open-questions-and-assumptions.md` A-### working-assumptions section for the pointer).

## 2. Component inventory

Every component below is authored once in `internal/web/static/base.css` and consumes tokens only.
The **states** column names what SYS-116 requires per component (default, hover, focus-visible,
active, disabled, error where applicable); "n/a" marks a state the component structurally cannot
have. The gallery fixture page (§4) renders one live instance of every row.

| Component | Selector(s) | States | Used on |
|---|---|---|---|
| Skip link | `.skip-link` | default (visually hidden), focus-visible (visible, positioned) | Every page (`layout.templ`) |
| Header nav | `header nav`, `.nav-links` (section links, left), `.nav-actions` (auth state + locale switcher, right, pushed there by `margin-left: auto`) | default | DEC-037, TASK-057: one modest bar, no mega-menu — wordmark, then the section links, then the logged-in/login state and locale switcher grouped right. Every page (`layout.templ`) |
| Nav section link | `.nav-link` | default (no underline), hover (accent-hover + underline), current (`aria-current="page"`: accent color + bold weight + bottom border — never color-alone, SYS-148), focus-visible (ring) | DEC-037, TASK-057: the Meets/Audit/Accounts/Login header links. `PageData.CurrentPath` (set once in `basePageData` from the request path) backs `PageData.NavCurrent(prefix)`, a prefix match so a section's own sub-pages (e.g. `/meets/{id}`) still mark their nav item current. `layout.templ` |
| Header wordmark | `.brand` | default, hover (accent-hover, inherited from the generic `a:hover` rule), focus-visible (ring) | DEC-036/DEC-037, TASK-056/057: the header's home link ("Bahnfrei" — a proper noun, not localized), set in `--font-family-display` at `--font-size-lg`, colored `--color-accent`. A typographic treatment, not a designed logo (explicitly out of scope). Every page (`layout.templ`) |
| Button | `button`/`.button`/`.button-secondary` (default — a bare `<button>` and an `<a class="button">` render this look with no class needed), `.button-primary`, `.button-danger` | default, hover (color-token swap: `--color-accent-hover` for secondary/primary background, escalating from `--color-warning-strong-bg` to `--color-danger-fg` for danger), focus-visible (ring), active (`filter: brightness(0.92)`, distinct from hover), disabled (opacity + `cursor:not-allowed`) | DEC-037, TASK-057: three token-driven styles — primary (solid `--color-accent`) is a form's main submit; secondary (outlined neutral on `--color-surface`, also the unclassed default) is cancel/secondary actions and the pre-existing "`<a class="button">` as a GET-confirm-page link" convention; danger (`--color-warning-strong-bg`/`-fg` by default, escalating toward `--color-danger-fg` on hover) is reserved for a destructive confirm page's actual commit (`confirmView.Danger`, `confirm.go`/`.templ` — athlete erasure, retention purge, bulk-DNS). Forms across the app (login, meets, entries, capture, admin, …) |
| Link | `a` | default (`--color-accent`, underlined — the in-prose convention), hover (`--color-accent-hover`), focus-visible (ring), disabled via `aria-disabled="true"` (n/a today — no disabled link exists yet, documented for completeness) | DEC-037, TASK-057: nav/table-of-controls links (`.nav-link`, `.button`, `.brand`) opt out of the underline in their own, more specific rules and carry a different non-color cue instead (see their rows). Visited stays the same color (app links, not documents). Nav, in-page references |
| Structured action panel/list | `.action-panel` (bounded card on `--color-surface`), `.action-list` (its vertical, line-separated `<ul>`), `.action-list-secondary` (a muted secondary link within an entry), `.action-list-nav` (a lightweight unboxed wayfinding row), `.button-row` (a page's primary/secondary action buttons side by side) | default only | DEC-038, TASK-058: replaces middot-separated prose-link paragraphs with a bounded panel per task area, navigation kept apart from consequential actions — the meet hub (three panels: preparation/competition-day/publication, each a `<nav aria-label>`), the results-capture index (the unit list, each entry's capture link paired with its PDF-download secondary action via `.action-list-secondary`; the "back to meet"/reconciliation wayfinding row stays the lighter `.action-list-nav`) and the public meet pages (`publicNav`, reused by every `/m/{id}/...` page). `.button-row` carries the meets-list "Neuen Wettkampf anlegen" (`.button-primary`)/"Aus Vorlage anlegen" (`.button-secondary`) pair and the meet hub's "Publizieren" (`.button-primary`)/"Archivieren" (`.button-secondary`, reversible — never danger, matching `confirmView`'s existing non-`Danger` classification) pair. Ordinary navigation inside an action list stays a plain link — not every entry is buttonized. |
| Text/number/date input | `input[type=text\|password\|number\|date\|url\|search]`, `select`, `textarea` | default (`--color-border` border, `--radius-sm`, `--color-surface` background — DEC-037, TASK-057), focus-visible (ring), disabled (opacity), invalid/error (`aria-invalid="true"` → red border, paired `.field-error` text) | Every form. Checkbox/radio/file/hidden inputs keep native rendering — excluded from this chrome, which would otherwise distort them. |
| Checkbox | `input[type=checkbox]` | default, focus-visible (ring), disabled | Consent/withdrawal flags, bulk-entry rows |
| Select | `select` | default, focus-visible (ring), disabled | Locale switcher, template/round pickers, bulk rows |
| Fieldset/legend | `fieldset`/`legend` | default (browser-native grouping box) | Grouped inputs (e.g. relay leg composition) |
| Form layout | `.field` (one label+input block, plus its paired hint/error/help-icon), `.form-grid` (a form's fields, consistent spacing), `.form-row` (fields laid out side by side within a `.form-grid`), `.form-panel` (bounded, lightly bordered, centered card on `--color-surface`) | default only | DEC-037, TASK-057: a stacked label-above-input convention so a label never wraps apart from its input; `.field` wraps the existing `<label>…<input/></label>`, preserving implicit label association. DEC-038, TASK-058 completed the sweep over the remaining office forms (closing OQ-153). Deliberately excluded: dense multi-row data-entry tables (bulk club entry, relay-composition edit), genuinely one-row search forms, and button-only pages. |
| Checkbox grid | `.checkbox-grid` (wraps a `fieldset`'s checkboxes) | default, focus-visible (ring, inherited from the generic checkbox rule), disabled | DEC-038, TASK-058: replaces the discipline-add form's `<select multiple>` category listbox with one checkbox per category, wrapping compactly. The checkbox group submits under the same field name as the listbox did, so handler validation — including rejecting an empty selection (SYS-002) — is unchanged. |
| Hint text | `.hint` | default (muted, smaller) | Any field needing a visible constraint (SYS-117) — adopted for hard format/unit/bound constraints (TASK-031); residual gaps tracked as OQ-076 |
| Contextual-help icon + popup | `.help` wrapper: `.help-trigger` (button) + `.help-popup` (`role="tooltip"`) | trigger: default, hover, focus-visible (ring), active, `aria-expanded` open/close; popup: hidden (default), open | TASK-031 (SYS-115, UC-037): every input in the help registry; hover + focus + click/tap open, meeting WCAG 2.2 SC 1.4.13 (dismissible, hoverable, persistent) |
| Inline field error | `.field-error` + `input[aria-invalid="true"]`/`select[aria-invalid="true"]` | error only (n/a default/hover/active) | Adopted (TASK-034, OQ-075, UC-038 #4) on the three representative forms: meet setup, online entry, result correction; further forms extend the same reusable mechanism |
| Table + wide-table scroll wrapper | `table`, `.table-scroll` (`tabindex="0"`) | default, focus-visible (ring, on the scroll wrapper) | Timetables, start lists, results, standings, roster, audit log |
| Result grid | `.results-table` (paired with a plain `table`) | default only | DEC-036, TASK-056: the two rank/bib/mark/points/total tables (`standingsPage` in `standings.templ`, `publicResultsFragment` in `public.templ`) render in `--font-family-display` with `font-variant-numeric: tabular-nums` so digit columns align — the face's `tnum` OpenType feature was verified (fonttools) before vendoring. Plain data tables (roster, start lists, audit log, timetables) are unaffected — they keep the system stack. |
| Offline-capture status badge | `.offline-status[data-state]` | online/default, offline, syncing (three distinct colors) | Field capture (UC-034; SYS-087) |
| Status chip | `.status-chip[data-tone]` — `data-tone` absent/`"neutral"` (the unqualified default), `"info"`, `"success"`, `"muted"` | one look per tone (no interactive states — a text badge, not a control) | DEC-038, TASK-058: generalizes the offline-status "colored pill" pattern to any lifecycle/publication state, built entirely from the existing semantic tokens (no new color). Meet status (`draft`/`live`/`published`/`closed`/`archived`, `statusTone` in `meets.go`) maps to neutral/info/success/success/muted — the market's grey→live→green register — on the meets-list Status column, the meet hub header and the public meet page header. The chip's text is always the real localized `meet.status.*` label; `data-tone` only selects the color (SYS-148: never color-alone). |
| Office blip-tolerance banner | `.offline-banner` | hidden (default) / visible+alert | Authenticated shell, all pages, while a connectivity blip is active (UC-034 #7) |
| Unofficial-results label | `.unofficial-label` | visible only when results are unofficial | Public results page (SYS-076) |
| Flash/alert message | `.error`, `.flash-error`, `role="alert"` | error only | Page-level validation/flow errors across the app |
| Destructive-action confirm page | `confirmPage`/`confirmView` (`internal/web/confirm.go`/`.templ`) — composed entirely of existing components (`.error` warning paragraph, plain text input, `.field-error`, `button`, `a`), no new CSS | n/a (a page composition, not a styled widget) | TASK-034 (OQ-074, UC-038 #3): the shared GET-confirm-sub-page pattern fronting the destructive actions (athlete erasure, account disable, meet archive, retention purge) — a real navigation rather than a CSP-incompatible dialog. Erasure and retention purge add a typed-confirmation input as defense-in-depth (ASVS). TASK-053 (DEC-030) reuses the page for account password reset with an optional password field. |
| Row-card responsive table | `.stack-table` (paired with `.capture-grid` or a plain `<table>`) | default (native table, ≥481px), row-card (≤480px, one card per row via `display:block` + `data-label`-prefixed cells) | TASK-045 (SYS-147, UC-039 #1): every point-of-competition table whose column count fits a one-column card without hiding in-progress state. Overriding `display` requires re-asserting the table/row/cell ARIA roles explicitly (Adrian Roselli). **Not** used for the vertical-jump grid (below). DEC-037, TASK-057: inherits the shared control chrome but keeps its own compact padding — row geometry and the ≥44px touch floor are unchanged. |
| Sticky first column | `.vertical-grid-sticky` (wraps `.table-scroll`) | default (native table, ≥481px), sticky (≤480px: bib/name columns pinned via `position: sticky`, height columns scroll within the wrapper) | TASK-045 (SYS-147, UC-039 #1): the vertical-jump height × trial matrix is inherently two-dimensional and does not linearize into a row-card without hiding in-progress state. The page itself never gains horizontal scroll; reaching a later height column scrolls this inner region — a deliberate exception to the row-card pattern above. |
| Compact capture-page chrome | `.capture-context` (breadcrumb line) + `.capture-title` (`<h1>`) | default only | TASK-045 (SYS-147, UC-039 #2): replaces the former single-line page heading with a small breadcrumb above a short, single-word heading, so the first capture row sits within the first 640px of page height at 360px width. |
| Letter-marker quick actions | `.marker-actions` (wrapper) + `.marker-btn` (`markerButton` in `capture.templ`) | hidden (≥481px), visible with focus-visible/hover/active (inherited from the shared `button` rules), enlarged to `--touch-target-min` at ≤480px | TASK-045 (SYS-147, UC-039 #3): the mark/trial inputs restrict the virtual keyboard to numeric or none, so these buttons are the documented alternate path for the non-numeric D5.2 markers (X foul, – pass, r retirement; o clear on the vertical grid). TASK-052 (OQ-133): hidden above the phone breakpoint, where a physical keyboard types the markers directly. |
| Per-cell save-state badge | `.cell-save-badge`, styling the existing `.cell-form[data-pending]`/`[data-state]` attributes (`islands/src/capture-offline.ts`) | pending (left border + badge text, `--color-info-fg`), confirmed (`--color-success-fg`), rejected (existing `.cell-reject`, TASK-044) | TASK-045 (SYS-148, UC-039 #4/#5): a badge distinguishable by more than color per WCAG 2.2, reusing the same state-attribute convention TASK-044 established. Once a save is acknowledged, the row's own result cells update in place from the sync ack's authoritative values (field-horizontal grid — the only capture family with an async/offline save path today), closing a prior below-the-fold-only update gap (usability-audit F3). |

## 3. CI style-conformance check (SYS-116, UC-038 #1)

`scripts/check-style-tokens.sh`, wired as the `style-conformance` job in
`.github/workflows/ci.yml` (alongside the other `scripts/check-*.sh` gates — coverage, license
headers). It scans every tracked `.css`/`.templ`/`.go` file (excluding generated `*_templ.go`) for
two bypass shapes:

1. A raw hex color (`#abc`, `#aabbcc`, `#aabbccdd`) or an `rgb()`/`rgba()` function outside
   `internal/web/static/tokens.css`.
2. An inline `style="…"` attribute in a `.templ` template (a total bypass regardless of its
   value).

**Documented allowlist** (UC-038 #1's "known limits" clause): `tokens.css` itself (where literals
are *defined*), the two vendored htmx assets (`static/htmx.min.js`, `static/htmx-LICENSE`) —
third-party, not project-authored style — and `internal/web/tokens_contrast_test.go` (TASK-056,
DEC-036): a WCAG contrast-ratio checker whose CSS color-syntax *parsing* code (string-prefix
matching, error messages) contains the literal substrings `rgb(`/`rgba(` without ever declaring a
color value itself — every literal it checks is read from `tokens.css`. This is a mechanical,
pragmatic check, not a CSS parser:
it cannot catch every possible bypass shape (e.g. a bare unitless number introduced as a new
`font-size` in some future file). New bypass shapes found later get added to the script or its
allowlist; the release usability-audit checklist (`docs/requirements/usability-audit-checklist.md`)
is the non-mechanical backstop SYS-116 itself names for what this check structurally cannot catch.

## 4. Component-gallery fixture page (UC-038 #2)

`GET /dev/design-gallery` (`internal/web/gallery.templ`, `handleDesignGallery` in `routes.go`) — a
dev/fixture-scope page, deliberately **not** linked from the shell nav (`layout.templ` never
references it; `TestDesignGalleryNotLinkedFromShellNavSYS116UC038_2` pins that). It is reachable
unauthenticated like `/healthz`: it renders pure static markup with no meet data, no PII and no
privileged action, so there is no reason to gate it behind a session.

It renders one instance of every row in §2's inventory, in every state that can be expressed as
static markup (default, disabled, invalid/error, all three offline-status colors). Two automated
consumers close UC-038 #2's loop:

- `internal/web/gallery_test.go` (`TestDesignGalleryRendersEveryInventoriedComponentSYS116UC038_2`):
  a Go HTTP test asserting every component id is present in the rendered page.
- `e2e/tests/design-gallery.spec.ts`: a real-browser Playwright keyboard walk — `Tab` through the
  page, and at every element `document.activeElement` newly lands on, assert a real, non-`none`,
  positive-width `outline` (the token-driven focus ring, applied via `:focus-visible` so mouse
  clicks don't show it needlessly) is actually rendered. A second test asserts the three
  offline-status colors are visually distinct and that the invalid-input/field-error pairing is
  both present and associated via `aria-describedby`.

  **A note for whoever touches this test next:** `input[type=date]` (and similarly-segmented
  native controls, e.g. `type=time`) exposes multiple internally-focusable date/month/year
  sub-fields to `Tab` without changing `document.activeElement`, and Chromium transiently reports
  `outline-style: none` mid-segment-transition — a real rendering quirk, not a missing ring. The
  walk only asserts on **first arrival** at a given element (tracked by id), not on every
  keystroke, to avoid a false failure from this quirk.

## 5. Consuming this for TASK-031 (contextual help component)

*(Status: implemented — TASK-031 followed this section as written: `help.templ`/`help.js` consume
only the tokens named below, the component is registered in §2's inventory and on the gallery page
(`#help-gallery-example`, covered by the keyboard walk), and its dedicated SC 1.4.13 behavior e2e
is `e2e/tests/contextual-help-UC037.spec.ts`. Kept as guidance for the next component author.)*

TASK-031's help-icon component should be born token-conformant, not retrofitted:

- Use `--color-accent`/`--focus-ring-*` for the trigger's own focus state (it is a real
  interactive element per WCAG 2.2 SC 1.4.13 — dismissible/hoverable/persistent, per
  `docs/research/ux-contextual-help-and-design-quality.md`).
- The popup/tooltip surface should use `--color-bg`/`--color-fg`/`--color-border` (a small
  elevated panel look, not a new color family) plus `--radius-sm`.
- Register any new component ids in the gallery page (§4) and a new row in §2's inventory table
  in the same change, so the keyboard walk covers it automatically (open/dismiss via keyboard is
  exactly what `e2e/tests/design-gallery.spec.ts` is built to catch regressions in).
- The `.hint`/`.field-error` conventions (§2) already exist for the *permanently visible*
  constraint text SYS-117 requires; the help-icon popup is for *supplementary* explanation only —
  do not let required information migrate into the hover/focus popup (research doc, Q1 finding 1).

## 6. Usability audit

Per-release process (not mechanically checkable) lives in
`docs/requirements/usability-audit-checklist.md` (NN/g heuristics + SYS-117 form conventions,
UC-038 #3–#4); its first run is recorded in `docs/delivery/usability-audit-2026-07.md`.
