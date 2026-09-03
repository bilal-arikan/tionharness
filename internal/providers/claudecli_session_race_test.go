package providers

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestCLISessionSecondTurnReceivesEveryLine is the regression shield for the
// live hang of 2026-09-03: Turn used to start a reader goroutine per turn over the
// shared bufio.Reader, and the previous turn's goroutine — still parked in
// ReadString after delivering its result — swallowed the first line(s) of the
// next turn (one buffered into its dead channel, the next dropped). When the
// stolen line was the `result`, turn 2 waited until the caller's deadline.
//
// The fake CLI writes turn 2's whole event burst the instant turn 1's result is
// out, which is the timing that made the race bite live; with the single
// per-process reader every line reaches the running Turn.
func TestCLISessionSecondTurnReceivesEveryLine(t *testing.T) {
	s, out := newTestCLISession(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	turn := func(text string) string {
		return `{"type":"system","subtype":"init","session_id":"s1"}` + "\n" +
			`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"` + text + `"}]}}` + "\n" +
			`{"type":"result","subtype":"success","result":"` + text + `","usage":{"input_tokens":3,"output_tokens":2},"num_turns":1}` + "\n"
	}

	for round := 1; round <= 5; round++ {
		reply := "reply-" + strings.Repeat("x", round)
		// Write the CLI's burst for this turn BEFORE (and regardless of) when Turn
		// starts consuming — the one-reader design must tolerate either order.
		go func() { _, _ = out.Write([]byte(turn(reply))) }()
		resp, err := s.Turn(ctx, "user "+reply, Request{}, nil)
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		if !strings.Contains(resp.Text, reply) {
			t.Fatalf("round %d: text = %q, want %q", round, resp.Text, reply)
		}
	}
	if s.turns != 5 {
		t.Fatalf("turns = %d, want 5", s.turns)
	}
}

// A stream that already ended (the process died between turns) must be reported
// by the NEXT Turn immediately instead of writing into a dead pipe and waiting.
func TestCLISessionTurnReportsEndedStream(t *testing.T) {
	s, out := newTestCLISession(t)
	s.readerOnce.Do(s.startReader)
	_ = out.Close()
	// Give the reader a moment to park the EOF.
	deadline := time.Now().Add(2 * time.Second)
	for len(s.lines) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := s.Turn(ctx, "hello", Request{}, nil); err == nil || !strings.Contains(err.Error(), "already ended") {
		t.Fatalf("err = %v, want 'stream already ended'", err)
	}
}
