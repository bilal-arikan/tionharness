package providers

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/proc"
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
	mu          sync.Mutex
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	stdout      *bufio.Reader
	stderr      *bytes.Buffer
	sysFilePath string // temp --append-system-prompt-file, removed on Close
	fingerprint string // launch config hash; mismatch ⇒ stale ⇒ restart
	model       string
	turns       int
	lastUsed    time.Time
	closed      bool
}

// Turn writes one user message to the live process and reads its stream-json events
// until the turn's result envelope, returning the parsed Response. prompt is the
// exact text to send (full transcript on a cold start, just the new user message on
// a warm reuse — the pool decides). onEvent, when set, streams each activity step.
func (s *CLISession) Turn(ctx context.Context, prompt string, onEvent func(TraceStep)) (*Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("cli session is closed")
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
	for !p.sawResult {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		ln, rerr := s.stdout.ReadString('\n')
		if ln != "" {
			p.feed(ln)
		}
		if rerr != nil {
			// Stream ended before a result → the process died mid-turn. Surface a
			// salvage if any content arrived; otherwise an error (the pool drops it).
			if partial := p.salvage(); partial != nil {
				s.turns++
				s.lastUsed = time.Now()
				return partial, nil
			}
			detail := strings.TrimSpace(s.stderr.String())
			return nil, fmt.Errorf("cli session stream ended before result: %v %s", rerr, detail)
		}
	}
	s.turns++
	s.lastUsed = time.Now()
	return p.finish()
}

// Close terminates the process and removes its system-prompt temp file. Safe to
// call more than once.
func (s *CLISession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if s.stdin != nil {
		_ = s.stdin.Close() // EOF lets the CLI exit cleanly
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	if s.sysFilePath != "" {
		_ = os.Remove(s.sysFilePath)
	}
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
			f, ferr := os.CreateTemp("", "tionswarm-sysprompt-persist-*.txt")
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

	cmd := proc.CommandContext(ctx, c.binPath, args...)
	cmd.Env = cliBaseEnv("ENABLE_TOOL_SEARCH=auto")
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
	fmt.Fprintln(h, c.mcpConfigPath)
	fmt.Fprintln(h, c.settingsPath)
	fmt.Fprintln(h, c.permissionPromptTool)
	fmt.Fprintln(h, strings.Join(c.allowedTools, ","))
	fmt.Fprintln(h, strings.Join(c.disallowedTools, ","))
	fmt.Fprintln(h, sys)
	return fmt.Sprintf("%x", h.Sum(nil))
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

	resp, err := sess.Turn(ctx, prompt, onEvent)
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
