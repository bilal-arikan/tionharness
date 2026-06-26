package market

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Connectors bridge external skill DIRECTORY SITES (skillsmp.com, crossaitools.com)
// into the market. These sites hold THOUSANDS of skills, so connectors are
// SEARCH-DRIVEN, not bulk-cached: the market search box queries them live and shows
// matching results as source-ref entries (install runs the ingest pipeline against
// each result's GitHub source). Connectors never flood the static catalog. (SK-IMP3)

// maxConnectorBytes caps a connector's site response. crossaitools' full listing is
// ~12 MB (it ignores ?q/?limit), fetched once and cached for local search.
const maxConnectorBytes = 24 << 20 // 24 MiB

// crossaitoolsCacheTTL bounds how long the cached crossaitools index is reused before
// a refetch, so local search stays fast without re-downloading 12 MB every keystroke.
const crossaitoolsCacheTTL = 24 * time.Hour

// crossaitoolsCacheFile is the lite index cache filename (in the remote cache dir).
const crossaitoolsCacheFile = "crossaitools-lite.json"

// ConnectorInfo describes a built-in search connector for the UI.
type ConnectorInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	Detail string `json:"detail"`
}

// connectors is the built-in search-connector registry (metadata only; search is
// dispatched by id in SearchConnectors).
var connectors = []ConnectorInfo{
	{ID: "skillsmp", Name: "SkillsMP", URL: "https://skillsmp.com", Detail: "skillsmp.com — canlı arama (GitHub-backed)"},
	{ID: "crossaitools", Name: "CrossAITools", URL: "https://crossaitools.com", Detail: "crossaitools.com — 21k+ skill, aranabilir (GitHub-backed)"},
}

// ListConnectors returns the built-in search connectors for the UI.
func ListConnectors() []ConnectorInfo { return connectors }

// SearchConnectors queries every built-in directory-site connector for a term and
// returns matching skills as source-ref packs (install → ingest the GitHub source).
// Per-connector failures are collected as warnings, never fatal — one site being down
// must not break the other. Results are de-duplicated by GitHub URL.
func (s *Store) SearchConnectors(ctx context.Context, query string, limit int) ([]Pack, []string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 40
	}
	var warnings []string
	type src struct {
		name string
		fn   func(context.Context, string, int) ([]RegistryEntry, error)
	}
	sources := []src{
		{"SkillsMP", searchSkillsMP},
		{"CrossAITools", s.searchCrossAITools},
	}
	seen := map[string]bool{}
	var out []Pack
	for _, sc := range sources {
		entries, err := sc.fn(ctx, query, limit)
		if err != nil {
			warnings = append(warnings, sc.name+": "+err.Error())
			continue
		}
		for _, e := range entries {
			if e.Source == nil || e.Source.URL == "" || seen[e.Source.URL] {
				continue
			}
			seen[e.Source.URL] = true
			out = append(out, entryToPack(e, sc.name))
		}
	}
	return out, warnings, nil
}

// --- skillsmp (live server-side search) ---

// skillsmpItem is the subset of a skillsmp skill entry we consume.
type skillsmpItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Author      string `json:"author"`
	Description string `json:"description"`
	GithubURL   string `json:"githubUrl"`
	Stars       int    `json:"stars"`
}

// searchSkillsMP queries skillsmp.com's search API and maps GitHub-backed hits to
// source-ref entries.
func searchSkillsMP(ctx context.Context, query string, limit int) ([]RegistryEntry, error) {
	u := "https://skillsmp.com/api/v1/skills/search?q=" + url.QueryEscape(query) + "&limit=" + strconv.Itoa(limit)
	data, err := httpGet(ctx, u, maxIndexBytes)
	if err != nil {
		return nil, err
	}
	items := decodeSkillsMPSearch(data)
	return skillsmpEntries(items), nil
}

// decodeSkillsMPSearch parses skillsmp's search response shapes: {data:{skills:[]}},
// {skills:[]}, {data:[]}, or a bare array.
func decodeSkillsMPSearch(data []byte) []skillsmpItem {
	var wrapped struct {
		Data struct {
			Skills []skillsmpItem `json:"skills"`
		} `json:"data"`
		Skills []skillsmpItem `json:"skills"`
	}
	if json.Unmarshal(data, &wrapped) == nil {
		if len(wrapped.Data.Skills) > 0 {
			return wrapped.Data.Skills
		}
		if len(wrapped.Skills) > 0 {
			return wrapped.Skills
		}
	}
	var arr []skillsmpItem
	if json.Unmarshal(data, &arr) == nil {
		return arr
	}
	return nil
}

// skillsmpEntries maps skillsmp items to source-ref entries (skipping urlless ones).
func skillsmpEntries(items []skillsmpItem) []RegistryEntry {
	out := make([]RegistryEntry, 0, len(items))
	for _, it := range items {
		gh := strings.TrimSpace(it.GithubURL)
		if gh == "" || !strings.Contains(strings.ToLower(gh), "github.com") {
			continue
		}
		slug := it.ID
		if slug == "" {
			slug = it.Name
		}
		out = append(out, RegistryEntry{
			ID:          "skill.skillsmp-" + strings.Trim(slugifyID(slug), "-"),
			Kind:        KindSkill,
			Name:        it.Name,
			Description: it.Description,
			Author:      it.Author,
			Source:      &SourceRef{Type: "github", URL: gh},
		})
	}
	return out
}

// --- crossaitools (fetch-once cache + local filter; site ignores ?q) ---

// crossaitoolsItem is the subset of a crossaitools listing we consume.
type crossaitoolsItem struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Repo          string `json:"repo"`
	Path          string `json:"path"`
	Stars         int    `json:"stars"`
	Installs      int    `json:"installs"`
	ListingStatus string `json:"listingStatus"`
}

// searchCrossAITools filters the cached crossaitools index by query (name/description/
// repo/path substring), sorts by popularity, and maps the top matches to source-ref
// entries. The index is fetched once (~12 MB) and cached on disk with a TTL.
func (s *Store) searchCrossAITools(ctx context.Context, query string, limit int) ([]RegistryEntry, error) {
	items, err := s.crossaitoolsIndex(ctx)
	if err != nil {
		return nil, err
	}
	return filterCrossAITools(items, query, limit), nil
}

// filterCrossAITools filters items by query substring (name/description/repo/path),
// sorts by stars desc, caps to limit, and maps to source-ref entries. Pure (no I/O).
func filterCrossAITools(items []crossaitoolsItem, query string, limit int) []RegistryEntry {
	q := strings.ToLower(strings.TrimSpace(query))
	matched := make([]crossaitoolsItem, 0, limit)
	for _, it := range items {
		hay := strings.ToLower(it.Name + " " + it.Description + " " + it.Repo + " " + it.Path)
		if q == "" || strings.Contains(hay, q) {
			matched = append(matched, it)
		}
	}
	sort.SliceStable(matched, func(i, j int) bool { return matched[i].Stars > matched[j].Stars })
	if limit > 0 && len(matched) > limit {
		matched = matched[:limit]
	}
	out := make([]RegistryEntry, 0, len(matched))
	for _, it := range matched {
		slug := it.ID
		if slug == "" {
			slug = it.Repo + "-" + it.Path
		}
		out = append(out, RegistryEntry{
			ID:          "skill.crossaitools-" + strings.Trim(slugifyID(slug), "-"),
			Kind:        KindSkill,
			Name:        it.Name,
			Description: it.Description,
			Source:      &SourceRef{Type: "github", URL: githubTreeURL(it.Repo, it.Path)},
		})
	}
	return out
}

// crossaitoolsIndex returns the cached lite index, refetching the full listing when
// the cache is missing or older than the TTL.
func (s *Store) crossaitoolsIndex(ctx context.Context) ([]crossaitoolsItem, error) {
	cachePath := ""
	if dir := s.remoteCacheDir(); dir != "" {
		cachePath = filepath.Join(dir, crossaitoolsCacheFile)
		if fi, err := os.Stat(cachePath); err == nil && time.Since(fi.ModTime()) < crossaitoolsCacheTTL {
			if data, rErr := os.ReadFile(cachePath); rErr == nil {
				var items []crossaitoolsItem
				if json.Unmarshal(data, &items) == nil && len(items) > 0 {
					return items, nil
				}
			}
		}
	}
	data, err := httpGet(ctx, "https://crossaitools.com/api/skills", maxConnectorBytes)
	if err != nil {
		return nil, err
	}
	raw, err := decodeCrossAITools(data)
	if err != nil {
		return nil, err
	}
	lite := make([]crossaitoolsItem, 0, len(raw))
	for _, it := range raw {
		if strings.TrimSpace(it.Repo) == "" {
			continue
		}
		if it.ListingStatus != "" && !strings.EqualFold(it.ListingStatus, "listed") {
			continue
		}
		lite = append(lite, crossaitoolsItem{
			ID: it.ID, Name: it.Name, Description: it.Description,
			Repo: it.Repo, Path: it.Path, Stars: it.Stars, Installs: it.Installs,
		})
	}
	if cachePath != "" {
		if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err == nil {
			if b, mErr := json.Marshal(lite); mErr == nil {
				_ = os.WriteFile(cachePath, b, 0o644)
			}
		}
	}
	return lite, nil
}

// decodeCrossAITools parses crossaitools' response (bare array or wrapped).
func decodeCrossAITools(data []byte) ([]crossaitoolsItem, error) {
	var arr []crossaitoolsItem
	if json.Unmarshal(data, &arr) == nil && len(arr) > 0 {
		return arr, nil
	}
	var wrap struct {
		Skills []crossaitoolsItem `json:"skills"`
		Data   []crossaitoolsItem `json:"data"`
		Items  []crossaitoolsItem `json:"items"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return nil, fmt.Errorf("invalid crossaitools response: %w", err)
	}
	switch {
	case len(wrap.Skills) > 0:
		return wrap.Skills, nil
	case len(wrap.Data) > 0:
		return wrap.Data, nil
	case len(wrap.Items) > 0:
		return wrap.Items, nil
	}
	return arr, nil
}

// githubTreeURL builds a github.com tree URL from "owner/repo" + a folder path
// ("" = repo root). Ref defaults to main; the ingest fetch falls back to master.
func githubTreeURL(repo, path string) string {
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return "https://github.com/" + repo
	}
	return "https://github.com/" + repo + "/tree/main/" + path
}

// slugifyID lower-cases an id and keeps only [a-z0-9-], collapsing other runs to "-".
func slugifyID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if b.Len() > 0 && !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
