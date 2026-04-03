//go:build windows

package pid

import (
	"syscall"
	"unsafe"
)

var (
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess                = kernel32.NewProc("OpenProcess")
	procGetExitCodeProcess         = kernel32.NewProc("GetExitCodeProcess")
	procCloseHandle                = kernel32.NewProc("CloseHandle")
	processQueryLimitedInformation = uint32(0x1000)
	stillActive                    = uint32(259)
)

// isProcessRunning checks whether a process with the given PID is alive
// on Windows using OpenProcess + GetExitCodeProcess.
func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}

	handle, _, err := procOpenProcess.Call(
		uintptr(processQueryLimitedInformation),
		0,
		uintptr(pid),
	)
	// Note: syscall.LazyProc.Call returns errno as the third value.
	// When the call succeeds, errno is 0, but syscall.Errno(0) is NOT nil
	// as an interface value. So we must check errno == 0 explicitly.
	// handle == 0 means OpenProcess failed.
	if handle == 0 {
		return false
	}
	_ = err // errno is not reliable for error checking here
	defer procCloseHandle.Call(handle)

	var exitCode uint32
	ret, _, _ := procGetExitCodeProcess.Call(handle, uintptr(unsafe.Pointer(&exitCode)))
	// ret == 0 means GetExitCodeProcess failed.
	if ret == 0 {
		return false
	}
	return exitCode == stillActive
}
