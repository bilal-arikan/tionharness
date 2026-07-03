package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// FSGlobTool finds files in the sandbox matching a glob pattern.
type FSGlobTool struct{ sb Sandbox }

// NewFSGlobTool binds the tool to a workspace sandbox.
func NewFSGlobTool(sb Sandbox) FSGlobTool { return FSGlobTool{sb: sb} }

func (FSGlobTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "Glob",
		Description: "Find files matching a glob pattern (e.g. \"**/*.go\", \"src/*.ts\"). Results are sorted by modification time, most-recently-modified first. " +
			"Searches the working directory by default; pass path to search a different directory. Honours .gitignore (and always skips .git) unless no_ignore is set. Returns paths relative to the search root.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"pattern":{"type":"string","description":"Glob pattern; ** matches any number of directories"},
				"path":{"type":"string","description":"Directory to search in (absolute or relative to the working directory); default working directory"},
				"no_ignore":{"type":"boolean","description":"Include files that .gitignore would exclude (default false)"}
			},
			"required":["pattern"],
			"additionalProperties":false
		}`),
	}
}

func (t FSGlobTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Pattern  string `json:"pattern"`
		Path     string `json:"path"`
		NoIgnore bool   `json:"no_ignore"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", argErr(err)
	}
	if strings.TrimSpace(args.Pattern) == "" {
		return "", fmt.Errorf("pattern is required")
	}
	root, err := t.searchRoot(args.Path)
	if err != nil {
		return "", err
	}
	re, err := globToRegexp(args.Pattern)
	if err != nil {
		return "", err
	}

	ign := NewIgnoreSet(root, !args.NoIgnore)
	ign.LoadDir("") // root .gitignore applies to the whole tree
	type hit struct {
		rel string
		mod time.Time
	}
	var hits []hit
	walkErr := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		rel := relTo(root, p)
		if d.IsDir() {
			if rel == "" || rel == "." {
				return nil
			}
			ign.LoadDir(rel)
			if ign.Ignored(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if ign.Ignored(rel, false) {
			return nil
		}
		if !re.MatchString(rel) {
			return nil
		}
		var mod time.Time
		if info, ierr := d.Info(); ierr == nil {
			mod = info.ModTime()
		}
		hits = append(hits, hit{rel: rel, mod: mod})
		if len(hits) >= fsGlobMaxHits {
			return filepath.SkipAll
		}
		return nil
	})
	if walkErr != nil {
		return "", walkErr
	}
	if len(hits) == 0 {
		return "No files matched.", nil
	}
	// Most-recently-modified first (ties broken by path for determinism) — the recent
	// files are usually the ones the agent is working on.
	sort.Slice(hits, func(i, j int) bool {
		if !hits[i].mod.Equal(hits[j].mod) {
			return hits[i].mod.After(hits[j].mod)
		}
		return hits[i].rel < hits[j].rel
	})
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.rel
	}
	res := strings.Join(out, "\n")
	if len(hits) >= fsGlobMaxHits {
		res += fmt.Sprintf("\n\n[stopped at %d matches]", fsGlobMaxHits)
	}
	return res, nil
}

// searchRoot resolves the directory a content/name walk starts from: the given path
// (absolute or relative to the sandbox), or the sandbox root when empty. It errors
// if the sandbox is unconfigured or the resolved path is not a directory.
func (t FSGlobTool) searchRoot(path string) (string, error) {
	return resolveSearchRoot(t.sb, path)
}

// resolveSearchRoot is shared by Glob and Grep: resolve an optional path argument to
// an existing directory, defaulting to the sandbox root.
func resolveSearchRoot(sb Sandbox, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		if !sb.Ready() {
			return "", fmt.Errorf("filesystem sandbox is not configured")
		}
		return sb.Root, nil
	}
	abs, err := sb.Resolve(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", path)
	}
	return abs, nil
}

// relTo returns p relative to root as a slash path ("" for root itself), falling
// back to the cleaned absolute path on error.
func relTo(root, p string) string {
	if rel, err := filepath.Rel(root, p); err == nil {
		if rel == "." {
			return ""
		}
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(p)
}
