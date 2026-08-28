package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/proc"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// codebaseSearch defaults: the per-project result cap and the fan-out breadth.
// Kept modest so a workspace-wide search stays token-efficient (the whole point of
// preferring the knowledge graph over grep).
const (
	defaultCodebaseSearchLimit = 30 // total results returned across all projects
	maxCodebaseProjects        = 40 // safety cap on how many projects to fan out to
)

// CodebaseWorkspaceSearchTool searches text/symbols across ALL projects indexed in
// THIS workspace's isolated codebase-memory store, in one call. codebase-memory's
// own search_code is project-scoped (one repo per call); this tool fans that out
// over every project in the store (list_projects → search_code each) and merges the
// results, so an agent can ask "where in the whole workspace is X?" without knowing
// which repo holds it. Graph/architecture queries are already fleet-wide natively;
// this fills the text-search gap.
//
// It shells out to the codebase-memory CLI (`<command> cli <tool> <json>`) with the
// workspace store selected via CBM_CACHE_DIR — the same executable and store the
// MCP server uses — so results are identical to the per-project MCP tools. Bound at
// registry-build time to the resolved executable path and store dir; registered
// only when an enabled codebase-memory server is present (command != "").
type CodebaseWorkspaceSearchTool struct {
	command string // absolute path to codebase-memory-mcp executable
}

// NewCodebaseWorkspaceSearchTool binds the tool to the codebase-memory executable.
// The server owns its own store (one cache root per account since cbm 0.10), so
// nothing is injected here.
func NewCodebaseWorkspaceSearchTool(command string) CodebaseWorkspaceSearchTool {
	return CodebaseWorkspaceSearchTool{command: command}
}

func (CodebaseWorkspaceSearchTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "codebase_workspace_search",
		Description: "Search text or symbols across ALL code projects indexed in THIS workspace " +
			"(fan-out over the whole isolated store), returning merged results grouped by project. " +
			"Use this for \"where in the workspace is X?\" when you don't know which repo holds it; for a " +
			"single known repo prefer the codebase-memory search_code tool (project-scoped, cheaper). " +
			"`pattern` is text (or a regex when `regex` is true). `mode`: compact (signatures, default), " +
			"full (with source), files (paths only). `limit` caps total results (default 30). Fans out to at most 40 projects; when the store holds more, the reply sets `truncated_projects` so you know coverage was partial.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "pattern": { "type": "string", "description": "Text or symbol to search for (regex when regex=true)." },
    "regex":   { "type": "boolean", "description": "Treat pattern as a regular expression." },
    "mode":    { "type": "string", "enum": ["compact", "full", "files"], "description": "Result detail: compact (default), full, or files." },
    "limit":   { "type": "integer", "description": "Max total results across all projects (default 30)." }
  },
  "required": ["pattern"],
  "additionalProperties": false
}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"pattern":"ListEnabledMCPServers"}`),
			json.RawMessage(`{"pattern":"func.*Prompt","regex":true,"mode":"files"}`),
		},
	}
}

// cbmProject is one row of the CLI list_projects output.
type cbmProject struct {
	Name string `json:"name"`
}

func (t CodebaseWorkspaceSearchTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Pattern string `json:"pattern"`
		Regex   bool   `json:"regex"`
		Mode    string `json:"mode"`
		Limit   int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", argErr(err)
	}
	args.Pattern = strings.TrimSpace(args.Pattern)
	if args.Pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}
	if args.Mode == "" {
		args.Mode = "compact"
	}
	if args.Limit <= 0 {
		args.Limit = defaultCodebaseSearchLimit
	}

	// 1) Enumerate the projects in this workspace's store.
	listOut, err := t.runCLI(ctx, "list_projects", map[string]any{})
	if err != nil {
		return "", fmt.Errorf("list_projects: %w", err)
	}
	var listResp struct {
		Projects []cbmProject `json:"projects"`
	}
	if err := json.Unmarshal([]byte(listOut), &listResp); err != nil {
		return "", fmt.Errorf("parse list_projects: %w", err)
	}
	if len(listResp.Projects) == 0 {
		return `{"projects_searched":0,"total":0,"results":[],"hint":"No projects indexed in this workspace's store yet."}`, nil
	}

	// 2) Fan out search_code over each project, merge results, cap the total.
	type merged struct {
		Project string          `json:"project"`
		Result  json.RawMessage `json:"result"`
	}
	var out []merged
	searched := 0
	total := 0
	// Ask each project for up to the remaining budget so one big repo can't crowd out
	// the rest, but we never exceed the overall cap.
	for _, p := range listResp.Projects {
		if searched >= maxCodebaseProjects || total >= args.Limit {
			break
		}
		searched++
		perProj := args.Limit - total
		res, err := t.runCLI(ctx, "search_code", map[string]any{
			"project": p.Name,
			"pattern": args.Pattern,
			"regex":   args.Regex,
			"mode":    args.Mode,
			"limit":   perProj,
		})
		if err != nil {
			// A single failing project must not sink the whole search — record and move on.
			out = append(out, merged{Project: p.Name, Result: json.RawMessage(fmt.Sprintf(`{"error":%q}`, err.Error()))})
			continue
		}
		var parsed struct {
			Results []json.RawMessage `json:"results"`
		}
		if json.Unmarshal([]byte(res), &parsed) == nil && len(parsed.Results) == 0 {
			continue // no hits in this project — omit to keep the payload lean
		}
		total += len(parsed.Results)
		out = append(out, merged{Project: p.Name, Result: json.RawMessage(res)})
	}
	// Stable, deterministic ordering by project name.
	sort.Slice(out, func(i, j int) bool { return out[i].Project < out[j].Project })

	resp := map[string]any{
		"pattern":           args.Pattern,
		"projects_searched": searched,
		"total":             total,
		"results":           out,
	}
	if searched >= maxCodebaseProjects {
		resp["truncated_projects"] = true
	}
	b, err := json.Marshal(resp)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// runCLI invokes the codebase-memory executable in CLI mode and returns the last
// non-empty stdout line (the JSON result; the tool also emits info logs to stderr,
// kept separate).
func (t CodebaseWorkspaceSearchTool) runCLI(ctx context.Context, tool string, payload map[string]any) (string, error) {
	arg, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	cmd := proc.CommandContext(ctx, t.command, "cli", tool, string(arg))
	cmd.Env = os.Environ()
	stdout, err := cmd.Output()
	if err != nil {
		return "", err
	}
	// The CLI prints the JSON result on stdout; return the last non-empty line so any
	// stray leading output is ignored.
	lines := strings.Split(strings.TrimSpace(string(stdout)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s, nil
		}
	}
	return "", fmt.Errorf("empty output from %s", tool)
}
