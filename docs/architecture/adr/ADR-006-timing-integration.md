# ADR-006 — Timing integration: FinishLynx file family first, watched-folder pattern

**Status:** **Accepted** — ratified by the founder 2026-07-05 (key external integration; incl. hub-first timing-agent amendment)
**Date:** 2026-07-05
**Traces:** SYS-060–062, STR-018, D7.2, C5, C7; depends on ADR-002

## Context

Every meet-management incumbent interoperates with timing hardware through the FinishLynx
file family — `lynx.ppl` (people), `lynx.sch` (schedule), `lynx.evt` (events/lanes) in,
`.lif` (results) out — exchanged via a shared local directory (D7.2, C5). ALGE and
TimeTronics tooling is Lynx-file-compatible in common configurations; Swiss timing crews'
workflows assume this pattern. It is inherently offline/LAN-native, which fits ADR-002.

## Decision

1. **First-class integration = the FinishLynx file family** over a configurable local shared
   directory: regenerate `.ppl/.sch/.evt` on start-list changes (SYS-060); watch for `.lif`
   arrivals and ingest them with unit matching and explicit conflict resolution — never
   silent overwrite (SYS-061, UC-014).
   *(Hub-first amendment, DEC-013/ADR-002 v2: the watched folder sits on the timing PC; a
   small local **timing agent** — same binary, agent mode — bridges it to the server over
   HTTPS, pointed at a URL that is the hub in MVP and a venue node later. The file-exchange
   semantics above are unchanged in both phases.)*
2. **Format conformance is fixture-driven:** the published FinishLynx field-order
   documentation is encoded as fixtures; golden-file tests validate byte-level output
   (UC-014 #1). Real-world `.lif` samples are collected as they become available (club
   contacts, OQ-013) and added to the fixture corpus.
3. **Generic documented CSV** import/export as the fallback for any other timing source
   (SYS-062), sharing the same conflict-resolution pipeline.
4. **Manual capture remains a co-equal path**, not a fallback afterthought: the founder's
   observed reality is human-measured field events entered in a results room (C7.3);
   the capture UI (UC-011/012) and the timing pipeline write through the same result-confirm
   flow, so provenance (manual vs. imported, hand vs. FAT) is always recorded (SYS-041).
5. EDM devices and scoreboard feeds stay **Later** (SYS-063/064) — separate ADRs when a
   target device/protocol exists.

## Alternatives considered

| Option | Why rejected |
|--------|--------------|
| Vendor-specific network APIs first | No documented public APIs at grassroots tier; the file pattern is the actual deployed lingua franca (C5) |
| Seltec/LA.portal integration | Closed ecosystem, no public surface (C7.1); revisit only if OQ-013 club validation reveals an entry point |
| Generic CSV only (skip Lynx formats) | Would force every timing crew into manual remapping — exactly the transcription failure STR-018 exists to remove |

## Consequences

- Drop-in compatibility with existing Swiss stadium timing rooms without vendor cooperation.
- File watching over a shared folder is OS-flavored (paths, locking); the abstraction gets an
  explicit adapter layer with per-OS E2E coverage in the offline suite (UC-019).
- Without real venue samples the risk is fixture-vs-reality drift; mitigated by the OQ-013
  club contact and by making the ingest pipeline forgiving (report-and-resolve, not reject).
