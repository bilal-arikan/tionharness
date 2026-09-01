package db

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

func TestDebugStringPolicyCoversEveryPersistedStringField(t *testing.T) {
	typ := reflect.TypeOf(DebugEvent{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Type.Kind() != reflect.String || field.Tag.Get("json") == "-" {
			continue
		}
		if _, ok := debugStringPolicies[field.Name]; !ok {
			t.Errorf("DebugEvent.%s has no debug journal string policy", field.Name)
		}
	}
	for field := range debugStringPolicies {
		if _, ok := typ.FieldByName(field); !ok {
			t.Errorf("debug string policy references missing DebugEvent.%s", field)
		}
	}
}

func TestDebugJournalRedactsEveryStringField(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	session, err := d.CreateSession(context.Background(), Session{Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	typ := reflect.TypeOf(DebugEvent{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Type.Kind() != reflect.String || field.Tag.Get("json") == "-" {
			continue
		}
		for _, raw := range []string{
			"single" + "TokenSecret", "prefix:" + "colon-secret", "Bearer " + "bearer-secret",
			strings.Join([]string{"123e4567", "e89b", "12d3", "a456", "426614174000"}, "-"),
			`{"prompt":"` + `private","tool_input":"secret"}`,
			strings.Repeat("çokgizli", 128),
		} {
			ev := DebugEvent{Type: DebugError, Err: true}
			reflect.ValueOf(&ev).Elem().FieldByName(field.Name).SetString(raw)
			if err := d.AppendDebugEvent(session.ID, ev, 0); err != nil {
				t.Fatalf("append %s: %v", field.Name, err)
			}
			journal, err := os.ReadFile(d.debugPath(session.ID))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(journal), raw) {
				t.Fatalf("DebugEvent.%s leaked %q", field.Name, raw)
			}
		}
	}
	if err := d.AppendDebugEvent(session.ID, DebugEvent{Type: DebugError, Error: "process_error", Detail: "provider_retry"}, 0); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(d.debugPath(session.ID))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"error":"process_error"`) || !strings.Contains(string(raw), `"detail":"provider_retry"`) {
		t.Fatalf("closed allowlist reasons were not preserved: %s", raw)
	}
}

// TestDebugJournalRoundTrip verifies append → read-back, type filtering and the
// aggregate summary (turns, llm-call tokens by model, per-tool rollups, errors).
func TestDebugJournalRoundTrip(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})

	events := []DebugEvent{
		{Type: DebugLLMCall, Model: "claude", In: 100, Out: 20, CacheRead: 50},
		{Type: DebugTool, Name: "Bash", DurMs: 120, OutBytes: 400},
		{Type: DebugTool, Name: "Bash", DurMs: 80, OutBytes: 200, Err: true},
		{Type: DebugTool, Name: "Read", DurMs: 10, OutBytes: 4000},
		{Type: DebugError, Detail: "boom"},
		{Type: DebugCompaction, SavedBytes: 1234},
		{Type: DebugTurn, DurMs: 500, Stop: "end_turn"},
	}
	for _, e := range events {
		if err := d.AppendDebugEvent(sess.ID, e, 0); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	all, err := d.ReadDebugEvents(ctx, sess.ID, "", 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(all) != len(events) {
		t.Fatalf("read count = %d, want %d", len(all), len(events))
	}
	if all[0].Time == 0 {
		t.Errorf("append did not stamp Time")
	}

	tools, _ := d.ReadDebugEvents(ctx, sess.ID, DebugTool, 0)
	if len(tools) != 3 {
		t.Fatalf("tool events = %d, want 3", len(tools))
	}

	sum, err := d.GetDebugSummary(ctx, sess.ID)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if sum.Turns != 1 || sum.LLMCalls != 1 || sum.ToolCalls != 3 {
		t.Errorf("counts: turns=%d llm=%d tools=%d", sum.Turns, sum.LLMCalls, sum.ToolCalls)
	}
	if sum.InputTokens != 100 || sum.OutputTokens != 20 || sum.CacheRead != 50 {
		t.Errorf("tokens: in=%d out=%d cache=%d", sum.InputTokens, sum.OutputTokens, sum.CacheRead)
	}
	modelKey := debugOpaqueFingerprint("Model", "claude")
	if sum.ByModel[modelKey] != 120 {
		t.Errorf("byModel[%s] = %d, want 120", modelKey, sum.ByModel[modelKey])
	}
	bashKey := debugOpaqueFingerprint("Name", "Bash")
	bash := sum.ByTool[bashKey]
	if bash.Calls != 2 || bash.Errors != 1 || bash.DurMs != 200 {
		t.Errorf("Bash stat = %+v", bash)
	}
	if sum.Errors != 1 || sum.LastError != "error [redacted]" {
		t.Errorf("errors=%d last=%q", sum.Errors, sum.LastError)
	}
	if sum.Compactions != 1 || sum.SavedBytes != 1234 {
		t.Errorf("compactions=%d saved=%d", sum.Compactions, sum.SavedBytes)
	}
	if len(sum.TopTools) == 0 || sum.TopTools[0] != bashKey {
		t.Errorf("topTools = %v, want %s first (slowest)", sum.TopTools, bashKey)
	}
}

func TestDebugJournalStartsWithBuildInfoOnce(t *testing.T) {
	SetDebugBuildInfo("abc1234", "2026-08-25T20:00:00Z")
	defer SetDebugBuildInfo("", "")

	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	agent, err := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	session, err := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := d.AppendDebugEvent(session.ID, DebugEvent{Type: DebugTurn}, 0); err != nil {
		t.Fatalf("append first event: %v", err)
	}
	if err := d.AppendDebugEvent(session.ID, DebugEvent{Type: DebugTool}, 0); err != nil {
		t.Fatalf("append second event: %v", err)
	}

	events, err := d.ReadDebugEvents(ctx, session.ID, "", 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("event count = %d, want 3", len(events))
	}
	if events[0].Type != DebugBuild || events[0].Name != "abc1234" || events[0].Detail != "build detail [redacted]" {
		t.Fatalf("first event = %+v, want build info", events[0])
	}
}

func TestDebugBuildCommitOnlyPreservesExpectedWireValues(t *testing.T) {
	for _, value := range []string{"abc1234", "ABCDEF0123456789", "unknown"} {
		if !isDebugBuildCommit(value) {
			t.Errorf("expected safe build commit %q", value)
		}
	}
	for _, value := range []string{"secret", "abc123g", "abc1234-dirty", ""} {
		if isDebugBuildCommit(value) {
			t.Errorf("accepted unsafe build commit %q", value)
		}
	}
}

func TestDebugJournalRedactsProviderFailureTextCentrally(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	agent, err := d.CreateAgent(ctx, Agent{Name: "A", Provider: "claude-cli"})
	if err != nil {
		t.Fatal(err)
	}
	session, err := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	secret := strings.Join([]string{"unique", "secret", "TSK731"}, "-")
	const cliSessionID = "cliSessionId=resume-authority-TSK731"
	uuidLike := strings.Join([]string{"123e4567", "e89b", "12d3", "a456", "426614174000"}, "-")
	providerFailure := `claude CLI process stderr Bearer ` + secret + ` malformed NDJSON {"prompt":"private prompt","tool_input":{"token":"` + secret + `"},"` + cliSessionID + `"}`
	events := []DebugEvent{
		{Type: DebugRecovery, AgentID: agent.ID, Detail: "provider_retry: " + providerFailure},
		{Type: DebugError, AgentID: agent.ID, Detail: "provider_error: " + providerFailure, Error: providerFailure, Err: true},
		{Type: DebugError, AgentID: agent.ID, Detail: "shortSecret", Error: "prefix:colon-secret", Args: `{"tool_input":"private-tool-input"}`, Err: true},
		{Type: DebugRecovery, AgentID: agent.ID, Detail: "Bearer " + "bearer-secret-value", Error: uuidLike},
		{Type: DebugError, AgentID: agent.ID, Detail: `{"prompt":"json-private"}`, Error: strings.Repeat("çokgizli", 200), Err: true},
		{Type: DebugError, AgentID: agent.ID, Detail: "process_error", Error: "process_exit", Err: true},
	}
	for _, ev := range events {
		if err := d.AppendDebugEvent(session.ID, ev, 0); err != nil {
			t.Fatal(err)
		}
	}

	raw, err := os.ReadFile(d.debugPath(session.ID))
	if err != nil {
		t.Fatal(err)
	}
	journal := string(raw)
	for _, forbidden := range []string{
		secret, "resume-authority-TSK731", "private prompt", "tool_input", "malformed NDJSON", "stderr Bearer",
		"short" + "Secret", "colon-secret", "bearer-secret-value", uuidLike,
		"json-private", "çokgizli", "private-tool-input",
	} {
		if strings.Contains(journal, forbidden) {
			t.Fatalf("debug journal leaked %q: %s", forbidden, journal)
		}
	}
	for _, safe := range []string{"provider_retry", "provider_error", "provider stream parse error", "process_error", "process_exit", "[redacted]", "[redacted tool input]"} {
		if !strings.Contains(journal, safe) {
			t.Fatalf("debug journal lacks safe classification %q: %s", safe, journal)
		}
	}
}

// TestDebugSummaryAnomaliesAndSeries verifies the Faz-3 derived fields: the
// per-turn / per-call time series and the heuristic anomaly findings.
func TestDebugSummaryAnomaliesAndSeries(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})

	emit := func(e DebugEvent) {
		if err := d.AppendDebugEvent(sess.ID, e, 0); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	// One slow dominant tool (Bash ~95% of tool time, > 2s total).
	emit(DebugEvent{Type: DebugTool, Name: "Bash", DurMs: 5000, OutBytes: 100})
	emit(DebugEvent{Type: DebugTool, Name: "Read", DurMs: 50, OutBytes: 100})
	// A failing tool (calls=3, 2 errors → >30%).
	for i := 0; i < 3; i++ {
		emit(DebugEvent{Type: DebugTool, Name: "Flaky", DurMs: 10, Err: i < 2})
	}
	// Frequent compaction (>=3) + error burst (>=3).
	for i := 0; i < 3; i++ {
		emit(DebugEvent{Type: DebugCompaction})
		emit(DebugEvent{Type: DebugError, Detail: "boom"})
	}
	// Time series points.
	emit(DebugEvent{Type: DebugTurn, DurMs: 100})
	emit(DebugEvent{Type: DebugTurn, DurMs: 200})
	emit(DebugEvent{Type: DebugLLMCall, Model: "m", In: 10, Out: 5, Think: 2})
	emit(DebugEvent{Type: DebugLLMCall, Model: "m", In: 20, Out: 5, Think: 3})

	sum, err := d.GetDebugSummary(ctx, sess.ID)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	// Thinking attribution: summed (2+3) and its share of total output (5/10).
	if sum.ThinkingTokens != 5 {
		t.Errorf("thinkingTokens = %d, want 5", sum.ThinkingTokens)
	}
	if sum.ThinkingShare != 0.5 {
		t.Errorf("thinkingShare = %v, want 0.5", sum.ThinkingShare)
	}
	if len(sum.TurnDurSeries) != 2 || sum.TurnDurSeries[1] != 200 {
		t.Errorf("turnDurSeries = %v", sum.TurnDurSeries)
	}
	if len(sum.TokenSeries) != 2 || sum.TokenSeries[1] != 25 {
		t.Errorf("tokenSeries = %v", sum.TokenSeries)
	}
	codes := map[string]bool{}
	for _, a := range sum.Anomalies {
		codes[a.Code] = true
	}
	for _, want := range []string{"tool_time_dominant", "tool_failing", "frequent_compaction", "error_burst"} {
		if !codes[want] {
			t.Errorf("missing anomaly %q; got %v", want, codes)
		}
	}
}

// TestDebugSummaryCacheBreaks verifies cache_break events are counted, the last
// reason is captured, and the anomaly escalates from info (1) to warn (>=2).
func TestDebugSummaryCacheBreaks(t *testing.T) {
	ctx := context.Background()
	d, _ := Open(filepath.Join(t.TempDir(), "store"))
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})

	// One break → info anomaly, reason captured.
	s1, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "one"})
	_ = d.AppendDebugEvent(s1.ID, DebugEvent{Type: DebugCacheBreak, Name: "ttl-or-server-eviction", Detail: "TTL doldu"}, 0)
	sum1, _ := d.GetDebugSummary(ctx, s1.ID)
	if sum1.CacheBreaks != 1 || sum1.LastCacheBreak != "cache break detail [redacted]" {
		t.Fatalf("cacheBreaks=%d last=%q, want redacted cache-break summary", sum1.CacheBreaks, sum1.LastCacheBreak)
	}
	if code := anomalyCode(sum1.Anomalies, "cache_break"); code == nil || code.Severity != "info" {
		t.Errorf("one break should raise an info cache_break anomaly, got %+v", sum1.Anomalies)
	}

	// Two breaks → warn anomaly.
	s2, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "two"})
	_ = d.AppendDebugEvent(s2.ID, DebugEvent{Type: DebugCacheBreak, Detail: "model değişti"}, 0)
	_ = d.AppendDebugEvent(s2.ID, DebugEvent{Type: DebugCacheBreak, Detail: "prompt değişti"}, 0)
	sum2, _ := d.GetDebugSummary(ctx, s2.ID)
	if sum2.CacheBreaks != 2 {
		t.Fatalf("cacheBreaks=%d, want 2", sum2.CacheBreaks)
	}
	if code := anomalyCode(sum2.Anomalies, "cache_breaks"); code == nil || code.Severity != "warn" {
		t.Errorf("two breaks should raise a warn cache_breaks anomaly, got %+v", sum2.Anomalies)
	}
}

// TestDebugSummaryCoolingWaste verifies the isolated cooling-waste rollup: only
// cache_break events carrying a WasteUSD contribute, they are summed, counted,
// and the estimated flag latches when any contributing figure is an estimate.
func TestDebugSummaryCoolingWaste(t *testing.T) {
	ctx := context.Background()
	d, _ := Open(filepath.Join(t.TempDir(), "store"))
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	s, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "cool"})

	// Two TTL breaks with waste (one estimated) + a model-change break with none.
	_ = d.AppendDebugEvent(s.ID, DebugEvent{Type: DebugCacheBreak, Name: "ttl-or-server-eviction", WasteUSD: 0.02}, 0)
	_ = d.AppendDebugEvent(s.ID, DebugEvent{Type: DebugCacheBreak, Name: "ttl-or-server-eviction", WasteUSD: 0.03, WasteEstimated: true}, 0)
	_ = d.AppendDebugEvent(s.ID, DebugEvent{Type: DebugCacheBreak, Name: "model-changed", Detail: "model değişti"}, 0)

	sum, _ := d.GetDebugSummary(ctx, s.ID)
	if sum.CacheBreaks != 3 {
		t.Fatalf("cacheBreaks=%d, want 3", sum.CacheBreaks)
	}
	if sum.CoolingBreaks != 2 {
		t.Errorf("coolingBreaks=%d, want 2 (only waste-bearing breaks)", sum.CoolingBreaks)
	}
	if math.Abs(sum.CoolingWasteUSD-0.05) > 1e-9 {
		t.Errorf("coolingWasteUsd=%v, want 0.05", sum.CoolingWasteUSD)
	}
	if !sum.CoolingWasteEstimated {
		t.Error("coolingWasteEstimated should latch true when any figure is estimated")
	}
}

// TestAddCoolingWaste verifies cooling-waste USD accumulates into today's usage
// row (upsert), latches the estimated flag, and ignores non-positive amounts.
func TestAddCoolingWaste(t *testing.T) {
	ctx := context.Background()
	d, _ := Open(filepath.Join(t.TempDir(), "store"))
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})

	if err := d.AddCoolingWaste(ctx, agent.ID, 0, false); err != nil {
		t.Fatalf("zero waste err: %v", err)
	}
	_ = d.AddCoolingWaste(ctx, agent.ID, 0.02, false)
	_ = d.AddCoolingWaste(ctx, agent.ID, 0.03, true) // estimate → latches flag

	u, _ := d.GetUsageToday(ctx, agent.ID)
	if math.Abs(u.CoolingWasteUSD-0.05) > 1e-9 {
		t.Errorf("coolingWasteUsd=%v, want 0.05", u.CoolingWasteUSD)
	}
	if !u.CoolingWasteEstimated {
		t.Error("estimated flag should latch true after an estimated contribution")
	}
}

// TestDebugSummaryCacheAndThinkingCoach verifies the two observability coaches:
// a sustained low warm-hit ratio raises low_cache_hit, and a large hidden-
// reasoning share of a meaningful output spend raises high_thinking.
func TestDebugSummaryCacheAndThinkingCoach(t *testing.T) {
	ctx := context.Background()
	d, _ := Open(filepath.Join(t.TempDir(), "store"))
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})

	// 3 calls, big prompt spend, almost no cache reads → low hit ratio.
	// Output carries a majority-thinking share too.
	s, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "coach"})
	for i := 0; i < 3; i++ {
		_ = d.AppendDebugEvent(s.ID, DebugEvent{
			Type: DebugLLMCall, Model: "m", In: 10000, Out: 1000, Think: 700, CacheRead: 500,
		}, 0)
	}
	sum, _ := d.GetDebugSummary(ctx, s.ID)
	if code := anomalyCode(sum.Anomalies, "low_cache_hit"); code == nil || code.Severity != "warn" {
		t.Errorf("low warm-hit ratio should raise a warn low_cache_hit anomaly, got %+v", sum.Anomalies)
	}
	if code := anomalyCode(sum.Anomalies, "high_thinking"); code == nil || code.Severity != "info" {
		t.Errorf("majority-thinking output should raise an info high_thinking anomaly, got %+v", sum.Anomalies)
	}

	// Warm session: high cache reads → no low_cache_hit.
	s2, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "warm"})
	for i := 0; i < 3; i++ {
		_ = d.AppendDebugEvent(s2.ID, DebugEvent{Type: DebugLLMCall, Model: "m", In: 500, Out: 200, CacheRead: 40000}, 0)
	}
	sum2, _ := d.GetDebugSummary(ctx, s2.ID)
	if code := anomalyCode(sum2.Anomalies, "low_cache_hit"); code != nil {
		t.Errorf("warm cache should not flag low_cache_hit, got %+v", sum2.Anomalies)
	}
}

// anomalyCode returns the first anomaly with the given code, or nil.
func anomalyCode(as []DebugAnomaly, code string) *DebugAnomaly {
	for i := range as {
		if as[i].Code == code {
			return &as[i]
		}
	}
	return nil
}

// TestDebugSummaryNoAnomaliesOnHealthy verifies a healthy session is quiet.
func TestDebugSummaryNoAnomaliesOnHealthy(t *testing.T) {
	ctx := context.Background()
	d, _ := Open(filepath.Join(t.TempDir(), "store"))
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})
	_ = d.AppendDebugEvent(sess.ID, DebugEvent{Type: DebugTool, Name: "Read", DurMs: 20, OutBytes: 100}, 0)
	_ = d.AppendDebugEvent(sess.ID, DebugEvent{Type: DebugTurn, DurMs: 500}, 0)
	sum, _ := d.GetDebugSummary(ctx, sess.ID)
	if len(sum.Anomalies) != 0 {
		t.Errorf("healthy session flagged anomalies: %v", sum.Anomalies)
	}
}

// TestDebugJournalCapPrunes verifies the file is pruned to the newest cap events
// once it grows past cap + cap/4, and that the survivors are the newest ones.
func TestDebugJournalCapPrunes(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})

	cap := 8
	// Append well past cap + cap/4 so pruning triggers at least once.
	for i := 0; i < 40; i++ {
		if err := d.AppendDebugEvent(sess.ID, DebugEvent{Type: DebugTool, Name: "T", DurMs: int64(i)}, cap); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	evs, err := d.ReadDebugEvents(ctx, sess.ID, "", 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(evs) > cap+cap/4 {
		t.Fatalf("retained %d events, want <= %d", len(evs), cap+cap/4)
	}
	// The newest event (DurMs=39) must survive; the oldest (DurMs=0) must be gone.
	last := evs[len(evs)-1]
	if last.DurMs != 39 {
		t.Errorf("newest survivor DurMs = %d, want 39", last.DurMs)
	}
}

// TestGetTurnDebugCacheBreak verifies the PER-MESSAGE rollup attributes a cache
// break to the turn that paid for it: the reason/detail travel with it, the
// avoidable cooling overpay is summed with its estimated flag, and a break tagged
// to a DIFFERENT turn never leaks into this one.
func TestGetTurnDebugCacheBreak(t *testing.T) {
	ctx := context.Background()
	d, _ := Open(filepath.Join(t.TempDir(), "store"))
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	s, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "t"})

	_ = d.AppendDebugEvent(s.ID, DebugEvent{
		Type: DebugLLMCall, TurnID: "MSG1", Model: "opus", In: 12, Out: 3, CacheWrite: 9000,
	}, 0)
	_ = d.AppendDebugEvent(s.ID, DebugEvent{
		Type: DebugCacheBreak, TurnID: "MSG1", Name: "ttl-or-server-eviction",
		Detail: "Önek değişmedi ama cache okunmadı", WasteUSD: 0.021, WasteEstimated: true,
	}, 0)
	// A second turn's break must stay out of MSG1's rollup.
	_ = d.AppendDebugEvent(s.ID, DebugEvent{
		Type: DebugCacheBreak, TurnID: "MSG2", Name: "model-changed", Detail: "Model değişti",
	}, 0)

	td, err := d.GetTurnDebug(ctx, s.ID, "MSG1")
	if err != nil {
		t.Fatalf("GetTurnDebug: %v", err)
	}
	if td.CacheBreaks != 1 {
		t.Fatalf("cacheBreaks=%d, want 1 (MSG2's break must not leak)", td.CacheBreaks)
	}
	if td.CacheBreakReason != "ttl-or-server-eviction" || td.CacheBreakDetail == "" {
		t.Errorf("reason=%q detail=%q, want the ttl tag with its detail", td.CacheBreakReason, td.CacheBreakDetail)
	}
	if td.CoolingWasteUSD != 0.021 || !td.CoolingWasteEstimated {
		t.Errorf("waste=%v est=%v, want 0.021 / true", td.CoolingWasteUSD, td.CoolingWasteEstimated)
	}

	// A turn with no break reports none — the zero value, so the panel renders nothing.
	td2, _ := d.GetTurnDebug(ctx, s.ID, "MSG3")
	if td2.CacheBreaks != 0 || td2.CacheBreakReason != "" {
		t.Errorf("unknown turn should carry no break, got %+v", td2)
	}
}

// TestDebugJournalNameAllowListSurvivesRoundTrip locks the per-type Name
// allow-list in sanitizeDebugName. These names are the only human-readable
// labels the debug panel has: a compaction row whose trigger is fingerprinted
// says "something compacted" without saying why (auto vs manual vs reactive),
// and the same holds for cache-break reasons, hook names, guardrail actions and
// slash commands. Shrinking the allow-list is therefore a silent regression, so
// every allowed value is asserted to come back raw here.
func TestDebugJournalNameAllowListSurvivesRoundTrip(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	// Mirrors the switch arms of sanitizeDebugName. Keep both in sync.
	allowed := map[string][]string{
		DebugCompaction: {
			"auto", "auto-native", "cli-native", "manual", "manual-native", "reactive",
			"native_skipped", "claim_consumed", "native_fallback_rolling",
		},
		DebugCacheBreak: {"model-changed", "prompt-or-tools-changed", "ttl-or-server-eviction"},
		DebugHook: {
			HookPreCompact, HookPreToolUse, HookPostToolUse, HookUserPromptSubmit,
			HookSessionStart, HookStop, HookSubagentStop, HookNotification, HookSessionEnd,
		},
		DebugGuardrail: {
			"cli_warn", "mcp_args_block", "mcp_prefill", "mcp_repair", "mcp_repair_block",
			"mcp_repair_index", "mcp_repair_retry", "stuck_gate", "warn",
		},
		DebugError: {"/compact", "/compact-custom", "/handoff", "/refresh-context"},
		DebugLifecycle: {
			"turn_cancelled_by_teardown", "autonomous_cancelled",
			"teardown_grace_exceeded", "queued_turn_dropped",
		},
		DebugPressure: {"fold_idle_floor"},
		DebugLLMCall:  {"failed_turn_billed"},
	}

	for eventType, names := range allowed {
		for _, name := range names {
			sess, err := d.CreateSession(ctx, Session{Title: "T"})
			if err != nil {
				t.Fatalf("create session: %v", err)
			}
			if err := d.AppendDebugEvent(sess.ID, DebugEvent{Type: eventType, Name: name}, 0); err != nil {
				t.Fatalf("append %s/%s: %v", eventType, name, err)
			}
			events, err := d.ReadDebugEvents(ctx, sess.ID, eventType, 0)
			if err != nil {
				t.Fatalf("read %s/%s: %v", eventType, name, err)
			}
			if len(events) != 1 {
				t.Fatalf("read %s/%s: got %d events, want 1", eventType, name, len(events))
			}
			if events[0].Name != name {
				t.Errorf("%s Name %q came back as %q: the allow-list no longer covers it, "+
					"so the debug panel can no longer show why this happened",
					eventType, name, events[0].Name)
			}
		}
	}
}

func TestDebugJournalStopAllowListSurvivesRoundTrip(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sess, err := d.CreateSession(ctx, Session{Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	canonical := []string{
		providers.StopEndTurn,
		providers.StopToolUse,
		providers.StopMaxTok,
		providers.StopPauseTurn,
		providers.StopRefusal,
		providers.StopContextWindow,
	}
	for _, stop := range canonical {
		if err := d.AppendDebugEvent(sess.ID, DebugEvent{Type: DebugTurn, Stop: stop}, 0); err != nil {
			t.Fatalf("append %q: %v", stop, err)
		}
	}
	if err := d.AppendDebugEvent(sess.ID, DebugEvent{Type: DebugTurn, Stop: "future-secret-stop"}, 0); err != nil {
		t.Fatalf("append unknown stop: %v", err)
	}
	events, err := d.ReadDebugEvents(ctx, sess.ID, DebugTurn, 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(events) != len(canonical)+1 {
		t.Fatalf("got %d events, want %d", len(events), len(canonical)+1)
	}
	for i, want := range canonical {
		if events[i].Stop != want {
			t.Errorf("event %d Stop = %q, want %q", i, events[i].Stop, want)
		}
	}
	if got := events[len(events)-1].Stop; got != "redacted" {
		t.Errorf("unknown Stop = %q, want redacted", got)
	}
}

// TestDebugJournalNameOutsideAllowListIsFingerprinted is the other half of the
// contract locked above: anything not on the allow-list must leave the journal
// as an opaque, stable fingerprint — never as the caller's raw string, which
// could carry a prompt, a path or a secret.
func TestDebugJournalNameOutsideAllowListIsFingerprinted(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	for _, name := range []string{
		"auto-but-not-really",
		"compaction triggered by user request",
		"sk-ant-api03-secret-token-value",
		"some free text",
	} {
		sess, err := d.CreateSession(ctx, Session{Title: "T"})
		if err != nil {
			t.Fatalf("create session: %v", err)
		}
		if err := d.AppendDebugEvent(sess.ID, DebugEvent{Type: DebugCompaction, Name: name}, 0); err != nil {
			t.Fatalf("append %q: %v", name, err)
		}
		events, err := d.ReadDebugEvents(ctx, sess.ID, DebugCompaction, 0)
		if err != nil {
			t.Fatalf("read %q: %v", name, err)
		}
		if len(events) != 1 {
			t.Fatalf("read %q: got %d events, want 1", name, len(events))
		}
		got := events[0].Name
		if got == name {
			t.Errorf("Name %q was stored raw; only allow-listed names may bypass redaction", name)
		}
		if !strings.HasPrefix(got, "fp:") {
			t.Errorf("Name %q became %q, want an fp: fingerprint", name, got)
		}
	}
}

// TestDebugJournalNameFingerprintIsStable pins the fingerprint to be
// deterministic for a given input: the debug panel groups redacted rows by this
// value, so an unstable hash would scatter one trigger across many buckets.
func TestDebugJournalNameFingerprintIsStable(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sess, err := d.CreateSession(ctx, Session{Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	const name = "unlisted compaction trigger"
	for i := 0; i < 2; i++ {
		if err := d.AppendDebugEvent(sess.ID, DebugEvent{Type: DebugCompaction, Name: name}, 0); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	events, err := d.ReadDebugEvents(ctx, sess.ID, DebugCompaction, 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].Name != events[1].Name {
		t.Errorf("same input fingerprinted as %q and %q; grouping by fingerprint would break",
			events[0].Name, events[1].Name)
	}
	if want := debugOpaqueFingerprint("Name", name); events[0].Name != want {
		t.Errorf("fingerprint = %q, want %q", events[0].Name, want)
	}
}

// TestDebugJournalPhaseAllowListSurvivesRoundTrip locks the Phase allow-list.
// Phase is what tells apart the teardown and queue-drop lifecycle events from
// each other: without it a "queued_turn_dropped" row cannot say whether the user
// cancelled one turn or cleared the whole queue, and a grace-exceeded row cannot
// say which teardown step ran out of time. A value missing from the allow-list
// is stored as "redacted", which loses exactly that distinction.
func TestDebugJournalPhaseAllowListSurvivesRoundTrip(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sess, err := d.CreateSession(ctx, Session{Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Emitted by recordTeardownLifecycle/noteTeardownGrace (internal/api/session_teardown.go)
	// and recordQueuedTurnDropped (internal/api/inbox.go).
	allowed := []string{
		"attempt", "signal", "success", "error", "cancelled",
		"inflight_turn", "mcp_calls", "worker", "user_cancel", "queue_cleared",
	}
	for _, phase := range allowed {
		if err := d.AppendDebugEvent(sess.ID, DebugEvent{Type: DebugLifecycle, Phase: phase}, 0); err != nil {
			t.Fatalf("append %q: %v", phase, err)
		}
	}
	if err := d.AppendDebugEvent(sess.ID, DebugEvent{Type: DebugLifecycle, Phase: "totally-free-text"}, 0); err != nil {
		t.Fatalf("append unlisted phase: %v", err)
	}

	events, err := d.ReadDebugEvents(ctx, sess.ID, DebugLifecycle, 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(events) != len(allowed)+1 {
		t.Fatalf("got %d events, want %d", len(events), len(allowed)+1)
	}
	for i, want := range allowed {
		if events[i].Phase != want {
			t.Errorf("event %d Phase = %q, want %q: the allow-list no longer covers it, "+
				"so the debug panel can no longer say which step this was", i, events[i].Phase, want)
		}
	}
	if got := events[len(events)-1].Phase; got != "redacted" {
		t.Errorf("unlisted Phase = %q, want redacted", got)
	}
}

// TestDebugSummaryIgnoresFailedTurnBilledTokens pins the fix for a double count:
// a failed turn writes its cost twice — once as the attempt's own llm_call event
// and once as the readable "failed_turn_billed" marker — so summing every
// llm_call showed that turn as two calls at twice its real token spend.
func TestDebugSummaryIgnoresFailedTurnBilledTokens(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sess, err := d.CreateSession(ctx, Session{Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	billed := DebugEvent{
		Type: DebugLLMCall, TurnID: "turn-1", Model: "claude-x",
		In: 1000, Out: 200, Think: 50, CacheRead: 300, CacheWrite: 400,
	}
	for _, ev := range []DebugEvent{billed, func() DebugEvent {
		dup := billed
		dup.Name = debugNameFailedTurnBilled
		return dup
	}()} {
		if err := d.AppendDebugEvent(sess.ID, ev, 0); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	sum, err := d.GetDebugSummary(ctx, sess.ID)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if sum.Events != 2 {
		t.Errorf("Events = %d, want 2: both events must stay readable in the journal", sum.Events)
	}
	if sum.LLMCalls != 1 {
		t.Errorf("LLMCalls = %d, want 1", sum.LLMCalls)
	}
	if sum.InputTokens != 1000 || sum.OutputTokens != 200 || sum.ThinkingTokens != 50 {
		t.Errorf("tokens = in %d / out %d / think %d, want 1000 / 200 / 50",
			sum.InputTokens, sum.OutputTokens, sum.ThinkingTokens)
	}
	if sum.CacheRead != 300 || sum.CacheWrite != 400 {
		t.Errorf("cache = read %d / write %d, want 300 / 400", sum.CacheRead, sum.CacheWrite)
	}
	// Model is stored as an opaque fingerprint, so the rollup is keyed by that.
	modelKey := debugOpaqueFingerprint("Model", "claude-x")
	if got := sum.ByModel[modelKey]; got != 1200 {
		t.Errorf("ByModel[%s] = %d, want 1200", modelKey, got)
	}

	td, err := d.GetTurnDebug(ctx, sess.ID, "turn-1")
	if err != nil {
		t.Fatalf("turn debug: %v", err)
	}
	if !td.Found {
		t.Fatal("turn debug not found")
	}
	if td.LLMCalls != 1 {
		t.Errorf("turn LLMCalls = %d, want 1", td.LLMCalls)
	}
	if td.InputTokens != 1000 || td.OutputTokens != 200 || td.ThinkingTokens != 50 {
		t.Errorf("turn tokens = in %d / out %d / think %d, want 1000 / 200 / 50",
			td.InputTokens, td.OutputTokens, td.ThinkingTokens)
	}
	if td.CacheRead != 300 || td.CacheWrite != 400 {
		t.Errorf("turn cache = read %d / write %d, want 300 / 400", td.CacheRead, td.CacheWrite)
	}
	st := td.ByModel[modelKey]
	if st.Calls != 1 || st.InputTokens != 1000 || st.OutputTokens != 200 {
		t.Errorf("turn ByModel[%s] = %+v, want 1 call / 1000 in / 200 out", modelKey, st)
	}
}

// TestDebugJournalProviderAllowListSurvivesRoundTrip locks the Provider allow-list.
// Provider is the only field that says which backend produced a row, so a value
// missing from the allow-list turns every event of that provider into an
// indistinguishable "redacted" — and the CLI compaction checkpoint dedupe keys
// off Provider (debug_cli_compaction.go), so redaction also collapses distinct
// providers onto one another.
func TestDebugJournalProviderAllowListSurvivesRoundTrip(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sess, err := d.CreateSession(ctx, Session{Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Mirrors debugEnumValues["Provider"] in debug_journal.go.
	allowed := []string{
		"anthropic", "claude-cli", "codex-cli", "ollama", "openai", "openrouter",
	}
	for _, provider := range allowed {
		if err := d.AppendDebugEvent(sess.ID, DebugEvent{Type: DebugLifecycle, Provider: provider}, 0); err != nil {
			t.Fatalf("append %q: %v", provider, err)
		}
	}
	if err := d.AppendDebugEvent(sess.ID, DebugEvent{Type: DebugLifecycle, Provider: "totally-free-text"}, 0); err != nil {
		t.Fatalf("append unlisted provider: %v", err)
	}

	events, err := d.ReadDebugEvents(ctx, sess.ID, DebugLifecycle, 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(events) != len(allowed)+1 {
		t.Fatalf("got %d events, want %d", len(events), len(allowed)+1)
	}
	for i, want := range allowed {
		if events[i].Provider != want {
			t.Errorf("event %d Provider = %q, want %q: the allow-list no longer covers it, "+
				"so the debug panel can no longer say which backend produced this row", i, events[i].Provider, want)
		}
	}
	if got := events[len(events)-1].Provider; got != "redacted" {
		t.Errorf("unlisted Provider = %q, want redacted", got)
	}
}

// TestDebugJournalSignalAllowListSurvivesRoundTrip locks the Signal allow-list.
// Signal carries the native-compaction stream markers (claudecli_stream.go) that
// tell a pre-compaction boundary apart from the post-compaction result; once a
// value is stored as "redacted" that ordering is unrecoverable from disk.
func TestDebugJournalSignalAllowListSurvivesRoundTrip(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sess, err := d.CreateSession(ctx, Session{Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Mirrors debugEnumValues["Signal"] in debug_journal.go.
	allowed := []string{
		"boundary", "compact_boundary", "compact_result", "post",
		"postcompact", "precompact", "status",
	}
	for _, signal := range allowed {
		if err := d.AppendDebugEvent(sess.ID, DebugEvent{Type: DebugLifecycle, Signal: signal}, 0); err != nil {
			t.Fatalf("append %q: %v", signal, err)
		}
	}
	if err := d.AppendDebugEvent(sess.ID, DebugEvent{Type: DebugLifecycle, Signal: "totally-free-text"}, 0); err != nil {
		t.Fatalf("append unlisted signal: %v", err)
	}

	events, err := d.ReadDebugEvents(ctx, sess.ID, DebugLifecycle, 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(events) != len(allowed)+1 {
		t.Fatalf("got %d events, want %d", len(events), len(allowed)+1)
	}
	for i, want := range allowed {
		if events[i].Signal != want {
			t.Errorf("event %d Signal = %q, want %q: the allow-list no longer covers it, "+
				"so the compaction lifecycle can no longer be reconstructed", i, events[i].Signal, want)
		}
	}
	if got := events[len(events)-1].Signal; got != "redacted" {
		t.Errorf("unlisted Signal = %q, want redacted", got)
	}
}
