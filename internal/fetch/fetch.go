// Package fetch is the shared ACQUISITION layer for the importer/ingest pipeline:
// it turns a source (a GitHub repo/plugin URL, an owner/repo shorthand, or a local
// folder) into a flat path->content tree, plus a few low-level GitHub helpers. It
// is deliberately format-agnostic — it knows how to GET bytes, not what they mean
// (that is the ingest adapters' job). Dependency-free (stdlib archive/tar+gzip).
package fetch

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// HTTPTimeout bounds a single archive/file fetch.
const HTTPTimeout = 30 * time.Second

// Size ceilings for an extracted tree. The GitHub tarball route (codeload) is ONE
// request for the whole repo — far friendlier to GitHub's anonymous rate limit
// than walking the contents API folder-by-folder.
const (
	MaxTreeTotal   = 64 << 20 // 64 MB total kept in memory
	MaxFileBytes   = 4 << 20  // 4 MB per extracted file
)

// Tree is a flat map of forward-slashed relative paths to file contents.
type Tree = map[string][]byte

// skipDirs are repo/noise directories never part of an importable artifact. Matched
// as a leading path segment so the in-memory tree stays small and clean.
var skipDirs = map[string]bool{
	".git": true, ".github": true, "node_modules": true, "dist": true,
	"build": true, ".idea": true, ".vscode": true, "benchmarks": true,
}

// TreeFrom acquires a directory tree from a source. source is "github" (a repo/tree
// URL or owner/repo shorthand) or "local" (a directory path). It returns the tree,
// the sub-path prefix the location pointed at (for filtering discovery; "" = root),
// and any non-fatal warnings.
func TreeFrom(source, location string) (tree Tree, prefix string, warnings []string, err error) {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "", "github":
		return treeFromGitHub(location)
	case "local":
		t, w, e := treeFromLocal(location)
		return t, "", w, e
	default:
		return nil, "", nil, fmt.Errorf("unsupported source %q (use github|local)", source)
	}
}

// treeFromGitHub downloads a repo tarball once and returns its tree plus the URL's
// sub-path as the discovery prefix.
func treeFromGitHub(rawURL string) (Tree, string, []string, error) {
	owner, repo, ref, dir, err := ParseGitHubURL(NormalizeRepoRef(rawURL))
	if err != nil {
		return nil, "", nil, err
	}
	tree, warnings, err := fetchGitHubArchive(owner, repo, ref)
	if err != nil {
		return nil, "", nil, err
	}
	return tree, strings.Trim(dir, "/"), warnings, nil
}

// treeFromLocal walks a local directory into a path->content tree.
func treeFromLocal(root string) (Tree, []string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, nil, fmt.Errorf("read %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("%q is not a directory", root)
	}
	tree := Tree{}
	var total int64
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if seg := FirstSegment(rel); seg != "" && skipDirs[seg] {
				return filepath.SkipDir
			}
			return nil
		}
		if total >= MaxTreeTotal {
			return nil
		}
		b, rderr := os.ReadFile(p)
		if rderr != nil || int64(len(b)) > MaxFileBytes {
			return nil
		}
		tree[rel] = b
		total += int64(len(b))
		return nil
	})
	return tree, nil, nil
}

// ParseGitHubURL extracts owner/repo/ref/dir from a github.com tree or blob URL (or
// a bare repo URL). Forms accepted:
//
//	github.com/<owner>/<repo>/tree/<ref>/<path...>   (dir)
//	github.com/<owner>/<repo>/blob/<ref>/<path...>/SKILL.md  (file → parent dir)
//	github.com/<owner>/<repo>                          (repo root, ref defaults to main)
func ParseGitHubURL(raw string) (owner, repo, ref, dir string, err error) {
	u, perr := url.Parse(strings.TrimSpace(raw))
	if perr != nil {
		return "", "", "", "", fmt.Errorf("invalid URL: %w", perr)
	}
	if !strings.EqualFold(u.Host, "github.com") {
		return "", "", "", "", fmt.Errorf("only github.com URLs are supported (got %q)", u.Host)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", "", fmt.Errorf("URL must be github.com/<owner>/<repo>[/tree/<ref>/<path>]")
	}
	owner, repo, ref = parts[0], parts[1], "main"
	if len(parts) >= 4 && (parts[2] == "tree" || parts[2] == "blob") {
		kind := parts[2]
		ref = parts[3]
		dir = strings.Join(parts[4:], "/")
		if kind == "blob" && strings.EqualFold(path.Base(dir), "SKILL.md") {
			dir = path.Dir(dir)
			if dir == "." {
				dir = ""
			}
		}
	}
	return owner, repo, ref, dir, nil
}

// NormalizeRepoRef expands a bare "owner/repo" (or "owner/repo/path") shorthand into
// a full github.com URL so callers accept the casual form users paste.
func NormalizeRepoRef(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.Contains(s, "://") || strings.HasPrefix(s, "github.com/") {
		return s
	}
	if strings.Count(s, "/") >= 1 && !strings.HasPrefix(s, "/") {
		return "https://github.com/" + s
	}
	return s
}

// fetchGitHubArchive downloads and extracts a repo tarball into a tree (paths
// relative to the repo root, top tarball dir stripped). It tries the requested ref,
// then "main"/"master" as fallbacks.
func fetchGitHubArchive(owner, repo, ref string) (Tree, []string, error) {
	var refs []string
	switch strings.TrimSpace(ref) {
	case "", "main", "master", "HEAD":
		refs = []string{"main", "master"}
	default:
		refs = []string{ref}
	}
	var lastErr error
	for _, r := range refs {
		u := fmt.Sprintf("https://codeload.github.com/%s/%s/tar.gz/%s",
			url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(r))
		tree, warns, err := downloadTarball(u)
		if err == nil {
			return tree, warns, nil
		}
		lastErr = err
	}
	return nil, nil, fmt.Errorf("download %s/%s archive: %w", owner, repo, lastErr)
}

// downloadTarball streams a .tar.gz from a GitHub host and extracts it into a tree,
// stripping the single top-level directory the archive wraps everything in. Junk
// dirs are skipped; per-file and total size are capped.
func downloadTarball(u string) (Tree, []string, error) {
	pu, perr := url.Parse(u)
	if perr != nil {
		return nil, nil, perr
	}
	switch strings.ToLower(pu.Host) {
	case "codeload.github.com", "github.com", "api.github.com":
	default:
		return nil, nil, fmt.Errorf("refusing non-GitHub host %q", pu.Host)
	}
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("User-Agent", "TionSwarm-skill-importer")
	resp, err := (&http.Client{Timeout: HTTPTimeout}).Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("GitHub HTTP %d", resp.StatusCode)
	}
	gz, err := gzip.NewReader(io.LimitReader(resp.Body, MaxTreeTotal))
	if err != nil {
		return nil, nil, fmt.Errorf("gunzip archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	tree := Tree{}
	var warnings []string
	var total int64
	for {
		hdr, terr := tr.Next()
		if terr == io.EOF {
			break
		}
		if terr != nil {
			return nil, nil, fmt.Errorf("read archive: %w", terr)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		rel := stripTopDir(filepath.ToSlash(hdr.Name))
		if rel == "" {
			continue
		}
		if seg := FirstSegment(rel); seg != "" && skipDirs[seg] {
			continue
		}
		if hdr.Size > MaxFileBytes {
			continue
		}
		if total >= MaxTreeTotal {
			warnings = append(warnings, "archive exceeded size cap; some files were skipped")
			break
		}
		b, rderr := io.ReadAll(io.LimitReader(tr, MaxFileBytes))
		if rderr != nil {
			continue
		}
		tree[rel] = b
		total += int64(len(b))
	}
	return tree, warnings, nil
}

// --- small path helpers (forward-slash, tree-relative) ---

// stripTopDir removes the single leading "repo-ref/" component GitHub archives wrap
// everything in. A path with no slash (the bare top dir) yields "".
func stripTopDir(p string) string {
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return ""
}

// FirstSegment returns the first path segment of a forward-slashed path.
func FirstSegment(p string) string {
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return p
}
