package diagnostics

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func protectFile(f *os.File, directory bool) error {
	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		return err
	}
	if st.Uid != uint32(os.Geteuid()) || (!directory && st.Nlink != 1) {
		return ErrSinkUnavailable
	}
	if directory {
		return f.Chmod(0o700)
	}
	return f.Chmod(0o600)
}

func lockFile(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return ErrWriterInUse
	}
	return err
}
