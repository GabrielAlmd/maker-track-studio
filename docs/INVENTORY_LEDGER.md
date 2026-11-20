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

Purchases and production will supply physical events in #13 and #16. The ledger writer participates in the source transaction. Each source line and revision needs a stable event identity across retries. Historical purchases, recorded usage and financial snapshots are preserved during upgrade. Stock is never seeded from counters or cost factors.

Schema 7 adds the ledger and a core version marker. A full backup preserves posting order, identity and audit metadata. Restoring an older archive begins with an empty ledger. Before downgrading, use the pre-upgrade backup with its matching build. See [SQLite recovery](SQLITE_RECOVERY.md).
