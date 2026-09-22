package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/mcp"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// maxResourceTextBytes caps how much of a TEXTUAL resource is inlined into the
// model context. A resource is an arbitrary document — a whole schema dump, a
// generated report — and MCP puts no bound on its size, so without a cap one
// read could consume a turn's entire window. The cap is generous enough for the
// documents resources are actually used for, and every truncation is REPORTED
// (see MCPReadResult.Truncated), never a silent trim.
const maxResourceTextBytes = 64 * 1024

// resourceFileDir is the sub-directory of the session scratchpad that binary
// resource reads are written into, so they are grouped rather than scattered
// among the agent's own working files.
const resourceFileDir = "mcp-resources"

// newMCPResourceLister builds the list_mcp_resources implementation for ONE
// registry build. scopeKey is the same "<sessionID>|<agentID>" key the catalog
// build computes, so a SCOPED server is read over the caller's own pool slot
// rather than a second connection nobody else dispatches to.
func (r *Runtime) newMCPResourceLister(scopeKey string) tools.MCPResourceLister {
	return func(ctx context.Context, server string) ([]tools.MCPServerResources, error) {
		return r.listMCPResources(ctx, scopeKey, server)
	}
}

// newMCPResourceReader builds the read_mcp_resource implementation for ONE
// registry build, with the same scoping guarantee as the lister.
func (r *Runtime) newMCPResourceReader(scopeKey string) tools.MCPResourceReader {
	return func(ctx context.Context, server, uri string) ([]tools.MCPReadResult, error) {
		return r.readMCPResource(ctx, scopeKey, server, uri)
	}
}

// mcpResourceConfigs resolves the enabled servers (optionally narrowed to one)
// into pool configs, applying the same scoping and scratchpad-root treatment the
// catalog build applies — otherwise the fingerprint would differ and a second
// connection would be dialed for the same server.
//
// An unknown server name is an ERROR, not an empty result: the caller named a
// server that does not exist here, and answering "no resources" would send it
// debugging a server instead of its typo (same rule selectMCPServers follows).
func (r *Runtime) mcpResourceConfigs(ctx context.Context, tool, scopeKey, server string) ([]mcp.ServerConfig, error) {
	if r.mcpPool == nil {
		return nil, fmt.Errorf("%s: MCP is not available in this workspace", tool)
	}
	servers, err := r.db.ListEnabledMCPServers(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: listing enabled MCP servers: %w", tool, err)
	}
	if len(servers) == 0 {
		return nil, fmt.Errorf("%s: no MCP servers are enabled in this workspace", tool)
	}
	selected := servers
	if server != "" {
		selected = nil
		for _, m := range servers {
			if strings.EqualFold(m.Name, server) {
				selected = append(selected, m)
				break
			}
		}
		if len(selected) == 0 {
			return nil, fmt.Errorf("%s: no enabled MCP server named %q (enabled: %s)",
				tool, server, strings.Join(enabledMCPNames(servers), ", "))
		}
	}
	cfgs := make([]mcp.ServerConfig, 0, len(selected))
	for _, m := range selected {
		cfg := toServerConfig(m)
		if m.Scope == "scoped" && scopeKey != "" {
			cfg.ScopeKey = scopeKey
		}
		cfg = r.applyMCPScratchpadRoot(ctx, cfg, m, scopeKey)
		cfgs = append(cfgs, cfg)
	}
	return cfgs, nil
}

// enabledMCPNames returns the sorted names of the enabled servers, for the
// "did you mean" half of an unknown-server error.
func enabledMCPNames(servers []db.MCPServer) []string {
	out := make([]string, 0, len(servers))
	for _, m := range servers {
		out = append(out, m.Name)
	}
	slices.Sort(out)
	return out
}

// listMCPResources lists resources across the selected servers. Failures are
// per-server: an unreachable or resource-less server is described in its own
// outcome and the others are still listed.
func (r *Runtime) listMCPResources(ctx context.Context, scopeKey, server string) ([]tools.MCPServerResources, error) {
	cfgs, err := r.mcpResourceConfigs(ctx, "list_mcp_resources", scopeKey, server)
	if err != nil {
		return nil, err
	}
	results := r.mcpPool.Resources(ctx, cfgs)
	out := make([]tools.MCPServerResources, 0, len(results))
	for _, res := range results {
		entries := make([]tools.MCPResourceEntry, 0, len(res.Resources))
		for _, e := range res.Resources {
			entries = append(entries, tools.MCPResourceEntry{
				Server:      res.Server,
				URI:         e.URI,
				Name:        e.Name,
				Description: e.Description,
				MimeType:    e.MimeType,
				Template:    e.Template,
			})
		}
		if res.Err != "" {
			r.logger.Warn("list_mcp_resources: server failed", "server", res.Server, "error", res.Err)
		}
		out = append(out, tools.MCPServerResources{
			Server:      res.Server,
			Entries:     entries,
			Unsupported: res.Unsupported,
			Err:         res.Err,
			Note:        res.Note,
		})
	}
	return out, nil
}

// readMCPResource reads one resource and shapes its blocks for the model:
// textual content inline (capped, truncation reported), binary content written
// to a file whose path is returned instead.
func (r *Runtime) readMCPResource(ctx context.Context, scopeKey, server, uri string) ([]tools.MCPReadResult, error) {
	cfgs, err := r.mcpResourceConfigs(ctx, "read_mcp_resource", scopeKey, server)
	if err != nil {
		return nil, err
	}
	// mcpResourceConfigs was given a server name, so exactly one config comes back.
	contents, err := r.mcpPool.ReadResource(ctx, cfgs[0], uri)
	if err != nil {
		return nil, err
	}
	out := make([]tools.MCPReadResult, 0, len(contents))
	for i, c := range contents {
		res := tools.MCPReadResult{URI: c.URI, MimeType: c.MimeType}
		if c.IsBlob {
			path, err := r.writeResourceBlob(ctx, server, uri, i, c)
			if err != nil {
				// Do NOT fall back to inlining base64 — that is the exact outcome the
				// file path exists to prevent. The read failed; say so.
				return nil, fmt.Errorf("read_mcp_resource: saving binary content of %s: %w", uri, err)
			}
			res.Path = path
			res.Bytes = len(c.Blob)
			out = append(out, res)
			continue
		}
		res.FullBytes = len(c.Text)
		res.Text = c.Text
		if len(c.Text) > maxResourceTextBytes {
			res.Text = truncateUTF8(c.Text, maxResourceTextBytes)
			res.Truncated = true
		}
		out = append(out, res)
	}
	return out, nil
}

// truncateUTF8 cuts s to at most max bytes without splitting a rune, so the
// inlined prefix is always valid UTF-8.
func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// writeResourceBlob writes one binary content block into the session
// scratchpad and returns its path. Binary bytes must never reach the model
// context as base64, so this is the only path for them.
//
// Without a session there is no scratchpad to write into; that is a hard error
// rather than a fallback to inlining, for the same reason.
func (r *Runtime) writeResourceBlob(ctx context.Context, server, uri string, index int, c mcp.ResourceContent) (string, error) {
	sid := SessionIDFrom(ctx)
	if sid == "" {
		return "", fmt.Errorf("no session is bound to this turn, so there is nowhere to save binary content")
	}
	pad, err := r.sessionScratchpad(sid)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(pad, resourceFileDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, resourceFileName(server, uri, index, c.MimeType))
	if err := os.WriteFile(path, c.Blob, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// resourceFileName builds a filesystem-safe name for a saved blob from the
// server, the resource uri and (when several blocks share a uri) the block
// index. The mime type supplies an extension when the uri has none, so the file
// is openable by its name alone.
func resourceFileName(server, uri string, index int, mimeType string) string {
	base := sanitizeResourcePathPart(server) + "-" + sanitizeResourcePathPart(uri)
	if len(base) > 96 {
		base = base[:96]
	}
	if index > 0 {
		base = fmt.Sprintf("%s-%d", base, index+1)
	}
	if filepath.Ext(base) == "" {
		if ext := resourceExtension(mimeType); ext != "" {
			base += ext
		}
	}
	return base
}

// sanitizeResourcePathPart reduces a server name or uri to characters that are
// safe in a filename on every supported OS (Windows rejects ':' and '?', both
// common in URIs), collapsing runs of replacements.
func sanitizeResourcePathPart(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// resourceExtension maps the mime types a resource realistically arrives as to a
// file extension. An unknown type gets ".bin": a wrong extension would be worse
// than none, because it invites a tool to misparse the file.
func resourceExtension(mimeType string) string {
	mt := strings.ToLower(strings.TrimSpace(strings.SplitN(mimeType, ";", 2)[0]))
	switch mt {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/svg+xml":
		return ".svg"
	case "application/pdf":
		return ".pdf"
	case "application/zip":
		return ".zip"
	case "application/json":
		return ".json"
	case "text/csv":
		return ".csv"
	case "":
		return ".bin"
	}
	return ".bin"
}
