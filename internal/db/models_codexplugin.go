package db

import "strings"

// CodexMarketplace is one codex-cli plugin marketplace configured for a
// workspace. It mirrors codex's own [marketplaces.<name>] table: a name that
// plugin selectors resolve against, a source location, and the source kind.
//
// TionHarness ships no marketplace of its own and never registers one on its
// own initiative — every entry here was added by the user, either by pointing at
// a local directory / Git repository or by importing the ones already installed
// on this machine.
type CodexMarketplace struct {
	// Name is the marketplace identifier. Plugin selectors are "<plugin>@<name>",
	// so this must match the `name` field inside the marketplace's own
	// marketplace.json or codex resolves nothing.
	Name string `json:"name"`
	// Source is an absolute local directory path, or a Git location
	// (owner/repo, HTTPS or SSH URL) when SourceType is "git".
	Source string `json:"source"`
	// SourceType is "local" or "git" — the two kinds codex accepts.
	SourceType string `json:"sourceType"`
	// Ref is an optional Git branch, tag or commit. Ignored for local sources.
	Ref string `json:"ref,omitempty"`
}

// Marketplace source kinds.
const (
	CodexMarketplaceLocal = "local"
	CodexMarketplaceGit   = "git"
)

// reservedCodexMarketplaces are names codex refuses to let anyone register from
// their own source. Measured on codex 0.153.3: `codex plugin marketplace add`
// rejects "openai-bundled" with "marketplace `openai-bundled` is reserved and
// cannot be added from this source", and the remote catalogs are owned by the
// ChatGPT backend rather than local config. Rendering such a block would make
// every turn fail under --strict-config, so they are refused at the edge.
var reservedCodexMarketplaces = map[string]bool{
	"openai-bundled":         true,
	"openai-primary-runtime": true,
	"openai-curated-remote":  true,
	"created-by-me-remote":   true,
}

// CodexMarketplaceNameReserved reports whether name is one codex owns itself.
func CodexMarketplaceNameReserved(name string) bool {
	return reservedCodexMarketplaces[strings.ToLower(strings.TrimSpace(name))]
}

// ValidCodexMarketplaceName reports whether name is usable as a marketplace
// identifier: non-empty, not reserved, and limited to the characters codex uses
// for bare table keys. Anything else would either collide with codex's own
// sources or need quoting that its selector syntax ("<plugin>@<name>") cannot
// express.
func ValidCodexMarketplaceName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || CodexMarketplaceNameReserved(name) {
		return false
	}
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-'
		if !ok {
			return false
		}
	}
	return true
}

// SplitCodexPluginSelector splits a "<plugin>@<marketplace>" selector. ok is
// false when either half is missing, which is the only shape codex accepts for
// a [plugins."..."] key.
func SplitCodexPluginSelector(selector string) (plugin, marketplace string, ok bool) {
	selector = strings.TrimSpace(selector)
	at := strings.LastIndex(selector, "@")
	if at <= 0 || at == len(selector)-1 {
		return "", "", false
	}
	return selector[:at], selector[at+1:], true
}
