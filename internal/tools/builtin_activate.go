package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// lazyEntry is one row of the load-on-demand tool catalog (name + summary).
type lazyEntry struct {
	name string
	desc string
}

func toLazyEntries(catalog []providers.ToolDef) []lazyEntry {
	out := make([]lazyEntry, 0, len(catalog))
	for _, d := range catalog {
		out = append(out, lazyEntry{name: d.Name, desc: d.Description})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// ---- activate_tools -------------------------------------------------------

// ActivateToolsTool loads the full schemas of lazy tools on demand: the model
// names the tools it wants, they enter the active set, and their real schemas
// are shipped on the next iteration so it can call them.
type ActivateToolsTool struct {
	active  *ActiveTools
	byName  map[string]string   // lazy tool name -> description
	eager   map[string]bool     // always-on tool names (already callable, no activation needed)
	bundles map[string][]string // bundle key -> member names (see bundles.go); nil disables bundle keys
}

// NewActivateToolsTool builds the tool over the active set, the lazy catalog and
// the set of eager (always-on) tool names. The eager set lets the tool answer a
// request to activate an already-shipped tool with a clear "already available"
// note instead of the misleading "unknown name" (eager tools are never in the
// lazy catalog). A nil eager set is fine — such names simply fall back to unknown.
func NewActivateToolsTool(active *ActiveTools, catalog []providers.ToolDef, eager map[string]bool) ActivateToolsTool {
	return NewActivateToolsToolBundled(active, catalog, eager, nil)
}

// NewActivateToolsToolBundled is NewActivateToolsTool plus the BUNDLE index
// (bundle key -> member names, from Registry.BundleIndex): with it the tool also
// accepts a bundle key ("group:automation", "mcp:playwright") and answers with the
// members' summaries. A nil index simply means no key is recognized as a bundle.
func NewActivateToolsToolBundled(active *ActiveTools, catalog []providers.ToolDef, eager map[string]bool, bundles map[string][]string) ActivateToolsTool {
	byName := make(map[string]string, len(catalog))
	for _, e := range toLazyEntries(catalog) {
		byName[e.name] = e.desc
	}
	// Keep only members that exist in the LAZY catalog: a bundle listing is built
	// from the summaries snapshotted here, and an always-on (eager) tool has no row
	// — listing it would print a name with an empty summary and invite a pointless
	// activation. A bundle left with no listable member is dropped entirely so it
	// reports as unknown rather than opening empty.
	var idx map[string][]string
	if len(bundles) > 0 {
		idx = make(map[string][]string, len(bundles))
		for key, members := range bundles {
			kept := make([]string, 0, len(members))
			for _, n := range members {
				if _, ok := byName[n]; ok {
					kept = append(kept, n)
				}
			}
			if len(kept) > 0 {
				idx[key] = kept
			}
		}
	}
	return ActivateToolsTool{active: active, byName: byName, eager: eager, bundles: idx}
}

func (ActivateToolsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "activate_tools",
		Description: "Load the full schemas of one or more tools listed in your system prompt under " +
			"\"Available Tools (load on demand)\". Call this BEFORE using such a tool: pass its exact " +
			"name(s). Activate everything you expect to need for the task in one call. IMPORTANT: the " +
			"activated tools only become callable on your NEXT step — do NOT call them in the SAME " +
			"response/batch as this activate_tools call, or the runtime will reject them as \"No such " +
			"tool available\". Activate now, use them next turn. An entry may also be a BUNDLE key " +
			"(\"group:<category>\" or \"mcp:<server>\"): that loads no schema at all — it only returns " +
			"the bundle members' name — summary lines, so you can then activate the one you need by name.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "names": { "type": "array", "items": { "type": "string" }, "description": "Exact tool names to activate, and/or bundle keys (group:<category>, mcp:<server>) to list." }
  },
  "required": ["names"],
  "additionalProperties": false
}`),
	}
}

// knownBundleKeys returns the sorted bundle keys this tool can open.
func (t ActivateToolsTool) knownBundleKeys() []string {
	out := make([]string, 0, len(t.bundles))
	for k := range t.bundles {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// openBundles renders the member listing for the given bundle keys. It NEVER
// activates a member: the whole point of a bundle is that it costs summaries, not
// schemas, so ActiveTools (and therefore the shipped schema set) is untouched.
// Rendering (header, row shape, truncation notice) is shared with the claude-cli
// gateway path via RenderBundleList, so a change to the listing format lands on
// both at once instead of drifting apart.
func (t ActivateToolsTool) openBundles(keys []string) string {
	listings := make([]BundleListing, 0, len(keys))
	for _, key := range keys {
		members := t.bundles[key]
		rows := make([]BundleListRow, 0, len(members))
		for _, n := range members {
			rows = append(rows, BundleListRow{Name: n, Desc: t.byName[n]})
		}
		listings = append(listings, BundleListing{Key: key, Members: rows})
	}
	return RenderBundleList(listings, BundleListOpts{
		HeaderFormat:   nativeBundleHeaderFormat,
		OverflowFormat: nativeBundleOverflowFormat,
		Max:            BundleListLimit,
		NameOf:         func(n string) string { return n },
	})
}

// resolveLazyName maps a requested tool name to a real catalog name, tolerating
// the common namespace confusion where the model prefixes a name with an
// mcp__<server>__ (or <server>__) segment it invented — or drops one an MCP tool
// actually has. An EXACT match always wins. Otherwise it compares by the final
// "__"-segment (the bare tool name) and returns the catalog entry only when that
// match is UNAMBIGUOUS (exactly one), so a wrong guess is never silently routed
// to the wrong tool. "" means no confident match (caller reports it as unknown).
func (t ActivateToolsTool) resolveLazyName(n string) string {
	if _, ok := t.byName[n]; ok {
		return n
	}
	bare := func(s string) string {
		if i := strings.LastIndex(s, "__"); i >= 0 {
			return s[i+2:]
		}
		return s
	}
	want := bare(n)
	var hits []string
	for name := range t.byName {
		if name == want || bare(name) == want {
			hits = append(hits, name)
		}
	}
	if len(hits) == 1 {
		return hits[0]
	}
	return ""
}

// resolveEagerName maps a requested name to an always-on (eager) tool, tolerating
// the same invented-namespace confusion as resolveLazyName. An EXACT match wins;
// otherwise it matches on the bare final "__"-segment when UNAMBIGUOUS. "" means
// the name is not an eager tool. Used only to turn "activate an already-shipped
// tool" into a helpful note rather than a misleading "unknown name".
func (t ActivateToolsTool) resolveEagerName(n string) string {
	if t.eager[n] {
		return n
	}
	bare := func(s string) string {
		if i := strings.LastIndex(s, "__"); i >= 0 {
			return s[i+2:]
		}
		return s
	}
	want := bare(n)
	var hits []string
	for name := range t.eager {
		if name == want || bare(name) == want {
			hits = append(hits, name)
		}
	}
	if len(hits) == 1 {
		return hits[0]
	}
	return ""
}

func (t ActivateToolsTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Names []string `json:"names"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErrFor("activate_tools", err)
	}
	var known, alwaysOn, unknown, external, bundleKeys, unknownBundles []string
	for _, n := range in.Names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		// Bundle keys are decided BEFORE any name resolution: a mistyped
		// "group:automaton" must report the known bundles, not fall into the fuzzy
		// name path (a key contains ":", which no tool name may contain).
		if _, _, isKey := SplitBundleKey(n); isKey {
			if ValidBundleKey(n) && len(t.bundles[n]) > 0 {
				bundleKeys = append(bundleKeys, n)
			} else {
				unknownBundles = append(unknownBundles, n)
			}
			continue
		}
		if r := t.resolveLazyName(n); r != "" {
			known = append(known, r)
		} else if r := t.resolveEagerName(n); r != "" {
			alwaysOn = append(alwaysOn, r)
		} else if IsExternalMCPName(n) {
			// Checked only AFTER name resolution, so the tolerant bare-name match keeps
			// working: an external MCP name that resolves to nothing here is not unknown,
			// it belongs to the CLI's own loader and needs a pointer at it, not a shrug.
			external = append(external, n)
		} else {
			unknown = append(unknown, n)
		}
	}
	// unknownBundle renders the shared "bad bundle key" note.
	unknownBundleNote := func(b *strings.Builder) {
		if len(unknownBundles) == 0 {
			return
		}
		fmt.Fprintf(b, "Unknown bundle(s): %s.", strings.Join(unknownBundles, ", "))
		if keys := t.knownBundleKeys(); len(keys) > 0 {
			fmt.Fprintf(b, " Known bundles: %s.", strings.Join(keys, ", "))
		} else {
			b.WriteString(" No bundles are available here.")
		}
		b.WriteString("\n")
	}

	if len(known) == 0 && len(bundleKeys) == 0 {
		var b strings.Builder
		if len(alwaysOn) > 0 {
			fmt.Fprintf(&b, "Nothing to activate: %s already available (always-on) — just call it directly.\n", strings.Join(alwaysOn, ", "))
		}
		if len(unknown) > 0 {
			fmt.Fprintf(&b, "Unknown names: %s. Use the exact names from the \"Available Tools (load on demand)\" list (or tool_search).\n", strings.Join(unknown, ", "))
		}
		if len(external) > 0 {
			b.WriteString(ExternalMCPActivateNote(external) + "\n")
		}
		unknownBundleNote(&b)
		if b.Len() == 0 {
			return "No tool names given.", nil
		}
		return strings.TrimSpace(b.String()), nil
	}
	added, already := t.active.Activate(known...)
	// The listing is always rendered for every requested key (a re-open is answered
	// idempotently); the open-set only feeds the "already opened" note.
	_, alreadyOpen := t.active.OpenBundle(bundleKeys...)

	var b strings.Builder
	if len(added) > 0 {
		fmt.Fprintf(&b, "Activated %d tool(s). Their schemas arrive on your NEXT step — do not call them in this same response:\n", len(added))
		for _, n := range added {
			fmt.Fprintf(&b, "- %s — %s\n", n, t.byName[n])
		}
	}
	if len(bundleKeys) > 0 {
		b.WriteString(t.openBundles(bundleKeys))
	}
	if len(alreadyOpen) > 0 {
		fmt.Fprintf(&b, "(already opened earlier this turn: %s)\n", strings.Join(alreadyOpen, ", "))
	}
	if len(already) > 0 {
		fmt.Fprintf(&b, "Already active: %s\n", strings.Join(already, ", "))
	}
	if len(alwaysOn) > 0 {
		fmt.Fprintf(&b, "Already available (always-on, no activation needed): %s\n", strings.Join(alwaysOn, ", "))
	}
	if len(unknown) > 0 {
		fmt.Fprintf(&b, "Unknown (skipped): %s\n", strings.Join(unknown, ", "))
	}
	if len(external) > 0 {
		b.WriteString(ExternalMCPActivateNote(external) + "\n")
	}
	unknownBundleNote(&b)
	return strings.TrimSpace(b.String()), nil
}

// ---- deactivate_tools -----------------------------------------------------

// DeactivateToolsTool drops tools from the active set to keep the shipped schema
// set lean once they are no longer needed.
type DeactivateToolsTool struct {
	active *ActiveTools
}

// NewDeactivateToolsTool builds the tool over the active set.
func NewDeactivateToolsTool(active *ActiveTools) DeactivateToolsTool {
	return DeactivateToolsTool{active: active}
}

func (DeactivateToolsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "deactivate_tools",
		Description: "Remove previously activated on-demand tools you no longer need, so their schemas " +
			"stop being sent. Optional housekeeping; pass the exact tool name(s). A bundle key " +
			"(\"group:<category>\", \"mcp:<server>\") closes a bundle you opened.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "names": { "type": "array", "items": { "type": "string" }, "description": "Exact tool names to deactivate." }
  },
  "required": ["names"],
  "additionalProperties": false
}`),
	}
}

func (t DeactivateToolsTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Names []string `json:"names"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErrFor("deactivate_tools", err)
	}
	var names, keys []string
	for _, n := range in.Names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, _, isKey := SplitBundleKey(n); isKey {
			keys = append(keys, n)
			continue
		}
		names = append(names, n)
	}
	removed := t.active.Deactivate(names...)
	closed := t.active.CloseBundle(keys...)
	if len(removed) == 0 && len(closed) == 0 {
		return "No active tools or open bundles matched; nothing deactivated.", nil
	}
	var b strings.Builder
	if len(removed) > 0 {
		fmt.Fprintf(&b, "Deactivated: %s\n", strings.Join(removed, ", "))
	}
	if len(closed) > 0 {
		fmt.Fprintf(&b, "Closed bundle(s): %s\n", strings.Join(closed, ", "))
	}
	return strings.TrimSpace(b.String()), nil
}

// ---- tool_search ----------------------------------------------------------

// ToolSearchTool searches the load-on-demand catalog by keyword, so the model can
// discover the right tool name in MCP-heavy workspaces with large catalogs. It is
// the keyword-search complement to the system-prompt catalog block: in MCP-heavy
// workspaces that block lists built-in tools in full but only summarises MCP tools
// per server, deferring individual discovery to this tool (mirrors the Anthropic
// "tool search" pattern — search instead of enumerate).
type ToolSearchTool struct {
	entries []lazyEntry
}

// NewToolSearchTool builds the tool over a snapshot of the lazy catalog.
func NewToolSearchTool(catalog []providers.ToolDef) ToolSearchTool {
	return ToolSearchTool{entries: toLazyEntries(catalog)}
}

func (ToolSearchTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "tool_search",
		Description: "Search the on-demand tool catalog by keyword to find a tool's exact name before " +
			"activating it. Returns matching name — description lines. Use this when a tool you need is " +
			"not listed individually in the \"Available Tools (load on demand)\" block (e.g. MCP tools, " +
			"which are summarised per server in MCP-heavy workspaces). Returns up to 30 matches; when more " +
			"exist it appends '…and N more; refine the query' so you can narrow it.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": { "type": "string", "description": "Keyword(s) to match against tool names and descriptions." }
  },
  "required": ["query"],
  "additionalProperties": false
}`),
	}
}

func (t ToolSearchTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErrFor("tool_search", err)
	}
	q := strings.ToLower(strings.TrimSpace(in.Query))
	if q == "" {
		return "Empty query.", nil
	}
	terms := strings.Fields(q)

	// Same wrong-loader redirect as the gateway path: a `select:` query, or one naming
	// a tool in a TionHarness namespace, scores zero here, and an empty answer is what
	// kept the model retrying the same call (SES79). Prepended to any outcome.
	var lead string
	if IsTionHarnessLoaderQuery(in.Query) {
		lead = TionHarnessLoaderNote(`activate_tools({"names":[...]})`) + "\n\n"
	}

	// Term-scoring (OR + rank), not strict AND: a tool matches when it contains
	// AT LEAST ONE query term, ranked by how many distinct terms it hits. This is
	// forgiving of the common "search these several tool names" query — passing a
	// list of full names (e.g. "list_tasks move_task create_task") surfaces all of
	// them instead of the empty result strict AND used to give (no single tool
	// contains every term). A name hit outweighs a description hit so exact-name
	// candidates float to the top. Ties break on term-count, then name-hit, then name.
	type scored struct {
		e        lazyEntry
		terms    int // distinct query terms found anywhere (name or desc)
		nameHits int // distinct query terms found in the name (stronger signal)
	}
	var ranked []scored
	for _, e := range t.entries {
		name := strings.ToLower(e.name)
		hay := name + " " + strings.ToLower(e.desc)
		var termHits, nameHits int
		for _, term := range terms {
			if strings.Contains(hay, term) {
				termHits++
				if strings.Contains(name, term) {
					nameHits++
				}
			}
		}
		if termHits > 0 {
			ranked = append(ranked, scored{e: e, terms: termHits, nameHits: nameHits})
		}
	}
	if len(ranked) == 0 {
		return lead + fmt.Sprintf("No on-demand tools match %q.", in.Query), nil
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].terms != ranked[j].terms {
			return ranked[i].terms > ranked[j].terms
		}
		if ranked[i].nameHits != ranked[j].nameHits {
			return ranked[i].nameHits > ranked[j].nameHits
		}
		return ranked[i].e.name < ranked[j].e.name
	})
	// Rendering (row shape, bundle tag, overflow notice) is shared with the
	// claude-cli gateway path via RenderToolSearch, so a change to the result
	// format lands on both at once instead of drifting apart.
	rows := make([]ToolSearchRow, 0, len(ranked))
	for _, r := range ranked {
		rows = append(rows, ToolSearchRow{Name: r.e.name, Desc: r.e.desc})
	}
	return lead + RenderToolSearch(rows, ToolSearchRenderOpts{
		Header:   "Matching tools (activate with activate_tools):",
		Max:      ToolSearchMaxRows,
		BundleOf: BundleOf,
	}), nil
}
