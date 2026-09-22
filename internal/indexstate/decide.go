package indexstate

import "strings"

// Desired describes the host's CURRENT indexing configuration for a root: the
// embedding model a new store would be built with, and the version of the tool
// that would build it.
type Desired struct {
	Embedding   string
	ToolVersion string
}

// Observed describes what is actually on disk for a root.
//
// Exists is the only field that is always meaningful. Embedding and ToolVersion
// come from the tool's own manifest and may be empty when it does not record
// them — which is why Decide treats an empty value as "unknown, do not rebuild"
// rather than "different, rebuild now". Rebuilding on missing metadata would
// re-embed a large repository every single startup.
type Observed struct {
	Exists      bool
	Embedding   string
	ToolVersion string
	// Stale is the tool's own verdict that the store has fallen behind the
	// working tree. Only consulted when Exists.
	Stale bool
}

// Decide maps (what is on disk, what the host wants) to the action that should
// run, and the phase the index is in before it runs. An empty action means
// nothing needs to happen.
//
// The order of the checks is the contract:
//
//  1. No store at all -> create.
//  2. Embedding model changed -> rebuild. Vectors produced by two different
//     models share a space only by accident; refreshing INTO such a store mixes
//     them and every later search silently returns nonsense. This is the case the
//     old one-shot create could not see at all.
//  3. Tool version changed -> rebuild. A new indexer may write a different
//     on-disk format, and a refresh across formats is not defined.
//  4. Store lags the tree -> refresh, which keeps the existing vectors.
//  5. Otherwise -> ready, nothing to do.
func Decide(obs Observed, want Desired) (action string, phase Phase) {
	if !obs.Exists {
		return ActionCreate, PhaseMissing
	}
	if changed(obs.Embedding, want.Embedding) {
		return ActionRebuild, PhaseStale
	}
	if changed(obs.ToolVersion, want.ToolVersion) {
		return ActionRebuild, PhaseStale
	}
	if obs.Stale {
		return ActionRefresh, PhaseStale
	}
	return "", PhaseReady
}

// changed reports whether a recorded value differs from the wanted one in a way
// worth rebuilding for.
//
// Either side being empty means "unknown", and an unknown never triggers a
// rebuild: the cost of being wrong in that direction is re-embedding a whole
// repository on every start, while the cost of waiting is one more session
// against an index that was working a moment ago.
func changed(have, want string) bool {
	have, want = strings.TrimSpace(have), strings.TrimSpace(want)
	if have == "" || want == "" {
		return false
	}
	return !strings.EqualFold(have, want)
}
