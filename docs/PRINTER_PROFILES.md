# Printer profiles

## Identity, lifecycle and cost snapshots

The app supports physical printers, independent cost configuration, print-job
assignment, the Printers page and versioned cost snapshots. The optional offline
preset catalog helps populate initial profiles.

Each printer has a Go-generated UUID, name/nickname, manufacturer, model, notes,
default/archive flags, and timestamps. Identical printer models are independent
physical machines with distinct IDs. Display names should distinguish them.

| Method | Path | Behavior |
| --- | --- | --- |
| GET | /printers?state=active | List active machines (default filter) |
| GET | /printers?state=archived | List archived machines |
| GET | /printers?state=all | List all machines |
| GET | /printers/:id | Read one machine, including archived machines |
| POST | /printers | Create; first active machine becomes default |
| PUT | /printers/:id | Replace metadata only |
| PUT | /printers/:id/default | Atomically select an active default |
| PUT | /printers/:id/archive | Archive without deleting its identity |
| PUT | /printers/:id/restore | Restore; becomes default if no active machines exist |
| PUT | /printers/:id/cost-config | Replace the complete validated numeric cost configuration |

Create/update accepts name, manufacturer, model (required, 1–100 characters),
and optional notes (up to 2000 characters). State changes have dedicated endpoints;
metadata payloads cannot change IDs, state, or timestamps.

Example metadata:

```json
{"name":"A1 – Workshop","manufacturer":"Bambu Lab","model":"A1","notes":""}
```

There is no DELETE endpoint. To archive the default while other active machines
exist, first select a replacement; otherwise return 409. The last active machine
may be archived, leaving no default. Archive/restore are idempotent.

SQLite transactions acquire the writer lock before inspecting lifecycle state.
A partial unique index prevents two defaults even outside the handlers, and a
CHECK constraint prevents an archived default. A database lock/write failure
returns a generic error, not a successful mutation.

## Cost configuration and history

New profiles copy the current global inputs at creation. GET /printers returns
cost_config. PUT /printers/:id/cost-config accepts {"cost_config": {...}} with
all seven basic calculation keys and eight AdvancedConfigDefaults keys. Unknown,
missing, null, non-finite and out-of-range values are rejected. This operation
does not modify global settings or other machines.

Print-job create/update accepts printer_id. Creation selects the default if
omitted. Explicit assignments must reference an active machine. Editing with
the same ID (or omitting/nulling it) retains the original assignment and
snapshot, including archived printers. Changing to another active ID captures
that machine's current configuration.

For transition compatibility, creation without profiles remains unassigned;
once profiles exist, an active default or explicit active ID is required.
New jobs require a configured active default or explicit active printer after
profiles exist. Legacy unassigned jobs continue using global settings until
explicitly assigned.

The printer_cost snapshot stores version 1, the machine name at assignment,
capture timestamp and raw inputs. Simple and advanced formulas normalize a copy
of those inputs; list, detail and aggregate costs use the same calculation.
The manual wear rate and kWh tariff are never overwritten with derived rates.
Phase-1 profiles lacking cost_config initialize it on first assignment.

This freezes configuration inputs, not the complete print record: deliberately
editing duration, material usage, or labor still changes its cost. Filament
average purchase prices remain live, and model-derived duration retains the
existing behavior. A complete immutable accounting ledger is outside this step.
Equipment costs currently use the captured printer_upfront_cost aggregate;
a per-printer equipment expense editor is not implemented in this increment.

## Opt-in cost presets

The bundled catalog in `apps/web/src/pages/printerPresets.js` contains ten
versioned PT/EU examples. It works offline and is offered only when creating a
printer or explicitly applying values in its cost editor. The catalog is a
starting point, not a price list or authority on operating costs. Each entry
records its review date, region, source, confidence and distinction between
manufacturer rated/maximum power and estimated average print power. Preset
values are provisional estimates, not measured values or a substitute for a
printer's published specifications and measured power draw.
The initial roster spans multiple manufacturers and common market segments
(compact, budget, open-frame, enclosed and CoreXY); it is a practical coverage
selection, not a sales ranking. Review the roster periodically as models and
regional availability change. Catalog schema changes use the `schemaVersion`
field and must keep saved printer values independent of catalog migrations.

Applying a hardware preset copies its values into the profile once; subsequent
catalog changes cannot update saved profiles. It does not replace electricity,
labor, filament waste or margin settings. An optional complete reference
preset can also provide example workspace cost values for those fields. In
Advanced mode the application derives the hourly wear rate from the saved
inputs; it does not rely on the preset's displayed `printer_wear_cost_hour`.
The A1 value is retained in the complete reference for traceability. Applying
values to an existing profile requires reviewing a field-by-field change list
and then saving. Job snapshots remain unchanged.

Preset purchase prices are EUR examples for PT/EU, and users should replace
them with their actual purchase amount. Profile values stay editable before
creation and in the printer cost editor. Printer creation can accept partial
cost inputs; the backend merges these over current workspace defaults and
validates the complete result before writing the profile.

## Migration, backups and rollback

- Additive printer cost_config and nullable job printer_id/printer_cost columns;
  no inferred printer or historical job assignments.
- Existing jobs and global cost settings are untouched.
- New exports use schema version 4, preventing older builds from silently
  dropping cost snapshots.
- This build accepts schema 1, 2, 3 and 4 backups.
- Restoring schema 1 clears printer records along with the other replaced data.
- Restore validates printer metadata, configuration, referenced printer IDs,
  snapshot version/content and exactly one default when active printers exist,
  then replaces records inside the existing import transaction.
- Export a backup **before upgrading**. For downgrade, use that pre-upgrade
  schema-1 backup; do not edit manifest versions to force compatibility.

## Current limitations

- Profiles freeze cost configuration at assignment, not every part of the
  print record. Editing duration, filament use, labor or average purchase prices
  can still change calculated costs.
- Equipment cost aggregation currently uses the captured upfront printer cost;
  there is no per-printer equipment expense editor.
- Review regional preset data against current manufacturer information before
  relying on it for a purchase or cost estimate.

## Validation

Tests cover physical identity, default switching, archive/restore, metadata
isolation, malformed IDs/input, database uniqueness, backup round trips, legacy
restore, additive migration, snapshot stability, explicit reassignment,
advanced-rate normalization, cost-consumer consistency, and invalid-import
preservation.

Run before merging:

```bash
cd apps/api
gofmt -w models/printer*.go models/print_job.go dto/dto.go handlers/printer*.go handlers/print_job*.go handlers/desktop_data_handler.go
go test ./...
go vet ./...
```

Security review: request body bounded; metadata allowlist; UUID parsing;
parameterized queries; generic database errors. Desktop loopback/CORS and
Self-Hosted proxy boundaries remain in force. No new dependencies.
