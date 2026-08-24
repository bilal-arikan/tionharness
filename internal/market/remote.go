package market

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RegistrySchemaV1 is the schema tag of a remote registry index document.
const RegistrySchemaV1 = "swarmregistry/v1"

// Download size ceilings — a registry index and an individual pack are both small
// JSON documents; these guard against a hostile or misbehaving server.
const (
	maxIndexBytes = 8 << 20 // 8 MiB
	maxPackBytes  = 4 << 20 // 4 MiB
)

// httpTimeout bounds a single registry/pack fetch.
const httpTimeout = 20 * time.Second

// Registry is a configured remote pack source (persisted in registries.json in
// the global market dir). URL points at the registry index (a registry.json).
type Registry struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
	AddedAt int64  `json:"addedAt"`
	// Connector, when set, names a built-in directory-site connector (e.g. "skillsmp")
	// whose refresh queries a site API and transforms its listing into source-ref
	// entries, instead of fetching a swarmregistry/v1 index from URL.
	Connector string `json:"connector,omitempty"`
}

// SourceRef marks a registry entry (or catalog pack) as an INGEST source rather
// than a prebuilt pack: instead of downloading a .harnesspack.json, install runs the
// ingest pipeline against URL (a GitHub repo/tree). This is what bridges directory
// sites (crossaitools/skillsmp/…) — whose listings point at GitHub — into the market.
type SourceRef struct {
	Type string   `json:"type"`           // "github"
	URL  string   `json:"url"`            // owner/repo[/tree/<ref>/<path>]
	Keys []string `json:"keys,omitempty"` // specific artifact keys (empty = all discovered)
}

// RegistryEntry is one pack listed in a registry index: the cheap manifest plus
// EITHER a payload download URL (+optional sha256) OR a Source ref (ingest on install).
type RegistryEntry struct {
	ID            string     `json:"id"`
	Kind          string     `json:"kind"`
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	Version       string     `json:"version,omitempty"`
	Author        string     `json:"author,omitempty"`
	Icon          string     `json:"icon,omitempty"`
	Color         string     `json:"color,omitempty"`
	Tags          []string   `json:"tags,omitempty"`
	URL           string     `json:"url,omitempty"`    // payload (.harnesspack.json) download URL
	SHA256        string     `json:"sha256,omitempty"` // optional integrity hash (hex)
	Source        *SourceRef `json:"source,omitempty"` // ingest source (alternative to URL)
	MinAppVersion string     `json:"minAppVersion,omitempty"`
}

// RegistryIndex is the document a remote registry serves at its URL.
type RegistryIndex struct {
	Schema    string          `json:"schema"`
	Name      string          `json:"name"`
	UpdatedAt int64           `json:"updatedAt,omitempty"`
	Packs     []RegistryEntry `json:"packs"`
}

// fetchIndex downloads and parses a registry index over HTTP(S).
func fetchIndex(ctx context.Context, url string) (RegistryIndex, error) {
	data, err := httpGet(ctx, url, maxIndexBytes)
	if err != nil {
		return RegistryIndex{}, err
	}
	var idx RegistryIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return RegistryIndex{}, fmt.Errorf("invalid registry index JSON: %w", err)
	}
	if idx.Schema != RegistrySchemaV1 {
		return RegistryIndex{}, fmt.Errorf("unsupported registry schema %q (want %q)", idx.Schema, RegistrySchemaV1)
	}
	return idx, nil
}

// fetchPayload downloads a pack's full envelope from url and, when wantSHA is
// non-empty, verifies its sha256 (optional integrity — skipped when absent).
func fetchPayload(ctx context.Context, url, wantSHA string) (Pack, error) {
	data, err := httpGet(ctx, url, maxPackBytes)
	if err != nil {
		return Pack{}, err
	}
	if wantSHA != "" {
		sum := sha256.Sum256(data)
		got := hex.EncodeToString(sum[:])
		if !strings.EqualFold(got, strings.TrimSpace(wantSHA)) {
			return Pack{}, fmt.Errorf("sha256 mismatch (got %s, want %s)", got, wantSHA)
		}
	}
	var p Pack
	if err := json.Unmarshal(data, &p); err != nil {
		return Pack{}, fmt.Errorf("invalid pack JSON: %w", err)
	}
	if p.Schema != SchemaV1 {
		return Pack{}, fmt.Errorf("unsupported pack schema %q (want %q)", p.Schema, SchemaV1)
	}
	if p.ID == "" || p.Kind == "" {
		return Pack{}, fmt.Errorf("pack id and kind are required")
	}
	return p, nil
}

// httpGet performs a bounded GET, returning the body (capped at maxBytes). Only
// http/https schemes are allowed; everything else is rejected up front.
func httpGet(ctx context.Context, url string, maxBytes int64) ([]byte, error) {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("only http(s) URLs are supported: %q", url)
	}
	ctx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry fetch failed: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return nil, err
	}
	return data, nil
}

// entryToPack maps a registry entry to a catalog Pack manifest (no payload),
// stamping the remote source, registry name and the lazy download coordinates.
func entryToPack(e RegistryEntry, registryName string) Pack {
	name := e.Name
	if name == "" {
		name = e.ID
	}
	return Pack{
		Schema:       SchemaV1,
		ID:           e.ID,
		Kind:         e.Kind,
		Name:         name,
		Description:  e.Description,
		Version:      e.Version,
		Author:       e.Author,
		Icon:         e.Icon,
		Color:        e.Color,
		Tags:         e.Tags,
		Source:       SourceRemote,
		RegistryName: registryName,
		SourceRef:    e.Source,
		remoteURL:    e.URL,
		remoteSHA:    e.SHA256,
	}
}

// compareVersions compares two dotted numeric version strings (e.g. "1.2.0").
// Returns -1 if a<b, 0 if equal, 1 if a>b. Non-numeric/missing segments are
// treated as 0; any pre-release suffix after a "-" is ignored. A best-effort
// semver-lite comparison sufficient for "is there a newer pack" checks.
func compareVersions(a, b string) int {
	pa := versionParts(a)
	pb := versionParts(b)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}

// versionParts splits a version into its leading numeric components.
func versionParts(v string) []int {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i] // drop pre-release / build metadata
	}
	if v == "" {
		return nil
	}
	segs := strings.Split(v, ".")
	out := make([]int, 0, len(segs))
	for _, s := range segs {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			n = 0
		}
		out = append(out, n)
	}
	return out
}
