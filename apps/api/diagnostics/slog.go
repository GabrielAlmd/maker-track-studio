package diagnostics

import (
	"context"
	"log/slog"
	"time"
)

// Slog returns a restricted adapter. Record.Message must be a catalog EventID;
// the catalog owns severity and message. Unknown keys/groups are discarded and
// LogValuer/Stringer methods are never evaluated. Use Emit for typed callers.
func (l *Logger) Slog() *slog.Logger { return slog.New(safeHandler{logger: l}) }

type safeHandler struct {
	logger  *Logger
	input   Input
	grouped bool
}

func (h safeHandler) Enabled(_ context.Context, level slog.Level) bool {
	minimum, _ := h.logger.minimumLevel().SlogLevel()
	return level >= minimum
}

func acceptAttribute(input *Input, attr slog.Attr) {
	// Inspect kinds, not Value.Resolve or Value.Any: opaque values are rejected.
	switch attr.Key {
	case "request_id":
		if attr.Value.Kind() == slog.KindString {
			input.RequestID = safeUUID(attr.Value.String())
		}
	case "operation_id":
		if attr.Value.Kind() == slog.KindString {
			input.OperationID = safeUUID(attr.Value.String())
		}
	case "error_code":
		if attr.Value.Kind() == slog.KindString {
			code := ErrorCode(attr.Value.String())
			if !code.Valid() {
				code = CodeUnknown
			}
			input.ErrorCode = code
		}
	case "duration":
		if attr.Value.Kind() == slog.KindDuration {
			input.Duration = max(time.Duration(0), attr.Value.Duration())
		}
	}
}

func (h safeHandler) Handle(_ context.Context, record slog.Record) error {
	input := h.input
	input.Event = EventID(record.Message)
	if !h.grouped {
		record.Attrs(func(a slog.Attr) bool { acceptAttribute(&input, a); return true })
	}
	h.logger.Emit(input)
	return nil
}

func (h safeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if !h.grouped {
		for _, attr := range attrs {
			acceptAttribute(&h.input, attr)
		}
	}
	return h
}

func (h safeHandler) WithGroup(name string) slog.Handler {
	if name != "" {
		h.grouped = true
	}
	return h
}
