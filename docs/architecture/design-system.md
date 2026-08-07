# Design System

**Status:** TASK-032 deliverable (STR-045 → SYS-116 → UC-038 #1–#2). Home: architecture rather
than requirements, because this document describes *what was built* (the token set, the
component inventory, the CI mechanism) — the requirement itself lives in `system-requirements.md`
(SYS-116), the acceptance criteria in `use-cases.md` (UC-038), and the traceability row in
`traceability-matrix.md`. Sits alongside `architecture.md` §3 (`web/` package) and §4
(cross-cutting concerns), which this document extends with a design-layer baseline the same way
`adr/ADR-003-technology-stack.md` set the SSR/HTMX/templ baseline it styles.

**Neutral default (OQ-061):** no brand identity exists yet for Bahnfrei. Every value below is a
placeholder chosen for contrast/legibility, not a brand decision. Because every stylesheet and
template reads colors/spacing/typography/radius exclusively through the named custom properties in
`internal/web/static/tokens.css`, a future rebrand is a values-only edit to that one file — no
template, no component markup, and no Go handler needs to change.

## 1. Design tokens

Defined in `internal/web/static/tokens.css` (loaded before `base.css` in every page, see
`layout.templ`), as CSS custom properties on `:root`. This is the *only* file allowed to declare a
raw color/typography/spacing/radius literal — `scripts/check-style-tokens.sh` (§3 below) enforces
that mechanically in CI.

| Category | Tokens | Notes |
|---|---|---|
| Color — surface/text | `--color-bg`, `--color-fg`, `--color-border` | Base page colors; `--color-border` is a low-opacity neutral (rgba), used for hairline dividers (header nav underline). |
| Color — accent | `--color-accent`, `--color-accent-fg` | Links, the focus ring, and any future primary-action styling. |
| Color — semantic status | `--color-success-bg`/`-fg`, `--color-warning-bg`/`-fg`, `--color-warning-strong-bg`/`-fg`, `--color-info-bg`/`-fg`, `--color-danger-fg` | Back the offline-capture indicator's three states (SYS-087), the unofficial-results label (SYS-076), the office blip-tolerance banner (UC-034 #7), and inline/flash error text. |
| Typography | `--font-family-base`, `--font-size-base`, `--font-size-sm`, `--line-height-base`, `--font-weight-bold` | One family (system-ui stack, no web-font fetch — ADR-002/ADR-003 offline-first), two sizes (body/hint), one weight beyond regular. |
| Spacing | `--space-1` … `--space-6` (0.25rem–1.5rem, matching the values already in use across the shell) | A small scale, not a full ramp — extend it only when a new value is genuinely needed, not per-component. |
| Radius | `--radius-sm` (0.4rem) | Badges and banners; nothing in this shell uses a second radius yet. |
| Focus ring | `--focus-ring-color`, `--focus-ring-width`, `--focus-ring-offset` | Kept separate from the raw accent color/spacing tokens so SYS-114's keyboard-focus contract has one named, testable seam (`e2e/tests/design-gallery.spec.ts` asserts against the *rendered* outline, not these tokens directly, so a future value change is automatically re-verified). |
| Touch target | `--touch-target-min` (2.75rem/44px) | SYS-147/UC-039 #2: the minimum hit-target size for a primary point-of-competition control operated by touch, outdoors, under time pressure — the Apple HIG (44x44 pt)/Material Design (48x48 dp) floor, well above WCAG 2.2 SC 2.5.8's 24x24 px minimum. Applied at the ≤480px phone breakpoint (TASK-045); desktop/office density is unaffected. |
| Chrome control floor | `--control-min-height-sm` (1.5rem/24px) | SYS-116 (N4/TASK-051): every `<select>` gets at least this height — WCAG 2.2 SC 2.5.8's own floor, named explicitly rather than left to a select's intrinsic (font-driven) height. Distinct from `--touch-target-min`: this is the comfort bar for small chrome controls (the header locale switcher, previously 21px), not a change to already-larger primary capture controls. |

`color-scheme: light dark` is declared (browser chrome — scrollbars, form-control native
rendering — adapts to the OS), but the token *values* are light-only for 0.1: STR-045/SYS-116
require a coherent, documented system, not a dark theme, and no page currently ships dark-mode
colors to pair with it. A dark palette is a values-only addition to `tokens.css` when wanted — same
rebranding mechanism as OQ-061 — recorded as an assumption, not a new open question (nothing here
blocks 0.1; see `open-questions-and-assumptions.md` A-### working-assumptions section for the
pointer).

## 2. Component inventory

Every component below is authored once in `internal/web/static/base.css` and consumes tokens only.
The **states** column names what SYS-116 requires per component (default, hover, focus-visible,
active, disabled, error where applicable); "n/a" marks a state the component structurally cannot
have. The gallery fixture page (§4) renders one live instance of every row.

| Component | Selector(s) | States | Used on |
|---|---|---|---|
| Skip link | `.skip-link` | default (visually hidden), focus-visible (visible, positioned) | Every page (`layout.templ`) |
| Header nav | `header nav` | default | Every page |
| Button | `button` | default, hover (opacity), focus-visible (ring), active (opacity), disabled (opacity + `cursor:not-allowed`) | Forms across the app (login, meets, entries, capture, admin, …) |
| Link | `a` | default, hover (underline weight), focus-visible (ring), disabled via `aria-disabled="true"` (n/a today — no disabled link exists yet, documented for completeness) | Nav, in-page references |
| Text/number/date input | `input[type=text\|number\|date\|…]` | default, focus-visible (ring), disabled (opacity), invalid/error (`aria-invalid="true"` → red border, paired `.field-error` text) | Every form |
| Checkbox | `input[type=checkbox]` | default, focus-visible (ring), disabled | Consent/withdrawal flags, bulk-entry rows |
| Select | `select` | default, focus-visible (ring), disabled | Locale switcher, template/round pickers, bulk rows |
| Fieldset/legend | `fieldset`/`legend` | default (browser-native grouping box) | Grouped inputs (e.g. relay leg composition) |
| Hint text | `.hint` | default (muted, smaller) | Any field needing a visible constraint (SYS-117) — adopted for hard format/unit/bound constraints by the TASK-031 hint pass (each hint `aria-describedby`-associated with its input; residual gaps in OQ-076) |
| Contextual-help icon + popup | `.help` wrapper: `.help-trigger` (button) + `.help-popup` (`role="tooltip"`) | trigger: default, hover, focus-visible (ring), active, `aria-expanded` open/close; popup: hidden (default), open | TASK-031 (SYS-115, UC-037): every input in the help registry (`internal/web/help.go`); markup `help.templ`, behavior `/static/help.js` (hover + focus + click/tap open; SC 1.4.13 Escape-dismiss/hoverable/persistent) — trigger uses `--color-accent`/`--color-accent-fg`, popup `--color-bg`/`--color-fg`/`--color-border` + `--radius-sm` per §5 |
| Inline field error | `.field-error` + `input[aria-invalid="true"]`/`select[aria-invalid="true"]` | error only (n/a default/hover/active) | Adopted (TASK-034, OQ-075, UC-038 #4) via the reusable `FieldErrors` map + `fieldError`/`fieldErrorID` helpers (`internal/web/fielderrors.go`/`.templ`) on the three representative forms: meet setup (`meets.templ`), online entry (`entries.templ`), result correction (`capture.templ`, row-scoped ids via `rowFieldKey`). The gallery page remains the fixture instance the keyboard-walk e2e checks; extend to further forms by calling the same helper. |
| Table + wide-table scroll wrapper | `table`, `.table-scroll` (`tabindex="0"`) | default, focus-visible (ring, on the scroll wrapper) | Timetables, start lists, results, standings, roster, audit log |
| Offline-capture status badge | `.offline-status[data-state]` | online/default, offline, syncing (three distinct colors) | Field capture (UC-034; SYS-087) |
| Office blip-tolerance banner | `.offline-banner` | hidden (default) / visible+alert | Authenticated shell, all pages, while a connectivity blip is active (UC-034 #7) |
| Unofficial-results label | `.unofficial-label` | visible only when results are unofficial | Public results page (SYS-076) |
| Flash/alert message | `.error`, `.flash-error`, `role="alert"` | error only | Page-level validation/flow errors across the app |
| Destructive-action confirm page | `confirmPage`/`confirmView` (`internal/web/confirm.go`/`.templ`) — composed entirely of existing components (`.error` warning paragraph, plain text input, `.field-error`, `button`, `a`), no new CSS | n/a (a page composition, not a styled widget) | TASK-034 (OQ-074, UC-038 #3): the shared GET-confirm-sub-page pattern fronting all four destructive actions (athlete erasure, account disable, meet archive, retention purge) — a real navigation rather than a CSP-incompatible `onclick="confirm()"` dialog. Two of the four (erasure, retention purge) add a typed-confirmation text input as defense-in-depth (ASVS review note); see `confirm.go`'s package doc for the full rationale. TASK-053 (DEC-030) reuses the same component for account password reset, adding an optional `PasswordField` (a `type="password"` input, paired with `.field-error` like every other field) rather than repurposing the typed-confirmation input, which means something different ("type this exact value"). |
| Row-card responsive table | `.stack-table` (paired with `.capture-grid` or a plain `<table>`) | default (native table, ≥481px), row-card (≤480px, one card per row via `display:block` + `data-label`-prefixed cells) | TASK-045 (SYS-147, UC-039 #1): the field-horizontal/track capture grids, the check-in table, and the two live-standings tables (`standingsTable`, `verticalCaptureStandings`) — every "point-of-competition" table whose column count fits a one-column card without hiding in-progress state. Overriding `display` drops the browser's implicit table/row/cell ARIA roles, so the markup re-asserts `role="table"/"rowgroup"/"row"/"columnheader"/"cell"` explicitly at every viewport (Adrian Roselli, "Tables, CSS Display Properties, and ARIA"); each primary control keeps its own `aria-label` regardless of layout. **Not** used for the vertical-jump grid (below). |
| Sticky first column | `.vertical-grid-sticky` (wraps `.table-scroll`) | default (native table, ≥481px), sticky (≤480px: bib/name columns pinned via `position: sticky`, height columns scroll within the wrapper) | TASK-045 (SYS-147, UC-039 #1): the vertical-jump height x trial matrix is inherently two-dimensional and does not linearize into a row-card without hiding most of the in-progress state (a jump-off can add many height columns). The trade-off: the PAGE itself never gains horizontal scroll (the UC-039 #1 assertion), but reaching a later height column still scrolls this inner region — a deliberate, documented exception to the row-card pattern above, not an oversight. |
| Compact capture-page chrome | `.capture-context` (breadcrumb line) + `.capture-title` (`<h1>`) | default only | TASK-045 (SYS-147, UC-039 #2): replaces the former single-line "MeetName — Discipline" `<h1>` on the field/track/vertical capture pages and check-in with a small breadcrumb (the meet/event context) above a short, single-word `<h1>` — so the first capture row sits within the first 640px of page height at 360px width. The `<title>` tag (browser tab) is unaffected; it keeps the full "MeetName — Discipline" string the handler already sets. |
| Letter-marker quick actions | `.marker-actions` (wrapper) + `.marker-btn` (`markerButton` in `capture.templ`) | hidden (≥481px), visible with focus-visible/hover/active (inherited from the shared `button` rules), enlarged to `--touch-target-min` at ≤480px | TASK-045 (SYS-147, UC-039 #3): the field-horizontal grid's mark input and the vertical grid's trial input declare `inputmode="decimal"`/`inputmode="none"` respectively (a numeric-only or no virtual keyboard) — these buttons are the documented alternate path for the non-numeric D5.2 markers (X foul, – pass, r retirement; o clear on the vertical grid), wired by the small family-agnostic `/static/capture-markers.js` (sets the sibling value input and re-submits the form, working under both a plain POST and the offline-capture island's intercepted submit). TASK-052 (OQ-133): hidden above the phone breakpoint — a physical keyboard already types X/–/r/o without friction there, so the buttons were only ever redundant density on the office/desktop table; behavior and markup below the breakpoint are unchanged. |
| Per-cell save-state badge | `.cell-save-badge`, styling the existing `.cell-form[data-pending]`/`[data-state]` attributes (`islands/src/capture-offline.ts`) | pending (left border + badge text, `--color-info-fg`), confirmed (`--color-success-fg`), rejected (existing `.cell-reject`, TASK-044) | TASK-045 (SYS-148, UC-039 #4/#5): a JS-inserted text/icon badge inside the cell form, distinguishable by more than color per WCAG 2.2 — reuses the SAME `data-pending`/`data-state` attribute names the offline-capture island and TASK-044 already established rather than inventing parallel ones. Once a save is acknowledged, the row's own Result/Points cells (`[data-role=result]`/`[data-role=points]`, field-horizontal grid only — the only capture family with an async/offline save path today) update in place from the sync ack's authoritative values, closing the "only the standings fragment below the fold updates" gap (usability-audit F3). |

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
are *defined*) and the two vendored htmx assets (`static/htmx.min.js`, `static/htmx-LICENSE`) —
third-party, not project-authored style. This is a mechanical, pragmatic check, not a CSS parser:
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
