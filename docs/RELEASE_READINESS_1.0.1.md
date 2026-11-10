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
| First-print walkthrough with real API and disposable data | Pending | Record browser/runtime, steps, persistence and calculated result. |
| Windows installer and portable UI | Pending | Clean native Windows: install, language, Wails readiness, first purchase/print, restart, uninstall/data retention. |
| Linux archive and Wails UI | Pending | Clean supported Linux: dependencies, launch, first purchase/print, restart and retained data. |
| Native Self-Hosted AMD64 and ARM64 | Pending | Runtime/Compose/browser results and immutable image identities for both architectures. |
| v1.0.0 → v1.0.1 persistence and upgrade | Pending | Disposable baseline data, record counts and representative costs before/after; exact baseline/patch identities required. |
| Backup export/import and rollback | Pending | Restore separate copy; stop patch and restore pre-upgrade data with v1.0.0; verify records. Include Self-Hosted CA/config volume recovery. |
| LAN boundary and second-device access | Pending | TLS trust, authentication, anonymous denial, direct API/guest/WAN denial, trusted second device. |
| Security and licensing | Pending | Final-source dependency/secret scans, image scans, license review; critical/high findings block release. |
| Artifacts | Pending | Checksums, SBOM and build identities match the distributed artifacts. |
| Documentation and notes | Pending | First-use/support guidance, accurate snapshot semantics, official download links and final patch notes. |

## Procedures and ownership

Use [release qualification](PUBLIC_RELEASE.md), [Desktop packaging](../apps/desktop/README.md),
[Self-Hosted execution checks](SELF_HOSTED.md#running-execution-checks),
[SQLite recovery](SQLITE_RECOVERY.md) and [CI policy](CI.md).
The maintainer records physical-device and published-artifact results. Local
source checks can support review but cannot substitute for those checks.
Remaining real-API, Wails and artifact work is tracked here without depending on
private local planning documents.
