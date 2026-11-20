# Inventory ledger

Use a physical count to establish opening stock for each filament UUID or finished model UUID. Until then, the balance reports only recorded changes and carries `verified: false`. Filament quantities are integer milligrams (1,000 per gram). Model quantities are whole pieces. A negative balance flags a discrepancy and remains visible for investigation. Correct a discrepancy by posting a signed adjustment and recording the reason. The original entry stays in the audit history.

## Local API

Submit JSON to `POST /v2/inventory/adjustments`. This example records 250.5 grams of opening stock. Substitute the catalog item's UUID and generate two stable UUIDs for the request and physical event.

```json
{
  "operation_id": "695d30ea-c86b-4273-9aca-526dfbc81088",
  "source_id": "577479b3-32d9-4aff-8f92-46abfa7cf027",
  "item_type": "filament",
  "item_id": "0bb55686-46e5-4759-aebf-4fd9c138d3fb",
  "kind": "opening",
  "quantity": 250500,
  "reason": "Opening workshop count",
  "actor": "Workshop operator"
}
```

- Use `filament` or `model`. Each item accepts one nonnegative opening quantity. Opening quantity represents stock at the start of recorded history. If movements already exist, account for those deltas when reconstructing the opening count. Later corrections use `kind: "adjustment"` and a nonzero signed quantity. Quantities and totals must fit the JavaScript safe integer range.
- A successful request returns HTTP 201 with the movement, server timestamp, sequence and balance at that sequence. Retry with the original body to recover the same receipt. Reuse of either ID with a changed body returns 409. Invalid fields return 400 and unknown items return 404. Deleting a catalog item with ledger history returns 409.
- Read `GET /v2/inventory/balances/{item_type}/{item_id}` for on-hand, reserved and available amounts, verification status, negative-stock warning and revision. Read `GET /v2/inventory/movements/{item_type}/{item_id}?after=0` for up to 100 entries oldest first. Pass the last sequence as the next cursor.
- Both runtimes use the same SQLite transaction boundary. Desktop uses the existing loopback API and Self-Hosted uses its authenticated LAN proxy. The actor is an operator-supplied audit label. Access control comes from the runtime.

## Integration and recovery

Physical movements own on-hand stock. Reservations use a separate availability contract. Reserved is currently zero. A future hold reduces availability, release restores it, and dispatch consumes the hold and posts a physical deduction in one transaction. Consumers must check verification and revision before reserving. See the [Business architecture contract](architecture/BUSINESS_V1_1.md).

New purchases and completed/failed v2 production runs supply material events. Finished goods follow in #16. The ledger writer participates in the source transaction. Each source line and revision needs a stable event identity across retries. Historical purchases, recorded usage and financial snapshots are preserved during upgrade. Stock is never seeded from counters or cost factors.

Schema 8 adds indexed source references to the schema 7 ledger and advances the core inventory marker to version 2. A full backup preserves posting order, identity and audit metadata. Restoring a pre-ledger archive begins with an empty ledger. Before downgrading, use the pre-upgrade backup with its matching build. See [SQLite recovery](SQLITE_RECOVERY.md).

## Automatic material postings (INV-05)

Purchase `amount` stays in kilograms in the existing API. New individual and batch purchases post `round(amount × 1,000,000)` milligrams. Values rounding to zero or exceeding the safe integer range are rejected. Price/date-only edits and identical PUT retries do not post again. Quantity edits append the difference; changing filament compensates the old identity and credits the new one. Deletion compensates the posted contribution and retains audit history. Source records and all movements commit or roll back together.

A completed or failed v2 run posts `-round(weight_used × 1,000)` for each **verified** material row. `weight_used` includes physical waste, which is not added again. The financial waste factor and model weight do not affect stock. In-progress, planned, cancelled, estimated and legacy-unverified records do not deduct stock. Terminal runs are immutable; both operation retries and later commands retaining those runs avoid duplicate deductions. Unrecorded actuals remain estimated/unverified; no zero actual is invented. A physical opening count verifies the ledger baseline, not missing production measurements.

Movement `source_key` is `purchase:<purchase UUID>` or `material:<PrintJobFilament UUID>`. Automatic kinds are `purchase`, `consumption` and `compensation`; the adjustment endpoint cannot forge these. Source/event identities and source keys survive backup/restore, including after a purchase is deleted. Existing manual movements retain empty source keys.

`GET /filaments?page=1&low_stock=true` selects only filaments with an opening count and an on-hand balance at or below zero. Unknown stock is excluded rather than inferred from purchase/use counters. Configurable thresholds and unknown-stock UI follow in #15.

Upgrade is additive and idempotent; it does not replay historical purchases or terminal runs. Editing/deleting a historical purchase without ledger history changes its financial record only. Establish opening stock by physical reconciliation; historical migration policy belongs to #14. ZIP schema 8 preserves source keys. Schemas 1–7 remain readable; older builds reject the new core marker/ZIP schema. Before downgrade restore the pre-upgrade backup with its matching build; removing source metadata would permit duplicate posting.
