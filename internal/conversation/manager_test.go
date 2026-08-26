package conversation

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

func msgsN(n int) []db.Message {
	out := make([]db.Message, n)
	for i := range out {
		out[i] = db.Message{Role: providers.RoleUser, Text: "m"}
	}
	return out
}

// TestClampStart guards the defensive cap against a history shorter than the
// recorded SummaryMsgCount (e.g. after message edits).
func TestClampStart(t *testing.T) {
	if got := clampStart(5, 10); got != 5 {
		t.Errorf("clampStart(5,10) = %d, want 5", got)
	}
	if got := clampStart(20, 10); got != 10 {
		t.Errorf("clampStart(20,10) = %d, want 10 (clamped)", got)
	}
}

// TestFoldBoundary verifies the shared compaction split used by both Prepare and
// ForceCompact: fold = all but the newest keepRecent, keepTail = the rest, and ok
// is false when there is not enough pending history to fold.
func TestFoldBoundary(t *testing.T) {
	keepRecent := 3

	// 10 messages, none summarized → fold 7, keep 3, newCount 7.
	fold, keep, newCount, ok := foldBoundary(msgsN(10), 0, keepRecent)
	if !ok || len(fold) != 7 || len(keep) != 3 || newCount != 7 {
		t.Errorf("fresh: fold=%d keep=%d newCount=%d ok=%v, want 7/3/7/true", len(fold), len(keep), newCount, ok)
	}

	// 3 already summarized of 10 → pending 7, fold 4, keep 3, newCount 7.
	fold, keep, newCount, ok = foldBoundary(msgsN(10), 3, keepRecent)
	if !ok || len(fold) != 4 || len(keep) != 3 || newCount != 7 {
		t.Errorf("offset: fold=%d keep=%d newCount=%d ok=%v, want 4/3/7/true", len(fold), len(keep), newCount, ok)
	}

	// Exactly / fewer than keepRecent pending → nothing to fold.
	if _, _, _, ok := foldBoundary(msgsN(3), 0, keepRecent); ok {
		t.Error("pending == keepRecent should not fold")
	}
	if _, _, _, ok := foldBoundary(msgsN(2), 0, keepRecent); ok {
		t.Error("pending < keepRecent should not fold")
	}
	// Start beyond history is clamped → no fold, no panic.
	if _, _, _, ok := foldBoundary(msgsN(5), 99, keepRecent); ok {
		t.Error("start beyond history should not fold")
	}
}

// TestPrepareComputesPressure checks the memory-pressure ratio: ContextTokens /
// maxTokens, with a small budget so a couple of messages push it high. Few
// enough messages (<= keepRecent) that no compaction fires, so the stub DB/
// provider are never touched.
func TestPrepareComputesPressure(t *testing.T) {
	m := NewManager()
	m.SetLimits(50, 8) // tiny budget, keepRecent high enough to skip compaction

	history := []db.Message{
		{Role: providers.RoleUser, Text: "hello there, this is a fairly long message"},
		{Role: providers.RoleAssistant, Text: "and here is a long reply to consume tokens"},
	}
	prep, err := m.Prepare(context.Background(), nil, nil, db.Session{}, db.Agent{}, history)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if prep.Compacted {
		t.Fatalf("did not expect compaction with %d messages under keepRecent", len(history))
	}
	if prep.Pressure <= 0 {
		t.Fatalf("pressure = %f, want > 0", prep.Pressure)
	}
	// Sanity: pressure must equal ContextTokens/maxTokens.
	want := float64(prep.ContextTokens) / 50.0
	if prep.Pressure != want {
		t.Fatalf("pressure = %f, want %f", prep.Pressure, want)
	}
}

// TestPrepareOverheadRaisesPressure verifies the non-message overhead threaded via
// WithContextOverhead is added to the fold/pressure footprint: two identical runs
// differ only by the overhead, and the overhead one must report exactly
// (contextTokens + overhead) / maxTokens. keepRecent is high enough that neither
// run folds, so the stub DB/provider are never touched.
func TestPrepareOverheadRaisesPressure(t *testing.T) {
	m := NewManager()
	m.SetLimits(1000, 8) // budget high enough that neither run folds

	history := []db.Message{
		{Role: providers.RoleUser, Text: "hello there"},
		{Role: providers.RoleAssistant, Text: "a short reply"},
	}

	base, err := m.Prepare(context.Background(), nil, nil, db.Session{}, db.Agent{}, history)
	if err != nil {
		t.Fatalf("base prepare: %v", err)
	}

	const overhead = 300
	ctx := WithContextOverhead(context.Background(), overhead)
	with, err := m.Prepare(ctx, nil, nil, db.Session{}, db.Agent{}, history)
	if err != nil {
		t.Fatalf("overhead prepare: %v", err)
	}
	if with.Compacted || base.Compacted {
		t.Fatalf("did not expect compaction under keepRecent")
	}
	// ContextTokens (messages+summary) is unchanged — overhead is a budget input, not
	// stored history — but pressure must fold the overhead in.
	if with.ContextTokens != base.ContextTokens {
		t.Fatalf("ContextTokens changed with overhead: %d vs %d", with.ContextTokens, base.ContextTokens)
	}
	want := float64(with.ContextTokens+overhead) / 1000.0
	if with.Pressure != want {
		t.Fatalf("overhead pressure = %f, want %f", with.Pressure, want)
	}
	if with.Pressure <= base.Pressure {
		t.Fatalf("overhead pressure %f must exceed base %f", with.Pressure, base.Pressure)
	}
}

// TestWithContextOverheadNoOp guards the default: a non-positive overhead leaves
// the budget message-only, so behaviour matches a context with none set.
func TestWithContextOverheadNoOp(t *testing.T) {
	if got := contextOverheadFrom(WithContextOverhead(context.Background(), 0)); got != 0 {
		t.Fatalf("overhead 0 = %d, want 0 (no-op)", got)
	}
	if got := contextOverheadFrom(WithContextOverhead(context.Background(), -5)); got != 0 {
		t.Fatalf("negative overhead = %d, want 0 (no-op)", got)
	}
	if got := contextOverheadFrom(context.Background()); got != 0 {
		t.Fatalf("unset overhead = %d, want 0", got)
	}
	if got := contextOverheadFrom(WithContextOverhead(context.Background(), 250)); got != 250 {
		t.Fatalf("overhead 250 = %d, want 250", got)
	}
}

// TestPrepareJournalsCompaction verifies the observability fix: when Prepare folds
// history into the rolling summary, it appends a `compaction` event to the
// session's debug journal (debug.jsonl) so the fold is visible in the Debug modal /
// read_session_debug / debug summary — not only in the in-app Logs. Regression
// guard for the "compaction ran on a spawned turn but I can't see where" gap.
func TestPrepareJournalsCompaction(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, _ := d.CreateAgent(ctx, db.Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, db.Session{AgentID: agent.ID, Title: "T"})

	m := NewManager()
	m.SetLimits(1, 2) // budget 1 token forces a fold; keepRecent 2

	// 6 alternating turns: more than keepRecent, so foldBoundary produces a fold.
	history := []db.Message{
		{Role: providers.RoleUser, Text: "u1"}, {Role: providers.RoleAssistant, Text: "a1"},
		{Role: providers.RoleUser, Text: "u2"}, {Role: providers.RoleAssistant, Text: "a2"},
		{Role: providers.RoleUser, Text: "u3"}, {Role: providers.RoleAssistant, Text: "a3"},
	}
	prep, err := m.Prepare(ctx, d, stubProvider{summary: "ROLLED UP"}, sess, agent, history)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !prep.Compacted {
		t.Fatalf("expected a fold with budget 1")
	}
	if prep.FoldedMsgs <= 0 {
		t.Fatalf("FoldedMsgs = %d, want > 0 (drives the on-screen compaction step)", prep.FoldedMsgs)
	}

	evs, err := d.ReadDebugEvents(ctx, sess.ID, db.DebugCompaction, 0)
	if err != nil {
		t.Fatalf("read debug: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("compaction events = %d, want 1", len(evs))
	}
	e := evs[0]
	if e.Name != "auto" {
		t.Errorf("trigger Name = %q, want \"auto\"", e.Name)
	}
	if e.SavedBytes <= 0 {
		t.Errorf("SavedBytes = %d, want > 0", e.SavedBytes)
	}
	if !strings.Contains(e.Detail, "folded") || !strings.Contains(e.Detail, "tokens") {
		t.Errorf("Detail = %q, want folded/tokens summary", e.Detail)
	}
	if e.AgentID != agent.ID {
		t.Errorf("AgentID = %q, want %q", e.AgentID, agent.ID)
	}
}

// TestForceCompactFiresManualPreCompact verifies an explicit /compact invokes
// the lifecycle seam only after a real fold is known to be possible.
func TestForceCompactFiresManualPreCompact(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, err := d.CreateAgent(ctx, db.Agent{Name: "A", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := d.CreateSession(ctx, db.Session{AgentID: agent.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	m := NewManager()
	m.SetLimits(1000, 2)
	history := []db.Message{
		{Role: providers.RoleUser, Text: "u1"}, {Role: providers.RoleAssistant, Text: "a1"},
		{Role: providers.RoleUser, Text: "u2"}, {Role: providers.RoleAssistant, Text: "a2"},
	}
	var triggers []string
	ctx = WithPreCompact(ctx, func(trigger string) {
		triggers = append(triggers, trigger)
	})

	folded, _, err := m.ForceCompact(ctx, d, stubProvider{summary: "ROLLED UP"}, sess, agent, history)
	if err != nil {
		t.Fatalf("force compact: %v", err)
	}
	if folded != 2 {
		t.Fatalf("folded = %d, want 2", folded)
	}
	if len(triggers) != 1 || triggers[0] != "manual" {
		t.Fatalf("triggers = %v, want [manual]", triggers)
	}
}

// TestPrepareZeroPressureWhenBudgetDisabled ensures a non-positive budget yields
// Pressure 0 (the feature is off) rather than a divide-by-zero.
func TestPrepareZeroPressureWhenBudgetDisabled(t *testing.T) {
	m := &Manager{maxTokens: 0, keepRecent: 8} // construct directly to bypass SetLimits' guard
	history := []db.Message{{Role: providers.RoleUser, Text: "hi"}}
	prep, err := m.Prepare(context.Background(), nil, nil, db.Session{}, db.Agent{}, history)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if prep.Pressure != 0 {
		t.Fatalf("pressure = %f, want 0 when maxTokens <= 0", prep.Pressure)
	}
}
