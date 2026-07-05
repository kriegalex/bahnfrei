# CLAUDE.md — Athletics Tournament System

This repository is an **open-source athletics tournament management system**, delivered as a
**spec-driven, agent-built** engagement. Read `plan.md` for the full brief. These rules override
default behavior and persist across context compaction.

## Prime directive
This engagement runs in **two phases**, separated by a hard, human-ratified gate:

- **Phase A — Requirements baseline (implementation-free).** Produce rigorous, traceable
  requirements. Do **NOT** write application code, choose a stack, or scaffold during Phase A.
- **Phase B — Implementation (Fable-led).** Only after the requirements baseline is approved
  **and** the foundational architecture ADRs are ratified by the human, build the system against
  the specs.

Never begin Phase B work before both gate conditions are met. When in doubt about which phase a
task belongs to, treat it as Phase A.

## Requirements as executable specs (the agentic layer)
Two-layer 29148 requirements are the traceable, regulator-facing baseline — necessary but not
directly agent-executable. Maintain **three** layers, each traced to the one above:

- **StRS** — stakeholder needs, in stakeholder language, **implementation-free** (`STR-###`).
- **SyRS** — technical/system requirements, testable, traced to StRS (`SYS-###`).
- **Use-cases / features** — vertical slices with **executable acceptance criteria**
  (Given/When/Then), each tracing **up** to `SYS-###`/`STR-###` and **down** to automated tests
  (`UC-###`). This is the unit of work Fable assigns and agents self-verify against.

Every requirement MUST be: uniquely identified, atomic, unambiguous, **verifiable/testable**, and
traceable. Use **SHALL** for mandatory. Record **rationale** and **source** per requirement.
Convert vague founder language ("state of the art", "bug free", "well tested") into **measurable**
quality attributes with targets — no adjectives as requirements.

**ID schemes (stable — never renumber; deprecate instead):** stakeholder `STR-###`, system
`SYS-###`, use-case `UC-###`, architecture decision `ADR-###`, work item `TASK-###`.

## Operating model
Work as a **multi-disciplinary team**, not a solo author. **Plan first**, keep a live task list,
and work autonomously within a phase without pausing between sub-steps. Spin up sub-agents for
parallelizable work and reconcile their outputs.

**Model tiering (match task to cheapest sufficient tier):**
- **Haiku** — search, extraction, lookups, scaffolding boilerplate, mechanical edits, status checks.
- **Sonnet** — the default for real work: the bulk of implementation, tests, refactoring, analysis.
- **Opus** — hard reasoning only: tricky multi-file debugging, security/privacy analysis, ambiguity.
- **Fable** — Tech Lead / long-horizon orchestration (see below). `fork` sub-agents inherit the
  parent model — do not rely on a `model` override to downgrade a fork.

## Phase B — Fable as Tech Lead
In Phase B, a **Fable-class agent is the Tech Lead and orchestrator**. It owns the architecture
baseline and coordinates implementation of the use-cases; it does not personally write most code.

Fable's responsibilities:
- **Own architecture.** Propose the stack, data model, and external integrations as **ADRs**;
  route every one-way-door decision through the human gate before building on it.
- **Decompose & delegate.** Turn `UC-###` slices into `TASK-###` work items; assign to worker
  agents at the right tier; keep its own context lean by pushing detail into sub-agents (workers
  return results and diffs, not raw tool output).
- **Reconcile.** Merge parallel slices, resolve conflicts, keep the traceability matrix current.
- **Guard the verification gate.** Nothing is "done" until its acceptance tests pass and trace to a
  `SYS-###`.

Best-practice pillars Fable enforces (2026 agentic norms, Opus/Fable-class):
1. **Spec-driven.** Specs are the source of truth. No code without a traced, testable spec; nothing
   ships until its executable acceptance criteria pass.
2. **Orchestrator–worker.** Fable delegates; workers do narrow, well-scoped units and return
   summaries + diffs. Fan out independent slices; reconcile via the traceability matrix.
3. **Closed-loop verification.** Every slice ships with automated tests mapped to `SYS-###`;
   typecheck + lint + tests green before "done". Definition of done is **machine-checkable**.
4. **Human one-way-door gates.** Stack, license, data model, external integrations, and anything
   irreversible/costly: Fable **proposes via ADR, human ratifies** before build.
5. **Durable memory.** Architecture intent lives in ADRs so context compaction never loses it.
6. **Small vertical slices.** PR-sized, independently verifiable, each traced end-to-end.
7. **Guardrails.** Least privilege; no destructive or outbound actions without confirmation; a
   security & privacy (nFADP/GDPR) review gate on anything touching personal data.
8. **Context engineering.** Docs are ID-addressable and retrieval-friendly (tables, stable IDs) so
   agents load only what a task needs.

## Quality bar (definition of done)
- Zero orphan requirements — everything is traced in `traceability-matrix.md`
  (`STR → SYS → UC → test → verification method`).
- **Cite every external fact.** Verify named systems by exact name/spelling (Swiss Athletics;
  "Seltec" — confirm, the seed wrote "setlec").
- Make assumptions explicit; consolidate open questions in
  `docs/requirements/open-questions-and-assumptions.md`.
- State scope boundaries: prioritized **MVP vs. later**, plus explicit **out-of-scope** items.
- In Phase B: each `TASK-###` is merged only with passing acceptance tests traced to a `SYS-###`,
  and any architecture decision it relied on recorded as an `ADR-###`.

## Deliverable layout
```
docs/research/domain-athletics.md
docs/research/competitive-analysis.md
docs/requirements/stakeholder-requirements.md      # StRS, STR-###
docs/requirements/system-requirements.md           # SyRS, SYS-###
docs/requirements/use-cases.md                     # UC-###, executable acceptance criteria
docs/requirements/traceability-matrix.md           # STR → SYS → UC → test → verification
docs/requirements/open-questions-and-assumptions.md
docs/requirements/glossary.md
docs/architecture/architecture.md                  # Phase B: baseline architecture
docs/architecture/adr/ADR-###-*.md                 # Phase B: architecture decision records
docs/delivery/work-breakdown.md                    # Phase B: TASK-### backlog Fable coordinates
README.md
# Application source is added in Phase B, after the gate.
```

## Domain context to keep in mind
- Athletics/track & field competition management: events & disciplines, heats/rounds/seeding,
  multi-day meets, timing & live results, records, age/gender categories, para classifications,
  officiating, federation sanctioning.
- Regulatory/locale context is **Swiss/EU**: privacy under **nFADP + GDPR**; multilingual CH
  (**DE/FR/IT/EN**); interoperability with **World Athletics** data/competition standards and
  timing providers; on-site/**offline venue operation** matters.

## Style
- Rigorous and concise. No marketing fluff. Clean, reviewable Markdown.
