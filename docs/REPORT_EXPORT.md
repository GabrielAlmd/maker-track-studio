# Reports and CSV data export

The desktop application offers **Reports & Data Export** in the main navigation. A full export is a portable record archive. A period report is an analytical archive. Neither ZIP can be imported as a desktop backup; use **Settings → Data & Backups** for restoration.

Period reports can also be saved as a PDF. The PDF is a printable summary and monthly trend table for the selected categories, generated from the same SQLite snapshot and calculations as the CSV report. Each page carries the Maker Track Studio logo and app name, and the PDF metadata identifies the app. It uses the application's English or Portuguese interface language and a bundled Inter font for UTF-8 text. Metric identifiers are mapped to explicit report labels, printer IDs resolve to the printer names in that report snapshot, and values use localized units and currency formatting. PDF dates use the selected locale (`Jan 2, 2006` in English, `02/01/2006` in Portuguese); period boundaries, generated times, and monthly grouping use UTC, which is stated in the PDF. CSV dates and numeric values remain canonical and locale-independent. The PDF does not contain underlying records; choose the CSV ZIP when those records are needed. Full data export remains ZIP only.

## Archive contract

The ZIP contains UTF-8 CSV files and `manifest.json`. `format_version` and `schema_version` are both `1`. The manifest also records the application name and version, UTC creation timestamp, mode, included datasets, and, for reports, selected categories and inclusive calendar dates. CSV headers are stable lowercase `snake_case` identifiers. Empty datasets still contain a header row. IDs are the original UUIDs. Null values are empty cells; an empty stored string also becomes an empty cell, so a consumer needing that distinction should use the SQLite backup.

Timestamps are ISO 8601 UTC values. Dates are interpreted on the UTC calendar. Money is in EUR (`EUR_cents` for integer cent fields); the manifest records the currency. Numeric values use a period as decimal separator and no thousands separator. Text beginning with a spreadsheet formula character (`=`, `+`, `-`, `@`), after leading whitespace, receives a leading apostrophe so spreadsheet software treats it as text. Remove that apostrophe only in a trusted data processing environment.

The full export includes every stored record in the datasets below, with no date filter. `app_config.csv` contains only recognized application configuration keys; unknown keys may contain secrets and are excluded. Diagnostic logs and temporary runtime files are not application records and are never included. Printer notes, order notes, and expense descriptions may contain personal information, so review archives before sharing them.

| CSV | Columns | Relationships and units |
| --- | --- | --- |
| `printers.csv` | `id,name,manufacturer,model,notes,is_default,is_archived,cost_config,created_at,updated_at` | `cost_config` is the stored JSON map. |
| `filaments.csv` | `id,brand,type,color,avg_price,personal_score,times_used,times_purchased,created_at,updated_at` | `avg_price` is currency per kg. |
| `filament_purchases.csv` | `id,filament_id,price,amount,purchase_date,created_at,updated_at` | `filament_id` → filaments; `amount` is kg; `price` is total currency units. |
| `models.csv` | `id,name,image_url,material_weight,print_time,waste_material,is_multicolor,created_at,updated_at` | Weights are grams; time is minutes. |
| `print_jobs.csv` | `id,printer_id,printer_cost,model_id,print_date,units_printed,labor_hours,print_time_minutes,created_at,updated_at` | Nullable IDs → printers/models. `printer_cost` is historical JSON; time is minutes. |
| `print_job_filaments.csv` | `id,print_job_id,filament_id,weight_used,created_at,updated_at` | IDs → print jobs/filaments; `weight_used` is grams. |
| `sales_channels.csv` | `id,name,notes,is_archived,created_at,updated_at` | Channel names and metadata. |
| `sales_orders.csv` | `id,reference,order_date,sales_channel_id,fulfilment_status,payment_status,packaging_cost_cents,shipping_cost_cents,platform_fee_cents,other_cost_cents,notes,production_cost_cents,total_cost_cents,net_profit_cents,margin_basis_points,created_at,updated_at` | Nullable channel ID → channels. Monetary fields are integer cents. Cost, profit, and margin fields are frozen snapshots when recorded. |
| `sales_order_items.csv` | `id,sales_order_id,model_id,model_name_snapshot,quantity,unit_price_cents,created_at,updated_at` | Order ID → orders; nullable model ID → models. Price is cents per unit. |
| `sales_order_allocations.csv` | `id,sales_order_item_id,print_job_id,units_allocated,production_cost_cents,created_at,updated_at` | Item ID → items; nullable job ID → print jobs. Cost is cents. |
| `app_config.csv` | `key,value,type,created_at,updated_at` | Only managed keys. `printer_equipment_costs` stores dated equipment expenses as JSON. |
| `equipment_expenses.csv` | `report_index,description,date,amount,category` | Flattened equipment expenses from `printer_equipment_costs`; entries have no persistent IDs. Dates can be empty. |

## Period reports

Monthly, quarterly, yearly, and custom ranges are inclusive on the displayed dates and implemented as `[start 00:00 UTC, day after end 00:00 UTC)`. Only selected categories appear. `summary.csv` has `category,metric,value,unit,kind`, where `kind` is `recorded`, `estimated`, or `snapshot`. `trends.csv` has `month,category,metric,value,unit,kind` and groups recorded activity by UTC calendar month; it contains no rows for months without recorded activity.

- Print jobs use `print_date`. The report includes job counts, units, recorded print minutes, the number of calculable and uncalculable jobs, and estimated production cost using the application's existing cost rules. Modern jobs use their stored printer cost snapshot; legacy jobs use the currently stored global configuration because no historical snapshot exists. Filament prices use the currently stored average price, so historical production costs remain estimates.
- Materials use the linked job's `print_date`. Consumption totals group by material type in grams. Consumed material cost is explicitly estimated from the current filament average price, since a historical per-job purchase price is not stored. The catalogue CSV is included as lookup data.
- Filament purchases use `purchase_date`. Recorded expenditure and quantity come from purchase rows. Dated equipment expenses from `printer_equipment_costs` appear in `equipment_expenses.csv` and the summary; undated entries cannot be placed in a period.
- Sales use `order_date`. Financial totals include only completed orders and their frozen cost and profit values. Revenue is the sum of item quantity × unit price. The report includes orders, items, allocations, and channels. Payment status is preserved but does not by itself establish receipt of cash.
- Printer usage groups jobs by `printer_id`. The printer catalogue is included to resolve IDs.
- Recent app events are optional and use the diagnostics logger's sanitized in-memory buffer from the current run only (up to 200 recent events). `application_events.csv` has `timestamp,level,event,component`; `summary.csv` reports the exported count. Retained diagnostic files, request IDs, messages, and free-form details are never exported. There is no historical activity database.

Catalogues without an event date (`models`, `filaments`, `printers`, `sales_channels`) are included in full when their category is selected. Their presence in a report does not imply activity within its period. Print jobs have no persisted completion or success/failure status, so those metrics are omitted. Sales orders have no completion timestamp. Historical diagnostic files remain under their own privacy and retention rules and are excluded from this format.

`equipment_expenses.csv` uses `report_index,description,date,amount,category`; its index identifies a row within that report, not a persistent ID. Expenses are currently stored within the `printer_equipment_costs` configuration value.

## Compatibility

Consumers should select files by name and columns by header. Future incompatible CSV changes require a new `schema_version`; archive structure changes require a new `format_version`. Full export never runs reporting calculations and does not modify the source database. The implementation reads a consistent SQLite snapshot and writes a temporary ZIP before presenting the file.
