package agent

import (
	"context"
	"errors"

	"github.com/bilal-arikan/tionharness/internal/indexstate"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// searchIndexBridge adapts the Runtime to tools.SearchIndexBridge, so the
// search_index tool can read and drive the index lifecycle without the tools
// package importing internal/agent.
type searchIndexBridge struct{ rt *Runtime }

// SearchIndexStatus reports the state of every managed index covering root.
func (b searchIndexBridge) SearchIndexStatus(ctx context.Context, root string) []tools.SearchIndexEntry {
	in := b.rt.SearchIndexStatus(ctx, root)
	out := make([]tools.SearchIndexEntry, len(in))
	for i, s := range in {
		out[i] = tools.SearchIndexEntry(s)
	}
	return out
}

// RefreshSearchIndex runs an explicit refresh/rebuild of one tool's index
// through the manager and all of its guards.
//
// A run already in flight is NOT an error: the work the caller asked for is
// happening, so it reports started=false and lets the tool say so.
func (b searchIndexBridge) RefreshSearchIndex(ctx context.Context, tool, root, action string) (tools.SearchIndexEntry, bool, error) {
	act := indexstate.ActionRefresh
	if action == "rebuild" {
		act = indexstate.ActionRebuild
	}
	entry, err := b.rt.RequestIndexRun(ctx, IndexRequest{
		Tool:   tool,
		Root:   root,
		Action: act,
	})
	if err != nil {
		if errors.Is(err, ErrIndexRunInFlight) {
			return entryToToolEntry(entry), false, nil
		}
		return tools.SearchIndexEntry{}, false, err
	}
	return entryToToolEntry(entry), true, nil
}

// IndexTools returns the managed tools enabled for this workspace, in the order
// a tool-less refresh acts on them.
func (b searchIndexBridge) IndexTools(ctx context.Context) []string {
	var out []string
	if b.rt.ZvecGrepEnabled() {
		out = append(out, exttoolsZvecGrepName)
	}
	if b.rt.codebaseMemoryCmd(ctx) != "" {
		out = append(out, codebaseMemoryToolName)
	}
	return out
}

// IndexRoots returns the roots this session may act on. The session's working
// root is the only one offered: it is what the agent is actually working in,
// and widening this set is what the root restriction exists to prevent.
func (b searchIndexBridge) IndexRoots(ctx context.Context) []string {
	dir := b.rt.effectiveWorkDir(ctx)
	if dir == "" {
		return nil
	}
	return []string{dir}
}

// entryToToolEntry converts a ledger entry into the tool's reply shape. Ledger
// entries are always managed — an unmanaged tool never reaches the ledger.
func entryToToolEntry(e indexstate.Entry) tools.SearchIndexEntry {
	return tools.SearchIndexEntry{
		Tool:        e.Tool,
		Root:        e.Root,
		Phase:       string(e.Phase),
		Action:      e.Action,
		Embedding:   e.Embedding,
		ToolVersion: e.ToolVersion,
		Error:       e.Error,
		Usable:      e.Usable(),
		Managed:     true,
	}
}
