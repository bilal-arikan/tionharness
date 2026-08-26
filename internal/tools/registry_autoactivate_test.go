package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

type autoActivateStub struct{ calls int }

func (s *autoActivateStub) Def() providers.ToolDef {
	return providers.ToolDef{Name: "server__deferred", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (s *autoActivateStub) Call(context.Context, json.RawMessage) (string, error) {
	s.calls++
	return "ran", nil
}

func TestRegistryCallDeferredAutoActivation(t *testing.T) {
	tests := []struct {
		name    string
		allow   func(string) bool
		call    string
		want    string
		isError bool
		calls   int
	}{
		{name: "first call activates without execution", call: "deferred", want: "activated automatically", isError: true, calls: 0},
		{name: "second call executes", call: "server__deferred", want: "ran", calls: 1},
		{name: "unknown remains unknown", call: "server__defered", want: "nearby tools: server__deferred", isError: true, calls: 1},
		{name: "disallowed is explicit", allow: func(string) bool { return false }, call: "server__deferred", want: "exists but is disabled or not permitted", isError: true, calls: 0},
	}

	var sharedActive = NewActiveTools()
	var sharedStub = &autoActivateStub{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			active, stub := sharedActive, sharedStub
			if tt.allow != nil {
				active, stub = NewActiveTools(), &autoActivateStub{}
			}
			reg := NewRegistry(stub)
			reg.MarkNameOnly("server__deferred")
			reg.ConfigureAutoActivation(active, tt.allow)
			res := reg.Call(context.Background(), providers.ToolCall{Name: tt.call})
			if !strings.Contains(res.Content, tt.want) || res.IsError != tt.isError {
				t.Fatalf("result = (%q, error=%v), want substring %q, error=%v", res.Content, res.IsError, tt.want, tt.isError)
			}
			if stub.calls != tt.calls {
				t.Fatalf("tool calls = %d, want %d", stub.calls, tt.calls)
			}
		})
	}

	for i := 0; i < 3; i++ {
		reg := NewRegistry()
		reg.ConfigureAutoActivation(NewActiveTools(), nil)
		res := reg.Call(context.Background(), providers.ToolCall{Name: "still_missing"})
		if !res.IsError || !strings.Contains(res.Content, `unknown tool "still_missing"`) {
			t.Fatalf("unknown call %d = %#v", i+1, res)
		}
	}
}
