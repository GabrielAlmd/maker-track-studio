package diagnostics

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func openTestWriter(t *testing.T, dir string) *RotatingWriter {
	t.Helper()
	w, err := NewRotatingWriter(dir, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

func TestRotationPreservesExactlyThreePreviousFiles(t *testing.T) {
	dir := t.TempDir()
	w := openTestWriter(t, dir)
	w.limit = 120 // Exercise many boundaries without changing public limits.
	for i := 0; i < 30; i++ {
		line := []byte(fmt.Sprintf("{\"sequence\":%d,\"event\":\"app.ready\"}\n", i))
		if err := w.WriteEvent(line); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Sync(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Fatalf("expected 4 logs and lock; got %d", len(entries))
	}
	previous := 30
	for slot := 0; slot <= 3; slot++ {
		name := "application.log"
		if slot > 0 {
			name += fmt.Sprintf(".%d", slot)
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if len(data) > 120 {
			t.Fatal("size limit exceeded")
		}
		lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
		for i := len(lines) - 1; i >= 0; i-- {
			var e struct {
				Sequence int `json:"sequence"`
			}
			if err := json.Unmarshal(lines[i], &e); err != nil {
				t.Fatal(err)
			}
			if e.Sequence != previous-1 {
				t.Fatal("rotation lost or reordered retained records")
			}
			previous = e.Sequence
		}
	}
}

func TestExactSizeBoundaryAndPartialTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "application.log")
	if err := os.WriteFile(path, []byte("{}\n{partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	w := openTestWriter(t, dir)
	w.limit = 6
	if err := w.WriteEvent([]byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	if err := w.Sync(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".1"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("rotated before exceeding boundary")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "{}\n{}\n" {
		t.Fatalf("bad tail recovery: %q", data)
	}
	if err := w.WriteEvent([]byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatal("did not rotate at boundary")
	}
}

func TestRotationFailureBackoffAndRecovery(t *testing.T) {
	dir := t.TempDir()
	w := openTestWriter(t, dir)
	w.limit = 3
	if err := w.WriteEvent([]byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	w.rename = func(string, string) error { return errors.New("sensitive locked path") }
	if err := w.WriteEvent([]byte("{}\n")); !errors.Is(err, ErrSinkUnavailable) {
		t.Fatalf("write: %v", err)
	}
	info, _ := os.Stat(filepath.Join(dir, "application.log"))
	if info.Size() != 3 {
		t.Fatal("failed rotation grew active file")
	}
	if err := w.WriteEvent([]byte("{}\n")); !errors.Is(err, ErrSinkUnavailable) {
		t.Fatal("ignored backoff")
	}
	w.rename = w.root.Rename
	w.retryAt = time.Time{}
	if err := w.WriteEvent([]byte("{}\n")); err != nil {
		t.Fatalf("did not recover: %v", err)
	}
}

func TestExclusiveWriterAndRejectedRecords(t *testing.T) {
	dir := t.TempDir()
	w := openTestWriter(t, dir)
	if second, err := NewRotatingWriter(dir, DefaultConfig()); err == nil {
		second.Close()
		t.Fatal("second writer acquired ownership")
	}
	for _, data := range [][]byte{nil, []byte("{}"), []byte("{}\n{}\n"), bytes.Repeat([]byte("x"), MaxEventBytes+1)} {
		if err := w.WriteEvent(data); err == nil {
			t.Fatal("invalid record accepted")
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := NewRotatingWriter(dir, DefaultConfig())
	if err != nil {
		t.Fatalf("lock not released: %v", err)
	}
	second.Close()
}

func TestSymlinkNotFollowed(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "untouched")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "application.log")); err != nil {
		t.Skip("symlink not available")
	}
	if w, err := NewRotatingWriter(dir, DefaultConfig()); err == nil {
		w.Close()
		t.Fatal("followed symlink")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "original" {
		t.Fatal("modified symlink target")
	}
}

func TestFlushFailureCanRecoverWithoutUnboundedGrowth(t *testing.T) {
	w := openTestWriter(t, t.TempDir())
	if err := w.WriteEvent([]byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	// Simulate a failed filesystem handle after buffered admission.
	if err := w.file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); !errors.Is(err, ErrSinkUnavailable) {
		t.Fatal("flush failure not reported")
	}
	if w.file != nil || w.buffer != nil {
		t.Fatal("failed buffer retained")
	}
	w.retryAt = time.Time{}
	if err := w.WriteEvent([]byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	if err := w.Sync(); err != nil {
		t.Fatal(err)
	}
}
