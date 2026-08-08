# Requirements-engineering documentation practice — evidence base (TASK-067)

Benchmark for auditing and maintaining this project's requirements corpus (StRS, SyRS,
use-cases, ADRs, open-questions/decision register, traceability matrix). Every claim
cites the source actually consulted. ISO/IEC/IEEE 29148:2018 itself is paywalled; its
content is taken from consistent secondary summaries and cross-checked against INCOSE
material, which 29148 draws on.

## 1. Requirement specifications (ISO/IEC/IEEE 29148, INCOSE)

29148 defines a document family with distinct audiences — BRS, OpsCon, **StRS**
(stakeholder expectations, implementation-free), **SyRS** (system requirements across
hardware/software/people), SRS — plus per-requirement and set-level quality
characteristics. It is process-agnostic (Agile artifacts permitted) and
self-declarative (no certification body).
Source: <https://www.modernrequirements.com/blogs/iso-29148-explained/>

**Nine individual characteristics:** necessary, appropriate, unambiguous, complete,
singular, feasible, verifiable, correct, conforming. **Set characteristics:** complete,
consistent, feasible, comprehensible, able-to-be-validated.

The INCOSE Guide to Writing Requirements (INCOSE-TP-2010-006-04 v4, 2023) refines these
into 42 writing rules; the ones most load-bearing for audits here:
R2 active voice (name the responsible entity); R7 no vague terms ("adequate",
"user-friendly"); R8 no escape clauses ("where possible"); R9 no open-ended clauses
("including but not limited to"); R18/R19 combinators ("and"/"or") signal a requirement
to split; R20 purpose phrases ("so that…") belong in a rationale **attribute**, not the
requirement body; R24 no pronouns — each requirement stands alone; R31 solution-free.
Source: <https://reqi.io/articles/incose-requirements-quality-42-rule-guide>

**Practice checklist:** stable unique IDs never renumbered; one SHALL per statement;
measurable thresholds instead of adjectives; rationale/source/verification method as
structured attributes outside the requirement sentence; StRS implementation-free; set
checked for consistency, not just rows.

**Anti-patterns:** requirement text accreting a change/status log; rationale narrated
inline; abstraction mixing (SyRS written as implementation instructions).

## 2. Use cases and acceptance criteria (Cockburn, BDD)

Cockburn's structural elements: design scope, explicit **goal level** (summary /
user-goal / sub-function), named primary actor, numbered main success scenario,
extensions referenced by step number, and four deliberate precision levels (actor+goal →
brief → extension conditions → extension handling).
Sources: <https://courses.cs.duke.edu/fall22/compsci307d/readings/cockburn_use_cases.pdf>
(excerpt); publisher description
<https://books.google.com/books/about/Writing_Effective_Use_Cases.html?id=TUZsAQAAQBAJ>

Given/When/Then practice: Given = specific precondition state; **one When per
scenario**; Then = observable, measurable outcome. Written collaboratively
("three amigos"), one behavior per scenario, at least one negative scenario where
failure modes are plausible; GWT is not forced onto trivial changes.
Source: <https://thedigitalprojectmanager.com/project-management/given-when-then-acceptance-criteria/>

**Anti-patterns:** implementation detail in Given/When/Then ("Given the API returns
200"); multiple scenarios crammed into one; happy-path-only coverage.

## 3. Architecture decision records (Nygard, MADR, GDS)

Nygard's format: short noun-phrase title; value-neutral **Context**; **Decision** in
active voice full sentences; **Status** from a small closed set; **Consequences**
listing positive, negative and neutral outcomes. Length "one or two pages" — "large
documents are never kept up to date". Numbering is sequential and never reused.
**Immutability:** "If a decision is reversed, we will keep the old one around, but mark
it as superseded" — a changed decision produces a *new* ADR, the old text is not
rewritten. Source (primary):
<https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions>

GDS operationalizes the lifecycle: superseded ADRs are marked "Status: superseded" with
a forward link as soon as the replacement is accepted; "if some implementation of the
original decision has occurred, you should write a new ADR".
Source: <https://gds-way.digital.cabinet-office.gov.uk/standards/architecture-decisions.html>

MADR v4 adds structured front matter (status, date, decision-makers,
consulted/informed), required Considered Options, Decision Outcome with justification,
and optional Confirmation (how the decision's success is checked). MADR does not itself
enforce immutability — that is a project convention to adopt explicitly.
Source (primary): <https://adr.github.io/madr/>

**Anti-patterns:** post-acceptance edits to Decision/Consequences; consequences listing
only positives; ADRs for trivial choices while load-bearing decisions go unrecorded;
implementation-log accretion ("Update: as of March…") inside an accepted record.

## 4. Decision / open-questions / assumptions registers (RAID practice)

A RAID-style register entry is a **scannable row**: category, description, owner, date,
status. Every entry has a designated owner; the log is reviewed on a defined cadence;
"information overload" — tracking everything and burying what matters — is a named
failure mode of the format itself.
Source: <https://www.atlassian.com/agile/project-management/raid-log>

The decision entry records *that* something was decided, by whom, when, and why.
Follow-on implementation status belongs in the linked artifact (tracker, commit
history), not appended into the entry — the same discipline as ADR immutability at
project-management granularity.

## 5. Traceability

Bidirectional traceability means explicitly stored links in both directions: forward
(requirement → design/code/tests: "has everything been verified?") and backward
(artifact → requirement: no "gold-plated" orphans). Sustaining practices: stable IDs
with consistent metadata; typed links; coverage validated in **both** directions; and
matrix updates propagated **in the same change** that alters a requirement — deferred
matrix maintenance is how "test cases end up passing execution against requirements
that have already changed". Manual matrices at scale develop stale links that "look
complete" while wrong — periodic spot-checks or machine checks are required.
Source: <https://www.jamasoftware.com/requirements-management-guide/requirements-traceability/bidirectional-traceability/>

A matrix cell is a stable pointer (IDs/paths), not a narrative.

## Cross-cutting finding

Every source family converges on one anti-pattern: **a spec or decision artifact
accreting implementation status or chronological narration into what should be a fixed,
atomic, point-in-time statement.** The consistent remedy is also shared: write a new,
separately identified record (a new ADR, a new register row, a commit message) instead
of growing the old one. Requirement rationale lives in attributes; decision entries
stay rows; matrix cells stay pointers; history lives in version control.
