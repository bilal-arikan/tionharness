package tools

import (
	"fmt"
	"strings"
)

// BundleListLimit caps how many members ONE bundle listing prints. Without it a
// large server's bundle would cost as much as the schemas the bundle exists to
// avoid. Shared by both call paths so the cap cannot drift apart.
const BundleListLimit = 40

// nativeBundleHeaderFormat / nativeBundleOverflowFormat are the native
// activate_tools wordings. They differ from the gateway's on purpose: the native
// path OPENS a bundle (it records the open set), while the gateway keeps no state
// and says so ("nothing was activated").
const (
	nativeBundleHeaderFormat   = "Opened %s (%d tools) — summaries only, no schema loaded. Load one with activate_tools(\"<name>\").\n"
	nativeBundleOverflowFormat = "…and %d more not shown; narrow with tool_search(\"<keyword>\").\n"
)

// BundleListRow is one member of a bundle, under its CATALOG name.
type BundleListRow struct {
	Name string
	Desc string
}

// BundleListing is one bundle to render: its key and its members, already in
// print order.
type BundleListing struct {
	Key     string
	Members []BundleListRow
}

// BundleListOpts holds the parts of a bundle listing that legitimately differ
// between the two call paths — the native builtin (ActivateToolsTool.openBundles)
// and the claude-cli gateway (interactionBackend.listBundles). Everything NOT
// expressed here is shared and must stay byte-identical on both paths.
type BundleListOpts struct {
	// HeaderFormat is the per-bundle first line. It receives the bundle key and
	// the member count, in that order, and must end with its own newline.
	HeaderFormat string
	// OverflowFormat is the truncation notice. It receives the hidden count and
	// must end with its own newline.
	OverflowFormat string
	// Max is how many members survive before the overflow notice replaces the
	// rest. Zero means no cap.
	Max int
	// NameOf maps a member's catalog name to the name printed in its row. The
	// gateway prints the namespaced callable form, the native path the bare name.
	// Required: a silent bare fallback would hand the CLI an uncallable name.
	NameOf func(string) string
}

// RenderBundleList renders bundle member listings into the model-facing result
// text: a header per bundle, one "- name — desc" row per member, and a truncation
// notice when the bundle exceeds Max. Pure: it reads nothing but its arguments
// and touches no activation state.
func RenderBundleList(listings []BundleListing, opts BundleListOpts) string {
	if opts.NameOf == nil {
		panic("tools: RenderBundleList requires a NameOf resolver")
	}
	if opts.HeaderFormat == "" || opts.OverflowFormat == "" {
		panic("tools: RenderBundleList requires both HeaderFormat and OverflowFormat")
	}
	var b strings.Builder
	for _, l := range listings {
		fmt.Fprintf(&b, opts.HeaderFormat, l.Key, len(l.Members))
		shown := l.Members
		hidden := 0
		if opts.Max > 0 && len(shown) > opts.Max {
			hidden = len(shown) - opts.Max
			shown = shown[:opts.Max]
		}
		for _, m := range shown {
			fmt.Fprintf(&b, "- %s — %s\n", opts.NameOf(m.Name), m.Desc)
		}
		if hidden > 0 {
			fmt.Fprintf(&b, opts.OverflowFormat, hidden)
		}
	}
	return b.String()
}
