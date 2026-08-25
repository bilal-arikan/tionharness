//go:build windows

package providers

import (
	"errors"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	stillActive = 259
)

var getProcessTimes = syscall.NewLazyDLL("kernel32.dll").NewProc("GetProcessTimes")

func processIsAlive(pid int) bool {
	handle, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		// Access denied proves the process exists. Other ambiguous failures are also
		// treated as alive so cleanup never deletes a potentially active home.
		return !errors.Is(err, windows.ERROR_INVALID_PARAMETER)
	}
	defer syscall.CloseHandle(handle)
	var exitCode uint32
	if err := syscall.GetExitCodeProcess(handle, &exitCode); err != nil {
		return true
	}
	return exitCode == stillActive
}

func processStartTime(pid int) (time.Time, bool) {
	handle, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return time.Time{}, false
	}
	defer syscall.CloseHandle(handle)
	var creation, exit, kernel, user syscall.Filetime
	result, _, _ := getProcessTimes.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&creation)),
		uintptr(unsafe.Pointer(&exit)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	)
	if result == 0 {
		return time.Time{}, false
	}
	return time.Unix(0, creation.Nanoseconds()), true
}
