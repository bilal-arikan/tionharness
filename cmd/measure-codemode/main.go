// Command measure-codemode is the Faz 0 measurement harness for the
// code-execution-with-MCP plan (_Docs/44 §5/§6): it connects to the MCP servers
// configured in a TionHarness data dir, pulls their REAL tool catalogs, and reports
// the per-turn context-occupancy cost of three scenarios:
//
//  1. eager-full  — every MCP tool's full schema shipped per turn (the industry
//     baseline Anthropic's "7 servers ≈ 67.3K tokens" number describes);
//  2. tier-lazy   — TionHarness's current default (MCP tools lazy + name-only:
//     catalog lines per turn, full schema only when activated);
//  3. code-mode   — the run_code tool alone (bindings live on disk, not in
//     context; the discovery listing is a one-time cost when requested).
//
// Token counts use the same density-aware estimator the runtime budgets with
// (internal/conversation.EstimateText), so numbers are comparable to the UI
// meter, not to any specific provider tokenizer.
//
// Usage:
//
//	go run ./cmd/measure-codemode [-data ~/.tionharness] [-timeout 30s]
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	defaultData := ""
	if home, err := os.UserHomeDir(); err == nil {
		defaultData = filepath.Join(home, ".tionharness")
	}
	dataDir := flag.String("data", defaultData, "TionHarness data dir (workspaces are scanned for MCP server configs)")
	timeout := flag.Duration("timeout", 30*time.Second, "per-server connect+list timeout")
	flag.Parse()

	servers, err := loadServers(*dataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if len(servers) == 0 {
		fmt.Fprintln(os.Stderr, "no enabled MCP servers found under", *dataDir)
		os.Exit(1)
	}

	report(servers, *timeout)
}
