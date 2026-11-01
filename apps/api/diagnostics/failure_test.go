package diagnostics

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
)

func TestInitializationFailureRetainsStageAndNumberOnly(t *testing.T) {
	raw := &os.PathError{Op: "open", Path: "secret-customer/password", Err: syscall.Errno(5)}
	err := initializationError("WINDOWS_SECURITY_OPEN", raw)
	if FailureCode(err) != "DIAG_WINDOWS_SECURITY_OPEN_5" {
		t.Fatal("lost stage or numeric OS code")
	}
	if !errors.Is(err, ErrSinkUnavailable) {
		t.Fatal("lost unavailable error classification")
	}
	if errors.Unwrap(err) == raw || strings.Contains(err.Error(), "secret") {
		t.Fatal("retained raw error")
	}
	outer := initializationError("DIRECTORY_PROTECT", err)
	if FailureCode(outer) != FailureCode(err) {
		t.Fatal("lost more precise internal stage")
	}
	if FailureCode(raw) != "DIAG_UNAVAILABLE" {
		t.Fatal("arbitrary raw error exposed")
	}
}
