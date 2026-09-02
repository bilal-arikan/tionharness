package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/proc"
)

type codexRPCEnvelope struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Method string          `json:"method"`
	Params struct {
		ThreadID string `json:"threadId"`
		Item     struct {
			Type string `json:"type"`
		} `json:"item"`
	} `json:"params"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// codexCompactStepTimeout bounds how long one native-compaction RPC step may
// wait for the app-server. Cancelling ctx already tears the subprocess down, so
// this covers the other case: the client stays connected and codex itself
// wedges. Without it /compact blocks forever and keeps holding the session's
// command-turn lock. Generous, because compacting a long thread is a model call
// — it only has to be shorter than "never". Kept in step with the fold floor the
// rolling paths use (conversation.FoldIdleOutputFloor): both bound the same
// "single request, then silence until the model's first token" shape, and a
// near-full context window can sit in that silence for many minutes.
const codexCompactStepTimeout = 10 * time.Minute

var codexCompactStepTimeoutDuration = codexCompactStepTimeout

type codexRPCLine struct {
	line string
	err  error
}

// codexRPCConn is the line-oriented JSON-RPC transport used by native
// compaction. The read side runs in its own goroutine (the same shape
// runAttempt uses) so every wait can be bounded by a timer — a blocking
// bufio.Scanner read cannot be interrupted otherwise.
type codexRPCConn struct {
	enc   *json.Encoder
	lines <-chan codexRPCLine
	done  chan struct{}
	// stderr is inspected only once the stream ends, to explain WHY it ended.
	stderr *syncBuffer
}

// syncBuffer is a bytes.Buffer safe to read while os/exec is still writing to
// it. Assigning a *bytes.Buffer to cmd.Stderr makes exec copy the pipe in a
// background goroutine that only stops at cmd.Wait(); runAttempt (codexcli.go)
// stays race-free by reading stderr strictly after Wait, but native compaction
// needs the text mid-flight — to explain a stream that ended early — while the
// process is still being waited on in a deferred cleanup. Guarding the buffer is
// the only way to read it there without a data race.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func newCodexRPCConn(w io.Writer, r io.Reader, stderr *syncBuffer) *codexRPCConn {
	lines := make(chan codexRPCLine)
	c := &codexRPCConn{enc: json.NewEncoder(w), lines: lines, done: make(chan struct{}), stderr: stderr}
	scan := bufio.NewScanner(r)
	scan.Buffer(make([]byte, 64*1024), 4*1024*1024)
	go func() {
		defer close(lines)
		for scan.Scan() {
			select {
			case lines <- codexRPCLine{line: scan.Text()}:
			case <-c.done:
				return
			}
		}
		if err := scan.Err(); err != nil {
			select {
			case lines <- codexRPCLine{err: err}:
			case <-c.done:
			}
		}
	}()
	return c
}

// close releases the reader goroutine even when it is parked on a send.
func (c *codexRPCConn) close() { close(c.done) }

func (c *codexRPCConn) send(id int, method string, params any) error {
	return c.enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
}

func (c *codexRPCConn) notify(method string) error {
	return c.enc.Encode(map[string]any{"jsonrpc": "2.0", "method": method})
}

// next returns the next decodable envelope. Undecodable lines are skipped, but
// an ended stream, a read error and a stalled server are all reported as errors:
// no wait on this connection is silent or unbounded. step names the in-flight
// RPC so the message says which one stalled.
func (c *codexRPCConn) next(step string) (codexRPCEnvelope, error) {
	timer := time.NewTimer(codexCompactStepTimeoutDuration)
	defer timer.Stop()
	for {
		select {
		case it, ok := <-c.lines:
			if !ok {
				return codexRPCEnvelope{}, fmt.Errorf("codex app-server exited during %s: %s", step, strings.TrimSpace(c.stderr.String()))
			}
			if it.err != nil {
				return codexRPCEnvelope{}, fmt.Errorf("read codex app-server %s response: %w", step, it.err)
			}
			var envelope codexRPCEnvelope
			if err := json.Unmarshal([]byte(it.line), &envelope); err != nil {
				continue
			}
			return envelope, nil
		case <-timer.C:
			return codexRPCEnvelope{}, fmt.Errorf("codex app-server sent no %s output within %s; native compaction gave up", step, codexCompactStepTimeoutDuration)
		}
	}
}

// request sends one JSON-RPC call and waits for ITS reply. Notifications (no
// id, decoded as 0) and replies to other ids are skipped rather than mistaken
// for the answer.
func (c *codexRPCConn) request(id int, method string, params any) error {
	if err := c.send(id, method, params); err != nil {
		return err
	}
	for {
		envelope, err := c.next(method)
		if err != nil {
			return err
		}
		if envelope.ID != id {
			continue
		}
		if envelope.Error != nil {
			return fmt.Errorf("codex app-server %s failed (%d): %s", method, envelope.Error.Code, envelope.Error.Message)
		}
		return nil
	}
}

// CompactNative invokes Codex App Server's documented thread/compact/start
// request. codex exec has no manual-compaction subcommand; sending "/compact"
// to exec would be an ordinary agent prompt and is deliberately forbidden.
func (c *CodexCLI) CompactNative(ctx context.Context, resumeSessionID string, req Request) (*Response, error) {
	if c.binPath == "" {
		return nil, errors.New("codex CLI: no binary configured")
	}
	if strings.TrimSpace(req.CLIResumeScope) == "" {
		return nil, errors.New("codex native compaction requires a CLI resume scope")
	}
	if !c.CanResumeScoped(req.CLIResumeScope, resumeSessionID) {
		return nil, fmt.Errorf("codex native compaction cannot resume thread %s in this session scope", resumeSessionID)
	}
	home, cleanup, err := prepareCodexTurnHome(c.configDir, req.CLIResumeScope)
	if err != nil {
		return nil, fmt.Errorf("codex CLI: prepare native compaction home: %w", err)
	}
	defer cleanup()

	cmd := proc.CommandContextNested(ctx, c.binPath, "app-server", "--listen", "stdio://", "--strict-config")
	cmd.Env = append(codexBaseEnv(), "CODEX_HOME="+home)
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr syncBuffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start codex app-server: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	conn := newCodexRPCConn(stdin, stdout, &stderr)
	defer conn.close()

	if err := conn.request(1, "initialize", map[string]any{"clientInfo": map[string]string{"name": "tionharness", "version": "1"}}); err != nil {
		return nil, err
	}
	if err := conn.notify("initialized"); err != nil {
		return nil, err
	}
	if err := conn.request(2, "thread/resume", map[string]any{"threadId": resumeSessionID}); err != nil {
		return nil, err
	}

	stepID := "codex-compact-" + resumeSessionID
	if req.OnEvent != nil {
		req.OnEvent(TraceStep{ID: stepID, Running: true, Kind: "compaction", Source: "cli-native", Provider: c.Name(), SessionAction: "native-compact"})
	}
	tombstone := func() {
		if req.OnEvent != nil {
			req.OnEvent(TraceStep{Kind: "tombstone", Ref: stepID})
		}
	}
	if err := conn.send(3, "thread/compact/start", map[string]any{"threadId": resumeSessionID}); err != nil {
		tombstone()
		return nil, err
	}
	// Both halves of the lifecycle are required: the ack proves the request was
	// accepted, the completion notification proves the thread was actually
	// compacted. conn.next bounds every wait, so a wedged app-server surfaces as a
	// timeout error instead of hanging this turn forever.
	for acknowledged, lifecycleCompleted := false, false; !acknowledged || !lifecycleCompleted; {
		envelope, rerr := conn.next("thread/compact/start")
		if rerr != nil {
			tombstone()
			return nil, rerr
		}
		if envelope.ID == 3 {
			if envelope.Error != nil {
				tombstone()
				return nil, fmt.Errorf("codex app-server thread/compact/start failed (%d): %s", envelope.Error.Code, envelope.Error.Message)
			}
			acknowledged = true
		}
		if codexCompactCompleted(envelope, resumeSessionID) {
			lifecycleCompleted = true
		}
	}
	completed := TraceStep{ID: stepID, Kind: "compaction", Trigger: "manual", Source: "cli-native", Provider: c.Name(), SessionAction: "native-compact"}
	if req.OnEvent != nil {
		req.OnEvent(completed)
	}
	return &Response{SessionID: resumeSessionID, Trace: []TraceStep{completed}}, nil
}

func codexCompactCompleted(envelope codexRPCEnvelope, threadID string) bool {
	return envelope.Params.ThreadID == threadID &&
		(envelope.Method == "thread/compacted" ||
			envelope.Method == "item/completed" && envelope.Params.Item.Type == "contextCompaction")
}

var _ CLINativeManualCompactor = (*CodexCLI)(nil)
