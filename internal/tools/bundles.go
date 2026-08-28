package tools

import (
	"sort"
	"strings"
)

// A BUNDLE is the shared grouping abstraction over both kinds of tool: a
// built-in belongs to its functional category ("group:automation", see
// categories.go), an MCP tool belongs to its originating SERVER
// ("mcp:playwright"). Bundle keys are the unit the default-visibility table
// (tierdefaults.go) is keyed by, and the unit activate_tools will later accept
// so a whole group's summaries can be listed without loading any schema.
//
// The MCP side deliberately derives the server from the NAMESPACED TOOL NAME
// (the already-sanitized prefix before nsSep), never from a user-facing
// db.MCPServer.Name — a display name may contain spaces and would not match the
// pool's sanitized keys.

// MCPBundlePrefix marks a bundle key naming an MCP SERVER: "mcp:playwright".
// Like GroupPrefix it cannot collide with a real tool name, because ":" is not a
// legal tool-name character.
const MCPBundlePrefix = "mcp:"

// bundleWildcard is the payload of a catch-all bundle key ("mcp:*"): a row that
// applies to every bundle of that kind which has no row of its own.
const bundleWildcard = "*"

// MCPBundleWildcard is the catch-all key for every MCP server: "mcp:*".
const MCPBundleWildcard = MCPBundlePrefix + bundleWildcard

// BundleOf returns the bundle key a tool name belongs to: "mcp:<server>" for a
// namespaced MCP tool, "group:<category>" for a built-in (CategoryOther when the
// built-in is not explicitly categorized).
func BundleOf(name string) string {
	if i := strings.Index(name, nsSep); i > 0 {
		return MCPBundlePrefix + name[:i]
	}
	return GroupPrefix + CategoryOf(name)
}

// SplitBundleKey reports the kind and payload of a bundle key:
//
//	"group:automation" -> ("group", "automation", true)
//	"mcp:playwright"   -> ("mcp", "playwright", true)
//	"notify"           -> ("", "", false)
//
// It validates the SHAPE only; use ValidBundleKey to also require a known
// category.
func SplitBundleKey(key string) (kind, value string, ok bool) {
	switch {
	case strings.HasPrefix(key, GroupPrefix):
		v := key[len(GroupPrefix):]
		if v == "" {
			return "", "", false
		}
		return "group", v, true
	case strings.HasPrefix(key, MCPBundlePrefix):
		v := key[len(MCPBundlePrefix):]
		if v == "" {
			return "", "", false
		}
		return "mcp", v, true
	}
	return "", "", false
}

// ValidBundleKey accepts a group key naming a KNOWN category (delegating to
// ValidGroupKey) or any well-formed "mcp:<server>" key. MCP server names are not
// knowable statically, so any non-empty payload is accepted there.
func ValidBundleKey(key string) bool {
	kind, value, ok := SplitBundleKey(key)
	if !ok {
		return false
	}
	if kind == "mcp" {
		return value != ""
	}
	return ValidGroupKey(key)
}

// MatchesBundle reports whether toolName belongs to the bundle named by
// bundleKey. It generalizes MatchesGroup to MCP bundles: a built-in matches a
// "group:" key by category, an MCP tool matches an "mcp:" key by server. The
// wildcard key "mcp:*" matches every namespaced MCP tool.
func MatchesBundle(toolName, bundleKey string) bool {
	kind, value, ok := SplitBundleKey(bundleKey)
	if !ok {
		return false
	}
	if kind == "group" {
		return MatchesGroup(toolName, bundleKey)
	}
	if !strings.Contains(toolName, nsSep) {
		return false
	}
	if value == bundleWildcard {
		return true
	}
	return BundleOf(toolName) == bundleKey
}

// BundleIndex returns bundle key -> sorted member names over everything the
// registry knows about (built-ins + attached MCP entries), filtered by allow
// (nil = allow all). It is a pure in-memory group-by: it never builds an MCP
// catalog, so it is safe to call on the registry-build path.
func (r *Registry) BundleIndex(allow func(string) bool) map[string][]string {
	out := map[string][]string{}
	add := func(name string) {
		if allow != nil && !allow(name) {
			return
		}
		key := BundleOf(name)
		out[key] = append(out[key], name)
	}
	for name := range r.builtins {
		add(name)
	}
	for _, e := range r.mcpEntries {
		add(e.NamespacedName)
	}
	for _, members := range out {
		sort.Strings(members)
	}
	return out
}
