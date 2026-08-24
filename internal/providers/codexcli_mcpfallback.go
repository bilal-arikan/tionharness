package providers

import (
	"sort"
	"strings"
)

// MCP startup recovery for the codex-cli transport.
//
// Codex renders every server in config.toml with `required = true` and aborts
// the session when one of them fails to initialize ("required MCP servers
// failed to initialize"), regardless of whether the turn would ever call that
// server's tools. The reachability probe in codexcli_probe.go cannot prevent
// this: it answers a transport question (is anything listening?), while the
// failure happens one layer up, during the MCP handshake itself — a port that
// accepts the connection and then drops the initialize request passes the probe
// and still kills the turn.
//
// The recovery is therefore reactive: recognise the failure, remove the servers
// it names from the config, and re-run the turn without their tools.

// codexMCPStartupFailure reports whether msg is codex refusing to run the turn
// because an MCP server did not come up. Matching is deliberately broad — the
// wording differs between the aggregate message and the per-server one
// ("<name>: handshaking with MCP server failed") — but always requires "mcp" so
// an unrelated initialization error is not misread as this case.
func codexMCPStartupFailure(msg string) bool {
	s := strings.ToLower(msg)
	if !strings.Contains(s, "mcp") {
		return false
	}
	return strings.Contains(s, "failed to initialize") ||
		strings.Contains(s, "handshaking") ||
		strings.Contains(s, "handshake")
}

// codexNamedMCPServers returns the configured server keys that appear in msg,
// sorted. Codex names the offending server in its error, so this narrows the
// fallback to the servers that actually broke instead of disabling all tools.
func codexNamedMCPServers(msg string, servers map[string]CLIMCPServer) []string {
	s := strings.ToLower(msg)
	var named []string
	for key := range servers {
		if key == "" {
			continue
		}
		if strings.Contains(s, strings.ToLower(key)) {
			named = append(named, key)
		}
	}
	sort.Strings(named)
	return named
}

// codexRemoteServerKeys returns the sorted keys of the remote (HTTP/SSE)
// servers. It is the fallback set when the error names no server: a remote
// endpoint is what fails a handshake mid-flight, while the stdio servers are
// spawned by codex itself and carry the TionSwarm bridge the agent needs.
func codexRemoteServerKeys(servers map[string]CLIMCPServer) []string {
	var keys []string
	for key, s := range servers {
		if codexServerIsRemote(s) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// codexServersWithout returns a copy of servers with the drop keys removed. The
// input map is left untouched: it belongs to the provider and is reused by
// every later turn, which must start from the full server set again.
func codexServersWithout(servers map[string]CLIMCPServer, drop []string) map[string]CLIMCPServer {
	if len(drop) == 0 {
		return servers
	}
	skip := make(map[string]bool, len(drop))
	for _, key := range drop {
		skip[key] = true
	}
	out := make(map[string]CLIMCPServer, len(servers))
	for key, s := range servers {
		if skip[key] {
			continue
		}
		out[key] = s
	}
	return out
}
