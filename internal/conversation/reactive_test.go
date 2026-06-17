package conversation

import (
	"context"
	"testing"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
)

// stubProvider returns a fixed summary text for any completion.
type stubProvider struct{ summary string }

func (s stubProvider) Name() string { return "stub" }
func (s stubProvider) Complete(_ context.Context, _ providers.Request) (*providers.Response, error) {
	return &providers.Response{Text: s.summary, StopReason: providers.StopEndTurn}, nil
}

func msg(role, text string) providers.Message {
	return providers.Message{Role: role, Text: text}
}

func TestCompactInFlightMessages_FoldsAtAssistantBoundary(t *testing.T) {
	// 10 alternating turns; keepRecent=2 keeps the last two. The fold boundary
	// must land on an assistant message so the summary(user) → assistant tail
	// preserves role alternation.
	msgs := []providers.Message{
		msg(providers.RoleUser, "u1"), msg(providers.RoleAssistant, "a1"),
		msg(providers.RoleUser, "u2"), msg(providers.RoleAssistant, "a2"),
		msg(providers.RoleUser, "u3"), msg(providers.RoleAssistant, "a3"),
		msg(providers.RoleUser, "u4"), msg(providers.RoleAssistant, "a4"),
		msg(providers.RoleUser, "u5"), msg(providers.RoleAssistant, "a5"),
	}
	out, ok, err := CompactInFlightMessages(context.Background(), nil, stubProvider{summary: "SUM"}, db.Agent{}, msgs, 2)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v, want ok=true", ok, err)
	}
	if out[0].Role != providers.RoleUser {
		t.Errorf("first message role = %q, want user (summary)", out[0].Role)
	}
	if out[1].Role != providers.RoleAssistant {
		t.Errorf("tail starts with %q, want assistant (alternation preserved)", out[1].Role)
	}
	// Tail must be shorter than the input (something was folded).
	if len(out) >= len(msgs) {
		t.Errorf("len(out)=%d not shorter than len(msgs)=%d", len(out), len(msgs))
	}
}

func TestCompactInFlightMessages_NoSafeBoundary(t *testing.T) {
	// Too short to fold under keepRecent → no-op, ok=false, slice unchanged.
	msgs := []providers.Message{msg(providers.RoleUser, "u1"), msg(providers.RoleAssistant, "a1")}
	out, ok, err := CompactInFlightMessages(context.Background(), nil, stubProvider{summary: "SUM"}, db.Agent{}, msgs, reactiveKeepRecentTest)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if ok {
		t.Errorf("ok=true, want false (nothing safe to fold)")
	}
	if len(out) != len(msgs) {
		t.Errorf("len(out)=%d, want %d (unchanged)", len(out), len(msgs))
	}
}

const reactiveKeepRecentTest = 6
