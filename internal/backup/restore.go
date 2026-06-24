package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ArchiveFile is one archive on disk for a workspace.
type ArchiveFile struct {
	Name     string `json:"name"`     // file name, e.g. "WS1-20260625-000808.zip"
	Bytes    int64  `json:"bytes"`    // size on disk
	Modified int64  `json:"modified"` // unix seconds
}

// WorkspaceArchives groups a workspace's archives, newest first.
type WorkspaceArchives struct {
	WorkspaceID   string        `json:"workspaceId"`
	WorkspaceName string        `json:"workspaceName"`
	Archives      []ArchiveFile `json:"archives"`
}

// ListArchives scans the backups root and returns each target workspace's
// archives (newest first). Workspaces with no archives are still listed (empty
// list) so the UI can show "no backups yet". The targets supply the id→name
// mapping and order.
func (m *Manager) ListArchives(targets []Target) []WorkspaceArchives {
	m.mu.Lock()
	root := m.resolveDir(m.cfg)
	m.mu.Unlock()

	out := make([]WorkspaceArchives, 0, len(targets))
	for _, t := range targets {
		wa := WorkspaceArchives{WorkspaceID: t.ID, WorkspaceName: t.Name, Archives: []ArchiveFile{}}
		dir := filepath.Join(root, t.ID)
		entries, err := os.ReadDir(dir)
		if err == nil {
			prefix := t.ID + "-"
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				name := e.Name()
				if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".zip") {
					continue
				}
				info, ierr := e.Info()
				if ierr != nil {
					continue
				}
				wa.Archives = append(wa.Archives, ArchiveFile{
					Name: name, Bytes: info.Size(), Modified: info.ModTime().Unix(),
				})
			}
			// Newest first ("<id>-YYYYMMDD-HHMMSS.zip" sorts chronologically).
			sort.Slice(wa.Archives, func(i, j int) bool { return wa.Archives[i].Name > wa.Archives[j].Name })
		}
		out = append(out, wa)
	}
	return out
}

// ResolveArchive validates a (workspaceID, archive name) pair and returns the
// absolute archive path. The name must be a bare file name (no path separators,
// no "..") that exists under the workspace's backups folder — this is the guard
// against path traversal from an API caller.
func (m *Manager) ResolveArchive(workspaceID, name string) (string, error) {
	if workspaceID == "" || name == "" {
		return "", fmt.Errorf("workspaceId and archive are required")
	}
	if name != filepath.Base(name) || strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("invalid archive name")
	}
	if !strings.HasSuffix(name, ".zip") {
		return "", fmt.Errorf("invalid archive name")
	}
	m.mu.Lock()
	root := m.resolveDir(m.cfg)
	m.mu.Unlock()

	path := filepath.Join(root, workspaceID, name)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", fmt.Errorf("archive not found")
	}
	return path, nil
}

// DeleteArchive removes a single archive file. The (workspaceID, name) pair is
// validated by ResolveArchive (path-traversal guard) before deletion.
func (m *Manager) DeleteArchive(workspaceID, name string) error {
	path, err := m.ResolveArchive(workspaceID, name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}
