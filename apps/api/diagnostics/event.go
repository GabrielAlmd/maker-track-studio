package diagnostics

import (
	"encoding/json"
	"os"
	"regexp"
	"time"

	"github.com/google/uuid"
)

// Input deliberately has no free-text message, arbitrary attributes, error
// object, path, SQL, request body or stack. Adapters must classify errors first.
type Input struct {
	Event       EventID
	RequestID   string
	OperationID string
	Duration    time.Duration
	ErrorCode   ErrorCode
	RouteID     string
	Method      string
	Status      int
}

type Identity struct {
	Version string
	BuildID string
	Commit  string
}

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,4})\.(0|[1-9][0-9]{0,4})\.(0|[1-9][0-9]{0,4})$`)
var buildPattern = regexp.MustCompile(`^build-[0-9]{1,20}$`)
var commitPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)

func safeIdentity(identity Identity) Identity {
	if identity.Version != "dev" && !versionPattern.MatchString(identity.Version) {
		identity.Version = "unknown"
	}
	if !buildPattern.MatchString(identity.BuildID) {
		identity.BuildID = "unknown"
	}
	if !commitPattern.MatchString(identity.Commit) {
		identity.Commit = "unknown"
	}
	return identity
}

func safeUUID(value string) string {
	if len(value) != 36 {
		return ""
	}
	id, err := uuid.Parse(value)
	if err != nil || id.Version() != 4 || id.Variant() != uuid.RFC4122 {
		return ""
	}
	return id.String()
}

type eventError struct {
	Code ErrorCode `json:"code"`
	Type string    `json:"type"`
}

func normalizeError(code ErrorCode) *eventError {
	if code == "" {
		return nil
	}
	if !code.Valid() {
		code = CodeUnknown
	}
	kind := "unknown"
	switch code {
	case CodeSQLiteInit, CodeAPIBind, CodeAPIServe:
		kind = "initialization"
	case CodePermission, CodeDiskFull, CodeIO:
		kind = "filesystem"
	case CodeTimeout:
		kind = "timeout"
	case CodeDatabase:
		kind = "database"
	case CodePanic:
		kind = "panic"
	case CodeFrontend:
		kind = "frontend"
	case CodeUnsafeState:
		kind = "unsafe_state"
	case CodeUncleanRun:
		kind = "unexpected_termination"
	}
	return &eventError{Code: code, Type: kind}
}

type wireEvent struct {
	SchemaVersion int         `json:"schema_version"`
	Timestamp     string      `json:"timestamp"`
	Level         Level       `json:"level"`
	Event         EventID     `json:"event"`
	Component     string      `json:"component"`
	Operation     string      `json:"operation"`
	Message       string      `json:"message"`
	ProcessID     int         `json:"process_id"`
	RunID         string      `json:"run_id"`
	Sequence      uint64      `json:"sequence"`
	AppVersion    string      `json:"app_version"`
	BuildID       string      `json:"build_id"`
	Commit        string      `json:"commit"`
	RequestID     string      `json:"request_id,omitempty"`
	OperationID   string      `json:"operation_id,omitempty"`
	DurationMS    float64     `json:"duration_ms,omitempty"`
	Error         *eventError `json:"error,omitempty"`
	Details       any         `json:"details,omitempty"`
}

// encodedEvent owns its bytes; it is never exposed without a copy.
type encodedEvent struct {
	data     []byte
	priority bool
	critical bool
}

func encode(input Input, identity Identity, runID string, sequence uint64, now time.Time, dropped *Counters) (encodedEvent, bool) {
	d, ok := Lookup(input.Event)
	if !ok {
		return encodedEvent{}, false
	}
	e := wireEvent{
		SchemaVersion: SchemaVersion, Timestamp: now.UTC().Format("2006-01-02T15:04:05.000Z"),
		Level: d.Level, Event: input.Event, Component: d.Component, Operation: d.Operation, Message: d.Message,
		ProcessID: os.Getpid(), RunID: runID, Sequence: sequence,
		AppVersion: identity.Version, BuildID: identity.BuildID, Commit: identity.Commit,
		RequestID: safeUUID(input.RequestID), OperationID: safeUUID(input.OperationID),
		Error: normalizeError(input.ErrorCode),
	}
	if dropped != nil {
		e.Details = dropped
	}
	if input.Event == RequestCompleted || input.Event == RequestFailed || input.Event == MutationCompleted {
		e.Details = safeHTTPDetails(input)
	}
	if input.Duration > 0 {
		e.DurationMS = float64(input.Duration) / float64(time.Millisecond)
	}
	data, err := json.Marshal(e)
	if err != nil || len(data)+1 > MaxEventBytes {
		return encodedEvent{}, false
	}
	data = append(data, '\n')
	return encodedEvent{data, d.Level == Error || d.Level == Critical, d.Level == Critical}, true
}

type httpDetails struct {
	Method string `json:"method,omitempty"`
	Route  string `json:"route_id,omitempty"`
	Status int    `json:"status,omitempty"`
}

func safeHTTPDetails(input Input) httpDetails {
	d := httpDetails{}
	switch input.Method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		d.Method = input.Method
	}
	switch input.RouteID {
	case "backup.export", "backup.import", "filament-types", "filaments", "filament",
		"purchases", "purchase", "purchases.batch", "models", "model", "print-jobs", "print-job",
		"print-jobs.dashboard", "print-jobs.stats", "print-job.cost", "job-filaments", "job-filament",
		"config", "config.entry", "migration.schema", "migration.template", "migration.inspect", "migration.preview", "migration.import", "unmatched":
		d.Route = input.RouteID
	}
	if input.Status >= 100 && input.Status <= 599 {
		d.Status = input.Status
	}
	return d
}
