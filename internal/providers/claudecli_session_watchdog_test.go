package providers

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

// newTestCLISession wires a CLISession onto in-memory pipes so a turn can be run
// without a real subprocess. The returned writer feeds the session's stdout.
func newTestCLISession(t *testing.T) (*CLISession, *io.PipeWriter) {
	t.Helper()
	outR, outW := io.Pipe()
	inR, inW := io.Pipe()
	go func() { _, _ = io.Copy(io.Discard, inR) }()
	t.Cleanup(func() {
		_ = outW.Close()
		_ = inR.Close()
	})
	return &CLISession{
		stdin:  inW,
		stdout: bufio.NewReader(outR),
		stderr: &bytes.Buffer{},
		model:  "test-model",
	}, outW
}

// A persistent turn used to block in bufio.ReadString, so cancellation was only
// observed BETWEEN complete lines: the user's Stop button did nothing and the
// wedged turn kept s.mu, blocking every later turn for the same key.
func TestCLISessionTurnHonoursContextCancellation(t *testing.T) {
	s, _ := newTestCLISession(t)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	done := make(chan error, 1)
	go func() {
		_, err := s.Turn(ctx, "hello", Request{}, nil)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Turn did not return after the context was cancelled")
	}

	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if !closed {
		t.Fatal("cancelled session was left open; the pool would hand the next turn a dead process")
	}
}

// After the first line the idle watchdog bounds stdout silence, so a CLI that
// stops emitting mid-turn is reclaimed instead of holding the session mutex.
func TestCLISessionTurnIdleWatchdogReclaimsSilentStream(t *testing.T) {
	s, out := newTestCLISession(t)
	prev := cliSessionIdleWindow()
	SetCLISessionIdleTimeout(80 * time.Millisecond)
	t.Cleanup(func() { SetCLISessionIdleTimeout(prev) })

	var kills []WatchdogKill
	killed := make(chan struct{}, 1)
	req := Request{OnWatchdog: func(k WatchdogKill) {
		kills = append(kills, k)
		killed <- struct{}{}
	}}

	done := make(chan error, 1)
	go func() {
		_, err := s.Turn(context.Background(), "hello", req, nil)
		done <- err
	}()
	// One line proves the turn is alive, then the stream goes silent forever.
	if _, err := out.Write([]byte(`{"type":"system","subtype":"init","session_id":"s1"}` + "\n")); err != nil {
		t.Fatalf("write stdout line: %v", err)
	}

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("silent stream returned success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("idle watchdog never fired")
	}
	select {
	case <-killed:
	default:
		t.Fatal("watchdog kill was not reported")
	}
	if kills[0].Reason != WatchdogReasonIdle || kills[0].Provider != "claude-cli" {
		t.Fatalf("watchdog record = %+v, want an idle claude-cli kill", kills[0])
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if !closed {
		t.Fatal("idle-killed session was left open")
	}
}
