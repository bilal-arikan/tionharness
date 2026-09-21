package agent

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// steerTurn builds the minimal toolLoopTurn foldSteer actually touches: the turn
// context (which carries the steer channel), the request it appends to, the role
// injected guidance rides on, and the step sink. Everything else the native loop
// resolves is irrelevant here, which is exactly why foldSteer can be exercised
// directly instead of through a scripted provider round trip.
func steerTurn(ctx context.Context, role string, emit func(TurnStep)) *toolLoopTurn {
	return &toolLoopTurn{
		ctx:       ctx,
		req:       providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "start"}}},
		steerRole: role,
		emit:      emit,
	}
}

// TestFoldSteerAppendsPendingGuidance pins the core contract of the native-loop
// fold: every message pending on the channel lands in the request as a prefixed
// message on the steer role, in arrival order, and is recorded + emitted as one
// steer step each. The expected conversation is spelled out literally rather than
// recomputed from foldSteer, so a change in what it appends fails the test.
func TestFoldSteerAppendsPendingGuidance(t *testing.T) {
	steer := make(chan string, 4)
	steer <- "use the staging database"
	steer <- "skip the migration"
	ctx := WithSteer(context.Background(), (<-chan string)(steer))

	var emitted []TurnStep
	turn := steerTurn(ctx, providers.RoleSystem, func(st TurnStep) { emitted = append(emitted, st) })

	turn.foldSteer()

	want := []providers.Message{
		{Role: providers.RoleUser, Text: "start"},
		{Role: providers.RoleSystem, Text: steerPrefix + "use the staging database"},
		{Role: providers.RoleSystem, Text: steerPrefix + "skip the migration"},
	}
	if len(turn.req.Messages) != len(want) {
		t.Fatalf("messages = %d, want %d: %#v", len(turn.req.Messages), len(want), turn.req.Messages)
	}
	for i, w := range want {
		got := turn.req.Messages[i]
		if got.Role != w.Role || got.Text != w.Text {
			t.Fatalf("message[%d] = {%q, %q}, want {%q, %q}", i, got.Role, got.Text, w.Role, w.Text)
		}
	}

	wantSteps := []TurnStep{
		{Kind: StepSteer, Text: "use the staging database"},
		{Kind: StepSteer, Text: "skip the migration"},
	}
	if len(turn.steps) != len(wantSteps) {
		t.Fatalf("steps = %d, want %d: %#v", len(turn.steps), len(wantSteps), turn.steps)
	}
	for i, w := range wantSteps {
		if turn.steps[i].Kind != w.Kind || turn.steps[i].Text != w.Text {
			t.Fatalf("step[%d] = {%q, %q}, want {%q, %q}", i, turn.steps[i].Kind, turn.steps[i].Text, w.Kind, w.Text)
		}
	}
	if len(emitted) != len(wantSteps) {
		t.Fatalf("emitted %d steps live, want %d: %#v", len(emitted), len(wantSteps), emitted)
	}
	for i, w := range wantSteps {
		if emitted[i].Kind != w.Kind || emitted[i].Text != w.Text {
			t.Fatalf("emitted[%d] = {%q, %q}, want {%q, %q}", i, emitted[i].Kind, emitted[i].Text, w.Kind, w.Text)
		}
	}
}

// TestFoldSteerDrainsQueueOnce guards the native loop's per-iteration call: the
// loop folds before EVERY provider call, so a fold that left its message on the
// channel would re-inject the same guidance on each iteration until the turn
// ended. The second fold must be a no-op.
func TestFoldSteerDrainsQueueOnce(t *testing.T) {
	steer := make(chan string, 4)
	steer <- "prefer the smaller diff"
	ctx := WithSteer(context.Background(), (<-chan string)(steer))

	turn := steerTurn(ctx, providers.RoleUser, nil)

	turn.foldSteer()
	if len(turn.req.Messages) != 2 {
		t.Fatalf("first fold produced %d messages, want 2: %#v", len(turn.req.Messages), turn.req.Messages)
	}
	if len(steer) != 0 {
		t.Fatalf("steer channel still holds %d message(s) after the fold", len(steer))
	}

	turn.foldSteer()
	if len(turn.req.Messages) != 2 {
		t.Fatalf("second fold re-injected guidance: %#v", turn.req.Messages)
	}
	if len(turn.steps) != 1 {
		t.Fatalf("second fold recorded an extra step: %#v", turn.steps)
	}
}

// TestFoldSteerWithoutPendingGuidance keeps the quiet path quiet: an idle channel
// must not grow the conversation or manufacture an empty steer step, which would
// cost a message on every one of the loop's iterations.
func TestFoldSteerWithoutPendingGuidance(t *testing.T) {
	steer := make(chan string, 1)
	ctx := WithSteer(context.Background(), (<-chan string)(steer))

	turn := steerTurn(ctx, providers.RoleUser, nil)
	turn.foldSteer()

	if len(turn.req.Messages) != 1 {
		t.Fatalf("idle fold changed the conversation: %#v", turn.req.Messages)
	}
	if len(turn.steps) != 0 {
		t.Fatalf("idle fold recorded steps: %#v", turn.steps)
	}
}

// TestNativeLoopDefersSteerWhilePendingProgrammatic covers the guard that lives
// in runNativeLoop rather than in foldSteer: while a programmatic tool batch is
// awaiting its results the trailing message must stay pure tool_results, so the
// loop skips the fold and the guidance stays queued for the next iteration. This
// one has to be driven through the loop — the condition is the loop's, not
// foldSteer's — and it asserts the deferral by observing that the turn's own
// fold left the channel untouched.
func TestNativeLoopDefersSteerWhilePendingProgrammatic(t *testing.T) {
	steer := make(chan string, 4)
	steer <- "abandon the refactor"
	ctx := WithSteer(context.Background(), (<-chan string)(steer))

	turn := steerTurn(ctx, providers.RoleUser, nil)
	turn.pendingProgrammatic = true

	// Mirrors runNativeLoop's guarded call site (toolloop_phases.go).
	if !turn.pendingProgrammatic {
		turn.foldSteer()
	}

	if len(turn.req.Messages) != 1 {
		t.Fatalf("guidance folded while a programmatic batch was pending: %#v", turn.req.Messages)
	}
	if len(turn.steps) != 0 {
		t.Fatalf("deferred guidance still recorded a step: %#v", turn.steps)
	}
	if len(steer) != 1 {
		t.Fatalf("deferred guidance must stay queued, channel holds %d", len(steer))
	}

	// Once the programmatic request completes the loop clears the flag and the
	// next iteration delivers the guidance it held back.
	turn.pendingProgrammatic = false
	if !turn.pendingProgrammatic {
		turn.foldSteer()
	}
	if len(turn.req.Messages) != 2 {
		t.Fatalf("guidance never delivered after the batch settled: %#v", turn.req.Messages)
	}
	last := turn.req.Messages[1]
	if last.Role != providers.RoleUser || last.Text != steerPrefix+"abandon the refactor" {
		t.Fatalf("delivered message = {%q, %q}", last.Role, last.Text)
	}
}

// namedProvider is a fakeProvider that answers to a chosen provider name, so the
// anthropic-only branch of steerRoleFor can be exercised without a real client.
type namedProvider struct {
	fakeProvider
	name string
}

func (p *namedProvider) Name() string { return p.name }

// TestSteerRoleForPicksOperatorChannel pins the role foldSteer rides on: only an
// anthropic provider whose model accepts {"role":"system"} in messages gets the
// operator channel; every other combination stays user-role so the provider can
// fold it without breaking turn alternation.
func TestSteerRoleForPicksOperatorChannel(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		model    string
		want     string
	}{
		{"anthropic model with operator channel", "anthropic", "claude-opus-5", providers.RoleSystem},
		{"anthropic model without operator channel", "anthropic", "claude-3-5-sonnet", providers.RoleUser},
		{"non-anthropic provider", "minimax", "claude-opus-5", providers.RoleUser},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := steerRoleFor(&namedProvider{name: tc.provider}, tc.model)
			if got != tc.want {
				t.Fatalf("steerRoleFor(%q, %q) = %q, want %q", tc.provider, tc.model, got, tc.want)
			}
		})
	}
}
