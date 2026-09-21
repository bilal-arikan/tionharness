package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// resourceHTTPServer is a Streamable HTTP MCP server with a configurable
// RESOURCE surface, so the capability gate, the template merge and the
// independent-failure rules can be driven end to end over a real transport.
type resourceHTTPServer struct {
	advertise    bool // include capabilities.resources on initialize
	resources    []map[string]any
	templates    []map[string]any
	templateFail bool // resources/templates/list answers with a JSON-RPC error
	listFail     bool // resources/list answers with a JSON-RPC error
	listGarbage  bool // resources/list answers with a well-formed but wrong-shaped result
	contents     []map[string]any
	called       []string // every method the client invoked, in order
}

func (f *resourceHTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID     *int            `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	f.called = append(f.called, req.Method)
	if req.ID == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	writeResult := func(result any) {
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": result})
	}
	writeErr := func(msg string) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": *req.ID,
			"error": map[string]any{"code": -32603, "message": msg},
		})
	}

	switch req.Method {
	case "initialize":
		caps := map[string]any{"tools": map[string]any{}}
		if f.advertise {
			caps["resources"] = map[string]any{"listChanged": true}
		}
		writeResult(map[string]any{"protocolVersion": protocolVersion, "capabilities": caps})
	case "tools/list":
		writeResult(map[string]any{"tools": []map[string]any{}})
	case "resources/list":
		switch {
		case f.listFail:
			writeErr("resources exploded")
		case f.listGarbage:
			// Well-formed JSON-RPC carrying a payload that is NOT a resource list.
			writeResult(map[string]any{"resources": "not-an-array"})
		default:
			writeResult(map[string]any{"resources": f.resources})
		}
	case "resources/templates/list":
		if f.templateFail {
			writeErr("templates exploded")
			return
		}
		writeResult(map[string]any{"resourceTemplates": f.templates})
	case "resources/read":
		writeResult(map[string]any{"contents": f.contents})
	default:
		writeErr("unknown method " + req.Method)
	}
}

func dialResourceServer(t *testing.T, f *resourceHTTPServer) Client {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := DialHTTP(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("DialHTTP: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// A server that never advertises capabilities.resources must not be ASKED. The
// point is not just the error value: a JSON-RPC "method not found" reaching the
// model reads like a bug in TionHarness rather than an absent optional feature.
func TestResourcesUnsupportedServerIsNeverQueried(t *testing.T) {
	f := &resourceHTTPServer{advertise: false}
	c := dialResourceServer(t, f)

	if c.SupportsResources() {
		t.Fatal("SupportsResources = true for a server that advertised none")
	}
	if _, err := c.ListResources(context.Background()); err != ErrResourcesUnsupported {
		t.Fatalf("ListResources error = %v, want ErrResourcesUnsupported", err)
	}
	if _, err := c.ReadResource(context.Background(), "x://y"); err != ErrResourcesUnsupported {
		t.Fatalf("ReadResource error = %v, want ErrResourcesUnsupported", err)
	}
	for _, m := range f.called {
		if strings.HasPrefix(m, "resources/") {
			t.Fatalf("client called %q on a server with no resource capability", m)
		}
	}
}

// Concrete resources and templates come back as ONE list, with templates
// flagged. The uriTemplate field is the spec's name for a template's uri and
// must land on Resource.URI.
func TestListResourcesMergesTemplates(t *testing.T) {
	f := &resourceHTTPServer{
		advertise: true,
		resources: []map[string]any{
			{"uri": "file:///readme.md", "name": "readme", "mimeType": "text/markdown", "description": "the readme"},
		},
		templates: []map[string]any{
			{"uriTemplate": "db://{table}", "name": "table", "mimeType": "application/json"},
		},
	}
	c := dialResourceServer(t, f)

	list, err := c.ListResources(context.Background())
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(list), list)
	}
	if list[0].Template || list[0].URI != "file:///readme.md" || list[0].Description != "the readme" {
		t.Errorf("concrete entry is wrong: %+v", list[0])
	}
	if !list[1].Template || list[1].URI != "db://{table}" {
		t.Errorf("template entry is wrong: %+v", list[1])
	}
}

// A templates/list failure must NOT take the concrete resources down with it:
// they are independent calls, and the entries we DID get are still the answer.
func TestListResourcesSurvivesTemplateFailure(t *testing.T) {
	f := &resourceHTTPServer{
		advertise:    true,
		resources:    []map[string]any{{"uri": "file:///a.txt"}},
		templateFail: true,
	}
	c := dialResourceServer(t, f)

	list, err := c.ListResources(context.Background())
	if err == nil {
		t.Fatal("template failure was swallowed; want it reported alongside the entries")
	}
	if !strings.Contains(err.Error(), "templates") {
		t.Errorf("error does not name the failing call: %v", err)
	}
	if len(list) != 1 || list[0].URI != "file:///a.txt" {
		t.Fatalf("concrete resources lost to a template failure: %+v", list)
	}
}

// A malformed resources/list payload is a VISIBLE error. Returning an empty
// list would make a broken server look like an empty one — the exact silent
// failure this repo forbids.
func TestListResourcesMalformedPayloadIsAnError(t *testing.T) {
	f := &resourceHTTPServer{advertise: true, listGarbage: true}
	c := dialResourceServer(t, f)

	list, err := c.ListResources(context.Background())
	if err == nil {
		t.Fatalf("malformed resource list decoded silently as %+v", list)
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error does not identify a decode failure: %v", err)
	}
}

// A resources/list failure IS fatal for that server — unlike a templates
// failure, there is nothing left to report — and the server's own message must
// survive into the error.
func TestListResourcesListFailureIsFatal(t *testing.T) {
	f := &resourceHTTPServer{advertise: true, listFail: true}
	c := dialResourceServer(t, f)

	list, err := c.ListResources(context.Background())
	if err == nil {
		t.Fatalf("resources/list failure was swallowed, got %+v", list)
	}
	if !strings.Contains(err.Error(), "resources exploded") {
		t.Errorf("the server's own reason is lost: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("entries returned despite a failed list: %+v", list)
	}
}

// A server with resource support but nothing to offer answers an EMPTY LIST,
// not an error: "none" is a valid, useful answer.
func TestListResourcesEmptyIsNotAnError(t *testing.T) {
	c := dialResourceServer(t, &resourceHTTPServer{advertise: true})

	list, err := c.ListResources(context.Background())
	if err != nil {
		t.Fatalf("empty resource set reported as an error: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("want no entries, got %+v", list)
	}
}

// Text and blob blocks must be told apart, and a blob must arrive as DECODED
// bytes — base64 in the model context is the thing the file path exists to
// avoid.
func TestReadResourceDecodesTextAndBlob(t *testing.T) {
	raw := []byte{0x89, 'P', 'N', 'G', 0x00, 0xFF}
	f := &resourceHTTPServer{
		advertise: true,
		contents: []map[string]any{
			{"uri": "file:///a.txt", "mimeType": "text/plain", "text": "hello"},
			{"uri": "file:///b.png", "mimeType": "image/png", "blob": base64.StdEncoding.EncodeToString(raw)},
		},
	}
	c := dialResourceServer(t, f)

	got, err := c.ReadResource(context.Background(), "file:///a.txt")
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d blocks, want 2", len(got))
	}
	if got[0].IsBlob || got[0].Text != "hello" {
		t.Errorf("text block is wrong: %+v", got[0])
	}
	if !got[1].IsBlob || string(got[1].Blob) != string(raw) {
		t.Errorf("blob block is wrong: %+v", got[1])
	}
}

// An empty-string text resource must stay TEXT, not be mistaken for a blob:
// IsBlob is what keeps a legitimately empty document distinguishable from
// binary content.
func TestReadResourceEmptyTextIsNotABlob(t *testing.T) {
	f := &resourceHTTPServer{
		advertise: true,
		contents:  []map[string]any{{"uri": "file:///empty", "mimeType": "text/plain", "text": ""}},
	}
	c := dialResourceServer(t, f)

	got, err := c.ReadResource(context.Background(), "file:///empty")
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(got) != 1 || got[0].IsBlob || got[0].Text != "" {
		t.Fatalf("empty text block mis-decoded: %+v", got)
	}
}

// Pool.Resources reports PER SERVER: one broken server must not erase the
// listing of the others, and a server with no resource support is described as
// such rather than as an empty or failed one.
func TestPoolResourcesPerServerOutcomes(t *testing.T) {
	good := &fakeClient{
		resourcesOK: true,
		resources:   []Resource{{URI: "file:///ok.md", Name: "ok"}},
	}
	plain := &fakeClient{} // advertises no resource support
	p := NewPool()
	defer p.Close()

	cfgs := []ServerConfig{
		{Name: "good", Command: "x", stdioDial: func(context.Context, string, []string, []string, string) (Client, error) {
			return good, nil
		}},
		{Name: "plain", Command: "x", stdioDial: func(context.Context, string, []string, []string, string) (Client, error) {
			return plain, nil
		}},
		{Name: "dead", Command: "x", stdioDial: func(context.Context, string, []string, []string, string) (Client, error) {
			return nil, errors.New("connection refused")
		}},
	}
	results := p.Resources(context.Background(), cfgs)
	if len(results) != 3 {
		t.Fatalf("got %d outcomes, want one per server", len(results))
	}
	if results[0].Err != "" || len(results[0].Resources) != 1 {
		t.Errorf("healthy server: %+v", results[0])
	}
	if !results[1].Unsupported || results[1].Err != "" {
		t.Errorf("unsupported server must be flagged, not failed: %+v", results[1])
	}
	if results[2].Err == "" {
		t.Errorf("dead server must report its failure: %+v", results[2])
	}
	// The decisive property: the dead and unsupported servers did not cost us the
	// healthy one's listing.
	if results[0].Resources[0].URI != "file:///ok.md" {
		t.Errorf("healthy server's resources lost: %+v", results[0])
	}
}

// Pool.ReadResource names a single server and uri, so a failure IS the answer
// and must surface as an error rather than an empty success.
func TestPoolReadResourceUnknownURIIsAnError(t *testing.T) {
	c := &fakeClient{resourcesOK: true}
	p := NewPool()
	defer p.Close()
	cfg := ServerConfig{Name: "s", Command: "x", stdioDial: func(context.Context, string, []string, []string, string) (Client, error) {
		return c, nil
	}}

	if _, err := p.ReadResource(context.Background(), cfg, "file:///missing"); err == nil {
		t.Fatal("unknown uri returned no error")
	}
}

// Reading from a server with no resource support must say exactly that.
func TestPoolReadResourceUnsupportedServer(t *testing.T) {
	p := NewPool()
	defer p.Close()
	cfg := ServerConfig{Name: "s", Command: "x", stdioDial: func(context.Context, string, []string, []string, string) (Client, error) {
		return &fakeClient{}, nil
	}}

	_, err := p.ReadResource(context.Background(), cfg, "file:///x")
	if err == nil || !strings.Contains(err.Error(), "resource support") {
		t.Fatalf("error = %v, want one naming the missing resource support", err)
	}
}
