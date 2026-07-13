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
| Hint text | `.hint` | default (muted, smaller) | Any field needing a visible constraint (SYS-117) — not yet adopted everywhere, see the usability-audit findings |
| Inline field error | `.field-error` + `input[aria-invalid="true"]`/`select[aria-invalid="true"]` | error only (n/a default/hover/active) | Documented convention; the gallery page is its first real usage — see `docs/delivery/usability-audit-2026-07.md` finding U-1 for adoption status across existing forms |
| Table + wide-table scroll wrapper | `table`, `.table-scroll` (`tabindex="0"`) | default, focus-visible (ring, on the scroll wrapper) | Timetables, start lists, results, standings, roster, audit log |
| Offline-capture status badge | `.offline-status[data-state]` | online/default, offline, syncing (three distinct colors) | Field capture (UC-034; SYS-087) |
| Office blip-tolerance banner | `.offline-banner` | hidden (default) / visible+alert | Authenticated shell, all pages, while a connectivity blip is active (UC-034 #7) |
| Unofficial-results label | `.unofficial-label` | visible only when results are unofficial | Public results page (SYS-076) |
| Flash/alert message | `.error`, `.flash-error`, `role="alert"` | error only | Page-level validation/flow errors across the app |

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
