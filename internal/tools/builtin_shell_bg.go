package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/swarmgo/internal/providers"
)

const (
	// bgShellRingBytes bounds the rolling output kept per background shell. Older
	// bytes are dropped once this is exceeded (shell_output reports the loss), so a
	// chatty long-running process (a dev server) can't grow memory without bound.
	bgShellRingBytes = 256 * 1024
	// bgShellMaxLive caps concurrent RUNNING background shells per manager — a brake
	// against an agent spawning a fleet of processes it never reaps.
	bgShellMaxLive = 16
	// bgShellKeepDone bounds how many FINISHED shells are retained (for a late
	// shell_output read) before the oldest are pruned.
	bgShellKeepDone = 16
)

// bgWriter is the stdout+stderr sink for a background shell: a byte ring that
// keeps the last bgShellRingBytes and tracks, by absolute byte position, how much
// output has already been handed to shell_output. drain() therefore returns only
// output that is NEW since the previous read, and reports when older output rolled
// off the ring before it could be delivered.
type bgWriter struct {
	mu        sync.Mutex
	buf       []byte
	max       int
	total     int64 // absolute bytes ever written
	delivered int64 // absolute bytes already returned via drain
}

func (w *bgWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	w.total += int64(len(p))
	if len(w.buf) > w.max {
		w.buf = w.buf[len(w.buf)-w.max:]
	}
	return len(p), nil
}

// drain returns the undelivered output and advances the cursor to the latest
// byte. lost is true when some output rolled off the ring before this read (the
// process out-ran the reader), so the caller can warn the model.
func (w *bgWriter) drain() (out string, lost bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	bufStart := w.total - int64(len(w.buf)) // absolute position of buf[0]
	from := w.delivered
	if from < bufStart {
		from = bufStart
		lost = true
	}
	w.delivered = w.total
	if from >= w.total {
		return "", lost
	}
	return string(w.buf[from-bufStart:]), lost
}

// bgProc is one tracked background shell process.
type bgProc struct {
	id        string
	command   string
	shell     string // "Bash" | "PowerShell"
	startedAt time.Time
	w         *bgWriter
	cancel    context.CancelFunc

	mu       sync.Mutex
	done     bool
	exitCode int
}

func (p *bgProc) finish(code int) {
	p.mu.Lock()
	p.done = true
	p.exitCode = code
	p.mu.Unlock()
}

func (p *bgProc) isDone() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.done
}

func (p *bgProc) statusLine() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	age := time.Since(p.startedAt).Round(time.Second)
	if p.done {
		return fmt.Sprintf("[%s %s exited (code %d) after %s]", p.id, p.shell, p.exitCode, age)
	}
	return fmt.Sprintf("[%s %s running, %s elapsed]", p.id, p.shell, age)
}

// ShellManager tracks background shell processes for one session so their output
// can be polled (shell_output) and they can be stopped (shell_kill) across turns.
// A nil *ShellManager is a valid value: Start reports background execution is
// unavailable, which is the behaviour on non-session (catalog/preview) builds.
type ShellManager struct {
	mu    sync.Mutex
	procs map[string]*bgProc
	seq   int
}

// NewShellManager constructs an empty manager.
func NewShellManager() *ShellManager { return &ShellManager{procs: map[string]*bgProc{}} }

// Start launches command in the background through the shell built by build,
// rooted at sb.Root, and returns the assigned shell id. It enforces the live-shell
// cap and prunes finished shells beyond the retention bound.
func (m *ShellManager) Start(sb Sandbox, command, label string, build func(context.Context, string) *exec.Cmd) (string, error) {
	if m == nil {
		return "", fmt.Errorf("background execution is not available in this context")
	}
	m.mu.Lock()
	live := 0
	for _, p := range m.procs {
		if !p.isDone() {
			live++
		}
	}
	if live >= bgShellMaxLive {
		m.mu.Unlock()
		return "", fmt.Errorf("too many background shells running (%d); stop one with shell_kill first", live)
	}
	m.pruneDoneLocked()
	m.seq++
	id := "bg" + strconv.Itoa(m.seq)
	ctx, cancel := context.WithCancel(context.Background())
	p := &bgProc{
		id:        id,
		command:   command,
		shell:     label,
		startedAt: time.Now(),
		w:         &bgWriter{max: bgShellRingBytes},
		cancel:    cancel,
	}
	m.procs[id] = p
	m.mu.Unlock()

	cmd := build(ctx, command)
	cmd.Dir = sb.Root
	cmd.Stdout = p.w
	cmd.Stderr = p.w
	if err := cmd.Start(); err != nil {
		cancel()
		p.finish(-1)
		return "", fmt.Errorf("failed to start background shell: %w", err)
	}
	go func() {
		err := cmd.Wait()
		cancel() // release the context regardless of how it exited
		code := 0
		if err != nil {
			code = 1
			var ee *exec.ExitError
			if ok := asExitError(err, &ee); ok {
				code = ee.ExitCode()
			}
		}
		p.finish(code)
	}()
	return id, nil
}

// asExitError is errors.As specialised to *exec.ExitError, kept local so the file
// needs no extra import churn at call sites.
func asExitError(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	return false
}

// pruneDoneLocked drops the oldest finished shells beyond bgShellKeepDone. Caller
// holds m.mu.
func (m *ShellManager) pruneDoneLocked() {
	var done []*bgProc
	for _, p := range m.procs {
		if p.isDone() {
			done = append(done, p)
		}
	}
	if len(done) <= bgShellKeepDone {
		return
	}
	sort.Slice(done, func(i, j int) bool { return done[i].startedAt.Before(done[j].startedAt) })
	for _, p := range done[:len(done)-bgShellKeepDone] {
		delete(m.procs, p.id)
	}
}

func (m *ShellManager) get(id string) *bgProc {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.procs[id]
}

// Output returns the new output produced by shell id since the previous read,
// prefixed with a status line.
func (m *ShellManager) Output(id string) (string, error) {
	p := m.get(id)
	if p == nil {
		return "", fmt.Errorf("no background shell %q — use shell_list to see running shells", id)
	}
	out, lost := p.w.drain()
	var b strings.Builder
	b.WriteString(p.statusLine())
	if lost {
		b.WriteString("\n[earlier output rolled off the 256KB buffer before it was read]")
	}
	if out == "" {
		b.WriteString("\n(no new output since last read)")
	} else {
		b.WriteString("\n")
		b.WriteString(out)
	}
	return b.String(), nil
}

// Kill cancels shell id (terminating its process) and reports the result.
func (m *ShellManager) Kill(id string) (string, error) {
	p := m.get(id)
	if p == nil {
		return "", fmt.Errorf("no background shell %q — use shell_list to see running shells", id)
	}
	if p.isDone() {
		return fmt.Sprintf("%s already finished (code %d)", id, p.exitCode), nil
	}
	p.cancel()
	return fmt.Sprintf("Signalled %s to stop; poll shell_output to confirm it exited.", id), nil
}

// List returns a one-line-per-shell summary of every tracked shell.
func (m *ShellManager) List() string {
	if m == nil {
		return "(background shells unavailable)"
	}
	m.mu.Lock()
	procs := make([]*bgProc, 0, len(m.procs))
	for _, p := range m.procs {
		procs = append(procs, p)
	}
	m.mu.Unlock()
	if len(procs) == 0 {
		return "(no background shells)"
	}
	sort.Slice(procs, func(i, j int) bool { return procs[i].startedAt.Before(procs[j].startedAt) })
	var b strings.Builder
	for _, p := range procs {
		cmd := p.command
		if len(cmd) > 80 {
			cmd = cmd[:80] + "…"
		}
		fmt.Fprintf(&b, "%s\t%s\n", p.statusLine(), cmd)
	}
	return strings.TrimRight(b.String(), "\n")
}

// --- Tools: shell_output / shell_kill / shell_list ---

// ShellOutputTool reads the accumulated output of a background shell.
type ShellOutputTool struct{ mgr *ShellManager }

// NewShellOutputTool binds the tool to a session's shell manager.
func NewShellOutputTool(m *ShellManager) ShellOutputTool { return ShellOutputTool{mgr: m} }

func (ShellOutputTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "shell_output",
		Description: "Read the NEW output (stdout+stderr) a background shell has produced since the last read, " +
			"plus its running/exited status. Start a background shell by calling Bash or PowerShell with " +
			"run_in_background=true. Poll this until the status line shows the shell exited.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"shell_id":{"type":"string","description":"The id returned when the background shell was started (e.g. bg1)"}},
			"required":["shell_id"],
			"additionalProperties":false
		}`),
	}
}

func (t ShellOutputTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		ShellID string `json:"shell_id"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", argErr(err)
	}
	if strings.TrimSpace(args.ShellID) == "" {
		return "", fmt.Errorf("shell_id is required")
	}
	return t.mgr.Output(args.ShellID)
}

// ShellKillTool terminates a running background shell.
type ShellKillTool struct{ mgr *ShellManager }

// NewShellKillTool binds the tool to a session's shell manager.
func NewShellKillTool(m *ShellManager) ShellKillTool { return ShellKillTool{mgr: m} }

func (ShellKillTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "shell_kill",
		Description: "Terminate a running background shell (started via Bash/PowerShell with run_in_background=true) by its id.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"shell_id":{"type":"string","description":"The id of the background shell to stop (e.g. bg1)"}},
			"required":["shell_id"],
			"additionalProperties":false
		}`),
	}
}

func (t ShellKillTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		ShellID string `json:"shell_id"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", argErr(err)
	}
	if strings.TrimSpace(args.ShellID) == "" {
		return "", fmt.Errorf("shell_id is required")
	}
	return t.mgr.Kill(args.ShellID)
}

// ShellListTool lists all tracked background shells.
type ShellListTool struct{ mgr *ShellManager }

// NewShellListTool binds the tool to a session's shell manager.
func NewShellListTool(m *ShellManager) ShellListTool { return ShellListTool{mgr: m} }

func (ShellListTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "shell_list",
		Description: "List every background shell in this session with its status (running/exited), age and command.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (t ShellListTool) Call(_ context.Context, _ json.RawMessage) (string, error) {
	return t.mgr.List(), nil
}
