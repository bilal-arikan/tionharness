package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
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
	// Dir is the working directory for a stdio subprocess (empty = inherit the
	// host process cwd, the previous behaviour). A file-writing MCP server (notably
	// the Playwright MCP) confines its file access to its workspace roots — and,
	// when the client advertises none as we do, to its cwd — so setting Dir to the
	// caller's session scratchpad is what lets browser_take_screenshot / PDF saves
	// land inside an allowed root. It rides the connection fingerprint (see
	// configFingerprint), so a per-session Dir re-dials rather than reusing a stale
	// root. Ignored by the http/sse transports.
	Dir string
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
		return DialStdio(ctx, c.Command, c.Args, c.envSlice(), c.Dir)
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

// NamespaceTool builds the namespaced tool name. Names already carrying either
// the requested server prefix or the CLI's mcp__ prefix are left unchanged.
func NamespaceTool(server, tool string) string {
	prefix := sanitize(server) + nsSep
	if strings.HasPrefix(tool, prefix) || strings.HasPrefix(tool, "mcp__") {
		return tool
	}
	return prefix + tool
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
		return CallToolResult{}, unknownServerErr(server, namespaced, cfgByServer)
	}
	client, err := cfg.dial(ctx)
	if err != nil {
		return CallToolResult{}, err
	}
	defer client.Close()
	return client.CallTool(ctx, tool, args)
}

// unknownServerErr builds the error for a namespaced call whose server part is
// not in cfgByServer, appending a "did you mean" hint listing the closest known
// server names. The hint goes straight back to the model as tool output, so a
// guessed namespace (e.g. "codebase_memory" for "codebase-memory-mcp") is
// corrected on the next attempt instead of failing opaquely.
func unknownServerErr(server, namespaced string, cfgByServer map[string]ServerConfig) error {
	known := make([]string, 0, len(cfgByServer))
	for name := range cfgByServer {
		known = append(known, name)
	}
	msg := fmt.Sprintf("mcp: no server %q for tool %q", server, namespaced)
	if sugg := SuggestServers(server, known); len(sugg) > 0 {
		msg += "; did you mean " + strings.Join(sugg, ", ") + "?"
	}
	return fmt.Errorf("%s", msg)
}

// SuggestServers returns the known server names closest to want (case-
// insensitive, normalized Levenshtein similarity), best first, capped at 3.
// An empty result means nothing is close enough to suggest — the "no server"
// error stands on its own. Used to turn a guessed/typo'd server namespace into
// an actionable hint instead of a dead end.
func SuggestServers(want string, known []string) []string {
	const (
		minSim  = 0.5 // below this a name is noise, not a typo
		maxHits = 3
	)
	type scored struct {
		name string
		sim  float64
	}
	var cands []scored
	for _, name := range known {
		if sim := nameSimilarity(want, name); sim >= minSim {
			cands = append(cands, scored{name, sim})
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].sim != cands[j].sim {
			return cands[i].sim > cands[j].sim
		}
		return cands[i].name < cands[j].name
	})
	out := make([]string, 0, min(len(cands), maxHits))
	for i, c := range cands {
		if i == maxHits {
			break
		}
		out = append(out, c.name)
	}
	return out
}

// nameSimilarity is a case-insensitive normalized Levenshtein similarity in
// [0,1]: 1.0 is an exact match, 0.0 shares nothing.
func nameSimilarity(a, b string) float64 {
	a, b = strings.ToLower(a), strings.ToLower(b)
	if a == b {
		return 1.0
	}
	if a == "" || b == "" {
		return 0.0
	}
	d := levenshtein(a, b)
	max := len(a)
	if len(b) > max {
		max = len(b)
	}
	return 1.0 - float64(d)/float64(max)
}

// levenshtein computes the edit distance between two strings (two-row DP).
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	prev := make([]int, lb+1)
	cur := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min3(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
