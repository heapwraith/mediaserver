//go:build windows

package sysinfo

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// GetSystemTimes is not wrapped by x/sys: called from kernel32 directly.
var getSystemTimes = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemTimes")

func filetime(f windows.Filetime) time.Duration {
	return time.Duration(int64(f.HighDateTime)<<32|int64(f.LowDateTime)) * 100 // 100 ns units
}

func processTime() time.Duration {
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &creation, &exit, &kernel, &user); err != nil {
		return 0
	}
	return filetime(kernel) + filetime(user)
}

func systemTimes() (busy, total time.Duration, ok bool) {
	var idle, kernel, user windows.Filetime
	// The Win32 call fills three FILETIMEs: the pointers go straight into
	// Call (the uintptr conversion inside the call expression is the pattern
	// the unsafe rules allow), and nothing else touches the memory.
	r, _, _ := getSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user))) //nolint:gosec // G103: audited above
	if r == 0 {
		return 0, 0, false
	}
	// Kernel time includes idle time.
	total = filetime(kernel) + filetime(user)
	return total - filetime(idle), total, true
}
