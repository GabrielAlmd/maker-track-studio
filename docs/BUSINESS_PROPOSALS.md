# Customer proposals

Open **Business Operations > Proposals > Choose a budget to quote** or use
**Create proposal from budget** on a saved [budget](BUSINESS_BUDGETS.md).
The draft uses that budget revision. Save the draft to see its internal cost
comparison. After setting a validity date, use **Issue proposal**, then **View
issued document** to review the customer-facing terms and prices. PDF export
and attachments are tracked in #27.

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
Confirmation links the order to the exact issued revision. Proposal writes have no effects on
orders, payments, Print Jobs or Inventory.

## Upgrade and recovery

Sales module version 3 adds the SQLite table `business_proposals` and uses the
existing commercial operation ledger. Existing customers, budgets, legacy
sales and frozen order snapshots retain their IDs and data. The ZIP format
remains version 1. Schema 13 introduced proposals. Schema 14 adds order acceptance and amendment history. Supported schema 12 and older
archives restore with no proposals. Imports validate proposal, customer and
budget revision references, repriced totals, issued snapshots and operation
records before replacing the workspace in a transaction.

Export a backup before upgrading. To roll back the application version, stop
the app, retain the upgraded workspace separately, and restore the pre-upgrade
archive with the compatible previous binary. See [SQLite recovery](SQLITE_RECOVERY.md).

## Confirm and correct an order

After the customer accepts, send `POST /v2/business/orders` with a fresh
`operation_id`, `expected_revision: 0`, `proposal_id`, `proposal_revision`
and optional `purchase_order` (up to 160 characters). Select the latest
**issued** revision. A newer draft leaves it eligible until another revision is
issued or the validity date passes in UTC. A successful response (201) gives
the new Sales Order ID, revision 1, customer ID, accepted proposal reference,
confirmation timestamp, PO and delivery terms. Save the operation UUID for
retries. A second independent acceptance returns 409.

In the browser or desktop app, open the issued document and choose **Confirm
customer acceptance**. This creates the order and opens its **Summary**. Use
**Production** to assign verified work, **Financials** to review order totals,
**Documents** to open the exact accepted revision, and **History** to review the
original acceptance and later corrections. Use **Correct order terms** in
Summary on confirmed or preparing orders to update the PO or delivery terms.
Confirmation creates unpaid demand in one SQLite transaction. Production
assignments leave Inventory reservations, invoice records and payment
records to their own workflows.

### Plan production and allocate accepted units

Open **Production** on a confirmed or preparing proposal-backed order.
Each line shows the required, planned, accepted, allocated and outstanding
units. Use **Find run items** to select items for the line's catalog model. Add
items from several Print Jobs to split the work across jobs or plates. The
model comes from the budget revision that fed the accepted proposal. Lines
without a catalog model can show demand and progress but cannot link run items.

- Enter planned units for a planned or in-progress run item. The plan can
  exceed the ordered quantity for reprints or extras, within the source's
  planned capacity. A failed or cancelled run remains visible in the links
  while its planned units drop out of the line total.
- Once a run is complete, enter allocated units to assign its verified accepted
  output to the line. Different orders can share a run item's accepted units
  up to its available capacity. A completed run shows its accepted quantity and
  how much remains available across orders.
- Outstanding units equal the required units minus the accepted units assigned
  to the line, with a minimum of zero. Accepted and allocated totals are equal
  in this workflow because allocation is how an accepted unit is assigned.
  Legacy single-model Sales Orders continue to use their Print Job allocation
  editor and retain their historical records.

Save the plan to apply all changes together. To release a link, remove it and
save. If the plan changed in another window, reload the saved plan before
editing it again. Use **Refresh production** to load run outcomes or allocations
recorded elsewhere. Review actual attempted, accepted and rejected units in the
Print Job before completing the run. An in-progress run can still be corrected
before inspection is final. For a failed or short run, keep the original record,
plan a replacement run, and allocate its accepted units after inspection.

A line is **Complete** when its required quantity is covered by verified accepted
units assigned to that line. The order's production is **Complete** when every
line is complete. Reducing or releasing an assignment can make demand outstanding
again. A production status change leaves order fulfilment, invoices, payment and
frozen financials untouched. Inventory records finished goods when core production
posts verified accepted output. Physical reservation and fulfilment integration
are tracked by [#26](https://github.com/GabrielAlmd/maker-track-studio/issues/26).

#### Order production API

- `GET /v2/business/orders/:id/production` returns the production revision,
  editability, line totals and links. GET and PUT responses include a derived
  `status` on the order production document and on each line. Possible values
  are `not_started`, `planned`, `in_progress`, `complete` and `cancelled`.
  Linked execution, including a failed attempt, can make progress
  `in_progress` while units remain outstanding. A change in the core run may
  change progress without changing the order's production-link revision.
  Each link includes the Print Job,
  run and run-item IDs, status, planned and accepted source capacity, and
  remaining accepted units available to allocate.
- `GET /v2/business/orders/:id/production/sources?line_id=<uuid>` lists compatible
  verified run items in pages of up to 50. Pass the preceding response's
  `next_cursor` as `after` for another page.
- `PUT /v2/business/orders/:id/production` replaces the plan and allocations
  together. Send `expected_revision` and a `links` array. Each link has
  `sales_order_item_id`, `print_run_item_id`, `units_planned` and
  `units_allocated` as IDs and whole, nonnegative quantities. Omit a link to
  release it or send an empty array to clear the plan. A link must have a
  positive planned or allocated quantity. Saving the same set again preserves
  allocation IDs and the revision. A changed set with a stale revision,
  unmatched model, invalid order status or exceeded capacity returns 409.

The existing Sales allocation endpoints remain available. Their read response
includes an optional `print_run_item_id` for new run-item allocations. The new
order endpoints use a separate production revision from the commercial
amendment revision.

Production status is computed from existing facts on each read or save. This
change needs no SQLite migration, backup schema change or data backfill. Existing
backups restore the same production facts and recompute progress on read.

### Amend confirmed terms

To correct a PO or delivery instruction on a confirmed or preparing order, send
`POST /v2/business/orders/:id/amendments` with `operation_id`, the current
`expected_revision`, a `reason` (1–500 characters), and the changed
`purchase_order` or `delivery_terms` (up to 2,000 characters). An empty string
clears that field. The local desktop operator or authenticated LAN operator can
perform this command. Read the current version with `GET /v2/business/orders/:id`
or append `?revision=N` to inspect an earlier version. A successful correction
returns 200 and appends the next revision. Price, customer, quantities and
approved revision remain frozen. Unknown fields return 400. A stale revision
or protected lifecycle state returns 409. Retry an identical command with the
same operation ID to retrieve its original result.

Legacy order reads include `workflow_version`, `legacy_write_supported`,
`legacy_projection_lossy` and line `net_cents`. For formal orders, use the
discounted line total or profitability revenue for financial calculations.
Legacy writes on those orders return 409. For a legacy draft PUT, include the
existing item `id` on retained lines. ID-less legacy requests match model
lines in their original order. Release affected allocations before removing a
line, changing its model or reducing quantity below the assigned units.

## Proposal PDFs and order documents

Open an issued proposal and select **Download proposal PDF**. A confirmed
order also provides the accepted revision under **Documents**. Desktop opens a
native Save dialog. Self-Hosted downloads the file through the browser. The
PDF uses the interface language and prints the frozen customer details, unit
prices, applied line and proposal discounts, net total, expiry and agreed
delivery and payment terms. You can export an earlier issued revision even
after issuing a replacement. Budget assumptions, costs, profit, margin,
markup and internal notes are excluded. Check your customer-facing descriptions
and terms before sharing the PDF.

Under **Documents**, attach a PDF, PNG or JPEG to keep purchase orders,
collection evidence and other supporting files with the order. Each file can
be up to 5 MiB, with up to 50 attachments on an order and 32 MiB of attachment
data across the workspace. Images can contain up to 16 million pixels. Files
are available as downloads. ZIP backups include the original attachment bytes.
Deleting a legacy draft also deletes its attachments.

Open **History** for the accepted proposal revision, amendment reasons,
production plan revisions, linked run transitions, fulfilment movements and
attachment activity. Existing commercial revisions and stock movements appear
after upgrade. Linked run and plan events start when the upgraded app records
them. Earlier legacy orders can have an empty activity history.

### API contracts

These endpoints are under `/v2/business` and return `Cache-Control: private,
no-store`.

- `GET /proposals/:id/document/pdf?revision=N&language=en` returns an
  `application/pdf` download for an explicit issued revision. Pass `pt-PT`
  for Portuguese. The existing JSON document includes `unit_price_cents`
  on each line as an additive field.
- `GET /orders/:id/attachments` lists each attachment's stable ID, name,
  validated media type, size in bytes, SHA-256 checksum and creation timestamp.
- `POST /orders/:id/attachments?name=<filename>&operation_id=<uuid>` accepts
  a bounded raw body with `Content-Type: application/octet-stream`. The
  server validates the filename and checks the payload against the extension.
  It checks PDF signatures and decodes images within the pixel limit. PDFs
  are handled as opaque downloads. Business Operations must be enabled to
  upload. Success returns 201 and metadata. A matching retry returns 200 and
  the same metadata. A different order, name or payload with that operation
  ID returns 409.
- `GET /orders/:id/attachments/:attachmentID` verifies attachment ownership
  and checksum and serves an octet-stream download with a safe filename and
  `X-Content-Type-Options: nosniff`.
- `GET /orders/:id/activity?page=1` returns `items`, `page` and `has_more`
  in pages of 25, newest first. Entries include a stable event identity, kind
  and timestamp plus the applicable revision, reference ID, detail or
  quantity. Order revisions and fulfilment movements keep their existing
  authoritative records. Production and cancellation activity is appended in
  the same transaction as the underlying change.

SQLite stores attachment bytes, metadata and order activity in the Business
module. The upgrade introduces Sales module version 6 and backup schema 16
with the same ZIP format. See [SQLite recovery](SQLITE_RECOVERY.md) for restore
validation and rollback.

### Record invoices and payments

Open **Financials → Invoices and payments** on a proposal-backed order.
Enter the external invoice reference, issue date, amount and optional due date.
You can record an invoice before or after shipping. This is an administrative
register. Use your invoicing system to issue legal fiscal documents.

Record each receipt as a **Payment**. The outstanding balance is the agreed
price after discounts minus net receipts. Receipts cover overdue invoiced
amounts first. An invoice is overdue from the day after its due date (UTC)
when those amounts exceed net receipts. The estimated proposal profit uses the
accepted revision's price and estimated costs. Actual profitability uses the
existing order cost data and completed snapshots.

For a correction, choose **Invoice void** and select the original invoice,
enter its full amount and give a reason. Record a replacement if appropriate.
Choose **Refund** to reverse all or part of a particular payment. Original
entries, reversals and their references remain visible in Financials and their
events appear in History. Legacy Sales orders retain their existing payment
labels and frozen profitability.

#### Finance API

`GET /v2/business/orders/:id/finance` returns the ordered entry history,
payable, invoiced, net paid, outstanding and estimated profit in EUR cents,
independent invoice/payment statuses and the overdue flag.
`POST` to the same resource appends one entry. For example, the first receipt
on an order can be recorded as follows.

```json
{
  "operation_id": "5f5d90e0-b52b-4954-a756-2261daa812c6",
  "expected_revision": 0,
  "kind": "payment",
  "reference": "Workshop deposit",
  "date": "2026-09-19",
  "amount_cents": 2500
}
```

Send the current entry count as the expected revision. Keep the operation UUID
and payload for retries. Success returns the immutable entry and its revision.
Dates use YYYY-MM-DD. Only invoices accept an optional `due_date`. Refunds
and voids require `original_revision` on the same order. Amounts are positive
integer cents. Malformed entries return 400, missing formal orders return 404
and stale revisions, excessive payments, invoice totals or reversals return
409. Writes update the legacy payment projection in the same transaction.
