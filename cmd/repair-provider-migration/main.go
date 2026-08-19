// Command repair-provider-migration fixes the on-disk fallout of the provider
// rework (kind-keyed providers → provider INSTANCES, per-workspace CLI homes →
// one app-global home). It is a one-time, idempotent repair over a TionSwarm
// data directory, in three passes:
//
//  1. providers.json — recreate the instances the settings→instances migration
//     never emitted. The "-anthropic" kinds (minimax-anthropic,
//     deepseek-anthropic) had no legacy settings key of their own (they reuse
//     the base provider's credential), so agents bound to them were left
//     pointing at an instance that does not exist.
//  2. agents — repoint any agent whose providerInstanceId names an instance
//     that is gone (e.g. a deleted test instance) to the default instance of
//     its own kind. Registry.Get treats an unknown instance as a hard error, so
//     such an agent cannot run at all.
//  3. sessions — the claude-cli resume ids. Every session stores the CLI
//     conversation to --resume next turn, but those transcripts live under the
//     config home that was in force when they were written. With the home moved
//     app-global, the CLI answers "No conversation found with session ID" and
//     the turn fails (and retries the same dead id). Each id is relocated into
//     the new home when its transcript is still findable in the workspace's
//     legacy home, and cleared otherwise so the next turn simply starts cold.
//
// Runs as a DRY RUN by default: it prints what it would do and changes nothing.
// Pass -apply to write. Stop the app first — it holds these files in memory and
// would write its own state back over the repair.
//
// Usage:
//
//	go run ./cmd/repair-provider-migration            # dry run against ~/.tionswarm
//	go run ./cmd/repair-provider-migration -apply
//	go run ./cmd/repair-provider-migration -data D:\ts -apply
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	var (
		dataDir = flag.String("data", defaultDataDir(), "TionSwarm data directory (contains providers.json + workspaces/)")
		apply   = flag.Bool("apply", false, "write the repairs (default: dry run, no changes)")
	)
	flag.Parse()

	if err := run(*dataDir, *apply); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(dataDir string, apply bool) error {
	if dataDir == "" {
		return fmt.Errorf("data directory is required")
	}
	if _, err := os.Stat(filepath.Join(dataDir, "providers.json")); err != nil {
		return fmt.Errorf("not a TionSwarm data directory (%s): %w", dataDir, err)
	}
	mode := "DRY RUN (no changes; pass -apply to write)"
	if apply {
		mode = "APPLY"
	}
	fmt.Printf("repair-provider-migration — %s\ndata: %s\n\n", mode, dataDir)

	instanceIDs, err := repairInstances(dataDir, apply)
	if err != nil {
		return err
	}
	if err := repairAgents(dataDir, instanceIDs, apply); err != nil {
		return err
	}
	if err := repairSessions(dataDir, apply); err != nil {
		return err
	}
	if !apply {
		fmt.Println("\nnothing was written (dry run).")
	}
	return nil
}

// defaultDataDir is ~/.tionswarm, matching internal/config's default.
func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".tionswarm")
}

// workspaceDirs lists the per-workspace roots under <dataDir>/workspaces.
func workspaceDirs(dataDir string) ([]string, error) {
	root := filepath.Join(dataDir, "workspaces")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read workspaces dir: %w", err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, filepath.Join(root, e.Name()))
		}
	}
	return out, nil
}
