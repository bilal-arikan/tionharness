//go:build !windows

package providers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeWarmCLI writes a claude stand-in that stays alive and answers every
// stream-json input line with a result, appending one line to starts per launch.
func fakeWarmCLI(t *testing.T) (bin, starts string) {
	t.Helper()
	dir := t.TempDir()
	bin = filepath.Join(dir, "claude")
	starts = filepath.Join(dir, "starts")
	script := "#!/bin/sh\necho start >> '" + starts + "'\n" +
		"while IFS= read -r line; do\n" +
		`  printf '%s\n' '{"type":"assistant","message":{"id":"m1","role":"assistant","content":[{"type":"text","text":"ok"}]}}'` + "\n" +
		`  printf '%s\n' '{"type":"result","subtype":"success","result":"ok","is_error":false,"session_id":"s1"}'` + "\n" +
		"done\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, starts
}

func launches(t *testing.T, starts string) int {
	t.Helper()
	data, _ := os.ReadFile(starts)
	return strings.Count(string(data), "start")
}

// TestWarmSessionOutlivesItsTurn: the warm process used to be bound to the first
// turn's ctx, so the end of every turn killed it and the next one fell back to
// one-shot. It must survive a finished turn, and a process that died between
// turns must be restarted inside the next turn instead of failing it.
func TestWarmSessionOutlivesItsTurn(t *testing.T) {
	bin, starts := fakeWarmCLI(t)
	c := NewClaudeCLI(bin, "", t.TempDir(), "", "")
	pool := NewCLISessionPool()
	defer pool.Close()
	req := Request{Model: "claude-sonnet-5-5", Messages: []Message{{Role: RoleUser, Text: "hi"}}}

	ctx1, cancel1 := context.WithTimeout(context.Background(), 10*time.Second)
	if resp, err := pool.Turn(ctx1, "k", c, req, nil); err != nil || resp.Text != "ok" {
		t.Fatalf("turn 1: %+v %v", resp, err)
	}
	cancel1() // the turn's request ends
	time.Sleep(200 * time.Millisecond)

	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()
	if resp, err := pool.Turn(ctx2, "k", c, req, nil); err != nil || resp.Text != "ok" {
		t.Fatalf("turn 2 must reuse the warm process: %+v %v", resp, err)
	}
	if n := launches(t, starts); n != 1 {
		t.Fatalf("process launched %d times across two turns, want 1 (warm reuse)", n)
	}

	// Kill it between turns: the next turn cold-restarts in place and succeeds.
	pool.mu.Lock()
	sess := pool.sessions["k"]
	pool.mu.Unlock()
	_ = sess.cmd.Process.Kill()
	time.Sleep(200 * time.Millisecond)
	if resp, err := pool.Turn(ctx2, "k", c, req, nil); err != nil || resp.Text != "ok" {
		t.Fatalf("turn after a dead process: %+v %v", resp, err)
	}
	if n := launches(t, starts); n != 2 {
		t.Fatalf("launches = %d, want 2 (one cold restart)", n)
	}
}
