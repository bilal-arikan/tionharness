//go:build windows

package proc

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// processAlive reports whether pid still runs, waiting up to d for it to exit.
func processAlive(t *testing.T, pid uint32, d time.Duration) bool {
	t.Helper()
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return false // already gone
	}
	defer windows.CloseHandle(h)
	ev, _ := windows.WaitForSingleObject(h, uint32(d/time.Millisecond))
	return ev == uint32(windows.WAIT_TIMEOUT)
}

func killPid(pid uint32) {
	if h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid); err == nil {
		_ = windows.TerminateProcess(h, 1)
		windows.CloseHandle(h)
	}
}

// startDetachedGrandchild runs a PowerShell parent that launches a long ping,
// prints its pid and exits — the ping is orphaned, out of taskkill /T's reach.
func startDetachedGrandchild(t *testing.T, job *Job) uint32 {
	t.Helper()
	ps, err := exec.LookPath("powershell")
	if err != nil {
		t.Skip("powershell not available")
	}
	cmd := Command(ps, "-NoProfile", "-NonInteractive", "-Command",
		"$p = Start-Process ping -ArgumentList '-n','60','127.0.0.1' -PassThru -WindowStyle Hidden; $p.Id")
	var out strings.Builder
	cmd.Stdout = &out
	if job != nil {
		if err := job.Start(cmd); err != nil {
			t.Fatalf("job start: %v", err)
		}
	} else if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("parent failed: %v (%s)", err, out.String())
	}
	pid, err := strconv.ParseUint(strings.TrimSpace(out.String()), 10, 32)
	if err != nil {
		t.Fatalf("parse grandchild pid from %q: %v", out.String(), err)
	}
	return uint32(pid)
}

// Closing the job must kill a grandchild that outlived its parent; that is the
// containment taskkill /T cannot give.
func TestJobCloseKillsDetachedGrandchild(t *testing.T) {
	job, err := NewJob(JobLimits{ActiveProcesses: 16})
	if err != nil {
		t.Fatalf("NewJob: %v", err)
	}
	pid := startDetachedGrandchild(t, job)
	t.Cleanup(func() { killPid(pid) })
	if !processAlive(t, pid, 0) {
		t.Fatal("grandchild already exited before Close; the test proves nothing")
	}
	if err := job.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if processAlive(t, pid, 5*time.Second) {
		t.Fatal("detached grandchild survived job Close")
	}
}

// Control: without a job the same grandchild survives its parent, so the test
// above measures the job and not an early ping exit.
func TestDetachedGrandchildSurvivesWithoutJob(t *testing.T) {
	pid := startDetachedGrandchild(t, nil)
	defer killPid(pid)
	if !processAlive(t, pid, 0) {
		t.Fatal("grandchild exited on its own; the containment test is not meaningful")
	}
}

// The active-process cap refuses creation beyond the limit inside the tree.
func TestJobActiveProcessLimitBlocksChildren(t *testing.T) {
	job, err := NewJob(JobLimits{ActiveProcesses: 1})
	if err != nil {
		t.Fatalf("NewJob: %v", err)
	}
	defer job.Close()
	cmd := Command("cmd", "/c", "ping -n 1 127.0.0.1 && echo CHILD_RAN")
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := job.Start(cmd); err != nil {
		t.Fatalf("job start: %v", err)
	}
	_ = cmd.Wait()
	if strings.Contains(out.String(), "CHILD_RAN") {
		t.Fatalf("child process started despite ActiveProcesses=1: %s", out.String())
	}
}
