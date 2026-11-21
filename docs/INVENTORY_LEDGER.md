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

Physical movements own on-hand stock. Reservations use a separate availability contract. A hold reduces availability, release restores it, and dispatch consumes the hold and posts a physical deduction in one transaction. Consumers must check verification and revision before reserving. See the [Business architecture contract](architecture/BUSINESS_V1_1.md).

New purchases and completed/failed v2 production runs supply material events. Verified completed runs also supply finished model stock. The ledger writer participates in the source transaction. Each source line and revision needs a stable event identity across retries. Historical purchases, recorded usage and financial snapshots are preserved during upgrade. Stock is never seeded from counters or cost factors.

Schema 8 adds indexed source references to the schema 7 ledger and advances the core inventory marker to version 2. A full backup preserves posting order, identity and audit metadata. Restoring a pre-ledger archive begins with an empty ledger. Before downgrading, use the pre-upgrade backup with its matching build. See [SQLite recovery](SQLITE_RECOVERY.md).

## Automatic material postings (INV-05)

Purchase `amount` stays in kilograms in the existing API. New individual and batch purchases post `round(amount × 1,000,000)` milligrams. Values rounding to zero or exceeding the safe integer range are rejected. Price/date-only edits and identical PUT retries do not post again. Quantity edits append the difference; changing filament compensates the old identity and credits the new one. Deletion compensates the posted contribution and retains audit history. Source records and all movements commit or roll back together.

A completed or failed v2 run posts `-round(weight_used × 1,000)` for each **verified** material row. `weight_used` includes physical waste, which is not added again. The financial waste factor and model weight do not affect stock. In-progress, planned, cancelled, estimated and legacy-unverified records do not deduct stock. Terminal runs are immutable; both operation retries and later commands retaining those runs avoid duplicate deductions. Unrecorded actuals remain estimated/unverified; no zero actual is invented. A physical opening count verifies the ledger baseline, not missing production measurements.

Movement `source_key` is `purchase:<purchase UUID>` or `material:<PrintJobFilament UUID>`. Automatic kinds are `purchase`, `consumption` and `compensation`; the adjustment endpoint cannot forge these. Source/event identities and source keys survive backup/restore, including after a purchase is deleted. Existing manual movements retain empty source keys.

`GET /filaments?page=1&low_stock=true` selects only filaments with an opening balance or physical count and an on-hand balance at or below zero. Unknown stock is excluded rather than inferred from purchase/use counters. Configurable thresholds and unknown-stock UI follow in #15.

## Physical stock reconciliation

After an upgrade or historical import, count each material in milligrams or each finished model in whole pieces. Read the item's balance and keep its revision while counting. Submit the observed total to `POST /v2/inventory/adjustments` with fresh operation and source UUIDs. For example, an operator who weighs 250.5 g and reads revision 17 would submit this request.

```json
{
  "operation_id": "73d7d2bc-032d-4a92-95cb-5dbb9a963ea8",
  "source_id": "81cdf374-fa37-4e86-bc74-a7cb4a86622c",
  "item_type": "filament",
  "item_id": "0bb55686-46e5-4759-aebf-4fd9c138d3fb",
  "kind": "count",
  "counted_quantity": 250500,
  "expected_revision": 17,
  "reason": "Weighed remaining PLA",
  "actor": "Workshop operator"
}
```

Use revision 0 for an item with no movements. Supply both count fields explicitly, including for an empty shelf. Omit `quantity`; the server records the signed difference. If the prior on-hand value was 300000 mg, the movement is −49500 mg and verified on-hand becomes 250500 mg. A matching count still records verification. A later recount uses the latest revision. Both the observed total and the signed difference must fit the ledger's safe integer range.

On a lost response, repeat the identical request to retrieve its original receipt, including after restoring a backup. A stale revision returns 409. Refresh the balance and confirm the count against any intervening movements before submitting a new event. Historical purchases, job material rows, and reported production units retain their provenance. Subsequent startup and archive restore preserve posted movements and their IDs without replaying these records.

## Inventory screen and thresholds (INV-07)

Inventory is always available in the core navigation, including with Business disabled.
Search by brand, type or color. Balances and warnings come from physical ledger
movements, never purchase or usage counters. Unknown historical stock shows
recorded changes separately until a physical count verifies the balance.
Choose **Count and audit** to enter total counted grams, a reason and an operator.
Counts use the displayed ledger revision; concurrent movements require reloading
and recounting. Identical retries retain operation identifiers. Movement history
retains the original entries and loads in sequence in batches of 100.

Set a per-material warning threshold in grams (zero by default). Verified stock
at or below the threshold warns on Inventory and Dashboard. Print planning also
compares selected material estimates with available stock, without blocking or
posting deductions. Finished-goods reservations use the Inventory contract below.

`GET /v2/inventory/materials` returns material identity, `threshold_mg` and the
existing balance projection. `PUT /v2/inventory/thresholds/:id` accepts
`{"threshold_mg": 100000}`: a required nonnegative safe integer in milligrams.
It returns 204, 400 for invalid input, or 404 for an unknown identity.

The additive `filaments.stock_threshold_mg` column defaults to zero. Backup schema
10 includes thresholds and rejects schema-10 archives missing that column;
older accepted archives restore with zero thresholds. IDs, financial history and
ledger data remain unchanged. Older app versions reject schema-10 backups;
retain a pre-upgrade backup for rollback and restore it with the older binary.

## Finished goods

Select the catalog model for each output on a planned run. When you complete
the run, each accepted quantity posts once to that model in whole pieces. A
mixed plate credits each item separately. Rejected pieces, failed runs and
commercial orders contribute zero. Uncatalogued output remains in production
history until an explicit identity can be established. The model name alone
cannot establish that identity.

Inventory lists models with recorded movements under **Finished goods**. Use
**Count** to verify a shelf total. A count below an active hold is rejected.
Before a count, the displayed change covers recorded movements only.

`GET /v2/inventory/finished-goods` returns model IDs, names and balances.
Model movement history uses `source_key=output:<run-item UUID>` to link a
credit and all subsequent fulfilment events to its original run item.
The run item carries the parent run UUID. A completed run remains immutable.

Business integrations call `inventory.OutputAvailability` for a verified
source and `inventory.ApplyFulfilment` in the same SQLite transaction as
their own writes. Each command supplies a stable operation UUID, unique
external line-event UUID, model, run item, expected lot revision, positive
units, reason and actor. The receipt sequence becomes the next lot revision.
The reserve operation UUID identifies the hold. Dispatch operation UUIDs
identify shipments at line level. Identical retries return the original receipt.

| Kind | Reference | Physical effect |
| --- | --- | --- |
| reserve | Opaque owner UUID | Hold available accepted units |
| release | Reserve operation UUID | Release remaining held units |
| dispatch | Reserve operation UUID | Remove units from both on-hand and held |
| restock | Dispatch operation UUID | Add inspected, accepted returns to on-hand |
| reject_return | Dispatch operation UUID | Record inspected rejects against the remaining returnable units |

Both the run-item capacity and the aggregate model balance limit a hold.
A discrepancy in the historical opening balance still requires a physical
count. A multi-line shipment must use one transaction and stable per-line
event IDs. The commercial integration belongs to #26.

Schema 11 adds units and reference fields to inventory movements and advances
the core inventory marker to 4. Older movements default to zero units and an
empty reference. Upgrades and imports preserve recorded stock, IDs and
financial snapshots. Runs completed before the upgrade may already be included
in shelf counts, so historical completions are never automatically replayed.
Use the pre-upgrade backup with its original application version for rollback.
