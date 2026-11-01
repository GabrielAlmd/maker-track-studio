package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/diagnostics"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ExportDesktopData asks for a destination in the native desktop dialog.
// An empty path means the user cancelled; success is returned only after saving.
func (a *App) ExportDesktopData() (string, error) {
	started := time.Now()
	a.emit(diagnostics.Input{Event: diagnostics.BackupExportStarted})
	path, err := saveDesktopBackup(a.ctx, runtime.SaveFileDialog, &http.Client{Timeout: 2 * time.Minute})
	event := diagnostics.BackupExportDone
	if err != nil {
		event = diagnostics.BackupExportFailed
	} else if path == "" {
		event = diagnostics.BackupExportCancelled
	}
	input := diagnostics.Input{Event: event, Duration: time.Since(started)}
	if err != nil {
		input.ErrorCode = diagnostics.CodeIO
	}
	a.emit(input)
	return path, err
}

func saveDesktopBackup(ctx context.Context, dialog func(context.Context, runtime.SaveDialogOptions) (string, error), client *http.Client) (string, error) {
	return saveDesktopArchive(ctx, dialog, client, "Export desktop backup", "3d-printing-cost-tracker-backup-", "/desktop-data/export", "")
}

// ExportReport saves a portable CSV ZIP through the native desktop dialog.
func (a *App) ExportReport(query string) (string, error) {
	if len(query) > 2048 {
		return "", errors.New("Report options are too long")
	}
	options, err := url.ParseQuery(query)
	if err != nil {
		return "", errors.New("Invalid report options")
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	if options.Get("format") == "pdf" {
		if options.Get("mode") != "report" {
			return "", errors.New("PDF is only available for period reports")
		}
		return saveDesktopFile(a.ctx, runtime.SaveFileDialog, client, "Export PDF report", "maker-track-report-", "/reports/export", query, "pdf", "application/pdf")
	}
	if options.Get("format") != "" && options.Get("format") != "zip" {
		return "", errors.New("Invalid report format")
	}
	return saveDesktopArchive(a.ctx, runtime.SaveFileDialog, client, "Export reports and data", "maker-track-report-", "/reports/export", query)
}

func saveDesktopArchive(ctx context.Context, dialog func(context.Context, runtime.SaveDialogOptions) (string, error), client *http.Client, title, prefix, endpoint, query string) (string, error) {
	return saveDesktopFile(ctx, dialog, client, title, prefix, endpoint, query, "zip", "application/zip")
}

func saveDesktopFile(ctx context.Context, dialog func(context.Context, runtime.SaveDialogOptions) (string, error), client *http.Client, title, prefix, endpoint, query, extension, mimeType string) (string, error) {
	kind := "report"
	if endpoint == "/desktop-data/export" {
		kind = "backup"
	}
	path, err := dialog(ctx, runtime.SaveDialogOptions{
		Title:           title,
		DefaultFilename: prefix + time.Now().Format("20060102-150405") + "." + extension,
		Filters:         []runtime.FileFilter{{DisplayName: strings.ToUpper(extension) + " " + kind + " (*." + extension + ")", Pattern: "*." + extension}},
	})
	if err != nil {
		return "", errors.New("Could not open the " + kind + " save dialog")
	}
	if path == "" {
		return "", nil
	}

	target := url.URL{Scheme: "http", Host: "127.0.0.1:8080", Path: endpoint, RawQuery: query}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return "", errors.New("Could not prepare the " + kind + " export")
	}
	response, err := client.Do(req)
	if err != nil {
		return "", errors.New("Could not retrieve the " + kind + " from the local application")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != mimeType {
		return "", errors.New("Could not create the " + kind)
	}

	// Stage beside the destination so a failed download leaves existing backups intact.
	file, err := os.CreateTemp(filepath.Dir(path), ".backup-*.tmp")
	if err != nil {
		return "", errors.New("Could not write to the selected folder")
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := io.Copy(file, response.Body); err != nil {
		return "", errors.New("Could not save the complete " + kind)
	}
	if err := file.Sync(); err != nil {
		return "", errors.New("Could not write the backup to disk")
	}
	if err := file.Close(); err != nil {
		return "", errors.New("Could not finish saving the " + kind)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return "", errors.New("Could not save the " + kind + " at the selected destination")
	}
	return path, nil
}
