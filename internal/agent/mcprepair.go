package agent

import (
	"encoding/json"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// mcpNotIndexedMarker is the exact substring codebase-memory-mcp (and any MCP
// server following the same convention) returns in an error result when the
// requested project has never been indexed. Matching on the message body — not
// a status code — is deliberate: MCP surfaces this as an ordinary isError tool
// result, so the body is the only reliable signal.
const mcpNotIndexedMarker = "project not found or not indexed"

// mcpToolPrefix is the namespace every MCP tool call carries (mcp__<server>__<tool>).
const mcpToolPrefix = "mcp__"

// mcpRepair is a per-turn repair for MCP tool calls that fail because their
// `project` argument names a repo the server has not indexed. Without it the
// model reads a raw JSON error, never connects it to a recovery action, and
// re-issues the identical call — a tight loop the generic loop guardrail only
// breaks after several repeats. This guard breaks it on the FIRST repeat:
//
//   - repair() (post-execution): on a not-indexed error it turns the response's
//     available_projects list into a concrete instruction (call list_projects,
//     copy an exact project id) appended to the result, and remembers the
//     offending call.
//   - precheck() (pre-execution): the very next identical call is refused with
//     that same instruction instead of hitting the server again.
//
// It is side-effect free beyond its own poisoned-set state; the loop owns
// turning its verdicts into synthetic results and appended hints, mirroring the
// toolGuard pattern in this package.
type mcpRepair struct {
	poisoned map[string]bool // callKey → already answered with a not-indexed repair this turn
}

func newMCPRepair() *mcpRepair {
	return &mcpRepair{poisoned: map[string]bool{}}
}

// precheck runs BEFORE a call executes. It returns a non-empty message when this
// exact call already produced a not-indexed error earlier this turn, so the loop
// can refuse it without a second round-trip. Unlike the loop guardrail this is
// NOT gated on a hard-stop setting: repeating a call we already KNOW resolves the
// project the same (unindexed) way cannot make progress, so it is always
// short-circuited.
func (m *mcpRepair) precheck(call providers.ToolCall) (blocked bool, msg string) {
	if !strings.HasPrefix(call.Name, mcpToolPrefix) {
		return false, ""
	}
	if m.poisoned[callKey(call)] {
		return true, mcpRepairInstruction(call.Name, nil)
	}
	return false, ""
}

// repair runs AFTER a call executed. On a not-indexed error it records the call
// as poisoned and returns a guidance hint to append to the result body (the raw
// server error is kept so the model still sees the available_projects list).
// Returns ("", false) for anything else, leaving the result untouched.
func (m *mcpRepair) repair(call providers.ToolCall, res providers.ToolResult) (hint string, ok bool) {
	if !res.IsError || !strings.HasPrefix(call.Name, mcpToolPrefix) {
		return "", false
	}
	if !strings.Contains(res.Content, mcpNotIndexedMarker) {
		return "", false
	}
	m.poisoned[callKey(call)] = true
	return mcpRepairInstruction(call.Name, parseAvailableProjects(res.Content)), true
}

// parseAvailableProjects extracts the available_projects list from a not-indexed
// error body. The body is (or begins with) a JSON object; a Decoder is used so
// trailing text after the object does not fail the parse. A parse miss returns
// nil — the instruction then simply omits the explicit list and still directs
// the model to list_projects. It never fabricates a list.
func parseAvailableProjects(body string) []string {
	i := strings.IndexByte(body, '{')
	if i < 0 {
		return nil
	}
	var payload struct {
		Available []string `json:"available_projects"`
	}
	dec := json.NewDecoder(strings.NewReader(body[i:]))
	if err := dec.Decode(&payload); err != nil {
		return nil
	}
	return payload.Available
}

// mcpSiblingTool rewrites a namespaced MCP tool name to a sibling tool on the
// same server (mcp__srv__search_code → mcp__srv__list_projects), so the guidance
// can name the exact recovery tool the model must call. Falls back to the bare
// sibling name if the input is not namespaced.
func mcpSiblingTool(tool, sibling string) string {
	if i := strings.LastIndex(tool, "__"); i >= 0 {
		return tool[:i+2] + sibling
	}
	return sibling
}

// mcpRepairInstruction renders the model-facing (English) recovery guidance for a
// not-indexed MCP call. It names the exact list_projects tool for the same
// server, states the project-id format, lists the currently indexed projects when
// known, and prescribes the Glob/Grep fallback when the target repo is absent.
func mcpRepairInstruction(tool string, projects []string) string {
	list := mcpSiblingTool(tool, "list_projects")
	var b strings.Builder
	b.WriteString("\n\n[mcp repair] The `project` argument names a repo that is not indexed, so this call cannot succeed — repeating it unchanged will fail identically. ")
	b.WriteString("Before calling any mcp__ codebase-memory tool again: call ")
	b.WriteString(list)
	b.WriteString(" once, then copy a project id VERBATIM from its output into `project` (format: C-Users-user-Desktop-<repo>). ")
	if len(projects) > 0 {
		b.WriteString("Indexed projects right now: ")
		b.WriteString(strings.Join(projects, ", "))
		b.WriteString(". ")
	}
	b.WriteString("If the repo you need is not in that list, do NOT keep using this MCP — fall back to Glob/Grep for this repo.")
	return b.String()
}
