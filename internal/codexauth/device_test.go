package codexauth

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeRunner is an injectable cmdRunner for tests: it never shells out to a
// real binary. Each Start call is scripted via scriptFn.
type fakeRunner struct {
	scriptFn func() (script fakeScript)
}

// fakeScript describes one fake process's behaviour.
type fakeScript struct {
	prompt    string        // stdout to emit immediately (the device prompt)
	delay     time.Duration // delay before exiting, so tests can observe the pending window
	exitErr   error         // Wait() return value
	writeAuth bool          // whether to write CODEX_HOME/auth.json just before exit
}

func (r *fakeRunner) Start(ctx context.Context, binPath string, env []string, args ...string) (io.ReadCloser, func() error, func(), error) {
	script := r.scriptFn()
	pr, pw := io.Pipe()

	var homeDir string
	for _, e := range env {
		if strings.HasPrefix(e, "CODEX_HOME=") {
			homeDir = strings.TrimPrefix(e, "CODEX_HOME=")
		}
	}

	killed := make(chan struct{})
	var killOnce = make(chan struct{}, 1)
	killOnce <- struct{}{}

	go func() {
		pw.Write([]byte(script.prompt))
		select {
		case <-time.After(script.delay):
		case <-killed:
			pw.Close()
			return
		}
		if script.writeAuth && homeDir != "" {
			_ = os.WriteFile(filepath.Join(homeDir, authFileName), []byte(`{"ok":true}`), 0o600)
		}
		pw.Close()
	}()

	wait := func() error {
		<-time.After(script.delay + 10*time.Millisecond)
		return script.exitErr
	}
	kill := func() {
		select {
		case <-killOnce:
			close(killed)
		default:
		}
	}
	return pr, wait, kill, nil
}

func TestFlowSuccessRequiresAuthFile(t *testing.T) {
	dir := t.TempDir()
	m := newManagerWithRunner(&fakeRunner{scriptFn: func() fakeScript {
		return fakeScript{
			prompt:    devicePromptRaw,
			delay:     20 * time.Millisecond,
			writeAuth: true,
		}
	}})

	flow, err := m.Start(context.Background(), "fake-codex", dir)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if state, _ := flow.State(); state != StatePending {
		t.Fatalf("state right after Start = %v, want pending (process should still be running)", state)
	}
	if flow.VerifyURL() != "https://auth.openai.com/codex/device" || flow.Code() != "Z937-61BWU" {
		t.Fatalf("parsed prompt mismatch: url=%q code=%q", flow.VerifyURL(), flow.Code())
	}

	waitForTerminal(t, flow)
	if state, msg := flow.State(); state != StateSuccess {
		t.Fatalf("final state = %v (%s), want success", state, msg)
	}
}

func TestFlowFailsWhenAuthFileNeverAppears(t *testing.T) {
	dir := t.TempDir()
	m := newManagerWithRunner(&fakeRunner{scriptFn: func() fakeScript {
		return fakeScript{prompt: devicePromptRaw, delay: 10 * time.Millisecond, writeAuth: false}
	}})

	flow, err := m.Start(context.Background(), "fake-codex", dir)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitForTerminal(t, flow)
	if state, _ := flow.State(); state != StateFailed {
		t.Fatalf("final state = %v, want failed (exit 0 but no auth.json)", state)
	}
}

func TestFlowNonZeroExitFails(t *testing.T) {
	dir := t.TempDir()
	m := newManagerWithRunner(&fakeRunner{scriptFn: func() fakeScript {
		return fakeScript{prompt: devicePromptRaw, delay: 5 * time.Millisecond, exitErr: errExit1}
	}})

	flow, err := m.Start(context.Background(), "fake-codex", dir)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitForTerminal(t, flow)
	if state, msg := flow.State(); state != StateFailed || msg == "" {
		t.Fatalf("final state = %v (%q), want failed with a message", state, msg)
	}
}

func TestConcurrentStartOnSameHomeRefused(t *testing.T) {
	dir := t.TempDir()
	m := newManagerWithRunner(&fakeRunner{scriptFn: func() fakeScript {
		return fakeScript{prompt: devicePromptRaw, delay: 200 * time.Millisecond, writeAuth: true}
	}})

	first, err := m.Start(context.Background(), "fake-codex", dir)
	if err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	if _, err := m.Start(context.Background(), "fake-codex", dir); err == nil {
		t.Fatalf("second Start() on same home succeeded, want refusal")
	}
	first.Cancel()
}

func TestCancelBeforeApprovalLeavesNoAuthFile(t *testing.T) {
	dir := t.TempDir()
	m := newManagerWithRunner(&fakeRunner{scriptFn: func() fakeScript {
		return fakeScript{prompt: devicePromptRaw, delay: time.Second, writeAuth: true}
	}})

	flow, err := m.Start(context.Background(), "fake-codex", dir)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	flow.Cancel()
	if state, _ := flow.State(); state != StateCancelled {
		t.Fatalf("state after Cancel = %v, want cancelled", state)
	}
	if _, err := os.Stat(filepath.Join(dir, authFileName)); err == nil {
		t.Fatalf("auth.json exists after cancel, want none")
	}
}

func TestStartEmptyBinPathRejected(t *testing.T) {
	m := NewManager()
	if _, err := m.Start(context.Background(), "", t.TempDir()); err == nil {
		t.Fatalf("Start() with empty binPath succeeded, want error")
	}
}

func waitForTerminal(t *testing.T, flow *Flow) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		if state, _ := flow.State(); state != StatePending {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("flow did not reach a terminal state in time")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// errExit1 stands in for a real *exec.ExitError in tests that only care that
// Wait() returned a non-nil error.
var errExit1 = &fakeExitError{}

type fakeExitError struct{}

func (*fakeExitError) Error() string { return "exit status 1" }
