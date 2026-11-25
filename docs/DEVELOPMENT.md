# Development guide

This guide is the practical entry point for changing Maker Track Studio. The
project has one Go API and SQLite data model shared by a Wails desktop app and
an optional LAN-only browser deployment.

## Project map

| Area | Location | Owns |
| --- | --- | --- |
| API and data | `apps/api` | Go handlers, DTOs, models, SQLite runtime, migrations, costs, backups, and tests |
| Frontend | `apps/web` | React pages/components, API clients, translations, visual scenarios, and frontend tests |
| Desktop app | `apps/desktop` | Wails startup, embedded runtime, native packaging, and desktop smoke tests |
| Self-Hosted | `deployments/self-hosted` | Compose distribution and deployment support files |
| Workflows | `.github/workflows` | CI, security checks, packaging, and release automation |
| Documentation | `docs` | Architecture contracts, compatibility rules, security boundaries, and guides |

Read [Module architecture](MODULE_ARCHITECTURE.md) before changing module
composition, [SQLite recovery](SQLITE_RECOVERY.md) before changing persisted
data, and [LAN security](LAN_SECURITY.md) before changing standalone API or
proxy behavior.

## Set up and run locally

Install the Go version declared by `apps/api/go.mod`, Node 24, and a C compiler
for SQLite. Linux Desktop development also requires GTK 3 and WebKitGTK 4.1
development packages. See the [Desktop README](../apps/desktop/README.md) for
platform details.

Install frontend dependencies and run its checks from `apps/web`:

```sh
npm ci
npm run lint
npm test
npm run build
```

Run backend checks from `apps/api`:

```sh
go test ./...
go vet ./...
```

To run the complete Desktop app on Linux, start Wails from `apps/desktop`:

```sh
go run github.com/wailsapp/wails/v2/cmd/wails@v2.12.0 dev -tags webkit2_41
```

On Windows, omit `-tags webkit2_41`. The standalone browser frontend expects
the local API at `127.0.0.1:8080`; follow the [API README](../apps/api/README.md)
and [Self-Hosted guide](SELF_HOSTED.md) before running a standalone deployment.

## Common change paths

### Start with tests

Always implement or update the relevant tests before changing application
behavior. For a bug, first reproduce it with a failing regression test and
verify that the failure demonstrates the reported behavior. Then make the
smallest correction and rerun the focused tests before the broader checks.
Do not skip a failing test or weaken its assertions to hide a defect.

The regression suites now cover first-purchase initialization, paginated API
responses, overlapping requests, backup restore, and Print Job labor hours,
quantities, cost snapshots, and input validation. Use the existing suites when
adding cases, and keep HTTP fixtures consistent with actual API contracts.

Empty filament and purchase collections use JSON arrays in both legacy and
paginated responses. Print Job quantities and individual purchase price/amount
must be positive; reject a non-finite derived purchase unit price before writing.
Labor hours accept finite non-negative fractions, including 0.25 in creation and
editing. Request hooks apply results only for the active request, and successful
backup restore clears the file input so the same file can be selected again.
Filtered Print Job lists distinguish no matching results from a new installation.
Keep these guarantees covered by regression tests and deterministic UI fixtures.

From `apps/web`, check native browser validation of Print Job labor inputs with:

```sh
npm run screenshots:install
npm run test:browser:cost-inputs
```

This Chromium check submits the real form against synthetic HTTP fixtures;
Go tests separately verify cost calculations and SQLite persistence. It does
not replace a complete workflow against the real API or packaged Desktop app.
Keep local investigation plans, test-run reports, and draft issues out of
commits; record durable workflow guidance in this development guide instead.

### Add or change an API behavior

Read adjacent handlers, DTOs, routes, services, and tests first. Keep request
validation and response shapes aligned across the Go API and frontend service.
For stored changes, plan additive SQLite migration behavior and explain backup,
restore, and rollback compatibility. See [API README](../apps/api/README.md),
[data migration](DATA_MIGRATION.md), and [SQLite recovery](SQLITE_RECOVERY.md).

### Add or change a frontend workflow

Follow existing React Hook Form and Yup patterns for forms, and show readable
user-facing errors. For a visible change, update its deterministic Playwright
scenario under `apps/web/visual` and regenerate the relevant synthetic image in
`docs/screenshots`. See the [web README](../apps/web/README.md) and
[screenshot catalogue](screenshots/README.md).

#### Searchable catalogue fields

Use `SearchableSelect` in `apps/web/src/components/ui` for a single choice from
a catalogue. Pass a controlled key and an `onChange` callback. Options default
to `{ id, name }`. Supply `getKey`, `getLabel`, `getSearchText` or `renderOption`
for other shapes. FormSelect connects the same input to React Hook Form and the
form's Yup schema. Search text stays in the input until the user commits a
choice. Brand entry also accepts new text.

| Workflow | What users can search |
| --- | --- |
| Print Jobs and production runs | Printers, models and filaments in forms and list filters, including imported plate mappings |
| Filaments and Purchases | Existing filaments, existing or new brands, and the configurable filament type catalogue |
| Printers | Bundled presets by manufacturer or model before preview and apply |
| Quick Sale | Models, active customers and eligible lots on the current stock page |
| Budgets and proposals | Customers, models, printers and budget materials, including retained historical entries |
| Orders | Channel and model filters and fields, compatible run items, fulfilment actions and original finance entries |

Model and filament wrappers provide sorting and domain labels. Fixed choices
such as status, language, page size, sort order and payment mode use native
selects. Spreadsheet mapping uses native controls for the current file's
worksheets and headers. List search and brand/type text filters accept partial
queries. Allocation tables edit quantities for several jobs at once.

Local catalogues filter the supplied options. For a server-backed catalogue,
pass a stable `loadOptions({ query, cursor, signal })` that returns
`{ items, nextCursor }`. The selector debounces the first request, offers
additional pages and retry, and discards abandoned responses. Supply the
current option in `options` when opening an existing record whose selection may
be outside the first page. Quick Sale searches the existing models endpoint
in pages of 25 and loads active customer choices from the Business catalogue.
For lots, Quick Sale keeps server-side eligibility and its
explicit stock paging controls. Changing the model, quantity or page requires
a fresh lot selection.

The shared component tests cover keyboard commit, cancellation, clearing,
active-option announcements, blur, disabled controls and remote request races.
The visual catalogue exercises both themes, Portuguese on a narrow viewport
and stock-lot invalidation. The LAN browser test submits searched IDs to the
same API used by Desktop.

### Change costs or a module

Keep cost calculations shared between job reporting and Sales & Orders. Read
[advanced cost settings](ADVANCED_COST_SETTINGS.md), [printer profiles](PRINTER_PROFILES.md),
[Sales & Orders](SALES_AND_ORDERS.md), and [module architecture](MODULE_ARCHITECTURE.md)
as applicable. Preserve existing job relationships, SQLite data, and backup
compatibility.

### Change Desktop or Self-Hosted runtime

Desktop changes can affect Wails startup, loopback API lifecycle, SQLite paths,
packaging, and smoke tests. Self-Hosted changes must preserve loopback binding
behind the authenticated HTTPS proxy and trusted-LAN boundary. Use the
[Desktop README](../apps/desktop/README.md), [Self-Hosted guide](SELF_HOSTED.md),
and [LAN security](LAN_SECURITY.md).

## Before sharing a change

- Run the smallest relevant checks; include API tests/vet or frontend lint,
  tests, and build when those areas change.
- For UI changes, update the deterministic visual scenario and tracked image.
- For persistence, backup, or import changes, document compatibility and
  recovery behavior and test the relevant paths.
- Keep fixtures synthetic. Do not commit local databases, backups, secrets,
  personal paths, customer data, or generated binaries.
- Keep API loopback/CORS boundaries, SQLite ownership, and Desktop feature
  availability intact unless a product requirement explicitly changes them.
- Describe user-visible behavior, motivation, and validation in the pull
  request. Follow [contribution guidance](../CONTRIBUTING.md).

For workflow behavior and release evidence, see [CI policy](CI.md) and
[release qualification](PUBLIC_RELEASE.md). The [developer wiki index](README.md)
links to the remaining architecture and feature references.
