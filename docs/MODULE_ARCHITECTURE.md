# Desktop module architecture

Issue #55 makes Sales & Orders the first built-in module of the Wails desktop app. Modules run in the same process, use the same loopback API, and share the existing SQLite database. Module registration is a build-time composition choice; the **Settings > Features** preference controls presentation only.

## Composition and lifecycle

- `apps/api/modules.Module` defines ID, module version, required core contract version, default feature state, owned backup tables, migration, HTTP routes, backup validation/deletion/restore, and a Print Job deletion guard.
- `apps/api/appmodules.Default` lists installed backend modules. Desktop startup migrates core tables, then migrates all installed modules in one SQLite transaction before registering routes. A failure names the module, rolls back the entire module transaction, and prevents HTTP startup.
- `module_versions` records installed module versions. An older build rejects a database or backup that requires a newer module version. An existing database without the ledger is migrated in place; no Sales IDs or rows are copied.
- `apps/web/src/modules/index.jsx` lists installed frontend descriptors. A descriptor supplies routes and navigation and may supply a feature setting, dashboard content and Print Job card content. Navigation, dashboard and Print Job card contributions render only when the persisted feature key is true. Routes remain registered when a feature is hidden, preserving existing deep links and API behavior.
- `apps/web/src/modules/translations.js` merges module translations into the core locales before i18n initialization.

The `sales_orders` module is version 1, requires core contract version 1, and defaults to disabled. Disabling it hides Sales navigation and Business dashboard content. It leaves Sales routes, data, completed profitability snapshots and backup contents intact. Removing the backend module from composition is a different operation: production-only startup has no Sales tables or routes and must use a database without retained Sales tables.

## Sales boundaries

| Boundary | Contract and owner |
| --- | --- |
| Production data | Core owns Models, Print Jobs, printers, filaments, purchases and cost inputs. Sales reads Model IDs/names and Print Job IDs, Model links, printed units and cost inputs through the existing core GORM models. Sales item `model_id` and allocation `print_job_id` remain nullable SQLite references; item names are snapshotted. |
| Production costs | Core exposes `LoadProductionCostConfig` and `CalculateProductionPrintJobCostState`. Sales uses those functions for live profitability and completion, so it uses the same cost rules as the Print Job API. Core cost logic does not import Sales. |
| Deletion | Core takes SQLite's writer lock, then asks registered modules to check Print Job deletion inside that transaction. Sales rejects deletion when an allocation references the job. Allocation writes use the same lock. |
| Sales persistence | `apps/api/modules/sales/storage` owns channels, orders, items and allocations. The module migrates their existing table names and constraints, so old rows, UUIDs, lifecycle states and frozen snapshots remain stable. |
| Sales API | `apps/api/modules/sales/api` owns Sales DTOs; `apps/api/modules/sales/handlers` owns Sales validation, routes, allocation, completion, summary and profitability rules. Existing HTTP paths and JSON fields are unchanged. |
| Backup | The core snapshot remains a whole SQLite database. The module validates Sales source tables and relationships before replacement, deletes Sales rows before core rows, and restores Sales rows after core Model and Print Job rows, all inside the existing import transaction. |
| Frontend | `apps/web/src/modules/sales` owns pages, components, service, pricing helper, Business dashboard, feature setting and translations. Core App, Layout, Dashboard, Settings and i18n consume module contributions. |

Sales owns no production-cost formula. A completed order's allocation costs and profitability remain frozen. A cancelled order retains allocations but releases active capacity. Model deletion preserves the item name snapshot; Print Job deletion is rejected while referenced. These rules remain unchanged by module enablement.

## Existing databases and backups

- Startup applies additive, idempotent Sales `AutoMigrate` to the same four tables. The migration and module-version record commit together. A failure rolls back both. Existing Sales rows and completed snapshots are covered by migration regression tests.
- The backup format and schema version remain at 1 and 4. Backups from supported schema versions 1–3 restore with empty Sales collections. Version 4 backups without snapshot columns remain supported. New snapshots include `module_versions`; older snapshots without that table remain supported.
- Import validates module data before changing the target and restores core and module rows in one transaction. It rejects a source or target with Sales legacy tables when Sales is not installed, and rejects a source ledger requiring a newer or unavailable module. This prevents an import from silently discarding retained module data. An older build should be used only with its compatible database or a backup made before the module upgrade.
- Feature preferences for modules absent from the current build are retained when installed feature settings are updated.
- Export/restore should still be preceded by a user backup. Rollback is to restore that backup with a compatible build; no destructive downgrade migration is attempted.

## Adding a built-in module

1. Put module storage, DTOs, handlers, frontend components and translations in module-owned packages. Use core models only for production data the module needs; expose shared business calculations as narrow core functions rather than importing module logic into core.
2. Implement `modules.Module`, including an idempotent migration, version and required core version, owned backup tables, backup validation/restore and any production deletion guard. Register it in `appmodules.Default`. Keep all changes in the migration transaction and preserve old table names for existing data.
3. Add a frontend descriptor to `apps/web/src/modules/index.jsx` and its translations to `apps/web/src/modules/translations.js`. Use the module's feature ID as its preference key. Core page files should render registry contributions without module-specific branches.
4. If a new module owns persisted rows, include them in the existing backup transaction and add restore tests for old and current archives. New backups carry `module_versions`; an import with an unavailable module version is rejected. If the backup schema version changes, document and test the transition before release.
5. Add tests for enabled/disabled UI, production-only startup, migration rollback, old database upgrade, backup compatibility and every cross-module transaction. Update deterministic screenshots when a visible screen or interaction changes.

### Module author checklist

- **Composition:** Give the backend module a stable, unique ID and a positive version. Add it to `apps/api/appmodules/default.go`. Add the matching frontend descriptor to `apps/web/src/modules/index.jsx` and its translations to `apps/web/src/modules/translations.js`. The ID is also the persisted feature preference key. Keep the backend and frontend installed module lists in sync for a desktop build.
- **Versioning and migration:** Declare the required core contract version. Bump the module version for a persisted schema or data migration. Make migrations additive and idempotent, run only through the transaction passed to `Migrate`, and preserve existing table names and IDs. If an older build cannot read the upgraded data, let the version check reject it; do not silently downgrade or delete rows.
- **Table ownership:** List every module-owned table in `BackupTables`, including child tables. No two modules may claim one table. If the module takes ownership of tables from a build that predates `module_versions`, add those names to `legacyModuleTables` in `apps/api/modules/registry.go` so a build without that module rejects databases and backups containing its retained data.
- **Routes and data flow:** Register module HTTP routes in `RegisterRoutes` and use `modules.WithRequestDB` for request context. Put handlers, DTOs and validation in the module. Keep core handlers and pages dependent on generic registry hooks. Use a narrow core function for shared calculations; keep the embedded API on loopback and its CORS allowlist restricted to Wails and development origins.
- **Feature state:** `EnabledByDefault` controls the initial preference. Disabling a feature hides its navigation and screen contributions; it does not drop tables, block its existing API, or change stored values. Keep routes registered so deep links continue to work. A production-only build removes the module from both composition lists and must reject a database or backup with retained module data.
- **Backup and cross-domain operations:** Implement `ValidateBackup`, `DeleteBackup`, and `RestoreBackup` for owned data, including references to core rows. Validation happens before target replacement; deletion and restoration run in the existing import transaction, with module rows restored after core rows. Add a registry guard for any core deletion that module data must prevent; check it under the same writer lock as the write.
- **Frontend contributions:** Put pages, components, API service, translations and module-only helpers under `apps/web/src/modules/<module>`. Define route, navigation, feature setting, dashboard or Print Job card contributions as needed. Core screens render generic contributions, with loading, empty and error states handled by each module component. Validate numeric form input and show readable errors.
- **Verification:** Cover first install, existing SQLite data, migration failure rollback, older backups, unavailable/newer module versions, enabled and disabled presentation, and a build composed without the module. Run focused Go and frontend checks. Update the deterministic Playwright scenario and tracked screenshot for each affected visible state. Check desktop packaging separately when packaging behavior changes.

This is an internal module contract, not a third-party plugin loader. There is still one desktop application and one SQLite database.
