package desktopruntime

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

func lockDatabaseOwner(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}

func protectDatabaseFile(f *os.File) error {
	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		return err
	}
	if st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || st.Mode&unix.S_IFMT != unix.S_IFREG {
		return fmt.Errorf("database files must be regular, singly linked and owned by the application user")
	}
	return f.Chmod(0o600)
}
