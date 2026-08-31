package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// seedCodexThread builds a scoped resume home containing one rollout file, so
// CanResumeScoped sees the thread exactly as a completed turn would have left it.
func seedCodexThread(t *testing.T, base, scope, threadID string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(base, "auth.json"), []byte(`{"token":"test"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	home, cleanup, err := prepareCodexTurnHome(base, scope)
	if err != nil {
		t.Fatalf("prepare scoped home: %v", err)
	}
	cleanup()
	rollouts := filepath.Join(home, "sessions", "2026", "08", "31")
	if err := os.MkdirAll(rollouts, 0o700); err != nil {
		t.Fatal(err)
	}
	name := "rollout-2026-08-31T00-00-00-" + threadID + ".jsonl"
	if err := os.WriteFile(filepath.Join(rollouts, name), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The resume gate is what made every /compact fail when the caller passed a
// scope that did not match the turn path's: a mismatching scope must be rejected
// before the app-server is ever started, and the turn's own scope must pass.
func TestCompactNativeResumeScopeGate(t *testing.T) {
	base := t.TempDir()
	const threadID = "019c-compact-thread"
	turnScope := strings.Join([]string{"SES1", "AG1", "codex-cli", "gpt-5-codex", "static system prefix"}, "\x00")
	seedCodexThread(t, base, turnScope, threadID)

	c := NewCodexCLI("codex-binary-that-does-not-exist", "", base)
	if !c.CanResumeScoped(turnScope, threadID) {
		t.Fatal("the turn path's own scope did not find its thread")
	}
	if c.CanResumeScoped("SES1", threadID) {
		t.Fatal("a session-id-only scope resolved to the turn's resume home")
	}

	if _, err := c.CompactNative(context.Background(), threadID, Request{}); err == nil ||
		!strings.Contains(err.Error(), "requires a CLI resume scope") {
		t.Fatalf("empty scope was accepted: %v", err)
	}
	// The old bug in nativeCompactSession: session.ID alone hashes to a home with
	// no rollout, so compaction dies here instead of running.
	_, err := c.CompactNative(context.Background(), threadID, Request{CLIResumeScope: "SES1"})
	if err == nil || !strings.Contains(err.Error(), "cannot resume thread") {
		t.Fatalf("mismatching scope was accepted: %v", err)
	}
}

// newTestRPCConn wires a conn to a server the test drives: it returns the conn,
// the buffer capturing what the conn sent, and a writer feeding it lines.
func newTestRPCConn(t *testing.T) (*codexRPCConn, *bytes.Buffer, *io.PipeWriter, *syncBuffer) {
	t.Helper()
	var sent bytes.Buffer
	var stderr syncBuffer
	serverOut, serverIn := io.Pipe()
	conn := newCodexRPCConn(&sent, serverOut, &stderr)
	t.Cleanup(func() {
		conn.close()
		_ = serverIn.Close()
	})
	return conn, &sent, serverIn, &stderr
}

func TestCodexRPCRequestSkipsNotificationsAndMatchesID(t *testing.T) {
	conn, sent, server, _ := newTestRPCConn(t)
	go func() {
		// A notification (no id), a reply to a different request, an undecodable
		// line, then the real answer. Only the last one may satisfy request(2).
		_, _ = io.WriteString(server, `{"jsonrpc":"2.0","method":"thread/started","params":{"threadId":"t1"}}`+"\n")
		_, _ = io.WriteString(server, `{"jsonrpc":"2.0","id":1,"result":{}}`+"\n")
		_, _ = io.WriteString(server, "not json at all\n")
		_, _ = io.WriteString(server, `{"jsonrpc":"2.0","id":2,"result":{"threadId":"t1"}}`+"\n")
	}()
	if err := conn.request(2, "thread/resume", map[string]any{"threadId": "t1"}); err != nil {
		t.Fatalf("request did not match its own reply: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(sent.Bytes(), &out); err != nil {
		t.Fatalf("request wrote malformed JSON-RPC: %v", err)
	}
	if out["method"] != "thread/resume" || out["id"] != float64(2) {
		t.Fatalf("unexpected outgoing frame: %v", out)
	}
}

func TestCodexRPCRequestReportsServerError(t *testing.T) {
	conn, _, server, _ := newTestRPCConn(t)
	go func() {
		_, _ = io.WriteString(server, `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"no such method"}}`+"\n")
	}()
	err := conn.request(1, "initialize", nil)
	if err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Fatalf("server error was not surfaced: %v", err)
	}
}

func TestCodexRPCNextTimesOutOnSilentServer(t *testing.T) {
	original := codexCompactStepTimeoutDuration
	codexCompactStepTimeoutDuration = 20 * time.Millisecond
	t.Cleanup(func() { codexCompactStepTimeoutDuration = original })

	conn, _, _, _ := newTestRPCConn(t)
	done := make(chan error, 1)
	go func() {
		_, err := conn.next("thread/compact/start")
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "sent no thread/compact/start output within") {
			t.Fatalf("silent server did not time out cleanly: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("next blocked past the deadline: the wait is still unbounded")
	}
}

func TestCodexRPCNextReportsClosedStream(t *testing.T) {
	conn, _, server, stderr := newTestRPCConn(t)
	_, _ = stderr.Write([]byte("codex: authentication required"))
	_ = server.Close()
	_, err := conn.next("initialize")
	if err == nil || !strings.Contains(err.Error(), "authentication required") {
		t.Fatalf("closed stream did not explain itself: %v", err)
	}
}

func TestCodexCompactCompletedMatchesLifecycleEvents(t *testing.T) {
	decode := func(line string) codexRPCEnvelope {
		t.Helper()
		var envelope codexRPCEnvelope
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			t.Fatalf("decode %s: %v", line, err)
		}
		return envelope
	}
	compacted := decode(`{"jsonrpc":"2.0","method":"thread/compacted","params":{"threadId":"t1"}}`)
	if !codexCompactCompleted(compacted, "t1") {
		t.Fatal("thread/compacted was not recognised as completion")
	}
	if codexCompactCompleted(compacted, "other-thread") {
		t.Fatal("completion of a foreign thread was accepted")
	}
	item := decode(`{"jsonrpc":"2.0","method":"item/completed","params":{"threadId":"t1","item":{"type":"contextCompaction"}}}`)
	if !codexCompactCompleted(item, "t1") {
		t.Fatal("contextCompaction item/completed was not recognised")
	}
	other := decode(`{"jsonrpc":"2.0","method":"item/completed","params":{"threadId":"t1","item":{"type":"assistantMessage"}}}`)
	if codexCompactCompleted(other, "t1") {
		t.Fatal("an unrelated item/completed counted as compaction")
	}
}

// TestCodexRPCStderrReadIsRaceFree: next() reads stderr to explain an ended
// stream while os/exec's copier goroutine may still be writing into it —
// cmd.Wait only runs in CompactNative's deferred cleanup. With a plain
// *bytes.Buffer that pair is a data race; CI runs -race and fails here.
func TestCodexRPCStderrReadIsRaceFree(t *testing.T) {
	conn, _, server, stderr := newTestRPCConn(t)
	writing := make(chan struct{})
	written := make(chan struct{})
	go func() {
		defer close(written)
		close(writing)
		for i := 0; i < 500; i++ {
			_, _ = stderr.Write([]byte("codex: starting up\n"))
		}
	}()
	<-writing
	_ = server.Close()
	if _, err := conn.next("initialize"); err == nil {
		t.Fatal("closed stream must report an error")
	}
	<-written
}
