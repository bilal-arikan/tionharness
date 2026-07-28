package conversation

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// capturingProvider records the last request it received, so a test can assert
// what was actually sent to the model, and returns a fixed handoff body.
type capturingProvider struct {
	last providers.Request
	out  string
}

func (p *capturingProvider) Name() string { return "capture" }
func (p *capturingProvider) Complete(_ context.Context, req providers.Request) (*providers.Response, error) {
	p.last = req
	return &providers.Response{Text: p.out, StopReason: providers.StopEndTurn}, nil
}

func TestBuildHandoff_InjectsEnvAndTranscript(t *testing.T) {
	cp := &capturingProvider{out: "1. Objective / Primary Intent: ship the feature"}
	env := HandoffEnv{
		WorkingDir: "/work/proj",
		GitBranch:  "feature/parser",
		Artifacts:  "  - spec.md (markdown, id ART1)",
	}
	rendered := RenderTranscript([]db.Message{
		{Role: providers.RoleUser, Text: "start the parser"},
		{Role: providers.RoleAssistant, Text: "working on it"},
	})

	out, err := BuildHandoff(context.Background(), nil, cp, db.Agent{Model: "m"}, "prev summary", rendered, env, "")
	if err != nil {
		t.Fatalf("BuildHandoff: %v", err)
	}
	if !strings.Contains(out, "Objective") {
		t.Errorf("handoff body not returned, got %q", out)
	}

	sent := cp.last.Messages[0].Text
	for _, want := range []string{"prev summary", "start the parser", "/work/proj", "feature/parser", "spec.md"} {
		if !strings.Contains(sent, want) {
			t.Errorf("prompt missing %q\n---\n%s", want, sent)
		}
	}
	if cp.last.MaxTokens != compactMaxOutputTokens {
		t.Errorf("MaxTokens = %d, want %d", cp.last.MaxTokens, compactMaxOutputTokens)
	}
}

func TestBuildHandoff_EmptyTranscriptErrors(t *testing.T) {
	cp := &capturingProvider{out: "x"}
	if _, err := BuildHandoff(context.Background(), nil, cp, db.Agent{Model: "m"}, "", "   ", HandoffEnv{}, ""); err == nil {
		t.Fatal("expected an error for an empty transcript")
	}
}

func TestHandoffEnv_RenderedOmitsEmpty(t *testing.T) {
	got := HandoffEnv{WorkingDir: "/w"}.rendered()
	if !strings.Contains(got, "/w") {
		t.Errorf("rendered env missing working dir: %q", got)
	}
	if strings.Contains(got, "Git branch") {
		t.Errorf("empty git branch should be omitted: %q", got)
	}
	if (HandoffEnv{}).rendered() != "(no environment snapshot)" {
		t.Errorf("empty env should render the placeholder, got %q", (HandoffEnv{}).rendered())
	}
}
