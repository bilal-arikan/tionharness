package agent

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// inboundFixture builds a runtime with a sender and a recipient agent.
func inboundFixture(t *testing.T) (*Runtime, db.Agent, db.Agent) {
	t.Helper()
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	from, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Sender", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create sender: %v", err)
	}
	to, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Recipient", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create recipient: %v", err)
	}
	return rt, from, to
}

// TestMessageTooLarge: over the configured byte cap, send_message and
// send_to_worker both fail with an explicit message_too_large error.
func TestMessageTooLarge(t *testing.T) {
	rt, from, to := inboundFixture(t)
	ctx := context.Background()
	rt.tun.SetAgentMessageMaxBytes(32)
	big := strings.Repeat("x", 33)

	_, err := rt.deliverOne(ctx, from.ID, "Sender", to, "", big)
	if !errors.Is(err, ErrMessageTooLarge) {
		t.Fatalf("send_message over the cap = %v, want ErrMessageTooLarge", err)
	}
	if !strings.Contains(err.Error(), "message_too_large") || !strings.Contains(err.Error(), "send_message") {
		t.Fatalf("error must name the failure and the path: %v", err)
	}
	// Nothing is recorded as delivered for a message that never passed the gate.
	sent, _ := rt.db.ListAgentMessagesFrom(ctx, from.ID)
	if len(sent) != 0 {
		t.Fatalf("oversized message must not produce a delivery receipt, got %+v", sent)
	}
	// Exactly at the cap is allowed.
	if err := rt.checkMessageSize("send_message", strings.Repeat("x", 32)); err != nil {
		t.Fatalf("a message exactly at the cap must pass: %v", err)
	}
}

// TestSendToWorkerEnforcesSizeAndPolicy covers the second path: the coordinator's
// follow-up is size-checked and policy-gated before it can take a queue slot.
func TestSendToWorkerEnforcesSize(t *testing.T) {
	rt, coordID, workerID, delivered := queueTestFixture(t)
	ctx := context.Background()

	rt.tun.SetAgentMessageMaxBytes(16)
	if _, err := rt.SendToWorker(ctx, coordID, workerID, strings.Repeat("y", 17)); !errors.Is(err, ErrMessageTooLarge) {
		t.Fatalf("send_to_worker over the cap = %v, want ErrMessageTooLarge", err)
	}
	rt.tun.SetAgentMessageMaxBytes(0) // back to the default

	// Accept (default) still delivers, and now returns a receipt id.
	res, err := rt.SendToWorker(ctx, coordID, workerID, "do it")
	if err != nil {
		t.Fatalf("accept delivery: %v", err)
	}
	if !res.Delivered || res.ReceiptID == "" {
		t.Fatalf("want a delivered result with a receipt, got %+v", res)
	}
	if msg, ok := delivered(); !ok || msg != "do it" {
		t.Fatalf("worker did not receive the message, got %q ok=%v", msg, ok)
	}

}

// TestDropDeliveryRecordsFailure: a delivery that fails AFTER being accepted is
// downgraded to "dropped" with the cause, so the sender can tell the difference
// between "delivered" and "accepted then lost".
func TestDropDeliveryRecordsFailure(t *testing.T) {
	rt, from, to := inboundFixture(t)
	ctx := context.Background()
	receipt, err := rt.db.CreateAgentMessage(ctx, db.AgentMessage{
		FromAgentID: from.ID, ToAgentID: to.ID, Channel: db.ChannelInbox,
		Body: "x", Status: db.DeliveryAccepted,
	})
	if err != nil {
		t.Fatalf("create receipt: %v", err)
	}
	err = rt.dropDelivery(ctx, receipt.ID, errors.New("pool exhausted"))
	if err == nil || !strings.Contains(err.Error(), "pool exhausted") || !strings.Contains(err.Error(), db.DeliveryDropped) {
		t.Fatalf("drop error must keep the cause and name the status: %v", err)
	}
	stored, err := rt.db.GetAgentMessage(ctx, receipt.ID)
	if err != nil {
		t.Fatalf("get receipt: %v", err)
	}
	if stored.Status != db.DeliveryDropped || stored.Reason == "" {
		t.Fatalf("want a dropped receipt with a reason, got %+v", stored)
	}
}

// TestCapNotification: the one-way worker→coordinator path is capped rather than
// refused, and says so with the same message_too_large marker. The cut is
// rune-safe (no broken UTF-8 on Turkish text).
func TestCapNotification(t *testing.T) {
	if got, cut := capNotification("short", 100); cut || got != "short" {
		t.Fatalf("under the cap must pass through unchanged, got %q cut=%v", got, cut)
	}
	note := strings.Repeat("ş", 40) // 80 bytes, 40 runes
	got, cut := capNotification(note, 31)
	if !cut {
		t.Fatal("over the cap must be capped")
	}
	if !strings.Contains(got, "message_too_large") {
		t.Fatalf("capped note must state why: %q", got)
	}
	head := strings.Split(got, "\n\n[message_too_large")[0]
	if !utf8.ValidString(head) {
		t.Fatalf("cap must not split a multi-byte rune: %q", head)
	}
	if len(head) > 31 {
		t.Fatalf("capped head is %d bytes, want <= 31", len(head))
	}
}
