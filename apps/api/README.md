# Local backend

Go/Gin/GORM backend embedded by `apps/desktop` and runnable as a standalone
loopback API. `desktopruntime/server.go` owns shared SQLite initialization and
HTTP setup; both entry points use the same routes, modules, validations and
migrations.

The backend provides filaments, purchases, models, print jobs, cost breakdowns,
dashboard summaries, settings and SQLite ZIP backups. All routes are available
to the local desktop application.

The local data-migration routes inspect bounded CSV/XLSX uploads, expose template
schemas, preview mapping/validation/duplicate decisions and commit supported
sections in one SQLite transaction. See `../../docs/DATA_MIGRATION.md`.

Standalone access requires the authenticated proxy described in
[LAN security setup](../../docs/LAN_SECURITY.md). Deployment and qualification
requirements are in the [Self-Hosted guide](../../docs/SELF_HOSTED.md). Both the
desktop and standalone entry points bind to loopback; CORS does not protect the
API from other local processes. Do not expose it directly on a LAN interface.

## Configuration

- `SQLITE_PATH`: optional database path override.
- `API_BIND_ADDRESS`: standalone listener IP; defaults to `127.0.0.1` and
  rejects non-loopback addresses.
- `API_PORT`: standalone port; defaults to `8080`.
- `API_SQLITE_PATH`: standalone SQLite path; defaults to
  `~/.local/share/maker-track-studio/server.db`, separate from the desktop
  database. Parent directories are created as needed.
- `API_DIAGNOSTICS_DIR`: standalone diagnostic log directory; otherwise the
  normal per-user diagnostic directory is used.
- `API_DIAGNOSTICS_LEVEL`: `DEBUG`, `INFO`, `WARNING`, `ERROR` or `CRITICAL`;
  defaults to `INFO`.
- `API_SHUTDOWN_TIMEOUT_SECONDS`: graceful shutdown limit from 1 to 300;
  defaults to 10 seconds.
- `API_PUBLIC_ORIGIN`: required exact HTTPS browser origin.
- `API_PROXY_SECRET_FILE`: required protected proxy key file; see the LAN guide.
- `API_TRUSTED_PROXIES`: required for standalone access; comma-separated proxy IPs/CIDRs. Forwarded
  client IP and HTTPS scheme headers are trusted only from these addresses.

## Standalone Linux development

Build with `go build -o /tmp/maker-track-api ./cmd/server` from `apps/api`.
Configure the credentials, trusted loopback proxy and HTTPS origin using
[LAN security setup](../../docs/LAN_SECURITY.md) before starting the binary.
The standalone command refuses to run without security configuration. Health
checks at `/health/live` and `/health/ready` require the same authenticated
boundary as application routes. SIGINT/SIGTERM drain requests and close SQLite.
Desktop startup and SQLite paths are unchanged.

## SQLite ownership and recovery

One process owns each SQLite file. Workspace requests and scheduled backups are
serialized; startup rejects another owner. See [SQLite recovery](../../docs/SQLITE_RECOVERY.md)
for storage policy, restore snapshots, compatibility and recovery procedures.

Optional standalone backups: set `API_BACKUP_DIR` to a private destination;
`API_BACKUP_TIME_UTC` defaults to `03:00`, and `API_BACKUP_RETENTION` defaults to
14 completed ZIPs. Backups are disabled without a destination. No additional
HTTP endpoints are exposed.

## Dashboard expense sources

The dashboard keeps recorded acquisition spend separate from calculated Print
Job production cost. Its all-time expense categories come from:

- printer acquisition: `printer_purchase_cost` on each physical printer profile;
- filament purchases: the transaction `price` on each filament purchase (not
  its amount multiplied by an estimated unit cost);
- spare parts, additional and other: explicitly categorized entries in
  `printer_equipment_costs`.

The derived `printer_upfront_cost`, annual maintenance estimates, printer wear,
and all Print Job cost totals are excluded so the same spend is not counted
twice. Older uncategorized equipment entries are treated as additional expenses;
virtual configuration defaults do not become expenses until the user records or
persists them.

## Validation

```bash
go test ./...
go vet ./...
```

Tests cover CRUD and cost flows, SQLite initialization, backup validation/round
trips, and spreadsheet mapping, duplicate, relationship and rollback behavior.
They use temporary or in-memory SQLite databases.
