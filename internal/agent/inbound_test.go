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

// TestInboundPolicyHoldParksMessage: a "hold" recipient neither receives the
// message nor loses it — the body is parked in a durable held receipt and the
// sender is told so explicitly.
func TestInboundPolicyHoldParksMessage(t *testing.T) {
	rt, from, to := inboundFixture(t)
	ctx := context.Background()
	if _, err := rt.db.UpdateAgent(ctx, to.ID, db.AgentProfilePatch{InboundPolicy: strptr(db.InboundHold)}); err != nil {
		t.Fatalf("set hold policy: %v", err)
	}
	to, _ = rt.db.GetAgent(ctx, to.ID)

	receipt, err := rt.deliverOne(ctx, from.ID, "Sender", to, "sum", "hold me")
	if err != nil {
		t.Fatalf("hold delivery should not error: %v", err)
	}
	if receipt.Status != db.DeliveryHeld || receipt.Policy != db.InboundHold {
		t.Fatalf("want held receipt, got %+v", receipt)
	}
	if receipt.Body != "hold me" {
		t.Fatalf("held receipt must keep the body, got %q", receipt.Body)
	}
	held, err := rt.ListHeldMessages(ctx, to.ID)
	if err != nil || len(held) != 1 {
		t.Fatalf("ListHeldMessages = %d, %v; want 1 held message", len(held), err)
	}
	if notice := heldNotice(receipt); !strings.Contains(notice, "HELD") || !strings.Contains(notice, receipt.ID) {
		t.Fatalf("held notice must name the state and the receipt: %q", notice)
	}
}

// TestInboundPolicyRefuseFails: a "refuse" recipient rejects the delivery with an
// error, and the refusal is still recorded (no silent drop).
func TestInboundPolicyRefuseFails(t *testing.T) {
	rt, from, to := inboundFixture(t)
	ctx := context.Background()
	if _, err := rt.db.UpdateAgent(ctx, to.ID, db.AgentProfilePatch{InboundPolicy: strptr(db.InboundRefuse)}); err != nil {
		t.Fatalf("set refuse policy: %v", err)
	}
	to, _ = rt.db.GetAgent(ctx, to.ID)

	if _, err := rt.deliverOne(ctx, from.ID, "Sender", to, "", "nope"); err == nil {
		t.Fatal("refuse policy must fail the delivery")
	}
	sent, err := rt.db.ListAgentMessagesFrom(ctx, from.ID)
	if err != nil {
		t.Fatalf("list receipts: %v", err)
	}
	if len(sent) != 1 || sent[0].Status != db.DeliveryRefused {
		t.Fatalf("want one refused receipt, got %+v", sent)
	}
	if sent[0].Reason == "" {
		t.Fatal("a refused receipt must carry a reason")
	}
}

// TestInboundPolicyDefaultIsAccept: an agent with no policy set behaves exactly
// as before this feature existed — the delivery goes through.
func TestInboundPolicyDefaultIsAccept(t *testing.T) {
	rt, _, to := inboundFixture(t)
	ctx := context.Background()
	policy, err := rt.inboundPolicyFor(ctx, to.ID, "")
	if err != nil {
		t.Fatalf("resolve policy: %v", err)
	}
	if policy != db.InboundAccept {
		t.Fatalf("unset policy = %q, want %q", policy, db.InboundAccept)
	}
}

// TestInboundPolicyInvalidValueErrors: a stored policy value outside the three
// known ones fails the delivery loudly instead of falling back to accept.
func TestInboundPolicyInvalidValueErrors(t *testing.T) {
	rt, _, to := inboundFixture(t)
	ctx := context.Background()
	// Write it behind UpdateAgent's validation to simulate a hand-edited row.
	if _, err := rt.db.UpdateAgent(ctx, to.ID, db.AgentProfilePatch{InboundPolicy: strptr("maybe")}); err == nil {
		t.Fatal("UpdateAgent must reject an unknown inbound policy")
	}
	if _, err := db.ValidateInboundPolicy("maybe"); err == nil {
		t.Fatal("ValidateInboundPolicy must reject an unknown value")
	}
}

// TestSessionPolicyOverridesAgent: a per-session policy wins over the agent's.
func TestSessionPolicyOverridesAgent(t *testing.T) {
	rt, _, to := inboundFixture(t)
	ctx := context.Background()
	if _, err := rt.db.UpdateAgent(ctx, to.ID, db.AgentProfilePatch{InboundPolicy: strptr(db.InboundRefuse)}); err != nil {
		t.Fatalf("set refuse policy: %v", err)
	}
	sess, err := rt.db.CreateSession(ctx, db.Session{AgentID: to.ID, Kind: "worker", SourceID: "s:w"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := rt.db.SetSessionInboundPolicy(ctx, sess.ID, db.InboundAccept); err != nil {
		t.Fatalf("set session policy: %v", err)
	}
	policy, err := rt.inboundPolicyFor(ctx, to.ID, sess.ID)
	if err != nil {
		t.Fatalf("resolve policy: %v", err)
	}
	if policy != db.InboundAccept {
		t.Fatalf("session override = %q, want %q", policy, db.InboundAccept)
	}
	if err := rt.db.SetSessionInboundPolicy(ctx, sess.ID, "sometimes"); err == nil {
		t.Fatal("an unknown session policy must be rejected")
	}
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
func TestSendToWorkerEnforcesSizeAndPolicy(t *testing.T) {
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

	// A holding worker session parks the follow-up instead of running a turn.
	if err := rt.db.SetSessionInboundPolicy(ctx, workerID, db.InboundHold); err != nil {
		t.Fatalf("set session hold: %v", err)
	}
	res, err = rt.SendToWorker(ctx, coordID, workerID, "and this")
	if err != nil {
		t.Fatalf("hold must not error: %v", err)
	}
	if !res.Held || res.Delivered || res.ReceiptID == "" {
		t.Fatalf("want a held result, got %+v", res)
	}
	held, _ := rt.ListHeldMessages(ctx, "")
	if len(held) != 1 || held[0].Body != "and this" {
		t.Fatalf("follow-up not parked, got %+v", held)
	}
}

// TestReleaseHeldMessageDeliversOnce: approving a held worker message delivers it
// and a second release loses the CAS, so the same message can never run twice.
func TestReleaseHeldMessageDeliversOnce(t *testing.T) {
	rt, coordID, workerID, delivered := queueTestFixture(t)
	ctx := context.Background()
	if err := rt.db.SetSessionInboundPolicy(ctx, workerID, db.InboundHold); err != nil {
		t.Fatalf("set session hold: %v", err)
	}
	res, err := rt.SendToWorker(ctx, coordID, workerID, "held work")
	if err != nil || !res.Held {
		t.Fatalf("expected a hold, got %+v (%v)", res, err)
	}

	receipt, err := rt.ReleaseHeldMessage(ctx, res.ReceiptID)
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if receipt.Status != db.DeliveryAccepted {
		t.Fatalf("released receipt = %q, want accepted", receipt.Status)
	}
	if msg, ok := delivered(); !ok || msg != "held work" {
		t.Fatalf("released message not delivered, got %q ok=%v", msg, ok)
	}
	if _, err := rt.ReleaseHeldMessage(ctx, res.ReceiptID); err == nil {
		t.Fatal("releasing the same message twice must fail")
	}
}

// TestRefuseHeldMessage: rejecting a parked message records the reason and keeps
// the body, and the message is no longer in the hold queue.
func TestRefuseHeldMessage(t *testing.T) {
	rt, coordID, workerID, _ := queueTestFixture(t)
	ctx := context.Background()
	if err := rt.db.SetSessionInboundPolicy(ctx, workerID, db.InboundHold); err != nil {
		t.Fatalf("set session hold: %v", err)
	}
	res, err := rt.SendToWorker(ctx, coordID, workerID, "unwanted")
	if err != nil || !res.Held {
		t.Fatalf("expected a hold, got %+v (%v)", res, err)
	}
	receipt, err := rt.RefuseHeldMessage(ctx, res.ReceiptID, "out of scope")
	if err != nil {
		t.Fatalf("refuse: %v", err)
	}
	if receipt.Status != db.DeliveryRefused || receipt.Reason != "out of scope" {
		t.Fatalf("want a refused receipt with the reason, got %+v", receipt)
	}
	if receipt.Body != "unwanted" {
		t.Fatalf("refusal must keep the body for audit, got %q", receipt.Body)
	}
	held, _ := rt.ListHeldMessages(ctx, "")
	if len(held) != 0 {
		t.Fatalf("refused message must leave the hold queue, got %+v", held)
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
		Body: "x", Policy: db.InboundAccept, Status: db.DeliveryAccepted,
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

func strptr(s string) *string { return &s }
