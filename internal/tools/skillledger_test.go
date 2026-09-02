package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// stubSkillLib serves one fixed body and counts how often it was read, so a test
// can prove the dedupe path never touches the library.
type stubSkillLib struct {
	body  string
	reads int
	err   error
}

func (s *stubSkillLib) Body(string) (string, error) {
	s.reads++
	if s.err != nil {
		return "", s.err
	}
	return s.body, nil
}

func (s *stubSkillLib) AllowedTools(string) []string { return nil }

func callSkill(t *testing.T, ctx context.Context, tool UseSkillTool, slug string, force bool) string {
	t.Helper()
	in, err := json.Marshal(map[string]any{"slug": slug, "force": force})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	out, err := tool.Call(ctx, in)
	if err != nil {
		t.Fatalf("use_skill(%s, force=%v): %v", slug, force, err)
	}
	return out
}

// TestUseSkillDedupesWithinEpoch is the core saving: the second load of a slug in
// the same session returns a pointer, not the body, and never re-reads the file.
func TestUseSkillDedupesWithinEpoch(t *testing.T) {
	lib := &stubSkillLib{body: "STEP ONE: do the thing."}
	tool := NewUseSkillTool(lib)
	ctx := WithSkillLedger(context.Background(), NewSkillLedger(), 0)

	first := callSkill(t, ctx, tool, "doctrine", false)
	if !strings.Contains(first, lib.body) {
		t.Fatalf("first load must carry the body, got %q", first)
	}
	second := callSkill(t, ctx, tool, "doctrine", false)
	if strings.Contains(second, lib.body) {
		t.Fatalf("second load must NOT resend the body, got %q", second)
	}
	if !strings.Contains(second, "already loaded") || !strings.Contains(second, "force=true") {
		t.Fatalf("pointer must name the state and the escape hatch, got %q", second)
	}
	if lib.reads != 1 {
		t.Fatalf("library reads = %d, want 1 (the dedupe must short-circuit before the read)", lib.reads)
	}
}

// TestUseSkillFoldEpochResendsBody guards the correctness half: a rolling-summary
// fold drops the earlier body out of the window, so the next load must serve it.
func TestUseSkillFoldEpochResendsBody(t *testing.T) {
	lib := &stubSkillLib{body: "STEP ONE: do the thing."}
	tool := NewUseSkillTool(lib)
	ledger := NewSkillLedger()

	callSkill(t, WithSkillLedger(context.Background(), ledger, 0), tool, "doctrine", false)
	afterFold := callSkill(t, WithSkillLedger(context.Background(), ledger, 1), tool, "doctrine", false)
	if !strings.Contains(afterFold, lib.body) {
		t.Fatalf("load after a fold must resend the body, got %q", afterFold)
	}
}

// TestUseSkillForceResendsAndRearms: force returns the body again, and the load
// after it dedupes against that fresh copy.
func TestUseSkillForceResendsAndRearms(t *testing.T) {
	lib := &stubSkillLib{body: "STEP ONE: do the thing."}
	tool := NewUseSkillTool(lib)
	ctx := WithSkillLedger(context.Background(), NewSkillLedger(), 0)

	callSkill(t, ctx, tool, "doctrine", false)
	forced := callSkill(t, ctx, tool, "doctrine", true)
	if !strings.Contains(forced, lib.body) {
		t.Fatalf("force must resend the body, got %q", forced)
	}
	after := callSkill(t, ctx, tool, "doctrine", false)
	if strings.Contains(after, lib.body) {
		t.Fatalf("load after a force must dedupe again, got %q", after)
	}
}

// TestUseSkillWithoutLedgerAlwaysSendsBody keeps the pre-ledger behaviour for
// callers (and tests) that wire no ledger onto the context.
func TestUseSkillWithoutLedgerAlwaysSendsBody(t *testing.T) {
	lib := &stubSkillLib{body: "STEP ONE: do the thing."}
	tool := NewUseSkillTool(lib)
	for i := 0; i < 3; i++ {
		if out := callSkill(t, context.Background(), tool, "doctrine", false); !strings.Contains(out, lib.body) {
			t.Fatalf("load %d without a ledger must carry the body, got %q", i, out)
		}
	}
	if lib.reads != 3 {
		t.Fatalf("library reads = %d, want 3", lib.reads)
	}
}

// TestSkillLedgerNoteOrdinal pins the ordinal the pointer text quotes.
func TestSkillLedgerNoteOrdinal(t *testing.T) {
	l := NewSkillLedger()
	if already, ord := l.Note("a", 0); already || ord != 1 {
		t.Fatalf("first Note(a) = (%v, %d), want (false, 1)", already, ord)
	}
	if already, ord := l.Note("b", 0); already || ord != 2 {
		t.Fatalf("first Note(b) = (%v, %d), want (false, 2)", already, ord)
	}
	if already, ord := l.Note("a", 0); !already || ord != 1 {
		t.Fatalf("repeat Note(a) = (%v, %d), want (true, 1)", already, ord)
	}
}
