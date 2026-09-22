//go:build windows

package tools

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

var trailingPidRe = regexp.MustCompile(`(\d+)\s*$`)

// runDetachedPing runs a PowerShell command that launches a long ping detached
// from itself, prints its pid and exits, returning that pid.
func runDetachedPing(t *testing.T, sb Sandbox) uint32 {
	t.Helper()
	tool := NewPowerShellTool(sb)
	if !tool.Available() {
		t.Skip("no PowerShell host")
	}
	in, _ := json.Marshal(map[string]any{
		"command": "$p = Start-Process ping -ArgumentList '-n','60','127.0.0.1' -PassThru -WindowStyle Hidden; $p.Id",
	})
	out, err := tool.Call(context.Background(), in)
	if err != nil {
		t.Fatalf("shell call: %v", err)
	}
	m := trailingPidRe.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no pid in output %q", out)
	}
	pid, _ := strconv.ParseUint(m[1], 10, 32)
	return uint32(pid)
}

func pidAlive(pid uint32, wait time.Duration) bool {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	ev, _ := windows.WaitForSingleObject(h, uint32(wait/time.Millisecond))
	return ev == uint32(windows.WAIT_TIMEOUT)
}

func terminatePid(pid uint32) {
	if h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid); err == nil {
		_ = windows.TerminateProcess(h, 1)
		windows.CloseHandle(h)
	}
}

// A confined shell call must not leave a detached process running after it
// returns; an unconfined one keeps the historical behaviour.
func TestConfinedShell_KillsDetachedProcessOnReturn(t *testing.T) {
	t.Setenv("TIONHARNESS_ENABLE_SHELL", "1")
	pid := runDetachedPing(t, NewConfinedSandbox(t.TempDir()))
	defer terminatePid(pid)
	if pidAlive(pid, 5*time.Second) {
		t.Fatal("detached process outlived the confined shell call")
	}
}

func TestUnconfinedShell_LeavesDetachedProcessAlone(t *testing.T) {
	t.Setenv("TIONHARNESS_ENABLE_SHELL", "1")
	pid := runDetachedPing(t, NewSandbox(t.TempDir()))
	defer terminatePid(pid)
	if !pidAlive(pid, 0) {
		t.Fatal("unconfined shell call killed its detached process; job containment leaked into interactive mode")
	}
}
