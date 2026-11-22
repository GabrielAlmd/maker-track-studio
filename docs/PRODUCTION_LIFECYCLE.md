# Production lifecycle (INV-03)

Use **Print Jobs → Plan production** to create one or more physical runs. Each
run contains its own model quantities and material rows. Save the plan, reopen
it and start the run. Record actual time, labor and measured material, then
complete, fail or cancel it. Add another run for a physical reprint. This flow
works with Business disabled in both Desktop and Self-Hosted.

The existing **Add Print Job** flow remains a legacy completed record. Its
reported units are unverified history, not newly verified finished goods.

## States and quantities

- `planned → in_progress | cancelled`
- `in_progress → completed | failed | cancelled`
- Terminal executions cannot reopen, change quantities/materials or be deleted.
  Their source IDs remain available for Inventory and commercial references.
- Every item has positive planned units, with attempted at most planned and
  accepted + rejected at most attempted. Terminal counts must reconcile exactly.
- Failed/cancelled runs accept zero units. Their measured material remains
  consumed. Estimated grams never represent recorded physical consumption.
- Material weights are finite, positive and at most 1e9 grams. Included waste
  stays part of the total; the existing financial waste factor is cost-only.
- Completing a verified catalog item credits its accepted pieces to Inventory.
  See [finished goods](INVENTORY_LEDGER.md#finished-goods) for source tracing,
  reservations, dispatch and stock reconciliation.

Terminal runs retain their original output and source IDs. Use a new run to
record a reprint and a physical count to reconcile a shelf discrepancy.

## API contract

`POST /v2/production/jobs`, `GET /v2/production/jobs/:id`, and
`PUT /v2/production/jobs/:id` expose an aggregate. Writes contain:

```json
{
  "operation_id": "UUID generated once for this save attempt",
  "expected_revision": 0,
  "printer_id": "existing printer UUID",
  "print_date": "2026-11-17T00:00:00Z",
  "runs": [{
    "status": "planned",
    "labor_hours": 0,
    "print_time_minutes": null,
    "items": [{"planned": 2, "attempted": 0, "accepted": 0, "rejected": 0}],
    "materials": []
  }]
}
```

New runs/items/materials omit IDs; Go generates UUIDs. Subsequent writes send
the returned stable IDs and latest job revision. Responses contain job ID,
revision, run/item/material IDs and the operation ID. The same operation and
payload returns the exact committed response, including after backup restore.
Reusing the operation with changed content or a stale revision returns 409.
Omitting an existing run/item/material, supplying another parent's ID, or
changing terminal execution data also returns 409 without partial writes.
All reads/checks/writes and the replay record share SQLite's writer transaction.

Runs contain at most 100 items and 100 material rows, jobs at most 100 runs.
Counts and durations are bounded at 1e9, labor at 1e6 hours. Planned runs may
contain estimated material only and no actual time/labor. Blank duration is
unknown, distinct from measured zero. A job's printer is fixed after creation.
Recorded material cannot be erased or reduced after execution starts.

The current single-model API remains supported for legacy records. Reads of
new jobs expose `workflow_version: 2`, `legacy_write_supported: false`, revision,
no singular model ID and an accepted-unit total in the legacy count projection.
Their legacy job/material mutations are rejected. Core UI opens the run editor;
legacy Sales allocation cannot use their null singular model. #23 owns the
run-item allocation adapter and mixed-model financial attribution. Existing
commercial snapshots and legacy cost behavior remain unchanged.

## Migration, backups and rollback

Backup ZIP format remains 1; schema is now 6. Additive columns and new
`print_runs`, `print_run_items`, `production_operations` tables preserve existing
job and material IDs. Startup and older-backup restore create one synthetic
`legacy_unverified` completed run/item per historical job, retain reported
units separately, and attach original material rows without copying consumption.
No accepted output or opening stock is inferred. Normalization is repeatable.

Backup import validates production relationships before replacing data and
restores runs, items, material links and retry receipts in the same transaction.
Supported older archives normalize through the shared desktop/standalone path.
The `core_production` compatibility marker in `module_versions` makes preceding
builds reject upgraded databases instead of ignoring the new tables. Core
recognizes this marker independently of optional Business module composition.

Back up before upgrading. Rollback means restoring the pre-upgrade backup with
its compatible binary; there is no destructive downgrade. Changes made after
that backup will not be present in the restored copy. Full packaged Desktop,
LAN and joint v1.1.0 Inventory/Business qualification remain release gates.

## Slicer estimates (INV-10)

See [supported slicer imports](SLICER_IMPORT.md) for editable plate/model and
material estimates. Runs now expose nullable `estimated_time_minutes`, separate
from actual time. Backup schema 19 adds this nullable column without changing
historical values or IDs; older backups restore it as unknown.
