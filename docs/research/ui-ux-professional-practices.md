# UI/UX evidence base — professional data-dense application design (TASK-060)

Evidence round for the 0.2 professional-UI track (DEC-040): published best practice for
data-dense operational web applications and for the visual craft that separates a
professional-reading UI from an amateur one. Joins `visual-identity-trends.md` (brand
identity evidence, DEC-036) and `ux-contextual-help-and-design-quality.md` (SYS-115/117
evidence). Every claim below was read from the cited page during the research session;
claims marked **[secondary]** could not be read from the primary page (JS-only rendering)
and are search-synthesis only — re-verify before treating them as load-bearing.
Binding conventions distilled from this evidence live in
`docs/architecture/design-system.md` §7; this document is the raw evidence and citations.

## 1. Spacing and sizing scales

- IBM Carbon builds all spacing on multiples of 2/4/8px, one scale (2–160px, 13 steps)
  reused from component-internal padding to page layout, with explicit permission to step
  down the scale at narrower breakpoints and the warning that "sections of a UI are
  allowed to be dense, but the whole page should not be crowded."
  <https://carbondesignsystem.com/elements/spacing/overview/>
- Atlassian uses an 8px base unit with half/quarter steps down to 2px, and documents
  usage bands: 0–8px for compact component internals, 12–24px for larger component
  padding, 32–80px reserved for page-level layout. Its four layout principles: group by
  similarity, group by proximity, hierarchy via scale + whitespace, optical adjustment.
  <https://atlassian.design/foundations/spacing>
- Refactoring UI (Wathan & Schoger): "Start with too much white space" — begin with more
  than feels right, remove until tight, never the reverse; "Establish a spacing and
  sizing system" — a constrained numeric scale, no hand-picked pixel values.
  <https://www.refactoringui.com/> (chapter list);
  <https://medium.com/refactoring-ui/7-practical-tips-for-cheating-at-design-40c736799886>
- Erik Kennedy (learnui.design): "Double your whitespace … sometimes a ridiculous
  amount" — named alongside grayscale-first design as the highest-leverage
  amateur-to-professional lever.
  <https://www.learnui.design/blog/7-rules-for-creating-gorgeous-ui-part-1.html>

Two independent enterprise systems converging on the same 4/8px-derived scale is a
strong signal; inconsistent eyeballed spacing is among the fastest visual tells of an
unpolished application. Bahnfrei's `--space-1`…`--space-6` tokens are already
4px-multiples; the gap is *consistent application* plus the proximity discipline in §3.

## 2. Data tables and dense grids

- NN/g's data-table framework (find / compare / view-edit / act): human-readable first
  column, related columns adjacent, frozen header row and first column on tables larger
  than the viewport, always-available row-hover highlighting, and a **non-modal side
  panel** (not a modal) for single-row edit — users refer back to other rows mid-edit.
  <https://www.nngroup.com/articles/data-tables/>
- Adrian Roselli: right-align numeric columns, left-align text; column headers
  bottom-aligned, cells top-aligned; zebra striping earns its place on wide tables
  (translucent stripes); the accessible responsive default is a scrollable wrapper
  (`role="region"` + `aria-labelledby` + `tabindex="0"`), because `display` overrides on
  table elements strip native table semantics unless ARIA roles are manually restored.
  <https://adrianroselli.com/2017/11/a-responsive-accessible-table.html>
- GOV.UK table component: numeric cells right-aligned via a dedicated modifier; prefer
  splitting large data sets over densifying; a small-text-below-tablet modifier exists
  but only for genuinely large tables. <https://design-system.service.gov.uk/components/table/>
- IBM Carbon documents five named row heights (24/32/40/48/64px) with a hard rule that
  the header row height always matches the body row height and the toolbar height pairs
  with the row size; column headers 14px SemiBold, cells 14px Regular; skeleton rows (not
  spinners) for slow-loading tables; row hover always on, purely to aid horizontal
  scanning. <https://carbondesignsystem.com/components/data-table/usage/>,
  <https://carbondesignsystem.com/components/data-table/style/>
- Shopify Polaris **[secondary]**: wrap cell content instead of truncating (truncation
  makes similar rows indistinguishable); design header/cell content to still make sense
  stacked as key–value pairs on narrow viewports.

Directly applicable to the entries, seeding, start-list, results and standings tables.
The Carbon header-height rule and named density steps give a reusable contract for a
"compact" capture-grid mode versus a "comfortable" public-results mode.

## 3. Forms in operational applications

- NN/g (form design & white space): place labels **above** fields — label and field fall
  in one eye fixation and label length is unconstrained; left-aligned labels only as a
  length compromise; placeholder-as-label is a named anti-pattern (label disappears on
  input, breaks tabbing, mistaken for a prefilled answer). The load-bearing proximity
  rule: **the label-to-own-field gap must be visibly smaller than the gap to the next
  field**, and long forms break into labeled sections of ~4–6 related fields.
  <https://www.nngroup.com/articles/form-design-white-space/>
- NN/g (Gestalt proximity): proximity overpowers competing cues such as color or shape
  similarity, and responsive breakpoints can silently destroy a grouping that worked at
  desktop width — grouping must be re-verified per breakpoint.
  <https://www.nngroup.com/articles/gestalt-proximity/>
- GOV.UK question pages: hint text is one short sentence, no full stop, no links; if the
  explanation needs more, it becomes a heading + body text above the field, never a long
  hint. **Never mark required fields with an asterisk — mark optional fields with
  "(optional)" instead.** Multi-step progress indicators should be tested *without*
  first: GOV.UK removed a 12-step indicator with no effect on completion rate; a minimal
  "Question 3 of 9" caption suffices where anything is needed at all.
  <https://design-system.service.gov.uk/patterns/question-pages/>

## 4. Dashboards, status displays, loading states

- NN/g complex-application guidelines (written for expert-user, specialized-domain
  apps): reduce clutter via staged/progressive disclosure of advanced fields; surface
  secondary detail in place (tooltip/hover) rather than a drill-down; and make key
  numbers salient **by removing decoration, not adding emphasis** — their side-by-side
  shows bare stat numbers reading as *more* important than the same numbers next to
  decorative icons. <https://www.nngroup.com/articles/complex-application-design/>
- NN/g skeleton screens: skeletons are for **full-page** loads under ~10s and must be
  structural (blocks where content will land — frame-only skeletons are explicitly not
  recommended); a **spinner** fits a single loading module; a **progress bar** is
  required past ~10s; below ~1s show nothing — a flashing indicator is noise.
  <https://www.nngroup.com/articles/skeleton-screens/>
- Nielsen's response-time thresholds (0.1s direct-manipulation / 1s flow-of-thought /
  10s attention) calibrate when a "working" affordance is needed at all.
  <https://www.nngroup.com/articles/response-times-3-important-limits/>

## 5. Color discipline

- Refactoring UI: build hue-constant HSL tint/shade ramps up front instead of ad hoc hex
  picks ("Ditch hex for HSL", "Define your shades up front"); de-emphasized text on a
  colored background is never flat grey — reduce white's opacity or hand-pick a
  same-hue tone ("it doesn't look so great on colored backgrounds … making the text
  closer to the background color is what actually helps create hierarchy").
  <https://www.refactoringui.com/>, 7-practical-tips article (§1).
- 60-30-10 proportion rule: ~60% dominant surface color, ~30% secondary, ~10% accent
  reserved for primary actions and highlights.
  <https://uxplanet.org/the-60-30-10-rule-a-foolproof-way-to-choose-colors-for-your-ui-design-d15625e56d25>
- Erik Kennedy: design in grayscale first, add one hue last, "only with purpose" —
  overuse of color is the named easy way to lose "clean and simple". (Part 1, rule 2.)

For Bahnfrei: warm cream is the dominant ~60%, `--color-surface` and neutrals the ~30%,
and the ratified teal must stay a ~10% accent (actions, links, focus, key data) — teal
painted across large areas would be the amateur register of the same palette.

## 6. Typography craft

- Butterick's Practical Typography, key numeric rules: body text 15–25px on the web;
  line spacing 120–145% of point size; line length 45–90 characters; bold *or* italic,
  never both; all-caps only below one line of text and letter-spaced +5–12%.
  <https://practicaltypography.com/summary-of-key-rules.html>
- Modular type scales: one base size × a fixed ratio (major third 1.25 / perfect fourth
  1.333 are the sane bands for dense data UI) generates every size; NN/g's ceiling: a
  pleasing design generally uses **no more than ~3 sizes** per view, with hierarchy
  carried by weight and color rather than size alone.
  <https://www.modularscale.com/>, <https://www.nngroup.com/articles/principles-visual-design/>
- Refactoring UI: 2–3 text colors and 2 font weights suffice for hierarchy; never use
  weights below 400 for UI text. (7-practical-tips, tip 1.)
- Tabular figures: `font-variant-numeric: tabular-nums` gives every digit the same
  advance width so stacked numbers align — required for right-aligned numeric columns to
  *look* aligned. The property is a silent no-op if the font lacks the `tnum` OpenType
  feature — Bahnfrei's self-hosted Barlow Semi Condensed build has the feature verified
  and `.results-table` already uses it (design-system.md §1); the remaining work is
  extending the same treatment to every numeric/timing column, not enabling it.
  <https://developer.mozilla.org/en-US/docs/Web/CSS/font-variant-numeric>
- Erik Kennedy, "make text pop — and un-pop": emphasis combines competing properties
  (e.g. a large number in a lighter weight and softer color paired with a small
  bold/uppercase label); single-direction styling ("just make it bigger") is the amateur
  pattern. (Part 2, rule 5.)

## 7. Depth, borders, motion, focus

- Refactoring UI: "use fewer borders" — replace with a box shadow, adjacent background
  colors, or spacing; shadows get a vertical offset (light from above) rather than large
  symmetric blur; designed **empty states** are a named principle ("don't overlook empty
  states") — a bare "no rows" table is a professionalism tell in a list-heavy app.
- Material Design duration & easing, exact figures: desktop transitions 150–200ms
  (mobile ~225–300ms, >400ms "may feel too slow"); standard easing
  `cubic-bezier(0.4, 0.0, 0.2, 1)`, deceleration for entering elements, acceleration for
  exiting. <https://m1.material.io/motion/duration-easing.html>
- WCAG 2.2: Focus Visible (SC 2.4.7) and Non-text Contrast (SC 1.4.11, focus indicator
  ≥3:1 against adjacent colors) bind at AA; Focus Appearance (SC 2.4.13, AAA) supplies a
  concrete quality bar worth meeting anyway — indicator area ≥ a 2px perimeter and ≥3:1
  change-of-contrast between focused and unfocused states. Removing the default outline
  without replacement is named failure F78.
  <https://www.w3.org/WAI/WCAG22/Understanding/focus-appearance.html>

## 8. Touch targets and density

- WCAG 2.2 SC 2.5.8 (AA): 24×24 CSS px minimum with a precisely specified
  spacing exception (24px-diameter non-intersecting circles); the Understanding document
  itself recommends aiming for SC 2.5.5 Enhanced (44×44px) for important controls, and
  explicitly endorses a **user-selectable density mode** as a legitimate way to serve
  conflicting accessibility needs.
  <https://www.w3.org/WAI/WCAG22/Understanding/target-size-minimum.html>
- Material Design: 48×48dp targets with ≥8dp gaps; a smaller visual glyph may sit inside
  the larger hit area. <https://support.google.com/accessibility/android/answer/7101858>
- Apple HIG **[secondary]**: 44×44pt minimum (long-standing; page is a JS-only SPA that
  could not be fetched — re-verify wording before quoting).

Read together the specs converge on ~44–48px as the practical target, with 24px only a
legal floor. Bahnfrei's existing `--touch-target-min` (44px, SYS-147) clears everything
except Material's 48dp; whether the packed icon-only capture-grid controls should target
48px specifically is a design-audit question, not settled here.

## Highest-leverage practices, ranked

Ranked by visual-credibility impact × implementation cost (cheapest big wins first):

1. Apply the existing 4/8px spacing scale *consistently*, with the proximity rule
   (in-group gap < between-group gap) — §1, §3.
2. Right-align + tabular-figure every numeric/timing/rank column, headers bottom-aligned
   — §2, §6.
3. Whitespace first: err generous, prune later; never densify the whole page — §1.
4. Hierarchy from weight + color at ≤3 sizes per view; no sub-400 weights — §6.
5. Fewer borders: spacing → background shift → soft offset shadow, border last — §7.
6. Always-on row hover + frozen headers on long tables; header height = row height — §2.
7. Loading affordances matched to context: nothing <1s, spinner per module, structural
   skeleton for full page, progress bar >10s — §4.
8. Focus rings to the 2.4.13 quality bar (≥2px, ≥3:1) — compliance and craft in one — §7.
9. Designed empty states for every list/table — §7.
10. Strip decorative icons from stat numbers; bare numbers + whitespace carry weight — §4.
11. Transitions in the 150–200ms standard-easing band; respect reduced-motion — §7.
12. Forms: optional-marking not asterisks, one-sentence hints, staged disclosure of
    advanced fields, no untested progress indicators — §3.
