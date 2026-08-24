package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// migrateWorkspace migrates the entity ids of one workspace (the directory that
// contains store/ and workspace/). Returns how many ids were (or would be)
// remapped.
func migrateWorkspace(wsDir string, apply bool) (int, error) {
	storeDir := filepath.Join(wsDir, "store")
	if _, err := os.Stat(storeDir); err != nil {
		return 0, nil // not a workspace dir
	}
	contentDir := filepath.Join(wsDir, "workspace")

	r, err := buildRemap(storeDir)
	if err != nil {
		return 0, err
	}
	fmt.Printf("workspace %s\n", filepath.Base(wsDir))
	reportRemap("entities", r)
	if len(r.m) == 0 {
		return 0, nil
	}
	if !apply {
		return len(r.m), nil
	}

	rep := strings.NewReplacer(replacerPairs(r.m)...)

	// 1) Rewrite text file contents under store/ only — that is where every
	// id reference lives (entity JSON + session.jsonl). The workspace content
	// dir holds only artifact bodies/uploads (no id references) and may be a
	// junction to a real project, so we never recurse into it.
	if err := rewriteContents(storeDir, rep); err != nil {
		return 0, err
	}
	// 2) Rename artifact content files, then the folders that are named by an id.
	if err := renameArtifactContents(contentDir, r.m); err != nil {
		return 0, err
	}
	if err := renameStoreEntities(storeDir, r.m); err != nil {
		return 0, err
	}
	if err := renameUsage(storeDir, r.m); err != nil {
		return 0, err
	}
	if err := renameSubdirs(filepath.Join(contentDir, "uploads"), r.m); err != nil {
		return 0, err
	}
	if err := renameSubdirs(filepath.Join(contentDir, "artifacts"), r.m); err != nil {
		return 0, err
	}
	// 3) Persist the advanced counters so the running app keeps numbering forward.
	if err := writeCounters(storeDir, r.counters); err != nil {
		return 0, err
	}
	fmt.Printf("      applied: contents rewritten + files/folders renamed + counters.json updated\n")
	return len(r.m), nil
}

func replacerPairs(m map[string]string) []string {
	pairs := make([]string, 0, len(m)*2)
	for old, nw := range m {
		pairs = append(pairs, old, nw)
	}
	return pairs
}

// rewriteContents walks root and, for every non-binary regular file, replaces
// every old id token with its new id, writing back atomically only on change.
func rewriteContents(root string, rep *strings.Replacer) error {
	if _, err := os.Stat(root); err != nil {
		return nil
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if isReparsePoint(path) { // never follow a junction/symlink into another tree
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || strings.HasSuffix(path, ".tmp") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.IndexByte(data, 0) >= 0 {
			return nil // binary (image/media): never contains an id token
		}
		out := rep.Replace(string(data))
		if out == string(data) {
			return nil
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, []byte(out), 0o644); err != nil {
			return err
		}
		return os.Rename(tmp, path)
	})
}

// renameStoreEntities renames "<id>.json" files (and session id folders) under
// each entity directory to their new id.
func renameStoreEntities(storeDir string, m map[string]string) error {
	for _, spec := range specs {
		dir := filepath.Join(storeDir, spec.dir)
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, e := range entries {
			if spec.isFolder {
				if !e.IsDir() {
					continue
				}
				if nw, ok := m[e.Name()]; ok {
					if err := rename(filepath.Join(dir, e.Name()), filepath.Join(dir, nw)); err != nil {
						return err
					}
				}
				continue
			}
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".json") {
				continue
			}
			id := strings.TrimSuffix(name, ".json")
			if nw, ok := m[id]; ok {
				if err := rename(filepath.Join(dir, name), filepath.Join(dir, nw+".json")); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// renameUsage renames usage files "<agentId>__<day>.json" when the agent id moved.
func renameUsage(storeDir string, m map[string]string) error {
	dir := filepath.Join(storeDir, "usage")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		i := strings.Index(name, "__")
		if i < 0 {
			continue
		}
		if nw, ok := m[name[:i]]; ok {
			if err := rename(filepath.Join(dir, name), filepath.Join(dir, nw+name[i:])); err != nil {
				return err
			}
		}
	}
	return nil
}

// renameArtifactContents renames the per-artifact content files
// (workspace/artifacts/<sessionDir>/<artifactId><ext>) whose basename is a
// migrated artifact id. The parent session folders are renamed afterwards by
// renameSubdirs.
func renameArtifactContents(contentDir string, m map[string]string) error {
	root := filepath.Join(contentDir, "artifacts")
	subdirs, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, sd := range subdirs {
		if !sd.IsDir() {
			continue
		}
		sub := filepath.Join(root, sd.Name())
		files, err := os.ReadDir(sub)
		if err != nil {
			return err
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			name := f.Name()
			ext := filepath.Ext(name)
			base := strings.TrimSuffix(name, ext)
			if nw, ok := m[base]; ok {
				if err := rename(filepath.Join(sub, name), filepath.Join(sub, nw+ext)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// renameSubdirs renames immediate subdirectories of root whose name is a
// migrated id (used for workspace/uploads/<sessionId> and the artifact session
// folders workspace/artifacts/<sessionId>).
func renameSubdirs(root string, m map[string]string) error {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if nw, ok := m[e.Name()]; ok {
			if err := rename(filepath.Join(root, e.Name()), filepath.Join(root, nw)); err != nil {
				return err
			}
		}
	}
	return nil
}

// rename moves src->dst, refusing to clobber an existing dst (a collision would
// signal a logic error rather than a normal case).
func rename(src, dst string) error {
	if src == dst {
		return nil
	}
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("refusing to overwrite existing %q", dst)
	}
	return os.Rename(src, dst)
}

// ---- backup ----

func backupPath(dataDir, suffix string) string {
	return filepath.Clean(dataDir) + "-" + suffix
}

// copyTree recursively copies src to dst (used for the pre-apply backup). It
// never follows reparse points (junctions/symlinks): those point outside the
// TionHarness data tree (e.g. a workspace whose content dir is junctioned to a real
// project) and are not ours to back up — each is skipped with a warning.
func copyTree(src, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("backup target %q already exists", dst)
	}
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if isReparsePoint(path) {
			fmt.Printf("  (backup) skipping junction/symlink: %s\n", path)
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

// isReparsePoint reports whether path is a symlink or (on Windows) a junction /
// mount point — anything we must not transparently descend into.
func isReparsePoint(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
