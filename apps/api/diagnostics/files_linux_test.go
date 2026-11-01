package diagnostics

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivatePermissionsAndHardlinkRejection(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "diagnostics")
	w := openTestWriter(t, dir)
	for name, want := range map[string]os.FileMode{dir: 0o700, filepath.Join(dir, "application.log"): 0o600, filepath.Join(dir, "writer.lock"): 0o600} {
		info, err := os.Stat(name)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("incorrect private permissions on %s", name)
		}
	}
	w.Close()
	other := t.TempDir()
	target := filepath.Join(other, "target")
	if err := os.WriteFile(target, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(target, filepath.Join(other, "application.log")); err != nil {
		t.Fatal(err)
	}
	if writer, err := NewRotatingWriter(other, DefaultConfig()); err == nil {
		writer.Close()
		t.Fatal("hardlink accepted")
	}
	info, _ := os.Stat(target)
	data, _ := os.ReadFile(target)
	if info.Mode().Perm() != 0o644 || string(data) != "keep" {
		t.Fatal("hardlink target modified")
	}
}
