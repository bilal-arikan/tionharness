//go:build !windows

package providers

import (
	"errors"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bilal-arikan/tionharness/internal/proc"
)

func processIsAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

func processStartTime(pid int) (time.Time, bool) {
	if runtime.GOOS == "darwin" {
		return psStartTime(pid)
	}
	if runtime.GOOS != "linux" {
		return time.Time{}, false
	}
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return time.Time{}, false
	}
	closingParen := strings.LastIndexByte(string(stat), ')')
	if closingParen < 0 {
		return time.Time{}, false
	}
	fields := strings.Fields(string(stat[closingParen+1:]))
	// Fields after the command start at field 3; process start ticks are field 22.
	if len(fields) <= 19 {
		return time.Time{}, false
	}
	startTicks, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	procStat, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Time{}, false
	}
	var bootSeconds int64
	for _, line := range strings.Split(string(procStat), "\n") {
		if strings.HasPrefix(line, "btime ") {
			bootSeconds, err = strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "btime ")), 10, 64)
			break
		}
	}
	if err != nil || bootSeconds == 0 {
		return time.Time{}, false
	}
	// Linux exposes process start ticks in the fixed USER_HZ ABI unit (100 Hz).
	return time.Unix(bootSeconds, startTicks*(int64(time.Second)/100)), true
}

// psStartTime reads a process start time from `ps -o lstart=` (macOS has no
// /proc). lstart has one-second resolution, inside processStartTimeTolerance.
// LC_ALL=C pins the English day/month names the layout below expects.
func psStartTime(pid int) (time.Time, bool) {
	cmd := proc.Command("/bin/ps", "-o", "lstart=", "-p", strconv.Itoa(pid))
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	if err != nil {
		return time.Time{}, false
	}
	return parsePSLstart(string(out))
}

// parsePSLstart parses lstart ("Tue Oct  7 03:23:12 2026"); the day's padding
// space is collapsed first, so one layout covers one- and two-digit days.
func parsePSLstart(s string) (time.Time, bool) {
	t, err := time.ParseInLocation("Mon Jan 2 15:04:05 2006", strings.Join(strings.Fields(s), " "), time.Local)
	return t, err == nil
}
