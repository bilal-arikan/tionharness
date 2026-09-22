package indexstate

import "testing"

func TestDecideCreatesWhenNoStoreExists(t *testing.T) {
	action, phase := Decide(Observed{Exists: false}, Desired{Embedding: "local/m", ToolVersion: "1.0.0"})
	if action != ActionCreate || phase != PhaseMissing {
		t.Fatalf("action=%q phase=%q, want create/missing", action, phase)
	}
}

func TestDecideRebuildsWhenTheEmbeddingModelChanged(t *testing.T) {
	// Vectors from two different models do not share a space: refreshing INTO
	// such a store mixes them and every later search silently returns nonsense.
	obs := Observed{Exists: true, Embedding: "local/potion-code-16m-v2", ToolVersion: "1.0.0"}
	want := Desired{Embedding: "local/potion-base-8m", ToolVersion: "1.0.0"}
	if action, _ := Decide(obs, want); action != ActionRebuild {
		t.Fatalf("action=%q, want rebuild on a model change", action)
	}
}

func TestDecideRebuildsWhenTheToolVersionChanged(t *testing.T) {
	obs := Observed{Exists: true, Embedding: "local/m", ToolVersion: "1.0.0"}
	want := Desired{Embedding: "local/m", ToolVersion: "2.0.0"}
	if action, _ := Decide(obs, want); action != ActionRebuild {
		t.Fatalf("action=%q, want rebuild on a tool version change", action)
	}
}

func TestDecideRefreshesAStaleStoreWithoutDiscardingIt(t *testing.T) {
	obs := Observed{Exists: true, Embedding: "local/m", ToolVersion: "1.0.0", Stale: true}
	want := Desired{Embedding: "local/m", ToolVersion: "1.0.0"}
	action, phase := Decide(obs, want)
	if action != ActionRefresh || phase != PhaseStale {
		t.Fatalf("action=%q phase=%q, want refresh/stale", action, phase)
	}
}

func TestDecideDoesNothingForACurrentStore(t *testing.T) {
	obs := Observed{Exists: true, Embedding: "local/m", ToolVersion: "1.0.0"}
	want := Desired{Embedding: "local/m", ToolVersion: "1.0.0"}
	action, phase := Decide(obs, want)
	if action != "" || phase != PhaseReady {
		t.Fatalf("action=%q phase=%q, want no action/ready", action, phase)
	}
}

func TestDecideTreatsUnknownMetadataAsNoRebuild(t *testing.T) {
	// An unreadable manifest is UNKNOWN, not "different". Rebuilding on unknown
	// would re-embed a whole repository on every start.
	cases := []struct {
		name string
		obs  Observed
		want Desired
	}{
		{"store metadata missing", Observed{Exists: true}, Desired{Embedding: "local/m", ToolVersion: "1.0.0"}},
		{"version probe failed", Observed{Exists: true, Embedding: "local/m", ToolVersion: "1.0.0"}, Desired{Embedding: "local/m"}},
		{"embedding unknown on both sides", Observed{Exists: true}, Desired{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if action, _ := Decide(tc.obs, tc.want); action == ActionRebuild {
				t.Fatal("rebuilt on unknown metadata")
			}
		})
	}
}

func TestDecideRebuildTakesPrecedenceOverRefresh(t *testing.T) {
	// A stale store built with the wrong model must be rebuilt, not refreshed:
	// a refresh would keep the incomparable vectors.
	obs := Observed{Exists: true, Embedding: "local/old", ToolVersion: "1.0.0", Stale: true}
	want := Desired{Embedding: "local/new", ToolVersion: "1.0.0"}
	if action, _ := Decide(obs, want); action != ActionRebuild {
		t.Fatalf("action=%q, want rebuild to win over refresh", action)
	}
}

func TestDecideIgnoresEmbeddingCaseDifferences(t *testing.T) {
	obs := Observed{Exists: true, Embedding: "Local/Potion-Code-16m-v2", ToolVersion: "1.0.0"}
	want := Desired{Embedding: "local/potion-code-16m-v2", ToolVersion: "1.0.0"}
	if action, _ := Decide(obs, want); action != "" {
		t.Fatalf("action=%q, want no action for a case-only difference", action)
	}
}
