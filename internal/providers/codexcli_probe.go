package providers

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Remote MCP servers are rendered with `required = true` (see renderCodexServer
// for why that must not be relaxed). The cost of that correctness is that a
// single unreachable remote server aborts session creation: codex exits 1 with
// "required MCP servers failed to initialize" before emitting any turn output,
// which TionHarness can only report as "codex CLI exited before producing any turn
// output". Probing reachability up front lets a dead server be OMITTED from the
// config instead — the turn loses that server's tools, loudly and once, rather
// than losing the whole session.

// codexMCPProbeTimeout bounds one reachability probe. It is deliberately short:
// this runs on every turn before the CLI starts, and the only question asked is
// whether something is listening at all.
const codexMCPProbeTimeout = 2 * time.Second

// codexMCPProbe reports whether the remote MCP server at rawURL is reachable.
// It answers a transport-level question only: a server that replies 404 or 500
// is up (its own handshake will decide the rest), while a refused connection,
// a DNS failure or a timeout is not.
type codexMCPProbe func(ctx context.Context, rawURL string) bool

// codexProbeRemoteMCP is the probe used in production. Tests replace it (via the
// probe parameter threaded through writeCodexConfig) so they perform no network
// I/O.
func codexProbeRemoteMCP(ctx context.Context, rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		// An unparseable or hostless URL can never be dialled; treat it as
		// unreachable rather than letting codex fail the whole session on it.
		return false
	}

	ctx, cancel := context.WithTimeout(ctx, codexMCPProbeTimeout)
	defer cancel()

	switch u.Scheme {
	case "http", "https":
		req, rerr := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if rerr != nil {
			return false
		}
		client := &http.Client{Timeout: codexMCPProbeTimeout}
		resp, herr := client.Do(req)
		if herr != nil {
			return false
		}
		// Any status at all means the endpoint answered. Status codes are the
		// server's business, not a reachability signal.
		resp.Body.Close()
		return true
	default:
		// Non-HTTP transports (or a bare host:port) still have to accept a TCP
		// connection before any protocol can run.
		var d net.Dialer
		conn, derr := d.DialContext(ctx, "tcp", hostPortWithDefault(u))
		if derr != nil {
			return false
		}
		conn.Close()
		return true
	}
}

// hostPortWithDefault returns u's host with a port, defaulting to the scheme's
// well-known port when the URL omits one.
func hostPortWithDefault(u *url.URL) string {
	if u.Port() != "" {
		return u.Host
	}
	if u.Scheme == "https" || u.Scheme == "wss" {
		return net.JoinHostPort(u.Hostname(), "443")
	}
	return net.JoinHostPort(u.Hostname(), "80")
}

// codexServerIsRemote reports whether s is reached over the network. It mirrors
// renderCodexServer's own remote/stdio decision exactly; the two must agree, or
// a server could be probed as one kind and rendered as the other.
func codexServerIsRemote(s CLIMCPServer) bool {
	return s.Transport == "sse" || s.Transport == "http" || s.URL != ""
}

// filterReachableCodexServers splits servers into the set to render and the
// sorted keys of the remote servers that failed their probe. stdio servers are
// never probed: they are spawned by codex itself, so there is nothing to
// connect to before the turn starts.
func filterReachableCodexServers(ctx context.Context, servers map[string]CLIMCPServer, probe codexMCPProbe) (map[string]CLIMCPServer, []string) {
	if len(servers) == 0 {
		return servers, nil
	}
	if probe == nil {
		probe = codexProbeRemoteMCP
	}

	kept := make(map[string]CLIMCPServer, len(servers))
	var dropped []string
	for _, key := range sortedKeys(servers) {
		s := servers[key]
		if !codexServerIsRemote(s) || probe(ctx, s.URL) {
			kept[key] = s
			continue
		}
		dropped = append(dropped, key)
	}
	return kept, dropped
}
