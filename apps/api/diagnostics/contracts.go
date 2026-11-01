// Package diagnostics provides bounded, sanitized local logging. The desktop
// integration and diagnostic report generation are implemented separately.
package diagnostics

import "log/slog"

const SchemaVersion = 1

type Level string

const (
	Debug    Level = "DEBUG"
	Info     Level = "INFO"
	Warning  Level = "WARNING"
	Error    Level = "ERROR"
	Critical Level = "CRITICAL"
)

// SlogLevel deliberately rejects unknown levels instead of silently treating
// them as INFO. CRITICAL describes severity; it never exits the process.
func (l Level) SlogLevel() (slog.Level, bool) {
	switch l {
	case Debug:
		return slog.LevelDebug, true
	case Info:
		return slog.LevelInfo, true
	case Warning:
		return slog.LevelWarn, true
	case Error:
		return slog.LevelError, true
	case Critical:
		return slog.LevelError + 4, true
	default:
		return 0, false
	}
}

type EventID string

const (
	AppStarting           EventID = "app.starting"
	AppReady              EventID = "app.ready"
	AppStopping           EventID = "app.stopping"
	AppStopped            EventID = "app.stopped"
	StartupFailed         EventID = "app.startup.failed"
	APIStopped            EventID = "api.stopped.unexpectedly"
	RequestCompleted      EventID = "http.request.completed"
	RequestFailed         EventID = "http.request.failed"
	OriginRejected        EventID = "http.origin.rejected"
	DatabaseSlow          EventID = "database.operation.slow"
	DatabaseFailed        EventID = "database.operation.failed"
	BackupExportStarted   EventID = "backup.export.started"
	BackupExportDone      EventID = "backup.export.completed"
	BackupExportFailed    EventID = "backup.export.failed"
	BackupImportStarted   EventID = "backup.import.started"
	BackupImportDone      EventID = "backup.import.completed"
	BackupImportFailed    EventID = "backup.import.failed"
	ConfigFallback        EventID = "diagnostics.config.fallback"
	EventsDropped         EventID = "logging.events_dropped"
	PersistenceFailed     EventID = "logging.persistence.failed"
	PersistenceRestored   EventID = "logging.persistence.restored"
	UnexpectedError       EventID = "runtime.unexpected_error"
	StateUnsafe           EventID = "runtime.state.unsafe"
	FrontendFailed        EventID = "frontend.failed"
	ReportCreated         EventID = "report.created"
	ReportFailed          EventID = "report.failed"
	PreviousRunUnclean    EventID = "app.previous_run.unclean"
	ShutdownFailed        EventID = "app.shutdown.failed"
	DesktopWarning        EventID = "desktop.runtime.warning"
	DesktopError          EventID = "desktop.runtime.error"
	DesktopFatal          EventID = "desktop.runtime.fatal"
	FrontendNetworkFailed EventID = "frontend.network.failed"
	FrontendPromiseFailed EventID = "frontend.promise.failed"
	FrontendScriptFailed  EventID = "frontend.script.failed"
	BackupExportCancelled EventID = "backup.export.cancelled"
	DatabaseReady         EventID = "database.ready"
	MutationCompleted     EventID = "http.mutation.completed"
	BackupImportRejected  EventID = "backup.import.rejected"
)

// Definition is a value copy of a catalog entry. Callers cannot mutate the
// catalog or supply a free-text message through this contract.
type Definition struct {
	Level     Level
	Component string
	Operation string
	Message   string
}

var catalog = map[EventID]Definition{
	AppStarting:           {Info, "desktop", "startup", "Application starting"},
	AppReady:              {Info, "desktop", "startup", "Application ready"},
	AppStopping:           {Info, "desktop", "shutdown", "Application stopping"},
	AppStopped:            {Info, "desktop", "shutdown", "Application stopped"},
	StartupFailed:         {Critical, "desktop", "startup", "Application initialization failed"},
	APIStopped:            {Critical, "api", "serve", "Local API stopped unexpectedly"},
	RequestCompleted:      {Debug, "http", "request", "Local request completed"},
	RequestFailed:         {Error, "http", "request", "Local request failed unexpectedly"},
	OriginRejected:        {Warning, "http", "cors", "Request origin rejected"},
	DatabaseSlow:          {Warning, "database", "query", "Database operation exceeded duration threshold"},
	DatabaseFailed:        {Error, "database", "query", "Database operation failed"},
	BackupExportStarted:   {Info, "desktop.backup", "backup.export", "Backup export started"},
	BackupExportDone:      {Info, "desktop.backup", "backup.export", "Backup export completed"},
	BackupExportFailed:    {Error, "desktop.backup", "backup.export", "Backup export failed"},
	BackupImportStarted:   {Info, "api.backup", "backup.import", "Backup import started"},
	BackupImportDone:      {Info, "api.backup", "backup.import", "Backup import completed"},
	BackupImportFailed:    {Error, "api.backup", "backup.import", "Backup import failed"},
	ConfigFallback:        {Warning, "diagnostics", "configure", "Invalid diagnostic configuration replaced by defaults"},
	EventsDropped:         {Warning, "diagnostics", "enqueue", "Diagnostic events dropped"},
	PersistenceFailed:     {Warning, "diagnostics", "persist", "Diagnostic persistence unavailable"},
	PersistenceRestored:   {Info, "diagnostics", "persist", "Diagnostic persistence restored"},
	UnexpectedError:       {Error, "runtime", "recover", "Unexpected operation failure"},
	StateUnsafe:           {Critical, "runtime", "recover", "Application state may be unsafe"},
	FrontendFailed:        {Error, "frontend", "render_or_execute", "Frontend operation failed unexpectedly"},
	ReportCreated:         {Info, "diagnostics", "report", "Diagnostic report created"},
	ReportFailed:          {Warning, "diagnostics", "report", "Diagnostic report could not be saved"},
	PreviousRunUnclean:    {Warning, "desktop", "startup.recovery", "Previous execution has no confirmed clean shutdown"},
	ShutdownFailed:        {Error, "desktop", "shutdown", "Application shutdown did not complete cleanly"},
	DesktopWarning:        {Warning, "desktop", "runtime", "Desktop runtime reported a warning"},
	DesktopError:          {Error, "desktop", "runtime", "Desktop runtime reported an error"},
	DesktopFatal:          {Critical, "desktop", "runtime", "Desktop runtime reported a fatal failure"},
	FrontendNetworkFailed: {Warning, "frontend", "http", "Local API could not be reached"},
	FrontendPromiseFailed: {Error, "frontend", "promise", "Unhandled frontend promise rejection"},
	FrontendScriptFailed:  {Error, "frontend", "script", "Unhandled frontend script error"},
	BackupExportCancelled: {Info, "desktop.backup", "backup.export", "Backup export cancelled"},
	DatabaseReady:         {Info, "database", "startup", "Local database initialized"},
	MutationCompleted:     {Info, "http", "mutation", "Local data operation completed"},
	BackupImportRejected:  {Info, "api.backup", "backup.import", "Backup import rejected"},
}

func Lookup(id EventID) (Definition, bool) {
	d, ok := catalog[id]
	return d, ok
}

type ErrorCode string

const (
	CodeUnknown           ErrorCode = "UNEXPECTED_FAILURE"
	CodeSQLiteInit        ErrorCode = "SQLITE_INIT_FAILED"
	CodeAPIBind           ErrorCode = "API_BIND_FAILED"
	CodeAPIServe          ErrorCode = "API_SERVE_FAILED"
	CodePermission        ErrorCode = "FILESYSTEM_PERMISSION"
	CodeDiskFull          ErrorCode = "FILESYSTEM_FULL"
	CodeIO                ErrorCode = "FILESYSTEM_IO_FAILED"
	CodeTimeout           ErrorCode = "OPERATION_TIMEOUT"
	CodeDatabase          ErrorCode = "DATABASE_OPERATION_FAILED"
	CodePanic             ErrorCode = "UNEXPECTED_PANIC"
	CodeUnsafeState       ErrorCode = "STATE_UNSAFE"
	CodeFrontend          ErrorCode = "FRONTEND_UNEXPECTED_FAILURE"
	CodeUncleanRun        ErrorCode = "PREVIOUS_RUN_UNCLEAN"
	CodeInvalidDiagnostic ErrorCode = "DIAGNOSTIC_INPUT_REJECTED"
)

func (c ErrorCode) Valid() bool {
	switch c {
	case CodeUnknown, CodeSQLiteInit, CodeAPIBind, CodeAPIServe, CodePermission,
		CodeDiskFull, CodeIO, CodeTimeout, CodeDatabase, CodePanic, CodeUnsafeState,
		CodeFrontend, CodeUncleanRun, CodeInvalidDiagnostic:
		return true
	default:
		return false
	}
}
