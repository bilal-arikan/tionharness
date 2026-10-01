package db

import (
	"context"
	"testing"
)

func TestCLIReplyDoesNotSkipConcurrentWorkerNote(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	session, err := d.CreateSession(ctx, Session{AgentID: "A1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.AddMessage(ctx, Message{SessionID: session.ID, Role: "user", Text: "sent to CLI"}); err != nil {
		t.Fatal(err)
	}
	// This note arrived after the request was composed, before its reply.
	if _, err := d.AddMessage(ctx, Message{SessionID: session.ID, Role: "user", Origin: "worker-note", Text: "unseen failure"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.AddMessageWithCLIState(ctx, Message{SessionID: session.ID, Role: "assistant", Text: "answer"}, CLIReplyState{
		UpdateResume: true, ResumeSessionID: "thread-1", ResumeSentMsgCount: 2, ResumeInputMsgCount: 1,
	}); err != nil {
		t.Fatal(err)
	}
	stored, _ := d.GetSession(ctx, session.ID)
	if stored.CLISentMsgCount != 1 {
		t.Fatalf("unseen note skipped by cursor %d", stored.CLISentMsgCount)
	}
	messages, _ := d.ListMessages(ctx, session.ID)
	if messages[stored.CLISentMsgCount].Text != "unseen failure" {
		t.Fatal("next delta lost concurrent notification")
	}
}
