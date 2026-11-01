package diagnostics

import (
	"errors"
	"fmt"
	"syscall"
)

// initializationFailure retains only a fixed stage and an OS numeric code.
// Never retain the original error: PathError and other wrappers contain paths.
type initializationFailure struct {
	stage  string
	number syscall.Errno
}

func (e *initializationFailure) Error() string {
	if e.number != 0 {
		return fmt.Sprintf("DIAG_%s_%d", e.stage, e.number)
	}
	return "DIAG_" + e.stage
}

func (*initializationFailure) Unwrap() error { return ErrSinkUnavailable }

func initializationError(stage string, err error) error {
	var existing *initializationFailure
	if errors.As(err, &existing) {
		return existing
	}
	var number syscall.Errno
	_ = errors.As(err, &number)
	return &initializationFailure{stage: stage, number: number}
}

// FailureCode never serializes err.Error() from an arbitrary error.
func FailureCode(err error) string {
	var failure *initializationFailure
	if errors.As(err, &failure) {
		return failure.Error()
	}
	if errors.Is(err, ErrWriterInUse) {
		return "DIAG_WRITER_IN_USE"
	}
	return "DIAG_UNAVAILABLE"
}
