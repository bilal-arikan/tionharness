package mcp

import "strings"

// ProjectIDForPath derives the codebase-memory-mcp project id from a repository
// path — the path-based key the server uses (C-Users-user-Desktop-<repo>).
// Shared by the capability context (internal/agent) and the MCP call repair
// (internal/mcp/repair) so the two can never disagree on the spelling.
func ProjectIDForPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	b := make([]rune, 0, len(p))
	for _, ch := range p {
		switch {
		case ch == '/' || ch == '\\' || ch == ':':
			if len(b) > 0 && b[len(b)-1] != '-' {
				b = append(b, '-')
			}
		case (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-':
			b = append(b, ch)
		}
	}
	return strings.Trim(string(b), "-")
}
