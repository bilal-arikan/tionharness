package db

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSetSessionCLICompactBoundaryWriteFailureDoesNotAdvanceMemory(t *testing.T) {
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
	header := d.dir(dirSessions, session.ID, sessionHeaderFile)
	if err := os.Remove(header); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(header, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := d.SetSessionCLICompactBoundary(ctx, session.ID, 42); err == nil {
		t.Fatal("boundary write unexpectedly succeeded")
	}
	got, err := d.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CLICompactMsgCount != 0 {
		t.Fatalf("CLICompactMsgCount = %d after failed write, want 0", got.CLICompactMsgCount)
	}
}

func TestAddMessageWithCLIStateFailureIsNotPublishedBeforeRecovery(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	d, err := Open(root)
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
	d.cliReplyTxnHook = func(phase cliReplyTxnPhase) error {
		if phase == cliReplyTxnMessage {
			return errors.New("injected process crash")
		}
		return nil
	}
	_, err = d.AddMessageWithCLIState(ctx, Message{
		SessionID: session.ID, Role: "assistant", Text: "must recover",
	}, CLIReplyState{
		UpdateResume: true, ResumeSessionID: "raw-cli-session", ResumeSentMsgCount: 2,
		UpdateCompactBoundary: true, CompactMsgCount: 1,
	})
	if err == nil {
		t.Fatal("interrupted reply write unexpectedly succeeded")
	}
	messages, _, err := d.ListMessagesTail(ctx, session.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("messages = %+v, interrupted write published reply in memory", messages)
	}
	got, err := d.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CLICompactMsgCount != 0 || got.CLISentMsgCount != 0 || got.CLISessionID != "" {
		t.Fatalf("CLI state advanced before recovery: %+v", got)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := reopened.ListMessages(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 || recovered[0].Text != "must recover" {
		t.Fatalf("recovered messages = %+v", recovered)
	}
	got, err = reopened.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CLISessionID != "raw-cli-session" || got.CLISentMsgCount != 2 || got.CLICompactMsgCount != 1 {
		t.Fatalf("recovered CLI state = %+v", got)
	}
}

func TestSetSessionCLICompactionStateFailureIsAtomic(t *testing.T) {
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
	if _, err := d.AddMessage(ctx, Message{SessionID: session.ID, Role: "user", Text: "existing transcript"}); err != nil {
		t.Fatal(err)
	}
	if err := d.SetSessionCLICompactionState(ctx, session.ID, "old-resume", 1); err != nil {
		t.Fatal(err)
	}
	if err := d.BeginSessionCLINativeCompaction(ctx, session.ID); err != nil {
		t.Fatal(err)
	}

	headerPath := d.dir(dirSessions, session.ID, sessionHeaderFile)
	messagePath := d.dir(dirSessions, session.ID, sessionMsgsFile)
	headerBefore, err := os.ReadFile(headerPath)
	if err != nil {
		t.Fatal(err)
	}
	messagesBefore, err := os.ReadFile(messagePath)
	if err != nil {
		t.Fatal(err)
	}
	// Block atomicWriteBytes at its temp-file stage while leaving the durable
	// header in place. This models the second old persistence call failing without
	// destroying the state needed to prove rollback.
	if err := os.Mkdir(headerPath+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := d.SetSessionCLICompactionState(ctx, session.ID, "new-resume", 7); err == nil {
		t.Fatal("atomic CLI compaction state write unexpectedly succeeded")
	}

	got, err := d.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CLISessionID != "old-resume" || got.CLISentMsgCount != 1 || got.CLICompactMsgCount != 1 || !got.CLINativeCompactionPending {
		t.Fatalf("CLI compaction state advanced after failed write: %+v", got)
	}
	headerAfter, err := os.ReadFile(headerPath)
	if err != nil {
		t.Fatal(err)
	}
	messagesAfter, err := os.ReadFile(messagePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(headerAfter, headerBefore) {
		t.Fatal("failed CLI compaction state write changed durable session header")
	}
	if !bytes.Equal(messagesAfter, messagesBefore) {
		t.Fatal("failed CLI compaction state write changed transcript")
	}
}
