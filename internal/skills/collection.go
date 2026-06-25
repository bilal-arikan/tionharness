package skills

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
	"sort"
	"strings"
)

// A skill COLLECTION is a directory tree (a GitHub repo, a Claude Code plugin, or
// a local folder) that holds MANY skills, each in its own sub-folder with a
// SKILL.md — the layout used by community packs like caveman, taste-skill and the
// marketing-skills repos, and by the catalogs at crossaitools/skillsmp/etc which
// ultimately point at such repos. The single-skill importer (import.go) handles
// one folder; this file discovers and bulk-imports every skill in a tree. (SK-IMP2)

// Archive download/extraction caps. The GitHub tarball route (codeload) is one
// request for the WHOLE repo — far friendlier to GitHub's anonymous rate limit
// than walking the contents API folder-by-folder.
const (
	archiveMaxTotal   = 64 << 20 // 64 MB total kept in memory (compressed stream is smaller)
	archiveMaxPerFile = 4 << 20  // 4 MB per extracted file
)

// skipDirs are top-level/repo noise never part of a skill (keeps the in-memory
// tree small and import clean). Matched as a leading path segment.
var skipDirs = map[string]bool{
	".git": true, ".github": true, "node_modules": true, "dist": true,
	"build": true, ".idea": true, ".vscode": true, "benchmarks": true,
}

// discoveredSkill is one skill found inside a collection tree: its SKILL.md text,
// its bundled files (keyed by path relative to the skill folder), and the folder's
// path relative to the collection root (the stable identity used to select it for
// import).
type discoveredSkill struct {
	relPath string            // skill folder path within the collection ("" = root)
	raw     string            // SKILL.md content
	files   map[string][]byte // bundled resources, relpath -> content
}

// ScannedSkill is the preview metadata for one discovered skill, returned to the
// UI so the user can pick which skills of a collection to import.
type ScannedSkill struct {
	Slug        string   `json:"slug"`        // suggested slug (slugified folder name)
	Name        string   `json:"name"`        // from SKILL.md frontmatter (falls back to slug)
	Description string   `json:"description"` // from SKILL.md frontmatter
	RelPath     string   `json:"relPath"`     // folder path within the collection (selection key)
	Files       []string `json:"files"`       // bundled resource relpaths
	Exists      bool     `json:"exists"`      // a skill with this slug already resolves
}

// ScanResult is the outcome of scanning a collection without importing anything.
type ScanResult struct {
	Source   string         `json:"source"`
	Location string         `json:"location"`
	Skills   []ScannedSkill `json:"skills"`
	Warnings []string       `json:"warnings"`
}

// SkipNote records a skill that was discovered but not imported, with why.
type SkipNote struct {
	Slug    string `json:"slug"`
	RelPath string `json:"relPath"`
	Reason  string `json:"reason"`
}

// CollectionImportResult aggregates a bulk import: what was imported, what was
// skipped (slug collisions, etc.) and any collection-level warnings.
type CollectionImportResult struct {
	Source     string         `json:"source"`
	Location   string         `json:"location"`
	Discovered int            `json:"discovered"`
	Imported   []ImportResult `json:"imported"`
	Skipped    []SkipNote     `json:"skipped"`
	Warnings   []string       `json:"warnings"`
}

// ScanCollection discovers every skill in a collection without writing anything,
// returning preview metadata. source is "github" (a repo/tree URL or owner/repo
// shorthand) or "local" (a directory path).
func (s *Store) ScanCollection(source, location string) (ScanResult, error) {
	found, warnings, err := s.discoverCollection(source, location)
	if err != nil {
		return ScanResult{}, err
	}
	out := ScanResult{Source: source, Location: location, Warnings: warnings}
	for _, d := range found {
		fm, _ := parseFrontmatter(d.raw)
		name := strings.TrimSpace(fm.scalar("name"))
		slug := suggestSlug(d.relPath, name)
		files := make([]string, 0, len(d.files))
		for f := range d.files {
			files = append(files, f)
		}
		sort.Strings(files)
		_, exists := s.Get(slug)
		out.Skills = append(out.Skills, ScannedSkill{
			Slug:        slug,
			Name:        name,
			Description: oneLine(fm.scalar("description")),
			RelPath:     d.relPath,
			Files:       files,
			Exists:      exists,
		})
	}
	sort.Slice(out.Skills, func(i, j int) bool { return out.Skills[i].Slug < out.Skills[j].Slug })
	return out, nil
}

// ImportCollection discovers a collection and imports the selected skills (by
// their relPath; nil/empty selects ALL). slugPrefix, when set, is prepended to
// every derived slug (slugified) so a collection can be namespaced to avoid
// collisions. A slug collision skips that one skill (recorded in Skipped) rather
// than aborting the whole batch. (SK-IMP2)
func (s *Store) ImportCollection(source, location string, selected []string, slugPrefix string, shared bool) (CollectionImportResult, error) {
	found, warnings, err := s.discoverCollection(source, location)
	if err != nil {
		return CollectionImportResult{}, err
	}
	res := CollectionImportResult{
		Source: source, Location: location, Discovered: len(found), Warnings: warnings,
	}
	want := map[string]bool{}
	for _, p := range selected {
		want[strings.Trim(strings.TrimSpace(p), "/")] = true
	}
	prefix := slugify(slugPrefix)
	for _, d := range found {
		if len(want) > 0 && !want[d.relPath] {
			continue
		}
		fm, _ := parseFrontmatter(d.raw)
		slug := suggestSlug(d.relPath, strings.TrimSpace(fm.scalar("name")))
		if prefix != "" {
			slug = prefix + "-" + slug
		}
		sourceURL := collectionItemURL(source, location, d.relPath)
		sk, ir, ierr := s.ImportCCSkill(slug, d.raw, sourceURL, d.files, shared)
		if ierr != nil {
			res.Skipped = append(res.Skipped, SkipNote{Slug: slug, RelPath: d.relPath, Reason: ierr.Error()})
			continue
		}
		_ = sk
		res.Imported = append(res.Imported, ir)
	}
	if len(res.Imported) == 0 && len(res.Skipped) == 0 && len(found) > 0 {
		res.Warnings = append(res.Warnings, "no skills matched the selection")
	}
	return res, nil
}

// discoverCollection routes to the github or local discovery backend and returns
// the discovered skills plus any non-fatal warnings.
func (s *Store) discoverCollection(source, location string) ([]discoveredSkill, []string, error) {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "", "github":
		return discoverGitHubCollection(location)
	case "local":
		return discoverLocalCollection(location)
	default:
		return nil, nil, fmt.Errorf("unsupported source %q (use github|local)", source)
	}
}

// discoverGitHubCollection downloads a repo tarball once (codeload), then finds
// every SKILL.md under the URL's sub-path (empty = whole repo). One HTTP request
// covers discovery AND content for all skills, so it stays well within GitHub's
// anonymous rate limit even for large packs.
func discoverGitHubCollection(rawURL string) ([]discoveredSkill, []string, error) {
	owner, repo, ref, dir, err := parseGitHubURL(normalizeRepoRef(rawURL))
	if err != nil {
		return nil, nil, err
	}
	files, warnings, err := fetchGitHubArchive(owner, repo, ref)
	if err != nil {
		return nil, nil, err
	}
	found := discoverInTree(files, dir)
	if len(found) == 0 {
		return nil, warnings, fmt.Errorf("no SKILL.md found in %s/%s under %q — point at a repo or skills folder", owner, repo, dir)
	}
	return found, warnings, nil
}

// discoverLocalCollection walks a local directory tree, building the same
// path->content map a tarball yields, then discovers skills in it.
func discoverLocalCollection(root string) ([]discoveredSkill, []string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, nil, fmt.Errorf("read %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("%q is not a directory", root)
	}
	files := map[string][]byte{}
	var warnings []string
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
			if seg := firstSegment(rel); seg != "" && skipDirs[seg] {
				return filepath.SkipDir
			}
			return nil
		}
		if total >= archiveMaxTotal {
			return nil
		}
		b, rderr := os.ReadFile(p)
		if rderr != nil || int64(len(b)) > archiveMaxPerFile {
			return nil
		}
		files[rel] = b
		total += int64(len(b))
		return nil
	})
	found := discoverInTree(files, "")
	if len(found) == 0 {
		return nil, warnings, fmt.Errorf("no SKILL.md found under %q (looked recursively)", root)
	}
	return found, warnings, nil
}

// discoverInTree groups a flat path->content map into skills. A skill folder is the
// parent of any SKILL.md (under the optional prefix filter). Each non-SKILL.md file
// is assigned to the DEEPEST skill folder that is its ancestor, so a nested
// sub-skill keeps its own resources and the parent doesn't claim them.
func discoverInTree(files map[string][]byte, prefix string) []discoveredSkill {
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	// Collect skill folders (parents of a SKILL.md) under the prefix.
	skillFolders := map[string]bool{}
	for p := range files {
		if path.Base(p) != "SKILL.md" {
			continue
		}
		folder := dirOf(p)
		if !underPrefix(folder, prefix) {
			continue
		}
		skillFolders[folder] = true
	}
	if len(skillFolders) == 0 {
		return nil
	}
	// Sorted folder list, longest-first, to assign each file to its deepest skill.
	folders := make([]string, 0, len(skillFolders))
	for f := range skillFolders {
		folders = append(folders, f)
	}
	sort.Slice(folders, func(i, j int) bool { return len(folders[i]) > len(folders[j]) })

	byFolder := map[string]*discoveredSkill{}
	for f := range skillFolders {
		raw := string(files[joinPath(f, "SKILL.md")])
		byFolder[f] = &discoveredSkill{relPath: f, raw: raw, files: map[string][]byte{}}
	}
	for p, data := range files {
		if path.Base(p) == "SKILL.md" && skillFolders[dirOf(p)] {
			continue // SKILL.md is rendered separately
		}
		owner, ok := deepestOwner(p, folders)
		if !ok {
			continue // file not inside any skill folder
		}
		rel := strings.TrimPrefix(p, joinPath(owner, ""))
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			continue
		}
		byFolder[owner].files[rel] = data
	}
	out := make([]discoveredSkill, 0, len(byFolder))
	for _, d := range byFolder {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].relPath < out[j].relPath })
	return out
}

// fetchGitHubArchive downloads and extracts a repo tarball into a path->content
// map (paths relative to the repo root, top tarball dir stripped). It tries the
// requested ref, then "main"/"master" as fallbacks.
func fetchGitHubArchive(owner, repo, ref string) (map[string][]byte, []string, error) {
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
		files, warns, err := downloadTarball(u)
		if err == nil {
			return files, warns, nil
		}
		lastErr = err
	}
	return nil, nil, fmt.Errorf("download %s/%s archive: %w", owner, repo, lastErr)
}

// downloadTarball streams a .tar.gz from a GitHub host and extracts it into a
// path->content map, stripping the single top-level directory the GitHub archive
// wraps everything in. Junk dirs are skipped; per-file and total size are capped.
func downloadTarball(u string) (map[string][]byte, []string, error) {
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
	req.Header.Set("User-Agent", "SwarmGo-skill-importer")
	resp, err := (&http.Client{Timeout: githubHTTPTimeout}).Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("GitHub HTTP %d", resp.StatusCode)
	}
	gz, err := gzip.NewReader(io.LimitReader(resp.Body, archiveMaxTotal))
	if err != nil {
		return nil, nil, fmt.Errorf("gunzip archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	files := map[string][]byte{}
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
		if seg := firstSegment(rel); seg != "" && skipDirs[seg] {
			continue
		}
		if hdr.Size > archiveMaxPerFile {
			continue
		}
		if total >= archiveMaxTotal {
			warnings = append(warnings, "archive exceeded size cap; some files were skipped")
			break
		}
		b, rderr := io.ReadAll(io.LimitReader(tr, archiveMaxPerFile))
		if rderr != nil {
			continue
		}
		files[rel] = b
		total += int64(len(b))
	}
	return files, warnings, nil
}

// --- small path helpers (forward-slash, archive-relative) ---

// stripTopDir removes the single leading "repo-ref/" component GitHub archives wrap
// everything in. A path with no slash (the bare top dir) yields "".
func stripTopDir(p string) string {
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return ""
}

// dirOf returns the parent directory of a forward-slashed path, "" for a top-level
// file (unlike path.Dir which returns ".").
func dirOf(p string) string {
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

// firstSegment returns the first path segment of a forward-slashed path.
func firstSegment(p string) string {
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return p
}

// underPrefix reports whether folder is the prefix itself or nested under it. An
// empty prefix matches everything.
func underPrefix(folder, prefix string) bool {
	if prefix == "" {
		return true
	}
	return folder == prefix || strings.HasPrefix(folder, prefix+"/")
}

// deepestOwner returns the deepest folder in folders (pre-sorted longest-first)
// that equals or is an ancestor of file path p, with ok=false when no skill folder
// owns the file. A root-level skill (folder "") claims any file not claimed by a
// deeper folder; it is returned as ("", true).
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
		return "", true // root skill claims any otherwise-unowned file
	}
	return "", false
}

// suggestSlug derives a slug for a discovered skill: the folder's base name,
// falling back to the frontmatter name, then "skill".
func suggestSlug(relPath, name string) string {
	base := path.Base(relPath)
	if relPath == "" {
		base = ""
	}
	slug := slugify(base)
	if slug == "" {
		slug = slugify(name)
	}
	if slug == "" {
		slug = "skill"
	}
	return slug
}

// normalizeRepoRef expands a bare "owner/repo" (or "owner/repo/path") shorthand
// into a full github.com URL so the importer accepts the casual form users paste.
func normalizeRepoRef(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.Contains(s, "://") || strings.HasPrefix(s, "github.com/") {
		return s
	}
	// owner/repo[...] with no scheme and no leading slash.
	if strings.Count(s, "/") >= 1 && !strings.HasPrefix(s, "/") {
		return "https://github.com/" + s
	}
	return s
}

// collectionItemURL builds a provenance URL for one imported skill within a
// collection (best-effort; empty for local sources).
func collectionItemURL(source, location, relPath string) string {
	if strings.EqualFold(source, "local") {
		return ""
	}
	base := strings.TrimRight(normalizeRepoRef(location), "/")
	if relPath == "" {
		return base
	}
	return base + "/" + relPath
}
