package diagnostics

import (
	"log/slog"
	"strings"
	"testing"
)

func TestLevelContract(t *testing.T) {
	for level, want := range map[Level]slog.Level{
		Debug: -4, Info: 0, Warning: 4, Error: 8, Critical: 12,
	} {
		got, ok := level.SlogLevel()
		if !ok || got != want {
			t.Errorf("level %s: got %d, %v; want %d", level, got, ok, want)
		}
	}
	if _, ok := Level("WARN").SlogLevel(); ok {
		t.Fatal("non-contract level accepted")
	}
}

func TestCatalogRejectsArbitraryMessagesAndReturnsCopies(t *testing.T) {
	if _, ok := Lookup(EventID("secret-token")); ok {
		t.Fatal("unknown event accepted")
	}
	d, ok := Lookup(BackupExportFailed)
	if !ok {
		t.Fatal("missing backup failure event")
	}
	d.Message = "sensitive user input"
	d2, _ := Lookup(BackupExportFailed)
	if d2.Message == d.Message {
		t.Fatal("caller mutated shared catalog")
	}
	for id, entry := range catalog {
		if _, ok := entry.Level.SlogLevel(); !ok || entry.Component == "" || entry.Operation == "" || entry.Message == "" {
			t.Errorf("invalid definition: %s", id)
		}
	}
	if ErrorCode("secret-token").Valid() || !CodeUnknown.Valid() {
		t.Fatal("invalid error code contract")
	}
}

func TestConfigBoundsAndSafeFallback(t *testing.T) {
	for _, size := range []int64{MiB, 10 * MiB, 100 * MiB} {
		input := Config{MinimumLevel: Info, MaxFileBytes: size}
		got, fallback := ResolveConfig(input)
		if fallback || got != input {
			t.Errorf("valid size rejected: %d", size)
		}
	}
	for _, input := range []Config{
		{}, {Info, MiB - 1}, {Info, 100*MiB + 1}, {Level("secret-token"), 10 * MiB},
	} {
		err := input.Validate()
		if err == nil || strings.Contains(err.Error(), "secret-token") {
			t.Fatal("invalid configuration not safely rejected")
		}
		got, fallback := ResolveConfig(input)
		if !fallback || got != DefaultConfig() {
			t.Fatal("invalid configuration must use complete defaults")
		}
	}
}
