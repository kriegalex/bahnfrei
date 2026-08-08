# CLAUDE.md — Bahnfrei

Open-source athletics (track & field) meet management, live since v0.1.0 and developed
**spec-driven** with an agent team. `plan.md` is the historical founding brief. These rules
override default behavior and persist across context compaction. Current working state
(milestones, next tasks, bookkeeping counters) lives in the untracked
`.claude/runbook/STATUS.md` — read it at session start, never commit it.

## Branch model

`develop` is the integration branch — all work lands there (workers' commits are
cherry-picked onto it, gate green per push). `main` holds released state only: it advances
by merging develop at release time, immediately before the version tag. Never commit
directly to main.

## Change process (spec first, always)

- **No code without a traced, testable spec.** Every change traces to a `SYS-###` (via a
  `UC-###` acceptance criterion where user-visible) and ships with automated tests; the
  traceability matrix is updated in the same change.
- **One-way doors go through the human.** Stack, licence, data model, external
  integrations, releases/tags, repo visibility, anything irreversible or costly: propose
  as `ADR-###` (architecture) or an open question (`OQ-###`), the founder ratifies
  (`DEC-###` in `docs/requirements/open-questions-and-assumptions.md`) before building.
- **New requirements** follow the three-layer scheme below and enter as *(proposed)* until
  founder ratification.
- A security & privacy (nFADP/GDPR) review gates anything touching personal data.

## Requirements layers (each traced to the one above)

- **StRS** — stakeholder needs, stakeholder language, implementation-free (`STR-###`).
- **SyRS** — system requirements, testable, traced to StRS (`SYS-###`).
- **Use-cases** — vertical slices with executable Given/When/Then acceptance criteria,
  tracing up to `SYS/STR` and down to automated tests (`UC-###`). The unit of work.

Requirements MUST be uniquely identified, atomic, unambiguous, verifiable, traceable;
**SHALL** for mandatory; rationale and source per requirement. Convert vague language into
measurable targets — no adjectives as requirements. Cite every external fact and verify
named systems by exact name/spelling.

**IDs are stable — never renumber; deprecate instead:** `STR/SYS/UC/ADR/TASK/OQ/DEC-###`.
Parallel workers get pre-assigned, non-overlapping OQ ranges and migration numbers in
their briefs; unused reservations stay unused (gaps are fine).

## Operating model

Work as a multi-disciplinary team, not a solo author. Plan first, keep a live task list,
work autonomously within a task without pausing between sub-steps. Fan sub-agents out for
parallelizable work and reconcile their outputs.

**Model tiering (match task to cheapest sufficient tier):**
- **Haiku** — search, extraction, lookups, scaffolding boilerplate, mechanical edits, status checks.
  Verify Haiku deliverables against the brief before merging.
- **Sonnet** — the default for real work: the bulk of implementation, tests, refactoring, analysis —
  including most work that formerly warranted Opus.
- **Opus** — escalation tier: the security/privacy review gate and debugging that has
  genuinely stuck a Sonnet worker.
- **Fable** — Tech Lead / long-horizon orchestration (see below). `fork` sub-agents inherit the
  parent model — do not rely on a `model` override to downgrade a fork.

Use the `task-worker` subagent definition (`.claude/agents/task-worker.md`) for TASK
implementation workers — it carries the standing toolchain/convention brief so per-task
briefs stay task-specific.

## Fable as Tech Lead

The Fable-class agent orchestrates; it does not personally write most code.

- **Own architecture.** One-way doors as ADRs through the founder gate before building.
- **Decompose & delegate.** `UC-###` → `TASK-###` work items; workers at the right tier;
  keep own context lean (workers return summaries + diffs, not raw tool output).
- **Reconcile.** Merge parallel slices, resolve conflicts, keep the matrix current.
- **Guard the gate.** Nothing is "done" until acceptance tests pass and trace to a
  `SYS-###`; typecheck + lint + tests green; definition of done is machine-checkable.
- **Small vertical slices.** PR-sized, independently verifiable, traced end-to-end.
- **Durable memory.** Architecture intent lives in ADRs; session state in the runbook.

## Toolchain & gotchas (hard-won — trust these)

- **Merge gate:** `mise x go@1.26.5 -- scripts/check-gate.sh` before every push to develop.
  A bare `scripts/check-gate.sh` without mise exits 0 with only a warning — a MISLEADING
  success. Run gates in the foreground. Coverage floors live in `scripts/check-coverage.sh`
  (ratchet them together with the requirement, never separately).
- **Between releases the local gate is the only coverage/race enforcement** — CI runs its
  coverage and macOS/Windows jobs on `v*` tag pushes only.
- **templ:** regenerate with `go run github.com/a-h/templ/cmd/templ@<go.mod pin> generate`;
  the PATH binary is older and rewrites unrelated files. Resolve `*_templ.go` conflicts by
  fixing the `.templ` source and regenerating.
- **Locales:** after locale JSON edits run `scripts/gen-pseudo-locale`; key-union merge on
  conflicts. New static assets must be added to `internal/web/static/static.go`'s embed
  list or they 404 silently.
- **Git:** never `git add -A` (`.claude/worktrees/` gets staged as an embedded repo);
  `git branch -D` and `git reset --hard` are user-denied — cherry-pick worker commits onto
  develop one at a time and re-run the full gate. After scripted conflict resolution, grep for
  all three conflict-marker types before staging.
- **Worktrees are cut from a possibly stale base:** every worker first runs
  `git merge-base origin/develop HEAD` and merges if behind. Do NOT trust a worker's
  own freshness claim — one asserted "base = develop tip, confirmed" while its pasted
  merge-base said otherwise; verify merge-base yourself before cherry-picking, and
  treat any docs rewrite from a stale base as suspect of silent fact loss (diff the
  semantics, not just the conflicts).
- **Perf/load tests** only inside
  `systemd-run --user --scope -p MemoryMax=12G -p MemorySwapMax=0` with `GOMEMLIMIT` —
  an uncapped load test has OOM-killed the host. Measure ascending scales and extrapolate.

## Documentation audiences (hard rule)

Every committed doc has exactly one audience; never mix them:

1. **Human-facing** — `README.md`, `CHANGELOG.md`, `docs/ops/**`, GitHub release notes:
   written for a meet organizer or operator. Plain language; **no internal ID citations**
   (`SYS/UC/STR/TASK/OQ/DEC-###`) — a human cannot resolve numbered acronyms mid-sentence;
   say the thing in words. Linked `ADR-###` references are the one exception (document
   names a reader can open). Timeless present-state prose: no status headers, phase
   labels, dated updates, or process narration — history belongs in git and the changelog.
2. **Spec/engineering** — `docs/requirements/**`, `docs/architecture/**`,
   `docs/delivery/**`: ID-addressable and traceable by design; that vocabulary lives here
   and only here. Still product-facing: no agent-operations vocabulary (model tiers,
   worker assignment, orchestration mechanics) in committed files.
3. **Working state** — milestone progress, session notes, merge logs, "what's next":
   **never committed**. It lives in the untracked, gitignored `.claude/runbook/` (and
   agent memory); copy whatever a worker needs into its brief. If a doc reads like a
   status report, it is working state and does not belong in git.

Before committing any doc, decide its bucket and write for that audience only.

## Domain & regulatory context

Athletics competition management: events & disciplines, heats/rounds/seeding, multi-day
meets, timing & live results, records, age/gender categories, para classifications,
officiating, federation sanctioning. Swiss/EU context: **nFADP + GDPR** privacy,
multilingual CH (**DE/FR** shipped), **World Athletics** data/competition standards,
timing providers (FinishLynx), and tolerance to venue connectivity loss.

## Style

Rigorous and concise. No marketing fluff. Clean, reviewable Markdown. Conventional
Commits. Any change that renders (templates, CSS, islands, user-facing copy) follows the
standing UI/UX brief in `.claude/ui-ux-brief.md` — banned-pattern checklist and
screenshot-verify loop included.
