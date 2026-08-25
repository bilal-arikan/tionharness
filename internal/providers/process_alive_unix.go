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
