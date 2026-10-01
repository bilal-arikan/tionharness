package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/decider"
)

func TestReadDeciderDebugRequiresScopeAndBoundsReads(t *testing.T) {
	h := decider.NewHub(decider.HubOptions{})
	tool := NewReadDeciderDebugTool(func() *decider.Hub { return h })
	text, err := tool.Call(context.Background(), nil)
	if err != nil || !strings.Contains(text, "No current session") {
		t.Fatalf("unscoped = %q %v", text, err)
	}
	text, err = tool.Call(context.Background(), []byte(`{"all_sessions":true}`))
	if err != nil || !strings.Contains(text, `"events": []`) || !strings.Contains(text, `"retainedEvents": 0`) {
		t.Fatalf("summary = %q %v", text, err)
	}
	for _, input := range []string{`{"days":91}`, `{"limit":-1}`, `{"limit":1001}`, `invalid json`} {
		if _, err := tool.Call(context.Background(), []byte(input)); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
	if tool.Def().Name != "read_decider_debug" {
		t.Fatal("unexpected tool name")
	}
}
