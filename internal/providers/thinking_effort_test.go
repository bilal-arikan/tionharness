package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestUsesCoarseEffort pins the family gate: DeepSeek's hosted V4 generation and
// the GLM-5.3 line, on any transport — but not local distills, the retired
// DeepSeek aliases, or GLM-5.2 and older.
func TestUsesCoarseEffort(t *testing.T) {
	yes := []string{
		"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-pro", "deepseek-v4-flash-vision-exp",
		"deepseek/deepseek-v4.1-flash", "DeepSeek-Flash",
		"glm-5.3", "glm-5.3-flash", "glm-5.3-flashx", "z-ai/glm-5.3",
	}
	for _, m := range yes {
		if !UsesCoarseEffort(m) {
			t.Errorf("UsesCoarseEffort(%q) = false, want true", m)
		}
	}
	no := []string{
		"deepseek-r1-distill-qwen-32b", "deepseek-chat", "deepseek-reasoner",
		"glm-5.2", "glm-5.1", "glm-5", "glm-4.7-flash", "claude-opus-4-8", "MiniMax-M3", "",
	}
	for _, m := range no {
		if UsesCoarseEffort(m) {
			t.Errorf("UsesCoarseEffort(%q) = true, want false", m)
		}
	}
}

// TestForcedThinking: only the GLM-5.3 family cannot stop reasoning; DeepSeek
// V4.x can be switched off.
func TestForcedThinking(t *testing.T) {
	for _, m := range []string{"glm-5.3", "glm-5.3-flash", "glm-5.3-flashx", "z-ai/glm-5.3"} {
		if !ForcedThinking(m) {
			t.Errorf("ForcedThinking(%q) = false, want true", m)
		}
	}
	for _, m := range []string{"deepseek-flash", "deepseek-v4-pro", "glm-5.2", "claude-fable-5"} {
		if ForcedThinking(m) {
			t.Errorf("ForcedThinking(%q) = true, want false", m)
		}
	}
}

// TestCoarseEffortForBudget pins the fold of every agent tier onto low/high/max,
// using the budgets thinkingBudgetForLevel produces.
func TestCoarseEffortForBudget(t *testing.T) {
	cases := []struct {
		budget int
		forced bool
		want   string
	}{
		{0, false, ""},       // off → omitted, the caller disables reasoning
		{0, true, "low"},     // off on a forced model → the lowest level
		{2048, false, "low"}, // low
		{8192, false, "high"},
		{16384, false, "high"},
		{32768, false, "max"}, // xhigh
		{65536, false, "max"},
		{131072, true, "max"}, // ultra
	}
	for _, c := range cases {
		if got := CoarseEffortForBudget(c.budget, c.forced); got != c.want {
			t.Errorf("CoarseEffortForBudget(%d, %v) = %q, want %q", c.budget, c.forced, got, c.want)
		}
	}
}

// TestCoarseEffortThinkingWire pins the Messages-API shape for the effort class.
func TestCoarseEffortThinkingWire(t *testing.T) {
	// DeepSeek off: an explicit disabled switch (it reasons by default), no effort.
	p, cfg, mt := thinkingFor("deepseek-flash", 0, 4096)
	if p == nil || p.Type != "disabled" || cfg != nil || mt != 4096 {
		t.Errorf("deepseek off: got %+v cfg %+v mt %d", p, cfg, mt)
	}
	// GLM-5.3 off: "disabled" fails there, so no thinking field + lowest effort.
	p, cfg, _ = thinkingFor("glm-5.3", 0, 4096)
	if p != nil || cfg == nil || cfg.Effort != "low" {
		t.Errorf("glm-5.3 off: got %+v cfg %+v", p, cfg)
	}
	// High: enabled + budget kept for schema validity, effort carries the depth.
	p, cfg, mt = thinkingFor("deepseek-v4-pro", 16384, 32768)
	if p == nil || p.Type != "enabled" || p.BudgetTokens != 16384 || cfg == nil || cfg.Effort != "high" || mt != 32768 {
		t.Errorf("deepseek high: got %+v cfg %+v mt %d", p, cfg, mt)
	}
	// Max: the effort comes from the UNCLAMPED budget, the budget itself is
	// clamped and max_tokens bumped above it.
	p, cfg, mt = thinkingFor("glm-5.3-flash", 65536, 4096)
	if p == nil || p.BudgetTokens != legacyThinkingBudgetCap || cfg == nil || cfg.Effort != "max" {
		t.Errorf("glm max: got %+v cfg %+v", p, cfg)
	}
	if mt <= legacyThinkingBudgetCap {
		t.Errorf("max_tokens must exceed the budget: got %d", mt)
	}
	// The legacy class is untouched: GLM-5.2 off still omits everything.
	if p, cfg, _ := thinkingFor("glm-5.2", 0, 4096); p != nil || cfg != nil {
		t.Errorf("glm-5.2 off should stay omitted: got %+v cfg %+v", p, cfg)
	}
}

// TestCoarseEffortOnTheWire checks the request bodies the Anthropic-protocol
// kinds actually send for a tool-loop turn (zero budget) and a max turn.
func TestCoarseEffortOnTheWire(t *testing.T) {
	var got map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = nil
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()

	send := func(name, model string, budget int) {
		t.Helper()
		a := NewAnthropic("k").WithEndpoint(name, srv.URL, model)
		if _, err := a.Complete(context.Background(), Request{
			Model:          model,
			ThinkingBudget: budget,
			Messages:       []Message{{Role: RoleUser, Text: "hi"}},
		}); err != nil {
			t.Fatalf("%s/%s: %v", name, model, err)
		}
	}

	send("zai", "glm-5.3", 0)
	if _, ok := got["thinking"]; ok {
		t.Errorf("glm-5.3 tool turn must not send a thinking field: %s", got["thinking"])
	}
	if string(got["output_config"]) != `{"effort":"low"}` {
		t.Errorf("glm-5.3 tool turn output_config = %s, want effort low", got["output_config"])
	}

	send("deepseek-anthropic", "deepseek-flash", 0)
	if string(got["thinking"]) != `{"type":"disabled"}` {
		t.Errorf("deepseek tool turn thinking = %s, want disabled", got["thinking"])
	}
	if _, ok := got["output_config"]; ok {
		t.Errorf("deepseek off must not send an effort: %s", got["output_config"])
	}

	send("deepseek-anthropic", "deepseek-v4-pro", 65536)
	if string(got["output_config"]) != `{"effort":"max"}` {
		t.Errorf("deepseek max output_config = %s, want effort max", got["output_config"])
	}
}
