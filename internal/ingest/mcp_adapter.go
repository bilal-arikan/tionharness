package ingest

import (
	"encoding/json"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/fetch"
	"github.com/bilal-arikan/swarmgo/internal/market"
	"github.com/bilal-arikan/swarmgo/internal/skills"
)

// mcpAdapter detects Model Context Protocol server configs (.mcp.json / mcp.json with
// an mcpServers map) and converts each server entry into an MCP SwarmPack. Secrets in
// env are the publisher's responsibility to omit (carried verbatim).
type mcpAdapter struct{}

func (mcpAdapter) Kind() string { return market.KindMCP }

// mcpServerSpec is the standard CC/.mcp.json server entry shape.
type mcpServerSpec struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	URL     string            `json:"url"`
	Type    string            `json:"type"` // stdio | sse | http
}

func (mcpAdapter) Scan(tree fetch.Tree, prefix, baseURL string) []Discovered {
	files := fetch.FindFiles(tree, prefix, func(n string) bool {
		ln := strings.ToLower(n)
		return ln == ".mcp.json" || ln == "mcp.json"
	})
	var out []Discovered
	for _, p := range files {
		var doc struct {
			MCPServers map[string]mcpServerSpec `json:"mcpServers"`
		}
		if err := json.Unmarshal(tree[p], &doc); err != nil {
			continue
		}
		for name, spec := range doc.MCPServers {
			name, spec := name, spec
			transport := spec.Type
			if transport == "" {
				if spec.Command != "" {
					transport = "stdio"
				} else if spec.URL != "" {
					transport = "http"
				}
			}
			argsJSON := ""
			if len(spec.Args) > 0 {
				if b, err := json.Marshal(spec.Args); err == nil {
					argsJSON = string(b)
				}
			}
			envJSON := ""
			if len(spec.Env) > 0 {
				if b, err := json.Marshal(spec.Env); err == nil {
					envJSON = string(b)
				}
			}
			desc := transport + " MCP server"
			if spec.Command != "" {
				desc = spec.Command + " " + strings.Join(spec.Args, " ")
			} else if spec.URL != "" {
				desc = spec.URL
			}
			relPath := p + "#" + name
			out = append(out, Discovered{
				Key:         market.KindMCP + ":" + relPath,
				Kind:        market.KindMCP,
				Slug:        skills.Slugify(name),
				Name:        name,
				Description: strings.TrimSpace(desc),
				RelPath:     relPath,
				build: func(opts Options) (market.Pack, error) {
					return market.Pack{
						Schema:      market.SchemaV1,
						ID:          market.KindMCP + "." + skills.Slugify(name),
						Kind:        market.KindMCP,
						Name:        name,
						Description: strings.TrimSpace(desc),
						Version:     "1.0.0",
						Payload: market.Payload{MCP: &market.MCPPayload{
							Name:      name,
							Transport: transport,
							Command:   spec.Command,
							Args:      argsJSON,
							URL:       spec.URL,
							EnvConfig: envJSON,
						}},
					}, nil
				},
			})
		}
	}
	return out
}
