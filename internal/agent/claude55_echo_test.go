package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestClaude55NativeLoopPreservesThinking(t *testing.T) {
	for _, model := range []string{"claude-opus-5-5", "claude-sonnet-5-5", "claude-sonnet-5"} {
		t.Run(model, func(t *testing.T) {
			rt, tun := newTestRuntime(t, t.TempDir())
			defer drainSpawns(t, rt)
			tun.SetNativeToolSearch(false)
			tun.SetProgrammaticTools(false)
			tun.SetWebTools(false)
			tun.SetServerCompaction(false)
			turn := toolLoopTurn{r: rt, ctx: context.Background(), agent: db.Agent{ID: "a1", Model: model}, provider: &fakeProvider{}}
			if _, _, err, done := turn.prepareNativeLoop(); err != nil || done {
				t.Fatalf("prepare: err=%v done=%v", err, done)
			}
			raw := json.RawMessage(`[{"type":"thinking","thinking":"note","signature":"signed"},{"type":"tool_use","id":"t1","name":"Read","input":{}}]`)
			got := turn.rawEcho(raw)
			if model == "claude-sonnet-5" {
				if got != nil {
					t.Fatal("legacy model behavior changed")
				}
			} else if string(got) != string(raw) {
				t.Fatal("signed thinking was dropped or rewritten")
			}
		})
	}
}
