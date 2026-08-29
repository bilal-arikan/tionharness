package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/insight"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// InsightApplyFindingTool lets an agent triage an insight finding by setting its
// lifecycle status (accepted / dismissed / applied / verified). It records the
// decision only — the proposed fix stays advisory (no workspace mutation here).
type InsightApplyFindingTool struct{ db *db.DB }

// NewInsightApplyFindingTool binds the tool to a workspace DB.
func NewInsightApplyFindingTool(database *db.DB) InsightApplyFindingTool {
	return InsightApplyFindingTool{db: database}
}

func (InsightApplyFindingTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "insight_apply_finding",
		Description: "Triage one or more insight findings by setting their status: \"new\" (untriaged), " +
			"\"triaged\" (reviewed, not yet decided), \"accepted\" (you will act on it), \"dismissed\" (not worth " +
			"acting on), \"applied\" (the proposed fix has been done) or \"verified\" (the fix is confirmed " +
			"effective). Records the decision only — it does not mutate the workspace itself. \"applied\" REQUIRES " +
			"evidence: the entityType+entityId of the workspace entity you actually changed; without it the call " +
			"fails (use \"accepted\" or \"dismissed\" for a decision you did not act on). " +
			"Find ids with insight_list_findings.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "id":     { "type": "string", "description": "The finding id. Use it alone, or together with ids." },
    "ids":    { "type": "array", "items": { "type": "string" }, "description": "Several finding ids to set at once (same status and evidence for all)." },
    "status": { "type": "string", "enum": ["new", "triaged", "accepted", "applied", "verified", "dismissed"], "description": "New lifecycle status." },
    "applyCluster": { "type": "boolean", "description": "Also set every finding clustered with the given id(s) — the near-duplicates insight_list_findings collapses under one representative." },
    "evidence": {
      "type": "object",
      "description": "The workspace entity you changed to apply the finding. Mandatory for status \"applied\".",
      "properties": {
        "entityType": { "type": "string", "description": "Entity kind you mutated, e.g. skill, agent, hook, automation." },
        "entityId":   { "type": "string", "description": "Its id/slug, e.g. tionharness-tool-discovery." }
      },
      "required": ["entityType", "entityId"],
      "additionalProperties": false
    }
  },
  "required": ["status"],
  "additionalProperties": false
}`),
	}
}

func (t InsightApplyFindingTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		ID           string                 `json:"id"`
		IDs          []string               `json:"ids"`
		Status       string                 `json:"status"`
		ApplyCluster bool                   `json:"applyCluster"`
		Evidence     *insight.AppliedEntity `json:"evidence"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", argErr(err)
	}
	status := insight.FindingStatus(args.Status)
	if !insight.ValidStatus(status) {
		return "", fmt.Errorf("invalid status %q", args.Status)
	}
	ids := mergeFindingIDs(args.ID, args.IDs)
	if len(ids) == 0 {
		return "", fmt.Errorf("no finding id given: pass id and/or ids")
	}
	store, err := insight.OpenFindingStore(t.db.Root())
	if err != nil {
		return "", err
	}
	if args.ApplyCluster {
		ids = expandToClusters(store, ids)
	}
	updated, err := store.SetStatusMany(ids, status, time.Now().Unix(), args.Evidence)
	if err != nil {
		return "", err
	}
	if len(updated) == 0 {
		return "", fmt.Errorf("finding(s) %s not found", strings.Join(ids, ", "))
	}
	out := fmt.Sprintf("%d finding(s) set to %s: %s.", len(updated), args.Status, strings.Join(updated, ", "))
	if missing := missingIDs(ids, updated); len(missing) > 0 {
		out += fmt.Sprintf(" Not found: %s.", strings.Join(missing, ", "))
	}
	if args.Evidence.Valid() {
		out += fmt.Sprintf(" Evidence: %s.", args.Evidence)
	}
	return out, nil
}

// mergeFindingIDs folds the singular id and the ids array into one deduplicated,
// order-preserving list.
func mergeFindingIDs(id string, ids []string) []string {
	out := make([]string, 0, len(ids)+1)
	seen := map[string]bool{}
	for _, v := range append([]string{id}, ids...) {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// expandToClusters adds, for every requested id, the other members of the display
// cluster it belongs to (internal/insight/cluster.go) — the near-duplicates the
// list view collapses under one representative, which describe the same root
// cause and are therefore closed by the same fix.
func expandToClusters(store *insight.FindingStore, ids []string) []string {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	out := append([]string(nil), ids...)
	for _, c := range insight.ClusterFindings(store.List("", "")) {
		hit := false
		for _, m := range c.Members {
			if want[m.ID] {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		for _, m := range c.Members {
			if !want[m.ID] {
				want[m.ID] = true
				out = append(out, m.ID)
			}
		}
	}
	return out
}

// missingIDs returns the requested ids the store did not carry, so a partially
// matching batch reports the gap instead of silently succeeding.
func missingIDs(requested, updated []string) []string {
	done := make(map[string]bool, len(updated))
	for _, id := range updated {
		done[id] = true
	}
	var missing []string
	for _, id := range requested {
		if !done[id] {
			missing = append(missing, id)
		}
	}
	return missing
}

// InsightScanner is the runtime capability the insight_scan tool delegates to.
// Defined here (not imported from agent) so the tools package stays free of an
// agent import; *agent.Runtime satisfies it structurally and is passed in at
// registry-build time.
type InsightScanner interface {
	RunInsightScan(ctx context.Context, scope insight.ScanScope, agentID string) (insight.ScanResult, error)
}

// InsightScanTool lets an agent trigger a retrospective scan (_Docs/60). Scans
// are incremental (a session already scanned by a lens is skipped unless it
// changed), so re-running is cheap and safe.
type InsightScanTool struct{ scanner InsightScanner }

// NewInsightScanTool binds the tool to the runtime scanner.
func NewInsightScanTool(scanner InsightScanner) InsightScanTool {
	return InsightScanTool{scanner: scanner}
}

func (InsightScanTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "insight_scan",
		Description: "Run a retrospective scan over this workspace's past sessions through the Insight " +
			"lenses (_Docs/60), producing findings routed to app-fix (bugs in TionHarness itself) or " +
			"workspace-opt (things to optimize here). Incremental: a session already scanned by a lens is " +
			"skipped unless it changed, so re-running is cheap. Returns a summary; read the findings with " +
			"insight_list_findings. Omit lensIds to run all enabled lenses.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "lensIds":         { "type": "array", "items": { "type": "string" }, "description": "Lens ids to run (default: every enabled lens). Each session is analyzed once per lens, so more lenses means proportionally more LLM calls." },
    "analysisAgentId": { "type": "string", "description": "Agent whose model runs the analysis (default: the workspace insight setting, else the first agent)." },
    "sessionAgentId":  { "type": "string", "description": "Only scan sessions owned by this agent (default: all)." },
    "includeArchived": { "type": "boolean", "description": "Include archived sessions (default false)." },
    "maxSessions":     { "type": "integer", "description": "Override the workspace session cap for this run. Omit (or 0) to use the workspace setting, which defaults to 10 — 0 does NOT mean unlimited here. Cost: each scanned session costs one LLM call per lens, so raising this multiplies spend; raise it only deliberately, in small steps." }
  },
  "additionalProperties": false
}`),
	}
}

func (t InsightScanTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		LensIDs         []string `json:"lensIds"`
		AnalysisAgentID string   `json:"analysisAgentId"`
		SessionAgentID  string   `json:"sessionAgentId"`
		IncludeArchived bool     `json:"includeArchived"`
		MaxSessions     int      `json:"maxSessions"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return "", argErr(err)
		}
	}
	res, err := t.scanner.RunInsightScan(ctx, insight.ScanScope{
		LensIDs:         args.LensIDs,
		AgentID:         args.SessionAgentID,
		IncludeArchived: args.IncludeArchived,
		MaxSessions:     args.MaxSessions,
	}, args.AnalysisAgentID)
	if err != nil {
		return "", err
	}
	out := fmt.Sprintf("Scan complete: %d sessions, %d analyzed, %d skipped (unchanged), %d prefiltered, %d findings.",
		res.Sessions, res.Analyzed, res.Skipped, res.Prefiltered, res.Findings)
	if len(res.Errors) > 0 {
		out += fmt.Sprintf(" %d error(s): %s", len(res.Errors), summarizeScanErrors(res.Errors))
	}
	out += " Use insight_list_findings to review the findings."
	return out, nil
}

// Bounds for the scan error summary: a failing provider repeats the same message
// once per session×lens, so the raw list is both huge and uninformative.
const (
	maxScanErrorSignatures = 3
	maxScanErrorMessageLen = 300
)

// summarizeScanErrors collapses repeated identical error messages into one line
// with a count, keeps at most maxScanErrorSignatures distinct signatures (in
// first-seen order) and reports the rest as "+N more". Nothing is hidden
// silently: the caller prints the total count and every kept message stays
// readable (long ones are truncated with an explicit ellipsis).
func summarizeScanErrors(errs []string) string {
	type entry struct {
		msg   string
		count int
	}
	order := make([]string, 0, len(errs))
	seen := make(map[string]*entry, len(errs))
	for _, e := range errs {
		if cur, ok := seen[e]; ok {
			cur.count++
			continue
		}
		seen[e] = &entry{msg: e, count: 1}
		order = append(order, e)
	}
	kept := order
	dropped := 0
	if len(kept) > maxScanErrorSignatures {
		dropped = len(kept) - maxScanErrorSignatures
		kept = kept[:maxScanErrorSignatures]
	}
	parts := make([]string, 0, len(kept)+1)
	for _, key := range kept {
		en := seen[key]
		msg := en.msg
		if len(msg) > maxScanErrorMessageLen {
			msg = msg[:maxScanErrorMessageLen] + "…(truncated)"
		}
		if en.count > 1 {
			msg = fmt.Sprintf("%s (×%d)", msg, en.count)
		}
		parts = append(parts, msg)
	}
	if dropped > 0 {
		parts = append(parts, fmt.Sprintf("+%d more distinct error(s)", dropped))
	}
	return strings.Join(parts, "; ")
}

// InsightFindingsTool exposes the retrospective scanner's findings store
// (_Docs/60) to an agent, so it can review what past-session scans surfaced —
// app-fix findings (bugs in TionHarness itself) and workspace-opt findings — and
// act on them. Read-only; scans are triggered via the API / the insight panel.
type InsightFindingsTool struct{ db *db.DB }

// NewInsightFindingsTool binds the tool to a workspace DB.
func NewInsightFindingsTool(database *db.DB) InsightFindingsTool {
	return InsightFindingsTool{db: database}
}

func (InsightFindingsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "insight_list_findings",
		Description: "List findings produced by the retrospective session scanner (Insight), ranked by " +
			"priority (severity × recurrence, REGRESSED first). Each finding is a recurring issue distilled " +
			"from past sessions and routed to a channel: \"app-fix\" (a bug/gap in TionHarness itself, to feed " +
			"development) or \"workspace-opt\" (something you can optimize inside this workspace). Each line " +
			"starts with the finding id — pass it to insight_apply_finding to triage. Optionally filter by " +
			"lens, channel or status. Set cluster:true to collapse near-duplicate findings into one " +
			"representative + a count (a single root cause often produces many similar findings). By default " +
			"each finding is one summary line; set verbose:true to also get root cause, proposed fix and file " +
			"pointer. Set runId to the scan run (or the scan SESSION id) whose findings you want — the way to " +
			"act on just-finished scan instead of the whole untriaged backlog. Read-only.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "lens":    { "type": "string", "description": "Filter to one lens id (e.g. tool-errors)." },
    "channel": { "type": "string", "enum": ["app-fix", "workspace-opt"], "description": "Filter to one channel." },
    "status":  { "type": "string", "enum": ["new", "triaged", "accepted", "applied", "verified", "dismissed"], "description": "Filter to one lifecycle status (e.g. \"new\" for untriaged findings)." },
    "runId":   { "type": "string", "description": "Only findings produced or re-confirmed by ONE scan run. Accepts the run id or the id of that run's scan session (the {{sessionId}} an insight-scan automation is given)." },
    "cluster": { "type": "boolean", "description": "Collapse near-duplicate findings into clusters (representative + count)." },
    "verbose": { "type": "boolean", "description": "Also print each finding's root cause, proposed fix and file pointer (default false = one summary line per finding). Costly: only set it once you have picked the ids you actually want to act on." },
    "limit":   { "type": "integer", "description": "Max findings (or clusters) to return (default 30)." }
  },
  "additionalProperties": false
}`),
	}
}

func (t InsightFindingsTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Lens    string `json:"lens"`
		Channel string `json:"channel"`
		Status  string `json:"status"`
		RunID   string `json:"runId"`
		Cluster bool   `json:"cluster"`
		Verbose bool   `json:"verbose"`
		Limit   int    `json:"limit"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return "", argErr(err)
		}
	}
	store, err := insight.OpenFindingStore(t.db.Root())
	if err != nil {
		return "", err
	}
	findings := store.List(args.Lens, insight.Channel(args.Channel))
	if ref := strings.TrimSpace(args.RunID); ref != "" {
		// The caller may pass either identity of a run; resolve through the run log
		// so a scan SESSION id works too. An unknown ref is an error, not an empty
		// list: silently returning "no findings" would read as "the scan found
		// nothing" when it actually means "that run does not exist".
		run, ok := insight.FindRun(t.db.Root(), ref)
		if !ok {
			return "", fmt.Errorf("unknown insight run %q: no run has that run id or scan session id", ref)
		}
		filtered := findings[:0]
		for _, f := range findings {
			if f.LastRunID == run.ID {
				filtered = append(filtered, f)
			}
		}
		findings = filtered
	}
	if args.Status != "" {
		filtered := findings[:0]
		for _, f := range findings {
			if string(f.Status) == args.Status {
				filtered = append(filtered, f)
			}
		}
		findings = filtered
	}
	if args.Limit <= 0 {
		args.Limit = 30
	}
	if len(findings) == 0 {
		return "No insight findings match. Run a scan first (insight_scan), or relax the filter.", nil
	}

	if args.Cluster {
		clusters := insight.ClusterFindings(findings)
		if len(clusters) > args.Limit {
			clusters = clusters[:args.Limit]
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%d cluster(s) (representative id first; act on it or list without cluster for all ids):\n", len(clusters))
		for _, c := range clusters {
			f := c.Representative
			fmt.Fprintf(&b, "\n%s [%s · %s]%s %s", f.ID, f.Channel, f.LensID, regressedMark(f), f.Title)
			if f.Severity != "" {
				fmt.Fprintf(&b, " (%s)", f.Severity)
			}
			if c.Size > 1 {
				fmt.Fprintf(&b, " — +%d similar", c.Size-1)
			}
			if !args.Verbose {
				continue
			}
			if f.RootCause != "" {
				fmt.Fprintf(&b, "\n  cause: %s", f.RootCause)
			}
			if f.ProposedFix != "" {
				fmt.Fprintf(&b, "\n  fix: %s", f.ProposedFix)
			}
			if f.FilePointer != "" {
				fmt.Fprintf(&b, "\n  file: %s", f.FilePointer)
			}
			// The member ids matter in verbose mode: they are what you pass to
			// insight_apply_finding (or reproduce with applyCluster) to close the
			// whole cluster, and without them the representative hides them.
			if c.Size > 1 {
				fmt.Fprintf(&b, "\n  members: %s", strings.Join(clusterMemberIDs(c), ", "))
			}
		}
		if !args.Verbose {
			b.WriteString("\n\nSummary view — call again with verbose:true for root cause, proposed fix, file pointer and cluster member ids.")
		}
		return b.String(), nil
	}

	if len(findings) > args.Limit {
		findings = findings[:args.Limit]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d finding(s), priority-ranked — the leading token is the id for insight_apply_finding:\n", len(findings))
	for _, f := range findings {
		status := string(f.Status)
		if status == "" {
			status = "new"
		}
		fmt.Fprintf(&b, "\n%s [%s · %s · %s]%s %s", f.ID, f.Channel, f.LensID, status, regressedMark(f), f.Title)
		if f.Severity != "" {
			fmt.Fprintf(&b, " (%s)", f.Severity)
		}
		if f.Occurrences > 1 {
			fmt.Fprintf(&b, " ×%d", f.Occurrences)
		}
		if !args.Verbose {
			continue
		}
		if f.RootCause != "" {
			fmt.Fprintf(&b, "\n  cause: %s", f.RootCause)
		}
		if f.ProposedFix != "" {
			fmt.Fprintf(&b, "\n  fix: %s", f.ProposedFix)
		}
		if f.FilePointer != "" {
			fmt.Fprintf(&b, "\n  file: %s", f.FilePointer)
		}
	}
	if !args.Verbose {
		b.WriteString("\n\nSummary view — call again with verbose:true for root cause, proposed fix and file pointer.")
	}
	return b.String(), nil
}

// clusterMemberIDs lists every id in a cluster, representative first (it is
// Members[0] by construction).
func clusterMemberIDs(c insight.Cluster) []string {
	out := make([]string, 0, len(c.Members))
	for _, m := range c.Members {
		out = append(out, m.ID)
	}
	return out
}

// regressedMark returns a compact marker for a regressed finding (a closed issue
// that came back), empty otherwise.
func regressedMark(f insight.Finding) string {
	if f.Regressed {
		return " ⚠REGRESSED"
	}
	return ""
}
