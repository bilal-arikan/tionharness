package agent

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

type codexPolicyCapture struct {
	providers.CLIProvider
	spec providers.CLIMCPSpec
}

func (c *codexPolicyCapture) ConfigureCLIMCP(spec providers.CLIMCPSpec) { c.spec = spec }

func TestCodexTurnPolicyReplacesPreviousTurn(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	tun.SetShellEnabled(true)
	c := &codexPolicyCapture{}
	turn := toolLoopTurn{r: rt, ctx: context.Background(), provider: providers.NewCodexCLI("codex", "", ""), cli: c,
		inter: tools.InteractionEndpoint{URL: "http://localhost/bridge", CoreToolNames: []string{"ask_user"}}}
	turn.configureCLIMCP()()
	if !c.spec.DisableNativeShell || len(c.spec.Servers) == 0 {
		t.Fatal("bridge must disable native shell even when shell tools are excluded by agent policy")
	}
	on := true
	turn.agent.NativeShell = &on
	turn.configureCLIMCP()()
	if c.spec.DisableNativeShell {
		t.Fatal("explicit native shell opt-in lost")
	}
	turn.agent.NativeShell = nil
	tun.SetShellEnabled(false)
	turn.configureCLIMCP()()
	if c.spec.DisableNativeShell {
		t.Fatal("disabled shell bridge must preserve native fallback")
	}
	tun.SetShellEnabled(true)
	turn.inter = tools.InteractionEndpoint{}
	turn.configureCLIMCP()()
	if c.spec.DisableNativeShell || len(c.spec.Servers) > 0 {
		t.Fatal("empty turn retained previous policy")
	}
	c.spec.Servers = map[string]providers.CLIMCPServer{"stale": {Command: "unused"}}
	turn.agent.MCPEnabled = true
	turn.agent.AllowedTools = "{"
	turn.configureCLIMCP()()
	if len(c.spec.Servers) > 0 {
		t.Fatal("malformed policy retained previous servers")
	}
}

func TestCodexServerToolPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, allow, block, overrides string
		exempt, wantErr               bool
		enabled, disabled             []string
	}{
		{name: "unrestricted"},
		{name: "exact allow", allow: `["Read","probe__read"]`, enabled: []string{"read"}},
		{name: "deny wins", allow: `["probe__read"]`, block: `["probe__read"]`, enabled: []string{"read"}, disabled: []string{"read"}},
		{name: "whole server", allow: `["probe__*"]`, block: `["probe__write"]`, disabled: []string{"write"}},
		{name: "exempt still denies", allow: `["Read"]`, block: `["probe__write"]`, exempt: true, disabled: []string{"write"}},
		{name: "override supersedes legacy", block: `["probe__read"]`, overrides: `{"probe__read":"full"}`},
		{name: "other server ignored", block: `["other__write"]`},
		{name: "whole server deny", block: `["probe*"]`, enabled: []string{}},
		{name: "allow prefix refused", allow: `["probe__read*"]`, wantErr: true},
		{name: "deny prefix refused", block: `["probe__write*"]`, wantErr: true},
		{name: "redundant prefix", allow: `["probe__*","probe__read*"]`},
		{name: "malformed allow", allow: `{`, wantErr: true},
		{name: "malformed deny", block: `{`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := codexServerToolPolicy(db.Agent{AllowedTools: tc.allow, BlockedTools: tc.block, ToolOverrides: tc.overrides}, "probe", tc.exempt)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v", err)
			}
			if tc.wantErr {
				return
			}
			if !reflect.DeepEqual(p.EnabledTools, tc.enabled) || !reflect.DeepEqual(p.DisabledTools, tc.disabled) {
				t.Fatalf("policy = enabled %v disabled %v; want %v / %v", p.EnabledTools, p.DisabledTools, tc.enabled, tc.disabled)
			}
		})
	}
}

func TestCodexMCPSpecToolRestrictions(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	if _, err := rt.db.CreateMCPServer(ctx, db.MCPServer{Name: "probe", Transport: db.MCPTransportStdio, Command: "unused", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	inter := tools.InteractionEndpoint{URL: "http://localhost/bridge", CoreToolNames: []string{"ask_user"}}
	spec, err := rt.codexMCPSpec(ctx, true, db.Agent{AllowedTools: `["probe__read"]`, BlockedTools: `["probe__write"]`}, inter)
	if err != nil {
		t.Fatal(err)
	}
	p := spec.Servers["probe"]
	if !reflect.DeepEqual(p.EnabledTools, []string{"read"}) || !reflect.DeepEqual(p.DisabledTools, []string{"write"}) {
		t.Fatalf("unfiltered server: %+v", p)
	}
	spec, err = rt.codexMCPSpec(ctx, true, db.Agent{BlockedTools: `["probe__write*"]`}, inter)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := spec.Servers["probe"]; ok {
		t.Fatal("unsupported deny glob mounted server")
	}
	if len(spec.Servers) != 1 {
		t.Fatal("unrelated interaction tier must survive")
	}
}
