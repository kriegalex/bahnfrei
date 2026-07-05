# Requirements Traceability Matrix

**Document status:** Phase A baseline. Chain: `STR → SYS → UC → test → verification method`.
The **test** column is populated in Phase B (test IDs/paths); at baseline it is `Phase B`.
**Verification methods:** T = automated Test, D = Demonstration (scripted manual),
I = Inspection (artifact/document review), A = Analysis (benchmark/load/measurement).

## 1. Stakeholder → System requirements (forward coverage)

Every STR maps to ≥1 SYS. (Priorities per StRS §3.)

| STR | System requirements |
|-----|---------------------|
| STR-001 | SYS-001, SYS-002, SYS-003 |
| STR-002 | SYS-004, SYS-071 |
| STR-003 | SYS-006 |
| STR-004 | SYS-001, SYS-120 |
| STR-005 | SYS-010, SYS-011, SYS-012, SYS-015 |
| STR-006 | SYS-013 |
| STR-007 | SYS-005, SYS-014, SYS-015 |
| STR-008 | SYS-016, SYS-028, SYS-046 |
| STR-009 | SYS-017 |
| STR-010 | SYS-018 |
| STR-011 | SYS-025 |
| STR-012 | SYS-026, SYS-027, SYS-028, SYS-030, SYS-121 |
| STR-013 | SYS-029, SYS-030 |
| STR-014 | SYS-040, SYS-041, SYS-045, SYS-050, SYS-142 |
| STR-015 | SYS-042, SYS-043 |
| STR-016 | SYS-031, SYS-044, SYS-053, SYS-121, SYS-142 |
| STR-017 | SYS-048 *(Later)* |
| STR-018 | SYS-060, SYS-061, SYS-062, SYS-063 *(L)*, SYS-064 *(L)*, SYS-078 *(L)* |
| STR-019 | SYS-046, SYS-047 |
| STR-020 | SYS-080 *(L — DEC-013)*, SYS-082 *(L)*, SYS-085, SYS-087, SYS-093, SYS-130 |
| STR-021 | SYS-083, SYS-085, SYS-086 |
| STR-022 | SYS-070, SYS-071, SYS-074, SYS-113, SYS-122 |
| STR-023 | SYS-072, SYS-111 |
| STR-024 | SYS-041, SYS-049, SYS-050, SYS-051, SYS-142 |
| STR-025 | SYS-073, SYS-077, SYS-078 *(L)* |
| STR-026 | SYS-073 |
| STR-027 | SYS-075 *(Later)* |
| STR-028 | SYS-070 |
| STR-029 | SYS-003, SYS-005, SYS-010, SYS-052 |
| STR-030 | SYS-052, SYS-010 (para extension: Later, DEC-007) |
| STR-031 | SYS-091, SYS-092, SYS-093, SYS-100, SYS-101, SYS-102, SYS-104, SYS-105 |
| STR-032 | SYS-100, SYS-102, SYS-103 |
| STR-033 | SYS-074, SYS-110, SYS-111 |
| STR-034 | SYS-112, SYS-113 |
| STR-035 | SYS-114, SYS-120, SYS-131 |
| STR-036 | SYS-092, SYS-132, SYS-133, SYS-146, CON-02 |
| STR-037 | SYS-062, SYS-073, SYS-078 *(L)*, SYS-105, SYS-144 |
| STR-038 | SYS-092, SYS-104, SYS-140, SYS-141, SYS-142, SYS-143, SYS-144, SYS-145, SYS-146 |
| STR-039 | SYS-084, SYS-131, SYS-132 |
| STR-040 | SYS-090, SYS-091, SYS-086 |
| STR-041 | SYS-081, SYS-084, SYS-085, SYS-086, SYS-087, SYS-130 |
| STR-042 | SYS-076, SYS-077, SYS-078 *(L)* |
| STR-043 | SYS-053, SYS-077 |

**Coverage check:** 43/43 STR covered. Zero orphan stakeholder requirements.
*(2026-07-05 delta: STR-042/043 added from founder answers; STR-030 re-scoped per DEC-007.)*

## 2. System requirements → Use-cases → verification

Every SYS maps to ≥1 STR (see §1) and is verified via a UC criterion, a CI gate, or an
inspection/analysis procedure.

| SYS | Verified by (UC / procedure) | Method | Test (Phase B) |
|-----|------------------------------|--------|----------------|
| SYS-001 | UC-001 #2 | T | Phase B |
| SYS-002 | UC-001 #3 | T | Phase B |
| SYS-003 | UC-002 #5 | T | Phase B |
| SYS-004 | UC-001 #4 | T | Phase B |
| SYS-005 | UC-002 #1–#4 | T | Phase B |
| SYS-006 | UC-001 #5 | T | Phase B |
| SYS-010 | UC-004 #1, UC-028 #1 | T | Phase B |
| SYS-011 | UC-003 #1–#3 | T | Phase B |
| SYS-012 | UC-003 #4 | T | Phase B |
| SYS-013 | UC-004 #1–#4 | T | Phase B |
| SYS-014 | UC-005 #1–#5 | T | Phase B |
| SYS-015 | UC-003 #5 | T | Phase B |
| SYS-016 | UC-008 #5 (scratch), UC-015 #3 (audit) | T | Phase B |
| SYS-017 | UC-006 #3 | T | Phase B |
| SYS-018 | UC-006 #1–#2 | T | Phase B |
| SYS-025 | UC-007 #1–#3 | T | Phase B |
| SYS-026 | UC-008 #1–#2 | T | Phase B |
| SYS-027 | UC-008 #3–#4 | T | Phase B |
| SYS-028 | UC-008 #5 | T | Phase B |
| SYS-029 | UC-009 #1–#3 | T | Phase B |
| SYS-030 | UC-009 #4 | T | Phase B |
| SYS-031 | UC-013 #1 | T | Phase B |
| SYS-040 | UC-010 #1 | T | Phase B |
| SYS-041 | UC-010 #2, #5 | T | Phase B |
| SYS-042 | UC-011 #1–#4 | T | Phase B |
| SYS-043 | UC-012 #1–#4 | T | Phase B |
| SYS-044 | UC-013 #1–#3 | T | Phase B |
| SYS-045 | UC-010 #3 | T | Phase B |
| SYS-046 | UC-015 #2–#3 | T | Phase B |
| SYS-047 | UC-015 #1, #4 | T | Phase B |
| SYS-048 *(L)* | UC-029 | T | Phase B (Later) |
| SYS-049 | UC-016 #1, #5 | T | Phase B |
| SYS-050 | UC-010 #4, UC-016 #2–#3, UC-013 #4 | T | Phase B |
| SYS-051 | UC-016 #4 | T | Phase B |
| SYS-052 | UC-028 #1–#2 (para: UC-028 #3, Later) | T | Phase B |
| SYS-053 | UC-033 #1–#5 | T | Phase B |
| SYS-060 | UC-014 #1–#2 | T | Phase B |
| SYS-061 | UC-014 #3–#5 | T | Phase B |
| SYS-062 | UC-014 #6 | T | Phase B |
| SYS-063 *(L)* | UC-032 | T | Phase B (Later) |
| SYS-064 *(L)* | UC-031 | T | Phase B (Later) |
| SYS-070 | UC-017 #2–#3 | T | Phase B |
| SYS-071 | UC-017 #1 | T/A | Phase B |
| SYS-072 | UC-018 #1–#3 | T | Phase B |
| SYS-073 | UC-027 #1–#3 | T | Phase B |
| SYS-074 | UC-017 #2 + UC-025 #1 | T | Phase B |
| SYS-075 *(L)* | UC-030 | T | Phase B (Later) |
| SYS-076 | UC-017 #5 | T | Phase B |
| SYS-077 | UC-035 #1–#3 | T | Phase B |
| SYS-078 *(Later)* | UC-036 #1–#2 | T/D | Phase B |
| SYS-080 *(Later — DEC-013)* | UC-019 #1, #3 | T | Phase B (venue-node gate) |
| SYS-081 | UC-020 #1–#2 | T | Phase B |
| SYS-082 *(Later — DEC-013)* | UC-019 #2 | T | Phase B (venue-node gate) |
| SYS-083 | UC-021 #1–#3 | T | Phase B |
| SYS-084 | UC-020 #3 | T | Phase B |
| SYS-085 | UC-034 #1–#3, #6 | T | Phase B |
| SYS-086 | UC-034 #4–#5 | T | Phase B |
| SYS-087 | UC-034 #7 | T | Phase B |
| SYS-090 | UC-022 #1, #3 | T | Phase B |
| SYS-091 | UC-022 #2, #4 | T/I | Phase B |
| SYS-092 | CI dependency/static scans + ASVS L2 audit checklist per release | A/I | Phase B |
| SYS-093 | UC-019 #4 + TLS config inspection | T/I | Phase B |
| SYS-100 | UC-023 #1 | T | Phase B |
| SYS-101 | UC-024 #1–#2 | T | Phase B |
| SYS-102 | UC-024 #3 | T | Phase B |
| SYS-103 | UC-023 #2–#3 | T | Phase B |
| SYS-104 | Document inspection per release | I | Phase B |
| SYS-105 | Egress-blocked suite (UC-019 #3) + code/config inspection | T/I | Phase B |
| SYS-110 | UC-025 #1–#3 | T | Phase B |
| SYS-111 | UC-025 #1 + UC-018 #3 | T | Phase B |
| SYS-112 | UC-026 #1–#2 | T/I | Phase B |
| SYS-113 | UC-017 #2, UC-026 #1 | T | Phase B |
| SYS-114 | Keyboard-only E2E pass of check-in/result-entry flows | T/D | Phase B |
| SYS-120 | Benchmark on reference dataset (1,500 athletes / 4,000 entries / 250 units) | A | Phase B |
| SYS-121 | Benchmark (seeding 200 entries ≤10 s; recompute ≤5 s) | A | Phase B |
| SYS-122 | Load test (UC-017 #4) | A | Phase B |
| SYS-130 | UC-020 #1 (restart ≤2 min) | T | Phase B |
| SYS-131 | UC-001 #1 (timed quickstart run) | D | Phase B |
| SYS-132 | Support-matrix CI (build/run on documented platforms & browsers) | T/I | Phase B |
| SYS-133 | Dependency inventory inspection (no paid service required) | I | Phase B |
| SYS-140 | Coverage gates in CI (≥90% branch domain logic; ≥80% line overall) | A | TASK-002: `scripts/check-coverage.sh` in `ci.yml` coverage job. *Method note:* Go's cover tooling measures **statement** coverage; the ≥90%/≥80% gates are enforced on statement coverage as the agreed proxy for branch coverage |
| SYS-141 | CI pipeline configuration inspection + gate behaviour test | I/T | TASK-002: `.github/workflows/ci.yml` — 3-OS build+test matrix, golangci-lint v2 (incl. depguard architecture rules), guarded `tsc` job, coverage gates, `go-licenses` allowlist (ADR-001 §4), `govulncheck`; SPDX+DCO in `governance.yml` (TASK-001) |
| SYS-142 | Reference fixture suites for every rule engine (UC-005 #5, UC-010 #5, UC-012 #4, UC-013 #1) | T | Phase B |
| SYS-143 | Release checklist + defect-tracker inspection per release | I | Phase B |
| SYS-144 | Schema/API docs versioned in repo; CI schema-diff check | I/T | Phase B |
| SYS-145 | ADR inventory inspection at each gate | I | Phase B |
| SYS-146 | Repository inspection before first release | I | TASK-001: `LICENSE` (AGPL-3.0-only), `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `GOVERNANCE.md`, CI SPDX+DCO gates (`.github/workflows/governance.yml`); release process pending TASK-028 |

**Coverage check:** all SYS verified; every UC traces to ≥1 SYS (see UC index). Zero orphans
in either direction.

## 3. Baseline QA status (Phase A definition-of-done self-check)

| Check | Status |
|-------|--------|
| Every requirement uniquely identified, atomic, testable | ✅ STR-001…043, SYS-001…146 (blocks), UC-001…033 |
| Zero orphan requirements (both directions) | ✅ §1–§2 above |
| Vague founder language converted to measurable targets | ✅ "well tested/bug free/state of the art" → SYS-140…146, SYS-092, SYS-120…122; no adjectives used as requirements |
| External facts cited; named systems verified by exact spelling | ✅ Seltec (not "setlec") confirmed; Swiss Athletics stack verified; SVM (not "CSI"); see `../research/*.md` |
| Assumptions explicit, open questions consolidated | ✅ `open-questions-and-assumptions.md` (OQ-001…012, A-001…012, TBD-001…009) |
| Scope boundaries stated (MVP / Later / out-of-scope) | ✅ StRS §4 |
| Implementation-free (no stack/scaffolding choices) | ✅ CON-05; verification mechanisms named only where rule- or format-defined (e.g., FinishLynx file family is an external interface, not a stack choice) |
| IDs stable, deprecation-only policy stated | ✅ headers of each document |

Known baseline limitations (tracked, non-blocking): TBD-001…009 in
`open-questions-and-assumptions.md`; Later-priority SYS/UC intentionally not decomposed further
until MVP is ratified.
