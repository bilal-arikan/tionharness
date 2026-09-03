package conversation

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// captureProvider records the request of its single Complete call.
type captureProvider struct {
	name string
	req  providers.Request
	hits int
}

func (p *captureProvider) Name() string { return p.name }
func (p *captureProvider) Complete(_ context.Context, req providers.Request) (*providers.Response, error) {
	p.req = req
	p.hits++
	return &providers.Response{Text: "folded", Model: req.Model}, nil
}

func openFoldTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

// TestFoldTargetOverridesModelAndProvider pins the two override shapes and the
// tool-less contract of a fold request: a CLI transport must see
// CLIRestrictNativeTools so it drops its built-in tool menu.
func TestFoldTargetOverridesModelAndProvider(t *testing.T) {
	d := openFoldTestDB(t)
	session := &captureProvider{name: "session"}
	agent := db.Agent{ID: "A1", Provider: "claude-cli", Model: "opus"}

	// No override: the handed provider + agent model, tool-less.
	if _, err := summarizeRendered(context.Background(), d, session, agent, "", "u: hi\n"); err != nil {
		t.Fatal(err)
	}
	if session.req.Model != "opus" || !session.req.CLIRestrictNativeTools || len(session.req.CLINativeTools) != 0 {
		t.Fatalf("plain fold request = model %q restrict=%v tools=%v; want opus, restricted, no tools",
			session.req.Model, session.req.CLIRestrictNativeTools, session.req.CLINativeTools)
	}

	// Agent-only override (nil provider): same provider object, cheaper model.
	cheap := agent
	cheap.Model = "haiku"
	ctx := WithFoldTarget(context.Background(), nil, cheap)
	if _, err := summarizeRendered(ctx, d, session, agent, "", "u: hi\n"); err != nil {
		t.Fatal(err)
	}
	if session.hits != 2 || session.req.Model != "haiku" {
		t.Fatalf("agent-only override: hits=%d model=%q; want the same provider called with haiku", session.hits, session.req.Model)
	}

	// Full override: another provider object entirely.
	native := &captureProvider{name: "anthropic"}
	routed := db.Agent{ID: "A1", Provider: "anthropic", ProviderInstanceID: "anthropic-main", Model: "claude-haiku-4-5-20251001"}
	ctx = WithFoldTarget(context.Background(), native, routed)
	if _, err := summarizeRendered(ctx, d, session, agent, "", "u: hi\n"); err != nil {
		t.Fatal(err)
	}
	if session.hits != 2 || native.hits != 1 || native.req.Model != routed.Model {
		t.Fatalf("provider override: session hits=%d native hits=%d model=%q", session.hits, native.hits, native.req.Model)
	}
	if got, ok := FoldTargetAgent(ctx); !ok || got.Provider != "anthropic" {
		t.Fatalf("FoldTargetAgent = %+v ok=%v", got, ok)
	}
}
