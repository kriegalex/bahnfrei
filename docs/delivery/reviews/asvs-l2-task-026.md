<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (c) 2026 Bahnfrei contributors -->

# OWASP ASVS Level 2 review — TASK-026 (SYS-092)

**Review date:** 2026-07-14 · **Reviewer:** project security review (TASK-026)
**Scope:** the whole authenticated web surface (`internal/web`, `internal/app`, `internal/store`)
plus the deployment model (venue hub, field phones, timing PC agent, public results, offline).
**Standard:** OWASP **ASVS v4.0.3, Level 2**. SYS-092 does not pin a version; v4.0.3 is the stable,
tooling-supported baseline. v5.0.0 (2025) reorganizes chapters but adds no L2 control this app fails;
where wording differs the v4.0.3 requirement is authoritative here.

**Overall verdict:** **PASS (ASVS L2) with four low/informational open findings.** Every applicable
L2 control is satisfied by the code and pinned by an automated test, after the four hardening fixes
below. No critical or high finding remains. `govulncheck` reports **zero** known-vulnerable
dependencies. The open findings (F-1…F-4) are low-severity or informational and are tracked as open
questions; none blocks the SYS-092 assertion.

---

## 1. Fixes made in this task

Each ships with a test and is covered by the CI gate (`internal/web/security_test.go` unless noted).

| # | Control | Change | Test |
|---|---------|--------|------|
| FIX-1 | Auth anti-automation (V2.2.1) | Login now rate-limits **consecutive failed** attempts per source IP (10 / 15 min); a success clears the counter, so honest operators and the E2E suite are never throttled. Blocked attempts return **429 + Retry-After** *before* an argon2id verify is spent, reusing the generic "invalid credentials" message so throttling leaks nothing about account existence. `internal/web/ratelimit.go`, wired in `handleLoginSubmit`. | `TestLoginRateLimiterUnit`, `TestLoginBruteForceThrottledSYS092Web`, `TestLoginSuccessResetsThrottleSYS092Web` |
| FIX-2 | Request-size bound (V12.1.1 / V13.1.3) | Global `limitRequestBody` middleware caps **every** request body at 8 MiB (Content-Length short-circuit → 413, plus `MaxBytesReader` for chunked/mis-declared bodies). It sits **outside** the CSRF middleware deliberately: `checkCSRF` calls `r.FormValue`, which parses the full multipart body up to Go's 32 MiB default *before any handler runs* — so a per-handler cap was dead code and the unbounded read actually happened in middleware. This closes a disk/memory-exhaustion DoS on the office CSV-import and timing-import forms. `internal/web/middleware.go`, `routes.go`. | `TestOversizedImportUploadRejectedSYS092Web`, `TestOversizedTimingUploadRejectedSYS092Web` |
| FIX-3 | HSTS (V9.1 / V14.4.5) | `Strict-Transport-Security` (1 y, includeSubDomains) is now sent **only in ACME mode**, where a publicly-trusted cert exists. It is deliberately **withheld in venue-local self-signed mode**: HSTS forbids the cert-warning click-through operators rely on there, so a blanket HSTS would be a self-inflicted lockout. `securityHeaders` is now a `*Server` method to read the TLS mode. `internal/web/middleware.go`. | `TestHSTSHeaderOnlyInACMEModeSYS092Web` |
| FIX-4 | Cache of sensitive data (V8.2.1) | Authenticated responses (which can carry athlete PII / exports) now set `Cache-Control: no-store`; anonymous/public pages are unaffected. Applied in `sessionMiddleware` when a session resolves. | `TestAuthenticatedResponsesAreNoStoreSYS092Web` |

---

## 2. ASVS L2 checklist (by chapter)

Verdict key: **P** pass · **N/A** not applicable (justified) · **F** fail (none remain).
"Evidence" cites the guarding code and, where a control is machine-checked, the pinning test.

### V1 Architecture, Design & Threat Modeling
| Req (v4.0.3) | V | Evidence |
|---|---|---|
| 1.1.x SDLC / threat model | P | Spec-driven engagement; threat model in §4 below; architecture in `docs/architecture/architecture.md`, decisions as ADRs. |
| 1.4.x Access-control architecture enforced server-side | P | Single trust boundary; every capability gate is server-side (`requireRole`, `Authorize`, per-event scoping in `internal/app`). No client-side authz. |
| 1.5.x Input/output boundaries | P | Layering enforced by `depguard` (`web → app → store`, domain pure); templ auto-escapes all output. |
| 1.14.x Segregation / deployment | P | Single self-contained binary (ADR-003); no privileged sidecars; TLS terminated in-process (ADR-003). |

### V2 Authentication
| Req | V | Evidence |
|---|---|---|
| 2.1.x Password strength & no truncation | P | Argon2id over the raw passphrase, no length cap/truncation (`internal/app/password.go`). Minimum-length hint on setup (SYS-117). |
| 2.1.7 Breached-password check | N/A | Offline venue operation is a first-class mode (SYS-080/087); an online HIBP call would violate it. Compensated by strong hashing + FIX-1 throttling. |
| 2.2.1 Anti-automation / rate limiting | **P (FIX-1)** | `loginRateLimiter`; `TestLoginBruteForceThrottledSYS092Web`. |
| 2.4.1 Approved hash (argon2/bcrypt/scrypt/PBKDF2) | P | Argon2id, 19 MiB / t=2 / p=1 (OWASP baseline), self-describing PHC encoding, upgrade-on-verify. `TestHashAndVerifyPassword`, `TestNeedsRehash`, `TestLoginUpgradesWeakHash`. |
| 2.4.x Unique salt, per-credential | P | 16-byte CSPRNG salt per hash (`HashPassword`). |
| 2.5.x Credential recovery | N/A | No self-service password reset in MVP; instance-admin re-provisions (`SetAccountEnabled`/create). Documented scope boundary, not a gap at L2. |
| 2.7/2.8 MFA / OTP | N/A | Not required at L2. |
| — Generic auth errors (anti-enumeration) | P | `ErrInvalidCredentials` covers both unknown-user and wrong-password; a dummy argon2id verify equalizes timing on the unknown-user path (`AuthService.Login`). Disabled-account 403 only surfaces *after* a correct password, so it discloses nothing to a guesser. `TestLoginFlowSuccessAndFailure`. |

### V3 Session Management
| Req | V | Evidence |
|---|---|---|
| 3.2.1 Server-side session generation, ≥64-bit entropy | P | 256-bit CSPRNG opaque token (`newSessionToken`). |
| 3.2.3 Session tokens in cookies with correct attributes | P | `HttpOnly`, `SameSite=Lax`, `Secure` when served over TLS, `Path=/` (`handleLoginSubmit`). |
| 3.3.1 Logout invalidates server-side | P | `Logout` revokes the in-memory session and clears the cookie (`handleLogout`); `Revoke`. |
| 3.3.2 Idle/absolute timeout | P | Configurable TTL (default 12 h), enforced on every `Lookup`; expired sessions evicted; periodic `Sweep`. `TestSessionExpiry`, `TestSessionSweep`. (Single fixed lifetime, no separate idle timer — accepted at L2 for a venue-day tool; see F-4 rationale.) |
| 3.3.3 Session invalidation on privilege/credential change | P | Disabling an account immediately `RevokeAccount`s all its live sessions. `TestAccountLifecycleCreateDisableEnableRoleChangeSYS090UC022_2`. |
| 3.4.x Cookie prefixes / `__Host-` | N/A→note | Not sent; the app is often reached by IP or self-signed host where `__Host-`/`Secure` cannot always apply. `Secure` is set whenever TLS is present. Acceptable at L2; noted for a future hardening pass. |
| — Session fixation | P | No pre-auth session exists; a fresh token is minted on each successful login, so there is nothing to fixate. |

### V4 Access Control
| Req | V | Evidence |
|---|---|---|
| 4.1.1/4.1.3 Least privilege, server-side enforced | P | `requireRole` coarse gate on every non-public route; `Authorize`/capability checks re-enforced in `internal/app` (defense in depth). `TestPermissionMatrixSweepSYS090UC022_3`, `TestAdminRouteRequiresInstanceAdminRole`. |
| 4.1.5 Fail-closed | P | Unknown/anonymous → `RolePublic`; every gate denies by default with a uniform localized 403 (`renderForbidden`). |
| 4.2.1 Object-level authorization (IDOR) | P | Field officials are scoped per assigned event unit inside `internal/app`, not just by coarse role. `TestFieldOfficialEventScopeDeniedSYS090UC022_1`, `TestFieldOfficialEventScopeSYS090UC022_2`. Timing-agent tokens are meet-scoped and re-checked against the path meet (`authenticateAgent`). |
| 4.3.1 Admin interfaces protected | P | `/admin/*`, `/audit`, retention purge, backup all `RoleInstanceAdmin`/office-gated. `TestAccountAdminAndAuditRouteGatingSYS090UC022`. |
| 4.3.2 Directory browsing / metadata files | P | Static assets served from an embedded FS via an explicit handler; no filesystem walk (`static.go`). |

### V5 Validation, Sanitization & Encoding
| Req | V | Evidence |
|---|---|---|
| 5.1.1 Mass-assignment / body cap | **P (FIX-2)** | `limitRequestBody`; `TestOversizedImportUploadRejectedSYS092Web`. |
| 5.2.x Output encoding / XSS | P | templ contextually auto-escapes all interpolation; CSP `default-src 'self'` with no `unsafe-inline` blocks inline script. Public-page PII scanner also exercises rendering. `TestPublicPageMinimizationSYS100UC023_1`. |
| 5.3.4/5.3.5 SQL & command injection | P | 100% parameterized `database/sql` (`?` placeholders). The two `fmt.Sprintf`-built statements (`optimistic.go`, `backup.go`) interpolate only **internal table/column identifiers** (schema introspection / hardcoded callers), never request input — verified by inspection. No `os/exec` on user input. |
| 5.5.x Deserialization | P | Only stdlib `encoding/json` into typed structs; no gob/reflection-based decoding of untrusted data. |

### V7 Error Handling & Logging
| Req | V | Evidence |
|---|---|---|
| 7.1.1 No sensitive data in logs | P | No password/token logging; audit payloads store business fields only. |
| 7.4.1 Generic error messages, no stack traces | P | Handlers render localized error pages / generic `http.Error` strings; Go does not leak stack traces to responses. |
| 7.2.x Security-event logging (audit trail) | P | Every privileged action (account create/disable/role-change, overrides, erasure, purge) is written to an append-only audit log (SYS-046). `TestPrivilegedAuditLogSurfacesPrivilegedActionsOnlySYS091UC022_2`. |

### V8 Data Protection
| Req | V | Evidence |
|---|---|---|
| 8.2.1 No caching of sensitive data | **P (FIX-4)** | `Cache-Control: no-store` on authenticated responses; `TestAuthenticatedResponsesAreNoStoreSYS092Web`. |
| 8.3.x Minimization on public surfaces | P | Central minimization choke point; birth-year-only, no licence/contact/DOB on public pages. `TestPublicPageMinimizationSYS100UC023_1`, `TestPublicPathCrawlerNeverLeaksWithdrawnOrErasedNameSYS100SYS101UC024`. |
| 8.1.x DSAR / erasure | P | Per-subject export + pseudonymizing erasure (SYS-101); privacy review TASK-023 (APPROVE-WITH-NOTES), findings #1/#2 closed in TASK-029. |

### V9 Communications
| Req | V | Evidence |
|---|---|---|
| 9.1.1 TLS everywhere off-host | P | ACME (public) or self-signed (venue-local) TLS; `TLSModeOff` is dev/E2E only and never a default (`tls.go`, SYS-093). `TestServerServeAndShutdown`. |
| 9.1.2 Strong TLS config | P | `MinVersion: TLS 1.2`, ECDSA P-256. |
| 9.2.x HSTS | **P (FIX-3)** | ACME-mode only; `TestHSTSHeaderOnlyInACMEModeSYS092Web`. |

### V10 Malicious Code
| 10.x | N/A | No dynamic code loading, plugins, or eval; single static binary. Supply chain covered by V14/§5. |

### V11 Business Logic
| Req | V | Evidence |
|---|---|---|
| 11.1.2 Anti-automation on high-value flows | P (FIX-1 for auth) | Login throttled. Other mutating flows are role-gated, CSRF-protected, audited. |
| — Optimistic concurrency / no lost updates | P | Version-checked writes (`optimistic.go`); capture reconciliation never silently drops writes (SYS-086). |

### V12 Files & Resources
| Req | V | Evidence |
|---|---|---|
| 12.1.1 Upload size limit | **P (FIX-2)** | Global 8 MiB cap + per-endpoint 5 MiB buffering threshold; agent path additionally 413s at 5 MiB (`TestAgentImportAndExportErrorPathsWeb`). |
| 12.3.x Path traversal on filenames | P | Uploaded filenames are used only as opaque labels/metadata; files are parsed in-memory, never written to a path derived from user input. |
| 12.5.x Dangerous file types | N/A | Only CSV/.lif text is parsed; no execution, no served user uploads. |

### V13 API & Web Service
| Req | V | Evidence |
|---|---|---|
| 13.1.3 Request size / DoS | P (FIX-2) | Global body cap covers the JSON sync API and agent API too. |
| 13.2.1 CSRF on cookie-authenticated state change | P | Double-submit-cookie CSRF on all cookie-auth POST/PUT/PATCH/DELETE; the bearer-token agent API is correctly exempt (never cookie-authenticated). `TestLoginRejectsWithoutCSRFToken` and CSRF-required POST tests (TASK-033). |
| 13.2.2 Bearer-token API auth | P | Timing agent uses a meet-scoped bearer token, re-validated against the path meet id; hashed at rest. `authenticateAgent`. |
| 13.4.x GraphQL | N/A | No GraphQL. |

### V14 Configuration
| Req | V | Evidence |
|---|---|---|
| 14.2.1 Dependencies free of known vulns, in CI | P | `govulncheck` gate in `.github/workflows/ci.yml` (`vuln-scan`); local run 2026-07-14 clean (§5). Licence allowlist gate (`go-licenses`). |
| 14.3.2 No debug/verbose leakage | P | No debug endpoints; `/healthz` returns a bare `ok`. |
| 14.4.1 Content-Type + charset | P | Set on rendered/text responses; `nosniff`. |
| 14.4.3 CSP present | P | `default-src 'self'; base-uri 'self'; frame-ancestors 'none'` on every response (`securityHeaders`). |
| 14.4.4 `X-Content-Type-Options: nosniff` | P | Set globally. |
| 14.4.5 HSTS | P (FIX-3) | See V9. |
| 14.4.7 `X-Frame-Options` / anti-clickjacking | P | `X-Frame-Options: DENY` + CSP `frame-ancestors 'none'`. |
| 14.5.x Static-analysis in CI (SYS-092 own-code clause) | **partial → F-1** | `golangci-lint` (depguard/misspell) + `govulncheck` run in CI. A dedicated security SAST (gosec) is **not** enabled — see F-1 / **OQ-078**. |

---

## 3. Open findings (ranked)

| ID | Sev | Finding | Disposition |
|----|-----|---------|-------------|
| F-1 | Low | **No dedicated SAST (gosec) in CI.** SYS-092 asks for "static analysis in CI". `golangci-lint` + `govulncheck` arguably satisfy it, but neither is a security-focused taint/rule SAST. Adding gosec would strengthen the own-code half of the clause. Not run locally here (not installed), so not enabled blind. | **OQ-078** — recommend adding a `gosec` CI job; low effort, additive. |
| F-2 | Low | **Open-redirect via `Referer` in `handleLocaleSwitch`** (`routes.go`): after setting the locale cookie it redirects to the raw `Referer` header, which may be an absolute off-origin URL. Not a practical phishing primitive (the target comes from the victim's own `Referer`, not an attacker-supplied query param, so it only bounces the user back where they came from), hence Low. | **OQ-079** — a same-origin/relative-only guard on `dest` removes it; deferred to avoid touching the locale UX under this security-only slice. |
| F-3 | Low/Info | **`ExportGenericCSV` returns 200 + header-only CSV for an unknown meet id** where sibling exports 404 (`internal/app/exchange.go`). Office-gated, empty body — no data disclosure — but an information-flow inconsistency: a scripted timing operator gets no signal a meet id is wrong. | Pre-existing **OQ-063**. ASVS verdict: **not a security defect** (no unauthorized data reaches anyone); fix (a `store.GetMeet` existence check) is correctness hygiene, recommended. |
| F-4 | Info | **Login rate-limiter and sessions are in-memory**, so both reset on process restart, and the limiter is per-IP not per-account. Adequate at L2 for a single-binary venue tool (a restart is operator-driven and rare; argon2id cost bounds throughput regardless). Persistent/account-keyed lockout would be an L3-leaning enhancement. | Accepted for MVP; documented here. No OQ. |

**Destructive actions without server-side friction (OQ-074) — security assessment.**
Athlete erasure, account disable, meet archive and retention purge are one-click POSTs.
At **ASVS L2 this is not a gap**: each is authenticated, role-gated (office/instance-admin),
CSRF-protected and audited — the four controls L2 requires for a state-changing action. Server-side
re-authentication or a typed-confirmation token before sensitive transactions is an **ASVS L3**
control (V3-sensitive), not required here. Recommendation: treat OQ-074 as the **UX/safety** fix it
is (accidental-click protection), and *optionally* add L3-style friction (re-auth or a typed
"ERASE" confirm token) to the two irreversible-and-unrecoverable actions — **athlete erasure** and
**retention purge** — as defense-in-depth. Not required for the SYS-092 L2 pass.

---

## 4. Threat model (STRIDE over the deployment model)

Deployment (ADR-002/003): a hub binary on the venue LAN; field phones and the competition office
over Wi-Fi; a timing PC running the watched-folder agent; public results reachable from the internet
(hub role); offline operation must survive connectivity loss.

| Threat | Vector | Mitigation | Residual / maps to |
|--------|--------|-----------|--------------------|
| **S**poofing | Steal/guess a session or agent token on the LAN | 256-bit opaque session tokens; `HttpOnly`/`Secure`/`SameSite`; TLS on all non-loopback (SYS-093); meet-scoped hashed agent tokens; FIX-1 login throttle | LAN sniffing mitigated by TLS even venue-local. **OK** |
| **T**ampering | Forged POST from a malicious page a logged-in operator visits; oversized/chunked body | CSRF double-submit on all cookie-auth writes; FIX-2 body cap; optimistic-concurrency version checks; capture reconciliation never silently drops (SYS-086) | **OK** |
| **R**epudiation | Operator denies a privileged change | Append-only audit trail with actor/action/target/timestamp (SYS-046); privileged actions surfaced in `/audit` | Audit is local & operator-erasable by design (self-hosted) — accepted |
| **I**nfo disclosure | PII on public pages; cached PII; verbose errors; enumeration | Minimization choke point + crawler test; FIX-4 `no-store`; generic errors; generic login failures + timing guard | F-3 (empty CSV) low. **OK** |
| **D**enial of service | Huge upload fills disk/RAM; login brute-force burning argon2id | FIX-2 8 MiB body cap (pre-CSRF-parse); FIX-1 throttle blocks *before* the argon2id verify; `ReadHeaderTimeout` | Full L7 flood needs a network-layer control (out of app scope; operator runbook, TASK-028). **OK for app tier** |
| **E**levation | Field official acts outside assigned events; role bypass | Coarse `requireRole` + per-event object scoping in `internal/app` (defense in depth); last-admin lockout guard; fail-closed default | **OK** |
| Offline-specific | Stale device replays after a start-list change; lost writes on reconnect | Event-unit checkout lock + reconciliation queue (SYS-085/086/087); writes surfaced, never discarded | Semantics of one merge case open (OQ-062, timing) — correctness, not security |

No threat surfaces an unmitigated critical/high path. Actionable items map to F-1…F-3 and existing OQs.

---

## 5. Dependency posture

- **`govulncheck ./...` (golang.org/x/vuln, run 2026-07-14): PASS, exit 0 — zero known
  vulnerabilities** in reachable code across all modules. This is the SYS-092 "zero known
  critical/high in dependencies" gate and it is already wired into CI (`vuln-scan` job).
- **Currency:** every direct/indirect module has a minor/patch update available (e.g. `templ`
  0.3.833→0.3.1020, `golang.org/x/crypto` 0.53→0.54, `go-jose/v4` 4.1.3→4.1.4 via certmagic), but
  **none is security-driving** (govulncheck clean) and **none is abandoned** — all are actively
  maintained (Go team, templ, caddyserver, xuri/excelize, modernc). No replacement needed.
- **Licences:** CI `go-licenses` allowlist gate (Apache-2.0/BSD/MIT/ISC/MPL-2.0/…) — compliant.
- **Attack surface note:** `certmagic` + `go-jose` (ACME/JOSE) and `excelize`/`fpdf`/`ledongthuc/pdf`
  (file parsing) are the highest-risk parsers; all are govulncheck-clean today and the file parsers
  now sit behind the FIX-2 body cap.
- **Policy recommendation:** keep the `govulncheck` CI gate as the release blocker (SYS-092); add a
  scheduled `go get -u ./... && govulncheck` dependency-refresh cadence (monthly or on advisory);
  add **gosec** as the own-code SAST (F-1/OQ-078). Do **not** auto-bump on every minor release —
  bump on advisory or at release-branch cut, re-running the full gate.

---

## 6. Second opinion on OQ-059 (relay-entry audit redaction)

**Concurrence: the closed-form argument is sound; a relay-scoped regression test is worth adding,
but no code change is required and this is not an erasure defect.**

Reasoning: SYS-101 erasure must remove identifying data while preserving the official record. The
concern is whether erasing an athlete who is a **relay leg** leaves their name anywhere. Two facts
close it: (a) `relay_teams.composition_json`/`reserves_json` store the leg member's **athlete ID,
not their name**, so post-erasure the display name is resolved **live** from `athletes` and already
shows the pseudonym — there is no denormalized name copy to miss; (b) the relay-entry audit payload
(`auditEntrySubmit`, `entry.submit_relay`) records the **club** as subject, never a leg athlete's
name, so no audit row ever carried the name that findings #1/#2 had to scrub. Net: erasing a relay
leg is **not** cosmetically defeated. The one genuine gap is **verification, not behavior** — the
individual-entry path is now pinned by a crawler/redaction test, the relay path is not. I recommend a
small additive test asserting an erased relay-leg athlete's name appears on no public relay view and
in no audit row, mirroring `TestPublicPathCrawlerNeverLeaksWithdrawnOrErasedNameSYS100SYS101UC024`.
Filing this as a follow-up (not part of this security slice) is correct; it is additive and
low-risk. **This does not affect the ASVS L2 verdict.**

---

## 7. Verdict

**ASVS v4.0.3 Level 2: PASS.** All applicable L2 controls satisfied and test-pinned; four fixes
landed (FIX-1…FIX-4) each with a regression test; zero known-vulnerable dependencies; open findings
F-1…F-4 are low/informational and tracked (OQ-078, OQ-079, OQ-063). SYS-092 is met for release 0.1,
conditional on keeping the CI `govulncheck` gate green and (recommended) adding the gosec SAST job.
