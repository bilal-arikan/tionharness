package codemode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/mcp"
)

// post sends a /call request to the bridge with the given token and body.
func post(t *testing.T, b *Bridge, token, body string) (int, callResponse, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, b.URL()+"/call", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatal(err)
	}
	var out callResponse
	_ = json.Unmarshal(buf.Bytes(), &out)
	return resp.StatusCode, out, buf.String()
}

// okCall is a dispatcher stub returning a fixed successful result.
func okCall(text string) CallFunc {
	return func(context.Context, string, json.RawMessage) (mcp.CallToolResult, error) {
		return mcp.CallToolResult{Text: text}, nil
	}
}

func TestBridgeDispatchesAndFlattens(t *testing.T) {
	var gotTool string
	var gotArgs string
	b, err := Start(Config{Call: func(_ context.Context, namespaced string, args json.RawMessage) (mcp.CallToolResult, error) {
		gotTool = namespaced
		gotArgs = string(args)
		return mcp.CallToolResult{Text: `{"ok":true}`}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	code, out, _ := post(t, b, b.Token(), `{"tool":"demo__ping","args":{"msg":"hi"}}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if out.IsError || out.Text != `{"ok":true}` {
		t.Fatalf("unexpected response: %+v", out)
	}
	if gotTool != "demo__ping" || !strings.Contains(gotArgs, `"hi"`) {
		t.Fatalf("dispatcher saw tool=%q args=%q", gotTool, gotArgs)
	}
	if s := b.Summary(); !strings.Contains(s, "1 tool call(s)") || !strings.Contains(s, "demo__ping") {
		t.Fatalf("summary = %q", s)
	}
}

func TestBridgeRejectsBadToken(t *testing.T) {
	b, err := Start(Config{Call: func(context.Context, string, json.RawMessage) (mcp.CallToolResult, error) {
		t.Fatal("dispatcher must not run on auth failure")
		return mcp.CallToolResult{}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	if code, _, _ := post(t, b, "wrong-token", `{"tool":"demo__ping"}`); code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", code)
	}
	if code, _, _ := post(t, b, "", `{"tool":"demo__ping"}`); code != http.StatusUnauthorized {
		t.Fatalf("no-token status = %d, want 401", code)
	}
}

func TestBridgeEnforcesAllowFilter(t *testing.T) {
	b, err := Start(Config{
		Call:  okCall("ok"),
		Allow: func(name string) bool { return name == "demo__allowed" },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	if code, _, _ := post(t, b, b.Token(), `{"tool":"demo__blocked"}`); code != http.StatusForbidden {
		t.Fatalf("blocked tool status = %d, want 403", code)
	}
	if code, out, _ := post(t, b, b.Token(), `{"tool":"demo__allowed"}`); code != http.StatusOK || out.Text != "ok" {
		t.Fatalf("allowed tool: code=%d out=%+v", code, out)
	}
}

func TestBridgeRejectsNonNamespacedTool(t *testing.T) {
	b, err := Start(Config{Call: okCall("")})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	if code, _, body := post(t, b, b.Token(), `{"tool":"Read"}`); code != http.StatusBadRequest {
		t.Fatalf("status = %d (%s), want 400", code, body)
	}
}

func TestBridgeSurfacesDispatcherErrorAsToolError(t *testing.T) {
	b, err := Start(Config{Call: func(context.Context, string, json.RawMessage) (mcp.CallToolResult, error) {
		return mcp.CallToolResult{}, fmt.Errorf("server unreachable")
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	code, out, _ := post(t, b, b.Token(), `{"tool":"demo__ping"}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (tool-level error)", code)
	}
	if !out.IsError || !strings.Contains(out.Text, "server unreachable") {
		t.Fatalf("want loud IsError response, got %+v", out)
	}
	if s := b.Summary(); !strings.Contains(s, "1 errored") {
		t.Fatalf("summary should count the error, got %q", s)
	}
}

func TestBridgeCallCap(t *testing.T) {
	b, err := Start(Config{Call: okCall("ok")})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	b.mu.Lock()
	b.total = bridgeMaxCalls // exhaust the cap without looping 200 HTTP calls
	b.mu.Unlock()
	if code, _, _ := post(t, b, b.Token(), `{"tool":"demo__ping"}`); code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", code)
	}
}

func TestBridgeGateDeniesCall(t *testing.T) {
	var observed []CallObservation
	b, err := Start(Config{
		Call: func(context.Context, string, json.RawMessage) (mcp.CallToolResult, error) {
			t.Error("dispatcher must not run for a gate-denied call")
			return mcp.CallToolResult{}, nil
		},
		Gate: func(tool string, _ json.RawMessage) (bool, string) {
			return false, fmt.Sprintf("permission denied by user: %q was not approved", tool)
		},
		Observe: func(ob CallObservation) { observed = append(observed, ob) },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	code, out, _ := post(t, b, b.Token(), `{"tool":"demo__mutate"}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (denial is a tool-level error)", code)
	}
	if !out.IsError || !strings.Contains(out.Text, "permission denied") {
		t.Fatalf("want loud denial, got %+v", out)
	}
	if len(observed) != 1 || !observed[0].Denied || !observed[0].IsError || observed[0].Tool != "demo__mutate" {
		t.Fatalf("observer must record the denial, got %+v", observed)
	}
	if s := b.Summary(); !strings.Contains(s, "1 denied by permission gate") {
		t.Fatalf("summary should count the denial, got %q", s)
	}
}

func TestBridgeGateAllowsAndObserverRecordsDispatch(t *testing.T) {
	var gateSaw string
	var observed []CallObservation
	b, err := Start(Config{
		Call: okCall("result-text"),
		Gate: func(tool string, args json.RawMessage) (bool, string) {
			gateSaw = tool + ":" + string(args)
			return true, ""
		},
		Observe: func(ob CallObservation) { observed = append(observed, ob) },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	code, out, _ := post(t, b, b.Token(), `{"tool":"demo__ping","args":{"q":"x"}}`)
	if code != http.StatusOK || out.IsError {
		t.Fatalf("code=%d out=%+v", code, out)
	}
	if !strings.Contains(gateSaw, "demo__ping") || !strings.Contains(gateSaw, `"x"`) {
		t.Fatalf("gate must see tool + args, saw %q", gateSaw)
	}
	if len(observed) != 1 {
		t.Fatalf("observed = %+v, want 1 record", observed)
	}
	ob := observed[0]
	if ob.Tool != "demo__ping" || ob.Denied || ob.IsError || ob.OutBytes != len("result-text") {
		t.Fatalf("unexpected observation: %+v", ob)
	}
}
