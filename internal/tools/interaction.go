package tools

import "context"

// InteractionEndpoint locates the SwarmGo Interaction MCP server for the current
// turn: the loopback URL a CLI subprocess (claude-cli, ...) should call, plus the
// per-run Bearer token that correlates its calls back to this turn. It is empty
// in autonomous runs and whenever no interaction server is wired.
type InteractionEndpoint struct {
	URL   string // e.g. http://127.0.0.1:8090/mcp/interaction
	Token string // per-run opaque secret carried as Authorization: Bearer
	// CoreToolNames / ExtendedToolNames are the bare tool names the Interaction MCP
	// server advertises for this turn, split into two tiers so the CLI MCP-config
	// writer can map each to its own server entry (claude-cli 2.1.x+):
	//   - Core: eager tools (Bash, ask_user, use_skill, ...) on a server marked
	//     alwaysLoad → never deferred by the CLI's tool search.
	//   - Extended: the self-management suite + NameOnly session-lifecycle tools on
	//     a separate server → subject to the CLI's ToolSearch deferral (lazy).
	// Each name becomes an `mcp__<server>__<tool>` allowlist entry, so the advertised
	// set and the allowlist derive from a SINGLE source per tier.
	CoreToolNames     []string
	ExtendedToolNames []string
}

// interactionKey keys the endpoint on a request context.
type interactionKey struct{}

// WithInteractionEndpoint attaches the interaction endpoint so the CLI MCP config
// writer can emit a server entry pointing the subprocess back at this turn. Kept
// in the tools package (not agent) so the bridge mirrors WithAsker and avoids an
// import cycle.
func WithInteractionEndpoint(ctx context.Context, url, token string, coreNames, extendedNames []string) context.Context {
	return context.WithValue(ctx, interactionKey{}, InteractionEndpoint{
		URL: url, Token: token, CoreToolNames: coreNames, ExtendedToolNames: extendedNames,
	})
}

// InteractionFrom returns the endpoint attached to ctx, or the zero value when
// none is present.
func InteractionFrom(ctx context.Context) InteractionEndpoint {
	ep, _ := ctx.Value(interactionKey{}).(InteractionEndpoint)
	return ep
}
