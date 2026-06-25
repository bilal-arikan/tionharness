package ingest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/market"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanAndBuildPacksLocal(t *testing.T) {
	root := t.TempDir()
	write(t, root, "skills/alpha/SKILL.md", "---\nname: Alpha\ndescription: first\n---\nbody")
	write(t, root, "skills/alpha/references/r.md", "ref")
	write(t, root, "skills/beta/SKILL.md", "---\nname: Beta\ndescription: second\ncontext: fork\n---\nbody")

	sr, err := Scan("local", root)
	if err != nil {
		t.Fatal(err)
	}
	if len(sr.Items) != 2 {
		t.Fatalf("want 2 items, got %d", len(sr.Items))
	}
	// beta uses context: fork → should carry a warning in the preview.
	var beta *Discovered
	for i := range sr.Items {
		if sr.Items[i].Slug == "beta" {
			beta = &sr.Items[i]
		}
	}
	if beta == nil || len(beta.Warnings) == 0 {
		t.Errorf("expected beta to carry a context:fork warning")
	}

	// Build only alpha, namespaced, and verify the pack carries nested files.
	packs, skipped, _, err := BuildPacks("local", root, []string{"skill:skills/alpha"}, Options{SlugPrefix: "pack", Shared: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 0 {
		t.Fatalf("unexpected skips: %v", skipped)
	}
	if len(packs) != 1 {
		t.Fatalf("want 1 pack, got %d", len(packs))
	}
	p := packs[0]
	if p.Kind != market.KindSkill || p.Payload.Skill.Slug != "pack-alpha" {
		t.Errorf("pack slug = %q (kind %q)", p.Payload.Skill.Slug, p.Kind)
	}
	if _, ok := p.Files["references/r.md"]; !ok {
		t.Errorf("nested file not carried into pack: %v", p.Files)
	}
	// access: shared should be reflected in the rendered body.
	if !containsStr(p.Payload.Skill.Body, "access: shared") {
		t.Errorf("shared not rendered into body")
	}
}

func TestAdaptersAgentCommandMCPAndDedup(t *testing.T) {
	root := t.TempDir()
	// A subagent.
	write(t, root, "agents/finder.md", "---\nname: Finder\ndescription: locates code\ntools: [Read, Grep]\nmodel: haiku\n---\nYou find things.")
	// A markdown slash command → becomes a skill.
	write(t, root, "commands/deploy.md", "---\ndescription: deploy the app\n---\nRun the deploy steps.")
	// An MCP config with one server.
	write(t, root, ".mcp.json", `{"mcpServers":{"fs":{"command":"npx","args":["-y","fs-mcp"],"env":{"ROOT":"/tmp"}}}}`)
	// A real skill.
	write(t, root, "skills/alpha/SKILL.md", "---\nname: Alpha\ndescription: a\n---\nbody")
	// A mirrored duplicate of the skill under plugins/ — must be deduped away.
	write(t, root, "plugins/pkg/skills/alpha/SKILL.md", "---\nname: Alpha\ndescription: a\n---\nbody")

	sr, err := Scan("local", root)
	if err != nil {
		t.Fatal(err)
	}
	byKind := map[string]int{}
	for _, it := range sr.Items {
		byKind[it.Kind]++
	}
	// 1 agent, 1 mcp, 2 skills (alpha + deploy command); the plugins/ mirror deduped.
	if byKind["agent"] != 1 || byKind["mcp"] != 1 || byKind["skill"] != 2 {
		t.Fatalf("unexpected kinds: %v (items=%d)", byKind, len(sr.Items))
	}

	// Build all and check each pack kind/payload.
	packs, skipped, _, err := BuildPacks("local", root, nil, Options{})
	if err != nil || len(skipped) != 0 {
		t.Fatalf("build err=%v skipped=%v", err, skipped)
	}
	var agentP, mcpP, deployP *market.Pack
	for i := range packs {
		switch {
		case packs[i].Kind == market.KindAgent:
			agentP = &packs[i]
		case packs[i].Kind == market.KindMCP:
			mcpP = &packs[i]
		case packs[i].Kind == market.KindSkill && packs[i].Payload.Skill.Slug == "deploy":
			deployP = &packs[i]
		}
	}
	if agentP == nil || agentP.Payload.Agent.Name != "Finder" || agentP.Payload.Agent.AllowedTools != "Read, Grep" {
		t.Errorf("agent pack wrong: %+v", agentP)
	}
	if mcpP == nil || mcpP.Payload.MCP.Command != "npx" || mcpP.Payload.MCP.Transport != "stdio" {
		t.Errorf("mcp pack wrong: %+v", mcpP)
	}
	if deployP == nil || !containsStr(deployP.Payload.Skill.Body, "Run the deploy steps.") {
		t.Errorf("command→skill pack wrong: %+v", deployP)
	}
}

func containsStr(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
