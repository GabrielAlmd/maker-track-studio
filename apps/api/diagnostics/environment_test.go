package diagnostics

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestEnvironmentDefaultsAndInvalidConfiguration(t *testing.T) {
	t.Setenv("DIAGNOSTICS_LEVEL", "INFO")
	t.Setenv("DIAGNOSTICS_MAX_FILE_MIB", "20")
	c, fallback := EnvironmentConfig()
	if fallback || c.MaxFileBytes != 20*MiB {
		t.Fatal("valid settings not used")
	}
	for _, value := range []string{"secret-token", "999999999999999999999", "0", "101"} {
		t.Setenv("DIAGNOSTICS_MAX_FILE_MIB", value)
		c, fallback = EnvironmentConfig()
		if !fallback || c != DefaultConfig() {
			t.Fatal("invalid settings not defaulted")
		}
	}
}

func TestDirectorySelection(t *testing.T) {
	home := t.TempDir()
	state := t.TempDir()
	for _, input := range []string{"", "relative", state} {
		path, err := directoryFor("linux", home, "", input)
		base := filepath.Join(home, ".local", "state")
		if input == state {
			base = state
		}
		if err != nil || path != filepath.Join(base, "3d-printing-cost-tracker", "logs") {
			t.Fatalf("wrong state location: %s", path)
		}
	}
}

func TestDebugOverrideExpires(t *testing.T) {
	c := DefaultConfig()
	c.MinimumLevel = Debug
	l, err := New(c, Identity{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close(context.Background())
	if !l.Emit(Input{Event: RequestCompleted}) {
		t.Fatal("DEBUG not enabled initially")
	}
	l.debugUntil = time.Now().Add(-time.Second)
	if l.Emit(Input{Event: RequestCompleted}) || !l.Emit(Input{Event: AppReady}) {
		t.Fatal("DEBUG did not revert to INFO")
	}
}
