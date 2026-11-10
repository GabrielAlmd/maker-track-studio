# Advanced cost settings

Settings exposes machine, labor and material-cost defaults for new printers.
Edit existing machines under **Printers > Edit costs**. For a worked example,
see [your first print](FIRST_PRINT.md). Each setting has a help tooltip available on hover, keyboard focus or tap.
Inputs are grouped by purpose and can be edited directly. The preview updates as
you type, including intermediate values in Advanced mode. Decimal commas are
accepted. Use **Save changes** to apply the form or **Discard changes** to return
to the saved values. The mode selector immediately updates field locks in the
draft; saved defaults are copied into newly created printers.

The existing API saves changed keys sequentially, with the mode saved last. If a
request fails, the form keeps the draft and identifies that some changes may
already have been saved. Retry writes only remaining changes; Discard returns
to the latest successfully saved values.

Select **Advanced** under **Calculation mode** to use:

| Result | Formula |
| --- | --- |
| Total investment | Purchase cost + additional upfront cost |
| Lifetime cost | Investment + annual maintenance × lifetime years |
| Annual printing hours | 8760 × uptime percentage / 100 |
| Capital cost/hour | Lifetime cost / (annual printing hours × lifetime years) |
| Electrical cost/hour | Average power in watts / 1000 × electricity tariff |
| Total machine cost/hour | (Capital + electrical cost/hour) × buffer factor |

Maintenance is editable independently. Configure these values for the printer
and electricity tariff being modeled; examples from an imported workbook are
not defaults or recommendations.

For each job, filament cost remains grams / 1000 × average filament €/kg × waste
factor (C4), summed across its filaments. Labor cost is job labor hours × hourly
labor rate (C6). Printing hours use the job's explicit minutes, including zero,
or the model duration when absent. Machine cost is printing hours × machine rate.
The cost breakdown allocates the buffer to both electricity and wear/capital,
so their sum equals the machine cost without counting electricity twice.

Selling prices retain the existing margin formula: total / (1 − margin / 100).
Costs and prices describe the whole job; divide by units printed for a piece.
Intermediate values retain precision; only displayed totals are rounded.
All job views and dashboard aggregates use the same configuration loader.
Assigned jobs use their saved printer configuration; changing defaults or
editing a printer does not replace those saved inputs. Legacy jobs without a
printer snapshot still use current global settings. Reassigning a job captures
the new printer configuration. Material purchase averages, job duration and
labor remain live inputs, so a printer snapshot is not a frozen total. Completed
Sales orders retain their frozen profitability. See [printer profiles](PRINTER_PROFILES.md)
and [Sales history](SALES_AND_ORDERS.md).

## Compatibility and rollback

No schema migration is required. The eight new settings use the existing
`app_config` key/value table. Missing advanced keys have virtual defaults on read
and are persisted on edit, including immediately after restoring an old backup.
Existing rows are never overwritten by defaults. The default mode is **Simple**
(`machine_cost_mode=0`) so upgrading or restoring an old database preserves costs.
This retains the historical calculation: printing hours × electricity value,
plus printing hours × manual wear rate. The old electricity calculation implicitly
assumes 1 kW; only Advanced mode uses the configured wattage and buffer.

Export a backup before changing costing assumptions, especially for legacy
unassigned jobs. New values travel through the existing SQLite backup/export/import flow without changes to its format.
Switching back to Simple restores the old calculation and its saved wear rate;
restore a backup to undo edits to shared electricity, labor, waste or margin values.
Older application versions ignore the new keys and use the manual calculation.
No recovery-route, CORS or loopback behavior changes are required.

In Simple mode, investment, lifetime, uptime, power and buffer fields are locked.
In Advanced mode, the manual wear rate is locked because it is calculated.
Shared electricity, labor, waste and margin inputs remain editable in both modes.
Switching modes preserves all saved inputs. Tooltips remain available on locked
fields and can be dismissed with Escape or by tapping outside.

## Equipment and improvements

The former Additional Upfront Cost input is now an editable list of expenses:
**description**, optional **date**, and **amount (€)**. Add or remove rows, edit
values directly, and use Save changes to persist them. The sum updates the live
investment preview. Editing is available in Advanced mode; switching to Simple
preserves the expenses. Avoid adding repairs already covered by annual maintenance.

Older databases expose their existing upfront amount as a virtual “Previous
additional costs” expense, with no invented purchase date. A zero amount produces
an empty list. Reads do not modify the database; the first list edit persists it.
The workbook default amount is used only when the old key is absent.

The existing app_config table stores `printer_equipment_costs` as a JSON array.
Saving the list and its numeric `printer_upfront_cost` sum is one SQLite
transaction. The cost engine continues using that sum, so older versions can
still calculate costs and the backup format is unchanged. An empty list writes
a zero total; the aggregate cannot be edited or deleted independently once the
list exists. API validation limits the list to 500 entries, descriptions to 200
characters, valid dates, and nonnegative amounts with a maximum total of 1e9.

Backups retain both the list and its total. Export before downgrading: older
versions can edit the numeric total without updating the list, so restore that
backup before returning to this version if costing settings were changed there.
