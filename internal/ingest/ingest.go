// Package ingest is the generic IMPORT pipeline: it acquires a foreign source tree
// (via internal/fetch), runs a set of ADAPTERS over it to detect artifacts (Claude
// Code skills, subagents, slash commands, MCP configs…), and translates each into a
// native SwarmPack (internal/market). The packs are then handed to the single
// install authority in the API layer — ingest itself never writes entities, it only
// fetches, detects and converts. Adding a new importable feature = one Adapter.
//
// Boundary: ingest depends on fetch (acquisition), skills (the CC→TionSwarm skill
// mapping helpers) and market (the pack envelope). Nothing depends on ingest except
// the API layer, so there are no import cycles. (SK-IMP3)
package ingest

import (
	"sort"
	"strconv"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/fetch"
	"github.com/bilal-arikan/tionswarm/internal/market"
)

// Options control how discovered artifacts become packs at install time.
type Options struct {
	Shared     bool   // advertise installed skills on-demand
	SlugPrefix string // namespace prefix for derived slugs/ids
	// Group namespaces the import for ORGANISATION: when non-empty it is written as
	// each imported skill's `group` frontmatter, so every skill from one import lands
	// under a single collapsible header in the Skills UI and never mixes with the
	// user's existing skills. Empty → each skill keeps whatever group its source
	// declared (usually none).
	Group string
}

// Discovered is one artifact found in a source tree: preview metadata plus a
// closure that builds its SwarmPack on demand (capturing the raw artifact data).
// The closure is not serialised; the API re-scans to rebuild it for install.
type Discovered struct {
	Key         string   `json:"key"`         // unique selection key (kind:relPath)
	Kind        string   `json:"kind"`        // market kind (skill/agent/mcp…)
	Slug        string   `json:"slug"`        // suggested slug/id
	Name        string   `json:"name"`        //
	Description string   `json:"description"` //
	RelPath     string   `json:"relPath"`     // artifact path within the tree
	Files       []string `json:"files"`       // bundled resource relpaths (preview)
	Warnings    []string `json:"warnings,omitempty"`
	Exists      bool     `json:"exists"` // set by the API layer (already installed)

	build func(Options) (market.Pack, error)
}

// Adapter detects foreign artifacts of one market kind in a fetched tree and
// produces Discovered entries, each carrying its own pack-builder closure.
type Adapter interface {
	Kind() string
	Scan(tree fetch.Tree, prefix, baseURL string) []Discovered
}

// registry is the ordered set of adapters consulted on every scan. Each adapter
// detects one foreign artifact shape and emits native packs; adding an importable
// feature = adding one Adapter here.
var registry = []Adapter{
	skillAdapter{},
	agentAdapter{},
	commandAdapter{},
	mcpAdapter{},
	hookAdapter{},
}

// ScanResult is the full discovery across all adapters.
type ScanResult struct {
	Source   string       `json:"source"`
	Location string       `json:"location"`
	Items    []Discovered `json:"items"`
	Warnings []string     `json:"warnings"`
}

// SkipNote records an artifact that could not be packed (a build error).
type SkipNote struct {
	Key    string `json:"key"`
	Slug   string `json:"slug"`
	Reason string `json:"reason"`
}

// Scan acquires the source tree and runs every adapter over it, returning the
// discovered artifacts (sorted by kind then slug) for preview/selection.
func Scan(source, location string) (ScanResult, error) {
	tree, prefix, warnings, err := fetch.TreeFrom(source, location)
	if err != nil {
		return ScanResult{}, err
	}
	base := provenanceBase(source, location)
	res := ScanResult{Source: source, Location: location, Warnings: warnings}
	// A marketplace.json (when present and the user didn't narrow the path) pins the
	// real plugin roots, so mirrored copies and unrelated subtrees are excluded.
	roots, marketName := pluginRoots(tree, prefix)
	if marketName != "" && !(len(roots) == 1 && roots[0] == "") {
		res.Warnings = append(res.Warnings, "marketplace.json \""+marketName+"\": "+plural(len(roots), "plugin root")+" targeted")
	}
	var all []Discovered
	for _, root := range roots {
		for _, a := range registry {
			all = append(all, a.Scan(tree, root, base)...)
		}
	}
	var dropped int
	res.Items, dropped = dedupItems(all)
	if dropped > 0 {
		res.Warnings = append(res.Warnings, plural(dropped, "duplicate artifact")+" skipped (same kind+slug in a mirrored/nested copy, e.g. plugins/ or dist/)")
	}
	sort.Slice(res.Items, func(i, j int) bool {
		if res.Items[i].Kind != res.Items[j].Kind {
			return res.Items[i].Kind < res.Items[j].Kind
		}
		return res.Items[i].Slug < res.Items[j].Slug
	})
	if len(res.Items) == 0 {
		res.Warnings = append(res.Warnings, "no importable artifacts (SKILL.md / agents / commands / MCP config) found")
	}
	return res, nil
}

// BuildPacks re-scans and returns the packs for the selected keys (empty = all),
// applying opts. Build errors are reported as skips, not fatal. The caller installs
// the returned packs through the single install authority.
func BuildPacks(source, location string, selectedKeys []string, opts Options) (packs []market.Pack, skipped []SkipNote, warnings []string, err error) {
	sr, serr := Scan(source, location)
	if serr != nil {
		return nil, nil, nil, serr
	}
	want := map[string]bool{}
	for _, k := range selectedKeys {
		want[strings.TrimSpace(k)] = true
	}
	for _, it := range sr.Items {
		if len(want) > 0 && !want[it.Key] {
			continue
		}
		if it.build == nil {
			continue
		}
		p, berr := it.build(opts)
		if berr != nil {
			skipped = append(skipped, SkipNote{Key: it.Key, Slug: it.Slug, Reason: berr.Error()})
			continue
		}
		packs = append(packs, p)
	}
	return packs, skipped, sr.Warnings, nil
}

// dedupItems drops items sharing a (kind, slug) identity — the case where a repo
// mirrors its plugin under plugins/ or ships a built copy under dist/ — keeping the
// SHALLOWEST relPath (the canonical source over a nested duplicate). Returns the
// kept items and how many were dropped.
func dedupItems(items []Discovered) ([]Discovered, int) {
	best := map[string]int{} // kind:slug -> index in keep
	var keep []Discovered
	dropped := 0
	for _, it := range items {
		id := it.Kind + ":" + it.Slug
		if j, ok := best[id]; ok {
			if depth(it.RelPath) < depth(keep[j].RelPath) {
				keep[j] = it // shallower copy wins
			}
			dropped++
			continue
		}
		best[id] = len(keep)
		keep = append(keep, it)
	}
	return keep, dropped
}

// depth counts the path segments of a forward-slashed relpath.
func depth(p string) int {
	if p == "" {
		return 0
	}
	return strings.Count(p, "/") + 1
}

// plural renders "N thing" / "N things".
func plural(n int, noun string) string {
	s := noun
	if n != 1 {
		s += "s"
	}
	return strconv.Itoa(n) + " " + s
}

// PreviewItem is a discovered artifact WITH its rendered body, for the detail view
// of a source-ref (directory-site) catalog entry before install.
type PreviewItem struct {
	Kind        string   `json:"kind"`
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Body        string   `json:"body"`
	Files       []string `json:"files"`
	Warnings    []string `json:"warnings"`
}

// Preview scans a source and returns its first few artifacts WITH rendered bodies, so
// the UI can show what a source-ref entry contains before installing. Capped to keep
// the single fetch cheap.
func Preview(source, location string) ([]PreviewItem, error) {
	sr, err := Scan(source, location)
	if err != nil {
		return nil, err
	}
	out := make([]PreviewItem, 0, len(sr.Items))
	for i, it := range sr.Items {
		if i >= 8 {
			break
		}
		body := ""
		if it.build != nil {
			if p, berr := it.build(Options{}); berr == nil {
				body = previewBody(p)
			}
		}
		out = append(out, PreviewItem{
			Kind: it.Kind, Slug: it.Slug, Name: it.Name, Description: it.Description,
			Body: body, Files: it.Files, Warnings: it.Warnings,
		})
	}
	return out, nil
}

// previewBody extracts a human-readable body from a built pack for the preview.
func previewBody(p market.Pack) string {
	switch {
	case p.Payload.Skill != nil:
		return p.Payload.Skill.Body
	case p.Payload.Agent != nil:
		return p.Payload.Agent.Soul
	case p.Payload.MCP != nil:
		return strings.TrimSpace(p.Payload.MCP.Command + " " + p.Payload.MCP.Args + " " + p.Payload.MCP.URL)
	}
	return ""
}

// --- shared adapter helpers ---

// provenanceBase returns the base URL recorded as an imported entity's source_url.
// Empty for local sources.
func provenanceBase(source, location string) string {
	if strings.EqualFold(strings.TrimSpace(source), "local") {
		return ""
	}
	return strings.TrimRight(fetch.NormalizeRepoRef(location), "/")
}

// itemURL joins a provenance base with an artifact's relative path.
func itemURL(base, relPath string) string {
	if base == "" {
		return ""
	}
	if relPath == "" {
		return base
	}
	return base + "/" + relPath
}

// sortedKeys returns a map's keys in sorted order (for stable file-list previews).
func sortedKeys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// applyPrefix prepends an (already-slugified) prefix to a slug, "" prefix → slug.
func applyPrefix(prefix, slug string) string {
	if prefix == "" {
		return slug
	}
	return prefix + "-" + slug
}
