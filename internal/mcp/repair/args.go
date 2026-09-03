// Package repair is the MCP call guard of the native tool loop: argument
// pre-checks and prefills (args.go), the not-indexed auto-repair of
// codebase-memory calls (repair.go), the per-server failure streak / breaker
// (streaks.go) and the "servers unavailable" turn notice (notice.go). It is pure
// — no runtime, no store — so every rule is unit-tested in isolation; the loop
// in internal/agent turns its verdicts into synthetic results and hints.
// Extracted from internal/agent on 2026-09-03 (_Docs/81, step 3).
package repair

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/mcp"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// This file is the PRE-execution half of the MCP argument guard; mcprepair.go is
// the post-execution half. A model that omits a required argument gets whatever
// error the server infers from the incomplete call, and those errors are often
// misleading — codebase-memory-mcp answers a missing `project` with
// "project not found or not indexed", which reads as "the repo has no index" and
// sends the agent off to grep a repo that was indexed all along. Catching the
// omission here means that call is either corrected or refused with an accurate
// message, and never reaches the server in a shape whose error lies.

// MissingRequiredArgs returns the required properties the schema declares that
// input does not supply, sorted for a stable message. A schema that is absent,
// unparsable, or declares no `required` array yields nothing — this guard only
// ever acts on an explicit contract, never on a guess.
//
// A property counts as supplied when the key is present and its value is neither
// JSON null nor an empty/blank string; `{"project": ""}` is exactly as unusable
// to the server as omitting it.
func MissingRequiredArgs(schema, input json.RawMessage) []string {
	if len(schema) == 0 {
		return nil
	}
	var s struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(schema, &s); err != nil || len(s.Required) == 0 {
		return nil
	}
	args := map[string]json.RawMessage{}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			// Not a JSON object: the server will reject it on its own terms, and
			// guessing at the arguments here would be worse than passing it through.
			return nil
		}
	}
	var missing []string
	for _, name := range s.Required {
		if !argSupplied(args[name]) {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

// argSupplied reports whether a raw argument value carries usable content.
func argSupplied(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s) != ""
	}
	return true
}

// PrefillArgs fills arguments TionHarness can derive itself when the model left
// them out. Today that is exactly one: `project` on a codebase-memory tool, which
// is derivable from the session's working directory via the server's own
// path→id rule. Returns the rewritten call and true when something was filled.
//
// It is deliberately narrow. Inventing values for arbitrary required arguments
// would paper over real model mistakes; this one is a pure restatement of
// context TionHarness already knows and the model has no reason to get right.
func PrefillArgs(call providers.ToolCall, missing []string, sessionCwd string) (providers.ToolCall, bool) {
	id := mcp.ProjectIDForPath(sessionCwd)
	if id == "" {
		return call, false
	}
	for _, name := range missing {
		if name != "project" {
			continue
		}
		fixed, err := withProjectArg(call, id)
		if err != nil {
			return call, false
		}
		return fixed, true
	}
	return call, false
}

// MissingArgsMessage renders the model-facing refusal for a call that omitted
// required arguments. It names the fields and states plainly that the call was
// never sent, so the model does not read the text as a server verdict about the
// data it was asking for.
func MissingArgsMessage(tool string, missing []string) string {
	return "This call to `" + tool + "` was not sent: it omits the required argument(s) " +
		"`" + strings.Join(missing, "`, `") + "`. " +
		"Re-issue the call with those arguments set. This is a local schema check — " +
		"it says nothing about whether the data you asked for exists."
}
