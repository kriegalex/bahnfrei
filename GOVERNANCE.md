# Governance

Bahnfrei is a founder-led open-source project.

## Roles

- **Founder / maintainer:** [@kriegalex](https://github.com/kriegalex). Final say on
  releases, roadmap, and one-way-door decisions. Additional maintainers may be invited as
  the contributor base grows; maintainership changes are recorded here.

## How decisions are made

- **Architecture and other one-way doors** (stack, data model, licence, external
  integrations) are proposed as ADRs in `docs/architecture/adr/` and ratified by the
  founder before anything is built on them. The ADR index in
  `docs/architecture/architecture.md` is the authoritative status list.
- **Requirements** are versioned specs (`docs/requirements/`) with stable IDs
  (`STR-###`/`SYS-###`/`UC-###`); IDs are never renumbered, only deprecated.
- **Day-to-day changes** follow `CONTRIBUTING.md`: spec-traced PRs, green CI, DCO.

## Licence stewardship (ADR-001)

- Code is **AGPL-3.0-only** with **DCO, no CLA** — deliberately: no single party (including
  the founder) accumulates relicensing rights, so any future licence change requires the
  consent of all contributors. This is a commitment device against proprietary capture.
- The **project name ("Bahnfrei") and logo are held personally by the founder** and are not
  covered by the code licence (ADR-001 §5). Forks are welcome under the AGPL but must not
  present themselves as this project.
- Documentation is **CC-BY-SA-4.0**.

## Release authority

Releases are cut by the founder following the documented release process
(`docs/delivery/`, established with release 0.1 / TASK-028). Security reports: contact the
maintainer privately at kriegalex@gmail.com before public disclosure.
