//go:build darwin

package sysinfo

import "time"

// systemTimes: the computer's CPU needs the Mach host API (CGO) on macOS.
func systemTimes() (busy, total time.Duration, ok bool) { return 0, 0, false }
