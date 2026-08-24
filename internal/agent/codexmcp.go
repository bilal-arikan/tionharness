package agent

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/mcp"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// This file is the codex-cli counterpart of climcp.go: it builds one turn's MCP
// delegation for a codex subprocess. The two paths are deliberately ASYMMETRIC.
//
//	claude: writeCLIMCPConfig marshals an --mcp-config JSON file and the spec
//	        carries only ConfigPath; Servers stays empty.
//	codex:  the reverse. Codex takes MCP servers as config keys rendered into its
//	        config.toml (providers.renderCodexConfig), so the spec carries the
//	        Servers map and ConfigPath stays EMPTY. There is no file to write and
//	        therefore no cleanup func to return.
//
// The server-discovery logic is duplicated rather than shared with climcp.go on
// purpose: there, building the map is interleaved with assembling the claude
// allow/disallow lists and with the JSON-only cliMCPServer type, so extracting a
// common helper would mean editing climcp.go's control flow — explicitly out of
// scope (no behaviour change is allowed on the claude path). The duplication is
// small and mechanical; the shared parts that ARE already factored out
// (toServerConfig, mcp.SplitNamespaced, the interaction tier keys) are reused
// directly.

// codexMCPSpec builds the CLIMCPSpec for one codex turn.
//
// It wires exactly the same TionHarness bridge climcp.go wires:
//   - every enabled external MCP server (when mcpEnabled), keyed by its
//     namespaced name so mcp__<key>__<tool> ids match the native path;
//   - the in-process Interaction MCP server as TWO entries at the same endpoint
//     (/core and /extended), so tool ids stay identical across both CLI dialects
//     and existing trace stripping keeps working.
//
// ConfigPath is intentionally left empty (see the file comment). Servers is nil
// when there is nothing to wire, which the caller reads as "no MCP delegation".
//
// Unlike writeCLIMCPConfig there is no permission-mode parameter: every
// mode-dependent decision on the claude path (plan-mode suppression, the
// permission-prompt tool) has no codex analogue — codex exec rejects every
// approval request outright, so permission mode is expressed purely as the
// sandbox flag the provider passes (-s read-only / -s workspace-write /
// bypass), never as MCP wiring.
func (r *Runtime) codexMCPSpec(ctx context.Context, mcpEnabled bool, ag db.Agent, inter tools.InteractionEndpoint) (providers.CLIMCPSpec, error) {
	servers := map[string]providers.CLIMCPServer{}
	var allowed, disallowed []string

	if mcpEnabled {
		list, err := r.db.ListEnabledMCPServers(ctx)
		if err != nil {
			return providers.CLIMCPSpec{}, err
		}
		// Codex has no --disallowedTools, so NOT mounting the server is the only
		// enforceable per-agent restriction on this path (see mcpservergate.go).
		gate := mcpServerGate(ag, r.allowlistExemptServer(ctx))
		for _, m := range list {
			sc := toServerConfig(m)
			key, _, _ := mcp.SplitNamespaced(mcp.NamespaceTool(sc.Name, "x"))
			if gate != nil && !gate(key) {
				r.logger.Debug("codex mcp spec: server withheld by agent tool restriction",
					"server", key, "agent", ag.ID)
				continue
			}
			entry := providers.CLIMCPServer{}
			switch sc.Transport {
			case db.MCPTransportSSE, db.MCPTransportHTTP:
				entry.Transport = sc.Transport
				entry.URL = sc.URL
				if len(sc.Headers) > 0 {
					entry.Headers = sc.Headers
				}
			default:
				entry.Transport = db.MCPTransportStdio
				entry.Command = sc.Command
				entry.Args = sc.Args
				entry.Env = sc.Env
			}
			servers[key] = entry
			allowed = append(allowed, "mcp__"+key)
		}
	}

	// The Interaction bridge carries TionHarness built-ins. A restricted profile
	// may intentionally allow only Codex-native read/shell tools plus an external
	// MCP server (validator is the first such profile). Mounting Interaction in
	// that case leaks a server-shaped catalog entry even when its tools/list is
	// empty, and older codex clients may retain the previous full catalog on a
	// resumed process. Only mount a tier when this turn's already-filtered endpoint
	// says it has at least one tool. Unrestricted agents still receive both tiers.
	if inter.URL != "" {
		for key, entry := range interactionServers(inter) {
			if !interactionTierEnabled(key, inter) {
				continue
			}
			servers[key] = entry
		}
		for _, t := range inter.CoreToolNames {
			allowed = append(allowed, "mcp__"+interactionCoreKey+"__"+t)
		}
		allowed = append(allowed, "mcp__"+interactionExtendedKey)
		disallowed = append(disallowed, codexNativeSuppressions()...)
	}

	if len(servers) == 0 {
		return providers.CLIMCPSpec{}, nil
	}

	r.logger.Info("codex mcp spec built",
		"servers", len(servers), "interaction", inter.URL != "")

	// AllowedTools / DisallowedTools travel unchanged even though codex has no
	// per-tool allow flags and renderCodexConfig ignores both fields today. They
	// are carried rather than dropped so (a) the spec stays one shape across CLI
	// dialects and (b) a future codex renderer can map them onto the per-server
	// enabled_tools / disabled_tools keys codex does support. Dropping them here
	// would hide that mapping opportunity behind a silent data loss.
	return providers.CLIMCPSpec{
		Servers:         servers,
		AllowedTools:    allowed,
		DisallowedTools: disallowed,
	}, nil
}

func interactionTierEnabled(key string, inter tools.InteractionEndpoint) bool {
	switch key {
	case interactionCoreKey:
		return len(inter.CoreToolNames) > 0
	case interactionExtendedKey:
		return len(inter.ExtendedToolNames) > 0
	default:
		return false
	}
}

// interactionServers renders the Interaction MCP endpoint as the two tier
// entries codex should mount. Both point at the SAME in-process endpoint via
// distinct path suffixes so the handler returns each tier's subset — identical
// to the claude path.
//
// AlwaysLoad is set on the core tier for shape parity only; codex has no
// tool-search deferral, so renderCodexConfig ignores it and mounts both tiers
// eagerly. That is correct for codex: every configured server's tools are always
// loaded, which is exactly what the core tier wants and merely less lazy than
// the claude path for the extended tier.
//
// Both URLs additionally carry ?full=1 (see fullTierQueryParam in package
// interaction): codex-cli receives tools/list_changed notifications but never
// re-fetches tools/list in response (it only logs them — confirmed live: a turn
// that called activate_tools never saw the tool on a later turn, and retried it
// 3 more times before giving up). The lazy activation gate can therefore never
// open for a codex turn, so ?full=1 tells the backend to (a) advertise the
// COMPLETE extended tier unconditionally instead of the empty-until-activated
// gateway set, and (b) drop the activate_tools/deactivate_tools/active_tools/
// tool_search meta-tools from both tiers — showing codex a mechanism that can
// never work only burns turns on a call→no-op→retry loop. This is a pure
// ADDITION to the URL query string; it does not touch the /core, /extended path
// segments the claude-cli path (climcp.go) also emits, so that path's request
// (no query string) is unaffected byte-for-byte.
func interactionServers(inter tools.InteractionEndpoint) map[string]providers.CLIMCPServer {
	base := trimTrailingSlash(inter.URL)
	authHeader := map[string]string{"Authorization": "Bearer " + inter.Token}
	return map[string]providers.CLIMCPServer{
		interactionCoreKey: {
			Transport:  db.MCPTransportHTTP,
			URL:        base + "/core?full=1",
			Headers:    authHeader,
			AlwaysLoad: true,
		},
		interactionExtendedKey: {
			Transport: db.MCPTransportHTTP,
			URL:       base + "/extended?full=1",
			Headers:   authHeader,
		},
	}
}

// codexNativeSuppressions lists the codex built-ins that would SHADOW a bridged
// TionHarness tool. Codex has no --disallowedTools flag, so this list is advisory
// for now: it rides DisallowedTools so the suppression intent is recorded in one
// place, and the codex config renderer disables the real overlaps structurally
// instead ([tools] update_plan / experimental_request_user_input / web_search).
//
// The list is deliberately much shorter than the claude one — codex simply does
// not ship most of those natives (no Task/Agent launcher, no Skill tool, no
// native TodoWrite family beyond update_plan). Only the genuine overlaps are
// named, so a reader can tell suppression from absence.
// web_search is left out on purpose: it only shadows a bridge when TionHarness's
// own WebSearch is enabled for the turn, which the config renderer decides via
// its DisableWebSearch flag.
func codexNativeSuppressions() []string {
	// update_plan shadows the bridged todo_write (the progress card sink);
	// experimental_request_user_input shadows ask_user. Both are also switched
	// off structurally in the rendered config.toml.
	return []string{"update_plan", "experimental_request_user_input"}
}

// trimTrailingSlash strips one trailing "/" so URL joins do not double it.
func trimTrailingSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
