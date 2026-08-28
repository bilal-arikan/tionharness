package tools

import (
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// def is a small helper so the tables below read as data, not construction noise.
func def(name, desc, schema string, examples ...string) providers.ToolDef {
	d := providers.ToolDef{Name: name, Description: desc}
	if schema != "" {
		d.InputSchema = json.RawMessage(schema)
	}
	for _, e := range examples {
		d.Examples = append(d.Examples, json.RawMessage(e))
	}
	return d
}

const (
	tinySchema = `{"type":"object","properties":{"a":{"type":"string"}}}`
	bigSchema  = `{"type":"object","properties":{"a":{"type":"string","description":"a fairly long description of the first parameter"},"b":{"type":"integer","description":"another long description that adds a lot of schema text"},"c":{"type":"array","items":{"type":"string"},"description":"and a third one, to make this schema clearly larger"}}}`
)

// Assertions here are RELATIVE (ordering, monotonicity) rather than absolute
// token counts: the estimator is an approximation and its constants may change.
func TestFullSchemaTokens(t *testing.T) {
	if got := FullSchemaTokens(nil); got != 0 {
		t.Fatalf("empty catalog must cost nothing, got %d", got)
	}
	small := FullSchemaTokens([]providers.ToolDef{def("t", "does a thing", tinySchema)})
	big := FullSchemaTokens([]providers.ToolDef{def("t", "does a thing", bigSchema)})
	if small <= 0 {
		t.Fatalf("a real tool must cost something, got %d", small)
	}
	if big <= small {
		t.Fatalf("larger schema must cost more: big=%d small=%d", big, small)
	}
	two := FullSchemaTokens([]providers.ToolDef{
		def("t", "does a thing", tinySchema),
		def("u", "does another thing", tinySchema),
	})
	if two <= small {
		t.Fatalf("two tools must cost more than one: two=%d one=%d", two, small)
	}
}

// Examples are folded into the FULL schema (foldExamples) and therefore raise the
// eager cost — but they must not leak into the cheaper catalog tiers.
func TestExamplesRaiseFullCostOnly(t *testing.T) {
	plain := def("t", "does a thing", tinySchema)
	withEx := def("t", "does a thing", tinySchema, `{"a":"one"}`, `{"a":"two"}`)

	if TierTokens(withEx, VisibilityFull) <= TierTokens(plain, VisibilityFull) {
		t.Fatal("examples must raise the full-tier cost")
	}
	if TierTokens(withEx, VisibilitySummary) != TierTokens(plain, VisibilitySummary) {
		t.Fatal("examples must not affect the summary tier")
	}

	// Pricing an already-folded def must not double-count the examples.
	folded := foldExamples(withEx)
	if TierTokens(folded, VisibilityFull) != TierTokens(withEx, VisibilityFull) {
		t.Fatal("folding is idempotent; cost must not change")
	}
}

func TestTierTokensOrdering(t *testing.T) {
	d := def("some_tool_name", "a description long enough to matter here", bigSchema)
	full := TierTokens(d, VisibilityFull)
	summary := TierTokens(d, VisibilitySummary)
	nameOnly := TierTokens(d, VisibilityNameOnly)
	hidden := TierTokens(d, VisibilityHidden)

	cases := []struct {
		name       string
		lower, hi  int
		lowerLabel string
	}{
		{"hidden < name-only", hidden, nameOnly, "hidden"},
		{"name-only < summary", nameOnly, summary, "name-only"},
		{"summary < full", summary, full, "summary"},
	}
	for _, c := range cases {
		if c.lower >= c.hi {
			t.Errorf("%s: %s=%d not below %d", c.name, c.lowerLabel, c.lower, c.hi)
		}
	}
	if hidden != 0 {
		t.Errorf("hidden tier must be free, got %d", hidden)
	}
	if unknown := TierTokens(d, "no-such-tier"); unknown != 0 {
		t.Errorf("unknown tier must price as hidden, got %d", unknown)
	}
}

func TestCurrentTokensUsesResolvedTier(t *testing.T) {
	defs := []providers.ToolDef{
		def("eager_tool", "shipped every turn", bigSchema),
		def("lazy_tool", "pulled on demand", bigSchema),
	}
	tiers := map[string]string{"eager_tool": VisibilityFull, "lazy_tool": VisibilityHidden}
	current := CurrentTokens(defs, func(name string) string { return tiers[name] })
	full := FullSchemaTokens(defs)

	if current >= full {
		t.Fatalf("a hidden tool must make the current cost cheaper than all-full: current=%d full=%d", current, full)
	}
	if current != TierTokens(defs[0], VisibilityFull) {
		t.Fatalf("current cost must be exactly the eager tool's full cost, got %d", current)
	}

	allFull := CurrentTokens(defs, func(string) string { return VisibilityFull })
	if allFull != full {
		t.Fatalf("everything at full must equal FullSchemaTokens: %d != %d", allFull, full)
	}
}

// A nil resolver is a programming error — reporting zero cost would silently
// understate every group in the UI.
func TestCurrentTokensNilResolverPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic for a nil tier resolver")
		}
	}()
	CurrentTokens([]providers.ToolDef{def("t", "d", tinySchema)}, nil)
}
