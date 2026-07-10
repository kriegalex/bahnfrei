# M1 Demo Script — UBS Kids Cup Club Visit (TASK-015, DEC-011)

**Purpose.** Founder-facing, step-by-step walkthrough of the M1 milestone demo: a complete
UBS Kids Cup meet on one laptop, phones capturing offline-tolerantly, live public results,
printed lists, and the official series upload file. Each step names the screen/URL and the
requirement it demonstrates.

**Duration.** ~25 minutes. **Kit.** Laptop (the "hub"), a Wi-Fi hotspot the laptop and
phones share, 2 phones for field capture (any modern browser, no app install), plus
spectators' own phones. Nothing needs internet access (SYS-093, ADR-002).

**Preparation (before leaving home).** Build the binary and seed the demo instance:

```
go build -o bahnfrei ./cmd/bahnfrei
./bahnfrei demo --data-dir ./demo-data
./bahnfrei serve --data-dir ./demo-data
```

`bahnfrei demo` refuses a non-fresh directory; it prints the meet ID, the demo credentials
and the operator/public URLs. Note the laptop's hotspot IP — phones use
`https://<laptop-ip>:8443/...` (self-signed venue certificate: accept the browser warning
once per device, SYS-093).

The seeded state (all created through the same service paths the UI uses):

| Item | Content |
|---|---|
| Meet | "UBS Kids Cup Uster (Demo)", from the built-in UKC template, dated today |
| Programme | 60 m, zone long jump, 200 g ball throw — one unit each, scheduled 09:30/10:30/11:30, timetable **published** (SYS-004) |
| Roster | 30 athletes, DE/FR Swiss names, divisions M/W 10–12, clubs TV Uster / LC Zürich / CA Fribourg / US Yverdon |
| Results | 79 pre-captured marks (every athlete has a 60 m time; a few long-jump/ball gaps left open on purpose — UC-033 #3 talking point) |
| Accounts | `organizer` / `demo-organizer-pw` (instance admin), `office` / `demo-office-pw` (competition office), `official` / `demo-official-pw` (field official, assigned to the 60 m and long-jump units) |

---

## 1. Install & quickstart story — 2 min

**Screen:** terminal + `https://<laptop-ip>:8443`. **Demonstrates:** UC-001 #1, SYS-131.

Tell it, don't do it live (the seed already ran): a fresh install is one binary and one
`serve` command; the first browser visit walks through creating the admin account — no
config file, ≤30 minutes end to end (CI-verified by `TestQuickstartFreshInstallE2E`).
Optionally show `./bahnfrei --version` and the `README.md` quickstart.

## 2. Template meet setup — 3 min

**Screen:** `/meets/{id}` (log in as `organizer`); show `/meets/from-template` for the form.
**Demonstrates:** UC-033 #1, SYS-053, SYS-004.

1. Open the meet page: the three UKC disciplines, per-birth-year divisions and UKC combined
   scoring came from the template with **no further configuration**.
2. Point at the published timetable and its version history; amend one unit's time and
   republish if asked — both versions stay retrievable (SYS-004, UC-001 #4).
3. Roster page `/meets/{id}/roster`: the 30 seeded athletes with bibs and clubs; add one
   live from the audience to show the office flow (UC-033, SYS-010).

## 3. Phone field capture, including going offline — 8 min

**Screen (phone 1):** `/meets/{id}/capture`, logged in as `official`.
**Demonstrates:** UC-011, UC-010 (subset), UC-034 #1/#2/#3, SYS-085/086/090.

1. Log in as `official` on phone 1. The capture index lists **only** the two assigned
   units — 60 m and long jump (per-event scoping, SYS-090, UC-022 #1).
2. Open the long-jump unit and **check it out** to this device (SYS-086).
3. Capture a live attempt for an athlete missing a mark (e.g. a `ZoneLJ` gap athlete);
   the unit standings update instantly.
4. **Go offline:** enable airplane mode (or walk out of hotspot range). The page shows the
   offline indicator; keep capturing attempts — entry is uninterrupted (UC-034 #1).
5. Reload the page while offline: captured attempts survive locally (UC-034 #3).
6. **Walk back / airplane mode off:** the queued captures submit automatically, in order,
   with no operator action, and repeated flaky reconnects cause no duplicates (UC-034 #2).
7. If the office revised the start list meanwhile, show `/meets/{id}/reconciliation`
   (as `office`): offline captures are surfaced for review, never silently discarded
   (UC-034 #4 — the documented Web.TEC 2 loss mode this design refuses to repeat).

## 4. Live public results on spectator phones — 4 min

**Screen (any phone, no login):** `https://<laptop-ip>:8443/m/{id}/results`.
**Demonstrates:** UC-017 (subset), SYS-070/071/074/076.

1. Hand the URL (or a QR code prepared from it) to the audience: public meet page `/m/{id}`,
   timetable `/m/{id}/timetable`, start lists `/m/{id}/startlists`, results `/m/{id}/results`.
2. The results page shows per-division rankings — rank, bib, name, club, birth year,
   per-discipline marks, points, total (UC-033 #4) — in DE or FR (SYS-074).
3. Capture one more mark on phone 1: open spectator pages update live via SSE without a
   reload (SYS-071).
4. Point out the "unofficial results" label with the UBS Kids Cup reference — the
   federation channel stays the official source (SYS-076).

## 5. Printed capture sheets & result lists — 3 min

**Screen:** office/organizer session. **Demonstrates:** UC-018 (subset), SYS-072.

1. Capture sheet PDF per unit: `/meets/{id}/capture/{unit}/sheet.pdf` — attempt grid with
   the seeded field, for clipboard fallback when a phone dies.
2. Result list PDF: `/meets/{id}/standings.pdf` — per-division ranking lists in the
   federation presentation structure, ready to pin to the clubhouse door.

## 6. UKC series-upload export — 2 min

**Screen:** `/meets/{id}/export/ukc-series` (office/organizer session).
**Demonstrates:** UC-035 #1–#3, SYS-077.

Download the file and open it: the organizer-template structure the UBS Kids Cup series
intake expects, fixture-verified (structure and points arithmetic tested against the
official table). This is the file the volunteer sends in after a local elimination — no
re-typing into another system.

## 7. One-action backup — 2 min

**Screen:** `/admin/backup` (as `organizer`, an instance admin) or the CLI.
**Demonstrates:** UC-020 #1–#3, SYS-081/084/130.

1. One click on `/admin/backup` (or `./bahnfrei backup --data-dir ./demo-data --out ukc.backup`)
   produces one portable artifact of the whole instance.
2. If time allows, restore it into a fresh directory and serve on another port:

   ```
   ./bahnfrei restore --data-dir ./restored --from ukc.backup
   ./bahnfrei serve --data-dir ./restored --addr :8444
   ```

   Same meet, same standings, same accounts (UC-020 #3). Mention the crash-mid-capture
   recovery drill in CI (UC-020 #2, SYS-081): kill -9 during capture loses no confirmed
   result.

## Close — what's next, and asks

- **M2 scope** (honest gaps today): online entries, check-in/seeding, full track program,
  FinishLynx exchange, vertical jumps & combined events, records, full i18n/a11y, privacy
  self-service (see `docs/delivery/work-breakdown.md`).
- **Asks for the clubs:** a real UKC local elimination to pilot; an Alabus entries export
  sample (OQ-011/DEC-011); feedback via GitHub issues.
- Everything shown is open source (AGPL-3.0-only) and self-hosted: the club owns its data
  (nFADP/GDPR posture, SYS-100+).

## Fallbacks

| Risk | Fallback |
|---|---|
| Hotspot fails | Everything runs on localhost; demo capture and public pages on the laptop in two browser windows |
| Phone refuses the self-signed cert | Accept-once flow per SYS-093; or use the laptop for capture |
| Seed dir not fresh (`demo` refuses) | Point `--data-dir` at a new empty directory — the refusal itself demonstrates the guard |
| Projector only | Public results page full-screen; capture on one phone held to the camera |
