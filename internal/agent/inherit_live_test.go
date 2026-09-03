package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// TestLiveInheritedTitler is a REAL end-to-end check that agent inheritance
// reaches the runtime: the titler role is customised through a derived child
// whose soul is overridden, and the title a real claude-cli call produces must
// follow the override; disabling the customisation must hand the role back to
// the locked built-in. It spends real subscription quota (three short haiku
// calls), so it is gated behind TIONHARNESS_LIVE_INHERIT=1.
//
//	TIONHARNESS_LIVE_INHERIT=1 go test ./internal/agent/ -run TestLiveInheritedTitler -v
func TestLiveInheritedTitler(t *testing.T) {
	if os.Getenv("TIONHARNESS_LIVE_INHERIT") != "1" {
		t.Skip("set TIONHARNESS_LIVE_INHERIT=1 to run the live inherited-titler check")
	}
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	if !rt.providers.ClaudeCLIAvailable() {
		t.Skip("claude CLI not on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	seedSystemAgents(t, rt)

	caller, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "claude-cli", Model: "haiku", ThinkingLevel: "off"})
	if err != nil {
		t.Fatal(err)
	}
	const source = "Kullanıcı: Go projemde SQLite yerine dosya tabanlı JSON deposuna geçmek istiyorum, migration planı çıkarır mısın?"
	const marker = "ZEBRA"

	// 1. Built-in titler (locked row) serves the role.
	builtin, ok := rt.db.FindAgentBySystemKey("titler")
	if !ok || !builtin.Locked {
		t.Fatalf("titler role should resolve to the locked built-in: %+v", builtin)
	}
	title1, err := rt.GenerateTitle(ctx, caller, source)
	if err != nil {
		t.Fatalf("title with built-in: %v", err)
	}
	t.Logf("built-in titler → %q", title1)
	if strings.Contains(strings.ToUpper(title1), marker) {
		t.Fatalf("built-in title unexpectedly carries the marker: %q", title1)
	}

	// 2. Customise: derive a bound child and override ONLY the soul.
	child, err := rt.db.DeriveAgent(ctx, builtin.ID, db.DeriveAgentOptions{Name: "Titler (özel)", BindRole: true})
	if err != nil {
		t.Fatal(err)
	}
	soul := "You generate titles. Reply with ONLY a title of 3 to 6 words. The title MUST begin with the exact word " + marker + " followed by a space. No quotes, no preamble."
	updated, err := rt.db.UpdateAgent(ctx, child.ID, db.AgentProfilePatch{Soul: &soul})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Overrides) != 1 || updated.Overrides[0] != "soul" || updated.Model != builtin.Model {
		t.Fatalf("child should override only the soul and inherit the model: overrides=%v model=%q", updated.Overrides, updated.Model)
	}
	if serving, _ := rt.db.FindAgentBySystemKey("titler"); serving.ID != child.ID {
		t.Fatalf("role should resolve to the customisation, got %q", serving.ID)
	}
	title2, err := rt.GenerateTitle(ctx, caller, source)
	if err != nil {
		t.Fatalf("title with customisation: %v", err)
	}
	t.Logf("customised titler → %q", title2)
	if !strings.Contains(strings.ToUpper(title2), marker) {
		t.Fatalf("customised soul did not reach the model: title=%q", title2)
	}

	// 3. Disable the customisation: the role falls back to the built-in.
	disabled := true
	if _, err := rt.db.UpdateAgent(ctx, child.ID, db.AgentProfilePatch{Disabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if serving, _ := rt.db.FindAgentBySystemKey("titler"); serving.ID != builtin.ID {
		t.Fatalf("role should fall back to the built-in, got %q", serving.ID)
	}
	title3, err := rt.GenerateTitle(ctx, caller, source)
	if err != nil {
		t.Fatalf("title after disabling customisation: %v", err)
	}
	t.Logf("built-in titler again → %q", title3)
	if strings.Contains(strings.ToUpper(title3), marker) {
		t.Fatalf("disabled customisation still reached the model: %q", title3)
	}
}

// TestLiveInheritedSoulReachesCompletion proves a plain derived agent runs with
// the soul it INHERITS (no override of its own): the child's resolved persona
// is what a real claude-cli completion sees. Same gate and cost as above (one
// short haiku call).
func TestLiveInheritedSoulReachesCompletion(t *testing.T) {
	if os.Getenv("TIONHARNESS_LIVE_INHERIT") != "1" {
		t.Skip("set TIONHARNESS_LIVE_INHERIT=1 to run the live inherited-soul check")
	}
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	if !rt.providers.ClaudeCLIAvailable() {
		t.Skip("claude CLI not on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	const marker = "PINEAPPLE"
	base, err := rt.db.CreateAgent(ctx, db.Agent{
		Name: "Base", Provider: "claude-cli", Model: "haiku", ThinkingLevel: "off",
		Soul: "You are inside an automated test. Answer in one short sentence and ALWAYS end your reply with the exact word " + marker + ".",
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := rt.db.DeriveAgent(ctx, base.ID, db.DeriveAgentOptions{Name: "Child"})
	if err != nil {
		t.Fatal(err)
	}
	effective, err := rt.db.GetAgent(ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(effective.Overrides) != 0 || effective.Soul != base.Soul {
		t.Fatalf("child should inherit the soul verbatim: overrides=%v", effective.Overrides)
	}
	resp, err := rt.guardedComplete(ctx, effective, providers.Request{
		Model:                  effective.Model,
		System:                 BuildSystemPrompt(effective),
		Messages:               []providers.Message{{Role: providers.RoleUser, Text: "What colour is the sky on a clear day?"}},
		MaxTokens:              128,
		CLIRestrictNativeTools: true,
	}, false)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	text := strings.TrimSpace(resp.Text)
	t.Logf("derived agent (model=%s) → %q", resp.Model, text)
	if !strings.Contains(strings.ToUpper(text), marker) {
		t.Fatalf("inherited soul did not reach the model: %q", text)
	}
}
