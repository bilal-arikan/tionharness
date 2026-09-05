package view

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

type fakeToolGroups struct{ groups []ToolGroup }

func (f fakeToolGroups) ToolGroups() []ToolGroup { return f.groups }

// subNodesFixture: two MCP servers, two tool groups, and one day of usage across
// two providers — everything the Araçlar and Bütçe nodes drill into.
func subNodesFixture() *Projector {
	return NewProjector(&fakeStore{
		mcp: []db.MCPServer{
			{ID: "M1", Name: "linear", Transport: db.MCPTransportStdio, Command: "linear-mcp", Enabled: true, CreatedAt: 10},
			{ID: "M2", Name: "old", Transport: db.MCPTransportHTTP, URL: "https://x/mcp", Enabled: false, CreatedAt: 20},
		},
		usage: []db.Usage{{
			Day: "2026-09-05",
			ByModel: map[string]db.KindStat{
				"anthropic|claude-opus-5":   {Calls: 3, InputTokens: 1000, OutputTokens: 200},
				"anthropic|claude-sonnet-5": {Calls: 1, InputTokens: 100, OutputTokens: 10},
				"openai|gpt":                {Calls: 2, InputTokens: 500, OutputTokens: 50},
			},
		}},
	}).WithSources(Sources{ToolGroups: fakeToolGroups{groups: []ToolGroup{
		{Key: "files", Tools: []string{"Read", "Write", "Bash"}},
		{Key: "search", Label: "Arama", Tools: []string{"WebSearch"}},
	}}})
}

func TestToolsChildrenListGroupsThenServers(t *testing.T) {
	ctx := context.Background()
	p := subNodesFixture()
	tools := Ref{Kind: KindTools, ID: ToolsRefID}
	if !IsExpandable(tools) || IsExpandable(Ref{Kind: KindTools, ID: ToolsRefID, Sub: "group:files"}) {
		t.Fatal("tools root expandable, a group is a leaf")
	}
	hs, err := p.Children(ctx, tools)
	if err != nil {
		t.Fatalf("children: %v", err)
	}
	wantSubs := []string{"group:files", "group:search", "mcp:M1", "mcp:M2"}
	if len(hs) != len(wantSubs) {
		t.Fatalf("children=%+v, want %d", hs, len(wantSubs))
	}
	for i, want := range wantSubs {
		if hs[i].Ref.Sub != want || hs[i].Ref.Kind != KindTools {
			t.Errorf("child %d = %+v, want sub %q", i, hs[i].Ref, want)
		}
	}
	if !strings.Contains(hs[0].Label, "files (3 araç)") || !strings.Contains(hs[1].Label, "Arama (1 araç)") {
		t.Errorf("group labels: %q / %q", hs[0].Label, hs[1].Label)
	}
	if !strings.Contains(hs[2].Label, "MCP linear [aktif]") || !strings.Contains(hs[3].Label, "[kapalı]") {
		t.Errorf("server labels: %q / %q", hs[2].Label, hs[3].Label)
	}
	// A leaf has no children and no error.
	if leaf, err := p.Children(ctx, Ref{Kind: KindTools, ID: ToolsRefID, Sub: "mcp:M1"}); err != nil || len(leaf) != 0 {
		t.Errorf("leaf children = %+v, %v", leaf, err)
	}
}

func TestToolsSubProjections(t *testing.T) {
	ctx := context.Background()
	p := subNodesFixture()
	group, err := p.Project(ctx, Ref{Kind: KindTools, ID: ToolsRefID, Sub: "group:files"}, LevelCard)
	if err != nil {
		t.Fatalf("group: %v", err)
	}
	if !strings.Contains(group.Header, "grup files · 3 araç") || !strings.Contains(group.Body, "Read, Write, Bash") {
		t.Errorf("group view: %q / %q", group.Header, group.Body)
	}
	if group.Ref.Sub != "group:files" {
		t.Errorf("ref sub lost: %+v", group.Ref)
	}
	server, err := p.Project(ctx, Ref{Kind: KindTools, ID: ToolsRefID, Sub: "mcp:M2"}, LevelCard)
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	if !strings.Contains(server.Header, "MCP old") || !strings.Contains(server.Header, "kapalı") ||
		!strings.Contains(server.Body, "https://x/mcp") {
		t.Errorf("server view: %q / %q", server.Header, server.Body)
	}
	for _, bad := range []string{"group:nope", "mcp:nope", "whatever"} {
		if _, err := p.Project(ctx, Ref{Kind: KindTools, ID: ToolsRefID, Sub: bad}, LevelCard); err == nil {
			t.Errorf("sub %q must error", bad)
		}
	}
	// The overview names the groups.
	overview, err := p.Project(ctx, Ref{Kind: KindTools, ID: ToolsRefID}, LevelCard)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if !strings.Contains(overview.Body, "yerleşik gruplar: files(3), Arama(1)") {
		t.Errorf("overview body: %q", overview.Body)
	}
}

func TestBudgetChildrenAreProvidersCostliestFirst(t *testing.T) {
	ctx := context.Background()
	p := subNodesFixture()
	budget := Ref{Kind: KindBudget, ID: BudgetRefID}
	if !IsExpandable(budget) || IsExpandable(Ref{Kind: KindBudget, ID: BudgetRefID, Sub: "provider:x"}) {
		t.Fatal("budget root expandable, a provider is a leaf")
	}
	hs, err := p.Children(ctx, budget)
	if err != nil {
		t.Fatalf("children: %v", err)
	}
	if len(hs) != 2 {
		t.Fatalf("children=%+v, want 2 providers", hs)
	}
	subs := []string{hs[0].Ref.Sub, hs[1].Ref.Sub}
	if !((subs[0] == "provider:anthropic" && subs[1] == "provider:openai") ||
		(subs[0] == "provider:openai" && subs[1] == "provider:anthropic")) {
		t.Errorf("provider subs = %v", subs)
	}
	for _, h := range hs {
		if h.Ref.Kind != KindBudget || !strings.Contains(h.Label, "model") {
			t.Errorf("handle %+v", h)
		}
		if strings.HasSuffix(h.Ref.Sub, "anthropic") && !strings.Contains(h.Label, "2 model") {
			t.Errorf("anthropic should carry 2 models: %q", h.Label)
		}
	}

	view, err := p.Project(ctx, Ref{Kind: KindBudget, ID: BudgetRefID, Sub: "provider:anthropic"}, LevelCard)
	if err != nil {
		t.Fatalf("provider view: %v", err)
	}
	if !strings.Contains(view.Header, "BUDGET · anthropic") || !strings.Contains(view.Header, "2 model") {
		t.Errorf("header: %q", view.Header)
	}
	if strings.Contains(view.Body, "gpt") || !strings.Contains(view.Body, "claude-opus-5") {
		t.Errorf("body must hold only the provider's rows: %q", view.Body)
	}
	if _, err := p.Project(ctx, Ref{Kind: KindBudget, ID: BudgetRefID, Sub: "provider:none"}, LevelCard); err == nil {
		t.Error("unknown provider must error")
	}
	// No usage today: an empty budget node, not an error.
	empty := NewProjector(&fakeStore{})
	if hs, err := empty.Children(ctx, budget); err != nil || len(hs) != 0 {
		t.Errorf("empty budget children = %+v, %v", hs, err)
	}
}

func TestGraphIncludesToolAndBudgetLeaves(t *testing.T) {
	g, err := subNodesFixture().Graph(context.Background())
	if err != nil {
		t.Fatalf("graph: %v", err)
	}
	tools := Ref{Kind: KindTools, ID: ToolsRefID}
	budget := Ref{Kind: KindBudget, ID: BudgetRefID}
	for _, want := range []Ref{
		{Kind: KindTools, ID: ToolsRefID, Sub: "group:files"},
		{Kind: KindTools, ID: ToolsRefID, Sub: "mcp:M1"},
		{Kind: KindBudget, ID: BudgetRefID, Sub: "provider:openai"},
	} {
		if !hasHandleRef(g.Nodes, want) {
			t.Errorf("node %s missing", want)
		}
		parent := tools
		if want.Kind == KindBudget {
			parent = budget
		}
		if !hasEdge(g.Edges, parent, want) {
			t.Errorf("edge %s -> %s missing", parent, want)
		}
	}
}
