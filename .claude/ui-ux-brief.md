# Standing UI/UX brief for Bahnfrei agents (TASK-060)

Read this before ANY change that renders — templates, CSS, islands, copy. Purpose: keep
LLM-default "AI slop" aesthetics out of the product. LLMs converge on the
high-probability center of web training data unless explicitly steered (Anthropic's own
finding: <https://claude.com/blog/improving-frontend-design-through-skills>). Sources of
truth this brief points at, never restates: `internal/web/static/tokens.css` (values),
`docs/architecture/design-system.md` §2 (component inventory) and §7 (binding design
principles), `docs/research/ui-ux-professional-practices.md` (evidence + numbers).

## 1. Vocabulary discipline

- No raw color/typography/spacing/radius literal outside `tokens.css` — CI enforces this
  (`scripts/check-style-tokens.sh`). Compose from existing custom properties only.
- Reuse the §2 inventory (`.button-primary`/`.button-secondary`, `.field`/`.form-grid`/
  `.form-row`/`.form-panel`, `.status-chip`, `.action-panel`/`.action-list`, `.hint`,
  `.field-error`, `.help`, `.stack-table`, `.checkbox-grid`, `.marker-btn`, …) instead of
  inventing parallel one-off classes. If a genuinely new component, token, or visual
  pattern seems needed, that is a founder-gated proposal (OQ/DEC or ADR) — raise it, do
  not decide it inside a task.

## 2. Banned patterns (check each one explicitly — a single skim is not enough)

LLM-generated UI has documented fingerprints. Go through this list item by item before
reporting a visual change done; if your output matches one, redo that part:

- Purple/blue or any gradient backgrounds; gradient text.
- Card-in-card nesting; borders around everything (order is: spacing → background shift
  → soft offset shadow → border last).
- Colored left-border accent strips on cards/alerts (the single most recognizable
  AI-generated-UI tell). Note: the shell has NO such component today — do not add one.
- Glassmorphism/decorative blur; floating orbs; neon-on-dark glow.
- Centered-hero layouts, badge-above-heading, reflexive 3-card grids, stat banners with
  decorative icons (bare numbers read as MORE important — NN/g).
- Emoji or decorative icons in product UI; icons are content and each must be justified.
- Dark-mode-by-default (light is the ratified register — outdoor legibility, SYS-113).
- Arbitrary spacing values (`13px`, `1.1rem`) — scale tokens only.
- Fonts: never introduce Inter/Roboto/Arial/Open Sans/Lato/Poppins/Space Grotesk/Geist
  or any new font. The faces are fixed: system-ui stack body, Barlow Semi Condensed
  display/numerals. Distinctiveness comes from execution (condensed numerals, tabular
  figures, restraint), not from new hues or faces — teal-as-accent is itself a common AI
  default, so ours must be earned by discipline around it.
- Sub-400 font weights; hierarchy by size alone; grey text on colored surfaces.

## 3. Grounding

Athletics is the design vernacular: start lists, lane assignments, bibs, splits, wind
readings, height progressions, tabular result grids. Before styling a surface, state in
one line its concrete subject, audience, and single job (e.g. "spectator on a phone in
sunlight scanning live 100m results" vs "office operator bulk-importing entries").
Derive choices from that, not from generic SaaS dashboard idiom.

## 4. Process: plan, critique, build, verify

1. **Plan** (short, in-task): which existing tokens/components compose this surface;
   what is the one thing this surface must make instantly readable.
2. **Critique the plan** against §2's banned list BEFORE writing code; say what you
   changed if anything matched.
3. **Build** with low, predictable CSS specificity. Hand-rolled cascade gotcha
   (Anthropic frontend-design skill): type-level selectors (`.section`) and
   element/utility selectors silently cancel each other, especially margins/paddings
   between sections — prefer composition over override, never `!important`.
4. **Verify with eyes**: you cannot judge rendered pixels from source. Build, run, and
   screenshot every changed state — including empty, loading, and error states — via the
   `verify` skill (Playwright). Read the screenshots back and self-critique against §2
   and design-system.md §7. Two to three rounds is the reported convergence point; if
   still not right after three, flag for human review instead of thrashing.

## 5. Quality floor (non-negotiable, mostly machine-checked)

- WCAG 2.2 AA contrast (`tokens_contrast_test.go` enforces token pairs — new pairs must
  land in it); visible focus on everything interactive (`--focus-ring-*`, ≥2px, ≥3:1).
- Responsive to 360px width; grouping/proximity re-checked at each breakpoint.
- `prefers-reduced-motion` respected; transitions 150–200ms standard easing.
- ≥44px (`--touch-target-min`) on point-of-competition touch controls.
- Numeric columns right-aligned + `tabular-nums`, headers bottom-aligned.
- Localized copy (DE/FR): run `scripts/gen-pseudo-locale` after locale JSON edits; tests
  scrape DE strings.

## 6. Microcopy

Plain language, active voice, the user's vocabulary — never internal IDs or system
structure. Action labels match their outcome copy ("Veröffentlichen" → "Veröffentlicht",
not a generic confirmation). Empty and error states get specific, helpful copy, never
silence or raw error text.

## Evidence trail

Full citations: `docs/research/ui-ux-professional-practices.md` (design practice) and
the TASK-060 research round records in the register (DEC-040). Key LLM-specific sources:
Anthropic frontend-design skill
(<https://github.com/anthropics/claude-code/blob/main/plugins/frontend-design/skills/frontend-design/SKILL.md>),
Anthropic frontend-aesthetics cookbook
(<https://platform.claude.com/cookbook/coding-prompting-for-frontend-aesthetics>),
practitioner anti-pattern catalogs
(<https://docs.bswen.com/blog/2026-03-20-ai-generated-ui-anti-patterns/>,
<https://vibecodekit.dev/ai-slop-design>), tokens-as-guardrails
(<https://www.builder.io/blog/design-system-ai-automation>,
<https://vercel.com/blog/how-to-prompt-v0>), screenshot-loop workflow
(<https://medium.com/@rotbart/giving-claude-code-eyes-round-trip-screenshot-testing-ce52f7dcc563>,
<https://youcanbuildthings.com/articles/fix-ugly-ui-claude-code/>).
