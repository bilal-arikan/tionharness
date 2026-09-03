package agent

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/climcp"
	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestWriteCLISettingsConvertsMatcher is the end-to-end wiring guard: a stored
// comma-glob hook must land in the generated claude-cli settings file as the
// converted+expanded regex (not the verbatim comma list that never fires).
func TestWriteCLISettingsConvertsMatcher(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	tun.SetCLIHooksEnabled(true)
	ctx := context.Background()
	// A WS15-shaped hook: plain shell matcher, no bridged names — the case this fixes.
	if _, err := rt.db.CreateHook(ctx, db.Hook{
		Event: db.HookPreToolUse, Type: "command", Enabled: true, TimeoutSec: 30,
		Matcher: "Bash,PowerShell",
		Command: "sqz hook claude",
	}); err != nil {
		t.Fatalf("create hook: %v", err)
	}

	path, cleanup, err := rt.writeCLISettings(ctx, nil, "high")
	if err != nil {
		t.Fatalf("writeCLISettings: %v", err)
	}
	defer cleanup()
	if path == "" {
		t.Fatal("expected a settings file path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	s := string(data)
	bBash, bPS := climcp.InteractionToolPrefix+"Bash", climcp.InteractionToolPrefix+"PowerShell"
	if want := `^(Bash|` + bBash + `|PowerShell|` + bPS + `)$`; !strings.Contains(s, want) {
		t.Errorf("settings must carry converted+expanded regex %q, got:\n%s", want, s)
	}
	if strings.Contains(s, `"Bash,PowerShell"`) {
		t.Errorf("settings must NOT carry the verbatim comma matcher:\n%s", s)
	}
}
