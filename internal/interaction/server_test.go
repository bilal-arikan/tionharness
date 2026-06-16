package interaction

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeBackend is a minimal Backend for protocol tests.
type fakeBackend struct {
	validToken string
	lastName   string
	lastArgs   json.RawMessage
}

func (f *fakeBackend) Valid(token string) bool { return token == f.validToken }
func (f *fakeBackend) Tools() []ToolSpec {
	return []ToolSpec{{Name: "ask_user", Description: "ask", InputSchema: json.RawMessage(`{"type":"object"}`)}}
}
func (f *fakeBackend) Call(_ context.Context, _, name string, args json.RawMessage) (CallResult, error) {
	f.lastName = name
	f.lastArgs = args
	return CallResult{Text: "ok:" + name}, nil
}

func do(h http.Handler, method, auth, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/mcp/interaction", strings.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestInteraction_RejectsBadToken(t *testing.T) {
	h := Handler(&fakeBackend{validToken: "good"}, nil)
	rec := do(h, http.MethodPost, "Bearer bad", `{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
	// Missing header too.
	if rec2 := do(h, http.MethodPost, "", `{}`); rec2.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth: want 401, got %d", rec2.Code)
	}
}

func TestInteraction_Initialize(t *testing.T) {
	h := Handler(&fakeBackend{validToken: "good"}, nil)
	rec := do(h, http.MethodPost, "Bearer good", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Result.ProtocolVersion != ProtocolVersion {
		t.Fatalf("want proto %s, got %s", ProtocolVersion, resp.Result.ProtocolVersion)
	}
	if got := rec.Header().Get("Mcp-Session-Id"); got != "good" {
		t.Fatalf("want session-id bound to token, got %q", got)
	}
}

func TestInteraction_ToolsList(t *testing.T) {
	h := Handler(&fakeBackend{validToken: "good"}, nil)
	rec := do(h, http.MethodPost, "Bearer good", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Result.Tools) != 1 || resp.Result.Tools[0].Name != "ask_user" {
		t.Fatalf("unexpected tools: %+v", resp.Result.Tools)
	}
}

func TestInteraction_ToolsCall(t *testing.T) {
	b := &fakeBackend{validToken: "good"}
	h := Handler(b, nil)
	rec := do(h, http.MethodPost, "Bearer good",
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ask_user","arguments":{"question":"q"}}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if b.lastName != "ask_user" {
		t.Fatalf("dispatch name = %q", b.lastName)
	}
	var resp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Result.Content) != 1 || resp.Result.Content[0].Text != "ok:ask_user" {
		t.Fatalf("unexpected result: %+v", resp.Result)
	}
}

func TestInteraction_GetNotAllowed(t *testing.T) {
	h := Handler(&fakeBackend{validToken: "good"}, nil)
	rec := do(h, http.MethodGet, "Bearer good", "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}
}
