# UX research — contextual input help & measurable design quality

Status: research deliverable backing STR-044/STR-045 → SYS-115/116/117 → UC-037/038
(founder request 2026-07-13). Authoritative sources only; fetched and verified 2026-07-13.

## Question 1 — is a "?" icon with floating help text on hover an appropriate pattern?

**Verdict: appropriate with three corrections.** The point-of-use help intent is well
supported by authoritative guidance; a *hover-only* implementation is not.

| # | Finding | Source |
|---|---------|--------|
| 1 | Never put information **vital to task completion** in a tooltip — it disappears and forces users to memorize it (e.g. password/format requirements). Tooltips are for supplementary explanation only. | Nielsen Norman Group, *Tooltip Guidelines* (A. Kendrick, 2019-01-27), guideline 1. <https://www.nngroup.com/articles/tooltip-guidelines/> |
| 2 | Hover-triggered tooltips **do not exist on touchscreens**; the touch-capable sibling is the "popup tip" paired with a **"?" or "i" icon**, opened by tap/click. Support both mouse *and* keyboard triggering; keep content brief; apply consistently across the product; position so related content is not blocked. | NN/g, ibid. (definition, popup-tips table, guidelines 2/3/5, additional recommendations) |
| 3 | Content that appears on hover/focus MUST be **dismissible** (e.g. Escape, without moving pointer/focus), **hoverable** (pointer can move onto the floating content), and **persistent** (stays until dismissed/de-hovered/invalid). This is a normative Level AA requirement, and our SYS-112 already mandates WCAG 2.2 AA. | WCAG 2.2 SC 1.4.13 *Content on Hover or Focus* (Level AA), W3C Understanding doc. <https://www.w3.org/WAI/WCAG22/Understanding/content-on-hover-or-focus.html> |
| 4 | Tooltip widget semantics: container `role="tooltip"`, trigger references content via `aria-describedby`, Escape dismisses, focus stays on the trigger. (Note: the APG pattern is flagged *work in progress, no task-force consensus* — hence SYS-115 mandates the behavior, and names the association technique only as an example.) | W3C WAI-ARIA Authoring Practices Guide, *Tooltip Pattern*. <https://www.w3.org/WAI/ARIA/apg/patterns/tooltip/> |
| 5 | Help relevant to **most users** belongs in permanently visible **hint text** (one short sentence, no links, never placeholder-as-label); hiding it costs discoverability and screen-reader usability. | GOV.UK Design System, *Text input* component — "Hint text" / "When not to use hint text" / "Avoid links". <https://design-system.service.gov.uk/components/text-input/> |

**Resulting design position (encoded in SYS-115):** the "?" icon is adopted, but it must
open on hover **and** keyboard focus **and** click/tap (our SYS-113 already bans hover-only
interactions); it must meet SC 1.4.13 (dismissible/hoverable/persistent); and anything
*required* to complete the field stays on-screen as label/hint text — the popup carries only
supplementary explanation.

## Question 2 — turning "modern design / UX best practices" into requirements

"Modern" is an adjective and not verifiable (CLAUDE.md: no adjectives as requirements). The
measurable proxies adopted, each with precedent in the sources above plus:

- **Single design system / tokens, no ad-hoc visual literals** (SYS-116) — consistency is
  the mechanism behind NN/g guideline 5 ("use tooltips consistently") generalized; it is
  also what makes visual quality reviewable and rebranding a data change (OQ-061).
- **Documented form/interaction conventions + per-release usability audit with zero open
  critical findings** (SYS-117) — checklist derived from the NN/g usability heuristics and
  the GOV.UK form patterns, executed like the existing
  `accessibility-manual-audit-checklist.md` (same I-verification mechanism as SYS-112).

Existing baseline already covering parts of the intent (no duplication added): SYS-112
(WCAG 2.2 AA), SYS-113 (responsive ≥360 px, no hover-only), SYS-114 (expert keyboard use),
STR-035 (learnability).
