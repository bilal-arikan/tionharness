package insight

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleLens = `---
id: tool-errors
name: "Tool Errors"
description: "Find tool failures."
channel: app-fix
enabled: true
model: claude-cli
scope: [steps, debug]
prefilter:
  requiresAny: [error]
---

# Analysis Instruction

Do the analysis.
`

func TestParseLensValid(t *testing.T) {
	l, err := ParseLens(sampleLens, "C:/x/tool-errors.md")
	if err != nil {
		t.Fatal(err)
	}
	if l.ID != "tool-errors" || l.Channel != ChannelAppFix || !l.Enabled {
		t.Fatalf("bad parse: %+v", l)
	}
	if l.Model != "claude-cli" {
		t.Fatalf("model not parsed: %q", l.Model)
	}
	if len(l.Scope) != 2 {
		t.Fatalf("scope not parsed: %v", l.Scope)
	}
	// Nested prefilter block is read flat.
	if len(l.Prefilter.RequiresAny) != 1 || l.Prefilter.RequiresAny[0] != "error" {
		t.Fatalf("prefilter.requiresAny not parsed: %v", l.Prefilter.RequiresAny)
	}
	if l.Prompt == "" {
		t.Fatal("prompt body should not be empty")
	}
}

func TestParseMinCount(t *testing.T) {
	cases := map[string]map[string]int{
		"":                        nil,
		"{}":                      nil,
		"{ tool: 12 }":            {"tool": 12},
		"{cache_break: 2}":        {"cache_break": 2},
		"{ a: 1, b: 3 }":          {"a": 1, "b": 3},
		"{ 'use_skill': 4 }":      {"use_skill": 4},
		"{ bad, tool: 5, x: no }": {"tool": 5}, // malformed pairs skipped, valid kept
	}
	for in, want := range cases {
		got := parseMinCount(in)
		if len(got) != len(want) {
			t.Fatalf("parseMinCount(%q) = %v, want %v", in, got, want)
		}
		for k, v := range want {
			if got[k] != v {
				t.Fatalf("parseMinCount(%q)[%q] = %d, want %d", in, k, got[k], v)
			}
		}
	}
}

// TestMinCountLensParsesAndMatches proves a lens file's inline-map minCount is
// parsed and enforced by the prefilter (the Faz 4 tool-usage-opt/context-cache
// lenses depend on this).
func TestMinCountLensParsesAndMatches(t *testing.T) {
	raw := "---\nid: x\nchannel: workspace-opt\nprefilter:\n  minCount: { tool: 3 }\n---\nbody\n"
	l, err := ParseLens(raw, "x.md")
	if err != nil {
		t.Fatal(err)
	}
	if l.Prefilter.MinCount["tool"] != 3 {
		t.Fatalf("minCount not parsed: %v", l.Prefilter.MinCount)
	}
	if l.Prefilter.Match(SessionSignals{Tools: map[string]int{"tool": 2}}) {
		t.Fatal("2 < 3 should not match minCount")
	}
	if !l.Prefilter.Match(SessionSignals{Tools: map[string]int{"tool": 3}}) {
		t.Fatal("3 >= 3 should match minCount")
	}
}

func TestSetFrontmatterEnabled(t *testing.T) {
	// Existing enabled line is replaced in place.
	in := "---\nid: x\nchannel: workspace-opt\nenabled: true\n---\nbody\n"
	out := string(SetFrontmatterEnabled([]byte(in), false))
	l, err := ParseLens(out, "x.md")
	if err != nil || l.Enabled {
		t.Fatalf("enabled should be false after toggle: %v enabled=%v", err, l.Enabled)
	}
	// Missing enabled line is inserted (defaults true → we set false).
	in2 := "---\nid: y\nchannel: app-fix\n---\nbody\n"
	out2 := string(SetFrontmatterEnabled([]byte(in2), false))
	l2, err := ParseLens(out2, "y.md")
	if err != nil || l2.Enabled {
		t.Fatalf("inserted enabled:false should disable: %v enabled=%v\n%s", err, l2.Enabled, out2)
	}
	// No frontmatter → unchanged.
	if got := string(SetFrontmatterEnabled([]byte("no frontmatter"), true)); got != "no frontmatter" {
		t.Fatalf("content without frontmatter must be unchanged: %q", got)
	}
}

func TestParseLensInvalidChannelIsError(t *testing.T) {
	raw := "---\nid: x\nchannel: bogus\n---\nbody\n"
	if _, err := ParseLens(raw, "x.md"); err == nil {
		t.Fatal("an invalid channel must be an error, not silently defaulted")
	}
}

func TestParseLensEnabledDefaultsTrue(t *testing.T) {
	raw := "---\nid: x\nchannel: workspace-opt\n---\nbody\n"
	l, err := ParseLens(raw, "x.md")
	if err != nil {
		t.Fatal(err)
	}
	if !l.Enabled {
		t.Fatal("enabled should default to true when unset")
	}
	raw2 := "---\nid: y\nchannel: workspace-opt\nenabled: false\n---\nbody\n"
	l2, _ := ParseLens(raw2, "y.md")
	if l2.Enabled {
		t.Fatal("enabled: false should disable the lens")
	}
}

func TestEnsureDefaultsSeedsAndRegistryLoads(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	// The shipped tool-errors lens must now exist on disk.
	if _, err := os.Stat(filepath.Join(dir, "tool-errors.md")); err != nil {
		t.Fatalf("default lens not seeded: %v", err)
	}

	// A user edit must not be overwritten on re-seed.
	edited := filepath.Join(dir, "tool-errors.md")
	if err := os.WriteFile(edited, []byte("---\nid: tool-errors\nchannel: app-fix\n---\nEDITED\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(edited)
	if string(got) != "---\nid: tool-errors\nchannel: app-fix\n---\nEDITED\n" {
		t.Fatalf("EnsureDefaults overwrote a user edit: %q", got)
	}

	reg, errs := LoadRegistry(dir)
	if len(errs) != 0 {
		t.Fatalf("registry load errors: %v", errs)
	}
	if _, ok := reg.Get("tool-errors"); !ok {
		t.Fatal("registry should contain the tool-errors lens")
	}
	if len(reg.Enabled()) == 0 {
		t.Fatal("expected at least one enabled lens")
	}
}

// TestEmbeddedDefaultsAllParse is an end-to-end smoke test of the shipped lens
// content: every //go:embed default must seed, parse cleanly (no registry
// errors), and carry a valid channel. It also asserts the Faz 4 lenses exist and
// that their inline-map minCount prefilters actually parsed.
func TestEmbeddedDefaultsAllParse(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	reg, errs := LoadRegistry(dir)
	if len(errs) != 0 {
		t.Fatalf("shipped lenses must all parse; got errors: %v", errs)
	}

	want := []string{
		"tool-errors", "skill-usage-opt", "context-hygiene",
		"tool-usage-opt", "context-cache-opt", "lessons-mining",
	}
	for _, id := range want {
		l, ok := reg.Get(id)
		if !ok {
			t.Fatalf("expected shipped lens %q to load", id)
		}
		if l.Channel != ChannelAppFix && l.Channel != ChannelWorkspaceOpt {
			t.Fatalf("lens %q has invalid channel %q", id, l.Channel)
		}
	}
	if len(reg.List()) != len(want) {
		t.Fatalf("expected %d shipped lenses, got %d", len(want), len(reg.List()))
	}

	// Faz 4 minCount prefilters must have parsed from their inline-map form.
	if tu, _ := reg.Get("tool-usage-opt"); tu.Prefilter.MinCount["tool"] != 12 {
		t.Fatalf("tool-usage-opt minCount not parsed: %v", tu.Prefilter.MinCount)
	}
	if cc, _ := reg.Get("context-cache-opt"); cc.Prefilter.MinCount["cache_break"] != 2 {
		t.Fatalf("context-cache-opt minCount not parsed: %v", cc.Prefilter.MinCount)
	}
}
