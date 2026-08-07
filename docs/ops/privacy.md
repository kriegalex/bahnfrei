# Operator privacy documentation

**Audience:** the club or federation running a bahnfrei instance, acting as the nFADP/GDPR
**controller** of its athletes' and officials' data
(see [ADR-002](../architecture/adr/ADR-002-deployment-and-application-model.md): self-hosting
clubs are their own controller — the project runs no subscription-SaaS business and has no
access to your instance's data).

This page has three parts: a data-processing overview, a template privacy notice you adapt for
your meet, and controller guidance. It is written in English with the
template text clearly marked for translation/adaptation — the operating audience is
German/French-speaking Swiss clubs, but this document itself ships in English for 0.1 (DE/FR
UI strings are handled separately in the application itself; this is delivery documentation, not
an in-app surface).

## 1. Data-processing overview

### 1.1 What is stored, where

Everything lives in the single SQLite database file in your instance's data directory (no
external service call carries personal data) plus a locally cached TLS certificate. The
categories of personal data it holds:

| Data | Fields | Who's in it | Purpose / lawful basis |
|---|---|---|---|
| Athlete identity | First/last name, full birth date (internal only — see §1.2), birth year, sex, nationality, club affiliation, licence/external IDs | Every entered athlete | Performance of the competition contract with the entrant/club (category eligibility, results attribution) |
| Publication consent | Results-publication withdrawal flag, photo-consent flag, extended-data-consent flag, who recorded it and when | Every athlete (esp. minors) | Consent (Art. 6(1)(a) GDPR / Art. 6 nFADP) — controls what appears on public surfaces (§1.3) |
| Entries & results | Seed marks, captured attempts/times, placings, status codes, corrections with audit trail | Every entered athlete | Contract performance; legitimate interest in an accurate, auditable competition record |
| Accounts | Username, display name, Argon2id password hash, role, enabled/disabled | Operators (admin, organizer, office, field official) and online-entry submitters | Contract performance (running the meet); the operator is the data subject here, not an athlete |
| Audit log | Actor, action, target, timestamp, before/after values, reason for corrections/overrides | Every privileged action taken by an operator | Legal obligation / legitimate interest — accountability and dispute resolution |

**Not currently collected:** dedicated contact fields (email, phone) on athlete or club records —
there is nothing to purge there yet. If a future release adds such a field, this table and the
retention policy below extend to cover it.

### 1.2 What never appears publicly

Public surfaces and exports (the meet's public results/timetable/startlist pages, any published
export) expose **at most**: name, club, nationality (where competition-relevant), category, bib,
and competition data (marks, placings, record flags). They **never** expose: full birth dates
(only the category-implied birth year), licence numbers, contact data, or consent flags. This is
enforced at the read path in the code, not by convention, and is covered by the project's
automated tests.

### 1.3 Consent enforcement

A withdrawn `results_publication_withdrawn` flag suppresses that athlete's row on public result
pages automatically while preserving the full internal official result — the suppression is a
rendering-time filter, not a data deletion, so nothing about the sporting record is lost. Consent
is recorded per athlete, with who recorded it and when in the audit trail (purged on the same
retention schedule as other audit PII, §1.4).

### 1.4 Retention

Data not needed for the permanent sporting record becomes purgeable automatically **90 days**
(default; configurable with `--retention-days` on `bahnfrei serve`) after the last meet an athlete
participated in ends. What that purge actually clears, as implemented:

- The consent-recorder identity (`RecordedBy`) on each athlete's consent record.
- Any full birth date already redacted (birth **year** is retained — it is category-defining data
  needed for the permanent record, not incidental contact/consent metadata).
- Personal-identifying names in older audit-log payloads (`RedactAuditPII`), for the
  `participant`, `result`, `entry`, and `athlete` entity types.

Trigger a purge manually (instance-admin, `/admin` → retention) or let it run automatically at
server startup. It is transactional and only ever *drops* PII that has aged out — it never touches
the sporting record itself (marks, placings, categories, record flags all survive unchanged).

### 1.5 Erasure — pseudonymization, not anonymization

An operator can additionally erase one athlete's identity outright (instance-admin/office level,
"erase" action). **This is important to understand correctly: it is pseudonymization, not full
anonymization.**

What erasure does:

- Replaces the athlete's name with a neutral marker (`Athlete-<id-suffix>`) — never their real
  name again, anywhere the system renders it, including older audit-log payloads.
- Clears the full birth date and all external IDs (licence numbers).
- Marks the record `anonymized` so a repeat erasure request is refused rather than silently
  re-processing.

What erasure **deliberately keeps**, because erasure is designed to *"preserve the integrity
of official competition results"*:

- Birth **year**, sex, nationality, and club affiliation — the category-defining and
  results-attribution data a sporting record needs to remain meaningful and auditable (e.g. for
  federation record verification) even after the person's name is gone.
- The link between the pseudonymized athlete record and their captured results — an erased
  athlete's marks, placings, and record flags are unchanged; only who they belong to (by name) is
  hidden.
- Re-import cannot resurface the erased identity (verified by construction): the pseudonym does
  not get overwritten by a later re-import of the same person under the same source data.

**Why this distinction matters for you as controller.** If a data subject exercises a
right-to-erasure request and expects their sporting history to disappear entirely, explain that
what this system offers is *identity* erasure with the competition record preserved in
pseudonymized form — not full deletion of the fact that a performance occurred. This is a
deliberate design trade-off, not a limitation nobody noticed: full
sporting records having documented dates/places/results is a long-standing convention this system
does not override. A **full-delete mode** (removing pseudonymized records entirely, e.g. for a
non-record club meet where no one has a preservation interest) is not built for 0.1 — it stays a
*Later* item; there is currently no workaround for a subject who insists on it beyond a manual
database edit outside the application (which you'd be doing as controller, on your own
infrastructure, not something the project can do for you).

## 2. Template privacy notice for meets

*(Template text — adapt the bracketed placeholders and any club-specific detail; everything else
is a description of what this software does, so keep it accurate to your actual configuration if
you change retention settings or hosting.)*

> **Privacy notice — [Meet name], [date(s)], [venue]**
>
> **Controller.** [Club/organizer name], [contact address/email]. We use the self-hosted, open
> source Bahnfrei system to run this meet; no third party operates or has access to this data.
>
> **What we collect.** For every entered athlete: name, birth year (and full birth date
> internally, for category verification — never shown publicly), sex, nationality, club, and
> licence number where applicable; entry and competition-day results; a publication-consent flag.
> For online-entry submitters and meet officials: an account with a username and password.
>
> **Why.** To run the competition — check-in, seeding, results, category eligibility, official
> reporting to [Swiss Athletics / your federation] — and to keep an auditable record of the
> results.
>
> **What is public.** Live and archived results pages show name, club, nationality (where
> relevant), category, bib, and competition results. They never show full birth dates, licence
> numbers, or contact data.
>
> **Consent, esp. for minors.** [Parent/guardian/athlete] may withdraw consent for public
> result-page display at any time by contacting [contact]; withdrawal is applied immediately and
> does not affect the official internal result.
>
> **Retention.** Contact/consent-recording metadata and full birth dates become purgeable
> [90 days, or your configured value] after the meet ends. Competition results and the
> category-defining minimum (birth year, sex, nationality, club) are kept as the permanent
> sporting record.
>
> **Your rights.** You may request a copy of everything we hold about you (subject-access export),
> correction, or erasure of your identity (see §1.5 above for what erasure does and does not
> remove) by contacting [contact]. We aim to respond within [your jurisdiction's statutory window].
>
> **No third-party sharing** beyond what's needed to submit official results to [your federation],
> where applicable.

## 3. Controller guidance

You, the operator, are the nFADP/GDPR **controller** for every instance you self-host — Bahnfrei
the project is not a processor and has no access to your data (see
[ADR-002](../architecture/adr/ADR-002-deployment-and-application-model.md)). Practical
implications:

1. **Publish a privacy notice** (adapt §2 above) somewhere entrants and officials can read it
   before submitting data — an entry form footer, a printed sheet at check-in, or your club's own
   site.
2. **Handle subject-access, rectification, and erasure requests yourself**, using the mechanisms
   in §1.4/§1.5. There is no project-run support desk for this — it is your legal obligation as
   controller.
3. **Set a retention period that fits your context** (`--retention-days`); the 90-day default is
   documented and privacy-protective but not mandatory. If your federation's results-challenge
   window is longer, extend it accordingly.
4. **Understand the erasure limitation before promising it to anyone** (§1.5) — do not tell a data
   subject their record will disappear entirely if what you can actually offer is pseudonymization.
5. **Secure your instance and its backups** — the database file is the entirety of your personal
   data footprint; treat backup artifacts (`docs/ops/operator-runbook.md` §4) with the same care
   as the live database (they are unencrypted SQLite content).
6. **No telemetry, no external calls with personal data** — nothing you enter leaves
   your infrastructure unless you configure ACME (which only ever sees your domain name, not
   athlete data) or manually export/share a file yourself.
