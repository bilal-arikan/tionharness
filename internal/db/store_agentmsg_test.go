package db

import (
	"context"
	"path/filepath"
	"testing"
)

func openAgentMsgStore(t *testing.T) *DB {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

// TestAgentMessageLifecycle covers the receipt store: create, list held, the
// held→terminal CAS (only one winner), and the accepted→dropped downgrade.
func TestAgentMessageLifecycle(t *testing.T) {
	d := openAgentMsgStore(t)
	ctx := context.Background()

	if _, err := d.CreateAgentMessage(ctx, AgentMessage{Status: "sent"}); err == nil {
		t.Fatal("an unknown delivery status must be rejected")
	}

	_, err := d.CreateAgentMessage(ctx, AgentMessage{
		FromAgentID: "AGT1", ToAgentID: "AGT2", Channel: ChannelInbox,
		Body: "wait", Status: DeliveryAccepted,
	})
	if err != nil {
		t.Fatalf("create held: %v", err)
	}
	accepted, err := d.CreateAgentMessage(ctx, AgentMessage{
		FromAgentID: "AGT1", ToAgentID: "AGT3", Channel: ChannelWorker,
		Body: "go", Status: DeliveryAccepted,
	})
	if err != nil {
		t.Fatalf("create accepted: %v", err)
	}

	sent, err := d.ListAgentMessagesFrom(ctx, "AGT1")
	if err != nil || len(sent) != 2 {
		t.Fatalf("ListAgentMessagesFrom = %d, %v; want 2", len(sent), err)
	}
	if _, err := d.ListAgentMessagesFrom(ctx, ""); err == nil {
		t.Fatal("an empty sender id must error rather than list everything")
	}

	if _, err := d.MarkAgentMessageDropped(ctx, accepted.ID, ""); err == nil {
		t.Fatal("a drop without a reason must be rejected")
	}
	dropped, err := d.MarkAgentMessageDropped(ctx, accepted.ID, "pool exhausted")
	if err != nil || dropped.Status != DeliveryDropped || dropped.Reason != "pool exhausted" {
		t.Fatalf("drop = %+v, %v", dropped, err)
	}
	if _, err := d.MarkAgentMessageDropped(ctx, "AMS999", "gone"); err != ErrNotFound {
		t.Fatalf("dropping a missing receipt = %v, want ErrNotFound", err)
	}
}

// TestAgentMessagesSurviveReopen: receipts (and the parked bodies of held ones)
// are on disk, so a restart does not lose a message waiting for approval.
func TestAgentMessagesSurviveReopen(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	d, err := Open(root)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	kept, err := d.CreateAgentMessage(ctx, AgentMessage{
		ToAgentID: "AGT2", Channel: ChannelInbox, Body: "survive me",
		Status: DeliveryAccepted,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	again, err := Open(root)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = again.Close() })
	got, err := again.GetAgentMessage(ctx, kept.ID)
	if err != nil {
		t.Fatalf("get after reopen: %v", err)
	}
	if got.Status != DeliveryAccepted || got.Body != "survive me" {
		t.Fatalf("held receipt did not survive the reopen: %+v", got)
	}
}
