package db

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func cliReplyTestState() CLIReplyState {
	return CLIReplyState{
		UpdateResume: true, ResumeSessionID: "new-authority", ResumeSentMsgCount: 9,
		UpdateCompactBoundary: true, CompactMsgCount: 8,
		ClearNativeCompactionPending: true,
	}
}

func TestCLIReplyWALRecoversEveryCrashPhaseExactlyOnce(t *testing.T) {
	for _, phase := range []cliReplyTxnPhase{
		cliReplyTxnPrepared,
		cliReplyTxnMessage,
		cliReplyTxnHeader,
		cliReplyTxnActivity,
		cliReplyTxnRetired,
	} {
		t.Run(string(phase), func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "store")
			d, err := Open(root)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			session, err := d.CreateSession(ctx, Session{
				Title: "T", CLISessionID: "stale-authority", CLISentMsgCount: 4,
				CLICompactMsgCount: 3, CLINativeCompactionPending: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			hookCalls := 0
			if err := d.SetActivityHook(func(ActivitySignal) error {
				hookCalls++
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			d.cliReplyTxnHook = func(got cliReplyTxnPhase) error {
				if got == phase {
					return errors.New("simulated crash")
				}
				return nil
			}
			_, err = d.AddMessageWithCLIState(ctx, Message{
				ID: "stable-reply", SessionID: session.ID, Role: "assistant", Text: "completed",
			}, cliReplyTestState())
			if err == nil {
				t.Fatal("simulated crash unexpectedly succeeded")
			}
			if hookCalls != 0 {
				t.Fatalf("activity hook calls before recovery = %d", hookCalls)
			}

			reopened, err := Open(root)
			if err != nil {
				t.Fatal(err)
			}
			msgs, err := reopened.ListMessages(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(msgs) != 1 || msgs[0].ID != "stable-reply" {
				t.Fatalf("messages after recovery = %+v", msgs)
			}
			got, err := reopened.GetSession(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.CLISessionID != "new-authority" || got.CLISentMsgCount != 9 || got.CLICompactMsgCount != 8 || got.CLINativeCompactionPending {
				t.Fatalf("session after recovery = %+v", got)
			}
			if _, err := os.Stat(filepath.Join(root, dirSessions, session.ID, cliReplyWALFile)); !os.IsNotExist(err) {
				t.Fatalf("WAL remains after recovery: %v", err)
			}

			again, err := Open(root)
			if err != nil {
				t.Fatal(err)
			}
			msgs, err = again.ListMessages(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(msgs) != 1 {
				t.Fatalf("second reopen duplicated reply: %+v", msgs)
			}
		})
	}
}

func TestCLIReplyWALRecoversBeforeLaterNormalMessage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	session, err := d.CreateSession(ctx, Session{Title: "T", CLINativeCompactionPending: true})
	if err != nil {
		t.Fatal(err)
	}
	d.cliReplyTxnHook = func(phase cliReplyTxnPhase) error {
		if phase == cliReplyTxnPrepared {
			return errors.New("simulated crash")
		}
		return nil
	}
	if _, err := d.AddMessageWithCLIState(ctx, Message{
		ID: "assistant-first", SessionID: session.ID, Role: "assistant", Text: "reply",
	}, cliReplyTestState()); err == nil {
		t.Fatal("prepared crash unexpectedly succeeded")
	}
	d.cliReplyTxnHook = nil
	if _, err := d.AddMessage(ctx, Message{
		ID: "user-second", SessionID: session.ID, Role: "user", Text: "later",
	}); err != nil {
		t.Fatal(err)
	}
	msgs, err := d.ListMessages(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 || msgs[0].ID != "assistant-first" || msgs[1].ID != "user-second" {
		t.Fatalf("message order = %+v", msgs)
	}
}

func TestCLIReplyWALSerializesSameSessionWithoutBlockingAnother(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first, err := d.CreateSession(ctx, Session{Title: "first", CLINativeCompactionPending: true})
	if err != nil {
		t.Fatal(err)
	}
	second, err := d.CreateSession(ctx, Session{Title: "second"})
	if err != nil {
		t.Fatal(err)
	}
	prepared := make(chan struct{})
	release := make(chan struct{})
	d.cliReplyTxnHook = func(phase cliReplyTxnPhase) error {
		if phase == cliReplyTxnPrepared {
			close(prepared)
			<-release
			return errors.New("simulated crash")
		}
		return nil
	}
	cliDone := make(chan error, 1)
	go func() {
		_, err := d.AddMessageWithCLIState(ctx, Message{
			ID: "assistant-first", SessionID: first.ID, Role: "assistant", Text: "reply",
		}, cliReplyTestState())
		cliDone <- err
	}()
	<-prepared

	sameDone := make(chan error, 1)
	go func() {
		_, err := d.AddMessage(ctx, Message{ID: "user-second", SessionID: first.ID, Role: "user", Text: "later"})
		sameDone <- err
	}()
	select {
	case err := <-sameDone:
		t.Fatalf("same-session mutation did not wait: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	otherDone := make(chan error, 1)
	go func() {
		_, err := d.AddMessage(ctx, Message{ID: "other", SessionID: second.ID, Role: "user", Text: "parallel"})
		otherDone <- err
	}()
	select {
	case err := <-otherDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("different-session mutation blocked")
	}

	close(release)
	if err := <-cliDone; err == nil {
		t.Fatal("simulated crash unexpectedly succeeded")
	}
	d.cliReplyTxnHook = nil
	if err := <-sameDone; err != nil {
		t.Fatal(err)
	}
	msgs, err := d.ListMessages(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 || msgs[0].ID != "assistant-first" || msgs[1].ID != "user-second" {
		t.Fatalf("same-session order = %+v", msgs)
	}
}

func TestCLIReplyActivityOutboxReplaysWithStableIdentity(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	session, err := d.CreateSession(ctx, Session{Title: "T", CLINativeCompactionPending: true})
	if err != nil {
		t.Fatal(err)
	}
	d.cliReplyTxnHook = func(phase cliReplyTxnPhase) error {
		if phase == cliReplyTxnRetired {
			return errors.New("crash before activity delivery")
		}
		return nil
	}
	if _, err := d.AddMessageWithCLIState(ctx, Message{
		ID: "stable-reply", SessionID: session.ID, Role: "assistant", Text: "reply",
	}, cliReplyTestState()); err == nil {
		t.Fatal("retired-phase crash unexpectedly succeeded")
	}

	var mu sync.Mutex
	deliveryAttempts := map[string]int{}
	processed := map[string]bool{}
	processedCount := 0
	receiver := func(sig ActivitySignal) error {
		mu.Lock()
		deliveryAttempts[sig.EventID]++
		if !processed[sig.EventID] {
			processed[sig.EventID] = true
			processedCount++
		}
		mu.Unlock()
		return nil
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	reopened.cliReplyActivityHook = func(ActivitySignal) error {
		return errors.New("crash after receiver acceptance")
	}
	if err := reopened.SetActivityHook(receiver); err == nil {
		t.Fatal("delivery crash unexpectedly succeeded")
	}

	again, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := again.SetActivityHook(receiver); err != nil {
		t.Fatal(err)
	}
	if err := again.SetActivityHook(receiver); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	eventID := "cli-reply:" + session.ID + ":stable-reply"
	if len(deliveryAttempts) != 1 || deliveryAttempts[eventID] != 2 || processedCount != 1 {
		t.Fatalf("stable delivery attempts=%+v processed=%d", deliveryAttempts, processedCount)
	}
	mu.Unlock()
	paths, err := filepath.Glob(filepath.Join(root, dirSessions, session.ID, cliReplyActivityPrefix+"*"+cliReplyActivitySuffix))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Fatalf("activity outbox remains: %v", paths)
	}
}

func TestCLIReplyActivityIdentityIncludesSession(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first, err := d.CreateSession(ctx, Session{Title: "first"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := d.CreateSession(ctx, Session{Title: "second"})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	if err := d.SetActivityHook(func(sig ActivitySignal) error {
		ids = append(ids, sig.EventID)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, sessionID := range []string{first.ID, second.ID} {
		if _, err := d.AddMessageWithCLIState(ctx, Message{ID: "same-txn", SessionID: sessionID, Role: "assistant"}, CLIReplyState{}); err != nil {
			t.Fatal(err)
		}
	}
	if len(ids) != 2 || ids[0] == ids[1] {
		t.Fatalf("event ids = %v", ids)
	}
}

func TestLegacyCLIReplyWALRecoveryCreatesNewOutbox(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	session, err := d.CreateSession(ctx, Session{})
	if err != nil {
		t.Fatal(err)
	}
	d.cliReplyTxnHook = func(phase cliReplyTxnPhase) error {
		if phase == cliReplyTxnPrepared {
			return errors.New("crash")
		}
		return nil
	}
	if _, err := d.AddMessageWithCLIState(ctx, Message{ID: "legacy", SessionID: session.ID, Role: "assistant"}, CLIReplyState{}); err == nil {
		t.Fatal("prepared crash unexpectedly succeeded")
	}
	walPath := filepath.Join(root, dirSessions, session.ID, cliReplyWALFile)
	raw, err := os.ReadFile(walPath)
	if err != nil {
		t.Fatal(err)
	}
	var wal cliReplyWAL
	if err := json.Unmarshal(raw, &wal); err != nil {
		t.Fatal(err)
	}
	wal.Version = 1
	wal.WorkspaceMessageTotal = 0
	wal.WorkspaceToolTotal = 0
	raw, err = json.Marshal(wal)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(walPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	var eventID string
	if err := reopened.SetActivityHook(func(sig ActivitySignal) error {
		eventID = sig.EventID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if eventID != "cli-reply:"+session.ID+":legacy" {
		t.Fatalf("legacy recovery event id = %q", eventID)
	}
}

func TestCLIReplyActivityCorruptionDoesNotPoisonValidSession(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	bad, err := d.CreateSession(ctx, Session{Title: "bad"})
	if err != nil {
		t.Fatal(err)
	}
	good, err := d.CreateSession(ctx, Session{Title: "good"})
	if err != nil {
		t.Fatal(err)
	}
	badPath := filepath.Join(root, dirSessions, bad.ID, cliReplyActivityPrefix+"broken"+cliReplyActivitySuffix)
	if err := os.WriteFile(badPath, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	goodSignal := ActivitySignal{EventID: "good-event", SessionID: good.ID, MessageTotal: 1, MessageDelta: 1, WorkspaceMessageTotal: 1}
	if err := persistCLIReplyActivity(filepath.Join(root, dirSessions, good.ID), goodSignal); err != nil {
		t.Fatal(err)
	}
	var delivered []string
	if err := d.SetActivityHook(func(sig ActivitySignal) error {
		delivered = append(delivered, sig.EventID)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(delivered) != 1 || delivered[0] != goodSignal.EventID {
		t.Fatalf("delivered = %v", delivered)
	}
	if _, err := os.Stat(badPath + ".quarantine"); err != nil {
		t.Fatalf("corrupt record was not quarantined: %v", err)
	}
	if _, err := Open(root); err != nil {
		t.Fatalf("workspace reopen poisoned: %v", err)
	}
}

func TestActivityHookPanicRetainsOutboxAndAllowsRetry(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := d.CreateSession(context.Background(), Session{})
	if err != nil {
		t.Fatal(err)
	}
	sig := ActivitySignal{EventID: "panic-event", SessionID: s.ID, MessageTotal: 1, MessageDelta: 1}
	if err := persistCLIReplyActivity(filepath.Join(root, dirSessions, s.ID), sig); err != nil {
		t.Fatal(err)
	}
	if err := d.SetActivityHook(func(ActivitySignal) error { panic("boom") }); err == nil {
		t.Fatal("panic was not converted to an error")
	}
	calls := 0
	if err := d.SetActivityHook(func(ActivitySignal) error { calls++; return nil }); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("retry calls = %d", calls)
	}
}

func TestActivityHookReentrantCLIReplyDoesNotDeadlock(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first, err := d.CreateSession(ctx, Session{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := d.CreateSession(ctx, Session{})
	if err != nil {
		t.Fatal(err)
	}
	var entered atomic.Bool
	reentered := make(chan error, 1)
	if err := d.SetActivityHook(func(ActivitySignal) error {
		if entered.CompareAndSwap(false, true) {
			_, addErr := d.AddMessageWithCLIState(ctx, Message{ID: "nested-same", SessionID: first.ID, Role: "assistant"}, CLIReplyState{})
			if addErr == nil {
				_, addErr = d.AddMessageWithCLIState(ctx, Message{ID: "nested-other", SessionID: second.ID, Role: "assistant"}, CLIReplyState{})
			}
			reentered <- addErr
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, addErr := d.AddMessageWithCLIState(ctx, Message{ID: "outer", SessionID: first.ID, Role: "assistant"}, CLIReplyState{})
		done <- addErr
	}()
	for name, ch := range map[string]<-chan error{"outer": done, "nested": reentered} {
		select {
		case err := <-ch:
			if err != nil {
				t.Fatalf("%s add: %v", name, err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s add deadlocked", name)
		}
	}
}

// TestCLIReplyActivityCarriesDeterministicWorkspaceTotals pins what a counter
// automation needs from the workspace-wide activity sequence: every append —
// plain or CLI-state — reports the total BEFORE and AFTER itself, the two chain
// with no gap and no repeat across sessions, and a threshold ("every 2 messages")
// is therefore crossed exactly once no matter which session moved the counter.
//
// The hook is installed before the first append on purpose. Delivery is durable:
// an append made while no hook is registered persists its signal and hands it to
// the next hook that appears, so installing the hook late does not filter that
// signal out — it only delays it into a later, harder-to-read position.
func TestCLIReplyActivityCarriesDeterministicWorkspaceTotals(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first, err := d.CreateSession(ctx, Session{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := d.CreateSession(ctx, Session{})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var signals []ActivitySignal
	if err := d.SetActivityHook(func(sig ActivitySignal) error {
		if sig.EventID != "" {
			mu.Lock()
			signals = append(signals, sig)
			mu.Unlock()
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.AddMessage(ctx, Message{SessionID: first.ID, Role: "user"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.AddMessageWithCLIState(ctx, Message{ID: "first", SessionID: first.ID, Role: "assistant"}, CLIReplyState{}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.AddMessageWithCLIState(ctx, Message{ID: "second", SessionID: second.ID, Role: "assistant"}, CLIReplyState{}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(signals) != 3 {
		t.Fatalf("signal count = %d, want 3: %+v", len(signals), signals)
	}
	for i, sig := range signals {
		wantPrevious := int64(i)
		if sig.WorkspaceMessagePrevious != wantPrevious || sig.WorkspaceMessageTotal != wantPrevious+1 {
			t.Fatalf("signal %d spans %d->%d, want %d->%d",
				i, sig.WorkspaceMessagePrevious, sig.WorkspaceMessageTotal, wantPrevious, wantPrevious+1)
		}
	}
	crossings := 0
	for _, sig := range signals {
		if sig.WorkspaceMessagePrevious/2 < sig.WorkspaceMessageTotal/2 {
			crossings++
		}
	}
	if crossings != 1 {
		t.Fatalf("threshold crossings = %d, want 1", crossings)
	}
}

// TestCLIReplyWALCorruptionQuarantinesWithoutBrickingTheWorkspace pins the
// corrupt-sidecar contract this repo already applies to inbox.json
// (_Docs/58-QUEUE-SENKRON.md): a record that cannot be parsed is never treated as
// "nothing to recover" and never deleted — it is renamed out of the way, the
// session is marked degraded, and the failure is logged. One unreadable sidecar
// must not make the whole workspace unopenable: refusing to boot would take every
// OTHER session down with it, which is a far larger loss than the one session
// whose pending transaction cannot be replayed.
func TestCLIReplyWALCorruptionQuarantinesWithoutBrickingTheWorkspace(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	session, err := d.CreateSession(context.Background(), Session{Title: "T", CLINativeCompactionPending: true})
	if err != nil {
		t.Fatal(err)
	}
	walPath := filepath.Join(root, dirSessions, session.ID, cliReplyWALFile)
	corrupt := []byte(`{"version":1,"message":"secret-authority"}`)
	if err := os.WriteFile(walPath, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatalf("a corrupt WAL in one session must not fail the workspace open: %v", err)
	}
	if _, err := reopened.GetSession(context.Background(), session.ID); err != nil {
		t.Fatalf("degraded session must still load: %v", err)
	}
	// Quarantined, not deleted: the bytes stay recoverable by hand.
	if _, err := os.Stat(walPath); !os.IsNotExist(err) {
		t.Fatalf("corrupt WAL was left in place to be replayed again: %v", err)
	}
	raw, err := os.ReadFile(walPath + ".quarantine")
	if err != nil {
		t.Fatalf("read quarantined WAL: %v", err)
	}
	if !bytes.Equal(raw, corrupt) {
		t.Fatalf("quarantined WAL = %q, want the original bytes", raw)
	}
	// The degraded marker is what keeps the next open from silently retrying.
	if _, err := os.Stat(filepath.Join(root, dirSessions, session.ID, cliReplyDegradedFile)); err != nil {
		t.Fatalf("degraded marker missing: %v", err)
	}
	if err := d.recoverCLIReplyTransaction(filepath.Join(root, dirSessions, session.ID)); !errors.Is(err, ErrCLIReplyRecoveryDegraded) {
		t.Fatalf("recovery after quarantine = %v, want ErrCLIReplyRecoveryDegraded", err)
	}
}
