package db

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// readTranscript reads a session's messages.jsonl back from disk, decoding every
// line. It is deliberately file-based: the point of these tests is what actually
// reached the transcript, not what the in-memory maps believe.
func readTranscript(t *testing.T, d *DB, sessionID string) []Message {
	t.Helper()
	f, err := os.Open(d.dir(dirSessions, sessionID, sessionMsgsFile))
	if err != nil {
		t.Fatalf("open transcript: %v", err)
	}
	defer f.Close()
	var out []Message
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var m Message
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("transcript line is not a complete JSON message (interleaved write?): %q: %v", line, err)
		}
		out = append(out, m)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan transcript: %v", err)
	}
	return out
}

func newTestSession(t *testing.T, d *DB) Session {
	t.Helper()
	ctx := context.Background()
	agent, err := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return sess
}

// TestAddMessageConcurrentAppendsStayIntactAndOrdered: with the file write moved
// out from under the global store lock, the only thing keeping two appends to the
// SAME session apart is the per-session transcript lock. Without it, two writers
// can have their lines torn into each other (a half-written JSON object followed
// by another message's tail) or land out of order relative to the in-memory
// slice. This test catches both: every line must decode on its own, all N must be
// present, and the file order must equal the in-memory order.
func TestAddMessageConcurrentAppendsStayIntactAndOrdered(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sess := newTestSession(t, d)

	const n = 64
	// A payload large enough that a single append is several kilobytes: a torn
	// interleave is only observable when the write is big enough to be split.
	pad := strings.Repeat("x", 4096)

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			if _, err := d.AddMessage(ctx, Message{
				SessionID: sess.ID,
				Role:      "user",
				Text:      "m" + strconv.Itoa(i) + ":" + pad,
			}); err != nil {
				t.Errorf("append %d: %v", i, err)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if t.Failed() {
		t.FailNow()
	}

	onDisk := readTranscript(t, d, sess.ID)
	if len(onDisk) != n {
		t.Fatalf("transcript has %d messages, want %d", len(onDisk), n)
	}
	seen := map[string]bool{}
	for _, m := range onDisk {
		if m.Text == "" || m.ID == "" || m.SessionID != sess.ID {
			t.Fatalf("incomplete message on disk: %+v", m)
		}
		if seen[m.ID] {
			t.Fatalf("message %s written twice", m.ID)
		}
		seen[m.ID] = true
	}

	inMem, _, err := d.ListMessagesTail(ctx, sess.ID, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(inMem) != len(onDisk) {
		t.Fatalf("memory has %d messages, disk has %d", len(inMem), len(onDisk))
	}
	for i := range inMem {
		if inMem[i].ID != onDisk[i].ID {
			t.Fatalf("order diverges at %d: memory %s, disk %s", i, inMem[i].ID, onDisk[i].ID)
		}
	}

	s, err := d.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if s.MessageCount != n {
		t.Fatalf("MessageCount = %d, want %d", s.MessageCount, n)
	}
}

// TestAddMessageConcurrentFailuresLeaveNothingBehind: when the transcript cannot
// be written at all, N concurrent appends must ALL fail and none of them may
// leave a message in memory or bump the session counters. Under concurrency a
// snapshot-and-restore rollback is the fragile part (one goroutine restoring a
// header snapshot taken before another goroutine's successful append would undo
// that append too); this asserts the end state instead of the mechanism.
func TestAddMessageConcurrentFailuresLeaveNothingBehind(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sess := newTestSession(t, d)
	if _, err := d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "user", Text: "ok"}); err != nil {
		t.Fatalf("first append: %v", err)
	}
	before, err := d.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}

	// Same failure injection as TestAddMessageRollsBackWhenAppendFails: without the
	// session directory, os.OpenFile fails for every subsequent append.
	if err := os.RemoveAll(d.dir(dirSessions, sess.ID)); err != nil {
		t.Fatalf("remove session dir: %v", err)
	}

	const n = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			if _, err := d.AddMessage(ctx, Message{
				SessionID: sess.ID,
				Role:      "assistant",
				Text:      "lost" + strconv.Itoa(i),
				Steps:     `[{"type":"tool","tool":"Bash"}]`,
			}); err == nil {
				t.Errorf("append %d: want error, got nil", i)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	after, err := d.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get session after failures: %v", err)
	}
	if after.MessageCount != before.MessageCount {
		t.Fatalf("MessageCount = %d, want %d", after.MessageCount, before.MessageCount)
	}
	if after.ToolCallCount != before.ToolCallCount {
		t.Fatalf("ToolCallCount = %d, want %d", after.ToolCallCount, before.ToolCallCount)
	}
	msgs, _, err := d.ListMessagesTail(ctx, sess.ID, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != before.MessageCount {
		t.Fatalf("in-memory transcript has %d messages, want %d", len(msgs), before.MessageCount)
	}
	for _, m := range msgs {
		if strings.HasPrefix(m.Text, "lost") {
			t.Fatalf("un-persisted message %q is still in memory", m.Text)
		}
	}
}

// TestAddMessageConcurrentAcrossSessionsIsIsolated: appends to DIFFERENT sessions
// must not be serialised by each other, and must not cross-contaminate the two
// transcripts. This is the property the per-session (rather than global) lock
// buys; the assertion here is correctness of the split, not its speed.
func TestAddMessageConcurrentAcrossSessionsIsIsolated(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, err := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	const sessions, perSession = 8, 16
	ids := make([]string, sessions)
	for i := range ids {
		s, err := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "S" + strconv.Itoa(i)})
		if err != nil {
			t.Fatalf("create session: %v", err)
		}
		ids[i] = s.ID
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, id := range ids {
		for j := 0; j < perSession; j++ {
			wg.Add(1)
			go func(id string, j int) {
				defer wg.Done()
				<-start
				if _, err := d.AddMessage(ctx, Message{
					SessionID: id,
					Role:      "user",
					Text:      id + "#" + strconv.Itoa(j),
				}); err != nil {
					t.Errorf("append %s/%d: %v", id, j, err)
				}
			}(id, j)
		}
	}
	close(start)
	wg.Wait()

	for _, id := range ids {
		onDisk := readTranscript(t, d, id)
		if len(onDisk) != perSession {
			t.Fatalf("session %s has %d lines, want %d", id, len(onDisk), perSession)
		}
		for _, m := range onDisk {
			if m.SessionID != id {
				t.Fatalf("session %s transcript contains a message of %s", id, m.SessionID)
			}
		}
	}
}

// BenchmarkAddMessageParallel measures the append path under the contention it
// actually sees in production: many goroutines appending to their own sessions
// while the store is also read. Before the split every one of these serialised on
// d.mu across a synchronous file write.
func BenchmarkAddMessageParallel(b *testing.B) {
	ctx := context.Background()
	dir := b.TempDir()
	d, err := Open(filepath.Join(dir, "store"))
	if err != nil {
		b.Fatalf("open: %v", err)
	}
	agent, err := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	if err != nil {
		b.Fatalf("create agent: %v", err)
	}
	var mu sync.Mutex
	next := 0
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		mu.Lock()
		next++
		n := next
		mu.Unlock()
		s, err := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "B" + strconv.Itoa(n)})
		if err != nil {
			b.Errorf("create session: %v", err)
			return
		}
		for pb.Next() {
			if _, err := d.AddMessage(ctx, Message{SessionID: s.ID, Role: "assistant", Text: "delta"}); err != nil {
				b.Errorf("append: %v", err)
				return
			}
		}
	})
}
