package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// defaultArchiveLimit caps how many sessions a single archive_sessions call may
// touch, so a too-broad filter can't sweep an entire workspace in one shot. The
// agent can raise it explicitly (or page through with tighter filters).
const defaultArchiveLimit = 100

// ArchiveSessionsTool bulk-archives OTHER chat sessions in THIS workspace. It is
// the "manage other sessions" complement to update_session (which only edits the
// current session): update_session archives the session the agent runs in, this
// tool archives many sibling sessions at once, filtered by age/title.
//
// Why it exists: without it an agent that is asked to "clean up old sessions" has
// to drop to raw REST (Invoke-RestMethod against /api/sessions/{id}/state), which
// loses the automatic workspace scoping the built-in tools get for free — the
// exact footgun that once archived the wrong (default) workspace. This tool binds
// to the workspace DB (r.db), so it is physically scoped to this workspace, and it
// always excludes the current session, so it cannot archive the session it runs
// in. Archiving is soft/reversible (state="archived" → restore with "active").
type ArchiveSessionsTool struct {
	db *db.DB
	// currentSessionID is resolved at registry-build time and is never touched by
	// this tool — the session the agent is running in must stay active.
	currentSessionID string
}

// NewArchiveSessionsTool binds the tool to a workspace DB and the current session
// id (which is always excluded from archiving).
func NewArchiveSessionsTool(database *db.DB, currentSessionID string) ArchiveSessionsTool {
	return ArchiveSessionsTool{db: database, currentSessionID: currentSessionID}
}

func (ArchiveSessionsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "archive_sessions",
		Description: "Bulk-archive OTHER chat sessions in THIS workspace (soft, reversible). Use this " +
			"to clean up old/finished sessions instead of raw HTTP calls — it is scoped to this " +
			"workspace and ALWAYS excludes the session you are running in. Filter with `idle_days` " +
			"(only sessions whose last activity is older than N days) and/or `title_contains`. " +
			"Set `dry_run` true first to preview exactly which sessions would be archived, then run " +
			"again without it to apply. Archiving only leaves the active list; restore via the UI or " +
			"by setting a session back to active. Use list_sessions to inspect candidates first.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "idle_days":      { "type": "integer", "description": "Only archive sessions whose last activity is older than this many days. 0 or omitted = no age filter (all active sessions match)." },
    "title_contains": { "type": "string", "description": "Only archive sessions whose title contains this text (case-insensitive)." },
    "exclude":        { "type": "array", "items": { "type": "string" }, "description": "Session IDs to keep active (in addition to the current session, which is always excluded)." },
    "dry_run":        { "type": "boolean", "description": "Preview only: list the sessions that WOULD be archived without changing anything." },
    "limit":          { "type": "integer", "description": "Safety cap on how many sessions to archive in one call (default 100). Matches beyond the cap are reported but left untouched." }
  },
  "additionalProperties": false
}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"idle_days":2,"dry_run":true}`),
			json.RawMessage(`{"idle_days":2}`),
			json.RawMessage(`{"title_contains":"test","dry_run":true}`),
		},
	}
}

func (t ArchiveSessionsTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		IdleDays      int      `json:"idle_days"`
		TitleContains string   `json:"title_contains"`
		Exclude       []string `json:"exclude"`
		DryRun        bool     `json:"dry_run"`
		Limit         int      `json:"limit"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return "", argErr(err)
		}
	}
	if args.IdleDays < 0 {
		return "", fmt.Errorf("idle_days must be >= 0")
	}
	if args.Limit <= 0 {
		args.Limit = defaultArchiveLimit
	}

	sessions, err := t.db.ListSessions(ctx, "") // workspace-scoped by construction
	if err != nil {
		return "", err
	}

	keep := make(map[string]struct{}, len(args.Exclude)+1)
	for _, id := range args.Exclude {
		if id = strings.TrimSpace(id); id != "" {
			keep[id] = struct{}{}
		}
	}
	// The current session is never archivable, even if a caller lists it in exclude
	// (or forgets to). "" (catalog/preview build) matches no real id, so this is a
	// no-op there rather than a wrong exclusion.
	if t.currentSessionID != "" {
		keep[t.currentSessionID] = struct{}{}
	}

	now := time.Now().Unix()
	var cutoff int64
	if args.IdleDays > 0 {
		cutoff = now - int64(args.IdleDays)*86400
	}
	needle := strings.ToLower(strings.TrimSpace(args.TitleContains))

	var matched []db.Session
	for _, s := range sessions {
		if s.Kind != "chat" {
			continue
		}
		if s.State != "active" { // already archived (or other) → nothing to do
			continue
		}
		if _, skip := keep[s.ID]; skip {
			continue
		}
		if cutoff > 0 && s.UpdatedAt >= cutoff { // too recent
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(s.Title), needle) {
			continue
		}
		matched = append(matched, s)
	}

	if len(matched) == 0 {
		return "No active sessions match — nothing to archive. (The current session is always excluded.)", nil
	}

	capped := false
	if len(matched) > args.Limit {
		matched = matched[:args.Limit]
		capped = true
	}

	if args.DryRun {
		var b strings.Builder
		fmt.Fprintf(&b, "DRY RUN — %d session(s) WOULD be archived (nothing changed):\n", len(matched))
		writeSessionLines(&b, matched, now)
		if capped {
			fmt.Fprintf(&b, "\n(capped at limit=%d; raise `limit` or tighten filters to cover the rest)", args.Limit)
		}
		return strings.TrimSpace(b.String()), nil
	}

	var archived []db.Session
	var failures []string
	for _, s := range matched {
		if err := t.db.SetSessionState(ctx, s.ID, "archived"); err != nil {
			failures = append(failures, fmt.Sprintf("%s (%s): %v", s.ID, sessClip(strings.TrimSpace(s.Title), 40), err))
			continue
		}
		archived = append(archived, s)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Archived %d session(s) in this workspace (soft, reversible):\n", len(archived))
	writeSessionLines(&b, archived, now)
	if capped {
		fmt.Fprintf(&b, "\n(capped at limit=%d; run again to archive the rest)", args.Limit)
	}
	if len(failures) > 0 {
		fmt.Fprintf(&b, "\n%d failed:\n- %s", len(failures), strings.Join(failures, "\n- "))
	}
	return strings.TrimSpace(b.String()), nil
}

// writeSessionLines renders a compact one-line-per-session listing shared by the
// dry-run and applied paths.
func writeSessionLines(b *strings.Builder, sessions []db.Session, now int64) {
	for _, s := range sessions {
		title := strings.TrimSpace(s.Title)
		if title == "" {
			title = "(untitled)"
		}
		fmt.Fprintf(b, "- %s · %q · %d msg · %s\n", s.ID, sessClip(title, 60), s.MessageCount, sessAge(now-s.UpdatedAt))
	}
}
