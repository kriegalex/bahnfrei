---
name: verify
description: Build, launch and drive a real bahnfrei server for runtime verification of web-surface changes.
---

# Verifying bahnfrei changes against the running server

## Build & launch

```bash
go build -o /tmp/bahnfrei-verify ./cmd/bahnfrei
/tmp/bahnfrei-verify serve --addr 127.0.0.1:<port> --tls-mode off --data-dir <fresh-tmp-dir>
# ready when GET /healthz returns 200 (usually <1s)
```

`--tls-mode off` is the dev/E2E mode (plaintext HTTP, loopback is a secure
context so service workers still register). A fresh `--data-dir` gives a
fresh SQLite database, so the first visit lands on `/setup`.

## Seeding through the real operator forms

Same recipe as `e2e/helpers/seed.ts` (all POSTs need the `csrf_token`
hidden field scraped from the corresponding GET form, and a session
cookie jar):

1. `POST /setup` — username/display_name/password (first-run only).
2. `POST /login` — username/password.
3. `POST /meets/from-template` — `template=ubs-kids-cup&date=…&venue=…`;
   the 303 Location is `/meets/{meetID}`.
4. `POST /meets/{id}/roster` — first_name/last_name/birth_year/sex/bib.
5. `POST /meets/{id}/timetable/publish` — required before the public
   timetable page exists (draft/published per SYS-004; unpublished → 404).
6. Unit IDs: scrape `GET /meets/{id}/capture` links; athlete IDs: scrape
   `name="athlete" value="…"` on the unit page.
7. Capture a mark: `POST /meets/{id}/capture/{unit}/attempt` —
   athlete/seq/value/version(=0 for new)/csrf_token.
8. Meet edit round-trip: `GET /meets/{id}/edit`, re-submit every input
   value AND every select's selected option (missing fields → 422).

## Public surface (anonymous — no session cookie)

- Pages: `/m/{id}`, `/m/{id}/timetable`, `/m/{id}/startlists`,
  `/m/{id}/results`; live fragment `/m/{id}/results/live`.
- SSE: `GET /events/meet-{id}` (no auth); a saved result publishes
  `event: results` synchronously.
- Locale: `GET /locale?lang=fr` sets the cookie; assert exact strings
  from `internal/web/i18n/locales/{de,fr}.json`.

## Gotchas

- POST to a GET-only route returns **403** (CSRF middleware answers
  before the mux would 405) — expected, not a bug.
- E2E admin fixture: `admin` / `s3cret-passphrase` (see seed.ts).
- A worked example driver: see the TASK-010 verification script pattern
  (python stdlib urllib + cookiejar + regex-scraped CSRF).
