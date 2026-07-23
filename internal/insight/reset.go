package insight

import (
	"os"
	"path/filepath"
)

// Reset clears a workspace's accumulated insight DATA so the panel starts fresh.
// It always removes the findings, the run log and the workspace-opt actions doc.
//
// The LEDGER is kept by default (deep=false): it records which (lens,session)
// pairs were already scanned, so keeping it means a re-scan will NOT re-analyze
// unchanged old sessions — the board clears and does not immediately re-fill with
// the same findings distilled from history. This is the reset you want when
// repeated scans pile up similar items.
//
// deep=true ALSO removes the ledger, so the next scan re-analyzes EVERY session
// from scratch. Use only for a true from-zero re-run — it will re-surface
// findings from old sessions even for issues you have since fixed, because the
// old session still contains the original failure evidence.
//
// Lenses and settings are configuration and are never touched.
func Reset(root string, deep bool) error {
	if root == "" {
		return nil
	}
	paths := []string{
		filepath.Join(root, findingsRelPath),
		filepath.Join(root, runsRelPath),
		filepath.Join(root, workspaceActionsRelPath),
	}
	if deep {
		paths = append(paths, filepath.Join(root, ledgerRelPath))
	}
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
