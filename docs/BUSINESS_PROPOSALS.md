# Customer proposals

Create a proposal from an exact [budget revision](BUSINESS_BUDGETS.md) using the
Business Operations API. Desktop uses the loopback API. Self-Hosted uses the
authenticated HTTPS proxy. Business Operations screens are tracked in #25,
order confirmation and acceptance in #22, and PDF generation in #27.

1. Select an active customer and a budget revision. A budget assigned to a
   customer can be quoted to that customer. An unassigned budget can be quoted
   to any active customer. Supply a customer-facing title and descriptions.
   The proposal copies the budget's line quantities and estimated costs.
2. Enter a manual unit price, target markup, or target margin for each line.
   Optionally apply a fixed or percentage discount per line and on the
   order. The internal response compares the net quote with the estimate and
   shows profit, margin and markup after discounts.
3. Set the validity date and any delivery date, lead time, delivery terms and
   payment terms. Issue the draft to freeze its terms, totals and customer
   name/address. Fetch the document for that exact issued revision for
   customer-facing use.
4. To revise a proposal, save a new draft using the latest proposal revision.
   The same customer and budget IDs carry forward. You can select a different
   revision of the same budget. Issuing the replacement links it to the
   previous issued revision. Issued documents remain readable with their
   original terms and prices.

## API contract

Use `/v2/business` in Desktop or `/api/v2/business` through the Self-Hosted HTTPS
proxy. All responses are private and use `Cache-Control: private, no-store`.
Share the document payload only. Other endpoints expose confidential cost and
pricing inputs. The document endpoint also requires operator access.

| Method and resource | Behavior |
| --- | --- |
| `GET /proposals` | Summary of the latest revision of each proposal, ordered by creation time descending. Includes drafts. |
| `GET /proposals/:id` | Latest full internal comparison, or the exact revision selected by `?revision=N`. |
| `GET /proposals/:id/revisions` | Summaries of all immutable revisions, newest first. |
| `POST /proposals` | Creates a proposal and draft revision 1. Returns 201. |
| `PUT /proposals/:id` | Saves a complete new draft after the expected latest revision. Returns 200. |
| `POST /proposals/:id/issue` | Copies the expected latest draft into a new issued revision. Returns 200. |
| `GET /proposals/:id/document?revision=N` | Explicit issued revision in EUR, with customer name/address, commercial terms, line descriptions, quantities, applied discounts and net amounts. Draft or absent revisions return 404. |

The list endpoints return `items`, `page`, `page_size`, and `has_more`.
Defaults are page 1 and 25 items, with at most 100 per page. Each write uses an
`operation_id` UUID. Create accepts `expected_revision: 0` or omission. Update
and issue require the latest revision number. Retries with the same operation
ID and request return the original committed revision. Stale revisions,
archived sources and conflicting reuse of operation IDs return 409. Invalid
fields return 400, missing references return 404, and requests above 512 KiB
return 413. Errors follow the [budget API](BUSINESS_BUDGETS.md) shape.

A create or update takes `budget_id`, `budget_revision`, `customer_id`, `terms`,
`lines`, and optional `order_discount`. Terms include a title (160 characters
max), `valid_until`, `due_date`, `lead_time_days` (0–3,650), `delivery_terms`,
and `payment_terms` (2,000 characters each). Dates use `YYYY-MM-DD`. Validity
is required at issuance, and validity and any due date must be on or after the
current UTC date. Lead time is a separate number of days. The name/address are
copied from the active customer revision when saving a draft and carried into
the issued revision. To pick up contact edits, save a new draft before issuing.

Supply one line per budget item with its `budget_line_id` and a customer-facing
`description` (200 characters max), plus `price` and an optional `discount`.
The budget line UUID is the stable line key within that proposal. A price or
discount is an object with `mode` and `value`.

- Price mode `manual` takes a unit price in EUR cents, including explicit zero.
  Modes `markup` and `margin` take nonnegative basis points (2,000 = 20%) and
  require positive estimated unit cost. A target margin must be below 100%.
- Markup multiplies unit cost by `1 + rate`. Margin divides it by
  `1 − rate`. Both round half-up to cents before multiplying by quantity.
  For an €8 unit cost, a 20% markup gives €9.60 and a 20% margin gives €10.00.
- Discount mode `fixed` takes cents. Mode `percent` takes 0–10,000 basis points
  and rounds half-up. Omit a discount for zero. Apply line discounts first,
  then the order discount on the remaining subtotal. The order discount is
  distributed proportionally across lines, with leftover cents assigned by
  largest remainder and ties broken by budget line UUID. Net line amounts sum
  exactly to the quoted total.
- The internal `profit_cents` is net price minus estimated cost.
  `margin_percent` divides profit by net price and `markup_percent` divides it
  by estimated cost. Both are display percentages and return null for a zero
  denominator. Monetary inputs, snapshots and totals are integer EUR cents
  bounded by the JSON-safe integer limit. Excessive amounts return 400.

The issue request carries only `operation_id` and `expected_revision`. An
issued record has `issued_at` and, for a replacement, `supersedes_revision`.
A draft keeps `issued_at` null. Saving a draft leaves the earlier issued quote
current. Issuing its replacement records the link and timestamp atomically.
The order-confirmation workflow in #22 will decide eligibility and associate
acceptance with the exact issued revision. Proposal writes have no effects on
orders, payments, Print Jobs or Inventory.

## Upgrade and recovery

Sales module version 3 adds the SQLite table `business_proposals` and uses the
existing commercial operation ledger. Existing customers, budgets, legacy
sales and frozen order snapshots retain their IDs and data. The ZIP format
remains version 1 and full backups use schema 13. Supported schema 12 and older
archives restore with no proposals. Imports validate proposal, customer and
budget revision references, repriced totals, issued snapshots and operation
records before replacing the workspace in a transaction.

Export a backup before upgrading. To roll back the application version, stop
the app, retain the upgraded workspace separately, and restore the pre-upgrade
archive with the compatible previous binary. See [SQLite recovery](SQLITE_RECOVERY.md).
