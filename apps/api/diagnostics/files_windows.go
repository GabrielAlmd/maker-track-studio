package diagnostics

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func protectFile(f *os.File, directory bool) error {
	h := windows.Handle(f.Fd())
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return initializationError("WINDOWS_FILE_INFO", err)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (!directory && info.NumberOfLinks != 1) {
		return initializationError("WINDOWS_FILE_LINK", nil)
	}
	// os.File handles do not request WRITE_DAC. Open a separate security handle
	// and verify file identity before changing its ACL (including directories).
	name, err := windows.UTF16PtrFromString(f.Name())
	if err != nil {
		return initializationError("WINDOWS_SECURITY_PATH", err)
	}
	securityHandle, err := windows.CreateFile(name, windows.READ_CONTROL|windows.WRITE_DAC|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return initializationError("WINDOWS_SECURITY_OPEN", err)
	}
	defer windows.CloseHandle(securityHandle)
	var securityInfo windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(securityHandle, &securityInfo); err != nil {
		return initializationError("WINDOWS_SECURITY_INFO", err)
	}
	if securityInfo.VolumeSerialNumber != info.VolumeSerialNumber || securityInfo.FileIndexHigh != info.FileIndexHigh ||
		securityInfo.FileIndexLow != info.FileIndexLow || securityInfo.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return initializationError("WINDOWS_SECURITY_IDENTITY", nil)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return initializationError("WINDOWS_TOKEN_USER", err)
	}
	inherit := ""
	if directory {
		inherit = "OICI"
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;" + inherit + ";FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return initializationError("WINDOWS_DESCRIPTOR", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return initializationError("WINDOWS_DACL", err)
	}
	err = windows.SetSecurityInfo(securityHandle, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil)
	if err != nil {
		// LOCALAPPDATA is created with a user-scoped inherited ACL by Windows.
		// Some enterprise policies deny WRITE_DAC to the owning process; retain
		// that OS-provided ACL rather than disabling diagnostics entirely.
		if err == windows.ERROR_ACCESS_DENIED || err == windows.ERROR_PRIVILEGE_NOT_HELD {
			return nil
		}
		return initializationError("WINDOWS_SECURITY_APPLY", err)
	}
	return nil
}

func lockFile(f *os.File) error {
	err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, &windows.Overlapped{})
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return ErrWriterInUse
	}
	return err
}
