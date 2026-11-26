# SQLite ownership, backups and recovery

Maker Track Studio has one workspace and one SQLite database per installation.
Multiple browser devices share the same standalone API through the authenticated
HTTPS proxy described in [LAN security](LAN_SECURITY.md). Run exactly **one API
process / replica** per database. There are no accounts, tenants, replication or
automatic desktop/server synchronization.

## Ownership and paths

Both desktop and standalone startup take an OS-held exclusive lock on
`<database>.owner.lock` before opening or migrating SQLite. A second current
runtime refuses startup, even on a different HTTP port. Symlink paths are
canonicalized; hard-linked database files are rejected. The lock is released
only after active workspace operations finish and SQLite closes. The OS releases
it after a crash. **Never delete or replace the lock file**, including during
upgrades; its existence does not mean the database is in use.

This is an application ownership lock, not protection against arbitrary SQLite
tools or older releases that do not implement the lock. Stop all desktop apps,
API services and import tools before offline maintenance. Never point desktop
and server at a shared database file. Give the service its own OS user and a
private persistent directory (0700); database and lock files are 0600 on Linux.
Windows desktop files inherit the user profile ACL. Existing parent directory
permissions are not rewritten, so check permissions on pre-existing mounts.

The on-disk defaults remain:

| Runtime | Location | Override |
| --- | --- | --- |
| Linux desktop | `~/.local/share/3d-printing-cost-tracker/app.db` | `SQLITE_PATH` |
| Windows desktop | `%USERPROFILE%/AppData/Roaming/3d-printing-cost-tracker/app.db` | `SQLITE_PATH` |
| Standalone | `~/.local/share/maker-track-studio/server.db` | `API_SQLITE_PATH` |

Keep the server database directory in persistent storage across service/image
replacements. Removing an application binary or container must not remove the
data directory. Deployment lifecycle behavior is covered by the Compose
configuration and release integration checks.

## Concurrency and storage policy

A single reusable SQLite connection uses a **5000 ms busy timeout**, **DELETE
rollback journaling**, and **FULL synchronization**. Settings are applied on every
new connection. The runtime serializes entire HTTP workspace requests, scheduled
backups and restores. This protects read/calculate/write operations that span
multiple SQL statements and keeps report reads coherent with application writes.
Existing transactions still provide atomic purchase batches, Sales allocations,
spreadsheet imports and archive replacement. Waiting requests respect client
cancellation. Simultaneous edits to the same field retain the existing
last-successful-request behavior; there is no collaborative editing/version UI.

WAL is deliberately not enabled: serialized workspace requests do not require
parallel SQLite readers/writers, and no WAL checkpoint policy is needed. Do not
change journal or synchronous modes while the application is running. Live SQLite
files must reside on reliable **local storage**, not NFS/SMB/NAS shares (including
volumes mounted from those shares). Network backup destinations are allowed,
subject to their filesystem's rename/fsync support. See SQLite's
[WAL restrictions](https://www.sqlite.org/wal.html) and
[PRAGMA reference](https://www.sqlite.org/pragma.html).

For Raspberry Pi, prefer an externally powered USB SSD and reliable power/UPS.
Leave room for the live database, journal, staged snapshots and backup archives.
Measure request latency, backup duration/size and disk writes on representative
workloads before tuning: this change does not claim an SD-card endurance or
throughput benchmark. Avoid frequent backup schedules, aggressive vacuum loops
or disabling fsync to reduce writes.

## Opt-in daily server backups

Configure the standalone process, for example:

```dotenv
API_BACKUP_DIR=/mnt/backup-usb/maker-track
API_BACKUP_TIME_UTC=03:00
API_BACKUP_RETENTION=14
```

`API_BACKUP_DIR` enables backups; absent it, no scheduler runs. Time defaults to
03:00 UTC and retention to 14 completed archives (valid range 1–3650). The next
backup runs at the next configured UTC time; missed runs while stopped are not
replayed. Files are named `maker-track-backup-<UTC timestamp>-<unique suffix>.zip`.
Only matching regular files count toward retention. Manual ZIPs, symlinks and
interrupted `.tmp` files are left untouched. The service logs a generic failure
to stderr if a run fails; monitor service logs and backup freshness.

Backups reuse the desktop ZIP format (`manifest.json` and `app.db`) and the
existing consistent `VACUUM INTO` export, never copy an open database file.
A private temporary archive is written beside its destination, flushed, closed,
and renamed before old archives are pruned. Linux also syncs the directory.
Interruption or destination failure before publication leaves retained archives
intact; abandoned temporary files may be removed with the service stopped.
Retention errors after publication are reported as failures and may leave extra
archives. Backup jobs have a two-minute context deadline and share the workspace
gate, so large backups temporarily delay requests. Shutdown cancels scheduled
work before releasing database ownership.

The backup directory may be outside the database volume, on USB, a separate host
directory, or an already mounted NAS. Provision the mount and private directory
for the service user. **Require the mount in the host service configuration**
(e.g. systemd `RequiresMountsFor=`); a missing mount must not silently redirect
backups onto the SD card. Network availability is not assumed: failures do not
stop normal application use or prune existing backups. A filesystem operation
blocked inside the OS by a failed network mount can exceed the context deadline;
use host mount timeouts and monitoring. Keep another tested copy off the device.

Database directories, recovery directories and backup mounts must never be
inside a static document root. No new HTTP backup listing/download endpoint is
added. Existing ZIP export/import endpoints remain behind the same proxy
credentials, HTTPS/origin checks and request limits as other server routes.

## Restore and rollback

1. Export a current ZIP through **Settings > Data & Backups** and retain it away
   from the device. Verify the destination and stop other users' work.
2. Import the selected ZIP through the same protected application UI. The server
   rejects unsupported/duplicate entries, oversized decompressed entries,
   incompatible versions, invalid SQLite integrity and invalid module data.
3. After validation and before replacing data, the owning runtime writes a
   recovery ZIP in `<database>.recovery/` beside the live database (retaining five
   snapshots). If that write fails, restore stops without changing the workspace.
   This also applies to desktop. These local recovery snapshots do not replace
   an independent backup destination.
4. The entire replacement and legacy migration occur in one SQLite transaction,
   with all other workspace requests/backups waiting. A partial import rolls
   back. Restore never swaps the open SQLite file. A newer request submitted after
   restore operates on the restored workspace; refresh other browser devices.
5. Check record counts, relationships, reports, Print Job costs and optional
   Sales allocations. To undo a completed restore, import the retained recovery
   ZIP through the same UI. No files need to be copied into an active database.

A full or unavailable recovery volume intentionally blocks restore. Correct the
storage problem first. Invalid uploads do not rotate recovery snapshots. ZIP
imports retain the existing 64 MiB upload and per-entry decompression limits;
plan and test restores before a workspace exceeds those limits.

## Compatibility and upgrade procedure

Current desktop and self-hosted builds share backup schema version 23 and ZIP
format version 1. Schema 23 adds four optional filament and purchase fields.
Startup adds empty columns to existing databases and keeps record IDs, prices,
quantities and inventory movements. Version 22 and earlier archives restore
without requiring these columns. Use a pre-upgrade ZIP with its compatible build
to roll back. Schema 22 (Sales module version 9) adds a nullable
`business_order_revisions.quick_sale_pricing` snapshot. Restore checks frozen
cost and suggested cents against their assumptions and the confirmed
quantity. Older Quick Sales and archives retain their agreed prices and null
estimates. Roll back by restoring a pre-upgrade ZIP with a compatible binary.
Schema 21 adds filament archive and warning settings. The
core inventory marker advances to 5. Restore validates tracking transitions
while replaying ledger history in sequence. Earlier backups restore with
tracking and warnings enabled and active catalogue entries. To downgrade,
restore a pre-upgrade backup with its matching binary. Schema 20 includes [model material requirements](MODEL_MATERIALS.md).
Restore validates requirement IDs, model and filament references, row order and
derived totals before replacing data. Earlier archives retain their model IDs
and aggregate estimates with an unknown split. Schema 19 adds estimated time
to production run records. Schema 18 (Sales module version 8) adds nullable Quick Sale
payable and customer-name snapshot columns to the existing Sales tables. The
migration does not convert historical orders to Quick Sales or replay their
stock. Earlier archives restore with these fields empty. Restore validates the
frozen payable against the original line, the confirmation and the finance
history before replacing SQLite data. Roll back with the pre-upgrade ZIP and
compatible prior binary. Schema 17 (Sales module version 7) adds immutable finance entries
and uses the existing command history for retry identity. Restore checks order
ownership, entry sequence, invoice limits, receipt balances and linked
reversals before replacing the workspace. Earlier backups import with empty
finance history. Migration preserves historical order IDs and cost snapshots.
Use the pre-upgrade ZIP and prior binary for rollback. Schema 16 (Sales module version 6) adds the order attachment and
activity tables. SQLite stores attachment bytes together with their metadata.
Restore validates the order link, names, size and type limits, SHA-256
checksums and event references before replacing live data in a single
transaction. Supported older backups restore without new attachment or
production activity rows. Keep a pre-upgrade archive and use it with its
compatible binary for rollback. Schema 15 (Sales module version 5) adds order production revisions,
planned run-item links, and an optional run-item reference on allocations.
Existing allocations retain their IDs, Print Job references and frozen costs.
Restore validates link ownership, source identity, and planned and accepted
capacity before replacing the workspace. Schema 14 adds immutable order
revisions, an optional accepted proposal reference, and line source and
discount fields. Older Sales rows retain their IDs and default to zero
discount. Restore validates the accepted revision, its lines and the amendment
chain before replacing the workspace. Schema 13 adds [Business proposals](BUSINESS_PROPOSALS.md), including
issued revisions and frozen customer terms. Schema 12 adds [Business customers and budgets](BUSINESS_BUDGETS.md),
including estimate revisions and operation history. Schema 11 adds finished
goods movement fields, schema 10 adds material stock thresholds, and schema 9
records observed physical counts and item revisions. Restore verifies counts
against their preceding balance and revision before replacing data. Schema 8
adds automatic material source references. Schema 7 includes Inventory
movements and their audit and retry identifiers. Restoring a pre-ledger archive
clears local ledger history and leaves balances unverified. Schema 6 adds
production lifecycle records. Schema 5 adds material estimate, waste and
measurement provenance to Print Job filament rows. Supported older ZIP schemas
(1–15) follow the existing migrations. Missing optional-module support fails
closed for databases or archives containing that module's data. Tests use disposable fixtures for
legacy migrations and current round trips; no live user data is migrated.

For v0.4.0 desktop data, retain the original data directory and export using that
version where possible. Transfer by explicit ZIP export/import into a separately
started server, or upgrade a **disposable copy** of the old SQLite database with
all old processes stopped. Never allow the old release and current runtime to
write the same file. A real historical v0.4.0 release database has not been
validated by these fixture tests; verify a private copy before retiring originals.

Before each version/image upgrade: export a ZIP, stop the service cleanly, retain
the old version/configuration and persistent directory, then start the new
version against the retained data path. Verify readiness and representative data
and do a restore drill in a disposable installation. To roll back, stop the new
version and restore the **pre-upgrade** data copy with its matching old version;
do not assume old binaries can read a newer migrated schema or ZIP. Cross-host
and AMD64/ARM64 transfers use explicit archives, not database replication.

## Power loss or disk failure

After an unclean shutdown, keep the database and any `-journal`, `-wal` or `-shm`
sidecars together; never delete a journal to bypass startup trouble. Let SQLite
perform recovery on the same local filesystem. With the app stopped, inspect a
copy using `PRAGMA integrity_check`; preserve the original if it fails. Do not
run application migrations repeatedly on a damaged disk.

If storage is lost or integrity fails, provision healthy local storage, start an
empty installation with the matching/current supported version and import a
known-good ZIP from independent storage. If a failing database prevents startup,
stop the service and move the whole damaged data directory aside before creating
an empty one; keep the damaged files for recovery. Verify counts, costs and Sales
relationships after restore before reopening access. A backup is only useful
when its restore has been tested; power-cut behavior on real Raspberry Pi storage
and deployment image upgrade/rollback still require platform release checks.
