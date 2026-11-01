package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type backupTransport func(*http.Request) (*http.Response, error)

func (f backupTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type brokenBackupReader struct{}

func (brokenBackupReader) Read([]byte) (int, error) { return 0, errors.New("interrupted") }

func TestSaveDesktopBackup(t *testing.T) {
	for _, scenario := range []string{"save", "cancel", "dialog error", "server error", "interrupted", "invalid destination"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "backup.zip")
			if err := os.WriteFile(path, []byte("previous backup"), 0600); err != nil {
				t.Fatal(err)
			}
			requested := false
			client := &http.Client{Transport: backupTransport(func(r *http.Request) (*http.Response, error) {
				requested = true
				if r.URL.String() != "http://127.0.0.1:8080/desktop-data/export" {
					t.Fatalf("unexpected URL: %s", r.URL)
				}
				status := http.StatusOK
				var body io.Reader = strings.NewReader("complete ZIP payload")
				if scenario == "server error" {
					status = http.StatusInternalServerError
				}
				if scenario == "interrupted" {
					body = io.MultiReader(strings.NewReader("partial"), brokenBackupReader{})
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/zip"}}, Body: io.NopCloser(body)}, nil
			})}
			dialog := func(_ context.Context, options runtime.SaveDialogOptions) (string, error) {
				if !strings.HasSuffix(options.DefaultFilename, ".zip") {
					t.Fatal("missing ZIP extension")
				}
				if scenario == "cancel" {
					return "", nil
				}
				if scenario == "dialog error" {
					return "", errors.New("dialog failed")
				}
				if scenario == "invalid destination" {
					return filepath.Join(dir, "missing", "backup.zip"), nil
				}
				return path, nil
			}
			result, err := saveDesktopBackup(context.Background(), dialog, client)
			wantError := scenario != "save" && scenario != "cancel"
			if (err != nil) != wantError {
				t.Fatalf("unexpected error: %v", err)
			}
			if scenario == "save" && result != path {
				t.Fatalf("unexpected saved path: %q", result)
			}
			if scenario != "save" && result != "" {
				t.Fatalf("unexpected success path: %q", result)
			}
			if (scenario == "cancel" || scenario == "dialog error") && requested {
				t.Fatal("requested backup after cancelling dialog")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			want := "previous backup"
			if scenario == "save" {
				want = "complete ZIP payload"
			}
			if string(data) != want {
				t.Fatalf("unexpected file content: %q", data)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("temporary files left behind: %v, %v", entries, err)
			}
		})
	}
}

func TestSaveReportArchiveUsesFixedLocalEndpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.zip")
	client := &http.Client{Transport: backupTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "127.0.0.1:8080" || r.URL.Path != "/reports/export" || r.URL.Query().Get("mode") != "report" {
			t.Fatalf("unexpected report URL: %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/zip"}}, Body: io.NopCloser(strings.NewReader("zip"))}, nil
	})}
	dialog := func(_ context.Context, options runtime.SaveDialogOptions) (string, error) {
		if !strings.HasPrefix(options.DefaultFilename, "maker-track-report-") {
			t.Fatal(options.DefaultFilename)
		}
		return path, nil
	}
	saved, err := saveDesktopArchive(context.Background(), dialog, client, "Export reports and data", "maker-track-report-", "/reports/export", "mode=report&period=month&date=2026-09-01&categories=print_jobs")
	if err != nil || saved != path {
		t.Fatalf("save: %q %v", saved, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "zip" {
		t.Fatalf("contents: %q %v", data, err)
	}
}

func TestSavePDFReportUsesNativeDialogAndPDFResponse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.pdf")
	client := &http.Client{Transport: backupTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "127.0.0.1:8080" || r.URL.Path != "/reports/export" || r.URL.Query().Get("format") != "pdf" {
			t.Fatalf("unexpected PDF URL: %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/pdf"}}, Body: io.NopCloser(strings.NewReader("%PDF-1.7\n%%EOF"))}, nil
	})}
	dialog := func(_ context.Context, options runtime.SaveDialogOptions) (string, error) {
		if !strings.HasSuffix(options.DefaultFilename, ".pdf") || options.Filters[0].Pattern != "*.pdf" {
			t.Fatalf("PDF dialog: %+v", options)
		}
		return path, nil
	}
	saved, err := saveDesktopFile(context.Background(), dialog, client, "Export PDF report", "maker-track-report-", "/reports/export", "mode=report&format=pdf&period=month&date=2026-09-01&categories=print_jobs", "pdf", "application/pdf")
	if err != nil || saved != path {
		t.Fatalf("save: %q %v", saved, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "%PDF-1.7\n%%EOF" {
		t.Fatalf("PDF content: %q %v", data, err)
	}
}
