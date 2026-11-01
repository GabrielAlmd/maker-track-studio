//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

func systemLocale() string {
	buffer := make([]uint16, 85)
	procedure := syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultLocaleName")
	result, _, _ := procedure.Call(uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	if result == 0 {
		return ""
	}
	return syscall.UTF16ToString(buffer)
}
