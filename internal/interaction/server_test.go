package interaction

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncRecorder is a concurrency-safe http.ResponseWriter for streaming tests. A
// plain httptest.ResponseRecorder races: the SSE handler writes frames from its own
// goroutine while the test polls the body. All body access here is mutex-guarded.
type syncRecorder struct {
	mu     sync.Mutex
	body   bytes.Buffer
	header http.Header
}

func newSyncRecorder() *syncRecorder { return &syncRecorder{header: make(http.Header)} }

func (r *syncRecorder) Header() http.Header { return r.header }

func (r *syncRecorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body.Write(p)
}

func (r *syncRecorder) WriteHeader(int) {}

func (r *syncRecorder) Flush() {}

func (r *syncRecorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body.String()
}

// fakeBackend is a minimal Backend for protocol tests.
type fakeBackend struct {
	validToken string
	lastName   string
	lastArgs   json.RawMessage
}

func (f *fakeBackend) Valid(token string) bool { return token == f.validToken }
func (f *fakeBackend) Tools(_, _ string) []ToolSpec {
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

// TestInteraction_GetStreamReceivesPush verifies the GET SSE stream is held open and
// a PushToolsChanged frame reaches it — the gateway push channel (Doc 52 Faz 1). The
// stream blocks until its request context is cancelled, so we drive it with a
// cancellable context and cancel once the frame has been observed.
func TestInteraction_GetStreamReceivesPush(t *testing.T) {
	srv := NewServer(&fakeBackend{validToken: "good"}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/mcp/interaction", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer good")
	rec := newSyncRecorder()

	done := make(chan struct{})
	go func() { srv.ServeHTTP(rec, req); close(done) }()

	// Wait until the stream is registered, then push a list_changed frame.
	deadline := time.Now().Add(2 * time.Second)
	for !srv.HasStream("good") {
		if time.Now().After(deadline) {
			t.Fatal("stream was not registered")
		}
		time.Sleep(2 * time.Millisecond)
	}
	if !srv.PushToolsChanged("good") {
		t.Fatal("PushToolsChanged returned false for an open stream")
	}

	// Give the writer a moment to flush, then close the stream and inspect the body.
	for time.Now().Before(deadline) {
		if strings.Contains(rec.String(), "tools/list_changed") {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	cancel()
	<-done
	if !strings.Contains(rec.String(), "notifications/tools/list_changed") {
		t.Fatalf("SSE stream did not carry list_changed; body=%q", rec.String())
	}
}

// TestInteraction_PushNoStreamIsNoop verifies a push to a session with no open
// stream is a safe no-op (returns false, does not block).
func TestInteraction_PushNoStreamIsNoop(t *testing.T) {
	srv := NewServer(&fakeBackend{validToken: "good"}, nil)
	if srv.PushToolsChanged("good") {
		t.Fatal("push to a session with no open stream should return false")
	}
}
