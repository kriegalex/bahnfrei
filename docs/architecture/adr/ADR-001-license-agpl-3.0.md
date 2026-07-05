# ADR-001 — Licence: AGPL-3.0-only, with DCO

**Status:** **Accepted** — ratified by the founder 2026-07-05 (one-way door; softening annex reviewed, kept as proposed)
**Date:** 2026-07-05
**Traces:** STR-036, SYS-146, CON-02, DEC-004

## Context

The founder wants a licence that "allows adoption and improvement by clubs, but not for a
private company to easily make money off of this project" (DEC-004). No OSI-approved licence
can prohibit commercial use outright (that would violate the Open Source Definition); the
strongest available lever is **strong copyleft with a network clause**, which makes
proprietary capture unattractive: anyone offering the software as a service must publish
their modifications.

## Decision

1. Licence the entire codebase **AGPL-3.0-only** (not "or-later": licence evolution stays a
   founder decision).
2. Contributions accepted under the **Developer Certificate of Origin (DCO)** (`Signed-off-by`),
   **no CLA**. A CLA that lets one party relicense would undermine the anti-capture intent and
   deters volunteer club contributors; the cost is that any future relicensing needs consent of
   all contributors — accepted deliberately as a commitment device.
3. Every source file carries an SPDX header (`AGPL-3.0-only`); a CI check enforces it.
4. Dependency licence policy: only AGPL-compatible dependencies (permissive MIT/BSD/Apache-2.0
   preferred); enforced by an automated licence scan in CI (feeds SYS-141).
5. Project name and logo (once chosen, DEC-009) are held by the founder and are **not** covered
   by the code licence — the practical brake on confusing commercial forks.
6. Documentation is licensed CC-BY-SA-4.0.

## Alternatives considered

| Option | Why rejected |
|--------|--------------|
| Apache-2.0 / MIT (permissive) | Directly contradicts DEC-004: a company could ship a closed SaaS on top |
| GPL-3.0 (without network clause) | The SaaS loophole is exactly the "easy money" path the founder wants closed |
| SSPL / BSL / fair-source | Not OSI-approved; would break STR-036 ("OSI-approved") and repel club/community trust |
| AGPL + CLA with relicensing rights | Concentrates capture power in one entity; deters contributors; unnecessary for a solo-founder community project |

## Consequences

- Clubs, federations, and hosting volunteers can use, modify, and host freely; anyone hosting a
  modified version must offer its source to users — the club ecosystem keeps all improvements.
- Some companies refuse AGPL dependencies entirely; that limits *embedding* of our code in
  commercial products — intended, not accidental. Interchange **formats and schemas** remain
  openly documented (SYS-144), so data interop never requires touching AGPL code.
- Timing-vendor or federation integrations happen across process/file boundaries (files, HTTP),
  which AGPL does not encumber.
- Relicensing later is effectively impossible without contributor consent — the founder should
  ratify this ADR only if comfortable with that permanence.

## Annex A — Softening options (ratification support, 2026-07-05)

"Softening" decomposes into **two independent dials**. Pick one position on each.

### Dial 1 — Copyleft strength (what companies may do with the code)

| Licence | OSI-approved (STR-036) | Company can run a **closed SaaS**? | Company can **embed in a proprietary product**? | Hosted improvements come back to the community? | Swiss/EU fit | Net effect vs. DEC-004 |
|---|---|---|---|---|---|---|
| **AGPL-3.0-only** *(proposed)* | Yes | No — network use triggers source-sharing | No | Yes | Good | Fully closes the "easy money" path |
| **EUPL-1.2** | Yes | No — its copyleft covers "Communication", incl. providing the software over a network (Art. 5, EUPL-1.2 text, [joinup.ec.europa.eu](https://joinup.ec.europa.eu/collection/eupl)) | Mostly no, **but** the compatibility clause lets combined works be re-released under listed licences (GPL-3.0, MPL-2.0, …) — a controlled leak of the network clause | Yes, with the leakage caveat | Excellent — legally binding official DE/FR/IT/EN texts, drafted for EU/Swiss-style law | The only genuine "AGPL-light": softer edges, SaaS door stays shut |
| **GPL-3.0-only** | Yes | **Yes** — the SaaS loophole is open | No (on distribution) | Only when binaries are distributed | Good | Reopens exactly what DEC-004 asked to close |
| **MPL-2.0** | Yes | Yes | Yes — proprietary code may link/surround; only *modified MPL files* stay open | File-level only | OK | Contributor-friendly middle ground; weak anti-capture |
| **Apache-2.0 / MIT** | Yes | Yes | Yes | Voluntary only | OK | Maximum adoption incl. corporate; abandons DEC-004 |
| **FSL / BSL (delayed-open)** | **No** | No (until the delay expires) | No | n/a | — | Breaks STR-036; and it is *harder* on companies, not softer — listed only for completeness |

### Dial 2 — Governance flexibility (how locked the licence itself is)

| Regime | Can the licence change later? | Dual-licensing revenue possible? | Contributor friction / trust cost |
|---|---|---|---|
| **DCO, "-only"** *(proposed)* | Practically never (all-contributor consent) | No | Lowest friction; strongest anti-capture credibility |
| **DCO, "-or-later"** | Automatic upgrade path to future FSF-published versions only | No | Low; delegates licence evolution to the FSF instead of freezing it |
| **CLA to the founder** | Founder can relicense / dual-license any time | Yes | Highest: deters volunteers, and the founder *becomes* the potential capturing company |
| **Fiduciary agreement to a neutral steward** (e.g. FSFE Fiduciary Licence Agreement held by a Swiss *Verein*) | Via the steward, with charter safeguards | Steward-controlled | Medium; a Verein fits the Swiss club context, but is governance overhead a solo founder doesn't need yet |

### Reading the matrix

- Every Dial-1 step below EUPL-1.2 reopens the closed-SaaS path — that is not "softening",
  it is reversing DEC-004. **EUPL-1.2 is the one real softening candidate**: same practical
  effect on SaaS capture, more permissive at the edges, and natively multilingual/EU-flavoured.
- If the discomfort is the *permanence* rather than the copyleft, soften Dial 2 instead
  (e.g. AGPL-3.0-**or-later** + DCO) and leave Dial 1 alone.
- The trademark lever (name/logo held by the founder, Decision §5) is a third, licence-independent
  softener/hardener and stays available under every row above.
