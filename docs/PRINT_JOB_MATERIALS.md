# Print Job material weights

For implemented INV-03 lifecycle behavior and schema 6, see [Production lifecycle](PRODUCTION_LIFECYCLE.md).

See [Model material requirements](MODEL_MATERIALS.md) for entering per-unit
estimates and reviewing the known waste subtotal before recording run usage.

The Print Job form records one row per filament identity. Enter the total grams
consumed by that filament for the job, including physical waste such as purge
and supports. The displayed total is the sum of the entered rows. A model's
material and waste defaults provide a starting value for an empty form; they
are not measured consumption. Check and replace that value with the measured
weight before treating the job as an actual material record.

Changing model or quantity retains manually entered material splits. A filament
can appear only once in a job; combine its usage into one row. Existing row IDs
are kept when editing unchanged or updated weights. Legacy `weight_used` values
remain in grams with their original meaning and IDs.

The form stores a model-derived row as `estimated`; editing its grams marks it
`verified`. Each row stores total grams and optional physical waste grams, which
are included in that total. Waste on an estimated row is itself an estimate;
it becomes recorded physical waste only on a verified row. The model estimate remains available separately
after verification. Older rows are `legacy_unverified` until reviewed.

For rows with physical waste recorded, the cost-only
`filament_waste_factor` applies to non-waste grams, then the physical waste
grams are added once. Older rows retain their historical cost calculation,
which applies the factor to all `weight_used` grams. The factor never changes
recorded grams. Physical stock is not deducted by this version of Print Jobs;
the future Inventory ledger will deduct verified actual grams once and never
deduct slicer estimates or apply the cost-only factor to stock.

The Print Job form sends job fields and material rows in one request. The API
commits them together; invalid material IDs or failed writes leave the existing
job and its material rows unchanged. Clients that omit the `filaments` property
retain the older job-only update behavior. A supplied empty array removes all
material rows.

## API and compatibility

`POST /print-jobs` and `PUT /print-jobs/:id` accept an optional `filaments`
array of `filament_id`, `weight_used`, `measurement_state`, optional
`estimated_weight_g`, optional `waste_weight_g`, and, on update, existing row
`id`.
Rows with IDs must belong to that job. The write is transactional. The older
`POST /print-job-filaments` and `PUT /print-job-filaments/:id` retain the
`print_job_id`, `filament_id`, and `weight_used` JSON fields. The weight must be
finite and positive. A second row for the same filament in one job returns
HTTP 409; an invalid weight returns HTTP 400. `GET` responses retain their
existing fields and add estimate, waste, and measurement state fields.
Existing UUIDs and historical rows are not deduplicated. Three nullable/default
columns are added to the existing SQLite table; absent state means
`legacy_unverified`. Backup schema 5 includes these fields. Supported older
backups restore rows as legacy unverified without inventing estimates or waste.
A database with historical duplicate rows remains readable; operators should
reconcile them before editing that material split. For rollback to an older
binary, use the pre-upgrade backup; writes made after that backup are lost.
