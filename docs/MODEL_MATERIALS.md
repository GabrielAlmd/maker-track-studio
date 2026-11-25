# Model material requirements

In **Models**, open a model by name to see its material breakdown, or choose
**Edit** to maintain it. Add one requirement for each material or color you
want to plan separately. Enter a material type, such as PLA or PETG, and a
part weight in grams for one finished unit. Color can be left blank. Select a
preferred catalogue filament if you have one. The preference can be changed or
cleared independently of the estimated type, color and weights.

Enter zero waste if it is known to be zero. Leave waste blank if it is
unknown. The known waste subtotal is shown as incomplete when any row has
unknown waste. Shared purge, support or other overhead for an entire plate
belongs on the print run. For a Print Job, `weight_used` includes physical
waste, and `waste_weight_g` is the part of that total identified as waste. See
[Print Job material weights](PRINT_JOB_MATERIALS.md).

Two distinct known colors establish a multicolor model. If all rows have the
same known color, the model is single-color. Otherwise, missing colors leave the
color classification unknown. Materials are classified by distinct material
types. Comparison trims whitespace and ignores case. Older models keep their
original aggregate weights and multicolor flag and have an unknown breakdown.
You can edit those estimates or provide a breakdown when it is known.

## Slicer review

Inspect supported text G-code summaries or Bambu 3MF layouts using the file
picker. A single G-code summary can create draft rows with the original tool
numbers, including gaps for unused tools. The tool grams reported by G-code
cover the plate and can include waste. The review shows them beside the draft.
Enter the material type and part weight per finished unit before saving, and
add color and per-unit waste if known. The displayed plate usage and time are
not used to fill model estimates. Layout-only 3MF or multiple plates are
available for review without creating requirements. Unsupported or malformed
files leave your current estimates available for editing. Supported formats
and input limits are listed in [Slicer imports](SLICER_IMPORT.md).

## API and storage

- `GET /models` and `GET /models/:id` return ordered `material_requirements`
  and a `material_summary` with `part_weight_g`, `known_waste_g`,
  `waste_complete`, `color_status` and `material_status`. Classification values
  are `single`, `multiple` and `unknown`. An empty array means the split is
  unknown. Read the legacy aggregate fields to retrieve its old estimates.
- `POST /models` and `PUT /models/:id` accept `material_requirements` with
  `material_type`, optional `color`, required `part_weight_g`, nullable
  `waste_weight_g`, nullable `preferred_filament_id` and optional `source_tool`
  (one-based). Supply each row's server-issued `id` to retain it on update.
  The array order sets the stored position. A request may contain up to 100
  rows. Weights must be finite and nonnegative, and the combined part and
  known waste weight must be at most 1,000,000,000 g.
- Omitting the array, or sending null, preserves existing requirements.
  Sending an empty array explicitly removes them and uses the supplied
  legacy aggregate values. For structured models the existing
  `material_weight`, `waste_material` and `is_multicolor` fields are projections
  of the requirement rows. They represent total part weight, known waste and
  positively established multicolor status. API list filters still use these
  fields. Model edits do not write job, inventory or financial records.
- Spreadsheet migration accepts aggregate model columns. Duplicate updates
  preserve the breakdown and its projections on models with requirements.
  CSV data export includes the structured rows as a separate dataset. Use the
  SQLite backup to move a workspace and retain its IDs and associations.
- Desktop and Self-Hosted startup add the requirement table without backfilling
  legacy splits. Backup schema 20 retains requirement IDs, order and references.
  Older backups restore with no requirements. To roll back the binary, restore
  a backup made before upgrading with the matching build. Data entered after
  that backup is excluded from the rollback.

See [SQLite recovery](SQLITE_RECOVERY.md) and [Report export](REPORT_EXPORT.md).
