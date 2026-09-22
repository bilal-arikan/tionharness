package providers

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// newRespondingCLISession wires a CLISession whose fake CLI answers every user turn
// written to stdin with a complete stream-json burst carrying reply. Like the real
// process it answers only AFTER the input arrives, so a reply can never be drained
// as a previous turn's leftover.
func newRespondingCLISession(t *testing.T, reply string) *CLISession {
	t.Helper()
	outR, outW := io.Pipe()
	inR, inW := io.Pipe()
	burst := `{"type":"system","subtype":"init","session_id":"s1"}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"` + reply + `"}]}}` + "\n" +
		`{"type":"result","subtype":"success","result":"` + reply + `","usage":{"input_tokens":3,"output_tokens":2},"num_turns":1}` + "\n"
	go func() {
		in := bufio.NewReader(inR)
		for {
			if _, err := in.ReadString('\n'); err != nil {
				return
			}
			if _, err := outW.Write([]byte(burst)); err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		_ = outW.Close()
		_ = inR.Close()
	})
	return &CLISession{
		stdin:  inW,
		stdout: bufio.NewReader(outR),
		stderr: &bytes.Buffer{},
		model:  "test-model",
	}
}

// waitMutexHeld returns once mu is held elsewhere — i.e. the turn under test has
// entered its critical section.
func waitMutexHeld(t *testing.T, mu *sync.Mutex) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for mu.TryLock() {
		mu.Unlock()
		if time.Now().After(deadline) {
			t.Fatal("the busy turn never took its session lock")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestCLISessionPoolTurnNotBlockedByBusySession is the regression shield for the
// live stall of 2026-09-21: every pool Turn first ran EvictIdle, which took pl.mu
// and then blocked on each session's s.mu — held by CLISession.Turn for a whole
// turn. While one conversation was mid-turn, every other conversation in the
// workspace waited for it (four of five parallel subagents started ~2 min late).
func TestCLISessionPoolTurnNotBlockedByBusySession(t *testing.T) {
	pool := NewCLISessionPool()
	c := NewClaudeCLI("claude", "", "", "", "")
	req := Request{Messages: []Message{{Role: RoleUser, Text: "hello"}}}

	// Conversation A is mid-turn: its CLI never answers, so its Turn keeps the
	// session lock until the test cancels it.
	busy, _ := newTestCLISession(t)
	busy.turns, busy.lastUsed = 1, time.Now()
	pool.sessions["SES-A|AGT"] = busy
	busyCtx, cancelBusy := context.WithCancel(context.Background())
	busyDone := make(chan struct{})
	go func() {
		defer close(busyDone)
		_, _ = busy.Turn(busyCtx, "long task", Request{}, nil)
	}()
	t.Cleanup(func() {
		cancelBusy()
		<-busyDone
	})
	waitMutexHeld(t, &busy.mu)

	// Conversation B has a warm process with a matching launch fingerprint, so the
	// pool reuses it instead of cold-starting a real CLI.
	warm := newRespondingCLISession(t, "reply-b")
	sys, _ := c.buildSystemAndPrompt(req)
	warm.fingerprint = c.persistentFingerprint(req, sys)
	warm.turns, warm.lastUsed = 1, time.Now()
	pool.sessions["SES-B|AGT"] = warm

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	type turnResult struct {
		resp *Response
		err  error
	}
	done := make(chan turnResult, 1)
	go func() {
		resp, err := pool.Turn(ctx, "SES-B|AGT", c, req, nil)
		done <- turnResult{resp, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("turn for B: %v", r.err)
		}
		if !strings.Contains(r.resp.Text, "reply-b") {
			t.Fatalf("turn for B: text = %q, want reply-b", r.resp.Text)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("turn for conversation B waited for conversation A's in-flight turn")
	}
}

// EvictIdle must still reap a genuinely idle session and keep a recent one, while
// skipping — never waiting for — a session that is mid-turn, even a stale one.
func TestCLISessionPoolEvictIdleSkipsBusySession(t *testing.T) {
	pool := NewCLISessionPool()
	stale := time.Now().Add(-time.Hour)

	busy, _ := newTestCLISession(t)
	busy.turns, busy.lastUsed = 1, stale
	idle, _ := newTestCLISession(t)
	idle.turns, idle.lastUsed = 1, stale
	fresh, _ := newTestCLISession(t)
	fresh.turns, fresh.lastUsed = 1, time.Now()
	pool.sessions["busy|AGT"] = busy
	pool.sessions["idle|AGT"] = idle
	pool.sessions["fresh|AGT"] = fresh

	// CLISession.Turn holds s.mu for the whole turn; hold it the same way.
	busy.mu.Lock()
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(busy.mu.Unlock) }
	t.Cleanup(release)

	done := make(chan struct{})
	go func() {
		defer close(done)
		pool.EvictIdle(30 * time.Minute)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("EvictIdle waited on a session that is mid-turn")
	}
	release()

	if _, ok := pool.sessions["busy|AGT"]; !ok {
		t.Fatal("mid-turn session was evicted")
	}
	if _, ok := pool.sessions["fresh|AGT"]; !ok {
		t.Fatal("recently used session was evicted")
	}
	if _, ok := pool.sessions["idle|AGT"]; ok {
		t.Fatal("idle session was not evicted")
	}
	idle.mu.Lock()
	closed := idle.closed
	idle.mu.Unlock()
	if !closed {
		t.Fatal("evicted idle session was not closed")
	}
}
