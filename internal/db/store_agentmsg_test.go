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

// TestValidateInboundPolicy: "" is the backward-compatible default (accept), the
// three known policies pass through, everything else is an error.
func TestValidateInboundPolicy(t *testing.T) {
	for in, want := range map[string]string{
		"":            InboundAccept,
		InboundAccept: InboundAccept,
		InboundHold:   InboundHold,
		InboundRefuse: InboundRefuse,
	} {
		got, err := ValidateInboundPolicy(in)
		if err != nil || got != want {
			t.Fatalf("ValidateInboundPolicy(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := ValidateInboundPolicy("ACCEPT"); err == nil {
		t.Fatal("an unknown policy must error, not fall back to accept")
	}
}

// TestResolveInboundPolicy: the session setting wins when set, otherwise the
// agent's, otherwise accept.
func TestResolveInboundPolicy(t *testing.T) {
	cases := []struct{ session, agent, want string }{
		{"", "", InboundAccept},
		{"", InboundHold, InboundHold},
		{InboundAccept, InboundRefuse, InboundAccept},
		{InboundRefuse, InboundAccept, InboundRefuse},
	}
	for _, c := range cases {
		got, err := ResolveInboundPolicy(c.session, c.agent)
		if err != nil || got != c.want {
			t.Fatalf("ResolveInboundPolicy(%q, %q) = %q, %v; want %q", c.session, c.agent, got, err, c.want)
		}
	}
	if _, err := ResolveInboundPolicy("nonsense", InboundAccept); err == nil {
		t.Fatal("an invalid session policy must error")
	}
}

// TestAgentMessageLifecycle covers the receipt store: create, list held, the
// held→terminal CAS (only one winner), and the accepted→dropped downgrade.
func TestAgentMessageLifecycle(t *testing.T) {
	d := openAgentMsgStore(t)
	ctx := context.Background()

	if _, err := d.CreateAgentMessage(ctx, AgentMessage{Status: "sent"}); err == nil {
		t.Fatal("an unknown delivery status must be rejected")
	}

	held, err := d.CreateAgentMessage(ctx, AgentMessage{
		FromAgentID: "AGT1", ToAgentID: "AGT2", Channel: ChannelInbox,
		Body: "wait", Policy: InboundHold, Status: DeliveryHeld,
	})
	if err != nil {
		t.Fatalf("create held: %v", err)
	}
	accepted, err := d.CreateAgentMessage(ctx, AgentMessage{
		FromAgentID: "AGT1", ToAgentID: "AGT3", Channel: ChannelWorker,
		Body: "go", Policy: InboundAccept, Status: DeliveryAccepted,
	})
	if err != nil {
		t.Fatalf("create accepted: %v", err)
	}

	list, err := d.ListHeldAgentMessages(ctx, "")
	if err != nil || len(list) != 1 || list[0].ID != held.ID {
		t.Fatalf("ListHeldAgentMessages = %+v, %v; want only the held one", list, err)
	}
	if list, _ := d.ListHeldAgentMessages(ctx, "AGT3"); len(list) != 0 {
		t.Fatalf("filtering by recipient must exclude other agents, got %+v", list)
	}
	sent, err := d.ListAgentMessagesFrom(ctx, "AGT1")
	if err != nil || len(sent) != 2 {
		t.Fatalf("ListAgentMessagesFrom = %d, %v; want 2", len(sent), err)
	}
	if _, err := d.ListAgentMessagesFrom(ctx, ""); err == nil {
		t.Fatal("an empty sender id must error rather than list everything")
	}

	if _, err := d.ResolveHeldAgentMessage(ctx, held.ID, "weird", ""); err == nil {
		t.Fatal("an unknown resolution status must be rejected")
	}
	got, err := d.ResolveHeldAgentMessage(ctx, held.ID, DeliveryAccepted, "released")
	if err != nil || got.Status != DeliveryAccepted || got.Reason != "released" {
		t.Fatalf("resolve = %+v, %v", got, err)
	}
	if _, err := d.ResolveHeldAgentMessage(ctx, held.ID, DeliveryRefused, "late"); err == nil {
		t.Fatal("resolving a non-held receipt must fail (CAS)")
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
	held, err := d.CreateAgentMessage(ctx, AgentMessage{
		ToAgentID: "AGT2", Channel: ChannelInbox, Body: "survive me",
		Policy: InboundHold, Status: DeliveryHeld,
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
	got, err := again.GetAgentMessage(ctx, held.ID)
	if err != nil {
		t.Fatalf("get after reopen: %v", err)
	}
	if got.Status != DeliveryHeld || got.Body != "survive me" {
		t.Fatalf("held receipt did not survive the reopen: %+v", got)
	}
}
