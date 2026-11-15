# Developer wiki

Welcome to the Maker Track Studio developer wiki. Use this page to find the
project map, local setup, implementation guides, and operational references.
The repository's [contributor guide](../CONTRIBUTING.md) covers pull request
expectations; the [development guide](DEVELOPMENT.md) is the recommended first
read for setting up and changing the code.

## Start here

1. Read the [project overview](../README.md) and [project shape](../AGENTS.md).
2. Follow [local development setup](DEVELOPMENT.md) and the [contribution
   guide](../CONTRIBUTING.md).
3. Choose a task path below and read the relevant contract before changing
   behavior.

| I want to… | Start with |
| --- | --- |
| Understand the codebase and run it locally | [Development guide](DEVELOPMENT.md) |
| Change or add an API resource | [API README](../apps/api/README.md), then [data migration](DATA_MIGRATION.md) or [SQLite recovery](SQLITE_RECOVERY.md) for persisted data |
| Change a screen, form, or frontend service | [Web README](../apps/web/README.md) and [visual catalogue](screenshots/README.md) |
| Add or change a built-in module | [Module architecture](MODULE_ARCHITECTURE.md) |
| Change cost calculations or printer data | [Advanced cost settings](ADVANCED_COST_SETTINGS.md) and [printer profiles](PRINTER_PROFILES.md) |
| Change backup, import, or persisted data | [Data migration](DATA_MIGRATION.md) and [SQLite recovery](SQLITE_RECOVERY.md) |
| Work on Sales & Orders | [Sales & Orders contract](SALES_AND_ORDERS.md) and [module architecture](MODULE_ARCHITECTURE.md) |
| Work on Desktop packaging | [Desktop README](../apps/desktop/README.md) |
| Work on Self-Hosted or its security boundary | [Self-Hosted guide](SELF_HOSTED.md) and [LAN security](LAN_SECURITY.md) |
| Understand CI or release checks | [CI policy](CI.md), [dependency security](DEPENDENCY_SECURITY.md), and [release qualification](PUBLIC_RELEASE.md) |

## Architecture and implementation contracts

- [Module architecture](MODULE_ARCHITECTURE.md) — composition, ownership,
  lifecycle, cross-module operations, and adding a built-in module.
- [Settings architecture](SETTINGS_ARCHITECTURE.md) — settings sections,
  lazy loading, persistence, and compatibility.
- [Sales & Orders](SALES_AND_ORDERS.md) — entities, APIs, lifecycle,
  profitability, migrations, and backup rules.
- [Printer profiles](PRINTER_PROFILES.md) — printer identity, costs, snapshots,
  presets, and compatibility.
- [Advanced cost settings](ADVANCED_COST_SETTINGS.md) — cost formulas,
  persistence, and rollback behavior.
- [Filament types](FILAMENT_TYPES.md) — catalog behavior and API/storage
  contract.
- [Reports and exports](REPORT_EXPORT.md) — report and archive formats.
- [Data migration](DATA_MIGRATION.md) — CSV/XLSX import flow and validation.
- [Diagnostics](DIAGNOSTICS.md) — local event, report, privacy, and recovery
  contracts.

## Runtime, security, and operations

- [SQLite recovery](SQLITE_RECOVERY.md) — database ownership, backups, restore,
  upgrades, and recovery.
- [Self-Hosted](SELF_HOSTED.md) — Compose installation, storage, lifecycle, and
  release validation.
- [LAN security](LAN_SECURITY.md) — proxy authentication, TLS, trusted peers,
  and deployment boundaries.
- [CI execution policy](CI.md) — workflow order, artifacts, runners, and failure
  handling.
- [Dependency security](DEPENDENCY_SECURITY.md) — dependency and image scan
  policy and evidence.
- [Release qualification](PUBLIC_RELEASE.md) and [roadmap](ROADMAP.md) —
  `v1.0.1` publication gates and remaining project work.

## Product and contributor references

- [First print](FIRST_PRINT.md) — a bilingual worked example with cost units and history rules.
- [Backups and support](SUPPORT.md) — restore, update and problem-reporting steps.

- [Screenshots](screenshots/README.md) — deterministic visual fixtures and how
  to regenerate them after UI changes.
- [Desktop installation guide](../apps/desktop/README-DISTRIBUTION.md) —
  end-user installation, backup, and recovery instructions.
- [Contributing](../CONTRIBUTING.md), [security policy](../SECURITY.md), and
  [code of conduct](../CODE_OF_CONDUCT.md) — community collaboration guidance.
- [Agent instructions](../AGENTS.md) — repository-specific guardrails and
  implementation expectations for coding agents.

Package READMEs provide local context: [API](../apps/api/README.md),
[web](../apps/web/README.md), [Desktop](../apps/desktop/README.md), and
[Self-Hosted deployment](../deployments/self-hosted/README.md).
