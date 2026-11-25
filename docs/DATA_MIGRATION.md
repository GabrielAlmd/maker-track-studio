# Guided data migration

Physical Inventory history transfers through a full workspace backup. CSV/XLSX imports do not establish an opening stock count. See the [Inventory ledger](INVENTORY_LEDGER.md).

For implemented INV-03 lifecycle behavior and schema 6, see [Production lifecycle](PRODUCTION_LIFECYCLE.md).

Open **Settings > Data Migration** to move existing tabular records into Maker
Track Studio. An empty workspace also links to the wizard from the Dashboard.
Migration is optional during first use and remains available later.

For the Models section, spreadsheet rows supply aggregate estimates. When a
duplicate update matches a model with [material requirements](MODEL_MATERIALS.md),
its existing material rows and derived totals are retained. Use a SQLite backup
to transfer requirement rows and their stable IDs.

## Supported sources and sections

- CSV imports one selected section at a time.
- XLSX imports one or more selected worksheets in one operation.
- Supported sections are Printers, Filaments, Purchases, Models and Print Jobs.

Download the CSV template for each section in the wizard. Template headings are
the stable application field names. Common alternative headings are matched
automatically, and each destination field can instead be mapped manually or
given a constant default.

Purchase and Print Job relationships use readable natural references:

- a filament is `Brand | Type | Color`;
- a printer is its name;
- a model is its name.

References must resolve to exactly one existing or planned record. The preview
reports missing or ambiguous references before persistence.

## Validation and duplicates

The preview classifies every source row as ready, warning, duplicate or invalid.
Required values, numeric boundaries, dates, booleans and relationships are
validated by the embedded API rather than trusted from the browser.

Each selected section has an explicit duplicate policy:

- **Skip** keeps the existing record unchanged.
- **Update** changes the matched record only after confirmation.
- **Create new** retains the existing match and creates another record.

The API never silently chooses an overwrite policy. Printers and Models match by
name, Filaments by brand/type/color, Purchases by filament/date, and Print Jobs
by printer/model/date.

## Safety, compatibility and rollback

Uploads are limited to 10 MB, 100 columns and 5,000 data rows per source. Files
are parsed in memory; formulas and macros are not executed and no supplied path
is opened. Only `.csv` and `.xlsx` are accepted.

Import repeats preflight validation and writes all selected sections inside one
SQLite transaction. Any database failure rolls back the entire operation. The
workflow uses the existing tables and UUID hooks, so there is no schema or ZIP
backup format change. Export a normal desktop backup before a large migration
and retain the original source file for recovery and audit purposes.

## Inventory after migration

Imported purchases are historical financial records. New imported Print Jobs retain reported production units and unverified material usage. Neither establishes a physical quantity. Updating a purchase that already posted stock applies only its change. Structured production runs retain their lifecycle and cannot be overwritten by a historical job import.

After import, weigh remaining filament and count finished units. Record each observed total and its current item revision using the [physical count procedure](INVENTORY_LEDGER.md#physical-stock-reconciliation). Until the first count or explicit opening balance, recorded changes carry `verified: false`, including when the displayed total is zero. Back up the reconciled workspace to retain its verification history.

## Business customers and budgets

Upgrade adds empty customer and budget tables. Existing Sales records keep their
IDs, allocations, statuses and financial snapshots. Full workspace ZIP backups
carry customers, budgets and revisions. See [Business budgets](BUSINESS_BUDGETS.md)
for the schema 12 upgrade and rollback procedure. CSV/XLSX sections continue to
cover the production entities listed above.
