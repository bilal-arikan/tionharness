package climcp

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// TestNativeToolAllowlistBase pins the always-present core of the `--tools`
// menu and the conditional families: web tools follow the agent toggle, the
// native shell family survives only while TionHarness's shell is not bridged,
// plan-mode tools ride only the modes that can answer their exit prompt, and a
// native whose bridge is advertised is NOT kept.
func TestNativeToolAllowlistBase(t *testing.T) {
	inter := tools.InteractionEndpoint{
		URL:               "http://127.0.0.1:1/mcp",
		CoreToolNames:     []string{"Bash", "todo_write", "use_skill"},
		ExtendedToolNames: []string{"update_session"},
	}
	got := toSet(NativeToolAllowlist(db.Agent{}, inter, "auto", true))
	for _, want := range []string{"Read", "Edit", "Write", "Glob", "Grep", "NotebookEdit", "ToolSearch", "WebSearch", "WebFetch"} {
		if !got[want] {
			t.Errorf("allowlist missing %q: %v", want, got)
		}
	}
	for _, banned := range []string{"Bash", "TodoWrite", "TaskCreate", "Skill", "EnterPlanMode", "Task", "Agent", "AskUserQuestion", "SendMessage"} {
		if got[banned] {
			t.Errorf("allowlist must not keep %q while its bridge is advertised / mode is auto", banned)
		}
	}

	// Web search opted out on the agent → the natives leave the menu.
	off := false
	got = toSet(NativeToolAllowlist(db.Agent{NativeWebSearch: &off}, inter, "auto", true))
	if got["WebSearch"] || got["WebFetch"] {
		t.Error("agent with native web search off must not keep WebSearch/WebFetch")
	}

	// Shell not bridged → native Bash family stays (the agent has no other shell).
	got = toSet(NativeToolAllowlist(db.Agent{}, inter, "auto", false))
	for _, want := range []string{"Bash", "BashOutput", "KillShell", "TaskOutput", "TaskStop"} {
		if !got[want] {
			t.Errorf("shell disabled: allowlist must keep native %q", want)
		}
	}

	// Bridges not advertised this turn → the WS17 invariant keeps the native fallback.
	bare := tools.InteractionEndpoint{URL: inter.URL, CoreToolNames: []string{"Bash"}}
	got = toSet(NativeToolAllowlist(db.Agent{}, bare, "ask", true))
	for _, want := range []string{"TodoWrite", "TaskCreate", "TaskUpdate", "TaskList", "TaskGet", "Skill", "EnterPlanMode", "ExitPlanMode"} {
		if !got[want] {
			t.Errorf("unbridged/ask: allowlist must keep native %q", want)
		}
	}
}

// TestNativeToolAllowlistDisjointFromSuppression is the consistency contract
// between the positive menu (`--tools`) and the suppression list
// (`--disallowedTools`) writeCLIMCPConfig emits for the same turn: nothing may be
// both kept and suppressed, in either shell state and either permission mode.
func TestNativeToolAllowlistDisjointFromSuppression(t *testing.T) {
	inter := tools.InteractionEndpoint{
		URL:               "http://127.0.0.1:1/mcp",
		Token:             "tok",
		CoreToolNames:     []string{"Bash", "ask_user", "todo_write", "use_skill", "permission_prompt"},
		ExtendedToolNames: []string{"update_session"},
	}
	for _, shell := range []bool{true, false} {
		for _, mode := range []string{"auto", "ask", "read-only"} {
			res, err := WriteConfig(context.Background(), &fakeHost{shell: shell}, false, db.Agent{}, inter, mode)
			if err != nil {
				t.Fatalf("WriteConfig(shell=%v, mode=%s): %v", shell, mode, err)
			}
			res.Cleanup()
			kept := toSet(NativeToolAllowlist(db.Agent{}, inter, mode, shell))
			for _, d := range res.Disallowed {
				if kept[d] {
					t.Errorf("shell=%v mode=%s: %q is both allowlisted and disallowed", shell, mode, d)
				}
			}
		}
	}
}
