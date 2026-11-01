package diagnostics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// Execute natively: cross-compilation alone cannot validate Windows ACL APIs.
func TestWindowsPrivateWriterStartupAndReopen(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "Maker Track Studio", "diagnostics", "logs")
	for attempt := 0; attempt < 2; attempt++ {
		writer, err := NewRotatingWriter(directory, DefaultConfig())
		if err != nil {
			t.Fatalf("startup %d: %s", attempt, FailureCode(err))
		}
		if err := writer.WriteEvent([]byte("{\"event\":\"app.starting\"}\n")); err != nil {
			writer.Close()
			t.Fatal(err)
		}
		if err := writer.Sync(); err != nil {
			writer.Close()
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{directory, filepath.Join(directory, "writer.lock"), filepath.Join(directory, "application.log")} {
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		sddl := sd.String()
		// SECURITY_DESCRIPTOR.String canonicalizes well-known user SIDs to SDDL
		// aliases (for example LA for the local administrator). Assert the
		// protected, single-principal shape instead of comparing raw SID text.
		if !strings.HasPrefix(sddl, "D:P") || strings.Count(sddl, "FA;;;") != 1 {
			t.Fatalf("missing protected user DACL: %s", sddl)
		}
		if strings.Contains(sddl, ";;;WD)") || strings.Contains(sddl, ";;;BU)") || strings.Contains(sddl, ";;;AU)") {
			t.Fatal("public access in diagnostic DACL")
		}
	}
	data, err := os.ReadFile(filepath.Join(directory, "application.log"))
	if err != nil || strings.Count(string(data), "app.starting") != 2 {
		t.Fatal("persisted records unavailable")
	}
}
