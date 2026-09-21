package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// MCPResourceEntry is one listed resource, flattened for the model.
type MCPResourceEntry struct {
	Server      string
	URI         string
	Name        string
	Description string
	MimeType    string
	// Template marks a PARAMETERIZED uri (e.g. "db://{table}"): it cannot be read
	// as-is, the placeholders must be filled in first.
	Template bool
}

// MCPServerResources is one server's listing outcome. Every server that was
// asked gets one of these, including the ones that failed — a broken server is
// part of the answer, not a reason to drop the rest (mirrors the per-server
// errs shape the MCP catalog build already uses).
type MCPServerResources struct {
	Server      string
	Entries     []MCPResourceEntry
	Unsupported bool   // the server never advertised capabilities.resources
	Err         string // hard failure for this server (not connected, list failed)
	Note        string // non-fatal problem (e.g. templates could not be listed)
}

// MCPResourceLister lists resources across the enabled MCP servers. An empty
// server name means "every enabled server"; a non-empty one narrows to it. The
// implementation lives in the agent layer (it owns the pool, the DB and the
// caller's scope key); this package only states the contract.
//
// It returns an error ONLY when the request itself cannot be served (no MCP
// configured, unknown server name). A server that fails to connect comes back
// as an outcome with Err set.
type MCPResourceLister func(ctx context.Context, server string) ([]MCPServerResources, error)

// MCPReadResult is the outcome of reading one resource.
type MCPReadResult struct {
	URI      string
	MimeType string
	// Text is the inlined content of a TEXTUAL resource, already truncated to the
	// caller's cap when Truncated is set.
	Text string
	// Truncated reports that Text is a prefix of a longer document. Never a
	// silent trim: the tool says so in its output, with both sizes.
	Truncated bool
	FullBytes int // size of the untruncated content, in bytes
	// Path is set INSTEAD of Text for a binary resource: the bytes were written
	// to a file under the session scratchpad and never enter the model context.
	Path  string
	Bytes int // size of the file at Path
}

// MCPResourceReader reads one resource from one server, writing binary content
// to a file rather than inlining it. Implemented in the agent layer.
type MCPResourceReader func(ctx context.Context, server, uri string) ([]MCPReadResult, error)

// MCPListResourcesTool exposes the MCP resources/list surface — the data half of
// MCP, which TionHarness previously could not see at all. A server whose value
// is in its RESOURCES (docs, schemas, datasets, generated files) rather than its
// tools was invisible to agents here.
//
// Native tool loop only. claude-cli and codex-cli spawn and own their MCP
// clients from their own config, so this pool-backed view would either
// duplicate a connection or describe one they do not use (see
// cliLazyBridgeExcluded).
type MCPListResourcesTool struct {
	list MCPResourceLister
}

// NewMCPListResourcesTool binds the tool to the runtime's lister.
func NewMCPListResourcesTool(list MCPResourceLister) MCPListResourcesTool {
	return MCPListResourcesTool{list: list}
}

func (MCPListResourcesTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "list_mcp_resources",
		Description: "List the RESOURCES (documents, schemas, datasets, generated files) exposed by the " +
			"connected MCP servers — the data half of MCP, separate from their tools. Returns uri, name, " +
			"mimeType and description per entry. Parameterized URI templates are listed too and marked as " +
			"templates: fill in their {placeholders} before reading. Omit `server` to list every enabled " +
			"server; a server with no resource support or no resources is reported as such rather than " +
			"silently omitted. Read one with read_mcp_resource.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "server": {
      "type": "string",
      "description": "Narrow the listing to this MCP server. Omit to list every enabled server."
    }
  },
  "additionalProperties": false
}`),
	}
}

type mcpListResourcesInput struct {
	Server string `json:"server"`
}

func (t MCPListResourcesTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[mcpListResourcesInput]("list_mcp_resources", input)
	if err != nil {
		return "", err
	}
	if t.list == nil {
		return "", fmt.Errorf("list_mcp_resources: no MCP resource lister is wired for this turn")
	}
	results, err := t.list(ctx, strings.TrimSpace(in.Server))
	if err != nil {
		return "", err
	}
	return formatMCPResourceList(results), nil
}

// formatMCPResourceList renders the per-server listings. Every server asked
// about appears, with its resources, its lack of support, or its failure — so a
// gap in the output is never ambiguous.
func formatMCPResourceList(results []MCPServerResources) string {
	if len(results) == 0 {
		return "no MCP servers matched the request; nothing to list"
	}
	var b strings.Builder
	total := 0
	for _, r := range results {
		total += len(r.Entries)
	}
	fmt.Fprintf(&b, "%d resource(s) across %d MCP server(s)\n", total, len(results))
	for _, r := range results {
		switch {
		case r.Err != "":
			fmt.Fprintf(&b, "\n%s: ERROR — %s\n", r.Server, r.Err)
			continue
		case r.Unsupported:
			fmt.Fprintf(&b, "\n%s: no resource support (the server does not advertise capabilities.resources)\n", r.Server)
			continue
		case len(r.Entries) == 0:
			fmt.Fprintf(&b, "\n%s: no resources\n", r.Server)
		default:
			fmt.Fprintf(&b, "\n%s: %d resource(s)\n", r.Server, len(r.Entries))
		}
		if r.Note != "" {
			fmt.Fprintf(&b, "  note: %s\n", r.Note)
		}
		for _, e := range r.Entries {
			kind := ""
			if e.Template {
				kind = " [TEMPLATE — fill in the {placeholders} before reading]"
			}
			fmt.Fprintf(&b, "  - %s%s\n", e.URI, kind)
			var meta []string
			if e.Name != "" {
				meta = append(meta, "name: "+e.Name)
			}
			if e.MimeType != "" {
				meta = append(meta, "mimeType: "+e.MimeType)
			}
			if len(meta) > 0 {
				fmt.Fprintf(&b, "    %s\n", strings.Join(meta, ", "))
			}
			if e.Description != "" {
				fmt.Fprintf(&b, "    %s\n", e.Description)
			}
		}
	}
	return b.String()
}

// MCPReadResourceTool reads one resource by (server, uri).
//
// Binary content is NEVER inlined: it is written to a file under the session
// scratchpad and the tool returns the path, mime type and size. Base64 in the
// context window is pure waste — a 2 MB image is roughly 700k tokens of noise
// the model cannot act on anyway, while a path is something it can hand to Read,
// a shell command or another tool.
type MCPReadResourceTool struct {
	read MCPResourceReader
}

// NewMCPReadResourceTool binds the tool to the runtime's reader.
func NewMCPReadResourceTool(read MCPResourceReader) MCPReadResourceTool {
	return MCPReadResourceTool{read: read}
}

func (MCPReadResourceTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "read_mcp_resource",
		Description: "Read one MCP resource by server + uri (find both with list_mcp_resources). Text content " +
			"is returned inline, capped and explicitly marked when truncated. BINARY content is written to a " +
			"file in this session's scratchpad and reported as a path + mimeType + size — it is never inlined. " +
			"A template uri must have its {placeholders} filled in first.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "server": { "type": "string", "description": "The MCP server that exposes the resource." },
    "uri":    { "type": "string", "description": "The resource uri, exactly as listed (placeholders filled in)." }
  },
  "required": ["server", "uri"],
  "additionalProperties": false
}`),
	}
}

type mcpReadResourceInput struct {
	Server string `json:"server"`
	URI    string `json:"uri"`
}

func (t MCPReadResourceTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[mcpReadResourceInput]("read_mcp_resource", input)
	if err != nil {
		return "", err
	}
	if t.read == nil {
		return "", fmt.Errorf("read_mcp_resource: no MCP resource reader is wired for this turn")
	}
	server := strings.TrimSpace(in.Server)
	uri := strings.TrimSpace(in.URI)
	if server == "" {
		return "", fmt.Errorf("read_mcp_resource: server is required")
	}
	if uri == "" {
		return "", fmt.Errorf("read_mcp_resource: uri is required")
	}
	results, err := t.read(ctx, server, uri)
	if err != nil {
		return "", err
	}
	return formatMCPReadResults(server, uri, results), nil
}

// formatMCPReadResults renders the content blocks of one read. A resource may
// legitimately be several blocks (a directory-like resource), so they are all
// rendered, each labelled with its own uri and mime type.
func formatMCPReadResults(server, uri string, results []MCPReadResult) string {
	if len(results) == 0 {
		return fmt.Sprintf("%s: %s returned no content blocks", server, uri)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s — %d content block(s)\n", server, uri, len(results))
	for i, r := range results {
		blockURI := r.URI
		if blockURI == "" {
			blockURI = uri
		}
		mime := r.MimeType
		if mime == "" {
			mime = "(unspecified)"
		}
		fmt.Fprintf(&b, "\n[block %d] uri: %s, mimeType: %s\n", i+1, blockURI, mime)
		if r.Path != "" {
			fmt.Fprintf(&b, "binary content written to: %s (%d bytes) — read or process it from there; it is not inlined.\n", r.Path, r.Bytes)
			continue
		}
		if r.Truncated {
			fmt.Fprintf(&b, "TRUNCATED: showing the first %d of %d bytes.\n", len(r.Text), r.FullBytes)
		}
		b.WriteString(r.Text)
		if !strings.HasSuffix(r.Text, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}
