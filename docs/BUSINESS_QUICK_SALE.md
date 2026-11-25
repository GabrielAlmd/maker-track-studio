# Quick Sale

Enable **Business Operations** in **Settings → Features**, then choose **Quick
Sale** from the Business Overview or Orders. Enter a reference, select a model
and enter the quantity and agreed unit price in EUR cents. You can select an
existing customer or leave the sale assigned to a walk-in customer.

Type part of the model or customer name, then tap a result or use the arrow
keys and Enter. Escape dismisses the search and keeps the previous selection.
For **Dispatch from stock**, search the current page by lot reference, date or
available quantity. Select the lot again after changing the model or quantity.

- **Make to order** confirms demand immediately. Use the order's **Production**
  tab to link runs and allocate accepted units when they're ready.
- **Dispatch from stock** uses a selected verified lot with enough units for
  the whole line. **Confirm and dispatch** records the order and dispatches the
  units immediately. If someone else commits that stock first, refresh the
  lots or change the quantity.

Use **Financials** to record receipts, administrative invoices or refunds.
The order's **History** records confirmation and any fulfilment events. Quick
Sale handles one model per order. Use the proposal workflow when a sale needs
multiple lines, formal terms or a customer-facing PDF.

## API and data contract

`GET /v2/business/quick-sales/stock/:modelID?quantity=1&page=1&page_size=25`
lists completed, verified lots with enough physical and commercial capacity.
Entries contain `print_run_item_id`, `print_date` and `units_available`. The
endpoint uses the standard Business pagination envelope with `items` and
`has_more`. Availability is checked again inside the creation transaction.

`POST /v2/business/quick-sales` accepts `operation_id`, an optional
`expected_revision` of zero, `reference`, `model_id`, `quantity` and
`unit_price_cents`. Optional `customer_id` identifies an active customer and
`stock_run_item_id` selects immediate dispatch. Quantity and price must be
positive integers. The total must fit within the JavaScript safe integer range.
A success returns HTTP 201 and the confirmed terms at revision 1. A replay
with the same operation ID and payload returns the original revision. Reusing
the ID with different inputs or selecting unavailable stock returns HTTP 409.
New sales require Business Operations to be enabled.

The order read at `/sales-orders/:id` exposes `source: "quick_sale"`,
`workflow_version: 2`, and `legacy_write_supported: false`. Quick Sale has no
accepted proposal. Its revision records a zero proposal UUID and revision 0.
A named sale stores the customer name at confirmation. Financial records use
the frozen order amount, and `estimated_profit_cents` is null for Quick Sale.
Stock movements remain in Inventory. Order, line, terms, allocation,
reservation and dispatch commit together when dispatching from stock. Payment
uses the separate versioned finance command. See [SQLite recovery](SQLITE_RECOVERY.md)
for migration and restore behavior.
