//go:build linux

package proc

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// requireCgroupJob skips unless this host gives NewJob a real cgroup leaf.
func requireCgroupJob(t *testing.T, limits bool) *cgroupSetup {
	t.Helper()
	s := probeCgroup()
	if s.parent == "" {
		t.Skipf("cgroup v2 containment unavailable here: %s", s.reason)
	}
	if limits && !s.limits {
		t.Skipf("pids controller unavailable here: %s", s.reason)
	}
	return s
}

func readCgroupFile(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return strings.TrimSpace(string(b))
}

// pidGone reports whether pid has exited (or is a zombie), waiting up to d.
func pidGone(pid int, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for {
		stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if err != nil {
			return true
		}
		// state is the field after "(comm)"
		if i := strings.LastIndexByte(string(stat), ')'); i >= 0 && i+2 < len(stat) && stat[i+2] == 'Z' {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A setsid'd grandchild leaves the process group, so a group kill misses it; the
// cgroup still holds it and Close must kill it and remove the leaf.
func TestJobLinuxCloseKillsSetsidGrandchild(t *testing.T) {
	requireCgroupJob(t, false)
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("setsid not available")
	}
	job, err := NewJob(JobLimits{ActiveProcesses: 16})
	if err != nil {
		t.Fatalf("NewJob: %v", err)
	}
	t.Cleanup(func() { _ = job.Close() })
	if job.dir == "" {
		t.Fatal("NewJob returned a process-group job although the probe succeeded")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "setsid sleep 60 </dev/null >/dev/null 2>&1 &")
	if err := job.Start(cmd); err != nil {
		t.Fatalf("job start: %v", err)
	}
	shellPid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		t.Fatalf("shell failed: %v", err)
	}
	var sleepers []int
	for deadline := time.Now().Add(5 * time.Second); len(sleepers) == 0 && time.Now().Before(deadline); {
		sleepers = parseCgroupProcs(readCgroupFile(t, job.dir, "cgroup.procs"))
		time.Sleep(20 * time.Millisecond)
	}
	if len(sleepers) == 0 {
		t.Fatal("no process left in the job cgroup after the shell exited; the test proves nothing")
	}
	for _, pid := range sleepers {
		t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
		if sid, err := unix.Getsid(pid); err == nil && sid == shellPid {
			t.Fatalf("pid %d did not leave the shell's session; setsid did not take effect", pid)
		}
	}
	dir := job.dir
	if err := job.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	for _, pid := range sleepers {
		if !pidGone(pid, 5*time.Second) {
			t.Fatalf("setsid grandchild %d survived job Close", pid)
		}
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("job cgroup %s not removed by Close: %v", dir, err)
	}
}

// pids.max holds a fork bomb: the tree never exceeds the cap and the kernel
// records refused forks in pids.events.
func TestJobLinuxPidsLimitHoldsForkBomb(t *testing.T) {
	requireCgroupJob(t, true)
	limits := JobLimits{ActiveProcesses: 2} // 32 tasks
	job, err := NewJob(limits)
	if err != nil {
		t.Fatalf("NewJob: %v", err)
	}
	t.Cleanup(func() { _ = job.Close() })
	max, _ := strconv.Atoi(pidsMaxValue(limits))
	if got := readCgroupFile(t, job.dir, "pids.max"); got != strconv.Itoa(max) {
		t.Fatalf("pids.max = %q, want %d", got, max)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c",
		`i=0; while [ $i -lt 300 ]; do sleep 30 & i=$((i+1)); done; wait`)
	if err := job.Start(cmd); err != nil {
		t.Fatalf("job start: %v", err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()

	refused := false
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		cur, err := strconv.Atoi(readCgroupFile(t, job.dir, "pids.current"))
		if err != nil {
			t.Fatalf("pids.current: %v", err)
		}
		if cur > max {
			t.Fatalf("pids.current = %d exceeds pids.max %d", cur, max)
		}
		if n := pidsEventsMax(readCgroupFile(t, job.dir, "pids.events")); n > 0 {
			refused = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !refused {
		t.Fatal("no fork was refused although the shell tried to start 300 sleepers under the cap")
	}
	if err := job.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("shell still running after Close")
	}
}

func pidsEventsMax(events string) int {
	for _, line := range strings.Split(events, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "max "); ok {
			n, _ := strconv.Atoi(strings.TrimSpace(v))
			return n
		}
	}
	return 0
}

// Concurrent jobs get distinct leaves and each Close removes its own.
func TestJobLinuxConcurrentJobs(t *testing.T) {
	requireCgroupJob(t, false)
	const n = 8
	var wg sync.WaitGroup
	dirs := make([]string, n)
	errs := make([]error, n)
	for i := range n {
		wg.Go(func() {
			job, err := NewJob(JobLimits{ActiveProcesses: 4})
			if err != nil {
				errs[i] = err
				return
			}
			dirs[i] = job.dir
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "exit 0")
			if err := job.Start(cmd); err != nil {
				errs[i] = err
				_ = job.Close()
				return
			}
			_ = cmd.Wait()
			errs[i] = job.Close()
		})
	}
	wg.Wait()
	seen := map[string]bool{}
	for i := range n {
		if errs[i] != nil {
			t.Fatalf("job %d: %v", i, errs[i])
		}
		if dirs[i] == "" || seen[dirs[i]] {
			t.Fatalf("job %d: missing or duplicate leaf %q", i, dirs[i])
		}
		seen[dirs[i]] = true
		if _, err := os.Stat(dirs[i]); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("leaf %s left behind: %v", dirs[i], err)
		}
	}
}

// Context cancellation kills the whole cgroup, so Wait returns promptly even when
// a setsid'd grandchild holds the output pipe (instead of waiting out WaitDelay).
func TestJobLinuxCancelKillsCgroup(t *testing.T) {
	requireCgroupJob(t, false)
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("setsid not available")
	}
	job, err := NewJob(JobLimits{ActiveProcesses: 16})
	if err != nil {
		t.Fatalf("NewJob: %v", err)
	}
	t.Cleanup(func() { _ = job.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "setsid sleep 60 & sleep 60")
	var out strings.Builder
	cmd.Stdout = &out
	if err := job.Start(cmd); err != nil {
		t.Fatalf("job start: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	start := time.Now()
	cancel()
	_ = cmd.Wait()
	if d := time.Since(start); d >= reapDelay {
		t.Fatalf("Wait took %v after cancel; the setsid grandchild kept the pipe open", d)
	}
	procs := readCgroupFile(t, job.dir, "cgroup.procs")
	for deadline := time.Now().Add(2 * time.Second); procs != "" && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
		procs = readCgroupFile(t, job.dir, "cgroup.procs")
	}
	if procs != "" {
		t.Fatalf("processes left in the job cgroup after cancel: %s", procs)
	}
}

// The status matches what NewJob does on this host.
func TestContainmentStatusConsistent(t *testing.T) {
	s := probeCgroup()
	enforced, reason := ContainmentStatus()
	if enforced != (reason == "") {
		t.Fatalf("enforced=%v with reason %q", enforced, reason)
	}
	job, _ := NewJob(JobLimits{})
	defer job.Close()
	if (job.dir != "") != (s.parent != "") {
		t.Fatalf("job dir %q vs probe parent %q", job.dir, s.parent)
	}
	t.Logf("containment: enforced=%v reason=%q", enforced, reason)
}
