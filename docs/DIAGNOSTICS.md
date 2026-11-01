# Local diagnostics contract

Status: phases 1–3 implemented. The desktop initializes local logging before
SQLite and the API, and integrates Gin, GORM, Wails and React. Diagnostic reports,
native crash capture and next-start recovery are not active yet. The authoritative
catalog and resource defaults are in `apps/api/diagnostics`. Contracts for later
phases below describe planned behavior, not existing report functionality.

## Delivery phases

| Phase | Deliverable | Status |
| --- | --- | --- |
| 1 | Levels, event/error catalog, configuration, privacy and wire contracts | Complete |
| 2 | Sanitizer, bounded queue/recent buffer, single writer and rotation | Implemented; Windows runtime verification pending |
| 3 | Desktop lifecycle, Gin/GORM and frontend integration | Implemented; native Windows execution pending |
| 4 | Incident coordination, reports, retention and native export | Pending |
| 5 | Run markers and next-start recovery | Pending |
| 6 | Stress, privacy and packaged Windows/Linux verification | Pending |

Future work, outside these phases: a free-text Report a Problem form, native
dumps, external watchdog and GitHub integration. No automatic network reporting.

## Finding application logs

Start the updated desktop executable (or Wails development mode). Files are
created before database/API initialization, including when initialization fails:

- Linux: `${XDG_STATE_HOME:-~/.local/state}/3d-printing-cost-tracker/logs/application.log`.
  Relative XDG_STATE_HOME values are ignored in favor of the home default.
- Windows: `%LOCALAPPDATA%\3d-printing-cost-tracker\diagnostics\logs\application.log`.
  If LOCALAPPDATA is unavailable, use the user's `AppData\Local` directory.

Rotated files use `.1`, `.2`, `.3`. The current file is created immediately and
buffered normal events appear within the one-second flush interval on healthy
storage. Default production level is INFO. Opening a normal screen or performing
a GET does not necessarily add an INFO event; startup, data mutations and backup
operations do. Running `--version` does not create logs or initialize SQLite.
Launching only Vite does not create Go diagnostic files.

Optional process environment settings (restart to apply):

| Setting | Default | Accepted values |
| --- | --- | --- |
| DIAGNOSTICS_LEVEL | INFO | DEBUG, INFO, WARNING, ERROR, CRITICAL |
| DIAGNOSTICS_MAX_FILE_MIB | 10 | Integer 1–100 |

An invalid value selects the complete defaults and a fixed warning, without
logging the rejected input. DEBUG automatically reverts to INFO after 30 minutes
of that execution. These environment settings do not change SQLite AppConfig,
backup contents or application build identity.

When file initialization fails, the application keeps bounded in-memory events,
prints a fixed notice without paths to stderr, and displays a nonblocking warning
after the frontend is ready. A conflicting writer lock is different: the second
instance shows an error and does not initialize SQLite or bind the local port.
Runtime write failures are reflected by the same frontend status warning.

If file initialization fails, the warning also shows a sanitized support code
such as `DIAG_WINDOWS_SECURITY_OPEN_5`. This identifies the failing initialization
step and numeric OS error, without storing the original error or user path.
Report that code rather than changing permissions or running as administrator.
`DIAG_UNAVAILABLE` means the failing operation was not classified (including
runtime failures after successful initialization). A user report with only the
older generic banner does not establish whether the failure was ACL, directory,
lock or I/O related.

Windows CI now executes native diagnostic storage tests and requires both the
installed executable and ZIP bundle to persist `app.ready` in their own temporary
LOCALAPPDATA. An API-only smoke-test success is insufficient. These checks must
run on Windows; cross-compilation does not establish correct ACL/locking behavior.

The application never confirms that an event is durable merely because it was
accepted into the queue. Files cannot be guaranteed when the directory is
unwritable, a filesystem call is blocked or the application is killed abruptly.

## Phase 3 integration

`apps/desktop/main.go` initializes the writer and logger before Wails startup.
Deferred cleanup covers Wails initialization failure as well as normal shutdown;
shutdown waits for requests using an independent timeout before draining logs.
Startup errors become fixed failure codes visible through `GetRuntimeStatus`.
React waits for this status before mounting API consumers, avoiding requests to
an unrelated service when port binding fails. No database is needed for status
or the restricted `ReportFrontendFailure(kind, requestID)` binding.

Gin uses explicit recovery/access middleware instead of `gin.Default()`. Each
request receives a backend-generated UUID in `X-Request-ID`; caller-supplied IDs
are ignored. CORS exposes this response header only under the existing origin
policy. Routes bind a GORM session with the request context. SQL callbacks and
formatted GORM messages are never evaluated for logging, including the temporary
database opened during backup import. Slow queries use a 500 ms threshold;
expected record-not-found results are silent. Request and database errors can be
correlated without storing SQL values, raw routes, URLs or bodies.

A recovered request panic records fixed critical/error events and returns a
generic 500 when headers have not been sent. Subsequent business requests return
503 until restart; established backup recovery endpoints remain accessible. This conservative state flag is not a database repair mechanism and
cannot reverse side effects or cancel already-running requests. Raw stacks and
panic values are not collected in this phase.

React has a rendering error boundary and global error/rejection listeners. Axios
network failures submit a fixed kind; server failures remain owned by the backend.
Cancellation is not a network failure. Each frontend kind is limited to once per
five seconds on both sides of the bridge. Wails warnings/errors are similarly
limited fixed notifications, without message text. No report is generated yet.

## Implemented package boundary

`New` accepts effective configuration, a narrowly validated build identity and
a `Sink`; nil selects memory-only collection. `Emit(Input)` accepts catalog IDs,
UUID correlation IDs, duration and a known error code. `Slog()` provides a
restricted `slog.Handler`: messages must be catalog IDs, and only `request_id`,
`operation_id`, `error_code` and typed `duration` attributes are accepted. Arbitrary
attributes, groups, error objects and LogValuer/Stringer values are not evaluated
or retained. Every enabled record is sanitized before the recent buffer/queue.

Producers cannot submit frames or arbitrary event-specific details. HTTP adapters
provide closed route IDs, known methods and bounded status codes; internal drop
counters have a separate fixed shape. Error code normalization replaces redaction of
free text, because no free-text error input is accepted. With these bounded fields
records are well below the event ceiling; oversize records fail closed.

`NewRotatingWriter` creates private storage and acquires its exclusive OS lock.
It is a low-level sink for sanitized events, not an input validator for arbitrary
JSON. Constructor failure returns a fixed error; the future desktop adapter must
choose memory-only operation and communicate that degradation. Runtime I/O
failures suspend file writing and use bounded retry delays. Raw filesystem error
messages are not exposed by the package.

`Recent()` returns owned JSON copies, `Dropped()` counts known queue/write losses,
and `PossibleBufferedLoss()` indicates that earlier buffered events may also have
been lost in unknown quantities. Health transitions are kept in the recent buffer
without recursively attempting persistence; drop summaries are persisted when
the sink permits. `Close` stops admission and waits at most two seconds; a blocked
worker retains its lock until its I/O actually returns. Do not start a replacement
worker over the same directory after a shutdown timeout.

Phase 2 verification: race-enabled tests with 100 producers / 100,000 events,
priority reserve, shutdown timeout, secret rejection, immutable snapshots,
rotation boundaries/retention, partial-tail recovery, injected rename/flush
failures, exclusive ownership, Linux permissions and link rejection. Windows
tests cross-compile, but ACL/lock behavior still requires native execution in
phase 6. Real full-disk, power-loss and packaged artifact tests remain pending.

## Ownership and compatibility

One Go process owns diagnostics before SQLite/API startup. React submits a
restricted diagnostic input through Wails, including when HTTP is unavailable.
The backend assigns trusted timestamps, sequence, run ID and build identity;
frontend callers cannot override them. Never trust a frontend payload merely
because it came through the desktop bridge.

The future package must not depend on SQLite, Gin or Wails.
Platform adapters and application integration live outside its portable core.
Existing SQLite and backup formats, loopback binding and CORS restrictions remain unchanged. Support export is available without a healthy API or database.

Use `%LOCALAPPDATA%/3d-printing-cost-tracker/diagnostics` on Windows and
`$XDG_STATE_HOME/3d-printing-cost-tracker` on Linux, falling back to
`~/.local/state/3d-printing-cost-tracker`. Separate logs, reports and run state.
Do not move the database. Restrict Linux directories/files to 0700/0600 and
request private user ACLs on Windows. If an enterprise policy denies changing
DACLs, retain the inherited `%LOCALAPPDATA%` ACL (the OS user application-data
boundary) and keep diagnostics working; never fall back to a public directory.

An OS-managed exclusive lock protects the complete diagnostics directory. A
second process must not rotate files or interpret the active process as crashed.
Do not infer exclusive ownership from a PID file alone. Future child processes
must forward events to the owner instead of sharing a file writer.

No schema migration is needed. Rollback leaves diagnostic files separate from
business data; unsupported diagnostic versions must not be exported as raw data.

## Events and severity

`contracts.go` is the initial event catalog. Each ID fixes its message, component,
operation and default level. Extend it explicitly when integrating a new
operation; do not create dynamic event IDs or override messages with error text.

| Level | Meaning |
| --- | --- |
| DEBUG | Temporary technical detail; routine successful HTTP completion |
| INFO | Startup/shutdown and significant operation boundaries |
| WARNING | Recoverable degradation, retries, slow operations, dropped events |
| ERROR | Unexpected failed operation with continued application availability |
| CRITICAL | Essential component failure or potentially unsafe application state |

Validation errors and cancellations are not unexpected failures. Aggregate repeated warnings. Log retries with a bounded attempt count,
then emit one definitive failure. Do not log every allocation, render or poll.

Severity never calls exit or panic. `UnexpectedError` is for an isolated failure;
use `StateUnsafe` if continued writes cannot be justified. Incident classification
is separate from severity; not every ERROR produces a report. Reporter failures
must never recursively request reports.

Error codes in `contracts.go` are a closed vocabulary. Normalize unknown errors
to `UNEXPECTED_FAILURE`; never preserve arbitrary `Error()` or panic values.
Operation-specific event IDs plus technical error codes identify failures without
embedding file paths, SQL or request contents.

## Event wire contract (schema version 1)

JSON Lines, UTF-8, one complete object followed by a newline. Unknown fields are
rejected at ingestion, not automatically forwarded. Omit absent optional fields.
This is the target serialized contract, not an unsanitized producer DTO.

| Field | Type and validation |
| --- | --- |
| schema_version | Integer, exactly 1 |
| timestamp | Backend UTC RFC3339 with milliseconds |
| level | DEBUG, INFO, WARNING, ERROR or CRITICAL |
| event | Known catalog ID |
| component, operation, message | Exactly the catalog values |
| process_id | Positive integer assigned by Go |
| run_id | Locally generated UUID v4, new per execution |
| sequence | Positive integer, ordered at admission, scoped to run_id |
| app_version | Validated build version, or fixed `dev`/`unknown` fallback |
| build_id | Validated build identifier, or `unknown` |
| commit | Lowercase 40-character Git hash from build metadata, or `unknown` |
| request_id, operation_id | Optional locally generated UUIDs |
| duration_ms | Optional finite nonnegative number, monotonic measurement |
| error | Optional object: known `code`, controlled `type`, optional `frames` |
| truncated | Optional boolean |
| details | Optional event-specific allowlist below; never arbitrary attributes |

Allowed `error.type` values: `initialization`, `filesystem`, `timeout`, `database`,
`panic`, `frontend`, `unsafe_state`, `unexpected_termination`, `unknown`.
Frames: at most 64 objects containing validated function, source-relative file
and positive line number. No arguments, locals, absolute paths or raw runtime
stack text. Unsupported frame text is omitted, not passed through.

Initial detail allowlist:

| Events | Allowed detail fields |
| --- | --- |
| http.request.*, http.mutation.completed | method from known HTTP methods, route ID from a backend catalog, status 100–599 |
| http.origin.rejected | None; omit raw Origin, path, query and headers |
| database.operation.* | None; duration is sufficient initially |
| logging.events_dropped | Nonnegative integer counters per known level |
| logging.persistence.* | Controlled stage: open/write/flush/sync/rotate |
| diagnostics.config.fallback | None; never include rejected values |
| report.created | Valid report ID |
| app.previous_run.unclean | Previous UUID run ID |
| Other initial events | None |

All correlation IDs are diagnostic IDs, not customer/database row identifiers.
Do not include thread or goroutine IDs by parsing stack strings. Routine events
need no stack. DEBUG uses the same privacy rules as every other level.

Example (illustrative build identity):

```json
{"schema_version":1,"timestamp":"2026-09-17T14:35:27.421Z","level":"ERROR","event":"backup.export.failed","component":"desktop.backup","operation":"backup.export","message":"Backup export failed","process_id":4312,"run_id":"c088e8ae-96fd-48d0-bf17-c09e2f5d8368","sequence":184,"app_version":"1.2.3","build_id":"unknown","duration_ms":842,"error":{"code":"FILESYSTEM_PERMISSION","type":"filesystem"}}
```

## Privacy boundary

Sanitize before any queue, recent buffer, console output or file persistence.
Revalidate independently when building a report and again before export. Unknown
fields are discarded/rejected; sanitizer failure yields only a fixed minimal
catalog event. Never fall back to serializing the original object.

| Permitted | Forbidden |
| --- | --- |
| Fixed operation name, outcome, duration | Customer names, job notes/descriptions, costs and margins |
| Known error code | Raw `error.Error()`, arbitrary panic payload |
| Route identifier and method | Full URL, query, HTTP body, cookies, authorization headers |
| Database operation duration | SQL text/parameters, rows, backups, secret tokens |
| Effective diagnostic limits | Environment dump, AppConfig dump, command-line arguments |
| Source-relative approved frame | Home paths, usernames, hostnames, private endpoints |
| Version, architecture, runtime | Keys, passwords, secrets, machine fingerprint |

Even strings with allowed names must be validated. Regex redaction is defense
in depth, never permission to accept free text. Secret-shaped fields are omitted;
redacted representations such as `Authorization: Bearer ***` are only appropriate
in a controlled display, not a reason to collect headers. Escape control bytes
and enforce byte budgets after JSON encoding. Invalid UTF-8 must not bypass
limits or create invalid JSON.

Review additions to the catalog and detail allowlist. Check direct log/console
calls and object dumps during integration. Seed synthetic secrets in every input
boundary and assert their absence from all outputs, including temporary files.

## Resource and failure contract

Defaults are defined in `config.go`. User settings initially consist only of
minimum level and file size (1–100 MiB). Invalid configuration falls back to all
defaults and emits a single safe warning. The desktop loads the two documented
environment variables, never a database configuration dump. The temporary DEBUG
override expires after 30 minutes and reverts to INFO for the current execution.

One writer owns `application.log` and exactly three prior slots `.1`–`.3`.
Rotate before an encoded event would exceed the configured size (10 MiB default).
Never split an event. The 16 KiB event ceiling includes JSON encoding and newline.
Truncate supported fields while preserving valid JSON; record truncation.

Rotation is not a multi-file atomic transaction. Handle failed renames and gaps
conservatively. On failure, suspend persistence instead of growing the active
file indefinitely. Retry after 1, 5, 15, then 60 seconds without spawning an
unbounded number of workers. Disk/permission/lock errors never terminate business
operations or cause recursive logging. Do not delete customer backups for space.

Queue limits apply simultaneously to bytes and count. Reserve 128 event slots
AND 128 × MaxEventBytes of its 8 MiB byte budget for ERROR/CRITICAL admission.
Lower-priority producers cannot consume that reserve. Drop rather than wait for
disk; count losses by level. Critical reports use an independent bounded pending
slot, not an unbounded priority bypass. Snapshot immutable sanitized values;
never retain application objects or locks across filesystem operations.

Flush every second or at 64 KiB. Critical incidents request Sync; normal shutdown
waits up to two seconds. A timeout stops waiting, not necessarily a kernel I/O
operation. Abrupt termination may lose queued events; power loss may lose OS
buffers. No durability or performance guarantee is claimed before measurement.

## Report contract (schema version 1)

Canonical private storage: one JSON document. Export: ZIP with only `report.json`
and derived `summary.txt`. Both representations use the same sanitized source.
No database, full logs, screenshots, memory dumps or arbitrary attachments.

| Section | Allowed contents |
| --- | --- |
| metadata | schema_version=1, sanitizer_version=1, report_id, incident_id, run_id, created_at, origin |
| application | Product identity, validated version/build/commit, release or development build type |
| environment | OS/version, architecture, Go version, optional WebView version; validated technical identifiers only |
| error | Known code/type, catalog component/operation, sanitized frames; nullable occurred_at |
| runtime_context | Diagnostic correlation IDs, controlled phase/component states, duration |
| recent_events | At most 200 events / 1 MiB, including the triggering event when available |
| configuration | Effective diagnostic limits only in the first version |
| quality | incomplete, truncated, dropped counts, controlled collection error codes, cause confidence |

Origin is `automatic`, `startup_recovery` or future `manual`. A report ID is a
UTC `YYYYMMDDTHHMMSSZ` prefix plus a UUID v4. Distinguish incident occurrence
from report creation; previous-run occurrence time can be unknown. Classification
confidence is `observed` or `inferred`; an unclean marker cannot prove a crash.

No arbitrary extension maps. Section fields must be explicitly versioned as the
implementation expands. Optional runtime resource values must be bounded and
cannot contain process lists, hardware serials or network inventory. Free-text
user descriptions are deferred, not silently included in version 1.

Build from bounded in-memory snapshots without querying the failing database,
HTTP service or business locks. One active generation. Max 2 MiB uncompressed;
write privately to a temporary file, sync/close and publish in the same filesystem.
Never list a temporary file as a completed report. Enforce retention: 10 reports,
30 days, 20 MiB, whichever is reached first; bound staging files separately to
one report and clean abandoned staging on startup. User-selected exports are not
subject to internal deletion. ZIP is not encryption.

Deduplicate by version/component/code/normalized frames for 10 minutes, at most
five automatic reports per run. Aggregate repeats. A failed report never triggers
another report. Reapply current privacy rules before exporting an old report;
unsupported schema versions fail closed. Native dialogs control export paths;
frontend requests identify reports by validated ID, never arbitrary local paths.

## Recovery and crash boundaries

Write a minimal private run marker after obtaining exclusive ownership. Read the
previous marker before replacing it. Mark clean only after application shutdown
has completed. Marker-write failure means recovery evidence may be unavailable;
it must not prevent ordinary application use.

An unclean marker leads to a best-effort report using bounded valid previous-run
events and existing reports. Deduplicate by previous run/incident. State that the
previous execution has no confirmed clean shutdown, not that its cause is known.
Do not repair SQLite automatically. Keep support and backup recovery usable.

Recover Go panics only at deliberate goroutine/request boundaries; continuation
requires known invariants. React boundaries and global JS listeners complement
each other but cannot capture a dead renderer. Native faults, fatal runtime
errors, OOM, SIGKILL, power loss and hangs have no in-process report guarantee.
Do not serialize, allocate, lock or invoke normal logging from native crash
handlers. Raw SetCrashOutput, minidumps and core dumps are disabled by default
because they bypass the sanitization contract.

## Verification gates

Phase 1: focused Go tests for levels, catalog isolation, unknown inputs and safe
configuration fallback. Subsequent phases add sanitizer/adversarial tests,
rotation fault injection, race/stress checks, previous-run subprocess crash
tests and real packaged Windows/Linux checks. Test failure at open/write/flush/
sync/rename/delete, full disk, blocked files, partial lines and reporter failure.

Targets for a declared reference machine: ordinary admission p95 below 1 ms,
less than 5% overhead in representative flows, report generation below 2 seconds
on a healthy local disk. Record actual measurements before claiming acceptance.
