# Privacy review — TASK-023 (nFADP/GDPR gate)

**Review date:** 2026-07-13 · **Reviewer:** Opus-tier privacy review agent (CLAUDE.md guardrail 7)
**Scope:** TASK-023 branch diff vs `main` (merged as `41cfbd9`) — SYS-100–103, UC-023/UC-024
**Verdict:** **APPROVE-WITH-NOTES** — merged; findings #1/#2 tracked as **TASK-029**.

## Findings

| # | Severity | Finding | Disposition |
|---|---|---|---|
| 1 | should-fix | Erasure leaves the subject's cleartext name in older audit payloads (`participant.register` in `internal/app/results.go`; `entry.submit` in `internal/app/entry.go`) — `EraseAthlete` never redacts them, so an erased person's name persists in `audit_log`. | TASK-029 — done |
| 2 | should-fix | Retention purge redacts `participant`/`result`/`athlete` audit rows but omits `entity_type = "entry"` — online-entry audit names are never purged. | TASK-029 — done |
| 3 | note | `EraseAthlete` is pseudonymization, not anonymization (keeps BirthYear/Sex/Nationality/ClubIDs + result linkage) — explicitly permitted by SYS-101; consider a future full-delete mode for non-record meets and document the framing in operator privacy docs (SYS-104). | TASK-028 (M3 — in scope of the operator privacy docs); full-delete mode stays Later |
| 4 | note | Minimization choke point is discipline-enforced, not type-enforced — a future public renderer could bypass it; consider a crawler-style regression asserting a withdrawn athlete's name appears on no public path. | TASK-029 — done |
| 5 | note | Withdrawn (not erased) athlete's public row keeps bib + birth year — accepted residual risk (OQ-043), fine for MVP. | Accepted |
| 6 | — | Consent semantics (opt-out publication, uniform withdrawal enforcement, age-18 display threshold, no fabricated consent on import) assessed sound. | — |
| 7 | note | 90-day retention default documented and configurable; `RedactAuditPII` trigger-drop/restore mechanism transactional and low-abuse-potential. | Accepted |

Erasure-unmatchability (re-import cannot resurface an erased athlete) verified by construction; pinning test suggested (TASK-029).
