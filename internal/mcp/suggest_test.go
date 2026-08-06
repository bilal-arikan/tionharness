package mcp

import (
	"strings"
	"testing"
)

func TestSuggestServers(t *testing.T) {
	known := []string{"codebase-memory-mcp", "playwright", "filesystem", "github", "slack"}

	cases := []struct {
		name string
		want string
		// expect contains the known names the suggestion must include, in order.
		expect []string
	}{
		{
			name:   "underscore vs hyphen namespace guess",
			want:   "codebase_memory",
			expect: []string{"codebase-memory-mcp"},
		},
		{
			name:   "exact match wins",
			want:   "playwright",
			expect: []string{"playwright"},
		},
		{
			name:   "case-insensitive",
			want:   "Codebase-Memory-MCP",
			expect: []string{"codebase-memory-mcp"},
		},
		{
			name:   "garbage yields nothing",
			want:   "zzzzzzzzzzzz",
			expect: nil,
		},
		{
			name:   "empty want yields nothing",
			want:   "",
			expect: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SuggestServers(tc.want, known)
			if len(got) != len(tc.expect) {
				t.Fatalf("SuggestServers(%q) = %v, want %v", tc.want, got, tc.expect)
			}
			for i, exp := range tc.expect {
				if got[i] != exp {
					t.Fatalf("SuggestServers(%q)[%d] = %q, want %q (full: %v)", tc.want, i, got[i], exp, got)
				}
			}
		})
	}
}

func TestSuggestServersCapsAtThree(t *testing.T) {
	// A tight family of near-identical names must not flood the hint.
	known := []string{"alpha-a", "alpha-b", "alpha-c", "alpha-d", "alpha-e"}
	got := SuggestServers("alpha", known)
	if len(got) > 3 {
		t.Fatalf("suggestion capped at 3, got %d: %v", len(got), got)
	}
}

func TestUnknownServerErrSuggests(t *testing.T) {
	cfgByServer := map[string]ServerConfig{
		"codebase-memory-mcp": {Name: "codebase-memory-mcp"},
		"playwright":          {Name: "playwright"},
	}
	err := unknownServerErr("codebase_memory", "codebase_memory__search_code", cfgByServer)
	msg := err.Error()
	if !strings.Contains(msg, `no server "codebase_memory" for tool "codebase_memory__search_code"`) {
		t.Fatalf("error lost the original detail: %q", msg)
	}
	if !strings.Contains(msg, "did you mean codebase-memory-mcp") {
		t.Fatalf("error missing suggestion: %q", msg)
	}
}

func TestNameSimilarity(t *testing.T) {
	if s := nameSimilarity("codebase_memory", "codebase-memory-mcp"); s < 0.5 {
		t.Fatalf("expected close similarity for underscore/hyphen variant, got %f", s)
	}
	if s := nameSimilarity("codebase-memory-mcp", "codebase-memory-mcp"); s != 1.0 {
		t.Fatalf("exact match must be 1.0, got %f", s)
	}
	if s := nameSimilarity("", "anything"); s != 0.0 {
		t.Fatalf("empty must be 0.0, got %f", s)
	}
	if s := nameSimilarity("zzzz", "aaaa"); s >= 0.5 {
		t.Fatalf("unrelated names must fall below threshold, got %f", s)
	}
}
