package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Transport kinds (mirrors db constants; duplicated to keep mcp dependency-free).
const (
	MCPTransportStdio = "stdio"
	MCPTransportSSE   = "sse"
	MCPTransportHTTP  = "http"
)

// ServerConfig is the transport-agnostic launch/connection spec for one MCP
// server. Callers build it from their own storage (e.g. db.MCPServer).
type ServerConfig struct {
	Name      string
	Transport string // stdio | sse | http
	Command   string
	Args      []string
	URL       string
	Env       map[string]string // extra environment variables (stdio)
	Headers   map[string]string // extra request headers (http, e.g. Authorization)
	// Description is a short, curated one-liner about what this server is for.
	// It rides the per-server line of the load-on-demand catalog so the model
	// keeps a semantic hint (e.g. "use for code search") even when the workspace
	// has too many MCP tools to enumerate individually. Optional.
	Description string
	// ScopeKey isolates the pooled connection to a caller identity (e.g.
	// "<sessionID>|<agentID>") instead of sharing one workspace-wide connection.
	// Empty (the default) keeps the shared, workspace-lifetime connection — the
	// original behaviour. Non-empty makes the pool key a distinct live connection
	// per scope, subject to idle eviction, for servers marked scope="scoped".
	// It is caller identity, not a dial parameter, so it is excluded from the
	// connection fingerprint (see configFingerprint).
	ScopeKey string
}

// envSlice renders Env as KEY=VALUE entries for exec.
func (c ServerConfig) envSlice() []string {
	out := make([]string, 0, len(c.Env))
	for k, v := range c.Env {
		out = append(out, k+"="+v)
	}
	return out
}

// dial opens a client for the configured transport. stdio launches a subprocess;
// http opens a Streamable HTTP connection. The legacy sse transport is not
// implemented (deprecated upstream in favor of Streamable HTTP) and returns a
// clear error so the UI can steer the operator to http.
func (c ServerConfig) dial(ctx context.Context) (Client, error) {
	switch c.Transport {
	case MCPTransportStdio, "":
		if c.Command == "" {
			return nil, fmt.Errorf("mcp %q: stdio transport requires a command", c.Name)
		}
		return DialStdio(ctx, c.Command, c.Args, c.envSlice())
	case MCPTransportHTTP:
		if c.URL == "" {
			return nil, fmt.Errorf("mcp %q: http transport requires a url", c.Name)
		}
		return DialHTTP(ctx, c.URL, c.Headers)
	case MCPTransportSSE:
		return nil, fmt.Errorf("mcp %q: the deprecated sse transport is not supported — use the http (Streamable HTTP) transport", c.Name)
	default:
		return nil, fmt.Errorf("mcp %q: unknown transport %q", c.Name, c.Transport)
	}
}

// Namespaced tool naming: "<server>__<tool>" so tools from different servers
// never collide and the origin is recoverable.
const nsSep = "__"

// NamespaceTool builds the namespaced tool name.
func NamespaceTool(server, tool string) string {
	return sanitize(server) + nsSep + tool
}

// SplitNamespaced recovers (server, tool) from a namespaced name.
func SplitNamespaced(name string) (server, tool string, ok bool) {
	i := strings.Index(name, nsSep)
	if i < 0 {
		return "", "", false
	}
	return name[:i], name[i+len(nsSep):], true
}

// sanitize keeps tool names safe for provider tool schemas (letters/digits/_/-).
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// NormalizeSchema makes an external MCP tool's JSON Schema safe to send to a
// provider (Anthropic in particular). It:
//
//   - strips every "$schema" key (Anthropic rejects it with a 400),
//   - removes the JSON Schema draft "$id"/"$ref"/"definitions"/"$defs" plumbing
//     that the tool-use schema dialect does not accept,
//   - guarantees a root object schema (some servers omit "type"),
//
// while preserving everything else verbatim — notably "additionalProperties",
// "required" and the "oneOf"/"anyOf"/"allOf" union keywords. Invalid or empty
// input falls back to a permissive empty object schema so the tool stays usable.
func NormalizeSchema(raw json.RawMessage) json.RawMessage {
	emptyObject := json.RawMessage(`{"type":"object","properties":{}}`)
	if len(raw) == 0 {
		return emptyObject
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return emptyObject
	}
	m, ok := v.(map[string]any)
	if !ok {
		return emptyObject
	}
	stripSchemaKeys(m)
	// Tool input is always an object; default a missing/typeless root to object.
	if _, has := m["type"]; !has {
		m["type"] = "object"
	}
	if m["type"] == "object" {
		if _, has := m["properties"]; !has {
			m["properties"] = map[string]any{}
		}
	}
	out, err := json.Marshal(m)
	if err != nil {
		return emptyObject
	}
	return out
}

// dropSchemaKeys are JSON Schema meta keys the provider tool-use dialect rejects.
var dropSchemaKeys = []string{"$schema", "$id", "$ref", "$defs", "definitions"}

// stripSchemaKeys recursively removes draft meta keys from a decoded schema.
func stripSchemaKeys(v any) {
	switch t := v.(type) {
	case map[string]any:
		for _, k := range dropSchemaKeys {
			delete(t, k)
		}
		for _, child := range t {
			stripSchemaKeys(child)
		}
	case []any:
		for _, child := range t {
			stripSchemaKeys(child)
		}
	}
}

// CatalogEntry is one tool in the aggregated catalog, tagged with its server.
type CatalogEntry struct {
	Server         string `json:"server"`
	NamespacedName string `json:"name"`
	Tool           Tool   `json:"tool"`
}

// ListServerTools dials a single server, lists its tools, and closes it. Used
// by the "test connection" endpoint and the catalog builder.
func ListServerTools(ctx context.Context, cfg ServerConfig) ([]Tool, error) {
	client, err := cfg.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	return client.ListTools(ctx)
}

// BuildCatalog connects to each server config and returns the union of their
// tools, namespaced. A server that fails to connect is skipped and its error
// recorded in errs (keyed by server name) rather than aborting the whole build.
func BuildCatalog(ctx context.Context, cfgs []ServerConfig) (entries []CatalogEntry, errs map[string]string) {
	errs = map[string]string{}
	for _, cfg := range cfgs {
		tools, err := ListServerTools(ctx, cfg)
		if err != nil {
			errs[cfg.Name] = err.Error()
			continue
		}
		for _, t := range tools {
			entries = append(entries, CatalogEntry{
				Server:         cfg.Name,
				NamespacedName: NamespaceTool(cfg.Name, t.Name),
				Tool:           t,
			})
		}
	}
	return entries, errs
}

// CallNamespaced dials the server owning the namespaced tool and invokes it.
// cfgByServer maps the sanitized server name to its config.
func CallNamespaced(ctx context.Context, cfgByServer map[string]ServerConfig, namespaced string, args json.RawMessage) (CallToolResult, error) {
	server, tool, ok := SplitNamespaced(namespaced)
	if !ok {
		return CallToolResult{}, fmt.Errorf("mcp: %q is not a namespaced tool name", namespaced)
	}
	cfg, ok := cfgByServer[server]
	if !ok {
		return CallToolResult{}, fmt.Errorf("mcp: no server %q for tool %q", server, namespaced)
	}
	client, err := cfg.dial(ctx)
	if err != nil {
		return CallToolResult{}, err
	}
	defer client.Close()
	return client.CallTool(ctx, tool, args)
}
