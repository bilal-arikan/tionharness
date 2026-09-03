package providers

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/proc"
)

// --- Persistent claude-cli session (Phase 4) ---
//
// The one-shot Complete path spawns a fresh `claude -p` process every turn. Even
// with --resume that re-sends the conversation prefix, so prompt-cache warmth is
// at the mercy of Anthropic's 5-minute TTL and a byte-identical prefix. A PERSISTENT
// process keeps ONE `claude` alive per (session, agent) and feeds turns over stdin
// in stream-json input mode: the process holds the conversation in-memory, so each
// follow-up turn ships only the NEW user message — maximal cache reuse and no
// per-turn process startup, mirroring how External Agent keeps one Claude Code process
// warm. Opt-in (ClaudePersistentSession setting), default off until validated live.
//
// Lifecycle: the appended system prompt + all CLI flags are fixed at process start
// (a fingerprint over them detects staleness — a changed persona/permission mode/
// MCP config restarts the process). The first turn ships the full transcript so a
// freshly-(re)started process gets prior history; subsequent warm turns ship only
// the latest user message. Volatile per-turn context (req.SystemDynamic) always
// rides in the message tail, never the cached system prefix (see buildSystemAndPrompt).

// cliUserInput is one user turn written to the CLI's stdin in --input-format
// stream-json. Matches the shape the CLI's stream-json input reader expects.
type cliUserInput struct {
	Type    string `json:"type"` // always "user"
	Message struct {
		Role    string         `json:"role"` // always "user"
		Content []cliTextBlock `json:"content"`
	} `json:"message"`
}

type cliTextBlock struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

// CLISession is one long-lived `claude` process bound to a single conversation.
// Turns are serialised by mu (the CLI handles one turn at a time). It is created
// and owned by a CLISessionPool.
type CLISession struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr *bytes.Buffer
	// lines is fed by the ONE reader goroutine of this process (startReader). It
	// used to be a goroutine per Turn over the shared bufio.Reader: the previous
	// turn's goroutine stayed blocked in ReadString after delivering its result and
	// consumed the first line(s) of the NEXT turn — buffering one into its dead
	// channel and dropping the next — so when the stolen line was the `result`,
	// the turn waited until the caller's deadline (live, 2026-09-03). A single
	// reader per process makes every line reach whichever Turn is running.
	lines       chan sessionReadItem
	readerOnce  sync.Once
	sysFilePath string // temp --append-system-prompt-file, removed on Close
	fingerprint string // launch config hash; mismatch ⇒ stale ⇒ restart
	model       string
	turns       int
	lastUsed    time.Time
	closed      bool
}

// cliSessionIdleTimeout bounds stdout SILENCE inside one persistent turn. The
// one-shot path deliberately guards only time-to-first-output (later silence is a
// legitimately long tool call), but a persistent process that wedges is worse: it
// holds s.mu, so every later turn for the same (session, agent) key blocks behind
// it forever. The window is therefore generous — a single long build or slow MCP
// fetch must not trip it — and only fires on a genuinely dead stream.
const cliSessionIdleTimeout = 15 * time.Minute

var (
	cliSessionIdleMu sync.RWMutex
	// cliSessionIdleTimeoutDuration is the effective window; <= 0 disables the idle
	// watchdog (the startup guard and ctx cancellation still apply). Tests shrink it.
	cliSessionIdleTimeoutDuration = cliSessionIdleTimeout
)

// SetCLISessionIdleTimeout configures the stdout-silence watchdog for every
// subsequent persistent claude-cli turn. d <= 0 disables it.
func SetCLISessionIdleTimeout(d time.Duration) {
	cliSessionIdleMu.Lock()
	cliSessionIdleTimeoutDuration = d
	cliSessionIdleMu.Unlock()
}

func cliSessionIdleWindow() time.Duration {
	cliSessionIdleMu.RLock()
	defer cliSessionIdleMu.RUnlock()
	return cliSessionIdleTimeoutDuration
}

// sessionReadItem is one line (or the terminal read error) from the persistent
// process's stdout, handed over by the reader goroutine.
type sessionReadItem struct {
	line string
	err  error
}

// Turn writes one user message to the live process and reads its stream-json events
// until the turn's result envelope, returning the parsed Response. prompt is the
// exact text to send (full transcript on a cold start, just the new user message on
// a warm reuse — the pool decides). onEvent, when set, streams each activity step.
// req supplies the per-turn watchdog sink (Request.OnWatchdog).
//
// The read runs in a goroutine so ctx cancellation, a startup hang and stdout
// silence are all observable BETWEEN bytes: bufio.ReadString blocks, so the naive
// loop only noticed cancellation between complete lines and a wedged CLI held the
// session mutex indefinitely (the user's Stop button did nothing). Any watchdog
// firing tears the process down and marks the session closed, so the pool drops it
// instead of handing the next turn a dead pipe.
func (s *CLISession) Turn(ctx context.Context, prompt string, req Request, onEvent func(TraceStep)) (*Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("cli session is closed")
	}

	s.readerOnce.Do(s.startReader)
	// Anything already buffered arrived BEFORE this turn's input is written, so it
	// belongs to the previous turn (a trailing rate_limit_event, a late summary) —
	// or is the stream's EOF. Drop the former, surface the latter.
	for drained := false; !drained; {
		select {
		case it := <-s.lines:
			if it.err != nil {
				return nil, fmt.Errorf("cli session stream already ended: %v %s", it.err, strings.TrimSpace(s.stderr.String()))
			}
		default:
			drained = true
		}
	}

	var in cliUserInput
	in.Type = "user"
	in.Message.Role = "user"
	in.Message.Content = []cliTextBlock{{Type: "text", Text: prompt}}
	line, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("encode user turn: %w", err)
	}
	if _, err := s.stdin.Write(append(line, '\n')); err != nil {
		return nil, fmt.Errorf("write user turn to cli stdin: %w", err)
	}

	p := newCLIParser(s.model, onEvent)
	lines := s.lines

	startupWindow := cliStartupTimeout()
	startup := time.NewTimer(startupWindow)
	defer startup.Stop()
	idleWindow := cliSessionIdleWindow()
	idle := time.NewTimer(idleWindow)
	if !idle.Stop() {
		<-idle.C
	}
	defer idle.Stop()
	sawOutput := false
	for !p.sawResult {
		select {
		case it := <-lines:
			if it.line != "" {
				if !sawOutput {
					sawOutput = true
					startup.Stop() // first output → the turn is alive
				}
				if !idle.Stop() {
					select {
					case <-idle.C:
					default:
					}
				}
				if idleWindow > 0 {
					idle.Reset(idleWindow)
				}
				p.feed(it.line)
			}
			if it.err != nil {
				// Stream ended before a result → the process died mid-turn. Surface a
				// salvage if any content arrived; otherwise an error (the pool drops it).
				if partial := p.salvage(); partial != nil {
					s.turns++
					s.lastUsed = time.Now()
					return partial, nil
				}
				detail := strings.TrimSpace(s.stderr.String())
				// Whatever the dead turn already spent is still billable — carry it out
				// with the failure (see UsageError).
				return nil, p.usageError(fmt.Errorf("cli session stream ended before result: %v %s", it.err, detail))
			}
		case <-startup.C:
			return nil, p.usageError(s.abortTurnLocked(req, WatchdogReasonStartup, startupWindow, fmt.Errorf(
				"cli session produced no output within %s and was killed as a likely hang (the process is dropped; the next turn cold-starts)",
				startupWindow)))
		case <-idle.C:
			return nil, p.usageError(s.abortTurnLocked(req, WatchdogReasonIdle, idleWindow, fmt.Errorf(
				"cli session stdout went silent for %s mid-turn and was killed (the process is dropped; the next turn cold-starts)",
				idleWindow)))
		case <-ctx.Done():
			// The turn was cancelled (human Stop, idle watchdog, hard cap). The
			// blocking read cannot be interrupted, so kill the process: leaving it
			// alive would keep the reader — and s.mu — held for every later turn.
			return nil, p.usageError(s.abortTurnLocked(req, "", 0, ctx.Err()))
		}
	}
	s.turns++
	s.lastUsed = time.Now()
	resp, err := p.finish()
	if err != nil {
		if detail := strings.TrimSpace(s.stderr.String()); detail != "" {
			detail = strings.TrimPrefix(truncateCLIDiagnostic(detail, ""), ": ")
			return nil, fmt.Errorf("%w; CLI stderr: %s", err, detail)
		}
	}
	return resp, err
}

// startReader launches the process's single stdout reader (see CLISession.lines).
// It runs until the stream ends and then parks the terminal error in the channel,
// so a later Turn learns the process is gone instead of blocking on a dead pipe.
// The buffer absorbs lines emitted between turns; a full buffer merely pauses
// reading until the next Turn drains it.
func (s *CLISession) startReader() {
	s.lines = make(chan sessionReadItem, 256)
	go func() {
		for {
			ln, rerr := s.stdout.ReadString('\n')
			s.lines <- sessionReadItem{ln, rerr}
			if rerr != nil {
				return
			}
		}
	}()
}

// killProcessLocked terminates the process tree and reports failure only when the
// process is STILL alive afterwards. On Windows taskkill (proc.KillTree) already
// ends the root, and a second TerminateProcess on the exited handle returns
// "access denied" — a phantom failure this used to propagate as "could NOT be
// killed" (live, 2026-09-03).
func (s *CLISession) killProcessLocked() error {
	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	proc.KillTree(s.cmd) // reap MCP servers / tool subprocesses holding the pipes
	err := s.cmd.Process.Kill()
	if err == nil || errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	if !processIsAlive(s.cmd.Process.Pid) {
		return nil
	}
	return err
}

// abortTurnLocked tears the persistent process down from INSIDE Turn (s.mu is
// already held, so Close/closeChecked would deadlock), marks the session closed so
// the pool drops it, reports the watchdog kill when reason is set, and returns the
// error the caller should propagate. Kill failures are not swallowed: they are
// folded into the returned error, because a process we could not kill still holds
// the pipes this session will never read again.
func (s *CLISession) abortTurnLocked(req Request, reason string, window time.Duration, cause error) error {
	killErr := s.killProcessLocked()
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	s.closed = true
	if s.sysFilePath != "" {
		_ = os.Remove(s.sysFilePath)
	}
	if reason != "" {
		reportWatchdogKill(req, WatchdogKill{
			Provider: "claude-cli",
			Model:    s.model,
			Reason:   reason,
			Window:   window,
			Detail:   strings.TrimSpace(s.stderr.String()),
		})
	}
	if killErr != nil {
		return fmt.Errorf("%w; the CLI process could NOT be killed: %v", cause, killErr)
	}
	return cause
}

// Close terminates the process and removes its system-prompt temp file. Safe to
// call more than once. Best-effort: any kill error is swallowed (use closeChecked
// when a caller must fail closed on a process it could not terminate).
func (s *CLISession) Close() { _ = s.closeChecked() }

// closeChecked is Close but returns a non-nil error when the process could NOT be
// killed, and — crucially — does NOT mark the session closed / does NOT remove its
// temp file in that case, so the process stays tracked and a retry can try again
// instead of being silently orphaned. os.ErrProcessDone (already exited) is success.
// Session delete uses this to fail closed: a live claude-cli that cannot be killed
// must block the delete, never outlive its session. Safe to call more than once.
func (s *CLISession) closeChecked() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	if s.stdin != nil {
		_ = s.stdin.Close() // EOF lets the CLI exit cleanly
	}
	// Reap the descendants (MCP servers, tool subprocesses) first: they inherit
	// the session's pipes and would otherwise survive the kill and keep them
	// open. Then kill the direct child and report a real failure.
	if err := s.killProcessLocked(); err != nil {
		return err // leave closed=false + temp file intact so a retry can re-kill
	}
	s.closed = true
	if s.sysFilePath != "" {
		_ = os.Remove(s.sysFilePath)
	}
	return nil
}

// startPersistent launches a long-lived claude process for one conversation. The
// system prompt (static prefix only) and all flags are fixed here; the caller feeds
// turns via CLISession.Turn. The returned session owns the temp system-prompt file.
func (c *ClaudeCLI) startPersistent(ctx context.Context, req Request) (*CLISession, error) {
	args := []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose"}
	model := req.Model
	if model == "" {
		model = c.model
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, c.permissionArgs(req)...)

	sys, _ := c.buildSystemAndPrompt(req)
	// Delivery mirrors the one-shot Complete path: inline by default, temp file when
	// req.SysPromptFile is set (see claudecli.go). sysPath stays "" in inline mode so
	// Close has nothing to remove.
	var sysPath string
	if sys != "" {
		if req.SysPromptFile {
			f, ferr := os.CreateTemp("", "tionharness-sysprompt-persist-*.txt")
			if ferr != nil {
				return nil, fmt.Errorf("write system prompt file: %w", ferr)
			}
			sysPath = f.Name()
			if _, werr := f.WriteString(sys); werr != nil {
				f.Close()
				os.Remove(sysPath)
				return nil, fmt.Errorf("write system prompt file: %w", werr)
			}
			if cerr := f.Close(); cerr != nil {
				os.Remove(sysPath)
				return nil, fmt.Errorf("write system prompt file: %w", cerr)
			}
			args = append(args, "--append-system-prompt-file", sysPath)
		} else {
			args = append(args, "--append-system-prompt", sys)
		}
	}
	args = append(args, c.mcpArgs()...)
	args = append(args, nativeToolArgs(req)...)

	cmd := proc.CommandContextNested(ctx, c.binPath, args...)
	// Persistent process: its MCP servers and tool subprocesses inherit the pipes,
	// so tear the whole tree down on cancellation instead of orphaning them.
	proc.TreeKill(cmd)
	cmd.Env = cliBaseEnv("ENABLE_TOOL_SEARCH=auto")
	// Thinking parity with the one-shot path: "Kapalı" disables thinking for the
	// whole persistent process (MAX_THINKING_TOKENS=0; restores ≥2.1.203 parallel
	// tool batching — see Request.DisableThinking). Safe to pin at launch: the
	// pool key is per (session, agent), so the agent's level is stable for the
	// process lifetime.
	if req.DisableThinking {
		cmd.Env = append(cmd.Env, "MAX_THINKING_TOKENS=0")
	}
	if req.WorkDir != "" {
		if fi, statErr := os.Stat(req.WorkDir); statErr == nil && fi.IsDir() {
			cmd.Dir = req.WorkDir
		}
	}
	stdinPipe, werr := cmd.StdinPipe()
	if werr != nil {
		if sysPath != "" {
			os.Remove(sysPath)
		}
		return nil, werr
	}
	stdoutPipe, oerr := cmd.StdoutPipe()
	if oerr != nil {
		if sysPath != "" {
			os.Remove(sysPath)
		}
		return nil, oerr
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if serr := cmd.Start(); serr != nil {
		if sysPath != "" {
			os.Remove(sysPath)
		}
		return nil, serr
	}

	return &CLISession{
		cmd:         cmd,
		stdin:       stdinPipe,
		stdout:      bufio.NewReader(stdoutPipe),
		stderr:      &stderr,
		sysFilePath: sysPath,
		fingerprint: c.persistentFingerprint(req, sys),
		model:       model,
	}, nil
}

// persistentFingerprint hashes everything that, if changed, requires restarting the
// process: the appended system prompt and the launch flags (permission mode + MCP
// config). The transcript/dynamic context are NOT included — they vary every turn
// and ride in the message stream, not the process config.
func (c *ClaudeCLI) persistentFingerprint(req Request, sys string) string {
	h := sha256.New()
	model := req.Model
	if model == "" {
		model = c.model
	}
	fmt.Fprintln(h, model)
	fmt.Fprintln(h, req.PermissionMode)
	// Hash the MCP config + settings file CONTENT, not their temp PATHS. Both
	// writeCLIMCPConfig and writeCLISettings write a fresh os.CreateTemp file every
	// turn, so the path churns even when the file is byte-identical — which would flip
	// this fingerprint and cold-restart the persistent process on EVERY turn, defeating
	// warm reuse whenever MCP is enabled (Doc 52 §3-D). Hashing content keeps the
	// process warm across turns whose config did not actually change.
	hashFileContent(h, c.mcpConfigPath)
	hashFileContent(h, c.settingsPath)
	fmt.Fprintln(h, c.permissionPromptTool)
	fmt.Fprintln(h, strings.Join(c.allowedTools, ","))
	fmt.Fprintln(h, strings.Join(c.disallowedTools, ","))
	fmt.Fprintln(h, req.CLIRestrictNativeTools, strings.Join(req.CLINativeTools, ","))
	fmt.Fprintln(h, sys)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// hashFileContent feeds a file's bytes into h so the persistent fingerprint tracks
// config CONTENT rather than its churning temp path. Falls back to the path string
// when the file is missing/unreadable — so an unreadable config still contributes a
// deterministic value instead of silently hashing nothing (which would collapse two
// genuinely different configs into the same fingerprint).
func hashFileContent(h io.Writer, path string) {
	if path == "" {
		return
	}
	if data, err := os.ReadFile(path); err == nil {
		_, _ = h.Write(data)
		return
	}
	fmt.Fprintln(h, path)
}

// CLISessionPool keeps one warm CLISession per conversation key (session id +
// agent id). It is owned by the agent Runtime (one per workspace) and lives across
// turns. Turn routes a completion through the warm process, restarting it on a
// config change or a dead process. EvictIdle reaps sessions unused past a TTL.
type CLISessionPool struct {
	mu       sync.Mutex
	sessions map[string]*CLISession
	// logger surfaces persistent-process lifecycle (cold start + reason, warm reuse,
	// process death, idle evict) in the in-app Logs screen. Nil-safe: without it the
	// pool is silent — which is exactly why a cold turn was once undiagnosable. Set
	// once by the owning Runtime. See _Docs/17.
	logger *slog.Logger
}

// SetLogger wires an optional lifecycle logger. Nil-safe; call once after New.
func (pl *CLISessionPool) SetLogger(l *slog.Logger) { pl.logger = l }

// log emits at the given level when a logger is wired; a no-op otherwise.
func (pl *CLISessionPool) log(level slog.Level, msg string, args ...any) {
	if pl.logger != nil {
		pl.logger.Log(context.Background(), level, msg, args...)
	}
}

// persistentIdleTTL is how long a warm session may sit unused before EvictIdle
// reaps it (and its process). Kept generous so an active back-and-forth never loses
// warmth, but bounded so an abandoned conversation doesn't hold a process forever.
const persistentIdleTTL = 30 * time.Minute

// NewCLISessionPool creates an empty pool.
func NewCLISessionPool() *CLISessionPool {
	return &CLISessionPool{sessions: map[string]*CLISession{}}
}

// Turn runs one completion through the warm process for key, starting (or
// restarting, on a config-fingerprint change) it as needed. c must be the
// already-ConfigureMCP'd provider for this turn. On a COLD (re)start the full
// transcript is sent so the new process has prior history; on a WARM reuse only the
// latest user message is sent (the process remembers the rest). Any process/IO
// error closes the session and propagates so the caller can fall back to one-shot
// Complete.
func (pl *CLISessionPool) Turn(ctx context.Context, key string, c *ClaudeCLI, req Request, onEvent func(TraceStep)) (*Response, error) {
	pl.EvictIdle(persistentIdleTTL) // reap abandoned conversations opportunistically
	sys, fullPrompt := c.buildSystemAndPrompt(req)
	fp := c.persistentFingerprint(req, sys)

	pl.mu.Lock()
	sess := pl.sessions[key]
	cold := sess == nil || sess.closed || sess.fingerprint != fp
	if cold {
		// Record WHY this turn is cold (writes the full cached prefix again) so a
		// surprise cold turn — the exact symptom that was once undiagnosable — is
		// visible in Logs: a config change (persona/permission/MCP) or a dead process.
		reason := "new-session"
		if sess != nil {
			if sess.closed {
				reason = "dead-process"
			} else {
				reason = "config-change" // fingerprint (system prompt / flags / MCP) drifted
			}
			sess.Close()
		}
		var err error
		sess, err = c.startPersistent(ctx, req)
		if err != nil {
			pl.mu.Unlock()
			pl.log(slog.LevelWarn, "cli persistent session cold start failed", "key", key, "reason", reason, "error", err)
			return nil, err
		}
		pl.sessions[key] = sess
		pl.log(slog.LevelInfo, "cli persistent session cold start (full prefix re-sent)", "key", key, "reason", reason)
	} else {
		pl.log(slog.LevelDebug, "cli persistent session warm reuse (delta only)", "key", key, "turns", sess.turns)
	}
	pl.mu.Unlock()

	// Cold start ships the whole transcript; a warm reuse ships only the new turn.
	prompt := fullPrompt
	if !cold {
		// Summary rides the uncached tail (fresh each turn) — see buildSystemAndPrompt.
		prompt = withDynamic(lastUserText(req.Messages), joinNonEmpty(req.SystemDynamic, req.Summary))
	}

	resp, err := sess.Turn(ctx, prompt, req, onEvent)
	if err != nil {
		pl.mu.Lock()
		if pl.sessions[key] == sess {
			delete(pl.sessions, key)
		}
		pl.mu.Unlock()
		sess.Close()
		// The process died mid-turn (stream ended before a result, or a write failed).
		// The caller falls back to one-shot Complete; the NEXT turn will cold-restart.
		pl.log(slog.LevelWarn, "cli persistent session turn failed (process dropped)", "key", key, "error", err)
		return nil, err
	}
	return resp, nil
}

// EvictIdle closes and drops every session unused for longer than ttl. Called
// periodically by the Runtime so abandoned conversations don't hold processes.
func (pl *CLISessionPool) EvictIdle(ttl time.Duration) {
	cutoff := time.Now().Add(-ttl)
	pl.mu.Lock()
	var dead []*CLISession
	for k, s := range pl.sessions {
		s.mu.Lock()
		idle := s.lastUsed.Before(cutoff) && s.turns > 0
		s.mu.Unlock()
		if idle {
			dead = append(dead, s)
			delete(pl.sessions, k)
		}
	}
	pl.mu.Unlock()
	for _, s := range dead {
		s.Close()
	}
	if len(dead) > 0 {
		pl.log(slog.LevelDebug, "cli persistent sessions evicted (idle)", "count", len(dead))
	}
}

// Drop closes and removes the session for a key (e.g. when the conversation ends).
func (pl *CLISessionPool) Drop(key string) {
	pl.mu.Lock()
	s := pl.sessions[key]
	delete(pl.sessions, key)
	pl.mu.Unlock()
	if s != nil {
		s.Close()
	}
}

// keyPrefixForSession is the pool-key prefix for all warm sessions belonging to a
// chat session, regardless of responding agent. The pool key is "<sessionID>|<agentID>"
// (see toolloop.go), so a session's warm processes are exactly the keys with this
// prefix. Kept as a helper so the match rule lives in one place.
func keyPrefixForSession(sessionID string) string { return sessionID + "|" }

// AliveForSession reports whether the session has at least one warm (open) CLI
// process kept alive between turns (persistent-pool mode). Used by the Session Info
// panel to surface a "hot process" the user can drop.
func (pl *CLISessionPool) AliveForSession(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	prefix := keyPrefixForSession(sessionID)
	pl.mu.Lock()
	defer pl.mu.Unlock()
	for k, s := range pl.sessions {
		if strings.HasPrefix(k, prefix) && s != nil && !s.closed {
			return true
		}
	}
	return false
}

// DropSession closes and removes every warm session for a chat session (all agents),
// so the NEXT turn cold-restarts with a fresh process. Returns how many were dropped.
// The conversation itself is untouched — only the warm process is recycled.
func (pl *CLISessionPool) DropSession(sessionID string) int {
	if sessionID == "" {
		return 0
	}
	prefix := keyPrefixForSession(sessionID)
	pl.mu.Lock()
	var dead []*CLISession
	for k, s := range pl.sessions {
		if strings.HasPrefix(k, prefix) {
			dead = append(dead, s)
			delete(pl.sessions, k)
		}
	}
	pl.mu.Unlock()
	for _, s := range dead {
		if s != nil {
			s.Close()
		}
	}
	if len(dead) > 0 {
		pl.log(slog.LevelInfo, "cli persistent sessions dropped (user restart)", "session", sessionID, "count", len(dead))
	}
	return len(dead)
}

// DropSessionChecked is DropSession but VERIFIES every kill: a process it could not
// terminate is kept in the pool (still tracked, not orphaned) and reported in the
// returned error, alongside the count actually dropped. Session delete uses this to
// fail closed — a warm claude-cli that survives the kill must block the delete rather
// than outlive its session as a zombie.
func (pl *CLISessionPool) DropSessionChecked(sessionID string) (int, error) {
	if sessionID == "" {
		return 0, nil
	}
	prefix := keyPrefixForSession(sessionID)
	type keyed struct {
		k string
		s *CLISession
	}
	pl.mu.Lock()
	var targets []keyed
	for k, s := range pl.sessions {
		if strings.HasPrefix(k, prefix) {
			targets = append(targets, keyed{k, s})
		}
	}
	pl.mu.Unlock()
	dropped := 0
	var errs []error
	for _, t := range targets {
		if t.s != nil {
			if err := t.s.closeChecked(); err != nil {
				errs = append(errs, fmt.Errorf("cli %s: %w", t.k, err))
				continue // leave it tracked in the pool; do not orphan it
			}
		}
		pl.mu.Lock()
		delete(pl.sessions, t.k)
		pl.mu.Unlock()
		dropped++
	}
	if dropped > 0 {
		pl.log(slog.LevelInfo, "cli persistent sessions dropped (session delete)", "session", sessionID, "count", dropped)
	}
	return dropped, errors.Join(errs...)
}

// Close terminates every live session. Called on workspace/runtime teardown.
func (pl *CLISessionPool) Close() {
	pl.mu.Lock()
	all := pl.sessions
	pl.sessions = map[string]*CLISession{}
	pl.mu.Unlock()
	for _, s := range all {
		s.Close()
	}
}

// lastUserText returns the text of the most recent user message (the new turn).
func lastUserText(msgs []Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == RoleUser {
			return msgs[i].Text
		}
	}
	return ""
}

// withDynamic prepends the volatile per-turn context as a delimited [Context] block
// so it rides in the uncached message tail. Shared by buildSystemAndPrompt and the
// persistent warm path.
func withDynamic(body, dyn string) string {
	body = strings.TrimSpace(body)
	if d := strings.TrimSpace(dyn); d != "" {
		return strings.TrimSpace("[Context]\n" + d + "\n[/Context]\n\n" + body)
	}
	return body
}
