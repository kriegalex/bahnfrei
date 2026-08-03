# Operator runbook

**Traces:** SYS-131/132, SYS-084/101/102, SYS-060-062 → STR-039, STR-018, STR-031; UC-001, UC-014,
UC-020. **Audience:** the club member who runs the instance — before, during, and after a meet.
Read `docs/ops/quickstart.md` first if you have not installed bahnfrei yet.

## 1. Roles at a glance

| Role | Can | Typical device |
|---|---|---|
| `instance_admin` | Everything: accounts, backup, retention purge, all of `meet_organizer` | Office laptop |
| `meet_organizer` | Set up and run meets end to end (entries, programme, timetable, sanctioning) | Office laptop |
| `competition_office` | Day-of-competition office actions: check-in, corrections, overrides, timing exchange | Office laptop / call-room phone |
| `field_official` | Capture results, scoped to assigned events | Infield tablet/phone |
| `entry_submitter` | Submit/withdraw entries for a club before the meet | Any device, pre-meet |

Accounts are created and role-assigned under **Konten / Comptes** (`/admin`, instance-admin only).
There is no self-registration for operator roles by design (SYS-090) — hand out credentials
directly. A `meet_organizer`+ account manages the meet from a single instance-wide workspace
(`/meets`); every other role works from meet-scoped links the organizer shares out of band (there
is no public directory of meets to browse — see the hub landing page, `docs/ops/quickstart.md`).

## 2. Network kit for a meet day (ADR-002 v2)

Bahnfrei is **hub-first**: one self-hosted server instance (the "hub") holds the meet, reachable
by every operator device over whatever transport exists. Field-wide venue Wi-Fi is an **explicit
non-goal** (ADR-002 v2, DEC-013) — nobody at this club tier builds it, and interruption tolerance
(SYS-085–087, UC-034) is the MVP answer to connectivity gaps instead. Practically, that means:

- **Minimum kit: one phone hotspot at the admin table.** The office laptop runs `bahnfrei serve`
  and joins (or hosts) that hotspot; every operator device — call-room phone, infield tablets,
  timing PC — reaches the laptop's address over the hotspot, venue Wi-Fi where it happens to
  exist, or each device's own mobile data. Nothing beyond that is required.
- **No field-wide Wi-Fi to plan, budget, or troubleshoot.** If your venue happens to have Wi-Fi
  covering the infield, use it — it's a bonus, not a dependency.
- **Every operator device needs the hub's address once**, e.g. `https://<laptop-ip>:8443` in venue
  mode (self-signed certificate — accept the one-time browser warning per device, SYS-093) or the
  hub's real domain in hosted mode. Write it on a whiteboard at the admin table; there is no
  service-discovery step.
- **Capture continues through connectivity blips.** A device that has loaded its assigned event
  unit keeps capturing locally while disconnected and submits automatically, in order, and
  idempotently on reconnection — no manual "sync" button, no duplicates (SYS-085–087). The
  authenticated shell shows a degraded-connectivity banner while a blip is active; it clears on
  its own on reconnect.
- **The hub is the meet-day single point of failure**, accepted for 0.1 with two named
  mitigations: printed capture sheets remain the documented fallback if a device or the hub itself
  is unreachable for longer than the offline queue tolerates, and disaster recovery is
  restore-anywhere from the one-file backup (§4 below).
- **Timing PC:** runs the `timing-agent` mode (§3) on the same network — it talks to the hub over
  HTTPS exactly like a browser device does, so it needs no special network path either.

**Checklist for the admin table:** office laptop, one phone hotspot (charged, credit/data
available), the hub's address written down, one spare phone/tablet as a backup capture device, a
few printed capture sheets. That's the whole network kit.

## 3. Timing-agent mode (ADR-006)

The timing PC (FinishLynx or compatible hardware) never talks to bahnfrei directly — a small agent
(the same binary, in `timing-agent` mode) watches its local results folder and bridges it to the
hub over HTTPS:

1. On the hub, open the meet's **Zeitmessung / Chronométrage** page (`/meets/{id}/timing`,
   competition-office level+) and create an agent token (a label like "Timing PC", e.g.). The
   plaintext token is shown **once** — copy it immediately.
2. On the timing PC, in the folder the timing software writes `.lif` files into and reads
   `lynx.ppl`/`.sch`/`.evt` from, run:

   ```
   bahnfrei timing-agent \
     --hub-url https://<hub-address>:8443 \
     --meet <meet-id> \
     --token <the-token-from-step-1> \
     --watch-dir .
   ```

   Add `--insecure-skip-verify` only against a venue's self-signed local certificate (never over
   the open internet); `--once` runs a single scan/poll cycle for scripted verification instead of
   watching continuously.
3. The agent regenerates `lynx.ppl`/`.sch`/`.evt` in the watch directory whenever the hub's start
   list changes, and uploads new/changed `.lif` result files as they appear. Conflicts (an
   existing manual result, an unknown bib) are never silently overwritten — they surface on the
   meet's timing page for an operator to resolve (SYS-061, UC-014).
4. Manual capture (the office/infield UI) remains a co-equal path at all times, not a fallback:
   both write through the same result-confirm flow, and provenance (manual vs. imported, hand vs.
   FAT) is always recorded (SYS-041).

No timing hardware yet, or a non-Lynx system (ALGE, TimeTronics)? The meet's timing page also
offers documented generic CSV import/export (SYS-062) through the same conflict-resolution
pipeline.

## 4. Backup & restore (SYS-084, UC-020)

All state — meet data, accounts, audit log — lives in one SQLite database file plus the TLS
certificate cache in the data directory. There are two equivalent ways to snapshot it:

- **Office UI:** instance-admin level, `/admin` → **Backup** — a one-click download of a
  self-contained backup artifact (checksum included).
- **CLI (for cron/scripted backups):**
  ```
  bahnfrei backup --data-dir ./data --out ./backups/meet-$(date +%F).bfbak
  ```

**Restore** onto a fresh installation (UC-020 #3 — restore never merges into an existing
database):

```
bahnfrei restore --data-dir ./restored-data --from ./backups/meet-2026-07-14.bfbak
```

Opening the restored database re-runs the startup consistency check (SYS-081/SYS-130), so a
restore is *verified*, not just copied — it reports the meet count and confirms the checksum
before you point `bahnfrei serve` at the restored directory. Take a backup at least once per
competition day (more often for a multi-day meet) and after the meet closes; UC-020 #3's own bar
is a full-fidelity restore in ≤15 minutes, which the CLI round-trip above comfortably meets on
commodity hardware.

## 5. Data retention & privacy

Personal data not needed for the permanent sporting record (contact details, consent-recorder
identity, audit-log PII) becomes purgeable automatically a configurable number of days after a
meet closes (default 90, `--retention-days` on `serve`). Trigger a purge manually from `/admin`
(instance-admin level) or let the startup job apply it. See `docs/ops/privacy.md` for what this
means for your meet's privacy notice and your obligations as the data controller.

## 6. Support & known limits

See `docs/ops/support-matrix.md` for supported OS/browsers and the **actually measured** public
live-results viewer capacity (the 2,000-concurrent-viewer target is met — SYS-122, DEC-015/
TASK-035); see `docs/ops/defect-policy.md` for how defects are triaged and what "no known
critical/high defects at release" means in practice.
