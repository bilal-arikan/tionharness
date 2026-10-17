package providers

import (
	"encoding/json"
	"fmt"
)

// Names and renderer for the server-side web tools claude-cli reports on its
// stream (web_search / web_fetch blocks). They used to live in the native
// Anthropic client, which was removed when TionHarness became CLI-only
// (_Docs/95-OPENCODE-CLI-SAGLAYICI.md); the CLI stream parser still needs them.
const (
	webSearchName = "web_search"
	webFetchName  = "web_fetch"
)

// renderWebToolResult summarizes a web search/fetch result block for the trace:
// a success carries an array (search) or object (fetch) of content; an error
// carries an object with error_code. Kept to a one-line summary — the model has
// the full content in its own context; the trace is for the human.
func renderWebToolResult(sub json.RawMessage, unit string) string {
	if len(sub) == 0 {
		return "(no content)"
	}
	// Error shape: {"type":"...","error_code":"max_uses_exceeded"}.
	var errObj struct {
		ErrorCode string `json:"error_code"`
	}
	if json.Unmarshal(sub, &errObj) == nil && errObj.ErrorCode != "" {
		return "error: " + errObj.ErrorCode
	}
	var items []json.RawMessage
	if json.Unmarshal(sub, &items) == nil {
		return fmt.Sprintf("%d %s(s) (executed server-side)", len(items), unit)
	}
	return "1 " + unit + " (executed server-side)"
}
