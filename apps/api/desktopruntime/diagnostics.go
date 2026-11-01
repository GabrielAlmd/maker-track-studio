package desktopruntime

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/diagnostics"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type requestIDKey struct{}

func requestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

func emit(events *diagnostics.Logger, input diagnostics.Input) {
	if events != nil {
		events.Emit(input)
	}
}

var routeIDs = map[string]string{
	"/desktop-data/export": "backup.export", "/desktop-data/import": "backup.import",
	"/data-migration/schema": "migration.schema", "/data-migration/templates/:section": "migration.template",
	"/data-migration/inspect": "migration.inspect", "/data-migration/preview": "migration.preview", "/data-migration/import": "migration.import",
	"/filament-types": "filament-types", "/filaments": "filaments", "/filaments/:id": "filament",
	"/filament-purchases": "purchases", "/filament-purchases/:id": "purchase", "/filament-purchases/batch": "purchases.batch",
	"/models": "models", "/models/:id": "model", "/print-jobs": "print-jobs", "/print-jobs/:id": "print-job",
	"/print-jobs/dashboard-summary": "print-jobs.dashboard", "/print-jobs/stats": "print-jobs.stats", "/print-jobs/:id/cost": "print-job.cost",
	"/print-job-filaments": "job-filaments", "/print-job-filaments/:id": "job-filament", "/app-config": "config", "/app-config/:key": "config.entry",
}

func diagnosticMiddleware(events *diagnostics.Logger) gin.HandlerFunc {
	var unsafe atomic.Bool
	return func(c *gin.Context) {
		started := time.Now()
		id, err := uuid.NewRandom()
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": "REQUEST_INIT_FAILED", "error": "Could not initialize request"})
			return
		}
		request := id.String()
		c.Header("X-Request-ID", request)
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), requestIDKey{}, request))
		route := routeIDs[c.FullPath()]
		if route == "" {
			route = "unmatched"
		}
		isImport := route == "backup.import"
		defer func() {
			if recover() != nil {
				// No raw panic value, headers, request body or runtime stack.
				unsafe.Store(true)
				emit(events, diagnostics.Input{Event: diagnostics.StateUnsafe, RequestID: request, ErrorCode: diagnostics.CodePanic})
				emit(events, diagnostics.Input{Event: diagnostics.RequestFailed, RequestID: request, ErrorCode: diagnostics.CodePanic,
					RouteID: route, Method: c.Request.Method, Status: http.StatusInternalServerError, Duration: time.Since(started)})
				if !c.Writer.Written() {
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": "INTERNAL_ERROR", "error": "Internal server error"})
				} else {
					c.Abort()
				}
				if isImport {
					emit(events, diagnostics.Input{Event: diagnostics.BackupImportFailed, RequestID: request, ErrorCode: diagnostics.CodePanic})
				}
				return
			}
			status := c.Writer.Status()
			input := diagnostics.Input{Event: diagnostics.RequestCompleted, RequestID: request,
				Duration: time.Since(started), RouteID: route, Method: c.Request.Method, Status: status}
			if status >= 500 {
				input.Event = diagnostics.RequestFailed
				input.ErrorCode = diagnostics.CodeUnknown
			} else if status < 300 && (c.Request.Method == "POST" || c.Request.Method == "PUT" || c.Request.Method == "DELETE") {
				input.Event = diagnostics.MutationCompleted
			}
			emit(events, input)
			if isImport {
				input.Event = diagnostics.BackupImportDone
				if status >= 500 {
					input.Event = diagnostics.BackupImportFailed
				} else if status >= 400 {
					input.Event = diagnostics.BackupImportRejected
				}
				emit(events, input)
			}
		}()
		// Unknown process state blocks business operations while backup recovery
		// remains available.
		if unsafe.Load() && route != "backup.export" && route != "backup.import" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": "APP_STATE_UNSAFE", "error": "Restart the application before continuing"})
			return
		}
		if isImport {
			emit(events, diagnostics.Input{Event: diagnostics.BackupImportStarted, RequestID: request})
		}
		c.Next()
	}
}

type databaseLogger struct {
	events *diagnostics.Logger
	level  logger.LogLevel
}

func (l databaseLogger) LogMode(level logger.LogLevel) logger.Interface { l.level = level; return l }

// GORM's arbitrary formatted messages/arguments are not a diagnostic contract.
func (l databaseLogger) Info(context.Context, string, ...interface{}) {}
func (l databaseLogger) Warn(context.Context, string, ...interface{}) {}
func (l databaseLogger) Error(ctx context.Context, _ string, _ ...interface{}) {
	if l.level >= logger.Error {
		emit(l.events, diagnostics.Input{Event: diagnostics.DatabaseFailed, RequestID: requestID(ctx), ErrorCode: diagnostics.CodeDatabase})
	}
}
func (l databaseLogger) Trace(ctx context.Context, begin time.Time, _ func() (string, int64), err error) {
	// Never evaluate fc: even parameterized SQL can contain customer data.
	duration := time.Since(begin)
	input := diagnostics.Input{RequestID: requestID(ctx), Duration: duration}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) && l.level >= logger.Error {
		input.Event, input.ErrorCode = diagnostics.DatabaseFailed, diagnostics.CodeDatabase
	} else if duration > 500*time.Millisecond && l.level >= logger.Warn {
		input.Event = diagnostics.DatabaseSlow
	} else {
		return
	}
	emit(l.events, input)
}

// net/http normally prints panic/request details through its ErrorLog. This
// adapter accepts the notification without interpreting or retaining the bytes.
type httpErrorWriter struct{ events *diagnostics.Logger }

func (w httpErrorWriter) Write(p []byte) (int, error) {
	emit(w.events, diagnostics.Input{Event: diagnostics.RequestFailed, ErrorCode: diagnostics.CodeUnknown})
	return len(p), nil
}
func safeHTTPErrorLog(events *diagnostics.Logger) *log.Logger {
	return log.New(httpErrorWriter{events}, "", 0)
}
