package ingest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/market"
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

func TestMapCCModel(t *testing.T) {
	cases := []struct {
		in, prov, model string
		warn            bool
	}{
		{"haiku", "claude-cli", "claude-haiku-4-5-20251001", false},
		{"sonnet", "claude-cli", "claude-sonnet-5", false},
		{"opus", "claude-cli", "claude-opus-4-8", false},
		{"claude-3-5-sonnet-20241022", "claude-cli", "claude-sonnet-5", false}, // family substring
		{"inherit", "", "", false},
		{"", "", "", false},
		{"gpt-4o", "", "", true},
	}
	for _, c := range cases {
		prov, model, warn := mapCCModel(c.in)
		if prov != c.prov || model != c.model || (warn != "") != c.warn {
			t.Errorf("mapCCModel(%q) = (%q,%q,warn=%v), want (%q,%q,warn=%v)", c.in, prov, model, warn != "", c.prov, c.model, c.warn)
		}
	}
}

func TestAgentModelMappedIntoPack(t *testing.T) {
	root := t.TempDir()
	write(t, root, "agents/finder.md", "---\nname: Finder\ndescription: d\nmodel: haiku\ntools: [Read]\n---\nbody")
	packs, _, _, err := BuildPacks("local", root, nil, Options{})
	if err != nil || len(packs) != 1 {
		t.Fatalf("build: %v packs=%d", err, len(packs))
	}
	a := packs[0].Payload.Agent
	if a.Provider != "claude-cli" || a.Model != "claude-haiku-4-5-20251001" {
		t.Errorf("agent model not mapped: provider=%q model=%q", a.Provider, a.Model)
	}
}

func TestParseSimpleTOML(t *testing.T) {
	raw := "" +
		"description = \"Switch level\"\n" +
		"# a comment\n" +
		"prompt = \"\"\"\nLine one {{args}}\nLine two\n\"\"\"\n" +
		"name = 'caveman'\n"
	kv := parseSimpleTOML(raw)
	if kv["description"] != "Switch level" {
		t.Errorf("description = %q", kv["description"])
	}
	if kv["name"] != "caveman" {
		t.Errorf("name = %q", kv["name"])
	}
	if !containsStr(kv["prompt"], "Line one {{args}}") || !containsStr(kv["prompt"], "Line two") {
		t.Errorf("prompt multiline = %q", kv["prompt"])
	}
}

func TestTOMLCommandImported(t *testing.T) {
	root := t.TempDir()
	write(t, root, "commands/caveman.toml",
		"description = \"Switch caveman level\"\nprompt = \"Switch to caveman {{args}} mode. Be terse.\"\n")

	sr, err := Scan("local", root)
	if err != nil {
		t.Fatal(err)
	}
	if len(sr.Items) != 1 || sr.Items[0].Kind != market.KindSkill || sr.Items[0].Slug != "caveman" {
		t.Fatalf("toml command not discovered as skill: %+v", sr.Items)
	}
	if len(sr.Items[0].Warnings) == 0 {
		t.Errorf("expected {{args}} warning")
	}
	packs, _, _, err := BuildPacks("local", root, nil, Options{})
	if err != nil || len(packs) != 1 {
		t.Fatalf("build: %v packs=%d", err, len(packs))
	}
	if !containsStr(packs[0].Payload.Skill.Body, "Switch to caveman {{args}} mode") {
		t.Errorf("toml prompt not in skill body: %q", packs[0].Payload.Skill.Body)
	}
}

func TestMarketplaceRootsTargeting(t *testing.T) {
	root := t.TempDir()
	// marketplace.json pins the real plugin to ./pkg; a mirror lives under dist-like
	// subtree that should be excluded by the targeted root.
	write(t, root, ".claude-plugin/marketplace.json",
		`{"name":"mp","plugins":[{"name":"pkg","source":"./pkg"}]}`)
	write(t, root, "pkg/skills/alpha/SKILL.md", "---\nname: Alpha\ndescription: a\n---\nbody")
	write(t, root, "other/skills/beta/SKILL.md", "---\nname: Beta\ndescription: b\n---\nbody")

	sr, err := Scan("local", root)
	if err != nil {
		t.Fatal(err)
	}
	// Only the manifest-pinned pkg/ skill is discovered; other/ is excluded.
	if len(sr.Items) != 1 || sr.Items[0].Slug != "alpha" {
		t.Fatalf("marketplace targeting failed: %+v", sr.Items)
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
