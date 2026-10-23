//go:build !windows

package app

import (
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// TestKeepAwakeHoldsWhileBusy: the inhibitor starts once while work runs, stays
// held across busy polls, and is released after the idle polls and on Stop.
func TestKeepAwakeHoldsWhileBusy(t *testing.T) {
	var busy atomic.Bool
	var mu sync.Mutex
	var started []int // pids; read through kill(pid, 0), never the shared *exec.Cmd
	k := &keepAwake{
		busy: busy.Load,
		start: func() (*exec.Cmd, error) {
			cmd := exec.Command("sleep", "60")
			if err := cmd.Start(); err != nil {
				return nil, err
			}
			mu.Lock()
			started = append(started, cmd.Process.Pid)
			mu.Unlock()
			return cmd, nil
		},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	alive := func(pid int) bool { return syscall.Kill(pid, 0) == nil }
	count := func() int { mu.Lock(); defer mu.Unlock(); return len(started) }

	busy.Store(true)
	go k.loop(10 * time.Millisecond)
	waitFor(t, func() bool { return count() == 1 })
	time.Sleep(50 * time.Millisecond)
	if count() != 1 {
		t.Fatalf("inhibitor started %d times while continuously busy, want 1", count())
	}

	busy.Store(false)
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return !alive(started[0]) })

	busy.Store(true)
	waitFor(t, func() bool { return count() == 2 })
	k.Stop()
	mu.Lock()
	defer mu.Unlock()
	if alive(started[1]) {
		t.Fatal("Stop must release the held inhibitor")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
