package diagnostics

import (
	"errors"
	"time"
)

const (
	MiB                         = 1024 * 1024
	PreviousLogFiles            = 3
	MaxEventBytes               = 16 * 1024
	QueueEvents                 = 2048
	QueueBytes                  = 8 * MiB
	PriorityReserveEvents       = 128
	RecentEvents                = 200
	RecentBytes                 = MiB
	WriteBufferBytes            = 64 * 1024
	MaxReportBytes              = 2 * MiB
	MaxStoredReports            = 10
	MaxStoredReportBytes        = 20 * MiB
	MaxAutomaticReportsPerRun   = 5
	MaxDescriptionCharacters    = 2000
	FlushInterval               = time.Second
	ShutdownWait                = 2 * time.Second
	DebugDuration               = 30 * time.Minute
	ReportRetention             = 30 * 24 * time.Hour
	IncidentDeduplicationWindow = 10 * time.Minute
)

// Config contains only supported user configuration. Resource ceilings and
// privacy policy are not user-configurable. No environment or database loading
// takes place here; a later integration owns that boundary.
type Config struct {
	MinimumLevel Level
	MaxFileBytes int64
}

func DefaultConfig() Config {
	return Config{MinimumLevel: Info, MaxFileBytes: 10 * MiB}
}

// Validate returns fixed messages: an invalid input may itself contain a secret.
func (c Config) Validate() error {
	if _, ok := c.MinimumLevel.SlogLevel(); !ok {
		return errors.New("invalid diagnostic minimum level")
	}
	if c.MaxFileBytes < MiB || c.MaxFileBytes > 100*MiB {
		return errors.New("diagnostic file size must be between 1 and 100 MiB")
	}
	return nil
}

// ResolveConfig uses all defaults if any setting is invalid. The caller may
// emit ConfigFallback once but must never log the rejected input.
func ResolveConfig(c Config) (effective Config, usedDefaults bool) {
	if c.Validate() != nil {
		return DefaultConfig(), true
	}
	return c, false
}
