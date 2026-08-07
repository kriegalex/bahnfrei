# Defect policy

**Status:** public policy, effective at release 0.1.

## 1. Severity definitions

| Severity | Definition | Examples in this system |
|---|---|---|
| **Critical** | Data loss or corruption of confirmed data; a security or privacy vulnerability that exposes personal data or lets an unauthorized actor act as another role; the system cannot start or is unusable for its core purpose (meet setup, capture, results) | A crash-recovery gap that loses a confirmed result; an authorization bypass reachable without credentials; public exposure of data that must never be public (full birth date, licence number) |
| **High** | A core competition-management function produces an incorrect result, silently drops data, or is blocked with no workaround; a defect that would make a real meet unrunnable or untrustworthy | Wrong record/PB flagging; a rule-defined hand-timing conversion applied incorrectly; an import that silently skips rows instead of reporting rejections |
| **Medium** | A function works but with a meaningful usability, accessibility, or correctness defect that has a workaround, or affects a non-core feature | A usability-audit finding short of critical (see `docs/requirements/usability-audit-checklist.md`); a non-primary export format with a formatting bug |
| **Low** | Cosmetic, edge-case, or minor documentation issues with no functional impact | A localization string missing a placeholder in an unusual code path; a typo in a help tooltip |

Severity is assigned by the maintainers who triage the defect, against this table, not by
reporter self-assessment; a report can be re-triaged if new evidence changes the picture.

## 2. Release gate

**No release ships with a known, unfixed Critical or High severity defect.** "Known" means: filed,
reproduced, and not yet closed with a fix and a passing regression test. A Critical/High defect
discovered after a release is cut triggers an out-of-band patch release per
`docs/ops/release-process.md` §5 (support window), not a wait for the next scheduled release.

Medium and Low defects do not block a release; they are tracked and prioritized against the
work-breakdown backlog like any other work item.

## 3. Regression-test requirement

**Every fixed defect gains a regression test before the fix is considered done.** This is the same
closed-loop-verification discipline the project runs under everywhere: nothing ships until its
automated acceptance tests pass, and every piece of merged work closes with passing tests.
Concretely:

- The test reproduces the defect's originally observed symptom and fails without the fix, on the
  commit immediately before it.
- The test is named or commented so a later reader can find both the defect and the fix (this
  project's convention: reference the relevant requirement or issue ID in the test name or a
  comment, as done throughout `internal/*_test.go`).
- The test is added to the same package/suite the defect lives in, so it runs under the normal
  `go test ./...` gate (or the E2E suite for a browser-observable defect) — no defect fix merges
  with only a manual verification note.

## 4. Reporting a defect

File an issue in the project's issue tracker describing: what you did, what you expected, what
happened, your bahnfrei version (`bahnfrei --version`), OS/browser, and — for anything touching
personal data — whether the report itself should avoid including real athlete data (use synthetic
data in the report; see `docs/ops/privacy.md` if you're unsure what counts as personal data).
Security-sensitive defects (anything enabling unauthorized access or data exposure) should be
reported through whatever private channel `CONTRIBUTING.md`/`GOVERNANCE.md` names, not a public
issue, until a fix is available.

## 5. What this policy does not cover

This is a defect (bug) policy, not a feature-request or roadmap process — see
`docs/delivery/work-breakdown.md` and `docs/requirements/open-questions-and-assumptions.md` for how
new work and open questions are tracked. A known **architectural limitation that is documented as
such**, with a measured extent and an open question tracking how (or whether) it will close, is not
a "known unfixed defect" under this policy in the sense that blocks a release — it is a disclosed
limitation, not a silent bug (for example, the public-viewer capacity gap was documented as an
open question while it was outstanding, with the measured numbers in
`docs/ops/support-matrix.md`, until a fix closed it). The distinction matters: silence about a
real limitation would violate this policy's intent even if no single defect report names it,
which is why open questions like this are documented publicly rather than left implicit.
