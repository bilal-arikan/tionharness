package agent

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// TestUnattendedTurnDeniesGitPublish: the CLI's own Bash is the agent's only
// shell, so an unattended turn (autonomous + confine brake) carries the claude
// permission rules that keep it from publishing to a remote, while an attended
// turn does not.
func TestUnattendedTurnDeniesGitPublish(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	tun.SetWorkdirGuards(true, true)
	inter := tools.InteractionEndpoint{URL: "http://localhost/bridge", CoreToolNames: []string{"ask_user"}}

	for _, tc := range []struct {
		name       string
		autonomous bool
		want       bool
	}{
		{"attended", false, false},
		{"unattended", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &codexPolicyCapture{}
			turn := toolLoopTurn{r: rt, ctx: context.Background(), autonomous: tc.autonomous,
				provider: providers.NewClaudeCLI("claude", "", "", "", ""), cli: c, inter: inter}
			if got := turn.unattended(); got != tc.want {
				t.Fatalf("unattended() = %v, want %v", got, tc.want)
			}
			turn.req.Unattended = turn.unattended()
			turn.configureCLIMCP()()
			for _, rule := range unattendedGitDeny {
				if has := slices.Contains(c.spec.DisallowedTools, rule); has != tc.want {
					t.Errorf("%q in disallowed = %v, want %v (got %v)", rule, has, tc.want, c.spec.DisallowedTools)
				}
			}
			// Plain Bash stays callable either way: only publishing is braked.
			if slices.Contains(c.spec.DisallowedTools, "Bash") {
				t.Errorf("native Bash must stay available: %v", c.spec.DisallowedTools)
			}
		})
	}

	// The confine brake off means a human chose to let autonomous turns run free.
	tun.SetWorkdirGuards(false, true)
	if (&toolLoopTurn{r: rt, autonomous: true}).unattended() {
		t.Fatal("autonomous turn with confine off must not be unattended")
	}
}
