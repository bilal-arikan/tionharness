package insight

import (
	"os"
	"path/filepath"
	"strings"
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
		"cache-cooling-waste", "workspace-tuning",
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
	// The two cache lenses split the SAME event type by its attributed cause, so
	// their compound minCount keys (which contain a colon) must survive parsing —
	// a dropped threshold would make the lens match every session.
	if cc, _ := reg.Get("context-cache-opt"); cc.Prefilter.MinCount["cache_break:prompt-or-tools-changed"] != 2 {
		t.Fatalf("context-cache-opt minCount not parsed: %v", cc.Prefilter.MinCount)
	}
	if cw, _ := reg.Get("cache-cooling-waste"); cw.Prefilter.MinCount["cache_break:ttl-or-server-eviction"] != 2 {
		t.Fatalf("cache-cooling-waste minCount not parsed: %v", cw.Prefilter.MinCount)
	}
	// Both cache lenses must request the cache slice surface — without it the
	// analyzer receives no cache evidence at all and can only guess.
	for _, id := range []string{"context-cache-opt", "cache-cooling-waste"} {
		if l, _ := reg.Get(id); !hasScope(l, ScopeCache) {
			t.Fatalf("lens %q must declare scope: [... cache], got %v", id, l.Scope)
		}
	}

	// workspace-tuning is the lens that pushes workspace-opt findings at ASSETS
	// (skills/agents/tools/hooks) instead of at CLAUDE.md. It only earns that job
	// if it is enabled, routed to workspace-opt and actually fires on friction
	// sessions — a prefilter that parsed to nothing would silently disable it.
	wt, ok := reg.Get("workspace-tuning")
	if !ok {
		t.Fatal("workspace-tuning lens must load")
	}
	if wt.Channel != ChannelWorkspaceOpt || !wt.Enabled {
		t.Fatalf("workspace-tuning must be an enabled workspace-opt lens: %+v", wt)
	}
	if len(wt.Prefilter.RequiresAny) == 0 {
		t.Fatalf("workspace-tuning prefilter did not parse: %+v", wt.Prefilter)
	}
	if !wt.Prefilter.Match(SessionSignals{DebugEvents: map[string]int{"error": 1}}) {
		t.Fatal("workspace-tuning must match a session carrying errors")
	}
	if wt.Prompt == "" {
		t.Fatal("workspace-tuning body (analysis instruction) is empty")
	}
}

// A minCount signal name may itself contain a colon (a cache break narrowed to
// its attributed cause). Splitting the pair on its FIRST colon dropped the
// threshold silently, which is worse than failing: the lens then matched EVERY
// session and burned analyzer calls on all of them.
func TestParseMinCountCompoundKey(t *testing.T) {
	got := parseMinCount(`{ "cache_break:ttl-or-server-eviction": 2, tool: 12 }`)
	if got["cache_break:ttl-or-server-eviction"] != 2 {
		t.Errorf("compound key not parsed: %v", got)
	}
	if got["tool"] != 12 {
		t.Errorf("plain key regressed: %v", got)
	}
	if len(got) != 2 {
		t.Errorf("unexpected extra keys: %v", got)
	}
}

// mergeLens must adopt the SHIPPED mechanics (prefilter/scope/body) while keeping
// the two keys that are decisions about this install. Preserving all frontmatter
// the way skills does would freeze prefilter/scope corrections — the failure that
// left the cache lens prefiltering on an event it was never shown.
func TestMergeLensAdoptsMechanicsKeepsUserKeys(t *testing.T) {
	shipped := []byte("---\nid: x\nchannel: workspace-opt\nenabled: true\nscope: [debug, cache]\nprefilter:\n  minCount: { \"cache_break:ttl-or-server-eviction\": 2 }\n---\nNEW BODY\n")
	onDisk := []byte("---\nid: x\nchannel: workspace-opt\nenabled: false\nmodel: claude-cli\nscope: [debug]\nprefilter:\n  minCount: { cache_break: 2 }\n---\nOLD BODY\n")

	l, err := ParseLens(string(mergeLens(onDisk, shipped)), "x.md")
	if err != nil {
		t.Fatal(err)
	}
	// User keys carried over.
	if l.Enabled {
		t.Error("the user's enabled:false must survive the refresh")
	}
	if l.Model != "claude-cli" {
		t.Errorf("the user's model choice must survive: %q", l.Model)
	}
	// Shipped mechanics adopted.
	if !hasScope(l, ScopeCache) {
		t.Errorf("shipped scope not adopted: %v", l.Scope)
	}
	if l.Prefilter.MinCount["cache_break:ttl-or-server-eviction"] != 2 {
		t.Errorf("shipped prefilter not adopted: %v", l.Prefilter.MinCount)
	}
	if l.Prompt != "NEW BODY" {
		t.Errorf("shipped body not adopted: %q", l.Prompt)
	}
}

// A lens the user never configured must come through as the shipped file verbatim
// (no injected keys).
func TestMergeLensWithoutUserKeys(t *testing.T) {
	shipped := []byte("---\nid: x\nchannel: workspace-opt\n---\nBODY\n")
	if got := string(mergeLens([]byte("---\nid: x\nchannel: workspace-opt\n---\nOLD\n"), shipped)); got != string(shipped) {
		t.Errorf("merge should be a no-op without user keys:\n got %q\nwant %q", got, shipped)
	}
}

// SetFrontmatterScalar rewrites a top-level key IN PLACE and must leave the
// nested prefilter block byte-identical — an in-place edit is what keeps the
// merged file's shape stable, and touching an indented line would corrupt the
// block it belongs to.
//
// (Note the reader's own limit: parseFrontmatter is flat, so a nested sub-key
// SHARING a top-level key's name would shadow it. No lens key does, and the
// writer above is what stops one from being created accidentally.)
func TestSetFrontmatterScalarInPlaceKeepsNestedBlock(t *testing.T) {
	raw := []byte("---\nid: x\nchannel: workspace-opt\nenabled: true\nprefilter:\n  minCount: { tool: 3 }\n---\nbody\n")
	out := string(SetFrontmatterScalar(raw, "enabled", "false"))

	if !strings.Contains(out, "  minCount: { tool: 3 }") {
		t.Errorf("nested prefilter block was mangled:\n%s", out)
	}
	if strings.Contains(out, "enabled: true") {
		t.Errorf("old value left behind:\n%s", out)
	}
	// In place, not appended: the key keeps its original position.
	if !strings.Contains(out, "channel: workspace-opt\nenabled: false\nprefilter:") {
		t.Errorf("key was not rewritten in place:\n%s", out)
	}
	l, err := ParseLens(out, "x.md")
	if err != nil {
		t.Fatal(err)
	}
	if l.Enabled || l.Prefilter.MinCount["tool"] != 3 {
		t.Errorf("re-parse lost data: enabled=%v minCount=%v", l.Enabled, l.Prefilter.MinCount)
	}

	// A key that is absent is inserted (used when the user never set `model`).
	added := string(SetFrontmatterScalar(raw, "model", "claude-cli"))
	if l2, err := ParseLens(added, "x.md"); err != nil || l2.Model != "claude-cli" {
		t.Errorf("absent key not inserted: %v %q", err, l2.Model)
	}
}

// HasDefault gates the UI's restore button; a blank/path-bearing id must never
// resolve (a blank one would open the defaults DIRECTORY, which succeeds).
func TestHasDefault(t *testing.T) {
	if !HasDefault("cache-cooling-waste") {
		t.Error("a shipped lens must report a default")
	}
	for _, id := range []string{"", "my-own-lens", "../escape", "sub/dir"} {
		if HasDefault(id) {
			t.Errorf("id %q must not report a shipped default", id)
		}
	}
}

// RestoreDefault brings a mangled shipped lens back and makes it parse again.
func TestRestoreDefault(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "cache-cooling-waste.md")
	if err := os.WriteFile(path, []byte("---\nid: cache-cooling-waste\nchannel: app-fix\n---\nWRECKED\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RestoreDefault(dir, "cache-cooling-waste"); err != nil {
		t.Fatal(err)
	}
	reg, errs := LoadRegistry(dir)
	if len(errs) != 0 {
		t.Fatalf("registry errors after restore: %v", errs)
	}
	l, ok := reg.Get("cache-cooling-waste")
	if !ok || l.Channel != ChannelWorkspaceOpt || !hasScope(l, ScopeCache) {
		t.Fatalf("restored lens is not the shipped one: %+v", l)
	}
	if err := RestoreDefault(dir, "not-a-shipped-lens"); err == nil {
		t.Error("restoring an unshipped id must fail")
	}
}
