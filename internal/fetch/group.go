package fetch

import (
	"path"
	"sort"
	"strings"
)

// Group is a folder identified by a MARKER file (e.g. SKILL.md) plus the marker's
// content and the sibling resource files belonging to that folder. It is the unit
// the ingest adapters turn into packs.
type Group struct {
	RelPath string            // folder path within the tree ("" = root)
	Marker  []byte            // the marker file's content
	Files   map[string][]byte // sibling resources (relpath -> content), deepest-owner grouped
}

// GroupByMarker groups a flat path->content tree into Groups, one per folder that
// contains a file named marker (under the optional prefix filter). Each non-marker
// file is assigned to the DEEPEST marker folder that is its ancestor, so a nested
// marker keeps its own resources and a parent folder doesn't claim them. A
// root-level marker (marker at the tree root) is supported (RelPath == "").
func GroupByMarker(tree Tree, prefix, marker string) []Group {
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	folders := map[string]bool{}
	for p := range tree {
		if path.Base(p) != marker {
			continue
		}
		folder := DirOf(p)
		if !underPrefix(folder, prefix) {
			continue
		}
		folders[folder] = true
	}
	if len(folders) == 0 {
		return nil
	}
	ordered := make([]string, 0, len(folders))
	for f := range folders {
		ordered = append(ordered, f)
	}
	sort.Slice(ordered, func(i, j int) bool { return len(ordered[i]) > len(ordered[j]) })

	byFolder := map[string]*Group{}
	for f := range folders {
		byFolder[f] = &Group{RelPath: f, Marker: tree[joinPath(f, marker)], Files: map[string][]byte{}}
	}
	for p, data := range tree {
		if path.Base(p) == marker && folders[DirOf(p)] {
			continue // marker is carried separately
		}
		owner, ok := deepestOwner(p, ordered)
		if !ok {
			continue
		}
		rel := strings.TrimPrefix(p, joinPath(owner, ""))
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			continue
		}
		byFolder[owner].Files[rel] = data
	}
	out := make([]Group, 0, len(byFolder))
	for _, g := range byFolder {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out
}

// FindFiles returns the tree paths whose base name matches pred, under the prefix,
// in sorted order. Used by adapters whose artifacts are single files (CC subagents,
// slash commands) rather than marker folders.
func FindFiles(tree Tree, prefix string, pred func(name string) bool) []string {
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	var out []string
	for p := range tree {
		if !underPrefix(p, prefix) {
			continue
		}
		if pred(path.Base(p)) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// DirOf returns the parent directory of a forward-slashed path, "" for a top-level
// file (unlike path.Dir which returns ".").
func DirOf(p string) string {
	d := path.Dir(p)
	if d == "." || d == "/" {
		return ""
	}
	return d
}

// joinPath joins a folder ("" = root) and a name with a forward slash.
func joinPath(folder, name string) string {
	if folder == "" {
		return name
	}
	if name == "" {
		return folder
	}
	return folder + "/" + name
}

// underPrefix reports whether p is the prefix itself or nested under it. An empty
// prefix matches everything.
func underPrefix(p, prefix string) bool {
	if prefix == "" {
		return true
	}
	return p == prefix || strings.HasPrefix(p, prefix+"/")
}

// deepestOwner returns the deepest folder in folders (pre-sorted longest-first) that
// equals or is an ancestor of path p, with ok=false when none owns it. A root-level
// folder ("") claims any otherwise-unowned file and is returned as ("", true).
func deepestOwner(p string, folders []string) (string, bool) {
	hasRoot := false
	for _, f := range folders {
		if f == "" {
			hasRoot = true
			continue
		}
		if p == f || strings.HasPrefix(p, f+"/") {
			return f, true
		}
	}
	if hasRoot {
		return "", true
	}
	return "", false
}
