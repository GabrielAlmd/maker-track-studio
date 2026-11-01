package desktopruntime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/handlers"
	"github.com/gin-gonic/gin"
)

// The sidecar is deliberately never removed: unlinking a locked inode would
// allow another process to lock a replacement file while the owner is alive.
func ownDatabase(path string) (string, *os.File, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", nil, err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return "", nil, err
	}
	path = filepath.Join(parent, filepath.Base(path))
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", nil, err
	}
	lockPath := path + ".owner.lock"
	if info, err := os.Lstat(lockPath); err == nil && !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("database ownership file must be regular")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", nil, err
	}
	owner, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return "", nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = owner.Close()
		}
	}()
	if err := protectDatabaseFile(owner); err != nil {
		return "", nil, err
	}
	if err := lockDatabaseOwner(owner); err != nil {
		return "", nil, fmt.Errorf("database already owned or ownership lock unavailable: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return "", nil, err
	}
	err = protectDatabaseFile(file)
	closeErr := file.Close()
	if err != nil {
		return "", nil, err
	}
	if closeErr != nil {
		return "", nil, closeErr
	}
	ok = true
	return path, owner, nil
}

func (r *Runtime) acquire(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.stopped.Done():
		return errors.New("workspace stopping")
	case r.gate <- struct{}{}:
		if r.stopped.Err() != nil {
			<-r.gate
			return errors.New("workspace stopping")
		}
		return nil
	}
}

// Serialize complete requests, including reads and reports. Several handlers
// read, calculate, then write across multiple transactions; a connection limit
// alone cannot protect those business operations from interleaving.
func (r *Runtime) workspaceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := r.acquire(c.Request.Context()); err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "Workspace unavailable"})
			return
		}
		defer func() { <-r.gate }()
		c.Set(handlers.RecoveryBackupDirectoryKey, r.recoveryDir)
		c.Next()
	}
}

func (r *Runtime) close() error {
	r.closeOnce.Do(func() {
		r.stop()
		r.workers.Wait()
		r.gate <- struct{}{}
		defer func() { <-r.gate }()
		r.closeErr = closeDatabase(r.db)
		// Do not release ownership before all handlers and SQLite connections stop.
		if err := r.owner.Close(); r.closeErr == nil {
			r.closeErr = err
		}
	})
	return r.closeErr
}
