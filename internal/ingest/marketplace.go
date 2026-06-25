package ingest

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/fetch"
)

// A Claude Code repo may declare itself a plugin "marketplace" via
// .claude-plugin/marketplace.json, listing one or more plugins and WHERE each lives
// (its source path). When present, that manifest is a far more precise discovery
// guide than a blind whole-tree scan: it pins the real plugin roots, so mirrored
// copies (plugins/, dist/) and unrelated subtrees are excluded up front. (SK-IMP3)

// marketplaceDoc is the subset of the marketplace.json schema we consume.
type marketplaceDoc struct {
	Name    string `json:"name"`
	Plugins []struct {
		Name   string `json:"name"`
		Source string `json:"source"`
	} `json:"plugins"`
}

// pluginRoots determines the discovery roots for a scan. When the user pointed at a
// specific sub-path it is honoured verbatim. Otherwise, if a marketplace.json
// declares plugin sources, those (resolved against the manifest's repo root) become
// the roots; failing that, the whole tree ([""]) is scanned. It also returns the
// marketplace name (provenance) when one drove the decision.
func pluginRoots(tree fetch.Tree, userPrefix string) (roots []string, marketName string) {
	if p := strings.Trim(strings.TrimSpace(userPrefix), "/"); p != "" {
		return []string{p}, ""
	}
	manifestPath, data := shallowestMarketplace(tree)
	if data == nil {
		return []string{""}, ""
	}
	var doc marketplaceDoc
	if err := json.Unmarshal(data, &doc); err != nil || len(doc.Plugins) == 0 {
		return []string{""}, ""
	}
	// Sources are relative to the repo root = the parent of the .claude-plugin dir.
	repoRoot := fetch.DirOf(fetch.DirOf(manifestPath))
	seen := map[string]bool{}
	for _, pl := range doc.Plugins {
		src := strings.Trim(strings.TrimPrefix(strings.TrimSpace(pl.Source), "./"), "/")
		root := src
		if repoRoot != "" {
			root = strings.Trim(repoRoot+"/"+src, "/")
		}
		if !seen[root] {
			seen[root] = true
			roots = append(roots, root)
		}
	}
	if len(roots) == 0 {
		return []string{""}, ""
	}
	sort.Strings(roots)
	return roots, strings.TrimSpace(doc.Name)
}

// shallowestMarketplace returns the path and content of the least-nested
// .claude-plugin/marketplace.json in the tree (so a mirrored copy deeper in the tree
// never overrides the canonical root manifest). Returns nil data when absent.
func shallowestMarketplace(tree fetch.Tree) (string, []byte) {
	bestPath := ""
	var bestData []byte
	bestDepth := -1
	for p, data := range tree {
		if !strings.EqualFold(p, ".claude-plugin/marketplace.json") &&
			!strings.HasSuffix(strings.ToLower(p), "/.claude-plugin/marketplace.json") {
			continue
		}
		d := strings.Count(p, "/")
		if bestDepth < 0 || d < bestDepth {
			bestDepth = d
			bestPath = p
			bestData = data
		}
	}
	return bestPath, bestData
}
