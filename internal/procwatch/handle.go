package procwatch

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"time"
)

// OutputTailBytes bounds the stdout+stderr tail kept per entry. It is a
// diagnostic ("what was it printing when it hung"), not a log: the full output
// belongs to whoever ran the process. Exported so a site that keeps its own
// larger buffer can hand over exactly the slice that will be retained.
const OutputTailBytes = 4 * 1024

// Handle is the live side of one tracked process. The spawn site holds it
// between starting and reaping the process:
//
//	h := reg.Begin(runCtx, procwatch.Meta{...})
//	defer h.Finish(err)   // or h.Finish(nil) on the success path
//	cmd.Start(); h.Started(cmd)
//
// Every method tolerates a nil receiver, so a call site instrumented against a
// registry that does not exist in this build behaves exactly as before.
type Handle struct {
	reg *Registry
	ctx context.Context

	mu       sync.Mutex
	e        Entry
	stop     func()
	stopping bool
	finished bool
	tail     []byte
}

// Started records the OS pid once the process is running. Passing a cmd that
// never started (Start returned an error) is fine — it records no pid.
func (h *Handle) Started(cmd *exec.Cmd) {
	if h == nil || cmd == nil || cmd.Process == nil {
		return
	}
	h.mu.Lock()
	h.e.PID = cmd.Process.Pid
	h.mu.Unlock()
	h.reg.emit(h.snapshot())
}

// AppendOutput adds to the retained output tail, keeping the last
// OutputTailBytes. Sites that already buffer combined output call it; the
// others leave the tail empty. It does NOT emit: output arrives byte by byte
// and a notify per chunk would flood the event bus.
func (h *Handle) AppendOutput(s string) {
	if h == nil || s == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.tail = append(h.tail, s...)
	if len(h.tail) > OutputTailBytes {
		h.tail = h.tail[len(h.tail)-OutputTailBytes:]
	}
}

// Finish assigns the terminal status and moves the entry into history. err is
// the error from cmd.Run/cmd.Wait (nil on success). Only the first call counts;
// a second one is a no-op, so `defer h.Finish(err)` is safe next to an explicit
// early Finish on a failure path.
//
// Classification, in order:
//   - a stop was requested   → killed
//   - the run context expired → timed out
//   - err is an ExitError    → failed with that exit code
//   - any other err          → failed, exit code -1 (it never ran)
//   - no err                 → succeeded
func (h *Handle) Finish(err error) {
	if h == nil {
		return
	}
	code := 0
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		code = exitErr.ExitCode()
	default:
		code = -1
	}
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	h.finishWith(err != nil, code, msg)
}

// FinishCode is the explicit variant for a site that already reduced the
// outcome to an exit code (the background shell manager reaps in its own
// goroutine and keeps the code). A stop request or an expired context still
// wins over the passed code, so a killed process is never reported as a plain
// non-zero exit.
func (h *Handle) FinishCode(code int) {
	if h == nil {
		return
	}
	h.finishWith(code != 0, code, "")
}

// finishWith assigns the terminal status once and hands the entry to history.
func (h *Handle) finishWith(failed bool, code int, msg string) {
	h.mu.Lock()
	if h.finished {
		h.mu.Unlock()
		return
	}
	h.finished = true
	h.e.EndedAt = time.Now().UnixMilli()
	h.e.Stoppable = false
	h.e.OutputTail = string(h.tail)
	h.e.Error = msg
	h.e.ExitCode = code
	switch {
	case h.stopping:
		h.e.Status = StatusKilled
	case h.ctx != nil && errors.Is(h.ctx.Err(), context.DeadlineExceeded):
		h.e.Status = StatusTimedOut
	case failed:
		h.e.Status = StatusFailed
	default:
		h.e.Status, h.e.ExitCode = StatusSucceeded, 0
	}
	h.mu.Unlock()
	h.reg.finish(h)
	h.reg.emit(h.snapshot())
}

// ID returns the ledger id, or "" for a nil handle.
func (h *Handle) ID() string {
	if h == nil {
		return ""
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.e.ID
}

// RequestStop marks the entry as stopping and invokes its cancel func, so the
// process reaches the ledger as "killed" rather than as a plain non-zero exit.
// It reports false when there is nothing to stop (already finished, or no cancel
// func was registered).
//
// A spawn site that offers its OWN stop path (the shell_manage tool killing a
// background shell) routes it through here instead of calling the cancel func
// directly — otherwise the panel would show the same kill as a failure.
func (h *Handle) RequestStop() bool {
	if h == nil {
		return false
	}
	h.mu.Lock()
	if h.finished || h.stop == nil {
		h.mu.Unlock()
		return false
	}
	h.stopping = true
	stop := h.stop
	h.mu.Unlock()
	stop()
	h.reg.emit(h.snapshot())
	return true
}

// MarkStopping records that the process is being terminated ON PURPOSE without
// invoking the registered cancel func — for a site that owns its own teardown
// path (a pool evicting a persistent CLI process, a watchdog killing a wedged
// one). The next Finish then reports "killed" instead of a failure.
//
// Call it BEFORE the kill: a Finish that lands first wins, as it should — the
// process exited on its own in that race.
func (h *Handle) MarkStopping() {
	if h == nil {
		return
	}
	h.mu.Lock()
	if !h.finished {
		h.stopping = true
	}
	h.mu.Unlock()
}

// snapshot copies the entry under the lock, filling the fields that are only
// materialised on read (the live output tail).
func (h *Handle) snapshot() Entry {
	h.mu.Lock()
	defer h.mu.Unlock()
	e := h.e
	if !h.finished {
		e.OutputTail = string(h.tail)
	}
	return e
}
