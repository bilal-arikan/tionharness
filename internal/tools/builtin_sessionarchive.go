package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// defaultArchiveLimit caps how many sessions a single archive_sessions call may
// touch, so a too-broad filter can't sweep an entire workspace in one shot. The
// agent can raise it explicitly (or page through with tighter filters).
const defaultArchiveLimit = 100

// ArchiveSessionsTool bulk-archives OTHER sessions in THIS workspace. It is
// the "manage other sessions" complement to update_session (which only edits the
// current session): update_session archives the session the agent runs in, this
// tool archives many sibling sessions at once, filtered by kind/age/title.
//
// Session kind: by default only "chat" sessions match (see defaultArchiveKinds),
// which is the behaviour this tool shipped with. Autonomous runs (spawn, flow,
// task, schedule, inbox, worker) produce their own session kinds and used to be
// unreachable by any bulk tool; pass `kinds` to include them, or kinds:["*"] for
// every kind.
//
// Why it exists: without it an agent that is asked to "clean up old sessions" has
// to drop to raw REST (Invoke-RestMethod against /api/sessions/{id}/state), which
// loses the automatic workspace scoping the built-in tools get for free — the
// exact footgun that once archived the wrong (default) workspace. This tool binds
// to the workspace DB (r.db), so it is physically scoped to this workspace. By
// default it excludes the current session (so a "clean up old sessions" sweep
// cannot self-archive by accident); pass include_current:true to opt in and archive
// the running session too. Archiving is soft/reversible (state="archived" → restore
// with "active"); it only leaves the active list, it does not stop the running turn.
type ArchiveSessionsTool struct {
	db *db.DB
	// currentSessionID is resolved at registry-build time. It is excluded from
	// archiving by default; a caller can opt in with include_current:true.
	currentSessionID string
}

// NewArchiveSessionsTool binds the tool to a workspace DB and the current session
// id (excluded from archiving unless the caller passes include_current:true).
func NewArchiveSessionsTool(database *db.DB, currentSessionID string) ArchiveSessionsTool {
	return ArchiveSessionsTool{db: database, currentSessionID: currentSessionID}
}

func (ArchiveSessionsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "archive_sessions",
		Description: "Bulk-archive sessions in THIS workspace (soft, reversible). Use this " +
			"to clean up old/finished sessions instead of raw HTTP calls — it is scoped to this " +
			"workspace and by default excludes the session you are running in. " +
			"By default it only matches `chat` sessions; pass `kinds` to also sweep the sessions " +
			"autonomous runs leave behind (spawned/worker/flow/task/schedule/inbox), e.g. " +
			"`kinds:[\"spawned\",\"flow\"]`, or `kinds:[\"*\"]` for every kind. " +
			"Filter further with `idle_days` (only sessions whose last activity is older than N days) " +
			"and/or `title_contains`. " +
			"Set `include_current` true to also archive the session you are running in (it stays " +
			"reversible and the current turn keeps running). " +
			"Set `dry_run` true first to preview exactly which sessions would be archived, then run " +
			"again without it to apply. Archiving only leaves the active list; restore via the UI or " +
			"by setting a session back to active. Use list_sessions to inspect candidates first.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "kinds":           { "type": "array", "items": { "type": "string" }, "description": "Session kinds to archive: chat, spawned, worker, flow, task, schedule, inbox — or [\"*\"] for all of them. Omitted = [\"chat\"] only (the historical behaviour). An unknown kind is an error. Note: schedule and inbox sessions are long-lived/system-owned, so \"*\" sweeps them too — preview with dry_run first." },
    "idle_days":       { "type": "integer", "description": "Only archive sessions whose last activity is older than this many days. 0 or omitted = no age filter (all active sessions match)." },
    "title_contains":  { "type": "string", "description": "Only archive sessions whose title contains this text (case-insensitive)." },
    "exclude":         { "type": "array", "items": { "type": "string" }, "description": "Session IDs to keep active." },
    "include_current": { "type": "boolean", "description": "Also archive the session you are running in (default false = keep it active). The exclude list still applies." },
    "dry_run":         { "type": "boolean", "description": "Preview only: list the sessions that WOULD be archived without changing anything." },
    "limit":           { "type": "integer", "description": "Safety cap on how many sessions to archive in one call (default 100). Matches beyond the cap are reported but left untouched." }
  },
  "additionalProperties": false
}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"idle_days":2,"dry_run":true}`),
			json.RawMessage(`{"idle_days":2}`),
			json.RawMessage(`{"kinds":["spawned","flow"],"idle_days":2,"dry_run":true}`),
			json.RawMessage(`{"kinds":["*"],"idle_days":7,"dry_run":true}`),
			json.RawMessage(`{"title_contains":"test","dry_run":true}`),
		},
	}
}

func (t ArchiveSessionsTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Kinds          []string `json:"kinds"`
		IdleDays       int      `json:"idle_days"`
		TitleContains  string   `json:"title_contains"`
		Exclude        []string `json:"exclude"`
		IncludeCurrent bool     `json:"include_current"`
		DryRun         bool     `json:"dry_run"`
		Limit          int      `json:"limit"`
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
	// Defaults to chat-only; an unknown kind fails loudly rather than matching nothing.
	wantKinds, err := resolveArchiveKinds(args.Kinds)
	if err != nil {
		return "", err
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
	// The current session is excluded by default so a broad "clean up old sessions"
	// sweep cannot self-archive by accident; include_current:true opts in. "" (catalog/
	// preview build) matches no real id, so this is a no-op there either way.
	if t.currentSessionID != "" && !args.IncludeCurrent {
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
		kind := s.Kind
		if kind == "" { // zero-value rows predate the kind stamp; the store defaults them to chat
			kind = sessionKindChat
		}
		if _, want := wantKinds[kind]; !want {
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

	kindList := sortedKinds(wantKinds)

	if len(matched) == 0 {
		note := fmt.Sprintf("No active sessions of kind [%s] match — nothing to archive.", kindList)
		if t.currentSessionID != "" && !args.IncludeCurrent {
			note += " (The current session is excluded; pass include_current:true to archive it too.)"
		}
		if len(args.Kinds) == 0 {
			note += " (Only `chat` sessions are matched by default; pass `kinds` — e.g. [\"spawned\",\"flow\"] or [\"*\"] — to sweep other kinds.)"
		}
		return note, nil
	}

	capped := false
	if len(matched) > args.Limit {
		matched = matched[:args.Limit]
		capped = true
	}

	if args.DryRun {
		var b strings.Builder
		fmt.Fprintf(&b, "DRY RUN — %d session(s) of kind [%s] WOULD be archived (nothing changed):\n", len(matched), kindList)
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
	fmt.Fprintf(&b, "Archived %d session(s) of kind [%s] in this workspace (soft, reversible):\n", len(archived), kindList)
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
// dry-run and applied paths. The kind is spelled out on every line so a dry-run
// preview shows exactly WHAT is about to be swept, not just how many.
func writeSessionLines(b *strings.Builder, sessions []db.Session, now int64) {
	for _, s := range sessions {
		title := strings.TrimSpace(s.Title)
		if title == "" {
			title = "(untitled)"
		}
		kind := s.Kind
		if kind == "" {
			kind = sessionKindChat
		}
		fmt.Fprintf(b, "- %s · [%s] · %q · %d msg · %s\n", s.ID, kind, sessClip(title, 60), s.MessageCount, sessAge(now-s.UpdatedAt))
	}
}
