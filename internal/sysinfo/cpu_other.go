//go:build !linux && !darwin && !windows

package sysinfo

import "time"

func processTime() time.Duration                        { return 0 }
func systemTimes() (busy, total time.Duration, ok bool) { return 0, 0, false }
