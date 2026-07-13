package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/codemode"
	"github.com/bilal-arikan/tionswarm/internal/conversation"
	"github.com/bilal-arikan/tionswarm/internal/mcp"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// lazyCatalogMCPListLimit mirrors internal/agent's constant of the same name:
// above this many MCP tools the per-turn catalog switches from one line per
// tool to one summary line per server.
const lazyCatalogMCPListLimit = 50

// serverResult is one server's measured catalog (or its connection failure).
type serverResult struct {
	cfg     mcp.ServerConfig
	entries []mcp.CatalogEntry
	err     error
}

// report connects to every server, measures the three scenarios and prints the
// Faz 0 baseline table.
func report(cfgs []mcp.ServerConfig, timeout time.Duration) {
	fmt.Printf("Faz 0 baseline — code execution with MCP (_Docs/44)\n")
	fmt.Printf("Estimator: internal/conversation.EstimateText (density-aware ~4 chars/token)\n\n")

	var results []serverResult
	var all []mcp.CatalogEntry
	for _, cfg := range cfgs {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		entries, err := mcp.ListServerTools(ctx, cfg)
		cancel()
		res := serverResult{cfg: cfg, err: err}
		for _, t := range entries {
			e := mcp.CatalogEntry{Server: cfg.Name, NamespacedName: mcp.NamespaceTool(cfg.Name, t.Name), Tool: t}
			res.entries = append(res.entries, e)
			all = append(all, e)
		}
		results = append(results, res)
	}

	fmt.Println("== Servers ==")
	for _, r := range results {
		if r.err != nil {
			fmt.Printf("  %-20s UNREACHABLE (%v)\n", r.cfg.Name, r.err)
			continue
		}
		bytes, tokens := eagerCost(r.entries)
		fmt.Printf("  %-20s %3d tools   full schemas: %7d bytes ≈ %6d tokens\n",
			r.cfg.Name, len(r.entries), bytes, tokens)
	}
	if len(all) == 0 {
		fmt.Println("\nno tools measured — nothing reachable")
		return
	}

	eagerBytes, eagerTokens := eagerCost(all)
	tierTokens, tierDesc := tierLazyCost(all)
	avgSchema := eagerTokens / len(all)

	runCodeDef := tools.NewRunCodeTool(tools.Sandbox{}, all, nil, nil, nil, nil, nil, nil).Def()
	runCodeTokens := defTokens(providers.ToolDef{
		Name: runCodeDef.Name, Description: runCodeDef.Description, InputSchema: runCodeDef.InputSchema,
	})

	bindDir, err := os.MkdirTemp("", "tionswarm-measure-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "temp dir:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(bindDir)
	modules, err := codemode.WriteBindings(filepath.Join(bindDir, "mcp"), all, nil, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bindings:", err)
		os.Exit(1)
	}
	bindBytes := dirBytes(filepath.Join(bindDir, "mcp"))
	listingTokens := conversation.EstimateText(discoveryListing(modules))

	fmt.Printf("\n== Per-turn context occupancy (%d tools across %d reachable servers) ==\n",
		len(all), reachable(results))
	fmt.Printf("  1. eager-full (industry baseline) : %7d bytes ≈ %6d tokens EVERY turn\n", eagerBytes, eagerTokens)
	fmt.Printf("  2. tier-lazy (TionSwarm today)      : %s ≈ %6d tokens/turn (+~%d tokens per activated tool)\n",
		tierDesc, tierTokens, avgSchema)
	fmt.Printf("  3. code-mode (run_code)           : 1 schema ≈ %6d tokens/turn\n", runCodeTokens)
	fmt.Printf("       bindings on DISK (0 context) : %7d bytes across %d modules\n", bindBytes, len(modules))
	fmt.Printf("       discovery listing (one-time) : ≈ %6d tokens when requested\n", listingTokens)

	fmt.Printf("\n== Reductions vs eager-full ==\n")
	fmt.Printf("  tier-lazy : %5.1f%%   code-mode : %5.1f%%\n",
		100*(1-float64(tierTokens)/float64(eagerTokens)),
		100*(1-float64(runCodeTokens)/float64(eagerTokens)))
}

// eagerCost sums the wire-format cost of shipping every entry's FULL ToolDef
// (name + description + normalized schema), mirroring registry.Defs.
func eagerCost(entries []mcp.CatalogEntry) (bytes, tokens int) {
	for _, e := range entries {
		d := providers.ToolDef{
			Name:        e.NamespacedName,
			Description: e.Tool.Description,
			InputSchema: mcp.NormalizeSchema(e.Tool.InputSchema),
		}
		raw, _ := json.Marshal(d)
		bytes += len(raw)
		tokens += conversation.EstimateText(string(raw))
	}
	return bytes, tokens
}

// defTokens estimates one ToolDef's wire cost.
func defTokens(d providers.ToolDef) int {
	raw, _ := json.Marshal(d)
	return conversation.EstimateText(string(raw))
}

// tierLazyCost estimates the per-turn cost of TionSwarm's current default for MCP
// tools: name-only lines in the load-on-demand catalog block while under the
// list limit, otherwise one "server — N tools" summary line per server (plus
// the pointer sentence), mirroring renderLazyToolCatalog in internal/agent.
func tierLazyCost(entries []mcp.CatalogEntry) (tokens int, desc string) {
	if len(entries) <= lazyCatalogMCPListLimit {
		var b strings.Builder
		for _, e := range entries {
			fmt.Fprintf(&b, "- `%s`\n", e.NamespacedName)
		}
		return conversation.EstimateText(b.String()), fmt.Sprintf("%d name-only lines", len(entries))
	}
	counts := map[string]int{}
	for _, e := range entries {
		counts[e.Server]++
	}
	var names []string
	for s := range counts {
		names = append(names, s)
	}
	sort.Strings(names)
	var b strings.Builder
	fmt.Fprintf(&b, "\n%d more tools are available from MCP servers but not listed individually "+
		"(to save context). Find one with `tool_search(\"keyword\")`, then `activate_tools` it. Servers:\n", len(entries))
	for _, s := range names {
		fmt.Fprintf(&b, "- `%s` — %d tools\n", s, counts[s])
	}
	return conversation.EstimateText(b.String()), fmt.Sprintf("%d per-server summary lines", len(counts))
}

// discoveryListing reconstructs the run_code discovery result text (module ->
// functions), matching the shape renderBindingListing emits.
func discoveryListing(modules map[string][]string) string {
	names := make([]string, 0, len(modules))
	for m := range modules {
		names = append(names, m)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("MCP bindings regenerated under .tionswarm/mcp/ (on PYTHONPATH for run_code scripts).\nModules:\n")
	for _, m := range names {
		fmt.Fprintf(&b, "- %s: %s\n", m, strings.Join(modules[m], ", "))
	}
	return b.String()
}

// dirBytes sums the sizes of all files under dir.
func dirBytes(dir string) int {
	total := 0
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += int(info.Size())
		}
		return nil
	})
	return total
}

// reachable counts servers that answered tools/list.
func reachable(results []serverResult) int {
	n := 0
	for _, r := range results {
		if r.err == nil {
			n++
		}
	}
	return n
}
