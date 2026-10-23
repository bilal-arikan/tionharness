package app

import (
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"github.com/bilal-arikan/tionharness/internal/proc"
)

// keepAwake holds an OS sleep inhibitor while any workspace runs work, so an idle
// machine does not sleep in the middle of a turn. Measured 2026-10-23 (WS3
// SES745): a laptop on battery slept for ~15-minute stretches during a PM turn;
// work advanced only in short maintenance wakes, CLI tool calls were aborted
// for "no progress" after each sleep, and the Docker VM clock drifted far
// enough to fail time-based tests. A 49-minute turn took four hours.
//
// macOS uses `caffeinate -i -w <pid>` (prevents idle sleep; -w releases it if
// this process dies), Linux `systemd-inhibit --what=idle:sleep`. Neither can
// keep a closed laptop lid on battery awake — the OS forces that sleep.
const (
	keepAwakePoll      = 15 * time.Second
	keepAwakeIdlePolls = 2 // release only after this many idle polls in a row
)

type keepAwake struct {
	busy   func() bool
	start  func() (*exec.Cmd, error)
	logger *slog.Logger
	stop   chan struct{}
	done   chan struct{}
}

// startKeepAwake begins polling busy; nil when the OS has no inhibitor command.
func startKeepAwake(busy func() bool, logger *slog.Logger) *keepAwake {
	start := inhibitorStarter()
	if start == nil {
		return nil
	}
	k := &keepAwake{busy: busy, start: start, logger: logger, stop: make(chan struct{}), done: make(chan struct{})}
	go k.loop(keepAwakePoll)
	return k
}

// inhibitorStarter returns how to launch this OS's sleep inhibitor, or nil.
func inhibitorStarter() func() (*exec.Cmd, error) {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "caffeinate", []string{"-i", "-w", strconv.Itoa(os.Getpid())}
	case "linux":
		name, args = "systemd-inhibit", []string{"--what=idle:sleep", "--who=TionHarness", "--why=agent work is running", "--mode=block", "sleep", "infinity"}
	default:
		return nil
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return nil
	}
	return func() (*exec.Cmd, error) {
		cmd := proc.Command(path, args...)
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return cmd, nil
	}
}

func (k *keepAwake) loop(every time.Duration) {
	defer close(k.done)
	var held *exec.Cmd
	idle := 0
	release := func() {
		if held != nil {
			_ = held.Process.Kill()
			_ = held.Wait()
			held = nil
			k.logger.Info("keep-awake released")
		}
	}
	defer release()
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		if k.busy() {
			idle = 0
			if held == nil {
				cmd, err := k.start()
				if err != nil {
					k.logger.Warn("keep-awake: inhibitor failed to start", "err", err)
				} else {
					held = cmd
					k.logger.Info("keep-awake held while agent work runs")
				}
			}
		} else if held != nil {
			if idle++; idle >= keepAwakeIdlePolls {
				release()
				idle = 0
			}
		}
		select {
		case <-k.stop:
			return
		case <-ticker.C:
		}
	}
}

// Stop releases the inhibitor and ends the loop.
func (k *keepAwake) Stop() {
	if k == nil {
		return
	}
	close(k.stop)
	<-k.done
}
