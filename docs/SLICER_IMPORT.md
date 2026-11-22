# Slicer imports (INV-10)

In **Print Jobs → Plan production**, select **Import slicer estimates**. Import
appends editable planned runs and preserves existing manual entries. Remove an
unneeded unsaved run using **Remove unsaved run**. Review models, quantities,
filament mappings and included waste before saving. Import inspection never
creates production, inventory movements or commercial records. The existing
aggregate save remains transactional and retry-safe.

## Supported formats and fields

| Input | Imported | Unknown / intentionally unsupported |
| --- | --- | --- |
| Text `.gcode` with PrusaSlicer-style summary comments | One run per selected plate file; comma-separated `; filament used [g] = …` slots; `; estimated printing time (normal mode) = …` in d/h/m/s, rounded up to minutes | Model identity/counts, repetitions and waste; length-only totals, silent-mode time, binary G-code |
| Bambu-style `.3mf` containing `Metadata/model_settings.config` | Explicit `plate` entries and their `model_instance` objects; one planned item per instance, with source object name shown for manual catalog mapping | Material grams, waste, time, repetitions, geometry-derived quantities, modifier parts and arbitrary/universal 3MF formats |

The G-code summary syntax comes from [PrusaSlicer's exporter](https://github.com/prusa3d/PrusaSlicer/blob/master/src/libslic3r/GCode.cpp).
Only the exact documented comment keys are read. A total is not copied to every
filament. Unused zero-weight tool slots are omitted while tool numbers are
preserved. No lengths, preset densities or geometry are converted into grams.
Model counts in G-code stay blank and must be entered; 3MF instance counts are
editable. Source names do not automatically match existing catalog identities.
Import labels are temporary review aids and are not persisted as catalog names.
Bambu layout structure is read independently of sliced summaries; even a sliced
3MF currently imports layout only. Export each plate as supported text G-code
when material/time estimates are needed. This is not universal 3MF compatibility.

Estimates remain visibly distinct from actual duration and measured consumption.
Editing imported grams retains estimated provenance until explicitly verified
on an executed run. Waste is included within total grams, never added twice.
A missing duration or waste remains null, distinct from a known zero.

## Limits and failure behavior

Select at most 16 files, 32 MiB per file and 64 MiB per batch, with at most 100
runs per job. 3MF inspection reads only the metadata entry (2 MiB limit), never
extracts archive paths or runs G-code. Archives have at most 4096 entries;
ambiguous duplicate metadata, duplicate instances, missing object references,
invalid XML, unsupported layouts and invalid numeric summaries are rejected.
A failed file rejects the whole batch without appending any runs. A subsequent
save failure leaves form edits available for retry.

3MF inspection uses the existing local API and LAN HTTPS proxy. The original
file is not stored. Multipart temporary files are cleaned up on success/failure.
The desktop and browser use the same parser and production API; no cloud
service, telemetry or new dependency is involved.

## API, SQLite and backup compatibility

`POST /v2/production/slicer/inspect` takes multipart `file` and returns
`{ "plates": [...] }`. Each plate has `source_label`, `items` containing
`source_model` and `planned`, null `estimated_time_minutes` and empty `materials`.
Invalid/unsupported input returns HTTP 400 with a generic error. This endpoint
has no database dependency and performs no application data writes.

The production aggregate accepts/returns optional nullable
`estimated_time_minutes` on each run (integer 0–1e9). It is separate from
`print_time_minutes`, which continues to mean actual duration and cannot be
provided on a planned run. Estimated time never enters actual cost calculations.
Terminal estimates are immutable along with terminal run data.

SQLite adds a nullable `print_runs.estimated_time_minutes` column via the shared
idempotent startup migration. Existing rows and older backups retain null,
without backfilling actual time into estimates. No IDs, financial snapshots or
physical balances change. Backup ZIP format remains 1; schema 19 includes the
column, requires it in schema-19 archives, and accepts supported older schemas.
Export/restore preserves estimates and operation receipts. Older binaries reject
schema-19 backups. Back up before upgrading; rollback uses a pre-upgrade backup
with its compatible binary, losing subsequent writes rather than attempting a
destructive reverse migration. Do not edit upgraded data with an older binary.

## Validation

Synthetic multi-plate and mixed-material G-code fixtures live in
`apps/web/visual/fixtures`; 3MF archive tests construct synthetic layouts in
memory. Screenshot scenarios use synthetic data only. A real user-provided
Bambu project was also checked read-only: one plate, seven model instances,
no sliced plate summaries, unknown material grams/time. The original project,
its models, images and source paths are not committed or used in screenshots.

Packaged Windows/Linux desktop and physical authenticated LAN testing remain
part of joint v1.1.0 release qualification.
