package market

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Connectors bridge external skill DIRECTORY SITES (whose listings point at GitHub)
// into the market: a connector queries the site's API and transforms each listing
// into a source-ref RegistryEntry, so install runs the ingest pipeline against the
// GitHub source. They reuse the remote-registry plumbing (cache/ledger/catalog) —
// a connector registry just refreshes via the site API instead of a swarmregistry
// index. (SK-IMP3, _Docs/38)

// ConnectorInfo describes a built-in connector for the UI's quick-add list.
type ConnectorInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	Detail string `json:"detail"`
}

// connectorDef is a built-in connector: its identity, canonical API URL, and a
// fetch func turning the site response into source-ref entries.
type connectorDef struct {
	info  ConnectorInfo
	fetch func(ctx context.Context, apiURL string) ([]RegistryEntry, error)
}

// connectors is the built-in connector registry.
var connectors = map[string]connectorDef{
	"skillsmp": {
		info: ConnectorInfo{
			ID: "skillsmp", Name: "SkillsMP", URL: "https://skillsmp.com/api/skills",
			Detail: "skillsmp.com skill marketplace (GitHub-backed)",
		},
		fetch: fetchSkillsMP,
	},
}

// ListConnectors returns the built-in connectors for the UI.
func ListConnectors() []ConnectorInfo {
	out := make([]ConnectorInfo, 0, len(connectors))
	for _, c := range connectors {
		out = append(out, c.info)
	}
	return out
}

// AddConnector registers a built-in connector as an enabled registry (idempotent by
// id), then returns the registry list. RefreshRemote will populate it via the site API.
func (s *Store) AddConnector(id string) ([]Registry, error) {
	def, ok := connectors[id]
	if !ok {
		return nil, fmt.Errorf("unknown connector %q", id)
	}
	regs := s.loadRegistries()
	for i := range regs {
		if regs[i].Connector == id {
			regs[i].Enabled = true
			if err := s.saveRegistries(regs); err != nil {
				return nil, err
			}
			return s.ListRegistries(), nil
		}
	}
	regs = append(regs, Registry{
		Name: def.info.Name, URL: def.info.URL, Connector: id, Enabled: true,
	})
	if err := s.saveRegistries(regs); err != nil {
		return nil, err
	}
	return s.ListRegistries(), nil
}

// fetchConnectorIndex runs a connector's fetch and wraps the entries in a
// RegistryIndex (so the existing cache path stores it like any other registry).
func fetchConnectorIndex(ctx context.Context, r Registry) (RegistryIndex, error) {
	def, ok := connectors[r.Connector]
	if !ok {
		return RegistryIndex{}, fmt.Errorf("unknown connector %q", r.Connector)
	}
	entries, err := def.fetch(ctx, r.URL)
	if err != nil {
		return RegistryIndex{}, err
	}
	return RegistryIndex{Schema: RegistrySchemaV1, Name: r.Name, Packs: entries}, nil
}

// --- skillsmp ---

// skillsmpItem is the subset of skillsmp.com's /api/skills entry we consume.
type skillsmpItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Author      string `json:"author"`
	Description string `json:"description"`
	GithubURL   string `json:"githubUrl"`
	Stars       int    `json:"stars"`
}

// fetchSkillsMP queries skillsmp.com's listing API and maps each GitHub-backed skill
// to a source-ref entry (install → ingest the githubUrl). Entries without a usable
// GitHub URL are skipped.
func fetchSkillsMP(ctx context.Context, apiURL string) ([]RegistryEntry, error) {
	data, err := httpGet(ctx, apiURL, maxIndexBytes)
	if err != nil {
		return nil, err
	}
	items, err := decodeSkillsMP(data)
	if err != nil {
		return nil, err
	}
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
			ID:          "skill.skillsmp-" + slug,
			Kind:        KindSkill,
			Name:        it.Name,
			Description: it.Description,
			Author:      it.Author,
			Source:      &SourceRef{Type: "github", URL: gh},
		})
	}
	return out, nil
}

// decodeSkillsMP parses skillsmp's response, tolerating either a bare array or an
// object wrapping the list under skills/data/items.
func decodeSkillsMP(data []byte) ([]skillsmpItem, error) {
	var arr []skillsmpItem
	if json.Unmarshal(data, &arr) == nil && len(arr) > 0 {
		return arr, nil
	}
	var wrap struct {
		Skills []skillsmpItem `json:"skills"`
		Data   []skillsmpItem `json:"data"`
		Items  []skillsmpItem `json:"items"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return nil, fmt.Errorf("invalid skillsmp response: %w", err)
	}
	switch {
	case len(wrap.Skills) > 0:
		return wrap.Skills, nil
	case len(wrap.Data) > 0:
		return wrap.Data, nil
	case len(wrap.Items) > 0:
		return wrap.Items, nil
	}
	return arr, nil // possibly empty bare array
}
