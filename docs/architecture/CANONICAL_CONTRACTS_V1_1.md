# v1.1.0 architecture, ownership and costing contracts

ARC-01 (#72) defines the contract for subsequent work in Epic #71. The baseline
below describes `develop` at `e5abca6`. The remaining sections specify target
behavior for new implementations. ARC-01 publishes this contract and executable
examples without enabling any new API, calculation version, persistence or UI.

Read with [Production](PRODUCTION_V1_1.md), [Business Operations](BUSINESS_V1_1.md),
[Printer profiles](../PRINTER_PROFILES.md) and [Inventory ledger](../INVENTORY_LEDGER.md).
Keep their lifecycle, allocation and stock invariants. Use the implementation
guides and the baseline below to identify shipped behavior.

## Ownership and dependencies

| Owner | Authoritative data and commands | Consumers and permitted references |
| --- | --- | --- |
| Core Catalogue and Settings | Model UUID/name and reusable per-piece requirements, filament identity, physical Printer UUID and profile, global defaults | Production and Business consume IDs and applicable defaults. Save editable assumptions in the consuming draft and copy values into a snapshot only when needed for provenance. |
| Core Purchases and filament pricing | Purchase UUID, quantity, amount paid and acquisition context. The filament record supplies the current catalogue average EUR/kg used by costing. | Estimates and run snapshots capture the price actually used and its basis. A stock count or hold supplies no purchase price. The catalogue average supplies no lot acquisition history. |
| Core Production | Planned layouts, one execution per run, measurements, actual assignment, run/item IDs and revisions, attempted/accepted/rejected counts, and per-run cost facts | Inventory consumes eligible physical events. Business consumes planned context and costed accepted output by stable source ID and revision. Reports consume cost state and provenance with totals. |
| Core Inventory | Movements, verification, on-hand, reservations, lot revisions and dispatch/return capacity | Business stores demand, allocation links and operation references. The Inventory service enforces physical capacity in the calling SQLite transaction using opaque owner/source IDs. |
| Optional Business Operations | Customer, budget and proposal revisions, confirmed terms, accepted price and discounts, demand and commercial allocation links, invoices, receipts, refunds and financial snapshots | Business consumes core costing and Inventory services. Production remains available without Business. Finance reads agreed terms and posted finance events independently of production and fulfilment. |
| Reports and exports | Read projections and presentation | Read cost and financial results from their owners. Carry basis, version and coverage when aggregating. Preserve the distinction between agreed revenue, cash received, estimated cost and actual profitability. |

The application composes optional modules through the existing registry. Core
does not import Business, and Inventory does not query Business tables. A core
operation that needs to validate a Business reference uses a registered guard
under the same writer lock. Business-specific pricing, discounts and finance
remain with Business. Cost calculation belongs to a shared core domain service
that takes validated inputs and returns values, state and version without
HTTP, database or frontend dependencies. Adapters load sources and apply
authorization, lifecycle, compatibility and transaction rules.

Use Go-generated UUIDs for durable identity. Keep entity IDs, child IDs,
aggregate revisions, operation IDs and calculator versions distinct. IDs and
revisions identify the exact source even if a display name changes. Historical
snapshots may include names and raw inputs for replay. Names, list positions
and dates cannot replace an ID. Update referenced children in place or use an
explicit versioned correction. Acquire SQLite's writer lock before checking
revision, availability and capacity, then commit domain changes, physical
effects and retry receipts together. Same operation and payload recover the
original result. Stale revisions or a changed payload under the same operation
ID conflict without side effects. Physical source uniqueness also prevents
duplicate posting under a fresh operation ID.

## Baseline and gaps

Code paths below are relative to `apps/api` unless under `apps/web/src`.

| Flow | Implementation and gap |
| --- | --- |
| Print Jobs and reports | `handlers/print_job_handler.go` centralizes `ProductionCostParts` and `calculatePrintJobCostState`. Its job total rounds after summing raw components. Assigned jobs carry a printer configuration snapshot. Legacy unassigned jobs use live global settings, material averages remain live, and absent time may use a model estimate. Reports reuse the job cost state and expose current-average material estimates separately. A calculated job total alone does not prove measured run cost. |
| Production and Inventory | `models/production.go` and `handlers/production_handler.go` store run time, output and material records. The v2 handler locks the job-wide printer assignment after creation. It stores job time/labor summaries and posts eligible output/consumption atomically. `inventory` owns movement and lot balances. The job snapshot does not capture a distinct actual run assignment and material-rate snapshot. |
| Budget and Quick Sale | `modules/sales/budget/estimate.go` calls the core components, rounds each to cents per unit and multiplies the unit estimate by quantity. `quick_sale_pricing.go` reuses this estimate for model-based pricing. An omitted printer uses global settings. Saved assumptions and sources preserve the estimate basis, but these paths do not yet support a weighted printer mix. |
| Proposal, Order and Finance | `modules/sales/budget` snapshots proposal revisions and confirmation terms. Quick Sale can save optional pricing provenance even when no proposal exists. Production links can reference verified run items, and fulfilment uses Inventory reservations and dispatch. Finance uses confirmed terms and separate finance entries. Legacy Sales profitability uses current job cost until completion, then reads frozen cents. The legacy profitability service returns unavailable for v2 orders, and Quick Sale stock selection supplies no actual lot cost. Preserve legacy proportional rounding and snapshots. |
| Forms and handoffs | `apps/web/src` already provides `SearchableSelect`, `ModelSelect`, `FilamentSelect`, `Modal`, React Hook Form and production material estimate helpers. Production can reuse model material requirements. Budget model selection does not load all saved assumptions. Quick Sale keys the editor by context and can lose inputs on remount. Versioned commands already provide revision checks, but recoverable drafts are not a cross-module feature. |

## Units, rounding and result states

Use explicit quantity basis per piece, plate execution, run, budget line,
order or report. A planned plate with repetitions scales planned time,
material and output once. An actual run is one attempt. Actual duration and
labor are totals for that attempt. Never derive them by multiplying accepted
quantity or estimated piece time. Convert a total to an average only when
its denominator and unit basis are known.

| Value | Canonical boundary and arithmetic |
| --- | --- |
| Counts | Integer pieces. Planned attempts and quantity multipliers are positive. Actual quantities may be zero. Verify terminal `accepted + rejected = attempted` and `attempted <= planned`. Only verified completed accepted output is allocatable. |
| Mass | Existing purchase API uses kg, material measurements use g, and Inventory stores integer mg. Convert at the ledger boundary once using the existing rounding rule per posting source (1 kg = 1,000,000 mg and 1 g = 1,000 mg, nearest with ties up for nonnegative mass). Preserve the measurement and its provenance. For cost, convert measured grams to kg without intermediate rounding. |
| Time and rates | Use integer printing minutes and decimal total labor hours consistent with existing inputs. Divide minutes by 60 for engine hours without intermediate rounding. Persist rate basis explicitly, including EUR/kg, EUR/kWh, EUR/hour and average watts. |
| Money | Carry unrounded component costs and ratios through the new calculator using reproducible decimal inputs and exact or equivalent checked arithmetic. For legacy floating inputs captured by a new adapter, snapshot the normalized decimal value (shortest round-trip decimal at the source field's precision) so replay uses the same inputs. Round a nonnegative run or estimate line total once to EUR cents, nearest with ties up. Sum rounded facts at higher levels. Display component cents by distributing any remainder from flooring components to the largest fractional components, ties in the fixed order material, energy, machine, labor, post-process, other. Components add to the displayed total. A display average is never multiplied back to reconstruct a total. |
| Percentages | Commercial percentage inputs use integer basis points (1% = 100 bps). Profit = discounted net revenue minus attributable cost. Margin divides profit by net revenue. Markup divides profit by cost. Use the exact ratio until display (nearest basis point, ties away from zero). Missing cost or a zero denominator gives unavailable for the dependent result. Discounts apply to lines first, then the order subtotal remaining. Round each applied discount once to cents, ties up, bounded by its base. Distribute order discount with largest remainder and a stable line identity tie-break as in the Business contract. |
| Bounds | Use checked operations and validate intermediates and aggregates, including products, revision increments and allocations. Integer amounts crossing browser JSON boundaries use at most `2^53 - 1` in magnitude and may have smaller domain limits. Reject non-finite, invalid, overflowing or unsupported inputs. Preserve signed profit, variance and ledger adjustments. |

The target result distinguishes estimated (specific machine or explicit
scenario), commercial reference, actual, historical with its original basis,
and unavailable with a reason. Null/absent means unknown where applicable.
Zero is a known value from validated input or calculation. An empty material
list establishes zero use only when explicitly known to be complete. Missing
rates, evidence or mappings cannot become zero via defaults. Preserve existing
legacy shapes and values through adapters. Mark report coverage when some facts
are unavailable. A subtotal over known facts cannot stand in for a complete
actual total. Request and display basis together with the number.

### Cost components and provenance

The canonical service consumes normalized inputs. Keep the historical Simple
and Advanced formulas and their semantics under their existing versions. Simple
uses configured per-hour wear and the historical electricity rate with its
implicit 1 kW basis. Advanced uses average watts / 1,000 × tariff for hourly
energy, and (investment + years × annual maintenance) ÷ (8,760 × uptime fraction
× years) for hourly wear. Apply the configured buffer once to both hourly rates.
Add material and labor once and add explicit post-processing and extras at the
declared level. Each machine uses its own captured investment/equipment inputs.
A copied or explicitly inherited global value records its origin. An already
captured machine total must include each applicable equipment expense once.
Treat copied templates as per-machine assumptions requiring review. Allocate a
shared equipment expense in explicit, non-overlapping shares across machines.
Copying a fleet-wide amount into every profile would charge the same expense
several times.

For a measured run use actual grams, including measured physical waste, once
at the recorded material rate. For a prospective estimate with planned gross
grams `g`, waste factor `f` and explicitly included waste grams `w`, charge
`((g − w) × f + w) / 1,000 × EUR/kg`. When no waste breakdown is available,
charge `g × f / 1,000 × EUR/kg` on the estimate basis. This retains the
estimate formula and charges itemized waste once. The financial factor never
updates physical stock. Capture filament/measurement identity, EUR/kg, pricing
basis and capture time. A catalogue average captured at execution is a valuation
basis for actual consumption, separately labeled from a proven lot price.
Keep source purchase IDs when a specific acquisition price is established.
Without the required measurement or valuation basis, cost is unavailable.
Preserve legacy waste-factor behavior, including in historical recomputations
that explicitly use that version. Snapshot raw printer/rate inputs, timing,
labor, material prices and source revisions with the calculator version. A
configuration snapshot alone does not freeze the cost of a run.

## Printer assignment and commercial reference

- A printer is one physical UUID even when two profiles have identical models.
  The global default is a suggestion for prospective work. An explicit
  selection takes precedence and historical assignment takes precedence for
  actual results. Change the plan safely before execution. Capture the actual
  active, compatible printer and its cost inputs when execution starts. Record
  the reason and lineage for a post-start correction, and preserve historical
  facts and commercial terms. Never substitute a default for a missing,
  archived or incompatible actual assignment. An archived printer remains
  readable by historical ID and snapshot.
- One job belongs to one physical execution machine. If a plan or order spans
  multiple machines, split the work into machine-specific jobs and preserve
  demand and planning links. A different actual machine for unexecuted work
  can be confirmed before execution. Move/split the remaining work if a job
  already has executed runs. Reassignments cannot rewrite previous executions.

A commercial reference is a scenario cost for one model and revision,
independent of which machine ultimately executes an order. For each eligible
physical machine, compute the model-attributable estimated cost and expected
accepted units on the same scenario basis. Use model-specific times, rates,
consumption, planned plate quantities and reviewed reject assumptions. Include
expected reject overhead in the applicable estimate. For a mixed plate,
apportion shared estimated cost by each item's captured positive integer
weight divided by total weight. An explicitly confirmed equal cost per planned
piece is a fallback. Preserve the whole plate cost across items before rounding
estimate lines.
A historical mix supplies weights, while the prospective costs use the captured
quotation inputs.

1. Prefer an explicit planned accepted-unit mix. Alternatively use an
   operator-confirmed representative period or run set of verified accepted
   output for comparable model, layout, material and production settings.
   Persist the basis and period or source references. Machines must be active,
   confirmed compatible with the scenario and sufficiently configured when
   producing a new reference. If a positive-weight member fails eligibility
   or comparability, require a revised mix or explicit fallback.
2. For weights `q[i] > 0` and unrounded estimated EUR per accepted unit `c[i]`,
   reference unit cost is `sum(q[i] × c[i]) / sum(q[i])`. Equivalently, sum each
   machine's estimated attributable cost for its mix quantity and divide by
   total expected accepted output. For 60 at EUR 1.50 and 40 at EUR 1.30, the
   reference is EUR 142 / 100 = EUR 1.42 per unit. Round the reference line
   amount after scaling the exact unit ratio by quoted quantity. This gives a
   commercial normalization. Physical plans still use whole plates and attempts
   and are estimated at their own counts. For a positive cost basis, suggest a
   selling unit price using
   `cost × (1 + markup)` or `cost ÷ (1 − margin)` with ratios converted
   from basis points. Target markup and margin are nonnegative and target
   margin must be below 100%. Round the resulting unit price once to cents.
   Keep explicit manual price cents. Version 1 requires positive cost for
   automated suggestions. Keep that guard pending the #74 version decision
   for known-zero inputs.
3. If a reliable mix is unavailable, require a user-selected eligible machine
   estimate or user-confirmed scenario with documented assumptions. Label it
   as such, including explicit use of a global scenario. Without that basis,
   reference cost is unavailable. A manually entered selling price remains
   possible where the workflow allows it.
4. Recalculate prospective estimates on explicit refresh or review after
   relevant changes. Keep the last captured result and mark it stale until
   recalculated. Freeze basis, weights, source values/revisions, calculator
   version, applicable quantities, cost and agreed price at proposal issue
   and order confirmation (including direct Quick Sale). Accepted terms use
   the exact issued revision. Prospective refresh, reassignment, changes to
   defaults and actual-cost updates never change the agreed selling price.
   An agreed amendment creates a separate commercial revision and preserves
   the original acceptance. Keep internal cost inputs and profitability out of
   the customer-facing proposal DTO and PDF through the existing allowlist.

For actual costs, round each run total once. Allocate a mixed run by explicit
positive integer cost weights captured before completion. Conserve the rounded
total with largest remainder and run-item UUID ties. An explicitly confirmed
equal cost per attempted piece is a fallback. Each item share includes rejects
for that item. For zero accepted output, retain the share as production expense
and leave unit cost unavailable. Allocate an item share over accepted unit ordinals as in
[Business Operations](BUSINESS_V1_1.md#prices-actual-costs-and-rounding).
Each ordinal has a fixed cent value and can be charged at most once to active
allocated or dispatched supply. Release and reallocation of an unfulfilled
hold preserve its cents. Cancellation cannot free a dispatched ordinal.
Only an inspected restock can make it eligible again. Keep the original frozen
financial record and record any financial reversal or adjustment separately
with its source. Cost adjustments
for zero-output expense reference the original run/item cost source and enforce
uniqueness across orders, including retries with a different operation ID.
An adjustment cannot duplicate an allocation. Aggregate actual unit cost as
attributable cents across the relevant accepted units of all machines divided
by those units. At order level, include only its allocated ordinal costs and
explicit order costs. Show
scope-matched reference, actual and variance, and the applicable estimated or
actual margin. Unallocated demand remains visible and prevents complete actual
profitability unless covered by another verified cost source.

## Reuse, overrides and draft lifecycle

Reuse selection, validation, formatting, modal and form primitives already in
the app. Extend a shared component when consumers share an interaction
contract. Keep business rules and incompatible behavior in their owning
workflow or service. Share application services when they have the same
authority, units, validation and error/retry semantics. A UI-only estimate may
guide editing, while the API supplies authoritative values. Preserve loading,
empty, error and retry behavior, focus and keyboard interaction, and EN/PT-PT
number and currency formatting.

Pass context by stable IDs and source revisions from catalogue to budget and
from budget/proposal or Quick Sale terms to order and production. Load
applicable defaults once per basis. Record source and unit basis at item/field
level (model/default, slicer, handoff, manual, actual). Recalculate derived
values when dependencies change, including per-piece quantities. Preserve
compatible pinned values, mappings and explicit zeros. Mark a quote stale
without dropping inputs. When compatibility changes, show the conflict and
require explicit remap, reset or retain decisions before saving. Actuals stay
separate from estimates. Keep normalized slicer provenance and applicable
plate/slot mappings through handoffs. A plate total cannot be inferred for an
individual model without attribution. Standalone Production and Quick Sale
retain their shorter paths.

Drafts are recoverable input, with no inventory or financial side effects.
Warn before losing dirty data through close, Escape, backdrop or navigation.
Scope recovery to the current installation/data store, workflow, entity or
new-draft UUID, source/base revision and draft format version. Offer explicit
restore/discard. On a revision conflict, retain input and offer reload with
reviewed merge or discard. An uncertain commit retries the same operation
identity and payload. Clear recovery data after a confirmed commit or explicit
discard. Expiry and schema incompatibility must be communicated and must never
submit or silently replace an active draft. Exclude temporary drafts from
portable backups and prevent a workspace restore/reset from reattaching drafts
to the replaced data. Keep private fields out of URLs, diagnostics and casual
browser storage. Restrict access to recovery data to the existing local or
authenticated LAN boundary.

## Compatibility, rollout and validation

ARC-01 leaves ZIP format 1, backup schema 23, module versions, database files,
routes and wire contracts as they are. Revert the documentation and examples
to roll back this change. Future persisted changes follow these gates.

1. Use additive, versioned SQLite migration and explicitly nullable new inputs.
   Preserve IDs, references, revisions, frozen prices and costs, and legacy
   readers. Label old evidence with its actual basis. Do not infer actual
   printers, paid amounts, measurements, material price history or verified
   output from live defaults, current rates or legacy counts. Never replay
   legacy purchases or jobs to seed stock.
2. Before enabling new writes, version the calculation and affected core/module
   and archive contracts. Add explicit backup validation and delete/restore
   coverage in dependency order. Check newer versions and invalid references
   before replacing the target. Restore atomically. Coordinate core and
   module upgrades across their existing migration boundaries before running
   cross-module backfills. Verify repeated startup/import and failure rollback.
3. Use pre-upgrade backups with their compatible binaries for data rollback.
   Stop the application, retain a copy of the upgraded data and restore the
   pre-upgrade archive. Writes after that archive are absent from the rolled
   back copy. Desktop and Self-Hosted share the same SQLite path through the
   core runtime and recovery services for their respective installations.
   Keep the desktop loopback and authenticated HTTPS LAN boundaries.

`apps/api/architecture/canonical_contract_test.go` checks worked arithmetic
examples. Existing architecture tests cover plate/run counts and conserved
mixed-run/ordinal allocations. The examples establish expected results for
later runtime tests. #74 owns calculator parity and version mapping for
versioned and legacy consumers. #75 owns exact compatibility evidence and
operator selection UI, capture timing for material valuation within execution,
per-machine equipment UI and snapshot schema. Automatic selection of a
representative historical period requires a defined sufficiency policy.
#78/#79 own source handoffs and override compatibility rules per workflow.
#80 must decide the protected local draft store and retention period before
persisting drafts. #82 verifies parity, retries, dispatch/cancellation/return
cost conservation, restart, real SQLite migration and backup behavior. #83
qualifies packaged Desktop and Self-Hosted.

These implementation choices must be resolved in their owning issues before
enabling the relevant behavior. They need no speculative schema change here.
