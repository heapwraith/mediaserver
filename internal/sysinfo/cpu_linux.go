//go:build linux

package sysinfo

import (
	"os"
	"time"
)

// USER_HZ: the unit of /proc/stat, 100 on every Linux the server targets.
const userHZ = 100

func systemTimes() (busy, total time.Duration, ok bool) {
	stat, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	return parseProcStat(string(stat), userHZ)
}
