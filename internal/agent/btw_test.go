package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// TestBuildBtwRequest_NoTools locks the side chat's defining safety property: the
// request carries NO tools, so a btw question can never run a command or edit a
// file. This is enforced structurally (an empty Request.Tools), not by trusting the
// model to obey the prompt — so a future refactor that starts attaching the agent's
// tool set here must fail this test.
func TestBuildBtwRequest_NoTools(t *testing.T) {
	req := buildBtwRequest(db.Agent{Model: "m"}, "static", "dynamic", nil, "neden bu hata çıkıyor?")
	if len(req.Tools) != 0 {
		t.Fatalf("btw request carries %d tools, want 0 — the side chat must be read-only", len(req.Tools))
	}
}

// TestBuildBtwRequest_DoesNotMutateHistory guards the other half of the contract:
// the conversation must come back byte-identical, because the main turn's history
// is what the prompt cache is keyed on. A naive `append(history, question)` would
// scribble the btw question into the caller's backing array whenever it had spare
// capacity — silently corrupting the next real turn. The history slice below is
// built WITH spare capacity precisely to catch that.
func TestBuildBtwRequest_DoesNotMutateHistory(t *testing.T) {
	history := make([]providers.Message, 2, 8) // len 2, cap 8 → append() would reuse the array
	history[0] = providers.Message{Role: providers.RoleUser, Text: "ana görev"}
	history[1] = providers.Message{Role: providers.RoleAssistant, Text: "üzerinde çalışıyorum"}

	req := buildBtwRequest(db.Agent{Model: "m"}, "static", "dynamic", history, "yan soru")

	if len(history) != 2 {
		t.Fatalf("history length changed to %d, want 2", len(history))
	}
	if history[1].Text != "üzerinde çalışıyorum" {
		t.Errorf("history[1] mutated: %q", history[1].Text)
	}
	// The spare slot behind the caller's slice must be untouched too.
	if spare := history[:cap(history)][2]; spare.Text != "" {
		t.Errorf("btw question leaked into the caller's backing array: %q", spare.Text)
	}

	// The question rides ONLY on the request's own copy, as the final user message.
	if len(req.Messages) != 3 {
		t.Fatalf("request has %d messages, want 3 (2 history + 1 question)", len(req.Messages))
	}
	last := req.Messages[2]
	if last.Role != providers.RoleUser {
		t.Errorf("last message role = %q, want user", last.Role)
	}
	if !strings.Contains(last.Text, "yan soru") {
		t.Errorf("last message does not carry the question: %q", last.Text)
	}
}

// TestAskBtw_RejectsEmptyQuestion checks the guard fires before any provider call —
// an empty side question is a client bug, not something to spend a turn on.
func TestAskBtw_RejectsEmptyQuestion(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	_, err := rt.AskBtw(context.Background(), db.Agent{Model: "m"}, "SES1", "", "", nil, "   ")
	if err == nil {
		t.Fatal("expected AskBtw to reject an empty question")
	}
}

// TestKindBtw_IsAuxiliary pins the side chat OUT of the conversation call kinds.
// The btw prompt is a throwaway (its own system suffix + a question that never
// enters the history), so letting the cache-break probe track it would report a
// spurious "prompt changed" break on every side question.
func TestKindBtw_IsAuxiliary(t *testing.T) {
	if isConversationKind(KindBtw) {
		t.Error("KindBtw must not be a conversation kind — it would trigger false cache-break reports")
	}
}
