package providers

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func fakeClaudeVersionExecutable(t *testing.T, version string) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "claude.cmd")
		if err := os.WriteFile(path, []byte("@echo off\r\necho Claude Code "+version+"\r\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	path := filepath.Join(dir, "claude")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' 'Claude Code "+version+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestClaudeNativeCompactionCapabilityVersionProbe(t *testing.T) {
	for _, tc := range []struct {
		version string
		want    bool
	}{{"2.1.237", false}, {"2.1.238", true}} {
		t.Run(tc.version, func(t *testing.T) {
			provider := NewClaudeCLI(fakeClaudeVersionExecutable(t, tc.version), "", "", "", "")
			if got := provider.NativeCompactionEvents(); got != tc.want {
				t.Fatalf("NativeCompactionEvents() = %v, want %v", got, tc.want)
			}
		})
	}
}

func feedClaudeFixture(t *testing.T, version, name string) (*cliStreamParser, []CLICompactionEvent) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "claudecli", version, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var events []CLICompactionEvent
	p := lifecycleParser(nil, func(ev CLICompactionEvent) { events = append(events, ev) })
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		p.feed(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return p, events
}

func TestClaudeCompactionFixtureMatrix(t *testing.T) {
	t.Run("supported success deduplicates terminal", func(t *testing.T) {
		p, events := feedClaudeFixture(t, "2.1.238", "success.jsonl")
		if len(p.resp.Trace) != 1 || countCLICompactionPhase(events, CLICompactionSuccess) != 1 {
			t.Fatalf("trace=%+v events=%+v", p.resp.Trace, events)
		}
	})
	t.Run("supported failure closes attempt", func(t *testing.T) {
		_, events := feedClaudeFixture(t, "2.1.238", "failed.jsonl")
		if countCLICompactionPhase(events, CLICompactionError) != 1 {
			t.Fatalf("events=%+v", events)
		}
	})
	t.Run("supported two correlated", func(t *testing.T) {
		_, events := feedClaudeFixture(t, "2.1.238", "two-correlated.jsonl")
		if countCLICompactionPhase(events, CLICompactionSuccess) != 2 {
			t.Fatalf("events=%+v", events)
		}
	})
	t.Run("supported crash closes open attempt", func(t *testing.T) {
		p, _ := feedClaudeFixture(t, "2.1.238", "cancel-crash.jsonl")
		var events []CLICompactionEvent
		p.onCompaction.onEvent = func(ev CLICompactionEvent) { events = append(events, ev) }
		p.terminateOpenNativeCompaction(CLICompactionError, "process_exit", true, 1)
		if countCLICompactionPhase(events, CLICompactionError) != 1 {
			t.Fatalf("events=%+v", events)
		}
	})
	t.Run("malformed and stderr-only do not invent compaction", func(t *testing.T) {
		malformed, malformedEvents := feedClaudeFixture(t, "2.1.238", "malformed.jsonl")
		if len(malformedEvents) != 0 || len(malformed.resp.Trace) != 1 || malformed.parseDropCount != 1 {
			t.Fatalf("malformed trace=%+v events=%+v drops=%d", malformed.resp.Trace, malformedEvents, malformed.parseDropCount)
		}
		stderrOnly, stderrEvents := feedClaudeFixture(t, "2.1.238", "stderr-only.txt")
		if len(stderrEvents) != 0 || len(stderrOnly.resp.Trace) != 0 {
			t.Fatalf("stderr-only trace=%+v events=%+v", stderrOnly.resp.Trace, stderrEvents)
		}
	})
	t.Run("old unknown schema fails closed", func(t *testing.T) {
		p, events := feedClaudeFixture(t, "2.1.237", "unknown-schema.jsonl")
		if len(events) != 0 || len(p.resp.Trace) != 0 {
			t.Fatalf("trace=%+v events=%+v", p.resp.Trace, events)
		}
	})
}

func countCLICompactionPhase(events []CLICompactionEvent, phase CLICompactionPhase) int {
	n := 0
	for _, ev := range events {
		if ev.Phase == phase {
			n++
		}
	}
	return n
}
