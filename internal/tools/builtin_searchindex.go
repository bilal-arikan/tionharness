package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// SearchIndexEntry is one index's state as reported to the agent. It mirrors
// agent.IndexStatus, redeclared here because the tools package must not import
// internal/agent (the bridge direction is agent -> tools).
type SearchIndexEntry struct {
	Tool        string `json:"tool"`
	Root        string `json:"root"`
	Phase       string `json:"phase"`
	Action      string `json:"action,omitempty"`
	Embedding   string `json:"embedding,omitempty"`
	ToolVersion string `json:"toolVersion,omitempty"`
	Error       string `json:"error,omitempty"`
	Usable      bool   `json:"usable"`
	Managed     bool   `json:"managed"`
	Note        string `json:"note,omitempty"`
}

// SearchIndexBridge is the runtime side of search_index: read the state of the
// indexes covering a root, and run an explicit refresh/rebuild on one.
//
// Refresh reports the claimed entry and whether a run was actually STARTED —
// false when one was already in flight, which is a normal outcome (the work is
// happening) rather than a failure.
type SearchIndexBridge interface {
	SearchIndexStatus(ctx context.Context, root string) []SearchIndexEntry
	RefreshSearchIndex(ctx context.Context, tool, root, action string) (entry SearchIndexEntry, started bool, err error)
	// IndexTools returns the managed tools enabled for this workspace ("zg",
	// "codebase-memory"); a refresh that names no tool acts on each of them.
	IndexTools(ctx context.Context) []string
	// IndexRoots returns the roots this session may act on: its working root
	// first. A request naming anything else is refused — see SearchIndexTool.
	IndexRoots(ctx context.Context) []string
}

// SearchIndexTool lets an agent ask TionHarness about, and act on, the search
// indexes behind zvec-grep and codebase-memory.
//
// It exists because the capability prompts tell the agent "never run `zg index`
// or index_repository yourself — TionHarness manages the indexes", and that
// instruction needs somewhere to send an agent that has just hit
// [INDEX_MISSING]. Without this tool the only options were to disobey the
// prompt or to silently fall back to grep forever.
//
// Drop is deliberately NOT an action here. Deleting an index is destructive and
// irreversible-in-practice (re-embedding a large repository is expensive), so it
// stays a user action behind the Settings panel's confirmation gate.
type SearchIndexTool struct {
	bridge SearchIndexBridge
}

// NewSearchIndexTool binds search_index to the runtime bridge.
func NewSearchIndexTool(bridge SearchIndexBridge) SearchIndexTool {
	return SearchIndexTool{bridge: bridge}
}

func (SearchIndexTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "search_index",
		Description: "Inspect or repair the search indexes TionHarness manages for you (zvec-grep's " +
			"vector store `zg` and codebase-memory's code graph `codebase-memory`). Use `status` when a search " +
			"returned [INDEX_MISSING] or looked stale, then `refresh` to bring the index up to date " +
			"(keeps the existing store) or `rebuild` to discard and re-create it (only when the index is " +
			"corrupt or built with a different embedding model). `tool` limits refresh/rebuild to one index; " +
			"omitted, every managed index for the root is acted on. Runs are asynchronous: the call returns " +
			"once the run is claimed, not when it finishes — re-check with `status`. You may only target " +
			"your own session's working root (the default) or a root already known to be indexed; " +
			"deleting an index is not available here. Never run `zg index` or index_repository through " +
			"the shell instead of this tool.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "action": { "type": "string", "enum": ["status", "refresh", "rebuild"], "description": "status (default): report index state. refresh: re-index in place. rebuild: discard and re-create the store." },
    "root": { "type": "string", "description": "Absolute path of the root to act on. Defaults to this session's working root; other paths are refused unless already indexed." },
    "tool": { "type": "string", "enum": ["zg", "codebase-memory"], "description": "Index to refresh/rebuild. Omit to act on every managed index for the root. Ignored by status." }
  },
  "additionalProperties": false
}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"action":"status"}`),
			json.RawMessage(`{"action":"refresh"}`),
		},
	}
}

// searchIndexResult is the tool's JSON reply.
type searchIndexResult struct {
	Action  string             `json:"action"`
	Tool    string             `json:"tool,omitempty"`
	Root    string             `json:"root"`
	Started bool               `json:"started,omitempty"`
	Message string             `json:"message"`
	Indexes []SearchIndexEntry `json:"indexes,omitempty"`
}

func (t SearchIndexTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Action string `json:"action"`
		Root   string `json:"root"`
		Tool   string `json:"tool"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return "", argErr(err)
		}
	}
	if t.bridge == nil {
		return "Search index management is not available in this context.", nil
	}

	action := strings.ToLower(strings.TrimSpace(in.Action))
	if action == "" {
		action = "status"
	}
	switch action {
	case "status", "refresh", "rebuild":
	default:
		return "", fmt.Errorf("unknown action %q: use status, refresh or rebuild", in.Action)
	}

	root, err := t.resolveRoot(ctx, in.Root)
	if err != nil {
		return "", err
	}

	if action == "status" {
		entries := t.bridge.SearchIndexStatus(ctx, root)
		res := searchIndexResult{Action: action, Root: root, Indexes: entries}
		if len(entries) == 0 {
			res.Message = "No managed search index covers this root (no indexing tool is enabled for this workspace)."
		} else {
			res.Message = "Index state for " + root + "."
		}
		return marshalResult(res)
	}

	tools, err := t.refreshTools(ctx, in.Tool)
	if err != nil {
		return "", err
	}
	res := searchIndexResult{Action: action, Root: root, Tool: strings.TrimSpace(in.Tool)}
	var msgs []string
	var errs []error
	for _, tool := range tools {
		entry, started, err := t.bridge.RefreshSearchIndex(ctx, tool, root, action)
		if err != nil {
			// One tool's refusal must not hide another tool's run that did start,
			// so a partial failure is reported per tool; only an all-failed request
			// is an error.
			errs = append(errs, fmt.Errorf("%s: %w", tool, err))
			msgs = append(msgs, tool+": "+err.Error())
			continue
		}
		res.Indexes = append(res.Indexes, entry)
		if started {
			res.Started = true
			msgs = append(msgs, tool+": a "+entry.Action+" is now running for "+entry.Root+
				". It runs in the background — call status again to see when it finishes.")
		} else {
			msgs = append(msgs, tool+": a run is already in flight for "+entry.Root+
				"; nothing new was started. Call status again to see when it finishes.")
		}
	}
	if len(res.Indexes) == 0 {
		return "", errors.Join(errs...)
	}
	res.Message = strings.Join(msgs, " ")
	return marshalResult(res)
}

// refreshTools resolves which indexes a refresh/rebuild acts on: the one named,
// or every managed tool enabled for this workspace. An unknown or disabled tool
// is an error, not a silent no-op.
func (t SearchIndexTool) refreshTools(ctx context.Context, want string) ([]string, error) {
	enabled := t.bridge.IndexTools(ctx)
	if len(enabled) == 0 {
		return nil, fmt.Errorf("no managed search index is enabled for this workspace")
	}
	want = strings.TrimSpace(want)
	if want == "" {
		return enabled, nil
	}
	if slices.Contains(enabled, want) {
		return []string{want}, nil
	}
	return nil, fmt.Errorf("index tool %q is not enabled for this workspace (enabled: %s)", want, strings.Join(enabled, ", "))
}

// resolveRoot applies the root restriction: an agent may act on its own
// session's working root, or on a root already known to be indexed, and nothing
// else.
//
// The restriction is the point of routing this through TionHarness at all. An
// unrestricted root would let an agent aim a repository-wide re-embed at any
// directory on the machine — the user's home, another project, a network share —
// which is both expensive and a way to write a store somewhere the user never
// agreed to.
func (t SearchIndexTool) resolveRoot(ctx context.Context, want string) (string, error) {
	allowed := t.bridge.IndexRoots(ctx)
	if len(allowed) == 0 {
		return "", fmt.Errorf("this session has no working root, so there is no index to act on")
	}
	if want = strings.TrimSpace(want); want == "" {
		return allowed[0], nil
	}
	for _, a := range allowed {
		if sameIndexRoot(a, want) {
			return a, nil
		}
	}
	return "", fmt.Errorf("root %q is not this session's working root (%s) or a known indexed root; "+
		"search_index only manages indexes for roots TionHarness already tracks", want, allowed[0])
}

// sameIndexRoot compares two roots the way the host filesystem does. Windows
// paths reached as C:\Repo and c:\repo are the same root, and a trailing
// separator or a mixed separator must not make a legitimate request look like
// an attempt to reach somewhere else.
func sameIndexRoot(a, b string) bool {
	norm := func(p string) string {
		p = strings.ReplaceAll(strings.TrimSpace(p), `\`, "/")
		p = strings.TrimRight(p, "/")
		return strings.ToLower(p)
	}
	return norm(a) == norm(b)
}

// marshalResult renders the reply, failing loudly rather than returning a
// half-formed string: a marshal error here means the struct changed shape and
// the caller should see that, not an empty result.
func marshalResult(res searchIndexResult) (string, error) {
	b, err := json.Marshal(res)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
