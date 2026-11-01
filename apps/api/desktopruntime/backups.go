package desktopruntime

import (
	"context"
	"fmt"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/handlers"
)

// BackupConfig enables one daily backup at TimeUTC (HH:MM), in UTC. Scheduling
// is opt-in and never creates a second SQLite-owning process.
type BackupConfig struct {
	Directory string
	TimeUTC   string
	Retention int
	// OnError receives a generic failure indication; never expose backup paths.
	OnError func()
}

func (b BackupConfig) validate() error {
	if b.Directory == "" || b.Retention < 1 || b.Retention > 3650 {
		return fmt.Errorf("backup directory and retention (1..3650) required")
	}
	if _, err := time.Parse("15:04", b.TimeUTC); err != nil || len(b.TimeUTC) != 5 {
		return fmt.Errorf("backup time must be HH:MM UTC")
	}
	return nil
}

func nextBackup(now time.Time, clock string) time.Time {
	parsed, _ := time.Parse("15:04", clock)
	now = now.UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), parsed.Hour(), parsed.Minute(), 0, 0, time.UTC)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func (r *Runtime) startBackups(config BackupConfig) {
	r.workers.Add(1)
	go func() {
		defer r.workers.Done()
		for {
			timer := time.NewTimer(time.Until(nextBackup(time.Now(), config.TimeUTC)))
			select {
			case <-r.stopped.Done():
				timer.Stop()
				return
			case <-timer.C:
				ctx, cancel := context.WithTimeout(r.stopped, 2*time.Minute)
				err := r.Backup(ctx, config.Directory, config.Retention)
				cancel()
				if err != nil && r.stopped.Err() == nil && config.OnError != nil {
					config.OnError()
				}
			}
		}
	}()
}

// Backup takes the same workspace gate as HTTP operations and restore.
func (r *Runtime) Backup(ctx context.Context, directory string, retention int) error {
	if err := r.acquire(ctx); err != nil {
		return err
	}
	defer func() { <-r.gate }()
	_, err := handlers.SaveDesktopDataBackup(r.db.WithContext(ctx), directory, retention)
	return err
}
