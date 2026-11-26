# Quick Sale

Enable **Business Operations** in **Settings → Features**, then choose **Quick
Sale** from the Business Overview or Orders. Enter a reference, select a model
and enter the quantity and agreed unit price in euros, for example `12.99` or
`12,99`. You can select an
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

## Optional cost and price suggestion

Open **Estimate cost and price** after choosing the model, quantity and
fulfilment. For made-to-order sales, enter estimated print minutes and labor
hours for one unit and choose a printer profile or the app cost settings.
Quick Sale uses the same filament suggestions as production planning and uses
the budget cost engine for material, waste, power, wear and labor. Enter time
for one unit even when ordering multiple units. The estimate multiplies that
rounded unit cost by quantity. Enter a manual unit cost for batch economics
or when model requirements, a compatible filament mapping or prices are missing.
Use the manual cost to account for additional expenses.

Current finished stock lots provide verified accepted quantities. The production
records lack an immutable total cost attributed to each lot. Live filament
prices and shared costs from mixed runs are unsuitable for historical lot cost.
For stock dispatch, enter a manual cost based on your records or leave
estimation off. Both options retain the same stock availability checks.

**Margin** is profit divided by sale price. **Markup** is profit divided by
cost. For a €5.00 cost and a 40% target, the suggested unit price is €8.33
with margin or €7.00 with markup. Prices round half-up to cents. Choose
**Cost only** to track estimated profit using your own price. The suggestion
can be applied with one click and edited before confirmation. Quantity, model,
lot and cost assumptions affect the estimate. Calculate the estimate again
after changing them, or turn off estimation to continue with only a sale price.

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

`POST /v2/business/quick-sales/estimate` returns HTTP 200 with unit and total
cost in cents, an optional `suggested_unit_price_cents` and a preview `token`.
The body contains `model_id`, `quantity`, an optional `stock_run_item_id`
and a `cost` object. Provide one of:

- `source: "manual"` and positive integer `manual_unit_cents`.
- `source: "model"` for made-to-order, integer `minutes_per_unit`,
  optional `labor_hours_per_unit` and `printer_id`, and a `filament_mapping`
  object keyed by material requirement UUID with the selected filament UUID
  as its value. Each nonzero weight must map to an active compatible filament
  with a positive price. Budget estimate limits apply, including 1,000,000
  units and 20 distinct filaments for a model estimate.

An optional `cost.target` object contains `mode` (`margin` or `markup`)
and nonnegative basis-point `value` (4000 represents 40%). Margin must be
less than 10000. Calculated costs and suggestions must be positive and their
line totals must fit within the JavaScript safe integer range. A stock estimate
also checks that the specified lot can still cover the sale.

`POST /v2/business/quick-sales` accepts `operation_id`, an optional
`expected_revision` of zero, `reference`, `model_id`, `quantity` and
`unit_price_cents`. Optional `customer_id` identifies an active customer and
`stock_run_item_id` selects immediate dispatch. Quantity and price must be
positive integers. The total must fit within the JavaScript safe integer range.
To confirm with estimated costs, also send the preview's
`cost` inputs and `estimate_token`. The server checks the estimate in the
same write transaction and returns HTTP 409 if its inputs have changed.
`unit_price_cents` remains the operator's final price. A success returns HTTP 201 and the confirmed terms at revision 1. A replay
with the same operation ID and payload returns the original revision. Reusing
the ID with different inputs or selecting unavailable stock returns HTTP 409.
New sales require Business Operations to be enabled.

The order read at `/sales-orders/:id` exposes `source: "quick_sale"`,
`workflow_version: 2`, and `legacy_write_supported: false`. Quick Sale has no
accepted proposal. Its revision records a zero proposal UUID and revision 0.
A named sale stores the customer name at confirmation. Financial records use
the frozen order amount. Confirmed estimates are included in the private
order revision's `quick_sale_pricing` snapshot, with the cost source and
applicable assumptions, target and suggestion. Financials derives
`estimated_profit_cents` from that frozen cost and the final amount. Sales
confirmed without estimation and older Quick Sales retain a null profit
estimate. Estimates are independent of realized profitability.
Stock movements remain in Inventory. Order, line, terms, allocation,
reservation and dispatch commit together when dispatching from stock. Payment
uses the separate versioned finance command. See [SQLite recovery](SQLITE_RECOVERY.md)
for migration and restore behavior.
