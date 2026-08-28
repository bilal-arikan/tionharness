package tools

import (
	"fmt"
	"strings"
)

// TionHarness bridges its OWN tools to the claude-cli under these two namespaces.
// Every other `mcp__<server>__…` name belongs to a real external MCP server the CLI
// loads itself.
const (
	InteractionNamespace = "mcp__tionharness_interaction__"
	ExtendedNamespace    = "mcp__tionharness_extended__"

	mcpNamePrefix = "mcp__"
)

// IsTionHarnessToolName reports whether a namespaced tool name is one of ours.
func IsTionHarnessToolName(name string) bool {
	return strings.HasPrefix(name, InteractionNamespace) || strings.HasPrefix(name, ExtendedNamespace)
}

// IsExternalMCPName reports whether a name looks like a tool served by an external
// MCP server (codebase-memory-mcp, playwright, …) rather than by TionHarness.
func IsExternalMCPName(name string) bool {
	return strings.HasPrefix(name, mcpNamePrefix) && !IsTionHarnessToolName(name)
}

// ExternalMCPActivateNote is the redirect emitted when activate_tools is handed an
// EXTERNAL MCP name. Those tools live in the CLI's own catalog, so activation here
// can only ever fail; before this note they silently landed in the generic "unknown
// name" list and the model retried the same wrong loader (SES79).
func ExternalMCPActivateNote(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return fmt.Sprintf("external MCP tools (not loadable with activate_tools): %s — load these with ToolSearch using the query %q.",
		strings.Join(names, ", "), "select:"+strings.Join(names, ","))
}

// IsTionHarnessLoaderQuery reports whether a tool_search query is really an attempt
// to load TionHarness's own on-demand tools through the CLI's ToolSearch — either the
// `select:` load syntax or a name in one of our namespaces.
func IsTionHarnessLoaderQuery(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return false
	}
	return strings.HasPrefix(q, "select:") ||
		strings.Contains(q, strings.ToLower(InteractionNamespace)) ||
		strings.Contains(q, strings.ToLower(ExtendedNamespace))
}

// TionHarnessLoaderNote is the counterpart redirect for a search that used the wrong
// loader. It must be emitted even when the search matches nothing: in a real session
// (WS20/SES79) the model issued `select:mcp__tionharness_extended__list_tasks`, got an
// EMPTY result, and repeated the same call for three turns because nothing told it the
// two loaders are separate. activateCall differs per path (the gateway tool takes
// "tools", the native one takes "names"), so it is passed in.
func TionHarnessLoaderNote(activateCall string) string {
	return "Note: TionHarness on-demand tools (" + InteractionNamespace + "… / " + ExtendedNamespace +
		"…) are NOT loaded from this search — load them with " + activateCall +
		". The ToolSearch \"select:<name>\" syntax loads only EXTERNAL mcp__<server>__… tools (e.g. mcp__codebase-memory-mcp__search_graph)."
}
