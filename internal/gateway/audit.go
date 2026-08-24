package gateway

import (
	"time"

	"github.com/bilal-arikan/tionharness/internal/mcp"
)

// AuditEntry is one proxied backend tool call, the gateway-audit parity record with the
// TS gateway's gateway-audit.jsonl (Doc 52 brainstorm #10). Meta-tools are not audited —
// only calls forwarded to a backend server.
type AuditEntry struct {
	Ts     string `json:"ts"`
	Server string `json:"server"`
	Tool   string `json:"tool"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}

// recordAudit emits an audit entry for a proxied call (fire-and-forget; no-op without a
// sink). namespaced is the "server__tool" name; ok = no transport error AND not a
// tool-level error. Errors here never affect the tool call.
func (b *mcpBackend) recordAudit(namespaced string, callErr error, isToolErr bool) {
	if b.audit == nil {
		return
	}
	server, tool, ok := mcp.SplitNamespaced(namespaced)
	if !ok {
		server, tool = "", namespaced
	}
	e := AuditEntry{Ts: time.Now().UTC().Format(time.RFC3339), Server: server, Tool: tool, OK: callErr == nil && !isToolErr}
	if callErr != nil {
		e.Error = callErr.Error()
	}
	b.audit(e)
}
