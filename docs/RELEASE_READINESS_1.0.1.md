# v1.0.1 readiness record

Status: preparation in progress. v1.0.0 is published. This record does not
publish or qualify v1.0.1. The maintainer creates the release after merge.

## Identity and evidence rules

- Baseline: published v1.0.0; record its exact tag commit and artifact checksum
  or image digest when running the upgrade drill.
- Patch source commit: pending final review/merge.
- Patch installer/archive checksums and image digests: pending packaging.
- Record each result with date, platform, command/workflow run URL, source SHA,
  artifact identity and evidence location. Never reuse an earlier build's scan
  as proof for a later build.
- Status values: **passed**, **pending**, **blocked**, **not applicable**.
  Explain blocked or not-applicable entries. A missing upgrade baseline is
  blocked, not a first-release exception.

## Release gates

| Requirement | Status | Evidence / remaining work |
| --- | --- | --- |
| Regression fixes and source checks | Pending | PR #1 records 244 Vitest tests, four fixture-backed Chromium cases, Go tests/vet, lint/build and 102 visual scenarios on its branch. Rerun relevant checks on final patch source; these are not real-API or Wails evidence. |
| First-print walkthrough with real API and disposable data | Passed | 2026-11-10 local Chromium/Vite + shared Go runtime, synthetic SQLite: €5.70 total, €2.85 per piece, reload persistence. See local verification below. Native distribution checks remain separate. |
| Windows installer and portable UI | Pending | Clean native Windows: install, language, Wails readiness, first purchase/print, restart, uninstall/data retention. |
| Linux archive and Wails UI | Pending | Clean supported Linux: dependencies, launch, first purchase/print, restart and retained data. |
| Native Self-Hosted AMD64 and ARM64 | Pending | Runtime/Compose/browser results and immutable image identities for both architectures. |
| v1.0.0 → v1.0.1 persistence and upgrade | Pending | Disposable baseline data, record counts and representative costs before/after; exact baseline/patch identities required. |
| Backup export/import and rollback | Pending | Local browser ZIP round trip passed on 2026-11-10, including workspace replacement. Published-v1.0.0 rollback and Self-Hosted CA/config volume recovery still require artifact checks. |
| LAN boundary and second-device access | Pending | TLS trust, authentication, anonymous denial, direct API/guest/WAN denial, trusted second device. |
| Security and licensing | Pending | Final-source dependency/secret scans, image scans, license review; critical/high findings block release. |
| Artifacts | Pending | Checksums, SBOM and build identities match the distributed artifacts. |
| Documentation and notes | Passed | First-use/support guidance, corrected snapshot semantics, release status and draft notes implemented. Local link targets checked. Confirm final artifact links/versions and notes at release time. |

## Procedures and ownership

Use [release qualification](PUBLIC_RELEASE.md), [Desktop packaging](../apps/desktop/README.md),
[Self-Hosted execution checks](SELF_HOSTED.md#running-execution-checks),
[SQLite recovery](SQLITE_RECOVERY.md) and [CI policy](CI.md).
The maintainer records physical-device and published-artifact results. Local
source checks can support review but cannot substitute for those checks.
Remaining real-API, Wails and artifact work is tracked here without depending on
private local planning documents.

## Local implementation verification — 2026-11-10

Source: `e61263f` (includes the documentation commits `1342dde` and `4376a0a`).
The commands below ran on the same application source before committing; this
record is a documentation-only follow-up. No release artifacts were built or
qualified by these checks.

| Check | Result | Scope |
| --- | --- | --- |
| `npm test -- --run src/pages/Settings.test.jsx src/pages/Printers.test.jsx` | Passed, 25 tests | Existing settings/printer behavior |
| `npm run lint` | Passed | Frontend lint |
| `npm run screenshots:check -- --grep 'settings-calculations\|printers-costs'` | Passed, 8 scenarios | English Simple/Advanced, Portuguese Simple and printer costs, both themes; references regenerated and reviewed |
| Vite production build (run by screenshot commands) | Passed | Existing large-chunk warning remains; not a native package build |
| Local Markdown target and whitespace checks | Passed | Changed guides and navigation; remote artifact availability not verified |
| First-print browser walkthrough | Passed | Real shared Go runtime and temporary SQLite, no HTTP fixtures; [steps and result](FIRST_PRINT.md#verification--verificação) |
| Browser backup round trip | Passed | Exported synthetic workspace, created one additional printer, imported ZIP, verified additional printer removed and original €5.70 job retained; upload input cleared |

To repeat the walkthrough, use a new disposable development workspace and the
values in [First print](FIRST_PRINT.md). For the restore drill, export the ZIP,
create a second synthetic printer, import the ZIP and confirm only Guide Printer
and its original print remain. Never perform this replacement drill on a real
workspace. The verification used loopback development access; it does not test
the authenticated HTTPS proxy or Wails bridge.

Published-v1.0.0 artifact retrieval/checksums, upgrade/rollback, native Windows,
packaged Linux, ARM64 and physical LAN checks were not executed in this session.
They remain pending for maintainer/CI validation of the intended release.
