package api

import (
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// TestEstimateToolCatalog verifies the tool-catalog token estimate accounts for
// each tool's name, description and input schema (plus per-tool framing), so the
// session context meter reflects the always-sent tool/MCP cost rather than zero.
func TestEstimateToolCatalog(t *testing.T) {
	if got := estimateToolCatalog(nil); got != 0 {
		t.Fatalf("empty catalog should cost 0 tokens, got %d", got)
	}

	defs := []providers.ToolDef{
		{
			Name:        "get_current_time",
			Description: "Returns the current time in the requested timezone.",
			InputSchema: []byte(`{"type":"object","properties":{"timezone":{"type":"string"}}}`),
		},
		{
			Name:        "http_get",
			Description: "Fetches a URL and returns the response body.",
			InputSchema: []byte(`{"type":"object","properties":{"url":{"type":"string"}},"required":["url"]}`),
		},
	}

	got := estimateToolCatalog(defs)
	if got <= 0 {
		t.Fatalf("non-empty catalog should cost > 0 tokens, got %d", got)
	}

	// A larger catalog must never cost fewer tokens than a subset of it.
	single := estimateToolCatalog(defs[:1])
	if got <= single {
		t.Fatalf("two tools (%d) should cost more than one (%d)", got, single)
	}
}
