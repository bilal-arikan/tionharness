package api

import (
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestSessionChipKeySpawnToolIsChat (TSK1005): a spawn_session child is filed
// under the "chat" chip, while run_subagent children keep their own chip.
func TestSessionChipKeySpawnToolIsChat(t *testing.T) {
	cases := []struct {
		name string
		s    db.Session
		want string
	}{
		{"spawn_session child", db.Session{Kind: "chat", Category: db.CategoryChat, ExecutionType: db.ExecutionInteractive,
			Origin: &db.SessionOrigin{Kind: db.OriginSpawn, TriggerSessionID: "SES1"}}, "chat"},
		{"run_subagent child", db.Session{Kind: "subagent", Category: db.CategorySubagent, ExecutionType: db.ExecutionSubagent}, chipSubagent},
	}
	for _, c := range cases {
		if got := sessionChipKey(c.s); got != c.want {
			t.Errorf("%s: chip = %q, want %q", c.name, got, c.want)
		}
	}
}
