package codexauth

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/proc"
)

// State is the lifecycle of a device-auth attempt.
type State string

const (
	StatePending   State = "pending"   // process running, waiting for the user to approve in the browser
	StateSuccess   State = "success"   // process exited 0 AND auth.json now exists
	StateFailed    State = "failed"    // process exited non-zero, or exited 0 but auth.json never appeared
	StateExpired   State = "expired"   // the 15-minute code lifetime elapsed before approval
	StateCancelled State = "cancelled" // the caller cancelled before a terminal state was reached
)

// codeLifetime bounds how long a started flow is left running before it is
// killed and reported expired — the CLI states the code itself expires in 15
// minutes, so there is no point holding the subprocess open past that.
const codeLifetime = 15 * time.Minute

// authFileName is the credential file codex writes into CODEX_HOME on a
// successful device-auth login. Only used to detect success; its contents are
// never read or interpreted here — codex owns that file exclusively.
const authFileName = "auth.json"

// promptWait bounds how long Start waits for the CLI to print the
// verification URL + code before giving up.
const promptWait = 30 * time.Second

// cmdRunner abstracts subprocess start/output/wait so tests can inject a
// fake binary instead of shelling out to real codex. The default is
// realCmdRunner, backed by os/exec.
type cmdRunner interface {
	// Start launches the command and returns a reader positioned at the
	// beginning of its combined stdout+stderr, plus a wait function that
	// blocks until exit and returns the process error (nil on exit 0).
	Start(ctx context.Context, binPath string, env []string, args ...string) (stdout io.ReadCloser, wait func() error, kill func(), err error)
}

type realCmdRunner struct{}

func (realCmdRunner) Start(ctx context.Context, binPath string, env []string, args ...string) (io.ReadCloser, func() error, func(), error) {
	cmd := proc.CommandContext(ctx, binPath, args...)
	cmd.Env = env
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw
	if err := cmd.Start(); err != nil {
		pw.Close()
		return nil, nil, nil, err
	}
	wait := func() error {
		err := cmd.Wait()
		pw.Close()
		return err
	}
	kill := func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
	return pr, wait, kill, nil
}

// Flow is one in-flight (or completed) device-auth attempt against a single
// CODEX_HOME.
type Flow struct {
	homeDir   string
	verifyURL string
	code      string

	mu       sync.Mutex
	state    State
	errMsg   string
	kill     func()
	waitDone chan struct{}
}

// State returns the flow's current lifecycle state and, when Failed, an
// error message.
func (f *Flow) State() (State, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state, f.errMsg
}

// VerifyURL and Code return the prompt values captured at Start.
func (f *Flow) VerifyURL() string { return f.verifyURL }
func (f *Flow) Code() string      { return f.code }

// Cancel kills the subprocess if still running. Per the verified behaviour of
// `codex login --device-auth`, killing before approval leaves no auth.json,
// so this is always safe and never corrupts an existing login. Cancel is a
// no-op once the flow has already reached a terminal state.
func (f *Flow) Cancel() {
	f.mu.Lock()
	if f.state != StatePending {
		f.mu.Unlock()
		return
	}
	f.state = StateCancelled
	kill := f.kill
	f.mu.Unlock()
	if kill != nil {
		kill()
	}
}

func (f *Flow) setTerminal(s State, errMsg string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state != StatePending {
		return // already terminal (e.g. Cancel raced the exit) — first write wins
	}
	f.state = s
	f.errMsg = errMsg
}

// Manager tracks at most one in-flight Flow per CODEX_HOME, so a second start
// against the same home is refused rather than silently queued or run
// concurrently — concurrent auth against one home is exactly what can rotate
// its refresh token into an invalid state.
type Manager struct {
	runner cmdRunner

	mu    sync.Mutex
	flows map[string]*Flow // keyed by absolute CODEX_HOME path
}

// NewManager creates a Manager backed by the real codex binary via os/exec.
func NewManager() *Manager {
	return &Manager{runner: realCmdRunner{}}
}

// newManagerWithRunner is the test seam: build a Manager over an injected
// cmdRunner so tests never shell out to real codex.
func newManagerWithRunner(r cmdRunner) *Manager {
	return &Manager{runner: r}
}

// Start begins a device-auth flow for homeDir using binPath as the codex
// executable. It blocks until the CLI prints its verification prompt (or
// promptWait elapses) and then returns immediately while the subprocess keeps
// running in the background, waiting for the user's browser approval.
//
// Returns an error if homeDir already has a flow in progress, if binPath is
// empty, or if the prompt could not be started/parsed.
func (m *Manager) Start(ctx context.Context, binPath, homeDir string) (*Flow, error) {
	if binPath == "" {
		return nil, fmt.Errorf("codex binary not found")
	}
	if homeDir == "" {
		return nil, fmt.Errorf("empty codex home dir")
	}
	key := filepath.Clean(homeDir)

	m.mu.Lock()
	if existing, ok := m.flows[key]; ok {
		if state, _ := existing.State(); state == StatePending {
			m.mu.Unlock()
			return nil, fmt.Errorf("a device-auth flow is already in progress for this codex home")
		}
	}
	m.mu.Unlock()

	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		return nil, fmt.Errorf("create codex home: %w", err)
	}

	env := append(os.Environ(), "CODEX_HOME="+homeDir)
	// The subprocess MUST outlive ctx: ctx only bounds how long we wait for the
	// prompt, while the login itself keeps running in the background until the
	// user approves in their browser. Binding the process to ctx killed it the
	// moment the HTTP handler returned, surfacing as "exit status 1" seconds
	// after the code was displayed. Cancellation before the prompt is still
	// honoured explicitly via kill() below.
	stdout, wait, kill, err := m.runner.Start(context.WithoutCancel(ctx), binPath, env, "login", "--device-auth")
	if err != nil {
		return nil, fmt.Errorf("start codex login: %w", err)
	}

	promptCh := make(chan struct{ url, code string }, 1)
	failCh := make(chan error, 1)
	go scanForPrompt(stdout, promptCh, failCh)

	var url, code string
	select {
	case p := <-promptCh:
		url, code = p.url, p.code
	case err := <-failCh:
		kill()
		return nil, fmt.Errorf("codex login produced no recognisable prompt: %w", err)
	case <-time.After(promptWait):
		kill()
		return nil, fmt.Errorf("timed out waiting for codex login prompt")
	case <-ctx.Done():
		kill()
		return nil, ctx.Err()
	}

	flow := &Flow{
		homeDir:   homeDir,
		verifyURL: url,
		code:      code,
		state:     StatePending,
		kill:      kill,
		waitDone:  make(chan struct{}),
	}

	m.mu.Lock()
	if m.flows == nil {
		m.flows = map[string]*Flow{}
	}
	m.flows[key] = flow
	m.mu.Unlock()

	go m.finish(flow, homeDir, wait)
	go expireAfter(flow, codeLifetime)

	return flow, nil
}

// finish waits for the subprocess to exit and classifies the terminal state:
// success requires BOTH a zero exit AND auth.json now present — a zero exit
// alone is not proof the CLI actually completed the login.
func (m *Manager) finish(flow *Flow, homeDir string, wait func() error) {
	err := wait()
	close(flow.waitDone)
	if state, _ := flow.State(); state != StatePending {
		return // already Cancelled or Expired — do not overwrite
	}
	if err != nil {
		flow.setTerminal(StateFailed, err.Error())
		return
	}
	if _, statErr := os.Stat(filepath.Join(homeDir, authFileName)); statErr != nil {
		flow.setTerminal(StateFailed, "codex exited without writing auth.json")
		return
	}
	flow.setTerminal(StateSuccess, "")
}

// expireAfter kills flow and marks it Expired if it is still pending once d
// has elapsed, bounding how long an unapproved subprocess is kept alive.
func expireAfter(flow *Flow, d time.Duration) {
	select {
	case <-time.After(d):
		flow.mu.Lock()
		pending := flow.state == StatePending
		kill := flow.kill
		if pending {
			flow.state = StateExpired
		}
		flow.mu.Unlock()
		if pending && kill != nil {
			kill()
		}
	case <-flow.waitDone:
	}
}

// scanForPrompt reads r line-by-line, accumulating output until
// ParseDevicePrompt recognises the verification prompt, then sends it on
// promptCh. If r is closed (process exited) before a prompt is recognised, it
// sends the accumulated output as an error on failCh.
func scanForPrompt(r io.ReadCloser, promptCh chan<- struct{ url, code string }, failCh chan<- error) {
	scanner := bufio.NewScanner(r)
	var buf strings.Builder
	for scanner.Scan() {
		buf.WriteString(scanner.Text())
		buf.WriteString("\n")
		if url, code, ok := ParseDevicePrompt(buf.String()); ok {
			promptCh <- struct{ url, code string }{url, code}
			// Keep draining stdout in the background so the subprocess never
			// blocks on a full pipe buffer.
			go io.Copy(io.Discard, r)
			return
		}
	}
	failCh <- fmt.Errorf("no prompt before output ended: %s", strings.TrimSpace(buf.String()))
}

// Flow returns the tracked flow for homeDir, if any.
func (m *Manager) Flow(homeDir string) (*Flow, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.flows[filepath.Clean(homeDir)]
	return f, ok
}

// LoginWithAPIKey runs `codex login --with-api-key`, piping key over stdin so
// it never appears as an argv element (which would leak it into process
// listings). Blocks until the command exits; returns nil on success.
func LoginWithAPIKey(ctx context.Context, binPath, homeDir, key string) error {
	if binPath == "" {
		return fmt.Errorf("codex binary not found")
	}
	if homeDir == "" {
		return fmt.Errorf("empty codex home dir")
	}
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("empty api key")
	}
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		return fmt.Errorf("create codex home: %w", err)
	}
	cmd := proc.CommandContext(ctx, binPath, "login", "--with-api-key")
	cmd.Env = append(os.Environ(), "CODEX_HOME="+homeDir)
	cmd.Stdin = strings.NewReader(key)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("codex login --with-api-key failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
