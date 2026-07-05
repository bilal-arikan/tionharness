// Command migrate-ids rewrites a TionSwarm data directory's legacy UUID entity ids
// to the human-readable prefixed scheme (AGT3, SES42, TSK17, ...) and, optionally,
// workspace ids to WS1/WS2/.... It is a one-time, idempotent migration.
//
// Strategy: every UUID is a globally-unique token, so the migration (1) builds an
// old->new id map for every migratable entity, (2) literally replaces those tokens
// across ALL text files in the workspace tree — catching every cross-reference
// (AgentID, SessionID, OwnerAgentID, Task.Dependencies, Flow.Graph node ids,
// content-file paths, ...) without enumerating fields — and (3) renames the files
// and folders that are named by an id (entity JSON, session folders, artifact
// content files, upload folders, usage files).
//
// Runs as a DRY RUN by default: it prints what it would do and changes nothing.
// Pass -apply to perform the migration (a full backup is taken first unless
// -backup=false).
//
// Usage:
//
//	go run ./cmd/migrate-ids                 # dry run against ~/.tionswarm
//	go run ./cmd/migrate-ids -apply          # apply (backs up first)
//	go run ./cmd/migrate-ids -data D:\sg -apply -workspaces=false
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	var (
		dataDir    = flag.String("data", defaultDataDir(), "TionSwarm data directory (contains workspaces.json + workspaces/)")
		apply      = flag.Bool("apply", false, "perform the migration (default: dry run, no changes)")
		backup     = flag.Bool("backup", true, "with -apply, copy the whole data dir to a timestamped backup first")
		doWS       = flag.Bool("workspaces", true, "also migrate workspace ids to the WS<n> scheme")
		backupTime = flag.String("backup-suffix", "idbackup", "suffix used for the backup directory name")
	)
	flag.Parse()

	if err := run(*dataDir, *apply, *backup, *doWS, *backupTime); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(dataDir string, apply, backup, doWS bool, backupSuffix string) error {
	if _, err := os.Stat(dataDir); err != nil {
		return fmt.Errorf("data dir %q not accessible: %w", dataDir, err)
	}
	fmt.Printf("TionSwarm id migration\n  data dir : %s\n  mode     : %s\n\n", dataDir, modeLabel(apply))

	metas, err := loadWorkspaceMetas(dataDir)
	if err != nil {
		return err
	}
	if len(metas) == 0 {
		fmt.Println("no workspaces.json found — scanning workspaces/ directly")
	}
	wsDirs := resolveWorkspaceDirs(dataDir, metas)
	if len(wsDirs) == 0 {
		return fmt.Errorf("no workspaces found under %s", dataDir)
	}

	// Backup must happen before the first mutation.
	if apply && backup {
		dst := backupPath(dataDir, backupSuffix)
		fmt.Printf("backing up %s -> %s ...\n", dataDir, dst)
		if err := copyTree(dataDir, dst); err != nil {
			return fmt.Errorf("backup failed (nothing changed): %w", err)
		}
		fmt.Println("backup complete.")
	}

	// 1) Migrate entity ids inside every workspace (uses original paths).
	totalRemapped := 0
	for _, wsDir := range wsDirs {
		n, err := migrateWorkspace(wsDir, apply)
		if err != nil {
			return fmt.Errorf("workspace %s: %w", wsDir, err)
		}
		totalRemapped += n
	}

	// 2) Migrate workspace ids last (renames the workspace dirs themselves).
	if doWS {
		if err := migrateWorkspaceIDs(dataDir, metas, apply); err != nil {
			return fmt.Errorf("workspace-id migration: %w", err)
		}
	}

	fmt.Printf("\nDone. %d entity id(s) %s.\n", totalRemapped, doneVerb(apply))
	if !apply {
		fmt.Println("This was a DRY RUN — re-run with -apply to make changes.")
	}
	return nil
}

func modeLabel(apply bool) string {
	if apply {
		return "APPLY (writes changes)"
	}
	return "DRY RUN (no changes)"
}

func doneVerb(apply bool) string {
	if apply {
		return "migrated"
	}
	return "would be migrated"
}

// defaultDataDir mirrors internal/config: $TIONSWARM_DATA_DIR or ~/.tionswarm.
func defaultDataDir() string {
	if v := os.Getenv("TIONSWARM_DATA_DIR"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".tionswarm"
	}
	return filepath.Join(home, ".tionswarm")
}
