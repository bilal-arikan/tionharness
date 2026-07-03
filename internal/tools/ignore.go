package tools

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ignoreRule is one compiled .gitignore line, scoped to the directory whose
// .gitignore it came from (base, slash-relative to the walk root; "" = root).
type ignoreRule struct {
	base     string
	re       *regexp.Regexp
	negate   bool // leading "!" — re-includes a previously excluded path
	dirOnly  bool // trailing "/" — matches directories only
	anchored bool // pattern contained a non-trailing "/" — matched against the full sub-path
}

// IgnoreSet is a pragmatic .gitignore matcher: it honours a repo's .gitignore files
// (root + nested, loaded lazily as directories are visited) plus a built-in default
// that always skips the .git metadata directory. It is NOT a byte-perfect
// reimplementation of Git's algorithm — it covers the common cases (directory names,
// globs, anchored paths, negation, dir-only) that keep a Grep/Glob walk out of
// node_modules/.git/dist. A nil set (or enabled=false) ignores nothing.
type IgnoreSet struct {
	root    string
	rules   []ignoreRule
	loaded  map[string]bool // rel dirs whose .gitignore has been loaded
	enabled bool
}

// NewIgnoreSet anchors a matcher at root. When enabled it seeds the always-on .git
// default and lazily loads .gitignore files via LoadDir during the walk.
func NewIgnoreSet(root string, enabled bool) *IgnoreSet {
	s := &IgnoreSet{root: root, loaded: map[string]bool{}, enabled: enabled}
	if enabled {
		if re, err := globToRegexp(".git"); err == nil {
			s.rules = append(s.rules, ignoreRule{re: re})
		}
	}
	return s
}

// LoadDir reads <root>/<relDir>/.gitignore once (relDir "" = the walk root),
// appending its rules scoped to relDir. A no-op when disabled or already loaded, or
// when the directory has no .gitignore.
func (s *IgnoreSet) LoadDir(relDir string) {
	if s == nil || !s.enabled || s.loaded[relDir] {
		return
	}
	s.loaded[relDir] = true
	dir := s.root
	if relDir != "" {
		dir = filepath.Join(s.root, filepath.FromSlash(relDir))
	}
	f, err := os.Open(filepath.Join(dir, ".gitignore"))
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if r, ok := compileIgnoreRule(sc.Text(), relDir); ok {
			s.rules = append(s.rules, r)
		}
	}
}

// compileIgnoreRule parses a single .gitignore line, scoped under base.
func compileIgnoreRule(line, base string) (ignoreRule, bool) {
	line = strings.TrimRight(line, " \r")
	if line == "" || strings.HasPrefix(line, "#") {
		return ignoreRule{}, false
	}
	neg := false
	if strings.HasPrefix(line, "!") {
		neg = true
		line = line[1:]
	}
	dirOnly := strings.HasSuffix(line, "/")
	line = strings.TrimSuffix(line, "/")
	line = strings.TrimSpace(line)
	if line == "" {
		return ignoreRule{}, false
	}
	anchored := strings.HasPrefix(line, "/") || strings.Contains(line, "/")
	line = strings.TrimPrefix(line, "/")
	re, err := globToRegexp(line)
	if err != nil {
		return ignoreRule{}, false
	}
	return ignoreRule{base: base, re: re, negate: neg, dirOnly: dirOnly, anchored: anchored}, true
}

// Ignored reports whether rel (a slash path relative to the walk root) is ignored.
// isDir marks directories so a Grep/Glob walk can SkipDir the whole subtree (which
// is why a pattern that names a directory need only match at that directory's own
// path — its descendants are never visited). Last matching rule wins (negation).
func (s *IgnoreSet) Ignored(rel string, isDir bool) bool {
	if s == nil || !s.enabled || rel == "" || rel == "." {
		return false
	}
	ignored := false
	for _, r := range s.rules {
		sub := rel
		if r.base != "" {
			if rel != r.base && !strings.HasPrefix(rel, r.base+"/") {
				continue // path is outside this rule's directory scope
			}
			sub = strings.TrimPrefix(rel, r.base+"/")
		}
		if r.dirOnly && !isDir {
			continue
		}
		var match bool
		if r.anchored {
			match = r.re.MatchString(sub)
		} else {
			name := sub
			if i := strings.LastIndex(sub, "/"); i >= 0 {
				name = sub[i+1:]
			}
			match = r.re.MatchString(name)
		}
		if match {
			ignored = !r.negate
		}
	}
	return ignored
}
