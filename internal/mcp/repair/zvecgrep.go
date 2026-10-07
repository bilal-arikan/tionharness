package repair

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/fspath"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// This file is the zvec-grep counterpart of repair.go. zvec-grep has no project
// id to get wrong — every call names an absolute `root`, prefilled from the
// session cwd in args.go when omitted — so its one repairable failure is a root
// without an index, which the server reports as an ordinary isError result
// (observed on zvec-grep 0.2.2):
//
//	[INDEX_MISSING] Indexed search requires a built zvec-grep index for <root>. …
//
// The server's own text already says to fall back to exact search. What it
// cannot say is that TionHarness builds the index, and that the model must not
// build one itself through the shell.

// ZvecGrepIndexMissingMarker opens zvec-grep's error for a root without an index.
const ZvecGrepIndexMissingMarker = "[INDEX_MISSING]"

// ZvecGrepIndexingHint is appended by the loop after a repair plan's hint, and
// only when the session repository's index really exists or is being built.
const ZvecGrepIndexingHint = " An index of this session's working directory is being built in the background; " +
	"it will not be ready within this turn, so use Glob/Grep now and retry zvec-grep on a later turn."

// zvecGrepToolPrefix starts every zvec-grep tool name (zvec_grep_search,
// zvec_grep_rg, zvec_grep_index, …), whatever the server row is called.
const zvecGrepToolPrefix = "zvec_grep_"

// IsZvecGrepTool reports whether name is a namespaced zvec-grep tool, in either
// namespace form (<server>__zvec_grep_search or mcp__<server>__zvec_grep_search).
func IsZvecGrepTool(name string) bool {
	i := strings.LastIndex(name, namespaceSep)
	return i >= 0 && strings.HasPrefix(name[i+len(namespaceSep):], zvecGrepToolPrefix)
}

// zvecGrepRoot turns the session working directory into a `root` argument: the
// cleaned directory when it is absolute, "" otherwise. The daemon needs an
// absolute path; a relative one would resolve against the daemon's own working
// directory and search the wrong tree without an error.
func zvecGrepRoot(sessionCwd string) string {
	cwd := strings.TrimSpace(sessionCwd)
	if cwd == "" || !filepath.IsAbs(cwd) {
		return ""
	}
	return filepath.Clean(cwd)
}

// RepairZvecGrep runs AFTER a zvec-grep call executed. On an [INDEX_MISSING]
// result it remembers the call, so Precheck refuses an identical repeat this
// turn, and returns the recovery hint. Plan.IndexPath is the session working
// directory when the failing root is that directory, lies inside it or contains
// it; the loop then builds that index. A root anywhere else is a path the model
// chose, and TionHarness does not index a tree the user never opened.
//
// Returns (zero, false) for anything else, leaving the result untouched.
func (m *Guard) RepairZvecGrep(call providers.ToolCall, res providers.ToolResult, sessionCwd string) (Plan, bool) {
	if !res.IsError || !IsZvecGrepTool(call.Name) || !strings.Contains(res.Content, ZvecGrepIndexMissingMarker) {
		return Plan{}, false
	}
	m.poisoned[CallKey(call)] = true
	root := callStringArg(call, "root")
	plan := Plan{Hint: zvecGrepRepairInstruction(root)}
	if cwd := zvecGrepRoot(sessionCwd); cwd != "" && (root == "" || pathContains(cwd, root) || pathContains(root, cwd)) {
		plan.IndexPath = cwd
	}
	return plan, true
}

// callStringArg reads one string argument of a call. A missing field, a
// non-string value or malformed input all read as "".
func callStringArg(call providers.ToolCall, key string) string {
	var args map[string]json.RawMessage
	if json.Unmarshal(call.Input, &args) != nil {
		return ""
	}
	var s string
	if json.Unmarshal(args[key], &s) != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

// pathContains reports whether child is parent or lies beneath it. Both must be
// absolute. On case-insensitive hosts (Windows, macOS) a drive-letter or folder
// case difference does not split one directory in two (see fspath.Within).
func pathContains(parent, child string) bool {
	if !filepath.IsAbs(parent) || !filepath.IsAbs(child) {
		return false
	}
	return fspath.Within(filepath.Clean(parent), filepath.Clean(child))
}

// zvecGrepRepairInstruction renders the model-facing recovery guidance for a root
// without an index. Besides the fallback it forbids the recovery a model with
// shell access reaches for — running `zg index` itself — because zvec-grep
// requires explicit authorization to create an index, and that decision belongs
// to the workspace's zvec-grep switch, not to the model.
func zvecGrepRepairInstruction(root string) string {
	where := "this root"
	if root != "" {
		where = "`" + root + "`"
	}
	return "\n\n[mcp repair] zvec-grep has no index for " + where + ", so this search cannot succeed — repeating it unchanged will fail identically. " +
		"Use Glob/Grep for this query. Do not create an index yourself (no `zg index` through the shell); TionHarness manages zvec-grep indexes."
}
