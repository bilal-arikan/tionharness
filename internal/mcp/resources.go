package mcp

// MCP resources: the server-side data surface that sits beside tools.
//
// A resource is addressable content the server is willing to hand over — a doc,
// a schema, a dataset, a generated file — identified by a URI. Three methods
// make up the surface TionHarness speaks:
//
//   - resources/list           : concrete resources, each with a fixed URI
//   - resources/templates/list : PARAMETERIZED uris (RFC 6570), e.g. "db://{table}"
//   - resources/read           : the content behind one URI
//
// Unlike tools, resources are OPTIONAL: a server advertises support through
// capabilities.resources on the initialize result. A server that does not
// advertise it is not asked (see SupportsResources) — probing it anyway would
// return a "method not found" JSON-RPC error that says nothing useful.

import (
	"context"
	"encoding/json"
	"fmt"
)

// Resource is one concrete resource advertised by a server. URI is the only
// field the spec requires; the rest are best-effort metadata the server may
// omit.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType"`
	// Template is true when this entry came from resources/templates/list, i.e.
	// URI is a URI TEMPLATE with {placeholders} that must be filled in before it
	// can be read. Kept on the same struct (rather than a parallel type) so one
	// list can carry both kinds while still telling them apart.
	Template bool `json:"template"`
}

// ResourceContent is one block returned by resources/read. Exactly one of Text
// or Blob carries the payload: Text for a textual resource, Blob for a binary
// one (base64 in the wire format, decoded bytes here).
//
// Blob is decoded eagerly rather than left as base64 on purpose: the caller
// writes it to a file, and base64 must NEVER reach the model context (a 2 MB
// image would be ~700k tokens of noise).
type ResourceContent struct {
	URI      string
	MimeType string
	Text     string
	Blob     []byte
	// IsBlob distinguishes a binary resource from a text one whose content is
	// legitimately empty. Without it a zero-byte blob and an empty string would
	// be indistinguishable, and the caller would inline "" as if it were the
	// document.
	IsBlob bool
}

// ServerCapabilities is the subset of the initialize result TionHarness acts on.
// The full object is larger; everything not modelled here is ignored rather
// than rejected, so a newer server stays usable.
//
// The pointer fields are the spec's own shape: capability presence is signalled
// by the KEY existing, with an object (possibly empty) as its value. A nil
// pointer therefore means "not advertised", which is exactly the distinction
// SupportsResources needs — an empty struct would conflate it with "advertised
// with no sub-options".
type ServerCapabilities struct {
	Tools     *struct{} `json:"tools"`
	Resources *struct {
		Subscribe   bool `json:"subscribe"`
		ListChanged bool `json:"listChanged"`
	} `json:"resources"`
	Prompts *struct{} `json:"prompts"`
}

// parseInitializeResult decodes the capabilities block of an initialize result.
// A result we cannot decode is an error: the handshake is the one message whose
// shape we must trust, and silently treating a malformed one as "no
// capabilities" would make every resource call disappear with no explanation.
func parseInitializeResult(raw json.RawMessage) (ServerCapabilities, error) {
	if len(raw) == 0 {
		return ServerCapabilities{}, nil
	}
	var out struct {
		Capabilities ServerCapabilities `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return ServerCapabilities{}, fmt.Errorf("mcp initialize: decode capabilities: %w", err)
	}
	return out.Capabilities, nil
}

// initializeParams is the handshake payload. Resources need no client-side
// capability declaration (only server→client features such as roots and
// sampling do), so this still advertises tools.listChanged alone.
func initializeParams() map[string]any {
	return map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": true}},
		"clientInfo":      map[string]string{"name": clientName, "version": clientVersion},
	}
}

// parseResourcesList decodes a resources/list result. A malformed payload is an
// ERROR, never an empty list: "this server has no resources" and "this server
// answered with something we cannot read" are different facts, and collapsing
// them would hide a broken server behind a plausible-looking empty result.
func parseResourcesList(raw json.RawMessage) ([]Resource, error) {
	var out struct {
		Resources []Resource `json:"resources"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("mcp resources/list decode: %w", err)
	}
	return out.Resources, nil
}

// parseResourceTemplatesList decodes a resources/templates/list result. The
// spec names the URI field "uriTemplate" here (not "uri"), so it is mapped onto
// Resource.URI and the entry is flagged as a template.
func parseResourceTemplatesList(raw json.RawMessage) ([]Resource, error) {
	var out struct {
		ResourceTemplates []struct {
			URITemplate string `json:"uriTemplate"`
			Name        string `json:"name"`
			Description string `json:"description"`
			MimeType    string `json:"mimeType"`
		} `json:"resourceTemplates"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("mcp resources/templates/list decode: %w", err)
	}
	list := make([]Resource, 0, len(out.ResourceTemplates))
	for _, t := range out.ResourceTemplates {
		list = append(list, Resource{
			URI:         t.URITemplate,
			Name:        t.Name,
			Description: t.Description,
			MimeType:    t.MimeType,
			Template:    true,
		})
	}
	return list, nil
}

// parseReadResource decodes a resources/read result into its content blocks.
// The wire format carries base64 in "blob"; encoding/json decodes a []byte
// field from base64 automatically, so a malformed blob fails here instead of
// reaching the caller as garbage.
func parseReadResource(raw json.RawMessage) ([]ResourceContent, error) {
	var out struct {
		Contents []struct {
			URI      string  `json:"uri"`
			MimeType string  `json:"mimeType"`
			Text     *string `json:"text"`
			Blob     []byte  `json:"blob"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("mcp resources/read decode: %w", err)
	}
	contents := make([]ResourceContent, 0, len(out.Contents))
	for _, c := range out.Contents {
		rc := ResourceContent{URI: c.URI, MimeType: c.MimeType}
		switch {
		case c.Text != nil:
			rc.Text = *c.Text
		case c.Blob != nil:
			rc.Blob = c.Blob
			rc.IsBlob = true
		default:
			// Neither field present: the server returned a content block with no
			// payload. Surface it as a zero-length TEXT block rather than guessing;
			// the caller reports an empty read, which is the honest description.
		}
		contents = append(contents, rc)
	}
	return contents, nil
}

// rpcCall is the shape both transports expose for a request/response round
// trip, so the resource logic below is written once and shared.
type rpcCall func(ctx context.Context, method string, params any) (json.RawMessage, error)

// listResources performs resources/list and resources/templates/list and
// returns the union, concrete entries first.
//
// The two calls are INDEPENDENT on purpose. Templates are the rarer, less
// uniformly implemented half of the surface: a server that lists resources fine
// but errors on resources/templates/list must still yield its concrete
// resources. So a template failure is attached to the returned error while the
// concrete list is returned alongside it, and the caller decides (our callers
// report the note and keep the entries). A resources/list failure, by contrast,
// is fatal for that server — there is nothing left to report.
func listResources(ctx context.Context, call rpcCall) ([]Resource, error) {
	raw, err := call(ctx, "resources/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	list, err := parseResourcesList(raw)
	if err != nil {
		return nil, err
	}
	traw, terr := call(ctx, "resources/templates/list", map[string]any{})
	if terr != nil {
		// Concrete resources survive a template failure; the reason rides along so
		// the caller can say WHY templates are missing instead of implying there
		// are none.
		return list, fmt.Errorf("resources/templates/list: %w", terr)
	}
	templates, terr := parseResourceTemplatesList(traw)
	if terr != nil {
		return list, terr
	}
	return append(list, templates...), nil
}

// readResource performs resources/read for one URI.
func readResource(ctx context.Context, call rpcCall, uri string) ([]ResourceContent, error) {
	raw, err := call(ctx, "resources/read", map[string]any{"uri": uri})
	if err != nil {
		return nil, err
	}
	return parseReadResource(raw)
}

// ErrResourcesUnsupported is returned by the resource calls when the server did
// not advertise capabilities.resources on initialize. It is a sentinel so
// callers can report "this server has no resource support" instead of relaying
// a JSON-RPC -32601 that reads like a bug.
var ErrResourcesUnsupported = fmt.Errorf("mcp: server does not advertise resource support")
