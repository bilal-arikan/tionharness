package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// wsMeta mirrors workspace.Meta (kept local so the tool has no internal deps).
type wsMeta struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
	Path      string `json:"path,omitempty"`
	CreatedBy string `json:"createdBy,omitempty"`
}

func workspacesJSONPath(dataDir string) string { return filepath.Join(dataDir, "workspaces.json") }

func loadWorkspaceMetas(dataDir string) ([]wsMeta, error) {
	var metas []wsMeta
	err := readJSON(workspacesJSONPath(dataDir), &metas)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return metas, nil
}

// resolveWorkspaceDirs returns the on-disk directory for every workspace. It
// uses workspaces.json when present (honoring custom Path), else scans
// <dataDir>/workspaces/*.
func resolveWorkspaceDirs(dataDir string, metas []wsMeta) []string {
	var dirs []string
	if len(metas) > 0 {
		for _, m := range metas {
			dirs = append(dirs, workspaceDir(dataDir, m))
		}
		return dirs
	}
	entries, err := os.ReadDir(filepath.Join(dataDir, "workspaces"))
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join(dataDir, "workspaces", e.Name()))
		}
	}
	return dirs
}

func workspaceDir(dataDir string, m wsMeta) string {
	if m.Path != "" {
		return m.Path
	}
	return filepath.Join(dataDir, "workspaces", m.ID)
}

// migrateWorkspaceIDs renames legacy UUID workspace ids to WS<n>: it renames the
// workspace directory and updates workspaces.json + ws-counter.json. Workspace
// ids are not embedded in any entity file (workspaces are isolated), so no
// content rewrite is needed.
func migrateWorkspaceIDs(dataDir string, metas []wsMeta, apply bool) error {
	if len(metas) == 0 {
		fmt.Println("\nworkspace ids: no workspaces.json — skipping workspace-id migration")
		return nil
	}

	counter := loadWSCounterN(dataDir)
	for _, m := range metas {
		if n := wsNum(m.ID); n > counter {
			counter = n
		}
	}

	type change struct {
		oldDir, newDir string
		oldID, newID   string
	}
	var changes []change
	for i := range metas {
		if wsNum(metas[i].ID) > 0 {
			continue // already WS<n>
		}
		counter++
		newID := "WS" + strconv.FormatInt(counter, 10)
		oldDir := workspaceDir(dataDir, metas[i])
		newDir := oldDir
		if metas[i].Path != "" {
			// Custom location: dir is "<parent>/tionharness-<id>".
			parent := filepath.Dir(metas[i].Path)
			newDir = filepath.Join(parent, "tionharness-"+newID)
			metas[i].Path = newDir
		} else {
			newDir = filepath.Join(dataDir, "workspaces", newID)
		}
		changes = append(changes, change{oldDir, newDir, metas[i].ID, newID})
		metas[i].ID = newID
	}

	if len(changes) == 0 {
		fmt.Println("\nworkspace ids: nothing to migrate (already WS<n>)")
		return nil
	}

	fmt.Printf("\nworkspace ids: %d to remap\n", len(changes))
	for _, c := range changes {
		fmt.Printf("      %s -> %s\n", c.oldID, c.newID)
	}
	if !apply {
		return nil
	}

	for _, c := range changes {
		if err := rename(c.oldDir, c.newDir); err != nil {
			return err
		}
	}
	if err := writeJSONAtomic(workspacesJSONPath(dataDir), metas); err != nil {
		return err
	}
	if err := writeWSCounterN(dataDir, counter); err != nil {
		return err
	}
	fmt.Println("      applied: dirs renamed + workspaces.json + ws-counter.json updated")
	return nil
}

func wsNum(id string) int64 {
	if !strings.HasPrefix(id, "WS") {
		return 0
	}
	n, err := strconv.ParseInt(id[2:], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func wsCounterPath(dataDir string) string { return filepath.Join(dataDir, "ws-counter.json") }

func loadWSCounterN(dataDir string) int64 {
	var v struct {
		N int64 `json:"n"`
	}
	if readJSON(wsCounterPath(dataDir), &v) != nil {
		return 0
	}
	return v.N
}

func writeWSCounterN(dataDir string, n int64) error {
	return writeJSONAtomic(wsCounterPath(dataDir), struct {
		N int64 `json:"n"`
	}{n})
}
