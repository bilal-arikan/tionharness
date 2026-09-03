package agent

import (
	"context"
	"log/slog"

	"github.com/bilal-arikan/tionharness/internal/climcp"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/mcp"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// cliHost adapts the Runtime to climcp.Host: the claude-cli config renderer lives
// in internal/climcp (_Docs/81 step 1) and reaches the workspace only through
// this seam.
type cliHost struct{ r *Runtime }

func (h cliHost) EnabledServers(ctx context.Context) ([]mcp.ServerConfig, error) {
	servers, err := h.r.db.ListEnabledMCPServers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]mcp.ServerConfig, 0, len(servers))
	for _, m := range servers {
		out = append(out, toServerConfig(m))
	}
	return out, nil
}

func (h cliHost) ServerGate(ctx context.Context, ag db.Agent) (func(string) bool, error) {
	return mcpServerGate(ag, h.r.allowlistExemptServer(ctx))
}

func (h cliHost) EnabledHooks(ctx context.Context, event string) ([]db.Hook, error) {
	return h.r.db.ListEnabledHooksByEvent(ctx, event)
}

func (h cliHost) ShellEnabled() bool                              { return h.r.tun.ShellEnabled() }
func (h cliHost) CLIHooksEnabled() bool                           { return h.r.tun.CLIHooksEnabled() }
func (h cliHost) Logger() *slog.Logger                            { return h.r.logger }
func (h cliHost) EmitDebug(ctx context.Context, ev db.DebugEvent) { h.r.emitDebug(ctx, ev) }

// writeCLIMCPConfig renders the claude --mcp-config file for one turn (see
// climcp.WriteConfig) and returns the path, the allowlist, the list of CLI
// built-ins to disallow, and a cleanup func.
func (r *Runtime) writeCLIMCPConfig(ctx context.Context, mcpEnabled bool, ag db.Agent, inter tools.InteractionEndpoint, mode string) (string, []string, []string, func(), error) {
	res, err := climcp.WriteConfig(ctx, cliHost{r}, mcpEnabled, ag, inter, mode)
	if err != nil {
		return "", nil, nil, nil, err
	}
	return res.Path, res.Allowed, res.Disallowed, res.Cleanup, nil
}

// writeCLISettings renders the per-turn claude --settings file (see
// climcp.WriteSettings).
func (r *Runtime) writeCLISettings(ctx context.Context, deny []string, effort string) (string, func(), error) {
	return climcp.WriteSettings(ctx, cliHost{r}, deny, effort)
}
