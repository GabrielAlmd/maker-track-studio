package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

var managedBackupName = regexp.MustCompile(`^maker-track-backup-[0-9]{8}T[0-9]{6}\.[0-9]{9}Z-[0-9]+\.zip$`)

// SaveDesktopDataBackup shares the desktop ZIP format and consistent SQLite
// snapshot path. Publish only complete, synced files; prune only after success.
// Callers must serialize access to the destination and the workspace.
func SaveDesktopDataBackup(db *gorm.DB, directory string, retention int) (string, error) {
	if strings.TrimSpace(directory) == "" || retention < 1 {
		return "", fmt.Errorf("backup directory and positive retention required")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	data, _, err := buildDesktopDataArchive(db)
	if err != nil {
		return "", err
	}
	if err := db.Statement.Context.Err(); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(directory, ".maker-track-backup-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := db.Statement.Context.Err(); err != nil {
		return "", err
	}
	suffix := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(file.Name()), ".maker-track-backup-"), ".tmp")
	name := "maker-track-backup-" + time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + suffix + ".zip"
	path := filepath.Join(directory, name)
	if err := os.Rename(file.Name(), path); err != nil {
		return "", err
	}
	if err := syncBackupDirectory(directory); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", err
	}
	var names []string
	for _, entry := range entries {
		if entry.Type().IsRegular() && managedBackupName.MatchString(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for len(names) > retention {
		if err := os.Remove(filepath.Join(directory, names[0])); err != nil {
			return "", err
		}
		names = names[1:]
	}
	if err := syncBackupDirectory(directory); err != nil {
		return "", err
	}
	return path, nil
}

func syncBackupDirectory(path string) error {
	// Windows cannot fsync a directory with os.Open; file.Sync above is required
	// on both platforms. Linux also persists rename/unlink directory entries.
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
