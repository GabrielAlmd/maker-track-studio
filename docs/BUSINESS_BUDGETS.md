# Customers and internal production budgets

Issue #20 adds the Business Operations API for reusable customers and internal
production estimates. Enable **Business Operations** in **Settings > Features**
to use **Customers** and **Budgets** in Desktop or Self-Hosted. The feature
preference is stored as `sales_orders`. Legacy order records are available from
**Orders**, and Inventory stays available with Business Operations disabled.

## Prepare an estimate

Use **Customers > New customer** and **Budgets > New budget** to enter a first
estimate. The saved budget shows its internal total and can start a proposal.

1. Create a customer with a display name and optional email, phone and address,
   or leave the budget unassigned. Archive a customer to remove it from new
   assignments while retaining it on existing budgets.
2. Create a budget with at least one line. Select a Model or supply an ad-hoc
   line name. A Model provides the default line name. Enter the expected print
   minutes, labor hours, and filament grams **per unit**, including known waste
   when available. Select an active printer profile to use its rates. Without
   one, the estimate uses the workspace cost settings.
3. Enter per-unit post-processing and other costs in EUR cents and set the
   quantity. Review the estimate privately. Budget endpoints and workspace
   backups can contain both customer information and confidential cost inputs.
   Share customer proposals through the [proposal document contract](BUSINESS_PROPOSALS.md).
4. Edit using the current revision and existing line IDs. Each successful edit
   creates a revision. Title, line name, customer and quantity changes retain
   unchanged lines' cost sources. Changing a line's assumptions captures
   current source values for that line. Archive a budget to suspend edits, or
   restore it to draft to resume.

An estimate records assumptions and the calculator version, capture time, printer
settings and material price sources used. Deleting a catalog Model or changing
filament or equipment rates leaves those snapshots readable. Estimates do not
create Print Jobs, record actual costs, reserve stock or modify Sales history.

## API contract

All routes are under `/v2/business`. Desktop uses its loopback API. Self-Hosted
uses `/api/v2/business` through the authenticated HTTPS proxy. Responses use
`Cache-Control: private, no-store`; the existing local or authenticated LAN
operator boundary applies. A UUID or the optional module display setting is
not an access control.

| Method and resource | Behavior |
| --- | --- |
| `GET /customers`, `GET /budgets` | Latest revision of each record, including archived records. `page` defaults to 1 and `page_size` to 25, max 100. Returns `items`, `page`, `page_size` and `has_more`. Budget list entries are summaries. |
| `GET /customers/:id`, `GET /budgets/:id` | Latest full record, or a specific immutable revision with `?revision=N`. |
| `POST /customers`, `POST /budgets` | Creates a record and returns 201, a stable ID and revision 1. Omit `expected_revision` or use 0. |
| `PUT /customers/:id`, `PUT /budgets/:id` | Submits the complete next revision and returns 200. Supply `expected_revision`, preserve IDs for retained budget lines, omit IDs on new lines, and omit removed lines. Set `is_archived` to archive or clear it to restore. |

Every write requires an `operation_id` UUID. Retry with the same ID and
decoded request values to read the original committed revision. Reusing an ID
for a different command or writing with a stale revision returns 409. IDs
from other budgets and IDs from removed lines cannot be assigned to new lines.
Errors include `error.code`, `error.message` and, where applicable,
`error.field` or `error.current_revision`. Invalid fields return 400, missing
records/references return 404, and bodies exceeding 512 KiB return 413.

Customer writes accept `name`, `email`, `phone`, `address` and `is_archived`.
Here is a budget create request using an ad-hoc line:

```json
{
  "operation_id": "6b047dee-77d5-4e21-9c90-1d4f37e483b1",
  "title": "Sample production",
  "customer_id": null,
  "items": [
    {
      "name": "Prototype housing",
      "quantity": 4,
      "assumptions": {
        "print_time_minutes": 90,
        "labor_hours": 0.25,
        "post_process_cents": 200,
        "other_cents": 50
      }
    }
  ]
}
```

A line optionally accepts `model_id` and its assumptions optionally accept
`printer_id` and up to 20 distinct `materials` entries with `filament_id`,
`weight_g` and optional `waste_g` (included in `weight_g`). Quantities are
whole units from 1 to 1,000,000, with at most 100 lines. Cost amounts are EUR
cents and limited to JSON-safe integers. The shared production calculator
computes the material, energy, wear and labor components from the captured
inputs. The budget rounds each component to cents per unit, sums the
components and the two manual cost categories, multiplies by the line
quantity, and sums line totals. This makes displayed components, unit costs
and totals reconcile exactly.

## Upgrade and recovery

The Sales module moves from version 1 to 2. Its migration adds
`business_customers`, `business_budgets` and `business_budget_operations`
without backfilling Sales orders or changing their IDs, allocations or
snapshots. Full ZIP backups include revisions and operation records starting
in schema 12. See [SQLite recovery](SQLITE_RECOVERY.md) for the current schema.
Supported older archives restore with empty customer and budget tables. Imports
validate references, revisions, estimates and retries before replacing
workspace data in one transaction.

Before an upgrade, save an SQLite ZIP backup using the prior version. For
rollback, stop the app, retain the upgraded workspace separately, and restore
the pre-upgrade backup with its compatible binary. Rollback returns the
workspace to the backup time. See [SQLite recovery](SQLITE_RECOVERY.md).
