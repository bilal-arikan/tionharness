package db

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

func TestCLICompactionLifecycleJSONAndSummaryCount(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	agent, err := d.CreateAgent(ctx, Agent{Name: "A", Provider: "claude-cli"})
	if err != nil {
		t.Fatal(err)
	}
	session, err := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	base := providers.CLICompactionEvent{
		Provider: "claude-cli", AttemptID: "attempt-1", Attempt: 1,
		CLISessionIDIn: "cli-in",
	}
	for _, phase := range []providers.CLICompactionPhase{
		providers.CLICompactionAttempt, providers.CLICompactionSignal, providers.CLICompactionError,
	} {
		ev := base
		ev.Phase = phase
		ev.Signal = "precompact"
		if phase == providers.CLICompactionError {
			ev.DurationMs = 12
			ev.ErrorKind = "process_exit"
			ev.Error = "claude CLI native compaction failed sk-secret-token"
		}
		if err := d.AppendCLICompactionEvent(session.ID, agent.ID, ev); err != nil {
			t.Fatal(err)
		}
	}
	success := base
	success.Phase = providers.CLICompactionSuccess
	success.DurationMs = 7
	success.CLISessionIDOut = "cli-out"
	if err := d.AppendCLICompactionEvent(session.ID, agent.ID, success); err != nil {
		t.Fatal(err)
	}
	if err := d.AppendCLICompactionEvent(session.ID, agent.ID, success); err != nil {
		t.Fatal(err)
	}

	events, err := d.ReadDebugEvents(ctx, session.ID, DebugCLICompaction, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("lifecycle events = %d, want 4", len(events))
	}
	allJSON, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"cli-in", "cli-out", "sk-secret-token"} {
		if strings.Contains(string(allJSON), secret) {
			t.Fatalf("lifecycle journal leaks raw secret %q: %s", secret, allJSON)
		}
	}
	errorJSON, err := json.Marshal(events[2])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(errorJSON), `"retryable":false`) || strings.Contains(string(errorJSON), "sk-secret") {
		t.Fatalf("error JSON = %s", errorJSON)
	}
	raw, err := json.Marshal(events[3])
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"type":"cli_compaction"`, `"phase":"success"`, `"provider":"claude-cli"`, `"attemptId":"fp:`, `"attempt":1`, `"durationMs":7`, `"cliSessionFingerprintIn":"`, `"cliSessionFingerprintOut":"`} {
		if !strings.Contains(string(raw), field) {
			t.Fatalf("success JSON %s lacks %s", raw, field)
		}
	}
	if strings.Contains(string(raw), "cli-in") || strings.Contains(string(raw), "cli-out") {
		t.Fatalf("success JSON leaks raw CLI session id: %s", raw)
	}
	if events[3].CLISessionFingerprintIn != cliSessionFingerprint("cli-in") || events[3].CLISessionFingerprintOut != cliSessionFingerprint("cli-out") {
		t.Fatalf("unstable fingerprints: %+v", events[3])
	}
	summary, err := d.GetDebugSummary(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Compactions != 1 {
		t.Fatalf("Compactions = %d, want success only", summary.Compactions)
	}
}

func TestCLICompactionSuccessPreservesUnrelatedRawJournalLines(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	session, err := d.CreateSession(context.Background(), Session{Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	path := d.debugPath(session.ID)
	validFuture := []byte(`{"type":"turn","futureField":{"nested":"keep"},"name":"raw-name"}` + "\n")
	unknownFuture := []byte(`{"type":"future_event","other":17,"futureField":"keep-too"}` + "\n")
	malformed := []byte("malformed unrelated line { keep-byte-for-byte\n")
	otherCheckpoint := []byte(`{"type":"_cli_compaction_dedupe","provider":"codex-cli","name":"other-checkpoint","futureField":true}` + "\n")
	sameProviderCheckpoint := []byte(`{"type":"_cli_compaction_dedupe","provider":"claude-cli","name":"old-checkpoint"}` + "\n")
	seed := bytes.Join([][]byte{validFuture, unknownFuture, malformed, otherCheckpoint, sameProviderCheckpoint}, nil)
	if err := os.WriteFile(path, seed, 0o644); err != nil {
		t.Fatal(err)
	}

	ev := providers.CLICompactionEvent{
		Phase: providers.CLICompactionSuccess, Provider: "claude-cli", AttemptID: "raw-preservation", Attempt: 1,
	}
	if err := d.appendCLICompactionEvent(session.ID, "agent", ev, 32); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	preserved := bytes.Join([][]byte{validFuture, unknownFuture, malformed, otherCheckpoint}, nil)
	if !bytes.HasPrefix(got, preserved) {
		t.Fatalf("unrelated raw journal prefix changed\nwant: %q\n got: %q", preserved, got)
	}
	if bytes.Contains(got, sameProviderCheckpoint) || !bytes.Contains(got, otherCheckpoint) {
		t.Fatalf("provider checkpoint replacement mismatch: %q", got)
	}
	if bytes.Count(got, []byte(`"type":"cli_compaction"`)) != 1 || bytes.Count(got, []byte(`"type":"compaction"`)) != 1 {
		t.Fatalf("success batch count mismatch: %q", got)
	}
}

func TestCLICompactionSuccessConcurrentDuplicateIsIdempotent(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	session, err := d.CreateSession(context.Background(), Session{Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	ev := providers.CLICompactionEvent{
		Phase: providers.CLICompactionSuccess, Provider: "claude-cli", AttemptID: "concurrent-attempt", Attempt: 1,
	}
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- d.AppendCLICompactionEvent(session.ID, "agent", ev)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	lifecycle, err := d.ReadDebugEvents(context.Background(), session.ID, DebugCLICompaction, 0)
	if err != nil {
		t.Fatal(err)
	}
	compactions, err := d.ReadDebugEvents(context.Background(), session.ID, DebugCompaction, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(lifecycle) != 1 || len(compactions) != 1 {
		t.Fatalf("concurrent duplicate wrote lifecycle=%d compactions=%d", len(lifecycle), len(compactions))
	}
}

func TestCLICompactionSuccessDedupeSurvivesRestart(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	session, err := d.CreateSession(context.Background(), Session{Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	ev := providers.CLICompactionEvent{
		Phase: providers.CLICompactionSuccess, Provider: "claude-cli", AttemptID: "durable-attempt", Attempt: 1,
	}
	if err := d.AppendCLICompactionEvent(session.ID, "agent", ev); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.AppendCLICompactionEvent(session.ID, "agent", ev); err != nil {
		t.Fatal(err)
	}
	lifecycle, err := d.ReadDebugEvents(context.Background(), session.ID, DebugCLICompaction, 0)
	if err != nil {
		t.Fatal(err)
	}
	compactions, err := d.ReadDebugEvents(context.Background(), session.ID, DebugCompaction, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(lifecycle) != 1 || len(compactions) != 1 {
		t.Fatalf("restart duplicate wrote lifecycle=%d compactions=%d", len(lifecycle), len(compactions))
	}
}

func TestCLICompactionSuccessDedupeSurvivesPruning(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	session, err := d.CreateSession(context.Background(), Session{Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	old := providers.CLICompactionEvent{Phase: providers.CLICompactionSuccess, Provider: "claude-cli", AttemptID: "old-attempt", Attempt: 1}
	if err := d.appendCLICompactionEvent(session.ID, "agent", old, 4); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if err := d.AppendDebugEvent(session.ID, DebugEvent{Type: DebugTurn, Name: "filler"}, 4); err != nil {
			t.Fatal(err)
		}
	}
	before, err := d.ReadDebugEvents(context.Background(), session.ID, DebugCLICompaction, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 0 {
		t.Fatalf("old visible lifecycle survived cap pruning: %d", len(before))
	}
	if err := d.appendCLICompactionEvent(session.ID, "agent", old, 4); err != nil {
		t.Fatal(err)
	}
	newAttempt := old
	newAttempt.AttemptID = "new-attempt"
	newAttempt.Attempt = 2
	if err := d.appendCLICompactionEvent(session.ID, "agent", newAttempt, 4); err != nil {
		t.Fatal(err)
	}
	if err := d.appendCLICompactionEvent(session.ID, "agent", newAttempt, 4); err != nil {
		t.Fatal(err)
	}
	lifecycle, err := d.ReadDebugEvents(context.Background(), session.ID, DebugCLICompaction, 0)
	if err != nil {
		t.Fatal(err)
	}
	compactions, err := d.ReadDebugEvents(context.Background(), session.ID, DebugCompaction, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(lifecycle) != 1 || lifecycle[0].AttemptID != debugOpaqueFingerprint("AttemptID", "new-attempt") || len(compactions) != 1 {
		t.Fatalf("pruned dedupe result lifecycle=%+v compactions=%+v", lifecycle, compactions)
	}
}

func TestCLICompactionSuccessAtomicFailureRetry(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	session, err := d.CreateSession(context.Background(), Session{Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.AppendDebugEvent(session.ID, DebugEvent{Type: DebugTurn}, 8); err != nil {
		t.Fatal(err)
	}
	path := d.debugPath(session.ID)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ev := providers.CLICompactionEvent{Phase: providers.CLICompactionSuccess, Provider: "claude-cli", AttemptID: "atomic-attempt", Attempt: 1}

	d.debugAtomicWrite = func(string, []byte) error { return errors.New("injected temp sync failure") }
	if err := d.appendCLICompactionEvent(session.ID, "agent", ev, 8); err == nil {
		t.Fatal("temp-write failure unexpectedly succeeded")
	}
	afterTempFailure, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, afterTempFailure) {
		t.Fatal("temp-write failure changed durable journal")
	}
	d.debugAtomicWrite = func(path string, data []byte) error {
		if err := os.WriteFile(path+".tmp", data[:len(data)/2], 0o644); err != nil {
			return err
		}
		return errors.New("injected rename failure")
	}
	if err := d.appendCLICompactionEvent(session.ID, "agent", ev, 8); err == nil || !strings.Contains(err.Error(), "rename") {
		t.Fatalf("rename failure = %v", err)
	}
	afterRenameFailure, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, afterRenameFailure) {
		t.Fatal("rename/torn-temp failure changed durable journal")
	}
	d.debugAtomicWrite = nil
	if err := os.Remove(path + ".tmp"); err != nil {
		t.Fatal(err)
	}
	if err := d.appendCLICompactionEvent(session.ID, "agent", ev, 8); err != nil {
		t.Fatal(err)
	}
	if err := d.appendCLICompactionEvent(session.ID, "agent", ev, 8); err != nil {
		t.Fatal(err)
	}
	lifecycle, err := d.ReadDebugEvents(context.Background(), session.ID, DebugCLICompaction, 0)
	if err != nil {
		t.Fatal(err)
	}
	compactions, err := d.ReadDebugEvents(context.Background(), session.ID, DebugCompaction, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(lifecycle) != 1 || len(compactions) != 1 {
		t.Fatalf("retry wrote lifecycle=%d compactions=%d", len(lifecycle), len(compactions))
	}
}

func TestCLICompactionSuccessCheckpointAndJournalStayBounded(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	session, err := d.CreateSession(context.Background(), Session{Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	const cap = 16
	var last providers.CLICompactionEvent
	for i := 0; i < 200; i++ {
		last = providers.CLICompactionEvent{
			Phase: providers.CLICompactionSuccess, Provider: "claude-cli",
			AttemptID: fmt.Sprintf("stress-attempt-%d", i), Attempt: i + 1,
		}
		if err := d.appendCLICompactionEvent(session.ID, "agent", last, cap); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.ReadFile(d.debugPath(session.ID))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.appendCLICompactionEvent(session.ID, "agent", last, cap); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(d.debugPath(session.ID))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("latest-attempt retry changed bounded journal")
	}
	records, err := readDebugRecords(d.debugPath(session.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) > cap+debugSuccessBatchHeadroom+len(debugEnumValues["Provider"]) {
		t.Fatalf("physical records = %d, bound = %d", len(records), cap+debugSuccessBatchHeadroom+len(debugEnumValues["Provider"]))
	}
	checkpoints := 0
	lifecycle := 0
	compactions := 0
	for _, record := range records {
		switch record.Type {
		case debugCLICompactionDedupe:
			checkpoints++
		case DebugCLICompaction:
			lifecycle++
		case DebugCompaction:
			compactions++
		}
	}
	if checkpoints != 1 || lifecycle == 0 || lifecycle != compactions {
		t.Fatalf("bounded records checkpoints=%d lifecycle=%d compactions=%d", checkpoints, lifecycle, compactions)
	}
}
