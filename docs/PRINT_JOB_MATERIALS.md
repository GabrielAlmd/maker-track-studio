# Print Job material weights

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

The current cost engine multiplies `weight_used` by the configured financial
`filament_waste_factor`. This factor changes estimated cost only. It does not
change the recorded grams, and no physical stock is deducted by this version of
Print Jobs. When physical waste is already included in recorded grams, set the
factor to 1 to avoid charging for it again. The future Inventory ledger will own
stock deductions, with estimate, verified actual, and waste provenance as
specified in [the production contract](architecture/PRODUCTION_V1_1.md).

Print Job details and material rows are currently saved by separate API calls.
If a later request fails, the earlier change may have been saved; reopen the
job and verify the material rows before retrying. Atomic aggregate writes are
planned in INV-03 (#11).

## API and compatibility

`POST /print-job-filaments` and `PUT /print-job-filaments/:id` retain the
`print_job_id`, `filament_id`, and `weight_used` JSON fields. The weight must be
finite and positive. A second row for the same filament in one job returns
HTTP 409; an invalid weight returns HTTP 400. `GET` responses remain unchanged.
Existing UUIDs and historical rows are not migrated or deduplicated. This
change adds no tables or columns, so backup schema 4 and older backup restore
behavior are unchanged. A database with historical duplicate rows remains
readable; operators should reconcile them before editing that material split.
