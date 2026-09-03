package repair

import (
	"encoding/json"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/mcp"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// NotIndexedMarker is the exact substring codebase-memory-mcp (and any MCP
// server following the same convention) returns in an error result when the
// requested project has never been indexed. Matching on the message body — not
// a status code — is deliberate: MCP surfaces this as an ordinary isError tool
// result, so the body is the only reliable signal.
const NotIndexedMarker = "project not found or not indexed"

// namespaceSep separates server from tool in a namespaced MCP tool name.
// Two forms reach this guard and BOTH must match:
//
//   - claude-cli form:  mcp__<server>__<tool>
//   - native-loop form: <server>__<tool>  (mcp.NamespaceTool, manager.go)
//
// Matching only the "mcp__" prefix silently disabled the whole guard on
// TionHarness's own agentic loop — the only loop it can actually run in — because
// the registry never produces that prefix. Built-in tool names carry no "__",
// so this separator is an unambiguous MCP marker.
const namespaceSep = "__"

// IsMCPToolCall reports whether name is a namespaced MCP tool call.
func IsMCPToolCall(name string) bool { return strings.Contains(name, namespaceSep) }

// Guard is a per-turn repair for MCP tool calls that fail because their
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
type Guard struct {
	poisoned map[string]bool // callKey → already answered with a not-indexed repair this turn
	repaired map[string]bool // callKey → already auto-corrected once this turn (no second rewrite)
}

func NewGuard() *Guard {
	return &Guard{poisoned: map[string]bool{}, repaired: map[string]bool{}}
}

// Plan is the verdict repair() hands back to the loop. Exactly one of the
// three fields drives the loop's next move; Hint may accompany IndexPath.
//
//   - Fixed non-nil    → re-run this corrected call once, in place of the failed one.
//   - IndexPath non-empty → kick off a background index of that repo, then explain.
//   - Hint non-empty   → append to the result body so the model reads the recovery
//     instruction where the failure happened.
//
// Keeping the decision data-only leaves Guard free of Runtime dependencies,
// so it stays unit-testable without a live workspace.
type Plan struct {
	Fixed     *providers.ToolCall
	IndexPath string
	Hint      string
}

// precheck runs BEFORE a call executes. It returns a non-empty message when this
// exact call already produced a not-indexed error earlier this turn, so the loop
// can refuse it without a second round-trip. Unlike the loop guardrail this is
// NOT gated on a hard-stop setting: repeating a call we already KNOW resolves the
// project the same (unindexed) way cannot make progress, so it is always
// short-circuited.
func (m *Guard) Precheck(call providers.ToolCall) (blocked bool, msg string) {
	if !IsMCPToolCall(call.Name) {
		return false, ""
	}
	if m.poisoned[CallKey(call)] {
		return true, mcpRepairInstruction(call.Name, "", nil)
	}
	return false, ""
}

// repair runs AFTER a call executed. On a not-indexed error it decides, in this
// order:
//
//  1. The `project` argument can be derived unambiguously from the server's own
//     available_projects list (it was missing, or it names the same repo in a
//     different shape) → return the corrected call so the loop re-runs it once.
//     The model never sees the failure and spends no turn on recovery.
//  2. The repo the session actually works on is absent from that list → ask the
//     loop to start a background index of sessionCwd, and explain that this turn
//     must fall back to Glob/Grep.
//  3. Neither → record the call as poisoned and hand back the guidance hint (the
//     raw server error is kept, so the model still sees available_projects).
//
// Returns (zero, false) for anything else, leaving the result untouched.
func (m *Guard) Repair(call providers.ToolCall, res providers.ToolResult, sessionCwd string) (Plan, bool) {
	if !res.IsError || !IsMCPToolCall(call.Name) {
		return Plan{}, false
	}
	if !strings.Contains(res.Content, NotIndexedMarker) {
		return Plan{}, false
	}
	key := CallKey(call)
	available := parseAvailableProjects(res.Content)
	want := CallProjectArg(call)
	preferred := mcp.ProjectIDForPath(sessionCwd)

	// (1) Auto-correct — at most once per call, so a rewrite that still fails
	// cannot ping-pong with the server.
	if !m.repaired[key] {
		if fixed := resolveProjectID(want, preferred, available); fixed != "" && fixed != want {
			corrected, err := withProjectArg(call, fixed)
			if err == nil {
				m.repaired[key] = true
				return Plan{Fixed: &corrected}, true
			}
			// A malformed Input cannot be rewritten; fall through to the hint so
			// the failure stays visible instead of being silently dropped.
		}
	}

	m.poisoned[key] = true
	plan := Plan{Hint: mcpRepairInstruction(call.Name, want, available)}
	// (2) The session's own repo is not in the index — trigger it for later turns.
	if preferred != "" && !containsProject(available, preferred) {
		plan.IndexPath = sessionCwd
		plan.Hint += " An index of this session's repo (" + preferred + ") has been started in the background; it will not be ready within this turn, so use Glob/Grep now and retry the index tools on a later turn."
	}
	return plan, true
}

// CallProjectArg reads the `project` argument of an MCP call. A missing field, a
// non-string value or malformed JSON all read as "" — the caller treats that as
// "unspecified", which is exactly the case auto-correction exists for.
func CallProjectArg(call providers.ToolCall) string {
	var args struct {
		Project string `json:"project"`
	}
	if err := json.Unmarshal(call.Input, &args); err != nil {
		return ""
	}
	return strings.TrimSpace(args.Project)
}

// withProjectArg returns a copy of call whose `project` argument is set to id,
// preserving every other argument. Errors when Input is not a JSON object.
func withProjectArg(call providers.ToolCall, id string) (providers.ToolCall, error) {
	args := map[string]any{}
	if len(call.Input) > 0 {
		if err := json.Unmarshal(call.Input, &args); err != nil {
			return call, err
		}
	}
	args["project"] = id
	raw, err := json.Marshal(args)
	if err != nil {
		return call, err
	}
	call.Input = raw
	return call, nil
}

// containsProject reports an exact membership test on the indexed-project list.
func containsProject(available []string, id string) bool {
	for _, a := range available {
		if a == id {
			return true
		}
	}
	return false
}

// resolveProjectID derives the project id this call SHOULD have carried, or ""
// when no single answer is defensible. It never guesses between two candidates:
// silently querying the wrong repo is worse than surfacing the error.
//
//	want      the `project` argument as sent ("" when omitted)
//	preferred the id of the repo this session works on ("" when unknown)
//	available the ids the server reports as indexed
func resolveProjectID(want, preferred string, available []string) string {
	if len(available) == 0 {
		return ""
	}
	if want != "" && containsProject(available, want) {
		return "" // argument is already right; the error has another cause
	}
	if want == "" {
		// Omitted argument — the single most common shape of this failure.
		if preferred != "" && containsProject(available, preferred) {
			return preferred
		}
		if len(available) == 1 {
			return available[0]
		}
		return ""
	}
	// Present but unknown: accept it only when exactly one indexed id plausibly
	// denotes the same repo (a bare repo name, a path, or a case difference).
	if match := matchProjectID(want, available); match != "" {
		return match
	}
	if preferred != "" && containsProject(available, preferred) {
		return preferred
	}
	if len(available) == 1 {
		return available[0]
	}
	return ""
}

// matchProjectID finds the one indexed id that denotes the same repo as want,
// tolerating case, a bare repo name ("SampleRepo") and a filesystem path
// ("C:\...\SampleRepo"). Ambiguity (two or more candidates) returns "".
func matchProjectID(want string, available []string) string {
	norm := strings.ToLower(want)
	if p := mcp.ProjectIDForPath(want); p != "" {
		norm = strings.ToLower(p)
	}
	var found string
	for _, a := range available {
		la := strings.ToLower(a)
		if la != norm && !strings.HasSuffix(la, "-"+norm) {
			continue
		}
		if found != "" {
			return "" // ambiguous
		}
		found = a
	}
	return found
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
//
// The opening sentence is chosen from `want`, because the server returns the same
// "project not found or not indexed" body for two very different faults. Telling a
// model its argument named an unindexed repo when it in fact sent NO argument sent
// it hunting for a missing index and straight to Glob/Grep — the exact wrong move
// when the repo was indexed all along.
func mcpRepairInstruction(tool, want string, projects []string) string {
	list := mcpSiblingTool(tool, "list_projects")
	var b strings.Builder
	if want == "" {
		b.WriteString("\n\n[mcp repair] This call omitted the required `project` argument, so the server could not resolve a project — the error does NOT mean the repo is unindexed. ")
		b.WriteString("Re-issue the call with `project` set. ")
	} else {
		b.WriteString("\n\n[mcp repair] The `project` argument (`" + want + "`) does not match any indexed repo, so this call cannot succeed — repeating it unchanged will fail identically. ")
	}
	b.WriteString("If you are unsure of the id, call ")
	b.WriteString(list)
	b.WriteString(" once and copy one VERBATIM from its output (format: C-Users-user-Desktop-<repo>). ")
	if len(projects) > 0 {
		b.WriteString("Indexed projects right now: ")
		b.WriteString(strings.Join(projects, ", "))
		b.WriteString(". ")
	}
	b.WriteString("If the repo you need is not in that list, do NOT keep using this MCP — fall back to Glob/Grep for this repo.")
	return b.String()
}
