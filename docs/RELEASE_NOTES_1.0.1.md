# v1.0.1 — draft patch notes

Status: in preparation, not yet published. v1.0.0 is the published baseline.
The maintainer will create the release after review and merge. Review this list
against the final release commit before publishing.

## Fixes

- The first purchase form handles empty, loading and unavailable filament data.
- Empty API collections return arrays consistently.
- Print jobs accept fractional labor hours and reject nonpositive quantities.
- Purchases reject nonpositive prices/amounts and non-finite unit prices.
- Older requests no longer replace newer search/list results.
- Restoring a backup clears the file selector so the same file can be selected again.
- Print-job searches distinguish no matching results from an empty workspace.

Regression coverage was added for these workflows in
[PR #1](https://github.com/GabrielAlmd/maker-track-studio/pull/1).
Passing source tests does not establish native installer or upgrade readiness;
see the [release checklist](RELEASE_READINESS_1.0.1.md).

## Upgrade from v1.0.0

1. Export a ZIP from **Settings > Data & Backups** and keep an independent copy.
2. Retain the v1.0.0 installer/archive or image digest and configuration.
3. Close Desktop, or stop the Self-Hosted service and retain cold volume copies.
4. Install the published v1.0.1 artifact when available. Check your records,
   representative print costs and a fresh backup export.
5. Verify restore in a disposable installation before relying on the backup.

This patch does not intentionally change the SQLite schema or ZIP format.
For rollback, stop the new version and use v1.0.0 with the pre-upgrade data;
changes made after that backup will not be present. Follow the
[Desktop guide](../apps/desktop/README-DISTRIBUTION.md),
[Self-Hosted guide](SELF_HOSTED.md) and [recovery procedure](SQLITE_RECOVERY.md).

## Boundaries and known limitations

Desktop remains local; Self-Hosted remains LAN-only. The two installations do
not synchronize. Diagnostics provide local logs; diagnostic report export and
next-start diagnostic recovery are still planned. Native Windows/Linux packages,
native ARM64 and physical LAN acceptance must have evidence recorded for the
final patch artifacts before qualification.
